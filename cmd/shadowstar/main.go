package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
	"github.com/shadowstar/shadowstar/pkg/gui"
	"github.com/shadowstar/shadowstar/pkg/nativegui"
	"github.com/shadowstar/shadowstar/pkg/sysproxy"
)

var (
	version = "1.3.0-stealth"
)

func main() {
	configPath := flag.String("c", "shadowstar-client.json", "Path to client config file")
	headless := flag.Bool("headless", false, "Run in headless CLI mode without opening GUI window")
	useWeb := flag.Bool("web", false, "Launch optional web browser GUI")
	connect := flag.Bool("connect", false, "Auto-connect on startup")
	showVersion := flag.Bool("version", false, "Print version")
	importURI := flag.String("import", "", "Import a shadowstar:// link into config")
	subURL := flag.String("sub", "", "Set subscription URL and update servers")
	updateSub := flag.Bool("update-sub", false, "Update servers from subscription URL")
	listServers := flag.Bool("list", false, "List configured servers")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ShadowStar Client v%s (Anti-DPI Protocol)\n", version)
		return
	}

	// Resolve absolute path for config
	absCfg := *configPath
	if !filepath.IsAbs(absCfg) {
		if _, err := os.Stat(absCfg); os.IsNotExist(err) {
			// Check next to executable
			if exe, err := os.Executable(); err == nil {
				exeDirCfg := filepath.Join(filepath.Dir(exe), *configPath)
				if _, err := os.Stat(exeDirCfg); err == nil {
					absCfg = exeDirCfg
				}
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

	// Default: Launch ultra-lightweight, sleek, native Windows desktop app (zero web browser)
	if err := nativegui.Run(absCfg, c, cfg, *connect); err != nil {
		log.Fatalf("Failed to launch GUI: %v", err)
	}
}
