package dns

import "crypto/tls"

// Config holds configuration parameters for the DNS engine.
type Config struct {
	FakeIPRange string      `json:"fake_ip_range"` // e.g. "198.18.0.0/15"
	UpstreamDoH []string    `json:"upstream_doh"`  // e.g. "https://1.1.1.1/dns-query"
	UpstreamDoT []string    `json:"upstream_dot"`  // e.g. "1.1.1.1:853"
	TLSConfig   *tls.Config `json:"-"`             // optional TLS configuration for DoT / DoH
}

// DefaultConfig returns recommended production defaults.
func DefaultConfig() *Config {
	return &Config{
		FakeIPRange: "198.18.0.0/15",
		UpstreamDoH: []string{
			"https://1.1.1.1/dns-query",
			"https://77.88.8.8/dns-query",
		},
		UpstreamDoT: []string{
			"1.1.1.1:853",
		},
	}
}
