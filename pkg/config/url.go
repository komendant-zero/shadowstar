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
	cfg.EnsureGroups()
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
