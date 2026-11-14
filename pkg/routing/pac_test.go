package routing

import (
	"strings"
	"testing"
)

func TestGeneratePAC(t *testing.T) {
	cfg := &Config{
		Mode:      "rule",
		Whitelist: []string{"custom-direct.org"},
		Blacklist: []string{"custom-proxy.com"},
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	pacHTTP := eng.GeneratePAC("127.0.0.1:10809")
	if !strings.Contains(pacHTTP, "function FindProxyForURL(url, host)") {
		t.Errorf("expected PAC script to define FindProxyForURL")
	}
	if !strings.Contains(pacHTTP, "PROXY 127.0.0.1:10809; DIRECT") {
		t.Errorf("expected PAC script to contain proxy spec")
	}
	if !strings.Contains(pacHTTP, "custom-direct.org") {
		t.Errorf("expected PAC script to contain whitelist domain")
	}
	if !strings.Contains(pacHTTP, "custom-proxy.com") {
		t.Errorf("expected PAC script to contain blacklist domain")
	}

	pacSOCKS := eng.GeneratePAC("socks5://127.0.0.1:10808")
	if !strings.Contains(pacSOCKS, "SOCKS5 127.0.0.1:10808; SOCKS 127.0.0.1:10808; DIRECT") {
		t.Errorf("expected PAC script to format SOCKS5 proxy spec")
	}

	// Verify IPv6 bracketed literal parsing
	if !strings.Contains(pacHTTP, `host.charAt(0) === "["`) {
		t.Errorf("expected PAC script to parse bracketed IPv6 literals")
	}

	// Verify plain hostname and colon check
	if !strings.Contains(pacHTTP, `isPlainHostName(host) && host.indexOf(":") === -1`) {
		t.Errorf("expected PAC script to verify host does not contain colons before treating as plain hostname")
	}

	// Verify RegExp replaces unsupported shExpMatch character classes
	if !strings.Contains(pacHTTP, `/^(10\.|127\.|169\.254\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|198\.1[89]\.)/.test(host)`) {
		t.Errorf("expected PAC script to use RegExp for private and benchmark subnets")
	}
	if strings.Contains(pacHTTP, "172.1[6-9].*") || strings.Contains(pacHTTP, "198.1[8-9].*") {
		t.Errorf("PAC script must NOT contain invalid shExpMatch regex character classes")
	}
}

func TestGeneratePAC_Sanitization(t *testing.T) {
	cfg := &Config{
		Mode:      "rule",
		Whitelist: []string{`safe.com`, `bad"domain.com`},
		Blacklist: []string{`evil"domain.org`},
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	pac := eng.GeneratePAC("")
	if strings.Contains(pac, `bad"domain`) || strings.Contains(pac, `evil"domain`) {
		t.Errorf("PAC script should escape quotes in whitelist/blacklist domains")
	}
	if !strings.Contains(pac, `bad\"domain.com`) {
		t.Errorf("expected escaped quote bad\\\"domain.com")
	}
}
