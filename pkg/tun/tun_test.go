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

func TestNetStack_EDNS0(t *testing.T) {
	mockDev := newMockTunDevice()
	router, _ := routing.NewEngine(routing.DefaultConfig())
	fakeDNS, _ := dns.NewFakeDNS("")

	ns := NewNetStack(mockDev, nil, router, fakeDNS)
	ns.Start()
	defer ns.Stop()

	// Build a raw DNS query with EDNS(0) OPT pseudo-record: ARCOUNT = 1
	// Header: ID=0x5678, Flags=0x0100, QDCOUNT=1, ANCOUNT=0, NSCOUNT=0, ARCOUNT=1
	dnsHeader := []byte{0x56, 0x78, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	qName := []byte{17, 'b', 'i', 'l', 'l', 'i', 'o', 'n', 'a', 'i', 'r', 'e', 'c', 'a', 's', 'i', 'n', 'o', 3, 'c', 'o', 'm', 0}
	qTypeClass := []byte{0x00, 0x01, 0x00, 0x01} // A, IN
	// EDNS(0) OPT pseudo-record: Name 0x00, Type 41 (0x0029), UDP payload 4096 (0x1000), TTL/flags 0, RDLEN 0
	optRecord := []byte{0x00, 0x00, 0x29, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	dnsPayload := append(dnsHeader, append(append(qName, qTypeClass...), optRecord...)...)

	// Wrap in UDP
	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], 54322)
	binary.BigEndian.PutUint16(udpHdr[2:4], 53)
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(8+len(dnsPayload)))

	// Wrap in IPv4
	srcIP := net.ParseIP("10.255.0.2").To4()
	dstIP := net.ParseIP("1.1.1.1").To4()

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
		// IP(20) + UDP(8) = 28 bytes offset
		dnsResp := reply[28:]
		txID := binary.BigEndian.Uint16(dnsResp[0:2])
		if txID != 0x5678 {
			t.Fatalf("unexpected DNS txID: %x", txID)
		}
		qdCount := binary.BigEndian.Uint16(dnsResp[4:6])
		anCount := binary.BigEndian.Uint16(dnsResp[6:8])
		if qdCount != 1 || anCount != 1 {
			t.Fatalf("expected QD=1, AN=1, got QD=%d, AN=%d", qdCount, anCount)
		}
		// Answer section must start right after Question (12 + len(qName) + 4)
		expectedAnsOffset := 12 + len(qName) + 4
		if dnsResp[expectedAnsOffset] != 0xC0 || dnsResp[expectedAnsOffset+1] != 0x0C {
			t.Fatalf("corrupted Answer pointer at offset %d: got %x %x, expected C0 0C", expectedAnsOffset, dnsResp[expectedAnsOffset], dnsResp[expectedAnsOffset+1])
		}
		ansIP := net.IP(dnsResp[expectedAnsOffset+12 : expectedAnsOffset+16])
		if !fakeDNS.IsFakeIP(ansIP) {
			t.Fatalf("expected FakeIP, got %s", ansIP)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for EDNS0 DNS reply")
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

func TestNetStack_GameMode(t *testing.T) {
	mockDev := newMockTunDevice()
	cfg := &routing.Config{
		Mode:          "rule",
		DefaultAction: routing.ActionDirect,
	}
	router, _ := routing.NewEngine(cfg)
	ns := NewNetStack(mockDev, nil, router, nil)

	if ns.IsGameMode() {
		t.Fatalf("expected GameMode to be false initially")
	}

	ns.SetGameMode(true)
	if !ns.IsGameMode() {
		t.Fatalf("expected GameMode to be true after SetGameMode(true)")
	}

	ns.SetGameMode(false)
	if ns.IsGameMode() {
		t.Fatalf("expected GameMode to be false after SetGameMode(false)")
	}
}

func TestNetStack_HTTPS_DNS_EmptyResponse(t *testing.T) {
	mockDev := newMockTunDevice()
	router, _ := routing.NewEngine(routing.DefaultConfig())
	fakeDNS, _ := dns.NewFakeDNS("")

	ns := NewNetStack(mockDev, nil, router, fakeDNS)
	ns.Start()
	defer ns.Stop()

	// Type 65 (HTTPS / SVCB) query
	dnsHeader := []byte{0x77, 0x88, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	qName := []byte{5, 'b', 'r', 'a', 'w', 'l', 5, 's', 't', 'a', 'r', 's', 3, 'c', 'o', 'm', 0}
	qTypeClass := []byte{0x00, 0x41, 0x00, 0x01} // TYPE 65 (0x0041), CLASS IN (0x0001)
	dnsPayload := append(dnsHeader, append(qName, qTypeClass...)...)

	// Wrap in UDP
	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], 51234)
	binary.BigEndian.PutUint16(udpHdr[2:4], 53)
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(8+len(dnsPayload)))

	// Wrap in IPv4
	srcIP := net.ParseIP("10.255.0.2").To4()
	dstIP := net.ParseIP("1.1.1.1").To4()

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
		dnsResp := reply[28:]
		txID := binary.BigEndian.Uint16(dnsResp[0:2])
		if txID != 0x7788 {
			t.Fatalf("unexpected txID: %x", txID)
		}
		anCount := binary.BigEndian.Uint16(dnsResp[6:8])
		if anCount != 0 {
			t.Fatalf("expected ANCOUNT=0 for non-A query in FakeDNS, got %d", anCount)
		}
		flags := binary.BigEndian.Uint16(dnsResp[2:4])
		// Check NOERROR (rcode = flags & 0x0F == 0)
		if flags&0x000F != 0 {
			t.Fatalf("expected RCODE NOERROR (0), got %d", flags&0x000F)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for HTTPS SVCB DNS response")
	}
}

