package protocol

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DTLS Record Header Constants for Deep Packet Inspection Camouflage
	DTLSContentTypeChangeCipherSpec = 0x14
	DTLSContentTypeAlert            = 0x15
	DTLSContentTypeHandshake        = 0x16
	DTLSContentTypeApplicationData  = 0x17

	DTLSVersion12Major = 0xfe
	DTLSVersion12Minor = 0xfd

	// Transport Control Flags
	FlagSYN  byte = 0x01
	FlagACK  byte = 0x02
	FlagDATA byte = 0x04
	FlagFIN  byte = 0x08
	FlagPING byte = 0x10

	MaxUDPPacketSize = 1280
	UDPHeaderSize    = 18
	MaxPayloadPerPkt = MaxUDPPacketSize - UDPHeaderSize // 1262 bytes
	DefaultRTO       = 100 * time.Millisecond
	MaxRTO           = 800 * time.Millisecond
	KeepalivePeriod  = 10 * time.Second
	SessionIdleTimeout = 60 * time.Second
)

var (
	ErrSessionClosed   = errors.New("udp session closed")
	ErrSessionTimeout  = errors.New("udp session timed out")
	ErrListenerClosed = errors.New("udp listener closed")
)

// unackedPacket tracks sent packets awaiting acknowledgment
type unackedPacket struct {
	seq    uint32
	wire   []byte
	sentAt time.Time
	retries int
}

// UDPSession implements net.Conn over reliable, ordered, DTLS-camouflaged UDP
type UDPSession struct {
	rawConn   *net.UDPConn
	raddr     *net.UDPAddr
	laddr     net.Addr
	sessionID uint32
	isServer  bool

	// Send state
	sendMu    sync.Mutex
	sendNext  uint32
	unacked   map[uint32]*unackedPacket
	rto       time.Duration

	// Receive state
	recvMu    sync.Mutex
	recvNext  uint32
	recvBuf   map[uint32][]byte
	readBuf   bytes.Buffer
	readCond  *sync.Cond

	// Lifecycle
	closed       atomic.Bool
	closeOnce    sync.Once
	closeChan    chan struct{}
	lastActivity atomic.Int64 // Unix nano
	onClose      func(sessID uint32, addr string)

	// Deadlines
	readDeadline  atomic.Int64 // Unix nano, 0 = none
	writeDeadline atomic.Int64 // Unix nano, 0 = none
	readTimer     *time.Timer
	readTimedOut  atomic.Bool
}

func newUDPSession(rawConn *net.UDPConn, raddr *net.UDPAddr, sessID uint32, isServer bool, onClose func(uint32, string)) *UDPSession {
	s := &UDPSession{
		rawConn:   rawConn,
		raddr:     raddr,
		laddr:     rawConn.LocalAddr(),
		sessionID: sessID,
		isServer:  isServer,
		sendNext:  1,
		unacked:   make(map[uint32]*unackedPacket),
		rto:       DefaultRTO,
		recvNext:  1,
		recvBuf:   make(map[uint32][]byte),
		closeChan: make(chan struct{}),
		onClose:   onClose,
	}
	s.readCond = sync.NewCond(&s.recvMu)
	s.lastActivity.Store(time.Now().UnixNano())

	go s.retransmitLoop()
	go s.keepaliveLoop()

	return s
}

// buildPacket constructs an 18-byte DTLS-camouflaged header followed by payload
func (s *UDPSession) buildPacket(seq uint32, ack uint32, flags byte, payload []byte) []byte {
	cType := byte(DTLSContentTypeApplicationData)
	if flags&FlagSYN != 0 {
		cType = DTLSContentTypeHandshake
	} else if flags&FlagFIN != 0 {
		cType = DTLSContentTypeAlert
	} else if flags&FlagPING != 0 {
		cType = DTLSContentTypeChangeCipherSpec
	}

	payloadLen := len(payload)
	pkt := make([]byte, UDPHeaderSize+payloadLen)
	pkt[0] = cType
	pkt[1] = DTLSVersion12Major
	pkt[2] = DTLSVersion12Minor
	binary.BigEndian.PutUint32(pkt[3:7], s.sessionID)
	binary.BigEndian.PutUint32(pkt[7:11], seq)
	binary.BigEndian.PutUint32(pkt[11:15], ack)
	pkt[15] = flags
	binary.BigEndian.PutUint16(pkt[16:18], uint16(payloadLen))

	if payloadLen > 0 {
		copy(pkt[UDPHeaderSize:], payload)
	}
	return pkt
}

