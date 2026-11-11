package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
	"github.com/shadowstar/shadowstar/pkg/mobile"
	"github.com/shadowstar/shadowstar/pkg/updater"
)

var (
	apiHost     = flag.String("host", "127.0.0.1", "Host interface for local HTTP REST API")
	apiPort     = flag.Int("api", 10810, "Local HTTP REST API port for Mobile UI control")
	subURL      = flag.String("sub", "", "Subscription URL to load nodes on startup")
	connectNode = flag.String("connect", "", "Direct shadowstar:// link, host:port:key or key to connect immediately")
	socksAddr   = flag.String("socks", "127.0.0.1:10808", "Local SOCKS5 proxy listen address")
	httpAddr    = flag.String("http", "127.0.0.1:10809", "Local HTTP proxy listen address")
	cfgPath     = flag.String("c", "shadowstar-mobile.json", "Mobile configuration storage file")
)

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func main() {
	updater.CleanupOldFiles()
	flag.Parse()

	log.Printf("[ShadowStar-Mobile] Core daemon starting on port %d...", *apiPort)
	log.Printf("[ShadowStar-Mobile] SOCKS5: %s | HTTP: %s", *socksAddr, *httpAddr)

	// If direct connect URI or key passed on command line
	if *connectNode != "" {
		prof, err := config.DecodeURIToProfile(*connectNode)
		if err == nil {
			if prof.ServerAddr == "" {
				prof.ServerAddr = "179.253.74.36:4433"
				prof.Name = "Direct Key Node"
			}
			rawJSON, _ := json.Marshal(prof)
			_, startErr := mobile.StartVPN(string(rawJSON))
			if startErr != nil {
				log.Printf("[ShadowStar-Mobile] Auto-connect error: %v", startErr)
			} else {
				log.Printf("[ShadowStar-Mobile] Auto-connected to node: %s (%s)", prof.Name, prof.ServerAddr)
			}
		} else {
			log.Printf("[ShadowStar-Mobile] Failed to parse connect argument: %v", err)
		}
	} else if *cfgPath != "" {
		// Auto-load config.json and connect to active node
		if cfg, err := config.LoadClientConfig(*cfgPath); err == nil && cfg != nil && len(cfg.Servers) > 0 {
			var activeProf *client.ServerProfile
			for i := range cfg.Servers {
				if cfg.Servers[i].ID == cfg.ActiveID {
					activeProf = &cfg.Servers[i]
					break
				}
			}
			if activeProf == nil {
				activeProf = &cfg.Servers[0]
			}
			if activeProf.ServerAddr != "" && activeProf.MasterKey != "" {
				rawJSON, _ := json.Marshal(activeProf)
				_, startErr := mobile.StartVPN(string(rawJSON))
				if startErr != nil {
					log.Printf("[ShadowStar-Mobile] Config auto-connect error: %v", startErr)
				} else {
					log.Printf("[ShadowStar-Mobile] Auto-connected to active server from config: %s (%s)", activeProf.Name, activeProf.ServerAddr)
				}
			}
		}
	}

	// Register REST API handlers for Mobile App / PWA frontend
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/connect", handleConnect)
	http.HandleFunc("/api/disconnect", handleDisconnect)
	http.HandleFunc("/api/subscription", handleSubscription)
	http.HandleFunc("/api/parse-uri", handleParseURI)
	http.HandleFunc("/api/ping", handlePing)
	http.HandleFunc("/api/auto-best", handleAutoBest)
	http.HandleFunc("/api/check-update", handleCheckUpdate)
	http.HandleFunc("/api/apply-update", handleApplyUpdate)

	// Serve mobile web interface: check disk paths first (dev mode), fallback to embedded WebFS
	var webHandler http.Handler
	candidates := []string{"mobile/web", "../mobile/web", "web", "pkg/mobile/web", "../pkg/mobile/web"}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			webHandler = http.FileServer(http.Dir(p))
			log.Printf("[ShadowStar-Mobile] Serving mobile frontend from disk (%s)", p)
			break
		}
	}
	if webHandler == nil {
		webHandler = mobile.GetWebHandler()
		log.Printf("[ShadowStar-Mobile] Serving mobile frontend from embedded assets")
	}

	http.Handle("/", webHandler)

	var listener net.Listener
	var err error
	for trial := 0; trial < 5; trial++ {
		listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", *apiHost, *apiPort))
		if err == nil {
			break
		}
		log.Printf("[ShadowStar-Mobile] Port %d is busy, trying port %d...", *apiPort, *apiPort+1)
		*apiPort++
	}
	if err != nil {
		log.Fatalf("[ShadowStar-Mobile] Failed to bind to HTTP port: %v", err)
	}

	server := &http.Server{}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[ShadowStar-Mobile] API server error: %v", err)
		}
	}()

	log.Printf("[ShadowStar-Mobile] Ready! Listening on http://%s:%d", *apiHost, *apiPort)

	// Graceful shutdown on SIGINT/SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[ShadowStar-Mobile] Shutting down...")
	_ = mobile.StopVPN()
	_ = server.Close()
}

