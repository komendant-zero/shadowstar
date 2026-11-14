package client

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shadowstar/shadowstar/pkg/protocol"
)

// HealthConfig holds operational parameters for HTTP 204 URL-testing and auto-failover
type HealthConfig struct {
	Enabled           bool          `json:"enabled"`
	TestURL           string        `json:"test_url"`           // Default: "http://cp.cloudflare.com/generate_204"
	FallbackURL       string        `json:"fallback_url"`       // Default: "http://connectivitycheck.gstatic.com/generate_204"
	Interval          time.Duration `json:"interval"`           // Default: 30 * time.Second
	Timeout           time.Duration `json:"timeout"`            // Default: 3000 * time.Millisecond
	FailoverThreshold int           `json:"failover_threshold"` // Default: 3
	RecoveryThreshold int           `json:"recovery_threshold"` // Default: 2
	RecoveryInterval  time.Duration `json:"recovery_interval"`  // Default: 60 * time.Second
}

// DefaultHealthConfig returns production defaults conforming to Milestone 3
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		Enabled:           true,
		TestURL:           "http://cp.cloudflare.com/generate_204",
		FallbackURL:       "http://connectivitycheck.gstatic.com/generate_204",
		Interval:          30 * time.Second,
		Timeout:           3000 * time.Millisecond,
		FailoverThreshold: 3,
		RecoveryThreshold: 2,
		RecoveryInterval:  60 * time.Second,
	}
}

// NodeHealth maintains runtime health telemetry for a server profile
type NodeHealth struct {
	ProfileID            string        `json:"profile_id"`
	ServerAddr           string        `json:"server_addr"`
	ConsecutiveFailures  int           `json:"consecutive_failures"`
	ConsecutiveSuccesses int           `json:"consecutive_successes"`
	LastLatency          time.Duration `json:"last_latency"`
	LastCheck            time.Time     `json:"last_check"`
	IsHealthy            bool          `json:"is_healthy"`
	LastError            string        `json:"last_error,omitempty"`
}

// SwitchCallback is notified whenever an automated failover or manual node change occurs
type SwitchCallback func(oldProfile, newProfile *ServerProfile)

// HealthMonitor defines the interface contract for health testing and failover
type HealthMonitor interface {
	CheckOnce(profile *ServerProfile) (latencyMs int, err error)
	CheckURL(targetURL string, timeout time.Duration) (latencyMs int, err error)
	ProbeNode(p *ServerProfile) bool
	StartAutoFailover(onSwitch SwitchCallback)
	Stop()
	GetNodeHealth(profileID string) (NodeHealth, bool)
	GetAllHealth() map[string]NodeHealth
}

type healthMonitorImpl struct {
	mu              sync.RWMutex
	client          *Client
	config          HealthConfig
	health          map[string]*NodeHealth
	onSwitch        SwitchCallback
	stopChan        chan struct{}
	running         bool
	serversProvider func() ([]ServerProfile, string)
}

// NewHealthMonitor creates a health monitor instance attached to client
func NewHealthMonitor(c *Client, cfg HealthConfig, serversProvider ...func() ([]ServerProfile, string)) HealthMonitor {
	var sp func() ([]ServerProfile, string)
	if len(serversProvider) > 0 && serversProvider[0] != nil {
		sp = serversProvider[0]
	} else {
		sp = func() ([]ServerProfile, string) {
			return c.GetServers(), c.GetActiveGroupID()
		}
	}

	return &healthMonitorImpl{
		client:          c,
		config:          cfg,
		health:          make(map[string]*NodeHealth),
		stopChan:        make(chan struct{}),
		serversProvider: sp,
	}
}

