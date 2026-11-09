package mobile

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
)

//go:embed web/*
var WebFS embed.FS

// GetWebHandler returns an http.Handler serving the embedded mobile web assets
func GetWebHandler() http.Handler {
	sub, err := fs.Sub(WebFS, "web")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}

// MobileBridge manages the ShadowStar VPN engine on mobile operating systems (Android/iOS)
type MobileBridge struct {
	mu           sync.Mutex
	client       *client.Client
	activeConfig *client.Config
	connected    bool
	socksAddr    string
	httpAddr     string
	startTime    time.Time
}

var (
	bridgeInstance *MobileBridge
	bridgeOnce     sync.Once
)

// GetBridge returns the singleton instance of MobileBridge
func GetBridge() *MobileBridge {
	bridgeOnce.Do(func() {
		bridgeInstance = &MobileBridge{
			socksAddr: "127.0.0.1:10808",
			httpAddr:  "127.0.0.1:10809",
		}
	})
	return bridgeInstance
}

// StartVPN initializes and launches the ShadowStar client with the given profile JSON
func StartVPN(profileJSON string) (string, error) {
	b := GetBridge()
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.connected {
		return "already running", nil
	}

	var prof client.ServerProfile
	if err := json.Unmarshal([]byte(profileJSON), &prof); err != nil {
		return "", fmt.Errorf("failed to parse profile: %w", err)
	}

	if prof.ServerAddr == "" || prof.MasterKey == "" {
		return "", fmt.Errorf("invalid profile: server_addr and master_key are required")
	}

	if prof.SNI == "" {
		prof.SNI = "www.microsoft.com"
	}

	c, err := client.NewClient(prof, b.socksAddr, b.httpAddr)
	if err != nil {
		return "", fmt.Errorf("failed to initialize client: %w", err)
	}

	if err := c.Start(); err != nil {
		return "", fmt.Errorf("failed to start client: %w", err)
	}

	b.client = c
	b.connected = true
	b.startTime = time.Now()

	return fmt.Sprintf("CONNECTED|%s|%s", b.socksAddr, b.httpAddr), nil
}

// StopVPN stops the active VPN connection
func StopVPN() error {
	b := GetBridge()
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.connected || b.client == nil {
		return nil
	}

	err := b.client.Stop()
	b.connected = false
	b.client = nil
	return err
}

// IsConnected returns whether the mobile VPN tunnel is active
func IsConnected() bool {
	b := GetBridge()
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.connected
}

// MobileStats represents real-time telemetry for mobile UI
type MobileStats struct {
	Connected     bool   `json:"connected"`
	SpeedDown     int64  `json:"speed_down"`
	SpeedUp       int64  `json:"speed_up"`
	BytesDown     uint64 `json:"bytes_down"`
	BytesUp       uint64 `json:"bytes_up"`
	ActiveStreams int64  `json:"active_streams"`
	UptimeSec     int64  `json:"uptime_sec"`
}

// GetStatsJSON returns real-time connection telemetry serialized as JSON
func GetStatsJSON() string {
	b := GetBridge()
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.connected || b.client == nil {
		empty, _ := json.Marshal(MobileStats{Connected: false})
		return string(empty)
	}

	st := b.client.GetStats()
	uptime := int64(time.Since(b.startTime).Seconds())

	mStats := MobileStats{
		Connected:     true,
		SpeedDown:     int64(st.SpeedDown),
		SpeedUp:       int64(st.SpeedUp),
		BytesDown:     st.BytesDown,
		BytesUp:       st.BytesUp,
		ActiveStreams: st.ActiveConns,
		UptimeSec:     uptime,
	}

	out, _ := json.Marshal(mStats)
	return string(out)
}

// PingServer tests latency to a node in milliseconds
func PingServer(serverAddr string, timeoutSec int) int {
	if timeoutSec <= 0 {
		timeoutSec = 3
	}
	lat, err := config.PingServer(serverAddr, time.Duration(timeoutSec)*time.Second)
	if err != nil {
		return -1
	}
	return lat
}

// FetchSubscription loads and parses a subscription URL, returning server list as JSON
func FetchSubscription(subURL string) (string, error) {
	servers, err := config.FetchSubscription(subURL)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(servers)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ParseShadowStarURI parses a shadowstar:// URI into a ServerProfile JSON
func ParseShadowStarURI(uri string) (string, error) {
	prof, err := config.DecodeURIToProfile(strings.TrimSpace(uri))
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(prof)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GenerateShadowStarURI encodes a ServerProfile JSON into a shadowstar:// URI
func GenerateShadowStarURI(profileJSON string) (string, error) {
	var prof client.ServerProfile
	if err := json.Unmarshal([]byte(profileJSON), &prof); err != nil {
		return "", err
	}
	return config.EncodeProfileToURI(prof), nil
}

// FindFastestServer accepts a JSON array of ServerProfiles, concurrently pings all of them,
// and returns the index of the server with the lowest latency.
func FindFastestServer(serversJSON string) int {
	var servers []client.ServerProfile
	if err := json.Unmarshal([]byte(serversJSON), &servers); err != nil || len(servers) == 0 {
		return -1
	}

	config.PingAllConcurrently(servers, 3*time.Second)

	bestIdx := -1
	minPing := 999999
	for i, s := range servers {
		if s.Ping > 0 && s.Ping < minPing {
			minPing = s.Ping
			bestIdx = i
		}
	}
	return bestIdx
}

// CheckLocalPortAvailable checks if the local SOCKS5 port is ready to accept traffic
func CheckLocalPortAvailable(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
