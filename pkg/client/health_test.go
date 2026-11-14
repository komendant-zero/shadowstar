package client

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/shadowstar/shadowstar/pkg/server"
)

func TestHealthCheck_HTTP204Success(t *testing.T) {
	// Start mock captive portal server returning 204 No Content
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind mock 204 listener: %v", err)
	}
	defer listener.Close()

	mockAddr := listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/generate_204", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	go func() {
		_ = http.Serve(listener, mux)
	}()

	profile := ServerProfile{
		ID:         "node-primary",
		Name:       "Primary Node",
		ServerAddr: "1.2.3.4:4433",
		MasterKey:  "testmasterkey123456789012345678",
		SNI:        "www.microsoft.com",
	}

	cli, err := NewClient(profile, "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Dial mock captive portal server
	cli.SetTunnelDialer(func(destAddr string) (net.Conn, error) {
		return net.DialTimeout("tcp", mockAddr, 2*time.Second)
	})

	cfg := DefaultHealthConfig()
	cfg.TestURL = fmt.Sprintf("http://%s/generate_204", mockAddr)
	cfg.Timeout = 2000 * time.Millisecond

	hm := NewHealthMonitor(cli, cfg)
	latency, err := hm.CheckOnce(&profile)
	if err != nil {
		t.Fatalf("CheckOnce failed: %v", err)
	}
	if latency <= 0 {
		t.Errorf("Expected positive latency, got %d ms", latency)
	}

	nh, ok := hm.GetNodeHealth("node-primary")
	if !ok {
		t.Fatalf("GetNodeHealth returned false for node-primary")
	}
	if !nh.IsHealthy {
		t.Errorf("Expected node-primary to be healthy")
	}
	if nh.ConsecutiveSuccesses != 1 {
		t.Errorf("Expected 1 consecutive success, got %d", nh.ConsecutiveSuccesses)
	}
}

func TestHealthCheck_HTTPFallback(t *testing.T) {
	// Start fallback server returning 200 OK (also accepted)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind fallback listener: %v", err)
	}
	defer listener.Close()

	mockAddr := listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/generate_204", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	go func() {
		_ = http.Serve(listener, mux)
	}()

	profile := ServerProfile{
		ID:         "node-fallback",
		Name:       "Fallback Node",
		ServerAddr: "1.2.3.4:4433",
		MasterKey:  "testmasterkey123456789012345678",
		SNI:        "www.microsoft.com",
	}

	cli, err := NewClient(profile, "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Route traffic: fail primary (destPort 8888), succeed on fallback (mockAddr)
	cli.SetTunnelDialer(func(destAddr string) (net.Conn, error) {
		if destAddr == "unreachable.invalid:8888" {
			return nil, errors.New("primary endpoint timed out")
		}
		return net.DialTimeout("tcp", mockAddr, 2*time.Second)
	})

	cfg := DefaultHealthConfig()
	cfg.TestURL = "http://unreachable.invalid:8888/generate_204"
	cfg.FallbackURL = fmt.Sprintf("http://%s/generate_204", mockAddr)
	cfg.Timeout = 2000 * time.Millisecond

	hm := NewHealthMonitor(cli, cfg)
	latency, err := hm.CheckOnce(&profile)
	if err != nil {
		t.Fatalf("CheckOnce failed on fallback: %v", err)
	}
	if latency <= 0 {
		t.Errorf("Expected positive latency, got %d", latency)
	}
}

