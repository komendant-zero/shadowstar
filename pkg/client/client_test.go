package client

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shadowstar/shadowstar/pkg/server"
)

func TestServerProfile_SNIPoolSanitization(t *testing.T) {
	p := ServerProfile{
		ID:         "profile-1",
		ServerAddr: "192.168.1.1",
		MasterKey:  "secret-key",
		SNI:        "www.cloudflare.com:443",
		SNIPool: []string{
			"www.google.com:443",
			"www.cloudflare.com",
			"  api.apple.com  ",
			"www.google.com", // duplicate
		},
	}

	sanitizeProfile(&p)

	if p.ServerAddr != "192.168.1.1:4433" {
		t.Errorf("Expected ServerAddr 192.168.1.1:4433, got %s", p.ServerAddr)
	}
	if p.SNI != "www.cloudflare.com" {
		t.Errorf("Expected SNI www.cloudflare.com, got %s", p.SNI)
	}
	if p.PaddingLen != 64 {
		t.Errorf("Expected default PaddingLen 64, got %d", p.PaddingLen)
	}
	if p.Fingerprint != "chrome_133" {
		t.Errorf("Expected default Fingerprint chrome_133, got %s", p.Fingerprint)
	}

	// Verify SNIPool has no duplicates, stripped ports, trimmed spaces
	expectedPool := []string{"www.google.com", "www.cloudflare.com", "api.apple.com"}
	if len(p.SNIPool) != len(expectedPool) {
		t.Fatalf("Expected %d SNIPool items, got %d: %v", len(expectedPool), len(p.SNIPool), p.SNIPool)
	}
	for i, exp := range expectedPool {
		if p.SNIPool[i] != exp {
			t.Errorf("Pool[%d]: expected %s, got %s", i, exp, p.SNIPool[i])
		}
	}
}

func TestSNIPoolManager_RoundRobinAndQuarantine(t *testing.T) {
	pool := []string{"domain-a.com", "domain-b.com", "domain-c.com"}
	mgr := NewSNIPoolManager(pool, "domain-a.com")

	// Round-robin selection: A -> B -> C -> A
	first := mgr.SelectSNI()
	second := mgr.SelectSNI()
	third := mgr.SelectSNI()
	fourth := mgr.SelectSNI()

	if first != "domain-a.com" || second != "domain-b.com" || third != "domain-c.com" || fourth != "domain-a.com" {
		t.Fatalf("Unexpected round-robin sequence: [%s, %s, %s, %s]", first, second, third, fourth)
	}

	// Report TCP RST error on domain-b.com -> immediate quarantine
	mgr.ReportFailure("domain-b.com", errors.New("read: connection reset by peer"))
	if !mgr.IsQuarantined("domain-b.com") {
		t.Fatalf("domain-b.com should be quarantined after TCP RST error")
	}

	// Selections should now skip domain-b.com: selecting only A and C
	for i := 0; i < 6; i++ {
		sni := mgr.SelectSNI()
		if sni == "domain-b.com" {
			t.Fatalf("Quarantined domain-b.com was selected during rotation!")
		}
		if sni != "domain-a.com" && sni != "domain-c.com" {
			t.Fatalf("Unexpected SNI selected: %s", sni)
		}
	}

	// Report success -> lifts quarantine
	mgr.ReportSuccess("domain-b.com")
	if mgr.IsQuarantined("domain-b.com") {
		t.Fatalf("domain-b.com still quarantined after ReportSuccess")
	}
}

func TestSNIPoolManager_AllQuarantinedFallback(t *testing.T) {
	pool := []string{"sni1.com", "sni2.com", "sni3.com"}
	mgr := NewSNIPoolManager(pool, "sni1.com")

	// Set short quarantine duration for testing
	mgr.SetQuarantineDuration(50 * time.Millisecond)

	mgr.ReportFailure("sni1.com", errors.New("connection reset"))
	time.Sleep(10 * time.Millisecond)
	mgr.ReportFailure("sni2.com", errors.New("connection reset"))
	time.Sleep(10 * time.Millisecond)
	mgr.ReportFailure("sni3.com", errors.New("connection reset"))

	// All 3 are quarantined. Fallback should pick earliest expiring (sni1.com)
	fallback := mgr.SelectSNI()
	if fallback != "sni1.com" {
		t.Fatalf("Expected fallback to earliest expiring sni1.com, got %s", fallback)
	}

	// Wait for quarantine to naturally expire
	time.Sleep(60 * time.Millisecond)
	if mgr.IsQuarantined("sni1.com") || mgr.IsQuarantined("sni2.com") || mgr.IsQuarantined("sni3.com") {
		t.Fatalf("Quarantine should have expired after duration")
	}
}

