package tun

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shadowstar/shadowstar/pkg/dns"
	"github.com/shadowstar/shadowstar/pkg/routing"
)

type mockTunDevice struct {
	inBuf  chan []byte
	outBuf chan []byte
	closed bool
	mu     sync.Mutex
}

func newMockTunDevice() *mockTunDevice {
	return &mockTunDevice{
		inBuf:  make(chan []byte, 100),
		outBuf: make(chan []byte, 100),
	}
}

func (m *mockTunDevice) Read(p []byte) (int, error) {
	pkt, ok := <-m.inBuf
	if !ok {
		return 0, ioEOF
	}
	copy(p, pkt)
	return len(pkt), nil
}

func (m *mockTunDevice) Write(p []byte) (int, error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	m.outBuf <- cp
	return len(p), nil
}

func (m *mockTunDevice) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.inBuf)
	}
	return nil
}

func (m *mockTunDevice) Name() string {
	return "mock-tun0"
}

var ioEOF = bytes.ErrTooLarge

func TestTunConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DeviceName != "ShadowStar" {
		t.Fatalf("unexpected device name: %s", cfg.DeviceName)
	}
	if cfg.IPv4Address != "10.255.0.2" {
		t.Fatalf("unexpected ipv4 address: %s", cfg.IPv4Address)
	}
	if cfg.MTU != 1500 {
		t.Fatalf("unexpected MTU: %d", cfg.MTU)
	}
}

func TestChecksum(t *testing.T) {
	data := []byte{0x45, 0x00, 0x00, 0x3c, 0x1c, 0x46, 0x40, 0x00, 0x40, 0x06, 0x00, 0x00, 0xac, 0x10, 0x0a, 0x63, 0xac, 0x10, 0x0a, 0x0c}
	cs := checksum(data)
	if cs == 0 {
		t.Fatalf("checksum cannot be zero for non-zero data")
	}
}

func TestNetStack_ICMP(t *testing.T) {
	mockDev := newMockTunDevice()
	router, _ := routing.NewEngine(routing.DefaultConfig())
	fakeDNS, _ := dns.NewFakeDNS("")

	ns := NewNetStack(mockDev, nil, router, fakeDNS)
	ns.Start()
	defer ns.Stop()

	// Build an ICMP Echo Request (IPv4)
	srcIP := net.ParseIP("10.255.0.2").To4()
	dstIP := net.ParseIP("1.1.1.1").To4()

	icmpPayload := []byte{8, 0, 0, 0, 0, 1, 0, 1, 'T', 'E', 'S', 'T'}
	cs := checksum(icmpPayload)
	binary.BigEndian.PutUint16(icmpPayload[2:4], cs)

	totalLen := 20 + len(icmpPayload)
	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], uint16(totalLen))
	ipHdr[9] = ProtoICMP
	copy(ipHdr[12:16], srcIP)
	copy(ipHdr[16:20], dstIP)
	binary.BigEndian.PutUint16(ipHdr[10:12], checksum(ipHdr))

	packet := append(ipHdr, icmpPayload...)
	mockDev.inBuf <- packet

	select {
	case reply := <-mockDev.outBuf:
		if len(reply) < 28 {
			t.Fatalf("reply packet too short: %d", len(reply))
		}
		// Verify IPv4 source is 1.1.1.1 and destination is 10.255.0.2
		repSrc := net.IP(reply[12:16]).String()
		repDst := net.IP(reply[16:20]).String()
		if repSrc != "1.1.1.1" || repDst != "10.255.0.2" {
			t.Fatalf("unexpected IP headers: src=%s, dst=%s", repSrc, repDst)
		}
		// Verify ICMP Type is 0 (Echo Reply)
		if reply[20] != 0 {
			t.Fatalf("expected ICMP Echo Reply (Type 0), got: %d", reply[20])
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for ICMP reply")
	}
}