func (s *UDPSession) sendRaw(wire []byte) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}
	var err error
	if s.isServer {
		_, err = s.rawConn.WriteToUDP(wire, s.raddr)
	} else {
		_, err = s.rawConn.Write(wire)
	}
	return err
}

// Write writes data to the reliable UDP stream
func (s *UDPSession) Write(p []byte) (n int, err error) {
	if s.closed.Load() {
		return 0, ErrSessionClosed
	}
	if len(p) == 0 {
		return 0, nil
	}

	total := len(p)
	offset := 0

	for offset < total {
		if s.closed.Load() {
			return offset, ErrSessionClosed
		}

		// Check write deadline
		if dl := s.writeDeadline.Load(); dl > 0 && time.Now().UnixNano() > dl {
			return offset, fmt.Errorf("write timeout: %w", net.ErrClosed)
		}

		chunkSize := total - offset
		if chunkSize > MaxPayloadPerPkt {
			chunkSize = MaxPayloadPerPkt
		}
		chunk := p[offset : offset+chunkSize]

		s.recvMu.Lock()
		currentAck := s.recvNext - 1
		s.recvMu.Unlock()

		s.sendMu.Lock()
		seq := s.sendNext
		s.sendNext++

		pktBytes := s.buildPacket(seq, currentAck, FlagDATA|FlagACK, chunk)
		s.unacked[seq] = &unackedPacket{
			seq:    seq,
			wire:   pktBytes,
			sentAt: time.Now(),
		}
		s.sendMu.Unlock()

		if err := s.sendRaw(pktBytes); err != nil {
			return offset, err
		}

		offset += chunkSize
	}

	return total, nil
}

// Read reads data from the reliable UDP stream
func (s *UDPSession) Read(p []byte) (n int, err error) {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	for s.readBuf.Len() == 0 {
		if s.closed.Load() {
			return 0, io.EOF
		}

		if s.readTimedOut.Load() {
			return 0, fmt.Errorf("read timeout: %w", net.ErrClosed)
		}

		s.readCond.Wait()

		if s.closed.Load() && s.readBuf.Len() == 0 {
			return 0, io.EOF
		}
		if s.readTimedOut.Load() && s.readBuf.Len() == 0 {
			return 0, fmt.Errorf("read timeout: %w", net.ErrClosed)
		}
	}

	return s.readBuf.Read(p)
}

// handlePacket processes an incoming datagram
func (s *UDPSession) handlePacket(data []byte) {
	if len(data) < UDPHeaderSize {
		return
	}

	sessID := binary.BigEndian.Uint32(data[3:7])
	if sessID != s.sessionID {
		return
	}

	seq := binary.BigEndian.Uint32(data[7:11])
	ack := binary.BigEndian.Uint32(data[11:15])
	flags := data[15]
	payloadLen := int(binary.BigEndian.Uint16(data[16:18]))

	if len(data) < UDPHeaderSize+payloadLen {
		return
	}
	payload := data[UDPHeaderSize : UDPHeaderSize+payloadLen]

	s.lastActivity.Store(time.Now().UnixNano())

	// Handle ACK
	if ack > 0 {
		s.sendMu.Lock()
		for unackSeq := range s.unacked {
			if unackSeq <= ack {
				delete(s.unacked, unackSeq)
			}
		}
		s.sendMu.Unlock()
	}

	// Handle FIN
	if flags&FlagFIN != 0 {
		s.Close()
		return
	}

	// Handle PING
	if flags&FlagPING != 0 {
		s.sendAckOnly()
		return
	}

	// Handle Data / SYN payload
	if payloadLen > 0 && (flags&FlagDATA != 0 || flags&FlagSYN != 0) {
		s.recvMu.Lock()
		if seq == s.recvNext {
			s.readBuf.Write(payload)
			s.recvNext++

			// Drain buffered consecutive sequences
			for {
				nextPayload, found := s.recvBuf[s.recvNext]
				if !found {
					break
				}
				s.readBuf.Write(nextPayload)
				delete(s.recvBuf, s.recvNext)
				s.recvNext++
			}
			s.readCond.Broadcast()
		} else if seq > s.recvNext {
			// Out-of-order packet: buffer if within reasonable window
			if seq < s.recvNext+1024 {
				s.recvBuf[seq] = append([]byte(nil), payload...)
			}
		}
		currentAck := s.recvNext - 1
		s.recvMu.Unlock()

		// Always reply with current cumulative ACK
		s.sendAck(currentAck)
	} else if flags&FlagSYN != 0 {
		// Bare SYN
		s.sendAckOnly()
	}
}

