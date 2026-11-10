package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/shadowstar/shadowstar/pkg/protocol"
)

// Config represents the server configuration
type Config struct {
	ListenAddr     string `json:"listen_addr"`
	MasterKey      string `json:"master_key"`      // 32-byte hex or plain string
	FallbackTarget string `json:"fallback_target"` // e.g. "www.microsoft.com:443"
	SNI            string `json:"sni"`             // expected SNI or allowed decoy
	Verbose        bool   `json:"verbose"`
}

// Server is the ShadowStar proxy server
type Server struct {
	config     *Config
	masterKey  []byte
	legacyKey  []byte
	listener   net.Listener
	mu         sync.Mutex
	closed     bool
	statsActive int64
}

// NewServer creates a new ShadowStar server
func NewServer(cfg *Config) (*Server, error) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":443"
	}
	if cfg.FallbackTarget == "" {
		cfg.FallbackTarget = "www.microsoft.com:443"
	}

	primaryKey, legacyKey := protocol.NormalizeMasterKey(cfg.MasterKey)

	return &Server{
		config:    cfg,
		masterKey: primaryKey,
		legacyKey: legacyKey,
	}, nil
}

// Start runs the server accept loop
func (s *Server) Start() error {
	l, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.ListenAddr, err)
	}
	s.listener = l
	log.Printf("[ShadowStar-Server] Listening on %s | Fallback: %s", s.config.ListenAddr, s.config.FallbackTarget)

	for {
		conn, err := l.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			log.Printf("[ShadowStar-Server] Accept error: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// Close stops the server
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// Read initial TLS record header (5 bytes)
	header := make([]byte, 5)
	_ = clientConn.SetReadDeadline(time.Now().Add(7 * time.Second))
	if _, err := io.ReadFull(clientConn, header); err != nil {
		return
	}

	// Check if this looks like a TLS handshake (0x16 0x03 ...)
	isTLS := (header[0] == protocol.TLSRecordHandshake && header[1] == 0x03)
	if !isTLS {
		// Not TLS - redirect immediately to fallback decoy
		s.redirectToFallback(clientConn, header)
		return
	}

	recordLen := int(binary.BigEndian.Uint16(header[3:5]))
	if recordLen > 16384 {
		s.redirectToFallback(clientConn, header)
		return
	}

	body := make([]byte, recordLen)
	if _, err := io.ReadFull(clientConn, body); err != nil {
		s.redirectToFallback(clientConn, header)
		return
	}
	_ = clientConn.SetReadDeadline(time.Time{})

	fullHello := append(header, body...)
	parsed, err := protocol.ParseClientHello(fullHello)
	if err != nil || parsed == nil || len(parsed.ClientPub) != 32 {
		// Malformed or unknown TLS handshake -> Active Probing decoy redirect!
		s.redirectToFallback(clientConn, fullHello)
		return
	}

	// Verify authentication tag embedded in TLS SessionID (with legacy key fallback)
	activeKey := s.masterKey
	validAuth := protocol.VerifyAuthSessionID(parsed.SessionID, activeKey, parsed.ClientPub, parsed.SNI)
	if !validAuth && len(s.legacyKey) > 0 {
		if protocol.VerifyAuthSessionID(parsed.SessionID, s.legacyKey, parsed.ClientPub, parsed.SNI) {
			validAuth = true
			activeKey = s.legacyKey
		}
	}
	if !validAuth {
		if s.config.Verbose {
			log.Printf("[Probe-Shield] Active probe or unauthenticated TLS from %s (SNI: %s). Redirecting to %s",
				clientConn.RemoteAddr(), parsed.SNI, s.config.FallbackTarget)
		}
		s.redirectToFallback(clientConn, fullHello)
		return
	}

	// Authenticated client!
	if s.config.Verbose {
		log.Printf("[Auth-OK] Valid ShadowStar handshake from %s (SNI: %s)", clientConn.RemoteAddr(), parsed.SNI)
	}

	// Generate Server's Ephemeral Keypair
	serverPriv, serverPub, err := protocol.GenerateX25519KeyPair()
	if err != nil {
		log.Printf("[ShadowStar-Server] KeyGen error: %v", err)
		return
	}

	// Compute Shared Secret via ECDH
	sharedSecret, err := protocol.ComputeECDH(serverPriv, parsed.ClientPub)
	if err != nil {
		log.Printf("[ShadowStar-Server] ECDH error: %v", err)
		return
	}

	// Derive Bidirectional Session Keys
	clientKey, serverKey, clientIV, serverIV, err := protocol.DeriveSessionKeys(sharedSecret, activeKey)
	if err != nil {
		log.Printf("[ShadowStar-Server] DeriveSessionKeys error: %v", err)
		return
	}

	// Send ServerHello to complete TLS mimicry
	serverHello := protocol.BuildServerHello(serverPub, parsed.SessionID)
	if _, err := clientConn.Write(serverHello); err != nil {
		log.Printf("[ShadowStar-Server] ServerHello write error: %v", err)
		return
	}

	// Initialize secure frame connection
	frameConn, err := protocol.NewSecureFrameConn(clientConn, serverKey, serverIV, clientKey, clientIV)
	if err != nil {
		log.Printf("[ShadowStar-Server] FrameConn error: %v", err)
		return
	}

	// Initialize multiplexing session
	muxSession := protocol.NewMuxSession(frameConn, clientConn, false)
	defer muxSession.Close()

	for {
		stream, err := muxSession.AcceptStream()
		if err != nil {
			return
		}

		go s.handleMuxStream(stream)
	}
}

func isBlockedTarget(host string) bool {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "" || h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	if ip != nil {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return true
		}
		// Block 169.254.169.254 (Cloud instance metadata API)
		if ip.Equal(net.ParseIP("169.254.169.254")) {
			return true
		}
	}
	return false
}

