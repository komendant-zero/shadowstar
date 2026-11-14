package dns

import (
	"bytes"
	"net"
	"testing"
)

func TestDNSMessage_Roundtrip(t *testing.T) {
	domain := "api.shadowstar.org"
	query := BuildQuery(domain, TypeA, 0x1234)

	msg, err := ParseMessage(query)
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}
	if msg.ID != 0x1234 {
		t.Errorf("expected ID 0x1234, got 0x%x", msg.ID)
	}
	if len(msg.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(msg.Questions))
	}
	q := msg.Questions[0]
	if q.Name != domain {
		t.Errorf("expected question name %s, got %s", domain, q.Name)
	}
	if q.Type != TypeA || q.Class != ClassINET {
		t.Errorf("expected TypeA ClassINET, got Type %d Class %d", q.Type, q.Class)
	}

	// Build response with 1 answer
	ans := ResourceRecord{
		Name:  domain,
		Type:  TypeA,
		Class: ClassINET,
		TTL:   1,
		Data:  []byte{198, 18, 0, 2},
	}
	resp := BuildResponse(msg, []ResourceRecord{ans}, RCodeNoError)

	respMsg, err := ParseMessage(resp)
	if err != nil {
		t.Fatalf("ParseMessage of response failed: %v", err)
	}
	if !respMsg.Response {
		t.Errorf("expected response flag QR=1")
	}
	if len(respMsg.Answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(respMsg.Answers))
	}
	a := respMsg.Answers[0]
	if a.Name != domain {
		t.Errorf("expected answer name %s, got %s", domain, a.Name)
	}
	if a.TTL != 1 {
		t.Errorf("expected TTL 1, got %d", a.TTL)
	}
	if !bytes.Equal(a.Data, []byte{198, 18, 0, 2}) {
		t.Errorf("expected IP 198.18.0.2, got %v", a.Data)
	}
}

func TestDNSMessage_CompressionPointer(t *testing.T) {
	// Question at offset 12: \x03www\x06google\x03com\x00 (16 bytes, ends at offset 28)
	// Answer with compression pointer 0xC00C pointing to offset 12
	qBytes := BuildQuery("www.google.com", TypeA, 0x5678)
	req, err := ParseMessage(qBytes)
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}

	ans := ResourceRecord{
		Name:  "www.google.com",
		Type:  TypeA,
		Class: ClassINET,
		TTL:   1,
		Data:  []byte{198, 18, 1, 10},
	}
	resp := BuildResponse(req, []ResourceRecord{ans}, RCodeNoError)

	parsed, err := ParseMessage(resp)
	if err != nil {
		t.Fatalf("failed to parse compressed response: %v", err)
	}
	if len(parsed.Answers) != 1 || parsed.Answers[0].Name != "www.google.com" {
		t.Errorf("compression pointer failed to decode name: %+v", parsed.Answers)
	}
}

func TestDNSMessage_CompressionLoop(t *testing.T) {
	// Construct message with cyclic compression pointer: offset 12 points to 14, 14 points to 12
	badMsg := []byte{
		0x00, 0x01, // ID
		0x01, 0x00, // Flags
		0x00, 0x01, // QDCOUNT=1
		0x00, 0x00, // ANCOUNT=0
		0x00, 0x00, // NSCOUNT=0
		0x00, 0x00, // ARCOUNT=0
		0xC0, 0x0E, // Offset 12: points to offset 14
		0xC0, 0x0C, // Offset 14: points to offset 12
		0x00, 0x01, 0x00, 0x01,
	}

	_, err := ParseMessage(badMsg)
	if err == nil {
		t.Fatal("expected error on cyclic compression pointer, got nil")
	}
}

func TestDNSMessage_Malformed(t *testing.T) {
	// < 12 bytes
	_, err := ParseMessage([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error on short buffer (<12 bytes)")
	}

	// Truncated question
	shortQ := []byte{
		0, 1, 0, 0,
		0, 1, 0, 0,
		0, 0, 0, 0,
		3, 'a', 'b', 'c', // Missing null terminator and QTYPE/QCLASS
	}
	_, err = ParseMessage(shortQ)
	if err == nil {
		t.Error("expected error on truncated question")
	}
}

func TestParsePTR(t *testing.T) {
	ip := ParsePTR("2.0.18.198.in-addr.arpa")
	if ip == nil {
		t.Fatal("ParsePTR returned nil")
	}
	expected := net.IPv4(198, 18, 0, 2)
	if !ip.Equal(expected) {
		t.Errorf("expected %s, got %s", expected, ip)
	}

	// Invalid PTR
	if ipBad := ParsePTR("invalid.domain.com"); ipBad != nil {
		t.Errorf("expected nil for invalid PTR, got %s", ipBad)
	}
}