func (h *healthMonitorImpl) checkSingleURL(targetURL string, timeout time.Duration) (int, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return 0, fmt.Errorf("invalid test url: %w", err)
	}

	host := u.Host
	port := "80"
	if strings.Contains(host, ":") {
		hPart, pPart, err := net.SplitHostPort(host)
		if err == nil {
			host = hPart
			port = pPart
		}
	} else if u.Scheme == "https" {
		port = "443"
	}

	destAddr := net.JoinHostPort(host, port)

	// Route exclusively through active encrypted tunnel
	conn, err := h.client.DialTunnel(destAddr)
	if err != nil {
		return 0, fmt.Errorf("tunnel dial failed: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	path := u.RequestURI()
	if path == "" {
		path = "/generate_204"
	}

	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36\r\nConnection: close\r\n\r\n", path, host)

	start := time.Now()
	if _, err := conn.Write([]byte(req)); err != nil {
		return 0, fmt.Errorf("write request failed: %w", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("read response failed: %w", err)
	}
	rtt := time.Since(start)

	parts := strings.Split(strings.TrimSpace(statusLine), " ")
	if len(parts) < 2 {
		return 0, fmt.Errorf("malformed status line: %q", statusLine)
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid status code in %q: %w", statusLine, err)
	}

	if code != http.StatusNoContent && code != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", code)
	}

	latencyMs := int(rtt.Milliseconds())
	if latencyMs <= 0 {
		latencyMs = 1
	}
	return latencyMs, nil
}

func (h *healthMonitorImpl) CheckURL(targetURL string, timeout time.Duration) (int, error) {
	return h.checkSingleURL(targetURL, timeout)
}

func (h *healthMonitorImpl) CheckOnce(profile *ServerProfile) (int, error) {
	if profile == nil {
		p := h.client.GetProfile()
		profile = &p
	}

	h.mu.Lock()
	nh, exists := h.health[profile.ID]
	if !exists {
		nh = &NodeHealth{
			ProfileID:  profile.ID,
			ServerAddr: profile.ServerAddr,
			IsHealthy:  true,
		}
		h.health[profile.ID] = nh
	}
	h.mu.Unlock()

	// Try primary endpoint (cp.cloudflare.com)
	latency, err := h.checkSingleURL(h.config.TestURL, h.config.Timeout)
	if err != nil && h.config.FallbackURL != "" {
		// Fallback to secondary endpoint (connectivitycheck.gstatic.com)
		latency, err = h.checkSingleURL(h.config.FallbackURL, h.config.Timeout)
	}

	h.mu.Lock()
	nh.LastCheck = time.Now()
	if err == nil {
		nh.ConsecutiveSuccesses++
		nh.ConsecutiveFailures = 0
		nh.IsHealthy = true
		nh.LastLatency = time.Duration(latency) * time.Millisecond
		nh.LastError = ""
		h.client.latency.Store(int64(latency))
		profile.Ping = latency
		h.mu.Unlock()
		return latency, nil
	}

	nh.ConsecutiveFailures++
	nh.ConsecutiveSuccesses = 0
	nh.LastError = err.Error()

	shouldFailover := nh.ConsecutiveFailures >= h.config.FailoverThreshold
	if shouldFailover {
		nh.IsHealthy = false
	}
	h.mu.Unlock()

	if shouldFailover {
		h.triggerFailover()
	}

	return 0, err
}

func (h *healthMonitorImpl) triggerFailover() {
	curProfile := h.client.GetProfile()
	servers, activeGroupID := h.serversProvider()

	if len(servers) <= 1 {
		h.client.Log(fmt.Sprintf("[Failover-Warning] Active node %s failed (%d times), but no alternative candidate nodes configured", curProfile.Name, h.config.FailoverThreshold))
		return
	}

	h.mu.RLock()
	var groupCandidates []ServerProfile
	var allCandidates []ServerProfile

	for _, s := range servers {
		if s.ID == curProfile.ID {
			continue
		}
		nh, exists := h.health[s.ID]
		isHealthy := !exists || nh.IsHealthy

		if isHealthy {
			if activeGroupID != "" && s.GroupID == activeGroupID {
				groupCandidates = append(groupCandidates, s)
			}
			allCandidates = append(allCandidates, s)
		}
	}
	h.mu.RUnlock()

	var selected *ServerProfile

	// Rank candidates: lowest latency first
	pickBest := func(cands []ServerProfile) *ServerProfile {
		if len(cands) == 0 {
			return nil
		}
		best := &cands[0]
		for i := 1; i < len(cands); i++ {
			if cands[i].Ping > 0 && (best.Ping <= 0 || cands[i].Ping < best.Ping) {
				best = &cands[i]
			}
		}
		return best
	}

	if len(groupCandidates) > 0 {
		selected = pickBest(groupCandidates)
	} else if len(allCandidates) > 0 {
		selected = pickBest(allCandidates)
	} else {
		// All nodes marked unhealthy -> circular deadlock breaker
		curIdx := 0
		for i, s := range servers {
			if s.ID == curProfile.ID {
				curIdx = i
				break
			}
		}
		nextIdx := (curIdx + 1) % len(servers)
		selected = &servers[nextIdx]

		h.mu.Lock()
		if nh, ok := h.health[selected.ID]; ok {
			nh.ConsecutiveFailures = 0
			nh.IsHealthy = true
		}
		h.mu.Unlock()
	}

	if selected != nil {
		h.client.Log(fmt.Sprintf("[Auto-Failover] Node %s exceeded failure threshold (%d). Switching to healthy candidate: %s (%s)",
			curProfile.Name, h.config.FailoverThreshold, selected.Name, selected.ServerAddr))

		oldCopy := curProfile
		newCopy := *selected
		h.client.UpdateProfile(newCopy)

		h.mu.RLock()
		cb := h.onSwitch
		h.mu.RUnlock()
		if cb != nil {
			cb(&oldCopy, &newCopy)
		}
	}
}

func (h *healthMonitorImpl) ProbeNode(p *ServerProfile) bool {
	h.mu.RLock()
	nh, exists := h.health[p.ID]
	h.mu.RUnlock()

	conn, err := net.DialTimeout("tcp", p.ServerAddr, h.config.Timeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	primaryKey, _ := protocol.NormalizeMasterKey(p.MasterKey)
	_, sp, hErr := protocol.PerformClientHandshake(conn, primaryKey, p.SNI, p.EnableChunker, p.PaddingLen, p.TimeOffset)
	if hErr == nil && sp != nil {
		h.mu.Lock()
		defer h.mu.Unlock()
		if !exists {
			nh = &NodeHealth{
				ProfileID:  p.ID,
				ServerAddr: p.ServerAddr,
			}
			h.health[p.ID] = nh
		}
		nh.ConsecutiveSuccesses++
		if nh.ConsecutiveSuccesses >= h.config.RecoveryThreshold {
			nh.IsHealthy = true
			nh.ConsecutiveFailures = 0
			nh.LastError = ""
		}
		return true
	}
	return false
}

func (h *healthMonitorImpl) startRecoveryLoop() {
	ticker := time.NewTicker(h.config.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			servers, _ := h.serversProvider()
			for _, p := range servers {
				h.mu.RLock()
				nh, exists := h.health[p.ID]
				h.mu.RUnlock()

				if exists && !nh.IsHealthy {
					if h.ProbeNode(&p) {
						h.client.Log(fmt.Sprintf("[Recovery-Prober] Node %s probed successfully", p.Name))
					}
				}
			}
		}
	}
}

func (h *healthMonitorImpl) StartAutoFailover(onSwitch SwitchCallback) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.onSwitch = onSwitch
	h.mu.Unlock()

	go h.startRecoveryLoop()

	go func() {
		ticker := time.NewTicker(h.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-h.stopChan:
				return
			case <-ticker.C:
				if !h.client.running {
					continue
				}
				p := h.client.GetProfile()
				_, _ = h.CheckOnce(&p)
			}
		}
	}()
}

func (h *healthMonitorImpl) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	h.running = false
	close(h.stopChan)
}

func (h *healthMonitorImpl) GetNodeHealth(profileID string) (NodeHealth, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	nh, ok := h.health[profileID]
	if !ok {
		return NodeHealth{}, false
	}
	return *nh, true
}

func (h *healthMonitorImpl) GetAllHealth() map[string]NodeHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]NodeHealth, len(h.health))
	for k, v := range h.health {
		res[k] = *v
	}
	return res
}
