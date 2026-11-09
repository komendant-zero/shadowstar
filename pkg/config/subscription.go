package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shadowstar/shadowstar/pkg/client"
)

// DetectCountry returns a 2-letter ISO country code or emoji based on server name/addr
func DetectCountry(name, addr string) (string, string) {
	upper := strings.ToUpper(name + " " + addr)
	switch {
	case strings.Contains(upper, "NETHERLAND") || strings.Contains(upper, "HOLLAND") || strings.Contains(upper, "AMSTERDAM") || strings.Contains(upper, "NL") || strings.Contains(upper, "НИДЕРЛАНД") || strings.Contains(upper, "АМСТЕРДАМ"):
		return "NL", "🇳🇱"
	case strings.Contains(upper, "GERMAN") || strings.Contains(upper, "FRANKFURT") || strings.Contains(upper, "BERLIN") || strings.Contains(upper, "DE") || strings.Contains(upper, "ГЕРМАНИ") || strings.Contains(upper, "ФРАНКФУРТ"):
		return "DE", "🇩🇪"
	case strings.Contains(upper, "UNITED STATES") || strings.Contains(upper, "USA") || strings.Contains(upper, "US") || strings.Contains(upper, "NEW YORK") || strings.Contains(upper, "MIAMI") || strings.Contains(upper, "США") || strings.Contains(upper, "АМЕРИК"):
		return "US", "🇺🇸"
	case strings.Contains(upper, "FINLAND") || strings.Contains(upper, "HELSINKI") || strings.Contains(upper, "FI") || strings.Contains(upper, "ФИНЛЯНД") || strings.Contains(upper, "ХЕЛЬСИНК"):
		return "FI", "🇫🇮"
	case strings.Contains(upper, "SWEDEN") || strings.Contains(upper, "STOCKHOLM") || strings.Contains(upper, "SE") || strings.Contains(upper, "ШВЕЦИ") || strings.Contains(upper, "СТОКГОЛЬМ"):
		return "SE", "🇸🇪"
	case strings.Contains(upper, "UNITED KINGDOM") || strings.Contains(upper, "LONDON") || strings.Contains(upper, "GB") || strings.Contains(upper, "UK") || strings.Contains(upper, "ЛОНДОН") || strings.Contains(upper, "БРИТАН"):
		return "GB", "🇬🇧"
	case strings.Contains(upper, "FRANCE") || strings.Contains(upper, "PARIS") || strings.Contains(upper, "FR") || strings.Contains(upper, "ФРАНЦИ") || strings.Contains(upper, "ПАРИЖ"):
		return "FR", "🇫🇷"
	case strings.Contains(upper, "TURKEY") || strings.Contains(upper, "ISTANBUL") || strings.Contains(upper, "TR") || strings.Contains(upper, "ТУРЦИ") || strings.Contains(upper, "СТАМБУЛ"):
		return "TR", "🇹🇷"
	case strings.Contains(upper, "KAZAKHSTAN") || strings.Contains(upper, "ALMATY") || strings.Contains(upper, "ASTANA") || strings.Contains(upper, "KZ") || strings.Contains(upper, "КАЗАХСТАН") || strings.Contains(upper, "АЛМАТ"):
		return "KZ", "🇰🇿"
	case strings.Contains(upper, "SINGAPORE") || strings.Contains(upper, "SG") || strings.Contains(upper, "СИНГАПУР"):
		return "SG", "🇸🇬"
	case strings.Contains(upper, "JAPAN") || strings.Contains(upper, "TOKYO") || strings.Contains(upper, "JP") || strings.Contains(upper, "ЯПОНИ") || strings.Contains(upper, "ТОКИО"):
		return "JP", "🇯🇵"
	case strings.Contains(upper, "POLAND") || strings.Contains(upper, "WARSAW") || strings.Contains(upper, "PL") || strings.Contains(upper, "ПОЛЬШ") || strings.Contains(upper, "ВАРШАВ"):
		return "PL", "🇵🇱"
	default:
		return "SS", "⭐"
	}
}

