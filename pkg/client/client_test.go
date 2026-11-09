package client

import (
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
