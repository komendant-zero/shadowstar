package routing

import (
	"testing"
)

func TestRuleMatching_Modes(t *testing.T) {
	// Global mode: everything is ActionProxy
	cfgGlobal := &Config{
		Mode:          "global",
		DefaultAction: ActionDirect,
	}
	engGlobal, err := NewEngine(cfgGlobal)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	if act := engGlobal.Match("yandex.ru"); act != ActionProxy {
		t.Errorf("global mode expected ActionProxy, got %v", act)
	}
	if act := engGlobal.Match("77.88.55.66"); act != ActionProxy {
		t.Errorf("global mode expected ActionProxy, got %v", act)
	}

	// Direct mode: everything is ActionDirect
	cfgDirect := &Config{
		Mode:          "direct",
		DefaultAction: ActionProxy,
	}
	engDirect, err := NewEngine(cfgDirect)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	if act := engDirect.Match("google.com"); act != ActionDirect {
		t.Errorf("direct mode expected ActionDirect, got %v", act)
	}
}

func TestRuleMatching_Loopback(t *testing.T) {
	eng, err := NewEngine(DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	targets := []string{
		"localhost",
		"localhost:8080",
		"127.0.0.1",
		"127.0.0.1:10809",
		"127.0.0.53:53",
		"[::1]:443",
	}
	for _, target := range targets {
		if act := eng.Match(target); act != ActionDirect {
			t.Errorf("expected loopback %q to be ActionDirect, got %v", target, act)
		}
	}
}

func TestRuleMatching_WhitelistBlacklist(t *testing.T) {
	cfg := &Config{
		Mode:          "rule",
		DefaultAction: ActionProxy,
		Whitelist:     []string{"bypass.me", ".direct.org", "*allowed*"},
		Blacklist:     []string{"block.me", ".forceproxy.com"},
		Rules: []Rule{
			{
				Action:  ActionDirect,
				Domains: []string{"block.me"}, // Blacklist takes precedence
			},
		},
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Whitelist tests
	if act := eng.Match("bypass.me"); act != ActionDirect {
		t.Errorf("expected bypass.me to be ActionDirect, got %v", act)
	}
	if act := eng.Match("sub.direct.org:443"); act != ActionDirect {
		t.Errorf("expected sub.direct.org to be ActionDirect, got %v", act)
	}
	if act := eng.Match("test-allowed-domain.net"); act != ActionDirect {
		t.Errorf("expected test-allowed-domain.net to be ActionDirect, got %v", act)
	}

	// Blacklist tests
	if act := eng.Match("block.me"); act != ActionProxy {
		t.Errorf("expected block.me to be ActionProxy, got %v", act)
	}
	if act := eng.Match("api.forceproxy.com"); act != ActionProxy {
		t.Errorf("expected api.forceproxy.com to be ActionProxy, got %v", act)
	}
}

func TestRuleMatching_DomainTypes(t *testing.T) {
	cfg := &Config{
		Mode:          "rule",
		DefaultAction: ActionProxy,
		Rules: []Rule{
			{
				Action:        ActionBlock,
				Domains:       []string{"ads.example.com"},
				DomainSuffix:  []string{".tracking.com", "malware.ru"},
				DomainKeyword: []string{"telemetry"},
			},
			{
				Action:       ActionDirect,
				DomainSuffix: []string{"geosite:category-ru"},
			},
		},
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	tests := []struct {
		dest   string
		expect Action
	}{
		{"ads.example.com", ActionBlock},
		{"sub.tracking.com:443", ActionBlock},
		{"evil.malware.ru", ActionBlock},
		{"my-telemetry-server.com", ActionBlock},
		{"music.yandex.ru", ActionDirect},
		{"gosuslugi.ru:443", ActionDirect},
		{"unknown-foreign-service.com", ActionProxy},
	}

	for _, tt := range tests {
		got := eng.Match(tt.dest)
		if got != tt.expect {
			t.Errorf("dest %q: expected %v, got %v", tt.dest, tt.expect, got)
		}
	}
}

func TestRuleMatching_IPRanges(t *testing.T) {
	cfg := &Config{
		Mode:          "rule",
		DefaultAction: ActionProxy,
		Rules: []Rule{
			{
				Action:   ActionDirect,
				IPRanges: []string{"geoip:private", "geoip:ru"},
			},
		},
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	tests := []struct {
		ip     string
		expect Action
	}{
		// Private / LAN IPs
		{"10.0.0.1", ActionDirect},
		{"192.168.1.1:80", ActionDirect},
		{"172.16.5.10", ActionDirect},
		{"100.64.0.1", ActionDirect},
		{"198.18.0.5", ActionDirect},
		{"[::1]:80", ActionDirect},

		// Russian autonomous systems
		{"77.88.55.66:443", ActionDirect},  // Yandex
		{"95.163.0.1:80", ActionDirect},   // VK
		{"212.188.0.1", ActionDirect},    // MTS

		// Foreign public IPs
		{"1.1.1.1:53", ActionProxy},
		{"8.8.8.8", ActionProxy},
		{"104.16.12.34:443", ActionProxy},
	}

	for _, tt := range tests {
		got := eng.Match(tt.ip)
		if got != tt.expect {
			t.Errorf("ip %q: expected %v, got %v", tt.ip, tt.expect, got)
		}
	}
}

func BenchmarkEngine_Match(b *testing.B) {
	eng, _ := NewEngine(DefaultConfig())
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = eng.Match("music.yandex.ru:443")
		_ = eng.Match("77.88.55.66:443")
		_ = eng.Match("www.google.com:443")
		_ = eng.Match("1.1.1.1:53")
	}
}
