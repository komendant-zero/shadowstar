package protocol

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"io"
	"sync"
	"testing"
	"time"
)

func TestUDPTransport_BasicBidirectional(t *testing.T) {
	listener, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	var wg sync.WaitGroup
	wg.Add(1)

	// Server handler
	var serverReceived []byte
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("listener.Accept failed: %v", err)
			return
		}

		buf := make([]byte, 5)
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Errorf("server ReadFull failed: %v", err)
			return
		}
		serverReceived = buf

		// Echo back with greeting
		if _, err := conn.Write([]byte("WORLD")); err != nil {
			t.Errorf("server Write failed: %v", err)
			return
		}
	}()

	// Client
	clientConn, err := DialUDP(serverAddr)
	if err != nil {
		t.Fatalf("DialUDP failed: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte("HELLO")); err != nil {
		t.Fatalf("client Write failed: %v", err)
	}

	replyBuf := make([]byte, 5)
	if _, err := io.ReadFull(clientConn, replyBuf); err != nil {
		t.Fatalf("client ReadFull failed: %v", err)
	}

	wg.Wait()

	if string(serverReceived) != "HELLO" {
		t.Errorf("expected server to receive 'HELLO', got %q", string(serverReceived))
	}
	if string(replyBuf) != "WORLD" {
		t.Errorf("expected client to receive 'WORLD', got %q", string(replyBuf))
	}
}

func TestUDPTransport_LargePayloadStreaming(t *testing.T) {
	listener, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer listener.Close()

	// 256 KB of random data spanning hundreds of UDP packets
	dataSize := 256 * 1024
	testData := make([]byte, dataSize)
	_, _ = rand.Read(testData)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}
		defer conn.Close()

		recvBuf := make([]byte, dataSize)
		if _, err := io.ReadFull(conn, recvBuf); err != nil {
			t.Errorf("Server ReadFull error: %v", err)
			return
		}

		if !bytes.Equal(recvBuf, testData) {
			t.Errorf("Server received corrupted data!")
		}

		// Echo first 16 bytes as confirmation
		_, _ = conn.Write(recvBuf[:16])
	}()

	clientConn, err := DialUDP(listener.Addr().String())
	if err != nil {
		t.Fatalf("DialUDP error: %v", err)
	}
	defer clientConn.Close()

	// Stream all data
	if _, err := clientConn.Write(testData); err != nil {
		t.Fatalf("Client Write error: %v", err)
	}

	confirmBuf := make([]byte, 16)
	if _, err := io.ReadFull(clientConn, confirmBuf); err != nil {
		t.Fatalf("Client Read confirmation error: %v", err)
	}

	if !bytes.Equal(confirmBuf, testData[:16]) {
		t.Fatalf("Confirmation mismatch!")
	}

	wg.Wait()
}

func TestUDPTransport_ReadDeadline(t *testing.T) {
	listener, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer listener.Close()

	clientConn, err := DialUDP(listener.Addr().String())
	if err != nil {
		t.Fatalf("DialUDP failed: %v", err)
	}
	defer clientConn.Close()

	// Set short read deadline
	_ = clientConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 10)
	_, err = clientConn.Read(buf)
	if err == nil {
		t.Fatal("expected read timeout error, got nil")
	}
}