func (s *UDPSession) sendAck(ackSeq uint32) {
	ackPkt := s.buildPacket(0, ackSeq, FlagACK, nil)
	_ = s.sendRaw(ackPkt)
}

func (s *UDPSession) sendAckOnly() {
	s.recvMu.Lock()
	currentAck := s.recvNext - 1
	s.recvMu.Unlock()
	s.sendAck(currentAck)
}

// retransmitLoop handles packet retransmission on timeout
func (s *UDPSession) retransmitLoop() {
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.closeChan:
			return
		case <-ticker.C:
			s.sendMu.Lock()
			now := time.Now()
			for _, unack := range s.unacked {
				if now.Sub(unack.sentAt) >= s.rto {
					_ = s.sendRaw(unack.wire)
					unack.sentAt = now
					unack.retries++
					if s.rto < MaxRTO {
						s.rto += 20 * time.Millisecond
					}
				}
			}
			s.sendMu.Unlock()
		}
	}
}

// keepaliveLoop manages periodic pings and idle timeouts
func (s *UDPSession) keepaliveLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.closeChan:
			return
		case <-ticker.C:
			last := time.Unix(0, s.lastActivity.Load())
			if time.Since(last) > SessionIdleTimeout {
				s.Close()
				return
			}
			if time.Since(last) >= KeepalivePeriod {
				s.recvMu.Lock()
				ack := s.recvNext - 1
				s.recvMu.Unlock()
				ping := s.buildPacket(0, ack, FlagPING, nil)
				_ = s.sendRaw(ping)
			}
		}
	}
}

// Close closes the UDP session gracefully
func (s *UDPSession) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.closeChan)

		// Send FIN packet
		s.recvMu.Lock()
		ack := s.recvNext - 1
		s.recvMu.Unlock()
		fin := s.buildPacket(0, ack, FlagFIN, nil)
		_ = s.sendRaw(fin)

		s.recvMu.Lock()
		s.readCond.Broadcast()
		s.recvMu.Unlock()

		if !s.isServer && s.rawConn != nil {
			_ = s.rawConn.Close()
		}

		if s.onClose != nil {
			s.onClose(s.sessionID, s.raddr.String())
		}
	})
	return nil
}

func (s *UDPSession) LocalAddr() net.Addr                { return s.laddr }
func (s *UDPSession) RemoteAddr() net.Addr               { return s.raddr }
func (s *UDPSession) SetDeadline(t time.Time) error      { _ = s.SetReadDeadline(t); return s.SetWriteDeadline(t) }
func (s *UDPSession) SetReadDeadline(t time.Time) error {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	if s.readTimer != nil {
		s.readTimer.Stop()
		s.readTimer = nil
	}

	if t.IsZero() {
		s.readDeadline.Store(0)
		s.readTimedOut.Store(false)
		return nil
	}

	s.readDeadline.Store(t.UnixNano())
	d := time.Until(t)
	if d <= 0 {
		s.readTimedOut.Store(true)
		s.readCond.Broadcast()
		return nil
	}

	s.readTimedOut.Store(false)
	s.readTimer = time.AfterFunc(d, func() {
		s.readTimedOut.Store(true)
		s.recvMu.Lock()
		s.readCond.Broadcast()
		s.recvMu.Unlock()
	})
	return nil
}
func (s *UDPSession) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		s.writeDeadline.Store(0)
	} else {
		s.writeDeadline.Store(t.UnixNano())
	}
	return nil
}

// --- UDP Listener (Server-Side) ---

// UDPListener listens on a UDP port and multiplexes incoming client sessions
type UDPListener struct {
	conn       *net.UDPConn
	sessions   map[string]*UDPSession
	mu         sync.RWMutex
	acceptChan chan *UDPSession
	closed     atomic.Bool
	closeChan  chan struct{}
}

