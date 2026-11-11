package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shadowstar/shadowstar/pkg/client"
)

// EncodeProfileToURI converts a ServerProfile into a shareable shadowstar:// link
func EncodeProfileToURI(p client.ServerProfile) string {
	payload, _ := json.Marshal(p)
	b64 := base64.RawURLEncoding.EncodeToString(payload)
	return fmt.Sprintf("shadowstar://%s#%s", b64, url.QueryEscape(p.Name))
}

// DecodeURIToProfile parses a shadowstar:// URI, base64 payload, or connection string back into a ServerProfile
func DecodeURIToProfile(rawURI string) (*client.ServerProfile, error) {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return nil, errors.New("empty input")
	}

	body := rawURI
	if strings.HasPrefix(strings.ToLower(body), "shadowstar://") {
		body = body[len("shadowstar://"):]
	}

	var name string
	if idx := strings.Index(body, "#"); idx != -1 {
		name, _ = url.QueryUnescape(body[idx+1:])
		body = body[:idx]
	}

	// 1. Try JSON Base64 (Standard & URL-safe)
	cleanB64 := strings.ReplaceAll(strings.ReplaceAll(body, "-", "+"), "_", "/")
	for len(cleanB64)%4 != 0 {
		cleanB64 += "="
	}
	if data, err := base64.StdEncoding.DecodeString(cleanB64); err == nil && len(data) > 0 {
		var profile client.ServerProfile
		if err := json.Unmarshal(data, &profile); err == nil && (profile.ServerAddr != "" || profile.MasterKey != "") {
			if name != "" && profile.Name == "" {
				profile.Name = name
			}
			if profile.Country == "" {
				country, _ := DetectCountry(profile.Name, profile.ServerAddr)
				profile.Country = country
			}
			if profile.SNI == "" {
				profile.SNI = "www.microsoft.com"
			}
			if profile.PaddingLen == 0 {
				profile.PaddingLen = 64
			}
			return &profile, nil
		}
	}

	// 2. If input is just a raw key (hex string or passphrase, length >= 16, without host separators)
	cleanKey := strings.TrimSpace(body)
	if len(cleanKey) >= 16 && !strings.ContainsAny(cleanKey, ".:@/ ?") {
		return &client.ServerProfile{
			ID:            fmt.Sprintf("node-%d", time.Now().UnixNano()%100000),
			Name:          "Node with Key",
			ServerAddr:    "",
			MasterKey:     cleanKey,
			SNI:           "www.microsoft.com",
			EnableChunker: true,
			PaddingLen:    64,
		}, nil
	}

	// 3. Try URL query style: key@host:port?sni=... or shadowstar://key@host:port
	uriToParse := rawURI
	if !strings.Contains(uriToParse, "://") {
		uriToParse = "shadowstar://" + uriToParse
	}
	if u, err := url.Parse(uriToParse); err == nil && u.Host != "" && (strings.Contains(u.Host, ".") || strings.Contains(u.Host, ":")) {
		serverAddr := u.Host
		var masterKey string
		if u.User != nil {
			masterKey = u.User.Username()
			if pass, ok := u.User.Password(); ok && masterKey == "" {
				masterKey = pass
			}
		}
		if masterKey == "" && u.Query().Get("key") != "" {
			masterKey = u.Query().Get("key")
		}
		sni := u.Query().Get("sni")
		if sni == "" {
			sni = "www.microsoft.com"
		}
		chunkerVal := u.Query().Get("chunker")
		chunker := true
		if chunkerVal == "0" || chunkerVal == "false" {
			chunker = false
		}
		padding, _ := strconv.Atoi(u.Query().Get("padding"))
		if padding == 0 {
			padding = 64
		}

		if serverAddr != "" {
			return &client.ServerProfile{
				ID:            fmt.Sprintf("node-%d", time.Now().UnixNano()%100000),
				Name:          name,
				ServerAddr:    serverAddr,
				MasterKey:     masterKey,
				SNI:           sni,
				EnableChunker: chunker,
				PaddingLen:    padding,
			}, nil
		}
	}

	// 4. Try host:port:key[:sni]
	parts := strings.Split(body, ":")
	if len(parts) >= 3 {
		p := &client.ServerProfile{
			ID:            fmt.Sprintf("node-%d", time.Now().UnixNano()%100000),
			Name:          "Node " + parts[0],
			ServerAddr:    parts[0] + ":" + parts[1],
			MasterKey:     parts[2],
			SNI:           "www.microsoft.com",
			EnableChunker: true,
			PaddingLen:    64,
		}
		if len(parts) >= 4 && parts[3] != "" {
			p.SNI = parts[3]
		}
		if name != "" {
			p.Name = name
		}
		return p, nil
	}

	// 4. Try key@host:port
	if strings.Contains(body, "@") {
		atParts := strings.SplitN(body, "@", 2)
		p := &client.ServerProfile{
			ID:            fmt.Sprintf("node-%d", time.Now().UnixNano()%100000),
			Name:          "Node " + atParts[1],
			ServerAddr:    atParts[1],
			MasterKey:     atParts[0],
			SNI:           "www.microsoft.com",
			EnableChunker: true,
			PaddingLen:    64,
		}
		if name != "" {
			p.Name = name
		}
		return p, nil
	}

	return nil, errors.New("invalid shadowstar link or key format")
}