func TestNetStack_FakeDNS(t *testing.T) {
	mockDev := newMockTunDevice()
	router, _ := routing.NewEngine(routing.DefaultConfig())
	fakeDNS, _ := dns.NewFakeDNS("")

	ns := NewNetStack(mockDev, nil, router, fakeDNS)
	ns.Start()
	defer ns.Stop()

	// Build a raw DNS query for "youtube.com" (Type A)
	dnsHeader := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	qName := []byte{7, 'y', 'o', 'u', 't', 'u', 'b', 'e', 3, 'c', 'o', 'm', 0}
	qTypeClass := []byte{0x00, 0x01, 0x00, 0x01} // A, IN
	dnsPayload := append(dnsHeader, append(qName, qTypeClass...)...)

	// Wrap in UDP
	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], 54321) // srcPort
	binary.BigEndian.PutUint16(udpHdr[2:4], 53)    // dstPort
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(8+len(dnsPayload)))

	// Wrap in IPv4
	srcIP := net.ParseIP("10.255.0.2").To4()
	dstIP := net.ParseIP("198.18.0.1").To4()

	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], uint16(20+8+len(dnsPayload)))
	ipHdr[9] = ProtoUDP
	copy(ipHdr[12:16], srcIP)
	copy(ipHdr[16:20], dstIP)
	binary.BigEndian.PutUint16(ipHdr[10:12], checksum(ipHdr))

	packet := append(ipHdr, append(udpHdr, dnsPayload...)...)
	mockDev.inBuf <- packet

	select {
	case reply := <-mockDev.outBuf:
		if len(reply) < 28+12 {
			t.Fatalf("DNS reply too short: %d", len(reply))
		}
		// Verify DNS reply has TxID 0x1234
		txID := binary.BigEndian.Uint16(reply[28:30])
		if txID != 0x1234 {
			t.Fatalf("unexpected DNS txID: %x", txID)
		}
		// Extract synthetic IP from answer (last 4 bytes)
		ansIP := net.IP(reply[len(reply)-4:])
		if !fakeDNS.IsFakeIP(ansIP) {
			t.Fatalf("expected synthetic IP in 198.18.0.0/15, got %s", ansIP)
		}
		domain, found := fakeDNS.LookupDomain(ansIP)
		if !found || domain != "youtube.com" {
			t.Fatalf("expected youtube.com mapped, got %s (found=%v)", domain, found)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for DNS reply")
	}
}