func (s *Server) handleMuxStream(stream *protocol.MuxStream) {
	defer stream.Close()

	// Read destination address
	target, err := protocol.ReadTargetAddress(stream)
	if err != nil {
		return
	}

	if isBlockedTarget(target.Host) {
		if s.config.Verbose {
			log.Printf("[Proxy-Block] Blocked restricted loopback/metadata target: %s", target.Host)
		}
		return
	}

	if target.Network == protocol.NetUDP {
		s.handleUDPStream(stream, target)
		return
	}

	destAddr := target.String()
	if s.config.Verbose {
		log.Printf("[Proxy-Route] Routing -> %s", destAddr)
	}

	targetConn, err := net.DialTimeout("tcp", destAddr, 10*time.Second)
	if err != nil {
		if s.config.Verbose {
			log.Printf("[Proxy-Route] Dial failed to %s: %v", destAddr, err)
		}
		return
	}

	var once sync.Once
	closeBoth := func() {
		_ = targetConn.Close()
		_ = stream.Close()
	}

	// Bidirectional data pipe
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer once.Do(closeBoth)
		buf := make([]byte, 32768)
		_, _ = io.CopyBuffer(targetConn, stream, buf)
	}()

	go func() {
		defer wg.Done()
		defer once.Do(closeBoth)
		buf := make([]byte, 32768)
		_, _ = io.CopyBuffer(stream, targetConn, buf)
	}()

	wg.Wait()
}

func (s *Server) handleUDPStream(stream *protocol.MuxStream, target *protocol.TargetAddress) {
	defer stream.Close()

	if isBlockedTarget(target.Host) {
		return
	}

	destAddr := target.String()
	if s.config.Verbose {
		log.Printf("[UDP-Route] Routing -> %s", destAddr)
	}

	udpConn, err := net.DialTimeout("udp", destAddr, 10*time.Second)
	if err != nil {
		if s.config.Verbose {
			log.Printf("[UDP-Route] Dial failed to %s: %v", destAddr, err)
		}
		return
	}
	defer udpConn.Close()

	var once sync.Once
	closeBoth := func() {
		_ = udpConn.Close()
		_ = stream.Close()
	}

	idleTimeout := 60 * time.Second
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream -> UDP
	go func() {
		defer wg.Done()
		defer once.Do(closeBoth)

		lenBuf := make([]byte, 2)
		for {
			_ = stream.SetReadDeadline(time.Now().Add(idleTimeout))
			if _, err := io.ReadFull(stream, lenBuf); err != nil {
				return
			}
			pktLen := int(binary.BigEndian.Uint16(lenBuf))
			if pktLen <= 0 || pktLen > 65535 {
				return
			}
			pkt := make([]byte, pktLen)
			if _, err := io.ReadFull(stream, pkt); err != nil {
				return
			}
			_ = udpConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := udpConn.Write(pkt); err != nil {
				return
			}
		}
	}()

	// UDP -> Stream
	go func() {
		defer wg.Done()
		defer once.Do(closeBoth)

		buf := make([]byte, 65535)
		for {
			_ = udpConn.SetReadDeadline(time.Now().Add(idleTimeout))
			n, err := udpConn.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				frame := make([]byte, 2+n)
				binary.BigEndian.PutUint16(frame[0:2], uint16(n))
				copy(frame[2:], buf[:n])
				_ = stream.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if _, err := stream.Write(frame); err != nil {
					return
				}
			}
		}
	}()

	wg.Wait()
}

// redirectToFallback handles active probing by transparently piping traffic to a real benign web server
func (s *Server) redirectToFallback(clientConn net.Conn, initialBytes []byte) {
	fallbackConn, err := net.DialTimeout("tcp", s.config.FallbackTarget, 5*time.Second)
	if err != nil {
		return
	}

	// Write initial peeked bytes to the real server
	if len(initialBytes) > 0 {
		if _, err := fallbackConn.Write(initialBytes); err != nil {
			fallbackConn.Close()
			return
		}
	}

	var once sync.Once
	closeBoth := func() {
		_ = fallbackConn.Close()
		_ = clientConn.Close()
	}

	// Transparent bridge
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer once.Do(closeBoth)
		buf := make([]byte, 32768)
		_, _ = io.CopyBuffer(fallbackConn, clientConn, buf)
	}()

	go func() {
		defer wg.Done()
		defer once.Do(closeBoth)
		buf := make([]byte, 32768)
		_, _ = io.CopyBuffer(clientConn, fallbackConn, buf)
	}()

	wg.Wait()
}
