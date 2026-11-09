package protocol

import (
	"bytes"
	"net"
	"testing"
)

func TestHandshakeAndVerification(t *testing.T) {
	masterKey := []byte("01234567890123456789012345678901")
	sni := "www.microsoft.com"

	priv, pub, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair failed: %v", err)
	}

	clientHello, err := BuildClientHello(sni, masterKey, pub, 64, 0)
	if err != nil {
		t.Fatalf("BuildClientHello failed: %v", err)
	}

	parsed, err := ParseClientHello(clientHello)
	if err != nil {
		t.Fatalf("ParseClientHello failed: %v", err)
	}

	if parsed.SNI != sni {
		t.Errorf("Expected SNI %s, got %s", sni, parsed.SNI)
	}

	if !bytes.Equal(parsed.ClientPub, pub) {
		t.Errorf("Parsed client public key mismatch")
	}

	valid := VerifyAuthSessionID(parsed.SessionID, masterKey, parsed.ClientPub, parsed.SNI)
	if !valid {
		t.Fatalf("VerifyAuthSessionID failed for valid handshake")
	}

	// Test invalid key
	wrongKey := []byte("wrongkeywrongkeywrongkeywrongkey")
	if VerifyAuthSessionID(parsed.SessionID, wrongKey, parsed.ClientPub, parsed.SNI) {
		t.Fatalf("VerifyAuthSessionID should have rejected wrong key")
	}

	// Test server hello
	sPriv, sPub, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("Server keypair failed: %v", err)
	}
	serverHello := BuildServerHello(sPub, parsed.SessionID)

	parsedServerPub, err := ParseServerHello(serverHello)
	if err != nil {
		t.Fatalf("ParseServerHello failed: %v", err)
	}

	if !bytes.Equal(parsedServerPub, sPub) {
		t.Fatalf("Parsed server public key mismatch")
	}

	// Test ECDH matching
	shared1, err := ComputeECDH(priv, parsedServerPub)
	if err != nil {
		t.Fatalf("Client ECDH failed: %v", err)
	}

	shared2, err := ComputeECDH(sPriv, parsed.ClientPub)
	if err != nil {
		t.Fatalf("Server ECDH failed: %v", err)
	}

	if !bytes.Equal(shared1, shared2) {
		t.Fatalf("ECDH shared secrets do not match!")
	}
}

func TestSecureFrameConn(t *testing.T) {
	pipeA, pipeB := net.Pipe()
	defer pipeA.Close()
	defer pipeB.Close()

	keyA := RandomBytes(32)
	ivA := RandomBytes(12)
	keyB := RandomBytes(32)
	ivB := RandomBytes(12)

	// conn1: writes with (keyA, ivA), reads with (keyB, ivB)
	conn1, err := NewSecureFrameConn(pipeA, keyA, ivA, keyB, ivB)
	if err != nil {
		t.Fatalf("conn1 create failed: %v", err)
	}

	// conn2: writes with (keyB, ivB), reads with (keyA, ivA)
	conn2, err := NewSecureFrameConn(pipeB, keyB, ivB, keyA, ivA)
	if err != nil {
		t.Fatalf("conn2 create failed: %v", err)
	}

	testPayload := []byte("ShadowStar high performance anti-censorship packet!")
	errChan := make(chan error, 1)

	go func() {
		err := conn1.WriteFrame(42, FrameData, testPayload)
		errChan <- err
	}()

	streamID, fType, payload, err := conn2.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}
	if wErr := <-errChan; wErr != nil {
		t.Fatalf("WriteFrame failed: %v", wErr)
	}

	if streamID != 42 {
		t.Errorf("Expected streamID 42, got %d", streamID)
	}
	if fType != FrameData {
		t.Errorf("Expected FrameData type, got %d", fType)
	}
	if !bytes.Equal(payload, testPayload) {
		t.Errorf("Decrypted payload does not match original!")
	}
}

func TestTargetAddressSerialization(t *testing.T) {
	// Test TCP domain
	buf := new(bytes.Buffer)
	original := "github.com:443"

	if err := WriteTargetAddress(buf, NetTCP, original); err != nil {
		t.Fatalf("WriteTargetAddress TCP failed: %v", err)
	}

	parsed, err := ReadTargetAddress(buf)
	if err != nil {
		t.Fatalf("ReadTargetAddress TCP failed: %v", err)
	}

	if parsed.Network != NetTCP {
		t.Fatalf("Expected NetTCP, got %d", parsed.Network)
	}
	if parsed.String() != original {
		t.Fatalf("Expected %s, got %s", original, parsed.String())
	}

	// Test UDP IPv4
	bufUDP := new(bytes.Buffer)
	originalUDP := "1.1.1.1:53"
	if err := WriteTargetAddress(bufUDP, NetUDP, originalUDP); err != nil {
		t.Fatalf("WriteTargetAddress UDP failed: %v", err)
	}

	parsedUDP, err := ReadTargetAddress(bufUDP)
	if err != nil {
		t.Fatalf("ReadTargetAddress UDP failed: %v", err)
	}

	if parsedUDP.Network != NetUDP {
		t.Fatalf("Expected NetUDP, got %d", parsedUDP.Network)
	}
	if parsedUDP.String() != originalUDP {
		t.Fatalf("Expected %s, got %s", originalUDP, parsedUDP.String())
	}
}
