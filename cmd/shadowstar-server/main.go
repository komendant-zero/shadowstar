package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
	"github.com/shadowstar/shadowstar/pkg/server"
)

var (
	version = "1.2.0-stealth"
)

func main() {
	configPath := flag.String("c", "", "Path to server config JSON")
	listenAddr := flag.String("listen", ":443", "Server listen address")
	masterKey := flag.String("key", "", "Master secret key (32 bytes)")
	fallback := flag.String("fallback", "www.microsoft.com:443", "Decoy target for active probing defense")
	sni := flag.String("sni", "www.microsoft.com", "Camouflaged SNI domain")
	verbose := flag.Bool("v", false, "Enable verbose logging")
	genConfig := flag.Bool("gen-config", false, "Generate new secure credentials and connection link")
	serverIP := flag.String("ip", "YOUR_SERVER_IP", "Public IP for link generation")
	showVersion := flag.Bool("version", false, "Print version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ShadowStar Server v%s (Anti-DPI Daemon)\n", version)
		return
	}

	if *genConfig {
		generateCredentials(*listenAddr, *fallback, *sni, *serverIP)
		return
	}

	var cfg server.Config
	if *configPath != "" {
		data, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatalf("Failed to read config file %s: %v", *configPath, err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Fatalf("Failed to parse config file: %v", err)
		}
	} else {
		if *masterKey == "" {
			// Generate random key if none specified
			k := make([]byte, 32)
			_, _ = rand.Read(k)
			*masterKey = hex.EncodeToString(k)
			log.Printf("[Notice] No master key provided. Generated ephemeral key: %s", *masterKey)
		}
		cfg = server.Config{
			ListenAddr:     *listenAddr,
			MasterKey:      *masterKey,
			FallbackTarget: *fallback,
			SNI:            *sni,
			Verbose:        *verbose,
		}
	}

	srv, err := server.NewServer(&cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	fmt.Println("=====================================================")
	fmt.Printf("  ⭐ SHADOWSTAR SERVER v%s (Anti-DPI Protocol)\n", version)
	fmt.Printf("  🛡️  Listen:    %s\n", cfg.ListenAddr)
	fmt.Printf("  🎭 Masquerade: SNI: %s | Decoy: %s\n", cfg.SNI, cfg.FallbackTarget)
	fmt.Println("=====================================================")

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[ShadowStar-Server] Shutting down...")
	_ = srv.Close()
}

func generateCredentials(listenAddr, fallback, sni, serverIP string) {
	keyBytes := make([]byte, 32)
	_, _ = rand.Read(keyBytes)
	keyHex := hex.EncodeToString(keyBytes)

	port := "443"
	if len(listenAddr) > 1 && listenAddr[0] == ':' {
		port = listenAddr[1:]
	}

	serverCfg := server.Config{
		ListenAddr:     listenAddr,
		MasterKey:      keyHex,
		FallbackTarget: fallback,
		SNI:            sni,
		Verbose:        false,
	}

	clientProfile := client.ServerProfile{
		ID:            "shadowstar-node-1",
		Name:          "ShadowStar Node",
		ServerAddr:    fmt.Sprintf("%s:%s", serverIP, port),
		MasterKey:     keyHex,
		SNI:           sni,
		EnableChunker: true,
		PaddingLen:    64,
	}

	uri := config.EncodeProfileToURI(clientProfile)

	serverJSON, _ := json.MarshalIndent(serverCfg, "", "  ")
	clientJSON, _ := json.MarshalIndent(clientProfile, "", "  ")

	fmt.Println("\n=======================================================")
	fmt.Println("   ⭐ SHADOWSTAR SECURE CREDENTIALS GENERATED")
	fmt.Println("=======================================================")
	fmt.Printf("\n--- Server Config (/etc/shadowstar/config.json) ---\n%s\n", string(serverJSON))
	fmt.Printf("\n--- Client Profile ---\n%s\n", string(clientJSON))
	fmt.Printf("\n--- Shareable Connection URI ---\n%s\n", uri)
	fmt.Println("\n=======================================================")
}
