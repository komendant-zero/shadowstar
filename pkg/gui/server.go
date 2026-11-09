package gui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
	"github.com/shadowstar/shadowstar/pkg/sysproxy"
)

//go:embed assets/*
var assetsFS embed.FS

// GUIServer manages the local desktop Web/API interface
type GUIServer struct {
	client     *client.Client
	cfg        *client.Config
	configPath string
	mu         sync.Mutex
	listener   net.Listener
	httpServer *http.Server
	port       int
}

// NewGUIServer initializes a GUI management server
func NewGUIServer(cfgPath string) (*GUIServer, error) {
	cfg, err := config.LoadClientConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client config: %w", err)
	}

	var activeProfile client.ServerProfile
	for _, s := range cfg.Servers {
		if s.ID == cfg.ActiveID {
			activeProfile = s
			break
		}
	}
	if activeProfile.ID == "" && len(cfg.Servers) > 0 {
		activeProfile = cfg.Servers[0]
		cfg.ActiveID = activeProfile.ID
	}

	c, err := client.NewClient(activeProfile, cfg.SocksAddr, cfg.HttpAddr)
	if err != nil {
		return nil, err
	}

	return &GUIServer{
		client:     c,
		cfg:        cfg,
		configPath: cfgPath,
		port:       18080,
	}, nil
}

// Start begins serving the Web UI and API
func (g *GUIServer) Start() error {
	subFS, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	// API Endpoints
	mux.HandleFunc("/api/status", g.handleStatus)
	mux.HandleFunc("/api/connect", g.handleConnect)
	mux.HandleFunc("/api/disconnect", g.handleDisconnect)
	mux.HandleFunc("/api/sysproxy", g.handleSysProxy)
	mux.HandleFunc("/api/servers", g.handleServers)
	mux.HandleFunc("/api/servers/select", g.handleSelectServer)
	mux.HandleFunc("/api/servers/add", g.handleAddServer)
	mux.HandleFunc("/api/servers/update", g.handleUpdateServer)
	mux.HandleFunc("/api/servers/import", g.handleImportServer)
	mux.HandleFunc("/api/servers/delete", g.handleDeleteServer)
	mux.HandleFunc("/api/servers/ping", g.handlePingServer)
	mux.HandleFunc("/api/servers/export", g.handleExportServer)
	mux.HandleFunc("/api/logs", g.handleLogs)
	mux.HandleFunc("/api/shutdown", g.handleShutdown)

	addr := fmt.Sprintf("127.0.0.1:%d", g.port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		// try dynamic port
		l, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		g.port = l.Addr().(*net.TCPAddr).Port
	}
	g.listener = l

	g.httpServer = &http.Server{Handler: mux}
	log.Printf("[ShadowStar-GUI] Web UI active on http://127.0.0.1:%d", g.port)

	go func() {
		_ = g.httpServer.Serve(l)
	}()

	return nil
}

func isAWGActive() bool {
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range interfaces {
		lower := strings.ToLower(iface.Name)
		if strings.Contains(lower, "amnezia") || strings.Contains(lower, "wireguard") {
			addrs, err := iface.Addrs()
			if err == nil && len(addrs) > 0 {
				return true
			}
		}
	}
	return false
}

// AutoConnect triggers connection programmatically
func (g *GUIServer) AutoConnect() error {
	if err := g.client.Start(); err != nil {
		return err
	}
	// Never touch network routes or system proxy if AWG is active to prevent conflicts!
	if isAWGActive() {
		g.client.Log("[Coexist] AmneziaWG активен. Системный стек сети оставлен без изменений во избежание конфликта.")
		return nil
	}
	_ = sysproxy.SetSystemProxy(g.cfg.HttpAddr)
	return nil
}

// GetURL returns the UI address
func (g *GUIServer) GetURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", g.port)
}

// LaunchWindow opens the desktop application window and returns the process handle
func (g *GUIServer) LaunchWindow() *exec.Cmd {
	url := g.GetURL()
	time.Sleep(150 * time.Millisecond)

	if runtime.GOOS == "windows" {
		// Try Edge App Mode (Clean borderless desktop window)
		edgePath := `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`
		if _, err := os.Stat(edgePath); os.IsNotExist(err) {
			edgePath = `C:\Program Files\Microsoft\Edge\Application\msedge.exe`
		}

		if _, err := os.Stat(edgePath); err == nil {
			dataDir := filepath.Join(os.TempDir(), "shadowstar_app_profile")
			cmd := exec.Command(edgePath,
				fmt.Sprintf("--app=%s", url),
				"--window-size=1180,820",
				fmt.Sprintf("--user-data-dir=%s", dataDir),
				"--no-first-run",
				"--no-default-browser-check",
			)
			if err := cmd.Start(); err == nil {
				return cmd
			}
		}

		// Fallback: standard default browser
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		_ = cmd.Start()
		return cmd
	} else if runtime.GOOS == "darwin" {
		cmd := exec.Command("open", url)
		_ = cmd.Start()
		return cmd
	} else {
		cmd := exec.Command("xdg-open", url)
		_ = cmd.Start()
		return cmd
	}
}