// GetPersistentConfigPath returns the OS-standard persistent configuration file path.
// This path resides in user profile directories and survives application reinstalls,
// binary updates, or deleting the local application directory.
func GetPersistentConfigPath() string {
	if custom := os.Getenv("SHADOWSTAR_CONFIG_DIR"); custom != "" {
		return filepath.Join(custom, "shadowstar-client.json")
	}
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			if home, err := os.UserHomeDir(); err == nil {
				appData = filepath.Join(home, "AppData", "Roaming")
			}
		}
		if appData != "" {
			return filepath.Join(appData, "ShadowStar", "shadowstar-client.json")
		}
	} else if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "ShadowStar", "shadowstar-client.json")
		}
	} else {
		// Linux / Android / other Unix
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "shadowstar", "shadowstar-client.json")
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".config", "shadowstar", "shadowstar-client.json")
		}
	}
	return "shadowstar-client.json"
}

// GetPersistentMobileConfigPath returns the persistent storage path for shadowstar-mobile.
func GetPersistentMobileConfigPath() string {
	p := GetPersistentConfigPath()
	dir := filepath.Dir(p)
	return filepath.Join(dir, "shadowstar-mobile.json")
}

// HasRealServers checks if the config contains at least one configured real server profile
func HasRealServers(cfg *client.Config) bool {
	if cfg == nil || len(cfg.Servers) == 0 {
		return false
	}
	for _, s := range cfg.Servers {
		addr := strings.TrimSpace(s.ServerAddr)
		if addr != "" && addr != "127.0.0.1:8443" && addr != "your-server-ip.com:4433" {
			return true
		}
	}
	return false
}

// LoadClientConfig reads client config from a JSON file, automatically restoring
// saved servers from OS persistent storage (%APPDATA% / ~/.config) if the local
// target file is missing, empty, or unconfigured (e.g. after reinstallation or updates).
func LoadClientConfig(path string) (*client.Config, error) {
	persistentPath := GetPersistentConfigPath()
	var loadedCfg *client.Config

	// 1. Check if the specified path exists
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		var cfg client.Config
		if json.Unmarshal(data, &cfg) == nil {
			loadedCfg = &cfg
		}
	}

	// 2. If path didn't have real servers, check persistent OS storage
	if !HasRealServers(loadedCfg) && persistentPath != "" {
		if pData, err := os.ReadFile(persistentPath); err == nil && len(pData) > 0 {
			var pCfg client.Config
			if json.Unmarshal(pData, &pCfg) == nil && HasRealServers(&pCfg) {
				if loadedCfg == nil {
					loadedCfg = &pCfg
				} else {
					// Restore all servers & groups from persistent storage!
					loadedCfg.Servers = pCfg.Servers
					loadedCfg.Groups = pCfg.Groups
					loadedCfg.ActiveID = pCfg.ActiveID
					loadedCfg.ActiveGroupID = pCfg.ActiveGroupID
					if pCfg.SubscriptionURL != "" {
						loadedCfg.SubscriptionURL = pCfg.SubscriptionURL
					}
				}
				// Save restored config back to path so local file is populated
				_ = SaveClientConfig(path, loadedCfg)
			}
		}
	}

	// 3. Fallback to default if still nothing
	if loadedCfg == nil {
		loadedCfg = &client.Config{
			SocksAddr: "127.0.0.1:10808",
			HttpAddr:  "127.0.0.1:10809",
			ActiveID:  "node-nl-vps",
			Servers: []client.ServerProfile{
				{
					ID:            "node-nl-vps",
					Name:          "ShadowStar VPS (Netherlands)",
					ServerAddr:    "179.253.74.36:4433",
					MasterKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					SNI:           "www.microsoft.com",
					EnableChunker: true,
					PaddingLen:    64,
				},
			},
		}
		_ = SaveClientConfig(path, loadedCfg)
	}

	loadedCfg.EnsureGroups()
	return loadedCfg, nil
}

// SaveClientConfig writes client config to a JSON file and always synchronizes
// a mirror copy to OS persistent storage to survive reinstalls.
func SaveClientConfig(path string, cfg *client.Config) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(path, data, 0644)

	// Always mirror to persistent storage location if HasRealServers
	persistentPath := GetPersistentConfigPath()
	if persistentPath != "" && filepath.Clean(path) != filepath.Clean(persistentPath) && HasRealServers(cfg) {
		pDir := filepath.Dir(persistentPath)
		_ = os.MkdirAll(pDir, 0755)
		_ = os.WriteFile(persistentPath, data, 0644)
	}

	return err
}
