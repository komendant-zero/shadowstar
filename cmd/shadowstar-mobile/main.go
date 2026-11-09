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
	"syscall"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
	"github.com/shadowstar/shadowstar/pkg/mobile"
)

var (
	apiPort     = flag.Int("api", 10810, "Local HTTP REST API port for Mobile UI control")
	subURL      = flag.String("sub", "", "Subscription URL to load nodes on startup")
	connectNode = flag.String("connect", "", "Direct shadowstar:// link or node address to connect immediately")
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
	flag.Parse()

	log.Printf("[ShadowStar-Mobile] Core daemon starting on port %d...", *apiPort)
	log.Printf("[ShadowStar-Mobile] SOCKS5: %s | HTTP: %s", *socksAddr, *httpAddr)

	// If direct connect URI passed on command line
	if *connectNode != "" {
		uri := *connectNode
		if !strings.HasPrefix(uri, "shadowstar://") {
			uri = "shadowstar://" + uri
		}
		profJSON, err := mobile.ParseShadowStarURI(uri)
		if err == nil {
			_, startErr := mobile.StartVPN(profJSON)
			if startErr != nil {
				log.Printf("[ShadowStar-Mobile] Auto-connect error: %v", startErr)
			} else {
				log.Printf("[ShadowStar-Mobile] Auto-connected to URI node")
			}
		} else {
			log.Printf("[ShadowStar-Mobile] Failed to parse connect URI: %v", err)
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
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", *apiPort))
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

	log.Printf("[ShadowStar-Mobile] Ready! Open: http://localhost:%d", *apiPort)

	// Graceful shutdown on SIGINT/SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[ShadowStar-Mobile] Shutting down...")
	_ = mobile.StopVPN()
	_ = server.Close()
}

func jsonResponse(w http.ResponseWriter, success bool, msg string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
	jsonResponse(w, true, "", stats)
}

func handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		jsonResponse(w, false, "Method not allowed", nil)
		return
	}

	var req struct {
		ProfileJSON string `json:"profile_json"`
		URI         string `json:"uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, false, "Invalid JSON payload", nil)
		return
	}

	profJSON := req.ProfileJSON
	if profJSON == "" && req.URI != "" {
		p, err := mobile.ParseShadowStarURI(req.URI)
		if err != nil {
			jsonResponse(w, false, "Invalid shadowstar:// URI: "+err.Error(), nil)
			return
		}
		profJSON = p
	}

	res, err := mobile.StartVPN(profJSON)
	if err != nil {
		jsonResponse(w, false, "Connection failed: "+err.Error(), nil)
		return
	}

	jsonResponse(w, true, res, nil)
}

func handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	err := mobile.StopVPN()
	if err != nil {
		jsonResponse(w, false, err.Error(), nil)
		return
	}
	jsonResponse(w, true, "Disconnected", nil)
}

func handleSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	subURL := r.URL.Query().Get("url")
	if subURL == "" {
		jsonResponse(w, false, "Missing 'url' query parameter", nil)
		return
	}

	nodesJSON, err := mobile.FetchSubscription(subURL)
	if err != nil {
		jsonResponse(w, false, "Failed to fetch subscription: "+err.Error(), nil)
		return
	}

	var nodes []any
	_ = json.Unmarshal([]byte(nodesJSON), &nodes)
	jsonResponse(w, true, "", nodes)
}

func handleParseURI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	uri := r.URL.Query().Get("uri")
	if uri == "" {
		jsonResponse(w, false, "Missing 'uri' parameter", nil)
		return
	}

	profJSON, err := mobile.ParseShadowStarURI(uri)
	if err != nil {
		jsonResponse(w, false, err.Error(), nil)
		return
	}

	var prof any
	_ = json.Unmarshal([]byte(profJSON), &prof)
	jsonResponse(w, true, "", prof)
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	addr := r.URL.Query().Get("addr")
	if addr == "" {
		jsonResponse(w, false, "Missing 'addr' parameter", nil)
		return
	}

	ms := mobile.PingServer(addr, 3)
	jsonResponse(w, true, "", map[string]any{"addr": addr, "ping": ms})
}

func handleAutoBest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	var req struct {
		Servers []client.ServerProfile `json:"servers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, false, "Invalid JSON", nil)
		return
	}

	rawJSON, _ := json.Marshal(req.Servers)
	bestIdx := mobile.FindFastestServer(string(rawJSON))
	jsonResponse(w, true, "", map[string]int{"best_index": bestIdx})
}
