package dns

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Server provides an in-process DNS server listening on UDP and TCP.
type Server struct {
	listenAddr  string
	fakeDNS     FakeDNS
	resolver    Resolver
	udpConn     *net.UDPConn
	tcpListener net.Listener
	quit        chan struct{}
	wg          sync.WaitGroup
	mu          sync.Mutex
	running     bool
}

// NewServer initializes an in-process DNS server.
func NewServer(listenAddr string, fakedns FakeDNS, resolver Resolver) (*Server, error) {
	if listenAddr == "" {
		listenAddr = "198.18.0.1:53"
	}
	return &Server{
		listenAddr: listenAddr,
		fakeDNS:    fakedns,
		resolver:   resolver,
		quit:       make(chan struct{}),
	}, nil
}

// Start opens the UDP and TCP listeners and begins serving DNS requests.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	uAddr, err := net.ResolveUDPAddr("udp", s.listenAddr)
	if err != nil {
		return err
	}
	uConn, err := net.ListenUDP("udp", uAddr)
	if err != nil {
		return fmt.Errorf("listen udp dns: %w", err)
	}
	s.udpConn = uConn

	// If dynamic port was chosen (e.g. :0), use that port for TCP as well
	tcpAddr := s.listenAddr
	if uAddr.Port == 0 {
		tcpAddr = uConn.LocalAddr().String()
	}

	tListener, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		_ = s.udpConn.Close()
		return fmt.Errorf("listen tcp dns: %w", err)
	}
	s.tcpListener = tListener
	s.running = true

	s.wg.Add(2)
	go s.serveUDP()
	go s.serveTCP()

	return nil
}

// Stop gracefully shuts down listeners and waits for active connections to finish.
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	close(s.quit)

	if s.udpConn != nil {
		_ = s.udpConn.Close()
	}
	if s.tcpListener != nil {
		_ = s.tcpListener.Close()
	}
	s.mu.Unlock()

	s.wg.Wait()
	return nil
}

// LocalAddr returns the bound UDP address of the server.
func (s *Server) LocalAddr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.udpConn != nil {
		return s.udpConn.LocalAddr()
	}
	return nil
}

func (s *Server) handlePacket(data []byte) []byte {
	req, err := ParseMessage(data)
	if err != nil || len(req.Questions) == 0 {
		return nil
	}

	q := req.Questions[0]
	switch q.Type {
	case TypeA:
		if s.fakeDNS == nil {
			return BuildResponse(req, nil, RCodeServFail)
		}
		fakeIP := s.fakeDNS.Allocate(q.Name)
		if fakeIP == nil {
			return BuildResponse(req, nil, RCodeServFail)
		}
		ip4 := fakeIP.To4()
		ans := ResourceRecord{
			Name:  q.Name,
			Type:  TypeA,
			Class: ClassINET,
			TTL:   1, // Enforce TTL=1s
			Data:  ip4,
		}
		return BuildResponse(req, []ResourceRecord{ans}, RCodeNoError)

	case TypeAAAA:
		// RFC 4074: return NOERROR with empty answer section (NODATA)
		return BuildResponse(req, nil, RCodeNoError)

	case TypePTR:
		ip := ParsePTR(q.Name)
		if ip != nil && s.fakeDNS != nil {
			if domain, found := s.fakeDNS.LookupDomain(ip); found {
				encoded := encodeName(domain)
				ans := ResourceRecord{
					Name:  q.Name,
					Type:  TypePTR,
					Class: ClassINET,
					TTL:   1,
					Data:  encoded,
				}
				return BuildResponse(req, []ResourceRecord{ans}, RCodeNoError)
			}
		}
		return BuildResponse(req, nil, RCodeNXDomain)

	default:
		return BuildResponse(req, nil, RCodeNoError)
	}
}

func (s *Server) serveUDP() {
	defer s.wg.Done()
	buf := make([]byte, 2048)
	for {
		n, remote, err := s.udpConn.ReadFrom(buf)
		if err != nil {
			return
		}
		resp := s.handlePacket(buf[:n])
		if len(resp) > 0 {
			_, _ = s.udpConn.WriteTo(resp, remote)
		}
	}
}

func (s *Server) serveTCP() {
	defer s.wg.Done()
	for {
		conn, err := s.tcpListener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			lenBuf := make([]byte, 2)
			if _, err := io.ReadFull(c, lenBuf); err != nil {
				return
			}
			pLen := binary.BigEndian.Uint16(lenBuf)
			pBuf := make([]byte, pLen)
			if _, err := io.ReadFull(c, pBuf); err != nil {
				return
			}
			resp := s.handlePacket(pBuf)
			if len(resp) > 0 {
				binary.BigEndian.PutUint16(lenBuf, uint16(len(resp)))
				_, _ = c.Write(lenBuf)
				_, _ = c.Write(resp)
			}
		}(conn)
	}
}
