package config

import (
	"encoding/base64"
	"testing"

	"github.com/shadowstar/shadowstar/pkg/client"
)

func TestParseSubscriptionJSON(t *testing.T) {
	jsonContent := `[
		{
			"id": "node-1",
			"name": "Amsterdam #1",
			"server_addr": "1.2.3.4:443",
			"master_key": "key1",
			"sni": "www.microsoft.com"
		},
		{
			"id": "node-2",
			"name": "Frankfurt Ultra",
			"server_addr": "5.6.7.8:443",
			"master_key": "key2",
			"sni": "dl.google.com"
		}
	]`

	profiles, err := ParseSubscriptionContent([]byte(jsonContent))
	if err != nil {
		t.Fatalf("ParseSubscriptionContent failed: %v", err)
	}

	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}

	if profiles[0].Country != "NL" {
		t.Errorf("expected country NL, got %s", profiles[0].Country)
	}
	if profiles[1].Country != "DE" {
		t.Errorf("expected country DE, got %s", profiles[1].Country)
	}
}

func TestParseSubscriptionBase64(t *testing.T) {
	p1 := client.ServerProfile{
		ID:         "node-nl",
		Name:       "Netherlands Fast",
		ServerAddr: "1.1.1.1:443",
		MasterKey:  "secret-1",
		SNI:        "www.microsoft.com",
	}
	p2 := client.ServerProfile{
		ID:         "node-us",
		Name:       "USA East",
		ServerAddr: "2.2.2.2:443",
		MasterKey:  "secret-2",
		SNI:        "www.microsoft.com",
	}

	uri1 := EncodeProfileToURI(p1)
	uri2 := EncodeProfileToURI(p2)

	combined := uri1 + "\n" + uri2
	b64 := base64.StdEncoding.EncodeToString([]byte(combined))

	profiles, err := ParseSubscriptionContent([]byte(b64))
	if err != nil {
		t.Fatalf("Failed to parse base64 subscription: %v", err)
	}

	if len(profiles) != 2 {
		t.Fatalf("Expected 2 profiles, got %d", len(profiles))
	}

	if profiles[0].Country != "NL" {
		t.Errorf("Expected NL, got %s", profiles[0].Country)
	}
	if profiles[1].Country != "US" {
		t.Errorf("Expected US, got %s", profiles[1].Country)
	}
}

func TestDecodeURIFallbackNoUser(t *testing.T) {
	raw := "shadowstar://1.2.3.4:443?sni=www.microsoft.com#TestServer"
	prof, err := DecodeURIToProfile(raw)
	if err != nil {
		t.Fatalf("DecodeURIToProfile failed on raw URL without user: %v", err)
	}
	if prof.ServerAddr != "1.2.3.4:443" {
		t.Errorf("expected 1.2.3.4:443, got %s", prof.ServerAddr)
	}
	if prof.Name != "TestServer" {
		t.Errorf("expected TestServer, got %s", prof.Name)
	}
}

func TestParseSubscriptionMultiLines(t *testing.T) {
	text := "shadowstar://key1@1.2.3.4:443/?sni=www.microsoft.com#Node1\n" +
		"key2@5.6.7.8:443\n" +
		"9.10.11.12:4433:key3\n"
	profiles, err := ParseSubscriptionContent([]byte(text))
	if err != nil {
		t.Fatalf("ParseSubscriptionContent failed on multi-lines: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}
	if profiles[0].ServerAddr != "1.2.3.4:443" || profiles[0].MasterKey != "key1" {
		t.Errorf("profile 0 mismatch: %+v", profiles[0])
	}
	if profiles[1].ServerAddr != "5.6.7.8:443" || profiles[1].MasterKey != "key2" {
		t.Errorf("profile 1 mismatch: %+v", profiles[1])
	}
	if profiles[2].ServerAddr != "9.10.11.12:4433" || profiles[2].MasterKey != "key3" {
		t.Errorf("profile 2 mismatch: %+v", profiles[2])
	}
}

func TestDecodeKeyFormats(t *testing.T) {
	// 1. Raw 64-character hex key
	hexKey := "01d7b18d323cbb7a771ec7baffd331cff73f188b620f7d56a3dfc22c6cf535b2"
	p1, err := DecodeURIToProfile(hexKey)
	if err != nil {
		t.Fatalf("Failed to decode raw hex key: %v", err)
	}
	if p1.MasterKey != hexKey {
		t.Errorf("Expected master key %s, got %s", hexKey, p1.MasterKey)
	}

	// 2. host:port:key
	p2, err := DecodeURIToProfile("179.253.74.36:4433:" + hexKey)
	if err != nil {
		t.Fatalf("Failed to decode host:port:key: %v", err)
	}
	if p2.ServerAddr != "179.253.74.36:4433" || p2.MasterKey != hexKey {
		t.Errorf("Mismatch in p2: %+v", p2)
	}

	// 3. key@host:port
	p3, err := DecodeURIToProfile(hexKey + "@179.253.74.36:4433")
	if err != nil {
		t.Fatalf("Failed to decode key@host:port: %v", err)
	}
	if p3.ServerAddr != "179.253.74.36:4433" || p3.MasterKey != hexKey {
		t.Errorf("Mismatch in p3: %+v", p3)
	}
}
