package tun

// Config holds settings for the L3 TUN virtual network adapter
type Config struct {
	Enabled       bool     `json:"enabled"`
	DeviceName    string   `json:"device_name"`    // e.g. "ShadowStar"
	IPv4Address   string   `json:"ipv4_address"`   // e.g. "10.255.0.2"
	IPv4Mask      string   `json:"ipv4_mask"`      // e.g. "255.255.255.0"
	IPv4Gateway   string   `json:"ipv4_gateway"`   // e.g. "10.255.0.1"
	DNSServers    []string `json:"dns_servers"`    // e.g. ["198.18.0.1", "1.1.1.1"]
	MTU           int      `json:"mtu"`            // default 1500
	StrictRoute   bool     `json:"strict_route"`   // route 0.0.0.0/1 and 128.0.0.0/1
	KillSwitch    bool     `json:"kill_switch"`    // block leaks on disconnect
	Socks5Addr    string   `json:"socks5_addr"`    // "127.0.0.1:10808"
	DNSPort       int      `json:"dns_port"`       // local DNS server port
}

// DefaultConfig returns recommended production defaults for ShadowStar TUN
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		DeviceName:  "ShadowStar",
		IPv4Address: "10.255.0.2",
		IPv4Mask:    "255.255.255.0",
		IPv4Gateway: "10.255.0.1",
		DNSServers:  []string{"198.18.0.1", "1.1.1.1"},
		MTU:         1500,
		StrictRoute: true,
		KillSwitch:  false,
		Socks5Addr:  "127.0.0.1:10808",
		DNSPort:     53,
	}
}