func TestUDPTransport_ShadowStarProtocolHandshakeAndMux(t *testing.T) {
	masterKey := []byte("01d7b18d323cbb7a771ec7baffd331cff73f188b620f7d56a3dfc22c6cf535b2")
	sni := "www.microsoft.com"

	listener, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer listener.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	// Server goroutine
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("listener.Accept failed: %v", err)
			return
		}

		// Read TLS record header (5 bytes)
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			t.Errorf("server ReadFull header error: %v", err)
			return
		}

		recordLen := int(binary.BigEndian.Uint16(header[3:5]))
		body := make([]byte, recordLen)
		if _, err := io.ReadFull(conn, body); err != nil {
			t.Errorf("server ReadFull body error: %v", err)
			return
		}

		fullHello := append(header, body...)
		parsed, err := ParseClientHello(fullHello)
		if err != nil || parsed == nil {
			t.Errorf("ParseClientHello error: %v", err)
			return
		}

		if !VerifyAuthSessionID(parsed.SessionID, masterKey, parsed.ClientPub, parsed.SNI) {
			t.Errorf("VerifyAuthSessionID failed!")
			return
		}

		serverPriv, serverPub, err := GenerateX25519KeyPair()
		if err != nil {
			t.Errorf("GenerateX25519KeyPair error: %v", err)
			return
		}

		sharedSecret, err := ComputeECDH(serverPriv, parsed.ClientPub)
		if err != nil {
			t.Errorf("ComputeECDH error: %v", err)
			return
		}

		clientKey, serverKey, clientIV, serverIV, err := DeriveSessionKeys(sharedSecret, masterKey)
		if err != nil {
			t.Errorf("DeriveSessionKeys error: %v", err)
			return
		}

		serverHello := BuildServerHello(serverPub, parsed.SessionID)
		if _, err := conn.Write(serverHello); err != nil {
			t.Errorf("conn.Write serverHello error: %v", err)
			return
		}

		frameConn, err := NewSecureFrameConn(conn, serverKey, serverIV, clientKey, clientIV)
		if err != nil {
			t.Errorf("NewSecureFrameConn error: %v", err)
			return
		}

		sess := NewMuxSession(frameConn, conn, false)

		stream, err := sess.AcceptStream()
		if err != nil {
			t.Errorf("AcceptStream error: %v", err)
			return
		}

		data := make([]byte, 1024)
		n, err := stream.Read(data)
		t.Logf("Server: read %d bytes: %s", n, string(data[:n]))
		if err != nil {
			t.Errorf("stream.Read error: %v", err)
			return
		}
		if string(data[:n]) != "UDP_SHADOWSTAR_PING" {
			t.Errorf("unexpected stream payload: %s", string(data[:n]))
			return
		}

		t.Log("Server: writing PONG...")
		_, _ = stream.Write([]byte("UDP_SHADOWSTAR_PONG"))
		stream.Close()
		sess.Close()
		conn.Close()
	}()

	// Client
	clientConn, err := DialUDP(listener.Addr().String())
	if err != nil {
		t.Fatalf("DialUDP failed: %v", err)
	}
	defer clientConn.Close()

	clientPriv, serverPub, err := PerformClientHandshake(clientConn, masterKey, sni, false, 0, 0)
	if err != nil {
		t.Fatalf("PerformClientHandshake failed: %v", err)
	}

	sharedSecret, err := ComputeECDH(clientPriv, serverPub)
	if err != nil {
		t.Fatalf("ComputeECDH failed: %v", err)
	}

	clientKey, serverKey, clientIV, serverIV, err := DeriveSessionKeys(sharedSecret, masterKey)
	if err != nil {
		t.Fatalf("DeriveSessionKeys failed: %v", err)
	}

	frameConn, err := NewSecureFrameConn(clientConn, clientKey, clientIV, serverKey, serverIV)
	if err != nil {
		t.Fatalf("NewSecureFrameConn failed: %v", err)
	}

	sess := NewMuxSession(frameConn, clientConn, true)
	defer sess.Close()

	stream, err := sess.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer stream.Close()

	if _, err := stream.Write([]byte("UDP_SHADOWSTAR_PING")); err != nil {
		t.Fatalf("stream.Write failed: %v", err)
	}

	reply := make([]byte, 1024)
	n, err := stream.Read(reply)
	if err != nil {
		t.Fatalf("stream.Read reply failed: %v", err)
	}

	if string(reply[:n]) != "UDP_SHADOWSTAR_PONG" {
		t.Fatalf("expected PONG, got: %s", string(reply[:n]))
	}

	wg.Wait()
}
