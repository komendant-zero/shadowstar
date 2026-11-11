package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
	"github.com/shadowstar/shadowstar/pkg/gui"
	"github.com/shadowstar/shadowstar/pkg/sysproxy"
	"github.com/shadowstar/shadowstar/pkg/tui"
	"github.com/shadowstar/shadowstar/pkg/updater"
)

var (
	version = "1.4.2-stealth"
)

func main() {
	updater.CleanupOldFiles()

	configPath := flag.String("c", "shadowstar-client.json", "Path to client config file")
	_ = flag.Bool("tui", true, "Launch interactive Terminal User Interface (default)")
	headless := flag.Bool("headless", false, "Run in background/daemon CLI mode without interactive TUI")
	useWeb := flag.Bool("web", false, "Launch optional web browser GUI")
	connect := flag.Bool("connect", false, "Auto-connect to active server on startup")
	showVersion := flag.Bool("version", false, "Print version")
	importURI := flag.String("import", "", "Import a shadowstar:// link into config")
	subURL := flag.String("sub", "", "Set subscription URL and update servers")
	updateSub := flag.Bool("update-sub", false, "Update servers from subscription URL")
	listServers := flag.Bool("list", false, "List configured servers")
	keyFlag := flag.String("key", "", "Master key to connect or override active node")
	serverFlag := flag.String("server", "", "Remote server address (host:port)")
	doUpdate := flag.Bool("update", false, "Check for updates and apply latest release from GitHub")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ShadowStar Client v%s (Anti-DPI Protocol)\n", version)
		return
	}

	if *doUpdate {
		fmt.Printf("◈ ShadowStar Client v%s — Checking for updates on GitHub...\n", version)
		rel, err := updater.CheckUpdate("")
		if err != nil {
			log.Fatalf("[Error] Failed to query GitHub releases: %v\n", err)
		}
		if !rel.IsNewer {
			fmt.Printf("[✓] You are already using the latest version (%s)!\n", version)
			return
		}

		fmt.Printf("[★] New version available: %s (Released: %s)\n", rel.TagName, rel.PublishedAt)
		if rel.Name != "" {
			fmt.Printf("    Title: %s\n", rel.Name)
		}
		fmt.Printf("    Release Page: %s\n", rel.HTMLURL)
		fmt.Printf("    Download Asset: %s\n", rel.DownloadURL)
		fmt.Println("[*] Downloading and installing update...")
		err = updater.DownloadAndApplyUpdate(rel.DownloadURL, func(pct int, status string) {
			barLen := 30
			filled := (pct * barLen) / 100
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)
			fmt.Printf("\r    [%s] %3d%% — %s", bar, pct, status)
		})
		if err != nil {
			log.Fatalf("\n[Error] Update failed: %v\n", err)
		}
		fmt.Println("\n[✓] Update applied successfully! Restarting ShadowStar...")
		return
	}

	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	// Smart config resolution: prioritize user's personal config if present
	absCfg := *configPath
	if *configPath == "shadowstar-client.json" {
		candidates := []string{
			filepath.Join(exeDir, "shadowstar-client.personal.json"),
			"shadowstar-client.personal.json",
			filepath.Join(exeDir, "shadowstar-client.json"),
			"shadowstar-client.json",
			filepath.Join(os.Getenv("APPDATA"), "ShadowStar", "shadowstar-client.json"),
		}
		for _, cand := range candidates {
			if cand == "" {
				continue
			}
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				absCfg = cand
				break
			}
		}
	} else if !filepath.IsAbs(absCfg) {
		if _, err := os.Stat(absCfg); os.IsNotExist(err) && exeDir != "" {
			exeDirCfg := filepath.Join(exeDir, absCfg)
			if _, err := os.Stat(exeDirCfg); err == nil {
				absCfg = exeDirCfg
			}
		}
	}
	absCfg, _ = filepath.Abs(absCfg)

	cfg, err := config.LoadClientConfig(absCfg)
	if err != nil {
		cfg = &client.Config{
			SocksAddr: "127.0.0.1:10808",
			HttpAddr:  "127.0.0.1:10809",
			Servers:   []client.ServerProfile{},
		}
	}

	// Fallback to personal config if active config has no real servers
	hasRealServers := false
	for _, s := range cfg.Servers {
		if s.ServerAddr != "" && s.ServerAddr != "your-server-ip.com:4433" && s.ServerAddr != "127.0.0.1:8443" {
			hasRealServers = true
			break
		}
	}
	if !hasRealServers {
		personalCandidates := []string{
			filepath.Join(exeDir, "shadowstar-client.personal.json"),
			"shadowstar-client.personal.json",
		}
		for _, pCand := range personalCandidates {
			if fi, err := os.Stat(pCand); err == nil && !fi.IsDir() {
				if pCfg, pErr := config.LoadClientConfig(pCand); pErr == nil && len(pCfg.Servers) > 0 {
					for _, ps := range pCfg.Servers {
						if ps.ServerAddr != "" && ps.ServerAddr != "your-server-ip.com:4433" && ps.ServerAddr != "127.0.0.1:8443" {
							cfg = pCfg
							absCfg, _ = filepath.Abs(pCand)
							hasRealServers = true
							break
						}
					}
					if hasRealServers {
						break
					}
				}
			}
		}
	}

	if *subURL != "" {
		cfg.SubscriptionURL = *subURL
		fmt.Printf("[Subscription] Fetching servers from %s ...\n", *subURL)
		nodes, err := config.FetchSubscription(*subURL)
		if err != nil {
			log.Fatalf("Failed to fetch subscription: %v", err)
		}
		cfg.Servers = append(cfg.Servers, nodes...)
		if len(cfg.Servers) > 0 {
			cfg.ActiveID = cfg.Servers[0].ID
		}
		_ = config.SaveClientConfig(absCfg, cfg)
		fmt.Printf("[Subscription] Successfully imported %d nodes! Active node: %s\n", len(nodes), cfg.ActiveID)
		return
	}

	if *updateSub {
		if cfg.SubscriptionURL == "" {
			log.Fatalf("No subscription URL configured in %s", absCfg)
		}
		fmt.Printf("[Subscription] Updating from %s ...\n", cfg.SubscriptionURL)
		nodes, err := config.FetchSubscription(cfg.SubscriptionURL)
		if err != nil {
			log.Fatalf("Subscription update failed: %v", err)
		}
		cfg.Servers = nodes
		if len(cfg.Servers) > 0 {
			cfg.ActiveID = cfg.Servers[0].ID
		}
		_ = config.SaveClientConfig(absCfg, cfg)
		fmt.Printf("[Subscription] Successfully updated %d nodes!\n", len(nodes))
		return
	}

	if *listServers {
		fmt.Printf("ShadowStar Servers (%d total):\n", len(cfg.Servers))
		for i, s := range cfg.Servers {
			active := " "
			if s.ID == cfg.ActiveID {
				active = "*"
			}
			fmt.Printf(" [%s] %d. %s (%s) - SNI: %s [Country: %s]\n", active, i+1, s.Name, s.ServerAddr, s.SNI, s.Country)
		}
		return
	}

	if *importURI != "" {
		profile, err := config.DecodeURIToProfile(*importURI)
		if err != nil {
			log.Fatalf("Failed to import link: %v", err)
		}
		cfg.Servers = append(cfg.Servers, *profile)
		cfg.ActiveID = profile.ID
		_ = config.SaveClientConfig(absCfg, cfg)
		fmt.Printf("Successfully imported profile: %s (%s)\n", profile.Name, profile.ServerAddr)
		return
	}

	if *keyFlag != "" || *serverFlag != "" {
		if len(cfg.Servers) == 0 {
			sAddr := *serverFlag
			if sAddr == "" {
				sAddr = "179.253.74.36:4433"
			}
			cfg.Servers = append(cfg.Servers, client.ServerProfile{
				ID:            "cmd-node",
				Name:          "Direct Node",
				ServerAddr:    sAddr,
				MasterKey:     *keyFlag,
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
			})
			cfg.ActiveID = "cmd-node"
		} else {
			for i := range cfg.Servers {
				if cfg.Servers[i].ID == cfg.ActiveID || len(cfg.Servers) == 1 {
					if *keyFlag != "" {
						cfg.Servers[i].MasterKey = *keyFlag
					}
					if *serverFlag != "" {
						cfg.Servers[i].ServerAddr = *serverFlag
					}
					break
				}
			}
		}
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
		log.Fatalf("Failed to initialize client: %v", err)
	}

	if *headless {
		fmt.Println("=====================================================")
		fmt.Printf("  ⭐ SHADOWSTAR CLIENT v%s (Headless Mode)\n", version)
		fmt.Printf("  📡 SOCKS5: %s | HTTP: %s\n", cfg.SocksAddr, cfg.HttpAddr)
		fmt.Printf("  🌐 Active Server: %s (%s)\n", activeProfile.Name, activeProfile.ServerAddr)
		fmt.Println("=====================================================")

		if *connect {
			if err := c.Start(); err != nil {
				log.Printf("[Warning] Auto-connect failed: %v", err)
			} else {
				log.Printf("[Client] Connected successfully!")
			}
		}

		// Trap SIGINT / SIGTERM for clean shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		fmt.Println("\n[ShadowStar] Gracefully shutting down...")
		_ = sysproxy.ClearSystemProxy()
		_ = c.Stop()
		return
	}

	if *useWeb {
		// Launch optional web browser GUI
		srv, err := gui.NewGUIServer(absCfg)
		if err != nil {
			log.Fatalf("Failed to initialize ShadowStar GUI: %v", err)
		}

		if err := srv.Start(); err != nil {
			log.Fatalf("Failed to start GUI server: %v", err)
		}

		if *connect {
			_ = srv.AutoConnect()
		}

		cmd := srv.LaunchWindow()
		srv.WaitForExit(cmd)
		return
	}

	// Default: Launch ultra-lightweight, sleek, cross-platform interactive Terminal UI (TUI)
	app := tui.New(absCfg, c, cfg)
	if err := app.Run(*connect); err != nil {
		log.Fatalf("Failed to launch TUI: %v", err)
	}
}