// WaitForExit waits for the window to close or an OS interrupt, then cleans up
func (g *GUIServer) WaitForExit(cmd *exec.Cmd) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	if cmd != nil && cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-done:
			log.Println("[ShadowStar] App window closed.")
		case <-sigChan:
			log.Println("[ShadowStar] Interrupted signal received.")
		}
	} else {
		<-sigChan
	}

	_ = sysproxy.ClearSystemProxy()
	_ = g.client.Stop()
}

type StatusResponse struct {
	BytesUp          uint64  `json:"bytes_up"`
	BytesDown        uint64  `json:"bytes_down"`
	SpeedUp          float64 `json:"speed_up"`
	SpeedDown        float64 `json:"speed_down"`
	ActiveConns      int64   `json:"active_conns"`
	Connected        bool    `json:"connected"`
	ConnectedTime    int64   `json:"connected_time"`
	Latency          int     `json:"latency"`
	SysProxy         bool    `json:"sysproxy_enabled"`
	AWGActive        bool    `json:"awg_active"`
	ActiveServerName string  `json:"active_server_name"`
	ActiveServerAddr string  `json:"active_server_addr"`
	ActiveServerSNI  string  `json:"active_server_sni"`
	SocksAddr        string  `json:"socks_addr"`
	HttpAddr         string  `json:"http_addr"`
	Version          string  `json:"version"`
}

func (g *GUIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats := g.client.GetStats()
	active := g.client.GetProfile()

	resp := StatusResponse{
		BytesUp:          stats.BytesUp,
		BytesDown:        stats.BytesDown,
		SpeedUp:          stats.SpeedUp,
		SpeedDown:        stats.SpeedDown,
		ActiveConns:      stats.ActiveConns,
		Connected:        stats.Connected,
		ConnectedTime:    stats.ConnectedTime,
		Latency:          stats.Latency,
		SysProxy:         sysproxy.GetSystemProxyStatus(),
		AWGActive:        isAWGActive(),
		ActiveServerName: active.Name,
		ActiveServerAddr: active.ServerAddr,
		ActiveServerSNI:  active.SNI,
		SocksAddr:        g.cfg.SocksAddr,
		HttpAddr:         g.cfg.HttpAddr,
		Version:          "1.2.0-stealth",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (g *GUIServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	err := g.client.Start()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Never touch network routes or system proxy if AWG is active to prevent conflicts!
	if isAWGActive() {
		g.client.Log("[Coexist] AmneziaWG активен. Системный стек сети оставлен без изменений во избежание конфликта.")
		w.WriteHeader(http.StatusOK)
		return
	}

	_ = sysproxy.SetSystemProxy(g.cfg.HttpAddr)
	w.WriteHeader(http.StatusOK)
}

func (g *GUIServer) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = sysproxy.ClearSystemProxy()
	_ = g.client.Stop()
	w.WriteHeader(http.StatusOK)
}

func (g *GUIServer) handleShutdown(w http.ResponseWriter, r *http.Request) {
	_ = sysproxy.ClearSystemProxy()
	_ = g.client.Stop()
	w.WriteHeader(http.StatusOK)
	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}()
}

func (g *GUIServer) handleSysProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Enable {
		if isAWGActive() {
			g.client.Log("[Coexist] AmneziaWG активен. Установка системного прокси отклонена во избежание конфликта.")
			http.Error(w, "AmneziaWG активен: системный прокси недоступен", http.StatusConflict)
			return
		}
		_ = sysproxy.SetSystemProxy(g.cfg.HttpAddr)
	} else {
		_ = sysproxy.ClearSystemProxy()
	}
	w.WriteHeader(http.StatusOK)
}

func (g *GUIServer) handleServers(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(g.cfg)
}

