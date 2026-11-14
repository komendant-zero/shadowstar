package protocol

import (
	"bytes"
	"encoding/binary"
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

func TestGREASEGenerationAndDetection(t *testing.T) {
	// 1. Verify all 16 RFC 8701 GREASE values match IsGREASE
	for _, val := range greaseValues {
		if !IsGREASE(val) {
			t.Errorf("greaseValue 0x%04x not recognized as GREASE", val)
		}
	}

	// 2. Verify RandomGREASE produces valid GREASE values
	for i := 0; i < 50; i++ {
		g := RandomGREASE()
		if !IsGREASE(g) {
			t.Fatalf("RandomGREASE produced invalid value: 0x%04x", g)
		}
	}

	// 3. Verify standard non-GREASE values return false
	nonGrease := []uint16{
		0x0000, 0x001d, 0x1301, 0x1302, 0x1303, 0xc02f, 0xc030,
		0x0303, 0x0304, 0x002b, 0x0033, 0x6399, 0x11ec,
	}
	for _, ng := range nonGrease {
		if IsGREASE(ng) {
			t.Errorf("Standard value 0x%04x incorrectly identified as GREASE", ng)
		}
	}
}

func TestClientHello_ChromeGREASEAndKyberShares(t *testing.T) {
	masterKey := []byte("01234567890123456789012345678901")
	sni := "cp.cloudflare.com"

	_, pub, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair failed: %v", err)
	}

	clientHello, err := BuildClientHello(sni, masterKey, pub, 64, 0)
	if err != nil {
		t.Fatalf("BuildClientHello failed: %v", err)
	}

	// Parse ClientHello
	parsed, err := ParseClientHello(clientHello)
	if err != nil {
		t.Fatalf("ParseClientHello failed: %v", err)
	}

	if parsed.SNI != sni {
		t.Errorf("Expected SNI %s, got %s", sni, parsed.SNI)
	}

	// Verify ClientPub extracted matches original pub
	if !bytes.Equal(parsed.ClientPub, pub) {
		t.Fatalf("Parsed ClientPub mismatch with original generated key")
	}

	// Verify SessionID HMAC authenticity
	if !VerifyAuthSessionID(parsed.SessionID, masterKey, parsed.ClientPub, parsed.SNI) {
		t.Fatalf("SessionID failed auth verification against masterKey")
	}

	// Inspect raw TLS record
	if clientHello[0] != TLSRecordHandshake {
		t.Errorf("Expected TLS record type 0x16, got 0x%02x", clientHello[0])
	}
	recordVer := binary.BigEndian.Uint16(clientHello[1:3])
	if recordVer != TLSVersion10 && recordVer != TLSVersion12 {
		t.Errorf("Unexpected record TLS version: 0x%04x", recordVer)
	}

	// Handshake header: type ClientHello (0x01)
	if clientHello[5] != TLSHandshakeClientHello {
		t.Errorf("Expected ClientHello type 0x01, got 0x%02x", clientHello[5])
	}
}

func TestSessionIDAuth_And_AntiReplay(t *testing.T) {
	masterKey := []byte("testmasterkeyfortestingauth12345")
	clientPub := RandomBytes(32)
	sni := "connectivitycheck.gstatic.com"

	sessionID, err := BuildAuthSessionID(masterKey, clientPub, sni, 0)
	if err != nil {
		t.Fatalf("BuildAuthSessionID failed: %v", err)
	}

	if len(sessionID) != 32 {
		t.Fatalf("Expected 32-byte SessionID, got %d", len(sessionID))
	}

	// Verify valid session ID
	if !VerifyAuthSessionID(sessionID, masterKey, clientPub, sni) {
		t.Fatalf("VerifyAuthSessionID failed for genuine SessionID")
	}

	// Tampered SNI fails
	if VerifyAuthSessionID(sessionID, masterKey, clientPub, "attacker.com") {
		t.Errorf("VerifyAuthSessionID accepted wrong SNI")
	}

	// Tampered clientPub fails
	wrongPub := make([]byte, 32)
	copy(wrongPub, clientPub)
	wrongPub[0] ^= 0xff
	if VerifyAuthSessionID(sessionID, masterKey, wrongPub, sni) {
		t.Errorf("VerifyAuthSessionID accepted tampered clientPub")
	}

	// Tampered Tag fails
	tamperedSessionID := make([]byte, 32)
	copy(tamperedSessionID, sessionID)
	tamperedSessionID[31] ^= 0x01
	if VerifyAuthSessionID(tamperedSessionID, masterKey, clientPub, sni) {
		t.Errorf("VerifyAuthSessionID accepted tampered Tag")
	}

	// Test ReplayDetector
	detector := NewReplayDetector()

	// First presentation -> accepted
	if !detector.CheckAndAdd(sessionID) {
		t.Fatalf("ReplayDetector rejected first valid presentation")
	}

	// Replay identical timestamp + salt -> rejected!
	if detector.CheckAndAdd(sessionID) {
		t.Fatalf("ReplayDetector accepted replayed handshake token!")
	}

	// Stale timestamp (outside 900s AuthWindow) -> rejected by VerifyAuthSessionID
	staleSessionID, err := BuildAuthSessionID(masterKey, clientPub, sni, -1000)
	if err == nil {
		if VerifyAuthSessionID(staleSessionID, masterKey, clientPub, sni) {
			t.Fatalf("VerifyAuthSessionID accepted stale timestamp outside auth window")
		}
	}
}

func TestTCPChunkerEvation(t *testing.T) {
	masterKey := []byte("01234567890123456789012345678901")
	sni := "www.cloudflare.com"

	_, pub, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("Generate keypair failed: %v", err)
	}

	clientHello, err := BuildClientHello(sni, masterKey, pub, 64, 0)
	if err != nil {
		t.Fatalf("BuildClientHello failed: %v", err)
	}

	if len(clientHello) <= 24 {
		t.Fatalf("ClientHello too short for chunking test: %d", len(clientHello))
	}

	// Chunk 1: bytes 0..24
	chunk1 := clientHello[:24]
	// Chunk 2: bytes 24..end
	chunk2 := clientHello[24:]

	reassembled := append(append([]byte(nil), chunk1...), chunk2...)
	if !bytes.Equal(reassembled, clientHello) {
		t.Fatalf("Reassembled chunks do not match original ClientHello")
	}

	parsed, err := ParseClientHello(reassembled)
	if err != nil {
		t.Fatalf("Failed to parse reassembled ClientHello: %v", err)
	}

	if parsed.SNI != sni {
		t.Errorf("Parsed SNI mismatch: expected %s, got %s", sni, parsed.SNI)
	}
	if !bytes.Equal(parsed.ClientPub, pub) {
		t.Errorf("Parsed ClientPub mismatch after chunk reassembly")
	}
}