func TestNetStack_TCPRetransmissionDeduplication(t *testing.T) {
	mockDev := newMockTunDevice()
	ns := NewNetStack(mockDev, nil, nil, nil)
	ns.Start()
	defer ns.Stop()

	// Start a local mock TCP target
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	targetPort := uint16(ln.Addr().(*net.TCPAddr).Port)
	receivedBytes := make(chan []byte, 10)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				receivedBytes <- chunk
			}
			if err != nil {
				break
			}
		}
	}()

	srcIP := net.ParseIP("10.254.1.2").To4()
	dstIP := net.ParseIP("127.0.0.1").To4()
	srcPort := uint16(54321)

	makeTCPPkt := func(seq, ack uint32, flags byte, data []byte) []byte {
		tcpLen := 20 + len(data)
		tcpHdr := make([]byte, 20)
		binary.BigEndian.PutUint16(tcpHdr[0:2], srcPort)
		binary.BigEndian.PutUint16(tcpHdr[2:4], targetPort)
		binary.BigEndian.PutUint32(tcpHdr[4:8], seq)
		binary.BigEndian.PutUint32(tcpHdr[8:12], ack)
		tcpHdr[12] = 0x50
		tcpHdr[13] = flags
		binary.BigEndian.PutUint16(tcpHdr[14:16], 65535)

		totalLen := 20 + tcpLen
		ipHdr := make([]byte, 20)
		ipHdr[0] = 0x45
		binary.BigEndian.PutUint16(ipHdr[2:4], uint16(totalLen))
		ipHdr[9] = ProtoTCP
		copy(ipHdr[12:16], srcIP)
		copy(ipHdr[16:20], dstIP)
		binary.BigEndian.PutUint16(ipHdr[10:12], checksum(ipHdr))

		return append(ipHdr, append(tcpHdr, data...)...)
	}

	// 1. Send SYN
	clientInitSeq := uint32(1000)
	ns.processIPv4Packet(makeTCPPkt(clientInitSeq, 0, TCPFlagSYN, nil))

	// Wait for SYN-ACK
	var serverInitSeq uint32
	select {
	case reply := <-mockDev.outBuf:
		if len(reply) < 40 {
			t.Fatalf("SYN-ACK too short: %d", len(reply))
		}
		tcpReply := reply[20:]
		serverInitSeq = binary.BigEndian.Uint32(tcpReply[4:8])
		ack := binary.BigEndian.Uint32(tcpReply[8:12])
		if ack != clientInitSeq+1 {
			t.Fatalf("expected ACK %d, got %d", clientInitSeq+1, ack)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for SYN-ACK")
	}

	// 2. Send initial DATA (20 bytes)
	dataPayload := []byte("hello telegram data!")
	ns.processIPv4Packet(makeTCPPkt(clientInitSeq+1, serverInitSeq+1, TCPFlagACK|TCPFlagPSH, dataPayload))

	select {
	case chunk := <-receivedBytes:
		if string(chunk) != string(dataPayload) {
			t.Fatalf("expected payload %q, got %q", string(dataPayload), string(chunk))
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for first data packet")
	}

	// Drain ACK reply from mockDev
	select {
	case <-mockDev.outBuf:
	case <-time.After(500 * time.Millisecond):
	}

	// 3. Send DUPLICATE / RETRANSMITTED DATA (same seq clientInitSeq+1)
	ns.processIPv4Packet(makeTCPPkt(clientInitSeq+1, serverInitSeq+1, TCPFlagACK|TCPFlagPSH, dataPayload))

	// Verify that target did NOT receive duplicate data!
	select {
	case dup := <-receivedBytes:
		t.Fatalf("ERROR: target received duplicate retransmitted data: %q! Stream corrupted!", string(dup))
	case <-time.After(300 * time.Millisecond):
		// SUCCESS: No duplicate data received
	}

	// But an ACK was sent back to confirm the sequence
	select {
	case reply := <-mockDev.outBuf:
		tcpReply := reply[20:]
		ack := binary.BigEndian.Uint32(tcpReply[8:12])
		expectedAck := clientInitSeq + 1 + uint32(len(dataPayload))
		if ack != expectedAck {
			t.Fatalf("expected ACK %d, got %d", expectedAck, ack)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected ACK for retransmission")
	}
}

func TestNetStack_TCPOptions_SYNACK(t *testing.T) {
	mockDev := newMockTunDevice()
	ns := NewNetStack(mockDev, nil, nil, nil)
	ns.Start()
	defer ns.Stop()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	targetPort := uint16(ln.Addr().(*net.TCPAddr).Port)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	srcIP := net.ParseIP("10.254.1.2").To4()
	dstIP := net.ParseIP("127.0.0.1").To4()
	srcPort := uint16(43210)

	// Send SYN
	clientInitSeq := uint32(5000)
	synHdr := make([]byte, 20)
	binary.BigEndian.PutUint16(synHdr[0:2], srcPort)
	binary.BigEndian.PutUint16(synHdr[2:4], targetPort)
	binary.BigEndian.PutUint32(synHdr[4:8], clientInitSeq)
	synHdr[12] = 0x50
	synHdr[13] = TCPFlagSYN
	binary.BigEndian.PutUint16(synHdr[14:16], 65535)

	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], 40)
	ipHdr[9] = ProtoTCP
	copy(ipHdr[12:16], srcIP)
	copy(ipHdr[16:20], dstIP)
	binary.BigEndian.PutUint16(ipHdr[10:12], checksum(ipHdr))

	synPkt := append(ipHdr, synHdr...)
	mockDev.inBuf <- synPkt

	select {
	case reply := <-mockDev.outBuf:
		if len(reply) != 52 {
			t.Fatalf("expected 52-byte IPv4+TCP SYN-ACK with options, got %d bytes", len(reply))
		}
		tcpReply := reply[20:]
		dataOffset := tcpReply[12] >> 4
		if dataOffset != 8 {
			t.Fatalf("expected data offset 8 (32 bytes), got %d", dataOffset)
		}
		flags := tcpReply[13]
		if flags != (TCPFlagSYN | TCPFlagACK) {
			t.Fatalf("expected SYN-ACK flags 0x12, got 0x%02x", flags)
		}

		// Verify MSS option (Kind 2, Len 4, Value 1360)
		if tcpReply[20] != 2 || tcpReply[21] != 4 {
			t.Fatalf("expected MSS option header [2 4], got [%d %d]", tcpReply[20], tcpReply[21])
		}
		mss := binary.BigEndian.Uint16(tcpReply[22:24])
		if mss != 1360 {
			t.Fatalf("expected MSS 1360, got %d", mss)
		}

		// Verify Window Scale (Kind 3, Len 3, Value 7)
		if tcpReply[25] != 3 || tcpReply[26] != 3 || tcpReply[27] != 7 {
			t.Fatalf("expected Window Scale [3 3 7], got [%d %d %d]", tcpReply[25], tcpReply[26], tcpReply[27])
		}

		// Verify SACK Permitted (Kind 4, Len 2)
		if tcpReply[28] != 4 || tcpReply[29] != 2 {
			t.Fatalf("expected SACK Permitted [4 2], got [%d %d]", tcpReply[28], tcpReply[29])
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for SYN-ACK")
	}
}

func TestNetStack_QUIC_PortUnreachable(t *testing.T) {
	mockDev := newMockTunDevice()
	ns := NewNetStack(mockDev, nil, nil, nil)
	ns.Start()
	defer ns.Stop()

	// Client sends QUIC UDP packet to port 443
	srcIP := net.ParseIP("10.254.1.2").To4()
	dstIP := net.ParseIP("142.250.180.206").To4() // YouTube/Google IP
	srcPort := uint16(51234)
	dstPort := uint16(443)

	quicData := []byte("QUIC_INITIAL_CLIENT_HELLO_PACKET")
	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], srcPort)
	binary.BigEndian.PutUint16(udpHdr[2:4], dstPort)
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(8+len(quicData)))

	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], uint16(20+8+len(quicData)))
	ipHdr[9] = ProtoUDP
	copy(ipHdr[12:16], srcIP)
	copy(ipHdr[16:20], dstIP)
	binary.BigEndian.PutUint16(ipHdr[10:12], checksum(ipHdr))

	pkt := append(ipHdr, append(udpHdr, quicData...)...)
	mockDev.inBuf <- pkt

	select {
	case reply := <-mockDev.outBuf:
		if len(reply) < 20+8 {
			t.Fatalf("ICMP reply too short: %d", len(reply))
		}
		// Verify IPv4 header
		proto := reply[9]
		if proto != ProtoICMP {
			t.Fatalf("expected protocol ICMP (1), got %d", proto)
		}
		repSrc := net.IP(reply[12:16]).String()
		repDst := net.IP(reply[16:20]).String()
		if repSrc != "142.250.180.206" || repDst != "10.254.1.2" {
			t.Fatalf("unexpected ICMP IPs: src=%s, dst=%s", repSrc, repDst)
		}

		// Verify ICMP Type 3 (Destination Unreachable), Code 3 (Port Unreachable)
		icmpReply := reply[20:]
		if icmpReply[0] != 3 || icmpReply[1] != 3 {
			t.Fatalf("expected ICMP Type 3 Code 3 (Port Unreachable), got Type %d Code %d", icmpReply[0], icmpReply[1])
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for ICMP Port Unreachable response")
	}
}