func (g *GUIServer) handleSelectServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	for _, s := range g.cfg.Servers {
		if s.ID == req.ID {
			g.cfg.ActiveID = s.ID
			g.client.UpdateProfile(s)
			_ = config.SaveClientConfig(g.configPath, g.cfg)
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	http.Error(w, "Server not found", http.StatusNotFound)
}

func (g *GUIServer) handleAddServer(w http.ResponseWriter, r *http.Request) {
	var profile client.ServerProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	profile.ServerAddr = strings.TrimSpace(profile.ServerAddr)
	profile.MasterKey = strings.TrimSpace(profile.MasterKey)
	profile.SNI = strings.TrimSpace(profile.SNI)
	if strings.Contains(profile.SNI, ":") {
		idx := strings.Index(profile.SNI, ":")
		profile.SNI = profile.SNI[:idx]
	}
	if profile.SNI == "" {
		profile.SNI = "www.microsoft.com"
	}

	profile.ID = fmt.Sprintf("node-%d", time.Now().UnixNano())
	g.mu.Lock()
	g.cfg.Servers = append(g.cfg.Servers, profile)
	g.cfg.ActiveID = profile.ID
	g.client.UpdateProfile(profile)
	_ = config.SaveClientConfig(g.configPath, g.cfg)
	g.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (g *GUIServer) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var profile client.ServerProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	profile.ServerAddr = strings.TrimSpace(profile.ServerAddr)
	profile.MasterKey = strings.TrimSpace(profile.MasterKey)
	profile.SNI = strings.TrimSpace(profile.SNI)
	if strings.Contains(profile.SNI, ":") {
		idx := strings.Index(profile.SNI, ":")
		profile.SNI = profile.SNI[:idx]
	}
	if profile.SNI == "" {
		profile.SNI = "www.microsoft.com"
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	found := false
	for i, s := range g.cfg.Servers {
		if s.ID == profile.ID {
			g.cfg.Servers[i].Name = profile.Name
			g.cfg.Servers[i].ServerAddr = profile.ServerAddr
			if profile.MasterKey != "" {
				g.cfg.Servers[i].MasterKey = profile.MasterKey
			}
			g.cfg.Servers[i].SNI = profile.SNI
			g.cfg.Servers[i].EnableChunker = profile.EnableChunker
			found = true
			if g.cfg.ActiveID == profile.ID {
				g.client.UpdateProfile(g.cfg.Servers[i])
			}
			break
		}
	}
	if !found {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}
	_ = config.SaveClientConfig(g.configPath, g.cfg)
	w.WriteHeader(http.StatusOK)
}

func (g *GUIServer) handleImportServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	profile, err := config.DecodeURIToProfile(req.URI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	g.cfg.Servers = append(g.cfg.Servers, *profile)
	g.cfg.ActiveID = profile.ID
	g.client.UpdateProfile(*profile)
	_ = config.SaveClientConfig(g.configPath, g.cfg)
	g.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (g *GUIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	logs := g.client.GetLogs()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}

func (g *GUIServer) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	newServers := make([]client.ServerProfile, 0, len(g.cfg.Servers))
	for _, s := range g.cfg.Servers {
		if s.ID != req.ID {
			newServers = append(newServers, s)
		}
	}
	if len(newServers) == 0 {
		http.Error(w, "Нельзя удалить последний сервер", http.StatusBadRequest)
		return
	}
	g.cfg.Servers = newServers
	if g.cfg.ActiveID == req.ID {
		g.cfg.ActiveID = newServers[0].ID
		g.client.UpdateProfile(newServers[0])
	}
	_ = config.SaveClientConfig(g.configPath, g.cfg)
	w.WriteHeader(http.StatusOK)
}

func (g *GUIServer) handlePingServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	var targetAddr string
	for i := range g.cfg.Servers {
		if g.cfg.Servers[i].ID == req.ID {
			targetAddr = g.cfg.Servers[i].ServerAddr
			break
		}
	}
	g.mu.Unlock()

	if targetAddr == "" {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	t0 := time.Now()
	conn, err := net.DialTimeout("tcp", targetAddr, 3*time.Second)
	latency := -1
	if err == nil {
		latency = int(time.Since(t0).Milliseconds())
		_ = conn.Close()
	}

	g.mu.Lock()
	for i := range g.cfg.Servers {
		if g.cfg.Servers[i].ID == req.ID {
			g.cfg.Servers[i].Ping = latency
			break
		}
	}
	g.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ping": latency})
}

func (g *GUIServer) handleExportServer(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, s := range g.cfg.Servers {
		if s.ID == id {
			uri := config.EncodeProfileToURI(s)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"uri": uri})
			return
		}
	}
	http.Error(w, "Server not found", http.StatusNotFound)
}