func TestSNIPoolManager_ConsecutiveErrorsThreshold(t *testing.T) {
	pool := []string{"target.com"}
	mgr := NewSNIPoolManager(pool, "target.com")

	genericErr := errors.New("handshake authentication failed")

	// 1st failure: below threshold (MaxConsecutiveSNIErrors = 2)
	mgr.ReportFailure("target.com", genericErr)
	if mgr.IsQuarantined("target.com") {
		t.Fatalf("SNI quarantined on first generic error (should require 2)")
	}

	// 2nd failure: reaches threshold -> quarantined
	mgr.ReportFailure("target.com", genericErr)
	if !mgr.IsQuarantined("target.com") {
		t.Fatalf("SNI not quarantined after 2 consecutive errors")
	}
}

func TestUDP_JitterTiming(t *testing.T) {
	profile := ServerProfile{
		ID:         "test",
		ServerAddr: "127.0.0.1:4433",
		MasterKey:  "masterkey12345678901234567890123",
		SNI:        "www.microsoft.com",
	}
	cli, err := NewClient(profile, "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	maxJitter := 25 * time.Millisecond
	cli.SetUDPJitterMax(maxJitter)
	cli.SetUDPObfuscation(true)

	var sawNonZero bool
	var minVal time.Duration = time.Hour
	var maxVal time.Duration = 0

	for i := 0; i < 50; i++ {
		j := cli.calculateUDPJitter()
		if j < 0 || j > maxJitter {
			t.Fatalf("calculateUDPJitter %v out of range [0, %v]", j, maxJitter)
		}
		if j > 0 {
			sawNonZero = true
		}
		if j < minVal {
			minVal = j
		}
		if j > maxVal {
			maxVal = j
		}
	}

	if !sawNonZero {
		t.Fatalf("calculateUDPJitter returned 0 for all 50 iterations")
	}

	// Disable obfuscation -> must return 0
	cli.SetUDPObfuscation(false)
	for i := 0; i < 10; i++ {
		if j := cli.calculateUDPJitter(); j != 0 {
			t.Fatalf("calculateUDPJitter returned non-zero %v when obfuscation disabled", j)
		}
	}
}

func TestEndToEndProxyFlow(t *testing.T) {
	// 1. Start a local HTTP echo/target server
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind target listener: %v", err)
	}
	defer targetListener.Close()

	targetAddr := targetListener.Addr().String()
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Hello from protected destination through ShadowStar!"))
		})
		_ = http.Serve(targetListener, mux)
	}()

	// 2. Start ShadowStar Server on an ephemeral port
	srvListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind server listener: %v", err)
	}
	serverAddr := srvListener.Addr().String()
	srvListener.Close() // freed for server

	masterKey := "TestMasterKeyForIntegrationTesting2026!"
	srvCfg := &server.Config{
		ListenAddr:     serverAddr,
		MasterKey:      masterKey,
		FallbackTarget: "www.microsoft.com:443",
		SNI:            "www.microsoft.com",
		Verbose:        false,
	}
	srv, err := server.NewServer(srvCfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer srv.Close()

	go func() {
		_ = srv.Start()
	}()
	time.Sleep(100 * time.Millisecond)

	// 3. Find free ports for client listeners
	l1, _ := net.Listen("tcp", "127.0.0.1:0")
	socksAddr := l1.Addr().String()
	l1.Close()

	l2, _ := net.Listen("tcp", "127.0.0.1:0")
	httpAddr := l2.Addr().String()
	l2.Close()

	profile := ServerProfile{
		ID:            "test-node",
		Name:          "Test Node",
		ServerAddr:    serverAddr,
		MasterKey:     masterKey,
		SNI:           "www.microsoft.com",
		EnableChunker: false, // fast in-process loopback test
		PaddingLen:    32,
	}

	cli, err := NewClient(profile, socksAddr, httpAddr)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	if err := cli.Start(); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	defer cli.Stop()

	time.Sleep(200 * time.Millisecond)

	// 4. Test HTTP Proxy through Client
	proxyURL, _ := url.Parse("http://" + httpAddr)
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	reqURL := fmt.Sprintf("http://%s/hello", targetAddr)
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		if strings.Contains(err.Error(), "forbidden by its access permissions") {
			t.Skipf("Skipping loopback socket connection in sandboxed container: %v", err)
			return
		}
		t.Fatalf("Failed to execute request via ShadowStar proxy: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expected := "Hello from protected destination through ShadowStar!"
	if string(body) != expected {
		t.Fatalf("Unexpected response body: got %q, expected %q", string(body), expected)
	}

	// Verify stats telemetry updated
	stats := cli.GetStats()
	if stats.BytesDown == 0 || stats.BytesUp == 0 {
		t.Errorf("Telemetry counters were not updated: Up=%d, Down=%d", stats.BytesUp, stats.BytesDown)
	}
}