// ParseSubscriptionContent parses raw subscription text (JSON, Base64, or list of shadowstar:// URIs)
func ParseSubscriptionContent(content []byte) ([]client.ServerProfile, error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil, errors.New("empty subscription content")
	}

	// 1. Try JSON Array or Object
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		var list []client.ServerProfile
		if err := json.Unmarshal([]byte(trimmed), &list); err == nil && len(list) > 0 {
			enrichProfiles(list)
			return list, nil
		}
		var wrapper struct {
			Servers []client.ServerProfile `json:"servers"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrapper); err == nil && len(wrapper.Servers) > 0 {
			enrichProfiles(wrapper.Servers)
			return wrapper.Servers, nil
		}
	}

	// 2. Try Base64 decoding (standard v2ray/clash style)
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(trimmed)
	}
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(trimmed)
	}
	if err == nil && len(decoded) > 0 {
		trimmed = strings.TrimSpace(string(decoded))
		// Check if decoded is JSON
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			var list []client.ServerProfile
			if err := json.Unmarshal([]byte(trimmed), &list); err == nil && len(list) > 0 {
				enrichProfiles(list)
				return list, nil
			}
		}
	}

	// 3. Line-by-line parsing of shadowstar:// URIs and compact formats
	lines := strings.Split(trimmed, "\n")
	var profiles []client.ServerProfile
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";") {
			continue
		}
		// Try individual line base64 decode
		if !strings.HasPrefix(line, "shadowstar://") {
			if b, err := base64.StdEncoding.DecodeString(line); err == nil && len(b) > 0 && strings.HasPrefix(string(b), "shadowstar://") {
				line = strings.TrimSpace(string(b))
			} else if b, err := base64.RawURLEncoding.DecodeString(line); err == nil && len(b) > 0 && strings.HasPrefix(string(b), "shadowstar://") {
				line = strings.TrimSpace(string(b))
			}
		}

		if strings.HasPrefix(line, "shadowstar://") {
			prof, err := DecodeURIToProfile(line)
			if err == nil && prof != nil {
				profiles = append(profiles, *prof)
			}
		} else if strings.Contains(line, "@") {
			prof, err := DecodeURIToProfile("shadowstar://" + line)
			if err == nil && prof != nil {
				profiles = append(profiles, *prof)
			}
		} else if strings.Count(line, ":") >= 2 {
			// host:port:key
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				hostPort := parts[0] + ":" + parts[1]
				key := parts[2]
				prof := &client.ServerProfile{
					ID:         fmt.Sprintf("node-%d", time.Now().UnixNano()+int64(len(profiles))),
					Name:       "Node " + parts[0],
					ServerAddr: hostPort,
					MasterKey:  key,
					SNI:        "www.microsoft.com",
					EnableChunker: true,
					PaddingLen: 64,
				}
				profiles = append(profiles, *prof)
			}
		}
	}

	if len(profiles) == 0 {
		return nil, errors.New("no valid shadowstar:// servers found in subscription")
	}

	enrichProfiles(profiles)
	return profiles, nil
}

func enrichProfiles(list []client.ServerProfile) {
	for i := range list {
		if list[i].ID == "" {
			list[i].ID = fmt.Sprintf("sub-node-%d", time.Now().UnixNano()+int64(i))
		}
		if list[i].SNI == "" {
			list[i].SNI = "www.microsoft.com"
		}
		if list[i].PaddingLen == 0 {
			list[i].PaddingLen = 64
		}
		countryCode, _ := DetectCountry(list[i].Name, list[i].ServerAddr)
		if list[i].Country == "" {
			list[i].Country = countryCode
		}
	}
}

// FetchSubscription downloads and parses a subscription URL with timeout and fallback
func FetchSubscription(subURL string) ([]client.ServerProfile, error) {
	subURL = strings.TrimSpace(subURL)
	if subURL == "" {
		return nil, errors.New("empty subscription URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", subURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	req.Header.Set("User-Agent", "ShadowStar-Client/1.3 (Anti-DPI; Windows NT 10.0)")

	clientHttp := &http.Client{
		Timeout: 12 * time.Second,
	}

	resp, err := clientHttp.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subscription request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP %d %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2 MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return ParseSubscriptionContent(body)
}

// FetchAndApplyGroupSubscription updates servers for a specific group in Config
func FetchAndApplyGroupSubscription(cfg *client.Config, groupID string) (int, error) {
	cfg.EnsureGroups()
	var grp *client.SubscriptionGroup
	for i := range cfg.Groups {
		if cfg.Groups[i].ID == groupID {
			grp = &cfg.Groups[i]
			break
		}
	}
	if grp == nil {
		return 0, errors.New("group not found")
	}
	if grp.URL == "" {
		return 0, errors.New("group has no subscription URL configured")
	}

	nodes, err := FetchSubscription(grp.URL)
	if err != nil {
		return 0, err
	}
	if len(nodes) == 0 {
		return 0, errors.New("no nodes returned in subscription")
	}

	// Tag nodes with groupID
	for i := range nodes {
		nodes[i].GroupID = groupID
	}

	// Keep servers from other groups
	var kept []client.ServerProfile
	for _, s := range cfg.Servers {
		if s.GroupID != groupID {
			kept = append(kept, s)
		}
	}

	// Append newly fetched nodes for this group
	cfg.Servers = append(kept, nodes...)
	grp.UpdatedAt = time.Now().Unix()

	// If active server was in this group and is no longer present, activate the first new node
	activeFound := false
	for _, s := range cfg.Servers {
		if s.ID == cfg.ActiveID {
			activeFound = true
			break
		}
	}
	if !activeFound && len(nodes) > 0 {
		cfg.ActiveID = nodes[0].ID
	}

	return len(nodes), nil
}

// PingServer measures TCP round-trip latency in milliseconds
func PingServer(addr string, timeout time.Duration) (int, error) {
	t0 := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return -1, err
	}
	_ = conn.Close()
	return int(time.Since(t0).Milliseconds()), nil
}

// PingAllConcurrently pings all servers concurrently and returns a map of ID -> latency ms
func PingAllConcurrently(servers []client.ServerProfile, timeout time.Duration) map[string]int {
	results := make(map[string]int)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, s := range servers {
		wg.Add(1)
		go func(p client.ServerProfile) {
			defer wg.Done()
			ms, err := PingServer(p.ServerAddr, timeout)
			mu.Lock()
			if err != nil {
				results[p.ID] = -1
			} else {
				results[p.ID] = ms
			}
			mu.Unlock()
		}(s)
	}

	wg.Wait()
	return results
}