func TestNetStack_TCPSYN_Deduplication(t *testing.T) {
	mockDev := newMockTunDevice()
	ns := NewNetStack(mockDev, nil, nil, nil)
	ns.Start()
	defer ns.Stop()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	targetPort := uint16(ln.Addr().(*net.TCPAddr).Port)
	acceptedCount := 0
	var acceptMu sync.Mutex

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			acceptMu.Lock()
			acceptedCount++
			acceptMu.Unlock()
			_ = conn.Close()
		}
	}()

	srcIP := net.ParseIP("10.254.1.2").To4()
	dstIP := net.ParseIP("127.0.0.1").To4()
	srcPort := uint16(48888)

	clientInitSeq := uint32(9000)
	makeSYN := func() []byte {
		synHdr := make([]byte, 20)
		binary.BigEndian.PutUint16(synHdr[0:2], srcPort)
		binary.BigEndian.PutUint16(synHdr[2:4], targetPort)
		binary.BigEndian.PutUint32(synHdr[4:8], clientInitSeq)
		synHdr[12] = 0x50
		synHdr[13] = TCPFlagSYN
		binary.BigEndian.PutUint16(synHdr[14:16], 65535)

		ipHdr := make([]byte, 20)
		ipHdr[0] = 0x45
		binary.BigEndian.PutUint16(ipHdr[2:4], 40)
		ipHdr[9] = ProtoTCP
		copy(ipHdr[12:16], srcIP)
		copy(ipHdr[16:20], dstIP)
		binary.BigEndian.PutUint16(ipHdr[10:12], checksum(ipHdr))

		return append(ipHdr, synHdr...)
	}

	// Send duplicate SYNs rapidly (simulating mobile RTO retransmit)
	mockDev.inBuf <- makeSYN()
	mockDev.inBuf <- makeSYN()

	// Exactly 1 SYN-ACK should be received
	select {
	case reply := <-mockDev.outBuf:
		tcpReply := reply[20:]
		if tcpReply[13] != (TCPFlagSYN | TCPFlagACK) {
			t.Fatalf("expected SYN-ACK flags 0x12, got 0x%02x", tcpReply[13])
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for SYN-ACK")
	}

	// Wait brief moment and verify server was dialed only once
	time.Sleep(100 * time.Millisecond)
	acceptMu.Lock()
	count := acceptedCount
	acceptMu.Unlock()

	if count != 1 {
		t.Fatalf("expected exactly 1 accepted outbound connection, got %d", count)
	}
}