func TestNetStack_UDPDirect(t *testing.T) {
	// 1. Start a local UDP echo server
	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("failed to listen UDP: %v", err)
	}
	defer serverConn.Close()
	serverPort := serverConn.LocalAddr().(*net.UDPAddr).Port

	// Echo server goroutine
	go func() {
		buf := make([]byte, 1024)
		for {
			n, clientAddr, err := serverConn.ReadFrom(buf)
			if err != nil {
				return
			}
			// Echo "ECHO:" + payload back
			resp := append([]byte("ECHO:"), buf[:n]...)
			_, _ = serverConn.WriteTo(resp, clientAddr)
		}
	}()

	mockDev := newMockTunDevice()
	cfg := &routing.Config{
		Mode:          "direct",
		DefaultAction: routing.ActionDirect,
	}
	router, _ := routing.NewEngine(cfg)
	ns := NewNetStack(mockDev, nil, router, nil)
	ns.Start()
	defer ns.Stop()

	// Send UDP packet from 10.255.0.2:45678 -> 127.0.0.1:serverPort
	srcIP := net.ParseIP("10.255.0.2").To4()
	dstIP := net.ParseIP("127.0.0.1").To4()
	udpPayload := []byte("GAME_PACKET_HELLO")

	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], 45678)
	binary.BigEndian.PutUint16(udpHdr[2:4], uint16(serverPort))
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(8+len(udpPayload)))

	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], uint16(20+8+len(udpPayload)))
	ipHdr[9] = ProtoUDP
	copy(ipHdr[12:16], srcIP)
	copy(ipHdr[16:20], dstIP)
	binary.BigEndian.PutUint16(ipHdr[10:12], checksum(ipHdr))

	packet := append(ipHdr, append(udpHdr, udpPayload...)...)
	mockDev.inBuf <- packet

	select {
	case reply := <-mockDev.outBuf:
		if len(reply) < 28 {
			t.Fatalf("reply too short: %d", len(reply))
		}
		repSrc := net.IP(reply[12:16]).String()
		repDst := net.IP(reply[16:20]).String()
		if repSrc != "127.0.0.1" || repDst != "10.255.0.2" {
			t.Fatalf("unexpected IP headers: src=%s, dst=%s", repSrc, repDst)
		}
		repSrcPort := binary.BigEndian.Uint16(reply[20:22])
		repDstPort := binary.BigEndian.Uint16(reply[22:24])
		if repSrcPort != uint16(serverPort) || repDstPort != 45678 {
			t.Fatalf("unexpected UDP ports: src=%d, dst=%d", repSrcPort, repDstPort)
		}
		payload := string(reply[28:])
		if payload != "ECHO:GAME_PACKET_HELLO" {
			t.Fatalf("unexpected payload: %s", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for UDP direct response")
	}
}

func TestSocks5UDP_Helpers(t *testing.T) {
	// Test build and parse helpers
	pkt := buildSocksUDPPacket("1.2.3.4", 9933, []byte("CASINO_SLOT_BET"))
	if len(pkt) != 10+len("CASINO_SLOT_BET") {
		t.Fatalf("unexpected packet length: %d", len(pkt))
	}
	parsed := parseSocksUDPResponse(pkt)
	if string(parsed) != "CASINO_SLOT_BET" {
		t.Fatalf("parsed payload mismatch: %s", string(parsed))
	}

	// Test Domain format
	domPkt := buildSocksUDPPacket("game.billionairecasino.com", 8443, []byte("LOGIN_CHALLENGE"))
	domParsed := parseSocksUDPResponse(domPkt)
	if string(domParsed) != "LOGIN_CHALLENGE" {
		t.Fatalf("domain parsed payload mismatch: %s", string(domParsed))
	}
}

func TestNetStack_UDPSocks5Proxy(t *testing.T) {
	// 1. Mock SOCKS5 UDP Relay listener
	relayConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("failed to listen mock relay: %v", err)
	}
	defer relayConn.Close()
	relayPort := relayConn.LocalAddr().(*net.UDPAddr).Port

	// 2. Mock SOCKS5 TCP server
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen mock socks5 tcp: %v", err)
	}
	defer tcpListener.Close()
	socksAddr := tcpListener.Addr().String()

	// SOCKS5 TCP handler
	go func() {
		conn, err := tcpListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read greeting
		buf := make([]byte, 128)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte{0x05, 0x00}) // Auth success

		// Read UDP associate request
		_, _ = conn.Read(buf)
		// Reply with relay IP & port
		rep := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, byte(relayPort >> 8), byte(relayPort & 0xff)}
		_, _ = conn.Write(rep)

		// Hold connection open
		_, _ = conn.Read(buf)
	}()

	// Mock UDP relay echo loop
	go func() {
		buf := make([]byte, 65535)
		for {
			n, clientAddr, err := relayConn.ReadFrom(buf)
			if err != nil {
				return
			}
			// Parse SOCKS5 request
			payload := parseSocksUDPResponse(buf[:n])
			if len(payload) > 0 {
				echoData := append([]byte("SOCKS_ECHO:"), payload...)
				// Echo back with SOCKS5 header for 1.2.3.4:9933
				respPkt := buildSocksUDPPacket("1.2.3.4", 9933, echoData)
				_, _ = relayConn.WriteTo(respPkt, clientAddr)
			}
		}
	}()

	mockDev := newMockTunDevice()
	cfg := &routing.Config{
		Mode:          "global",
		DefaultAction: routing.ActionProxy,
	}
	router, _ := routing.NewEngine(cfg)
	ns := NewNetStack(mockDev, nil, router, nil)
	ns.SetSocksAddr(socksAddr)
	ns.Start()
	defer ns.Stop()

	// Send UDP packet from 10.255.0.2:55555 -> 1.2.3.4:9933 (foreign game server)
	srcIP := net.ParseIP("10.255.0.2").To4()
	dstIP := net.ParseIP("1.2.3.4").To4()
	gamePayload := []byte("CASINO_SPIN_EVENT")

	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], 55555)
	binary.BigEndian.PutUint16(udpHdr[2:4], 9933)
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(8+len(gamePayload)))

	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], uint16(20+8+len(gamePayload)))
	ipHdr[9] = ProtoUDP
	copy(ipHdr[12:16], srcIP)
	copy(ipHdr[16:20], dstIP)
	binary.BigEndian.PutUint16(ipHdr[10:12], checksum(ipHdr))

	packet := append(ipHdr, append(udpHdr, gamePayload...)...)
	mockDev.inBuf <- packet

	select {
	case reply := <-mockDev.outBuf:
		if len(reply) < 28 {
			t.Fatalf("reply too short: %d", len(reply))
		}
		repSrc := net.IP(reply[12:16]).String()
		repDst := net.IP(reply[16:20]).String()
		if repSrc != "1.2.3.4" || repDst != "10.255.0.2" {
			t.Fatalf("unexpected IP headers: src=%s, dst=%s", repSrc, repDst)
		}
		repSrcPort := binary.BigEndian.Uint16(reply[20:22])
		repDstPort := binary.BigEndian.Uint16(reply[22:24])
		if repSrcPort != 9933 || repDstPort != 55555 {
			t.Fatalf("unexpected UDP ports: src=%d, dst=%d", repSrcPort, repDstPort)
		}
		payload := string(reply[28:])
		if payload != "SOCKS_ECHO:CASINO_SPIN_EVENT" {
			t.Fatalf("unexpected payload: %s", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for UDP SOCKS5 proxy response")
	}
}