func TestAutoFailover_ThreeFailuresTriggerSwitch(t *testing.T) {
	profile1 := ServerProfile{
		ID:         "node-1",
		Name:       "Node 1",
		ServerAddr: "10.0.0.1:4433",
		MasterKey:  "masterkey12345678901234567890123",
		SNI:        "sni1.com",
		Ping:       50,
	}
	profile2 := ServerProfile{
		ID:         "node-2",
		Name:       "Node 2",
		ServerAddr: "10.0.0.2:4433",
		MasterKey:  "masterkey12345678901234567890123",
		SNI:        "sni2.com",
		Ping:       60,
	}

	cli, err := NewClient(profile1, "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	cli.SetServers([]ServerProfile{profile1, profile2})

	// Simulator: dial always fails
	cli.SetTunnelDialer(func(destAddr string) (net.Conn, error) {
		return nil, errors.New("tunnel connection timeout")
	})

	cfg := DefaultHealthConfig()
	cfg.FailoverThreshold = 3

	var switchedOld, switchedNew *ServerProfile
	switchCount := 0

	hm := NewHealthMonitor(cli, cfg)
	hm.StartAutoFailover(func(oldNode, newNode *ServerProfile) {
		switchCount++
		switchedOld = oldNode
		switchedNew = newNode
	})
	defer hm.Stop()

	// Failure 1: no switch
	_, _ = hm.CheckOnce(&profile1)
	if cli.GetProfile().ID != "node-1" {
		t.Fatalf("Switched prematurely on failure 1")
	}

	// Failure 2: no switch
	_, _ = hm.CheckOnce(&profile1)
	if cli.GetProfile().ID != "node-1" {
		t.Fatalf("Switched prematurely on failure 2")
	}

	// Failure 3: triggers failover to node-2!
	_, _ = hm.CheckOnce(&profile1)
	if cli.GetProfile().ID != "node-2" {
		t.Fatalf("Expected active profile to switch to node-2, got %s", cli.GetProfile().ID)
	}

	if switchCount != 1 {
		t.Errorf("Expected 1 switch callback invocation, got %d", switchCount)
	}
	if switchedOld == nil || switchedOld.ID != "node-1" {
		t.Errorf("switchedOld expected node-1, got %v", switchedOld)
	}
	if switchedNew == nil || switchedNew.ID != "node-2" {
		t.Errorf("switchedNew expected node-2, got %v", switchedNew)
	}

	// Verify node-1 is marked unhealthy
	nh, ok := hm.GetNodeHealth("node-1")
	if !ok || nh.IsHealthy {
		t.Errorf("Expected node-1 to be marked unhealthy, got %+v", nh)
	}
}

func TestCandidateSelection_LowestLatency(t *testing.T) {
	nodeActive := ServerProfile{
		ID:         "node-active",
		Name:       "Node Active",
		ServerAddr: "10.0.0.1:4433",
		MasterKey:  "key12345678901234567890123456789",
		SNI:        "sni1.com",
		Ping:       50,
	}
	nodeSlow := ServerProfile{
		ID:         "node-slow",
		Name:       "Node Slow",
		ServerAddr: "10.0.0.2:4433",
		MasterKey:  "key12345678901234567890123456789",
		SNI:        "sni2.com",
		Ping:       200,
	}
	nodeFast := ServerProfile{
		ID:         "node-fast",
		Name:       "Node Fast",
		ServerAddr: "10.0.0.3:4433",
		MasterKey:  "key12345678901234567890123456789",
		SNI:        "sni3.com",
		Ping:       25,
	}
	nodeMid := ServerProfile{
		ID:         "node-mid",
		Name:       "Node Mid",
		ServerAddr: "10.0.0.4:4433",
		MasterKey:  "key12345678901234567890123456789",
		SNI:        "sni4.com",
		Ping:       80,
	}

	cli, err := NewClient(nodeActive, "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	cli.SetServers([]ServerProfile{nodeActive, nodeSlow, nodeFast, nodeMid})
	cli.SetTunnelDialer(func(destAddr string) (net.Conn, error) {
		return nil, errors.New("timeout")
	})

	cfg := DefaultHealthConfig()
	cfg.FailoverThreshold = 1

	hm := NewHealthMonitor(cli, cfg)
	// Fail active node once -> triggers failover to lowest ping
	_, _ = hm.CheckOnce(&nodeActive)

	active := cli.GetProfile()
	if active.ID != "node-fast" {
		t.Fatalf("Expected candidate selection to pick node-fast (Ping 25ms), got %s (Ping %dms)", active.ID, active.Ping)
	}
}

func TestRecoveryProbing(t *testing.T) {
	// Start genuine server instance for live handshake probing
	srvListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind server listener: %v", err)
	}
	serverAddr := srvListener.Addr().String()
	srvListener.Close()

	masterKey := "TestRecoveryMasterKey2026Valid1234"
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

	recoveredNode := ServerProfile{
		ID:            "recovered-node",
		Name:          "Recovered Node",
		ServerAddr:    serverAddr,
		MasterKey:     masterKey,
		SNI:           "www.microsoft.com",
		EnableChunker: false,
		PaddingLen:    64,
	}

	cli, err := NewClient(recoveredNode, "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	cfg := DefaultHealthConfig()
	cfg.RecoveryThreshold = 2
	cfg.Timeout = 3 * time.Second

	hm := NewHealthMonitor(cli, cfg)

	// Simulate node initially failed
	healthImpl := hm.(*healthMonitorImpl)
	healthImpl.health[recoveredNode.ID] = &NodeHealth{
		ProfileID:           recoveredNode.ID,
		ServerAddr:          recoveredNode.ServerAddr,
		IsHealthy:           false,
		ConsecutiveFailures: 3,
	}

	// Probe 1: success, but below RecoveryThreshold (2)
	ok1 := hm.ProbeNode(&recoveredNode)
	if !ok1 {
		t.Fatalf("Probe 1 failed to connect to live server")
	}
	nh1, _ := hm.GetNodeHealth(recoveredNode.ID)
	if nh1.IsHealthy {
		t.Errorf("Node marked healthy prematurely after 1 probe")
	}

	// Probe 2: reaches threshold -> recovered!
	ok2 := hm.ProbeNode(&recoveredNode)
	if !ok2 {
		t.Fatalf("Probe 2 failed to connect to live server")
	}
	nh2, _ := hm.GetNodeHealth(recoveredNode.ID)
	if !nh2.IsHealthy {
		t.Errorf("Node not marked healthy after reaching recovery threshold")
	}
	if nh2.ConsecutiveFailures != 0 {
		t.Errorf("Expected consecutive failures to reset to 0, got %d", nh2.ConsecutiveFailures)
	}
}
