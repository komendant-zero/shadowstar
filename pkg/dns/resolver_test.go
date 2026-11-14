package dns

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolver_DoH_Mock(t *testing.T) {
	// Start mock DoH HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/dns-message" {
			http.Error(w, "invalid accept header", http.StatusNotAcceptable)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil || len(data) < 12 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		reqMsg, err := ParseMessage(data)
		if err != nil || len(reqMsg.Questions) == 0 {
			http.Error(w, "bad dns packet", http.StatusBadRequest)
			return
		}

		// Verify ID=0 per RFC 8484
		if reqMsg.ID != 0 {
			t.Logf("Notice: DoH query ID was %d (RFC 8484 suggests 0)", reqMsg.ID)
		}

		ans := ResourceRecord{
			Name:  reqMsg.Questions[0].Name,
			Type:  TypeA,
			Class: ClassINET,
			TTL:   60,
			Data:  []byte{93, 184, 216, 34}, // 93.184.216.34 (example.com)
		}
		resp := BuildResponse(reqMsg, []ResourceRecord{ans}, RCodeNoError)

		w.Header().Set("Content-Type", "application/dns-message")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(resp)
	}))
	defer ts.Close()

	cfg := &Config{
		UpstreamDoH: []string{ts.URL},
	}
	resolver, err := NewResolver(cfg)
	if err != nil {
		t.Fatalf("NewResolver failed: %v", err)
	}

	ips, err := resolver.Resolve("example.com")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(ips) != 1 {
		t.Fatalf("expected 1 IP, got %d", len(ips))
	}
	expected := net.IPv4(93, 184, 216, 34)
	if !ips[0].Equal(expected) {
		t.Errorf("expected %s, got %s", expected, ips[0])
	}
}

func TestResolver_DoT_Mock(t *testing.T) {
	cert, key := generateSelfSignedCert(t)
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		t.Fatalf("failed to create TLS key pair: %v", err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		prefix := make([]byte, 2)
		if _, err := io.ReadFull(conn, prefix); err != nil {
			return
		}
		pLen := binary.BigEndian.Uint16(prefix)
		reqBuf := make([]byte, pLen)
		if _, err := io.ReadFull(conn, reqBuf); err != nil {
			return
		}

		req, err := ParseMessage(reqBuf)
		if err != nil || len(req.Questions) == 0 {
			return
		}

		ans := ResourceRecord{
			Name:  req.Questions[0].Name,
			Type:  TypeA,
			Class: ClassINET,
			TTL:   300,
			Data:  []byte{1, 1, 1, 1},
		}
		resp := BuildResponse(req, []ResourceRecord{ans}, RCodeNoError)

		binary.BigEndian.PutUint16(prefix, uint16(len(resp)))
		_, _ = conn.Write(prefix)
		_, _ = conn.Write(resp)
	}()

	cfg := &Config{
		UpstreamDoT: []string{listener.Addr().String()},
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	res, err := NewResolver(cfg)
	if err != nil {
		t.Fatalf("NewResolver failed: %v", err)
	}

	ips, err := res.Resolve("example.com")
	if err != nil {
		t.Fatalf("DoT Resolve failed: %v", err)
	}
	if len(ips) != 1 {
		t.Fatalf("expected 1 IP from DoT resolver, got %d", len(ips))
	}
	expected := net.IPv4(1, 1, 1, 1)
	if !ips[0].Equal(expected) {
		t.Errorf("expected %s, got %s", expected, ips[0])
	}
}

func generateSelfSignedCert(t *testing.T) ([]byte, []byte) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"ShadowStar Test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	certPEM := encodePEM("CERTIFICATE", derBytes)
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	keyPEM := encodePEM("RSA PRIVATE KEY", privBytes)
	return certPEM, keyPEM
}

func encodePEM(typeStr string, raw []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("-----BEGIN " + typeStr + "-----\n")
	b64 := x509EncodeB64(raw)
	for len(b64) > 64 {
		buf.WriteString(b64[:64] + "\n")
		b64 = b64[64:]
	}
	buf.WriteString(b64 + "\n")
	buf.WriteString("-----END " + typeStr + "-----\n")
	return buf.Bytes()
}

func x509EncodeB64(b []byte) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var buf bytes.Buffer
	for i := 0; i < len(b); i += 3 {
		remaining := len(b) - i
		var b0, b1, b2 byte
		b0 = b[i]
		if remaining > 1 {
			b1 = b[i+1]
		}
		if remaining > 2 {
			b2 = b[i+2]
		}
		buf.WriteByte(table[(b0>>2)&0x3F])
		buf.WriteByte(table[((b0&0x3)<<4)|((b1>>4)&0x0F)])
		if remaining > 1 {
			buf.WriteByte(table[((b1&0x0F)<<2)|((b2>>6)&0x03)])
		} else {
			buf.WriteByte('=')
		}
		if remaining > 2 {
			buf.WriteByte(table[b2&0x3F])
		} else {
			buf.WriteByte('=')
		}
	}
	return buf.String()
}
