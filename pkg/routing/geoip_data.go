package routing

import (
	"fmt"
	"strings"
	"sync"
)

var (
	privateCIDRs = []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.88.99.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
		"255.255.255.255/32",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	// ruCIDRs contains representative RIPE NCC IPv4 allocations for Russian autonomous systems
	// (Yandex, VK/Mail.ru, Rostelecom, MTS, Megafon, Beeline, ER-Telecom, Sberbank, etc.)
	ruCIDRs = []string{
		// Yandex
		"5.45.192.0/18",
		"5.255.192.0/18",
		"37.9.64.0/18",
		"37.140.128.0/18",
		"77.88.0.0/18",
		"87.250.224.0/19",
		"93.158.128.0/18",
		"95.108.128.0/17",
		"141.8.128.0/18",
		"178.154.128.0/18",
		"185.32.188.0/22",
		"213.180.192.0/19",

		// VK / Mail.Ru / Odnoklassniki
		"94.100.176.0/20",
		"95.163.0.0/16",
		"128.140.168.0/21",
		"178.22.88.0/21",
		"185.16.244.0/22",
		"185.30.176.0/22",
		"217.69.128.0/18",

		// Rostelecom / RT
		"77.37.128.0/17",
		"78.85.0.0/16",
		"81.177.0.0/16",
		"85.26.0.0/15",
		"92.243.160.0/19",
		"178.176.0.0/15",
		"178.178.0.0/15",
		"188.162.0.0/16",
		"188.232.0.0/15",
		"188.234.0.0/15",
		"212.48.128.0/18",
		"212.188.0.0/16",
		"213.59.0.0/16",
		"213.87.0.0/16",
		"217.106.0.0/15",
		"217.118.64.0/19",

		// MTS (Mobile TeleSystems)
		"94.25.0.0/16",
		"178.64.0.0/13",
		"178.72.0.0/13",
		"80.89.128.0/19",

		// Megafon
		"78.25.0.0/16",
		"83.149.0.0/16",
		"95.31.0.0/16",

		// Beeline / VimpelCom
		"77.41.0.0/16",
		"77.51.0.0/16",
		"81.9.112.0/20",
		"85.115.224.0/19",
		"194.186.0.0/16",

		// Sberbank & Financial
		"185.165.120.0/22",
		"185.165.124.0/22",
		"194.54.14.0/24",
		"194.54.15.0/24",
		"194.67.64.0/19",
		"212.45.0.0/19",

		// ER-Telecom (Dom.ru)
		"188.168.0.0/15",
		"188.170.0.0/15",
		"188.226.0.0/16",

		// Selectel & Russian Cloud
		"185.129.100.0/22",
		"188.93.16.0/20",
		"193.106.112.0/22",
		"195.208.0.0/16",
	}

	geoIPCacheMu sync.RWMutex
	geoIPCache   = make(map[string]*IPMatcher)
)

// GetGeoIPMatcher returns a cached, optimized IPMatcher for the specified tag (e.g. "geoip:private", "geoip:ru").
func GetGeoIPMatcher(tag string) (*IPMatcher, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))

	geoIPCacheMu.RLock()
	if m, ok := geoIPCache[tag]; ok {
		geoIPCacheMu.RUnlock()
		return m, nil
	}
	geoIPCacheMu.RUnlock()

	geoIPCacheMu.Lock()
	defer geoIPCacheMu.Unlock()

	if m, ok := geoIPCache[tag]; ok {
		return m, nil
	}

	var cidrs []string
	switch tag {
	case "geoip:private", "private":
		cidrs = privateCIDRs
	case "geoip:ru", "ru":
		cidrs = ruCIDRs
	default:
		return nil, fmt.Errorf("unknown geoip tag: %s", tag)
	}

	matcher := NewIPMatcher()
	for _, cidr := range cidrs {
		_ = matcher.AddCIDR(cidr)
	}
	matcher.Optimize()

	geoIPCache[tag] = matcher
	return matcher, nil
}