// ListenUDP starts listening on the specified UDP address
func ListenUDP(addr string) (*UDPListener, error) {
	uaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve udp addr: %w", err)
	}

	conn, err := net.ListenUDP("udp", uaddr)
	if err != nil {
		return nil, fmt.Errorf("listen udp on %s: %w", addr, err)
	}

	l := &UDPListener{
		conn:       conn,
		sessions:   make(map[string]*UDPSession),
		acceptChan: make(chan *UDPSession, 128),
		closeChan:  make(chan struct{}),
	}

	go l.readLoop()
	return l, nil
}

func (l *UDPListener) sessionKey(raddr *net.UDPAddr, sessID uint32) string {
	return fmt.Sprintf("%s-%d", raddr.String(), sessID)
}

func (l *UDPListener) readLoop() {
	buf := make([]byte, 2048)
	for {
		if l.closed.Load() {
			return
		}

		n, raddr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			if l.closed.Load() {
				return
			}
			continue
		}

		if n < UDPHeaderSize {
			continue
		}

		sessID := binary.BigEndian.Uint32(buf[3:7])
		flags := buf[15]
		key := l.sessionKey(raddr, sessID)

		l.mu.RLock()
		sess, exists := l.sessions[key]
		l.mu.RUnlock()

		if !exists {
			// New session candidate
			if flags&FlagSYN != 0 {
				sess = newUDPSession(l.conn, raddr, sessID, true, func(id uint32, addr string) {
					l.mu.Lock()
					delete(l.sessions, fmt.Sprintf("%s-%d", addr, id))
					l.mu.Unlock()
				})

				l.mu.Lock()
				l.sessions[key] = sess
				l.mu.Unlock()

				select {
				case l.acceptChan <- sess:
				default:
					sess.Close()
					continue
				}
			} else {
				// Stray packet from unknown session: ignore
				continue
			}
		}

		// Forward packet to session
		pktCopy := make([]byte, n)
		copy(pktCopy, buf[:n])
		sess.handlePacket(pktCopy)
	}
}

// Accept waits for and returns the next client connection
func (l *UDPListener) Accept() (net.Conn, error) {
	select {
	case <-l.closeChan:
		return nil, ErrListenerClosed
	case sess, ok := <-l.acceptChan:
		if !ok {
			return nil, ErrListenerClosed
		}
		return sess, nil
	}
}

// Close closes the UDP listener and all child sessions
func (l *UDPListener) Close() error {
	if l.closed.CompareAndSwap(false, true) {
		close(l.closeChan)
		_ = l.conn.Close()

		l.mu.Lock()
		sessionsToClose := make([]*UDPSession, 0, len(l.sessions))
		for _, sess := range l.sessions {
			sessionsToClose = append(sessionsToClose, sess)
		}
		l.sessions = make(map[string]*UDPSession)
		l.mu.Unlock()

		for _, sess := range sessionsToClose {
			sess.Close()
		}
	}
	return nil
}

func (l *UDPListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

// --- UDP Dialer (Client-Side) ---

// DialUDP connects to a remote UDP address, establishing a reliable UDPSession
func DialUDP(remoteAddr string) (net.Conn, error) {
	raddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve remote udp: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("dial udp to %s: %w", remoteAddr, err)
	}

	// Generate random non-zero SessionID
	nBig, _ := rand.Int(rand.Reader, big.NewInt(0x7fffffff))
	sessID := uint32(nBig.Int64() + 1)

	sess := newUDPSession(conn, raddr, sessID, false, nil)

	// Send initial SYN packet to wake up and initiate session on server
	synPkt := sess.buildPacket(0, 0, FlagSYN, nil)
	if err := sess.sendRaw(synPkt); err != nil {
		sess.Close()
		return nil, fmt.Errorf("send initial syn: %w", err)
	}

	// Read loop for client UDP connection
	go func() {
		buf := make([]byte, 2048)
		for {
			if sess.closed.Load() {
				return
			}
			n, err := conn.Read(buf)
			if err != nil {
				sess.Close()
				return
			}
			if n >= UDPHeaderSize {
				pktCopy := make([]byte, n)
				copy(pktCopy, buf[:n])
				sess.handlePacket(pktCopy)
			}
		}
	}()

	return sess, nil
}
