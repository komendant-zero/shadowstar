package dns

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestDNSServer_UDPandTCP(t *testing.T) {
	fd, err := NewFakeDNS("198.18.0.0/15")
	if err != nil {
		t.Fatalf("NewFakeDNS failed: %v", err)
	}

	server, err := NewServer("127.0.0.1:0", fd, nil)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("server.Start failed: %v", err)
	}
	defer server.Stop()

	serverAddr := server.LocalAddr().String()

	// 1. Test UDP query for Type A
	conn, err := net.Dial("udp", serverAddr)
	if err != nil {
		t.Fatalf("net.Dial udp failed: %v", err)
	}
	defer conn.Close()

	qBytes := BuildQuery("service.shadowstar.org", TypeA, 0x4321)
	if _, err := conn.Write(qBytes); err != nil {
		t.Fatalf("failed to send udp query: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	respBuf := make([]byte, 1024)
	n, err := conn.Read(respBuf)
	if err != nil {
		t.Fatalf("failed to read udp response: %v", err)
	}

	msg, err := ParseMessage(respBuf[:n])
	if err != nil {
		t.Fatalf("failed to parse dns response: %v", err)
	}
	if len(msg.Answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(msg.Answers))
	}
	ans := msg.Answers[0]
	if ans.Type != TypeA {
		t.Errorf("expected TypeA answer, got %d", ans.Type)
	}
	if ans.TTL != 1 {
		t.Errorf("expected TTL=1s, got %d", ans.TTL)
	}
	allocatedIP := net.IPv4(ans.Data[0], ans.Data[1], ans.Data[2], ans.Data[3])
	if !fd.IsFakeIP(allocatedIP) {
		t.Errorf("expected allocated IP to be within FakeIP pool, got %s", allocatedIP)
	}

	// 2. Test AAAA query -> expect NOERROR with NODATA (0 answers)
	qAAAA := BuildQuery("service.shadowstar.org", TypeAAAA, 0x4322)
	if _, err := conn.Write(qAAAA); err != nil {
		t.Fatalf("failed to send AAAA query: %v", err)
	}
	n, err = conn.Read(respBuf)
	if err != nil {
		t.Fatalf("failed to read AAAA response: %v", err)
	}
	msgAAAA, err := ParseMessage(respBuf[:n])
	if err != nil {
		t.Fatalf("failed to parse AAAA response: %v", err)
	}
	if msgAAAA.RCode != RCodeNoError {
		t.Errorf("expected NOERROR for AAAA, got %d", msgAAAA.RCode)
	}
	if len(msgAAAA.Answers) != 0 {
		t.Errorf("expected NODATA (0 answers) for AAAA query, got %d", len(msgAAAA.Answers))
	}

	// 3. Test PTR Reverse DNS query
	ptrName := fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", ans.Data[3], ans.Data[2], ans.Data[1], ans.Data[0])
	qPTR := BuildQuery(ptrName, TypePTR, 0x4323)
	if _, err := conn.Write(qPTR); err != nil {
		t.Fatalf("failed to send PTR query: %v", err)
	}
	n, err = conn.Read(respBuf)
	if err != nil {
		t.Fatalf("failed to read PTR response: %v", err)
	}
	msgPTR, err := ParseMessage(respBuf[:n])
	if err != nil {
		t.Fatalf("failed to parse PTR response: %v", err)
	}
	if len(msgPTR.Answers) != 1 {
		t.Fatalf("expected 1 PTR answer, got %d", len(msgPTR.Answers))
	}
	if msgPTR.Answers[0].Type != TypePTR {
		t.Errorf("expected TypePTR answer, got %d", msgPTR.Answers[0].Type)
	}

	// 4. Test TCP query
	tcpConn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("net.Dial tcp failed: %v", err)
	}
	defer tcpConn.Close()

	prefix := make([]byte, 2)
	binary.BigEndian.PutUint16(prefix, uint16(len(qBytes)))
	if _, err := tcpConn.Write(prefix); err != nil {
		t.Fatalf("failed to write tcp prefix: %v", err)
	}
	if _, err := tcpConn.Write(qBytes); err != nil {
		t.Fatalf("failed to write tcp query: %v", err)
	}

	_ = tcpConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(tcpConn, prefix); err != nil {
		t.Fatalf("failed to read tcp response prefix: %v", err)
	}
	respLen := binary.BigEndian.Uint16(prefix)
	tcpResp := make([]byte, respLen)
	if _, err := io.ReadFull(tcpConn, tcpResp); err != nil {
		t.Fatalf("failed to read tcp response body: %v", err)
	}

	tcpMsg, err := ParseMessage(tcpResp)
	if err != nil {
		t.Fatalf("failed to parse tcp dns message: %v", err)
	}
	if len(tcpMsg.Answers) != 1 {
		t.Errorf("expected 1 tcp answer, got %d", len(tcpMsg.Answers))
	}
}
