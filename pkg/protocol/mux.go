package protocol

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// MuxStream represents a single virtual connection multiplexed over a SecureFrameConn
type MuxStream struct {
	id           uint32
	session      *MuxSession
	readBuf      chan []byte
	readRem      []byte
	closeOnce    sync.Once
	closed       atomic.Bool
	closeChan    chan struct{}
	readDeadline atomic.Int64 // UnixNano timestamp; 0 = no deadline
}

func (s *MuxStream) Read(b []byte) (n int, err error) {
	if len(s.readRem) > 0 {
		n = copy(b, s.readRem)
		s.readRem = s.readRem[n:]
		return n, nil
	}

	dl := s.readDeadline.Load()
	var timer *time.Timer
	var timeoutChan <-chan time.Time
	if dl > 0 {
		rem := time.Duration(dl - time.Now().UnixNano())
		if rem <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		timer = time.NewTimer(rem)
		defer timer.Stop()
		timeoutChan = timer.C
	}

	select {
	case chunk, ok := <-s.readBuf:
		if !ok || chunk == nil {
			return 0, io.EOF
		}
		n = copy(b, chunk)
		if n < len(chunk) {
			s.readRem = chunk[n:]
		}
		return n, nil
	case <-s.closeChan:
		// Drain any remaining buffered chunks
		select {
		case chunk, ok := <-s.readBuf:
			if ok && len(chunk) > 0 {
				n = copy(b, chunk)
				if n < len(chunk) {
					s.readRem = chunk[n:]
				}
				return n, nil
			}
		default:
		}
		return 0, io.EOF
	case <-timeoutChan:
		return 0, os.ErrDeadlineExceeded
	}
}

func (s *MuxStream) Write(b []byte) (n int, err error) {
	if s.closed.Load() {
		return 0, errors.New("stream closed")
	}

	total := len(b)
	for len(b) > 0 {
		chunkSize := len(b)
		if chunkSize > MaxPayloadSize {
			chunkSize = MaxPayloadSize
		}

		err := s.session.frameConn.WriteFrame(s.id, FrameData, b[:chunkSize])
		if err != nil {
			return total - len(b), err
		}
		b = b[chunkSize:]
	}
	return total, nil
}

func (s *MuxStream) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.closeChan)
		_ = s.session.frameConn.WriteFrame(s.id, FrameClose, nil)
		s.session.removeStream(s.id)
	})
	return nil
}

func (s *MuxStream) LocalAddr() net.Addr                { return s.session.LocalAddr() }
func (s *MuxStream) RemoteAddr() net.Addr               { return s.session.RemoteAddr() }
func (s *MuxStream) SetDeadline(t time.Time) error      { return s.SetReadDeadline(t) }
func (s *MuxStream) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		s.readDeadline.Store(0)
	} else {
		s.readDeadline.Store(t.UnixNano())
	}
	return nil
}
func (s *MuxStream) SetWriteDeadline(t time.Time) error { return nil }

// MuxSession coordinates multiplexed streams over a SecureFrameConn
type MuxSession struct {
	frameConn  *SecureFrameConn
	underlying net.Conn
	isClient   bool
	nextID     atomic.Uint32
	streams    sync.Map // map[uint32]*MuxStream
	acceptChan chan *MuxStream
	closeOnce  sync.Once
	closed     atomic.Bool
}

// NewMuxSession creates a multiplexing session
func NewMuxSession(frameConn *SecureFrameConn, underlying net.Conn, isClient bool) *MuxSession {
	sess := &MuxSession{
		frameConn:  frameConn,
		underlying: underlying,
		isClient:   isClient,
		acceptChan: make(chan *MuxStream, 512),
	}
	if isClient {
		sess.nextID.Store(1)
	} else {
		sess.nextID.Store(2)
	}

	go sess.readLoop()
	go sess.heartbeatLoop()
	return sess
}

func (s *MuxSession) LocalAddr() net.Addr  { return s.underlying.LocalAddr() }
func (s *MuxSession) RemoteAddr() net.Addr { return s.underlying.RemoteAddr() }
func (s *MuxSession) IsClosed() bool       { return s.closed.Load() }

func (s *MuxSession) OpenStream() (*MuxStream, error) {
	if s.closed.Load() {
		return nil, errors.New("mux session closed")
	}

	id := s.nextID.Add(2) - 2
	stream := &MuxStream{
		id:        id,
		session:   s,
		readBuf:   make(chan []byte, 512),
		closeChan: make(chan struct{}),
	}
	s.streams.Store(id, stream)

	err := s.frameConn.WriteFrame(id, FrameOpen, nil)
	if err != nil {
		s.removeStream(id)
		return nil, err
	}
	return stream, nil
}

func (s *MuxSession) AcceptStream() (*MuxStream, error) {
	stream, ok := <-s.acceptChan
	if !ok || stream == nil {
		return nil, errors.New("mux session closed")
	}
	return stream, nil
}

func (s *MuxSession) removeStream(id uint32) {
	if val, ok := s.streams.LoadAndDelete(id); ok {
		st := val.(*MuxStream)
		st.closed.Store(true)
		st.closeOnce.Do(func() {
			close(st.closeChan)
		})
	}
}

func (s *MuxSession) readLoop() {
	defer s.Close()

	for {
		streamID, frameType, payload, err := s.frameConn.ReadFrame()
		if err != nil {
			return
		}

		switch frameType {
		case FrameOpen:
			stream := &MuxStream{
				id:        streamID,
				session:   s,
				readBuf:   make(chan []byte, 512),
				closeChan: make(chan struct{}),
			}
			s.streams.Store(streamID, stream)

			select {
			case s.acceptChan <- stream:
			default:
				stream.Close()
			}

		case FrameData:
			if val, ok := s.streams.Load(streamID); ok {
				stream := val.(*MuxStream)
				if !stream.closed.Load() {
					select {
					case stream.readBuf <- payload:
					case <-stream.closeChan:
					default:
						// Read buffer saturated, delegate without blocking multiplexer readLoop
						go func(st *MuxStream, p []byte) {
							select {
							case st.readBuf <- p:
							case <-st.closeChan:
							case <-time.After(3 * time.Second):
								st.Close()
							}
						}(stream, payload)
					}
				}
			}

		case FrameClose:
			s.removeStream(streamID)

		case FramePing:
			_ = s.frameConn.WriteFrame(0, FramePong, nil)

		case FramePong:
			// keepalive confirmed
		}
	}
}

func (s *MuxSession) heartbeatLoop() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.closed.Load() {
				return
			}
			_ = s.frameConn.WriteFrame(0, FramePing, nil)
		}
	}
}

func (s *MuxSession) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.underlying.Close()

		s.streams.Range(func(key, value any) bool {
			st := value.(*MuxStream)
			st.closed.Store(true)
			st.closeOnce.Do(func() {
				close(st.closeChan)
			})
			s.streams.Delete(key)
			return true
		})
		close(s.acceptChan)
	})
	return nil
}