func jsonResponse(w http.ResponseWriter, r *http.Request, success bool, msg string, data any) {
	w.Header().Set("Content-Type", "application/json")
	origin := r.Header.Get("Origin")
	if origin == "" || strings.HasPrefix(origin, "http://127.0.0.1") || strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "https://localhost") {
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}
	_ = json.NewEncoder(w).Encode(APIResponse{
		Success: success,
		Message: msg,
		Data:    data,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	var stats mobile.MobileStats
	_ = json.Unmarshal([]byte(mobile.GetStatsJSON()), &stats)
	jsonResponse(w, r, true, "", stats)
}

func handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		jsonResponse(w, r, false, "Method not allowed", nil)
		return
	}

	var req struct {
		ProfileJSON string `json:"profile_json"`
		URI         string `json:"uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, r, false, "Invalid JSON payload", nil)
		return
	}

	profJSON := req.ProfileJSON
	if profJSON == "" && req.URI != "" {
		prof, err := config.DecodeURIToProfile(req.URI)
		if err != nil {
			jsonResponse(w, r, false, "Invalid link/key: "+err.Error(), nil)
			return
		}
		if prof.ServerAddr == "" {
			prof.ServerAddr = "179.253.74.36:4433"
			prof.Name = "Direct Key Node"
		}
		raw, _ := json.Marshal(prof)
		profJSON = string(raw)
	}

	res, err := mobile.StartVPN(profJSON)
	if err != nil {
		jsonResponse(w, r, false, "Connection failed: "+err.Error(), nil)
		return
	}

	jsonResponse(w, r, true, res, nil)
}

func handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	err := mobile.StopVPN()
	if err != nil {
		jsonResponse(w, r, false, err.Error(), nil)
		return
	}
	jsonResponse(w, r, true, "Disconnected", nil)
}

func handleSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	subURL := r.URL.Query().Get("url")
	if subURL == "" {
		jsonResponse(w, r, false, "Missing 'url' query parameter", nil)
		return
	}

	nodesJSON, err := mobile.FetchSubscription(subURL)
	if err != nil {
		jsonResponse(w, r, false, "Failed to fetch subscription: "+err.Error(), nil)
		return
	}

	var nodes []any
	_ = json.Unmarshal([]byte(nodesJSON), &nodes)
	jsonResponse(w, r, true, "", nodes)
}

func handleParseURI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	uri := r.URL.Query().Get("uri")
	if uri == "" {
		jsonResponse(w, r, false, "Missing 'uri' parameter", nil)
		return
	}

	prof, err := config.DecodeURIToProfile(uri)
	if err != nil {
		jsonResponse(w, r, false, err.Error(), nil)
		return
	}

	jsonResponse(w, r, true, "", prof)
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	addr := r.URL.Query().Get("addr")
	if addr == "" {
		jsonResponse(w, r, false, "Missing 'addr' parameter", nil)
		return
	}

	ms := mobile.PingServer(addr, 3)
	jsonResponse(w, r, true, "", map[string]any{"addr": addr, "ping": ms})
}

func handleAutoBest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	var req struct {
		Servers []client.ServerProfile `json:"servers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, r, false, "Invalid JSON", nil)
		return
	}

	rawJSON, _ := json.Marshal(req.Servers)
	bestIdx := mobile.FindFastestServer(string(rawJSON))
	jsonResponse(w, r, true, "", map[string]int{"best_index": bestIdx})
}

var (
	updateMu       sync.Mutex
	isUpdating     bool
	updateProgress int
	updateStatus   string
	updateErr      string
)

func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	rel, err := updater.CheckUpdate("")
	if err != nil {
		jsonResponse(w, r, false, "Failed to check update: "+err.Error(), nil)
		return
	}
	jsonResponse(w, r, true, "", map[string]any{
		"tag_name":        rel.TagName,
		"name":            rel.Name,
		"body":            rel.Body,
		"html_url":        rel.HTMLURL,
		"published_at":    rel.PublishedAt,
		"is_newer":        rel.IsNewer,
		"download_url":    rel.DownloadURL,
		"apk_url":         rel.APKURL,
		"current_version": updater.CurrentVersion,
	})
}

func handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	updateMu.Lock()
	defer updateMu.Unlock()

	if r.Method == http.MethodGet {
		jsonResponse(w, r, true, "", map[string]any{
			"is_updating": isUpdating,
			"progress":    updateProgress,
			"status":      updateStatus,
			"error":       updateErr,
		})
		return
	}

	if isUpdating {
		jsonResponse(w, r, true, "Update already in progress", map[string]any{
			"is_updating": true,
			"progress":    updateProgress,
			"status":      updateStatus,
		})
		return
	}

	var req struct {
		DownloadURL string `json:"download_url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	isUpdating = true
	updateProgress = 0
	updateStatus = "Starting update..."
	updateErr = ""

	go func(targetURL string) {
		err := updater.DownloadAndApplyUpdate(targetURL, func(pct int, status string) {
			updateMu.Lock()
			updateProgress = pct
			updateStatus = status
			updateMu.Unlock()
		})
		updateMu.Lock()
		isUpdating = false
		if err != nil {
			updateErr = err.Error()
			updateStatus = "Error: " + err.Error()
		} else {
			updateProgress = 100
			updateStatus = "Update applied successfully"
		}
		updateMu.Unlock()
	}(req.DownloadURL)

	jsonResponse(w, r, true, "Update initiated", map[string]any{
		"is_updating": true,
		"progress":    0,
		"status":      "Starting update...",
	})
}
