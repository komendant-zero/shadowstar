package tun

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/shadowstar/shadowstar/pkg/dns"
	"github.com/shadowstar/shadowstar/pkg/routing"
)

func TestPacketRelay_ICMP(t *testing.T) {
	router, _ := routing.NewEngine(routing.DefaultConfig())
	fakeDNS, _ := dns.NewFakeDNS("")

	// Start packet relay on dynamic port
	relay, err := NewPacketRelay("127.0.0.1:0", nil, router, fakeDNS)
	if err != nil {
		t.Fatalf("failed to create PacketRelay: %v", err)
	}
	relayAddr := relay.conn.LocalAddr().String()
	relay.Start()
	defer relay.Stop()

	// Client simulating Android VpnService
	clientConn, err := net.Dial("udp", relayAddr)
	if err != nil {
		t.Fatalf("failed to dial relay: %v", err)
	}
	defer clientConn.Close()

	// Construct ICMP ping packet
	srcIP := net.ParseIP("10.254.1.2").To4()
	dstIP := net.ParseIP("1.1.1.1").To4()

	icmpPayload := []byte{8, 0, 0, 0, 0, 1, 0, 1, 'P', 'I', 'N', 'G'}
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

	// Send packet to relay
	_, err = clientConn.Write(packet)
	if err != nil {
		t.Fatalf("failed to write to relay: %v", err)
	}

	// Read reply
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	respBuf := make([]byte, 2048)
	rn, err := clientConn.Read(respBuf)
	if err != nil {
		t.Fatalf("failed to read response from relay: %v", err)
	}

	if rn < 28 {
		t.Fatalf("packet too short: %d", rn)
	}

	repSrc := net.IP(respBuf[12:16]).String()
	repDst := net.IP(respBuf[16:20]).String()
	if repSrc != "1.1.1.1" || repDst != "10.254.1.2" {
		t.Fatalf("unexpected reply IPs: src=%s dst=%s", repSrc, repDst)
	}
	if respBuf[20] != 0 {
		t.Fatalf("expected Echo Reply type 0, got %d", respBuf[20])
	}
}
