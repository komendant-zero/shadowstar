package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shadowstar/shadowstar/pkg/client"
)

// EncodeProfileToURI converts a ServerProfile into a shareable shadowstar:// link
func EncodeProfileToURI(p client.ServerProfile) string {
	payload, _ := json.Marshal(p)
	b64 := base64.RawURLEncoding.EncodeToString(payload)
	return fmt.Sprintf("shadowstar://%s#%s", b64, url.QueryEscape(p.Name))
}

// DecodeURIToProfile parses a shadowstar:// URI back into a ServerProfile
func DecodeURIToProfile(rawURI string) (*client.ServerProfile, error) {
	rawURI = strings.TrimSpace(rawURI)
	if !strings.HasPrefix(rawURI, "shadowstar://") {
		return nil, errors.New("invalid protocol scheme: expected shadowstar://")
	}

	body := strings.TrimPrefix(rawURI, "shadowstar://")
	var name string
	if idx := strings.Index(body, "#"); idx != -1 {
		name, _ = url.QueryUnescape(body[idx+1:])
		body = body[:idx]
	}

	// Try JSON base64
	data, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		data, err = base64.StdEncoding.DecodeString(body)
	}

	if err == nil {
		var profile client.ServerProfile
		if err := json.Unmarshal(data, &profile); err == nil {
			if name != "" && profile.Name == "" {
				profile.Name = name
			}
			if profile.Country == "" {
				country, _ := DetectCountry(profile.Name, profile.ServerAddr)
				profile.Country = country
			}
			return &profile, nil
		}
	}

	// Fallback to URL-like query string: user:pass@host:port?sni=...
	u, err := url.Parse(rawURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	serverAddr := u.Host
	var masterKey string
	if u.User != nil {
		masterKey = u.User.Username()
		if pass, ok := u.User.Password(); ok && masterKey == "" {
			masterKey = pass
		}
	}
	sni := u.Query().Get("sni")
	if sni == "" {
		sni = "www.microsoft.com"
	}
	chunker := u.Query().Get("chunker") == "1" || u.Query().Get("chunker") == "true"
	padding, _ := strconv.Atoi(u.Query().Get("padding"))
	if padding == 0 {
		padding = 64
	}

	return &client.ServerProfile{
		ID:            fmt.Sprintf("node-%d", os.Getpid()),
		Name:          name,
		ServerAddr:    serverAddr,
		MasterKey:     masterKey,
		SNI:           sni,
		EnableChunker: chunker,
		PaddingLen:    padding,
	}, nil
}

// LoadClientConfig reads client config from a JSON file
func LoadClientConfig(path string) (*client.Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Default config
		def := &client.Config{
			SocksAddr: "127.0.0.1:10808",
			HttpAddr:  "127.0.0.1:10809",
			ActiveID:  "default-server",
			Servers: []client.ServerProfile{
				{
					ID:            "default-server",
					Name:          "Default Node",
					ServerAddr:    "127.0.0.1:8443",
					MasterKey:     "ShadowStarSuperSecretKey2026!Master",
					SNI:           "www.microsoft.com",
					EnableChunker: true,
					PaddingLen:    64,
				},
			},
		}
		_ = SaveClientConfig(path, def)
		return def, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg client.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveClientConfig writes client config to a JSON file
func SaveClientConfig(path string, cfg *client.Config) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
