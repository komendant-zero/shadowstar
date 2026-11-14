package routing

import (
	"net"
	"testing"
)

func TestIPMatcher_Intervals(t *testing.T) {
	m := NewIPMatcher()
	if err := m.AddCIDR("192.168.1.0/24"); err != nil {
		t.Fatalf("AddCIDR failed: %v", err)
	}
	if err := m.AddCIDR("10.0.0.0/8"); err != nil {
		t.Fatalf("AddCIDR failed: %v", err)
	}
	m.Optimize()

	if !m.Match(net.ParseIP("192.168.1.1")) {
		t.Errorf("expected 192.168.1.1 to match")
	}
	if !m.Match(net.ParseIP("192.168.1.254")) {
		t.Errorf("expected 192.168.1.254 to match")
	}
	if m.Match(net.ParseIP("192.168.2.1")) {
		t.Errorf("expected 192.168.2.1 not to match")
	}
	if !m.Match(net.ParseIP("10.254.254.254")) {
		t.Errorf("expected 10.254.254.254 to match")
	}
	if m.Match(net.ParseIP("11.0.0.1")) {
		t.Errorf("expected 11.0.0.1 not to match")
	}
}

func TestIPMatcher_Intervals_Uint32Overflow(t *testing.T) {
	m := NewIPMatcher()
	if err := m.AddCIDR("0.0.0.0/0"); err != nil {
		t.Fatalf("AddCIDR failed: %v", err)
	}
	if err := m.AddCIDR("10.0.0.0/8"); err != nil {
		t.Fatalf("AddCIDR failed: %v", err)
	}
	m.Optimize()

	// Verify intervals are monotonic and merged into a single [0, 0xFFFFFFFF] range
	if len(m.rangesV4) != 1 {
		t.Fatalf("expected 1 merged range covering entire IPv4 space, got %d", len(m.rangesV4))
	}
	if m.rangesV4[0].Start != 0 || m.rangesV4[0].End != 0xFFFFFFFF {
		t.Errorf("expected range [0, 0xFFFFFFFF], got [%d, %d]", m.rangesV4[0].Start, m.rangesV4[0].End)
	}

	testIPs := []string{"0.0.0.1", "10.0.0.1", "11.0.0.1", "192.168.1.1", "255.255.255.255"}
	for _, ipStr := range testIPs {
		if !m.Match(net.ParseIP(ipStr)) {
			t.Errorf("expected %s to match 0.0.0.0/0", ipStr)
		}
	}

	// Also verify non-default range adjacent to top of address space
	m2 := NewIPMatcher()
	if err := m2.AddCIDR("10.0.0.0/8"); err != nil {
		t.Fatalf("AddCIDR failed: %v", err)
	}
	if err := m2.AddCIDR("128.0.0.0/1"); err != nil { // 128.0.0.0 to 255.255.255.255
		t.Fatalf("AddCIDR failed: %v", err)
	}
	m2.Optimize()

	if len(m2.rangesV4) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(m2.rangesV4))
	}
	if m2.rangesV4[0].End >= m2.rangesV4[1].End {
		t.Errorf("ranges are not monotonically increasing in End: %d >= %d", m2.rangesV4[0].End, m2.rangesV4[1].End)
	}
	if !m2.Match(net.ParseIP("10.1.2.3")) {
		t.Errorf("expected 10.1.2.3 to match")
	}
	if m2.Match(net.ParseIP("11.0.0.1")) {
		t.Errorf("expected 11.0.0.1 not to match")
	}
	if !m2.Match(net.ParseIP("128.0.0.1")) {
		t.Errorf("expected 128.0.0.1 to match")
	}
	if !m2.Match(net.ParseIP("255.255.255.255")) {
		t.Errorf("expected 255.255.255.255 to match")
	}
}

func TestGeoIP_Private(t *testing.T) {
	matcher, err := GetGeoIPMatcher("geoip:private")
	if err != nil {
		t.Fatalf("failed to get geoip:private: %v", err)
	}

	positives := []string{
		"127.0.0.1",
		"10.10.10.10",
		"172.16.0.1",
		"172.31.255.254",
		"192.168.0.1",
		"100.64.1.1",
		"169.254.10.20",
		"198.18.0.2",
		"198.19.255.254",
		"::1",
	}
	for _, ipStr := range positives {
		ip := net.ParseIP(ipStr)
		if !matcher.Match(ip) {
			t.Errorf("expected %s to match geoip:private", ipStr)
		}
	}

	negatives := []string{
		"8.8.8.8",
		"1.1.1.1",
		"77.88.55.66",
		"172.32.0.1",
		"198.20.0.1",
	}
	for _, ipStr := range negatives {
		ip := net.ParseIP(ipStr)
		if matcher.Match(ip) {
			t.Errorf("expected %s NOT to match geoip:private", ipStr)
		}
	}
}

func TestGeoIP_RU(t *testing.T) {
	matcher, err := GetGeoIPMatcher("geoip:ru")
	if err != nil {
		t.Fatalf("failed to get geoip:ru: %v", err)
	}

	positives := []string{
		"77.88.55.66",    // Yandex
		"87.250.250.250", // Yandex DNS
		"95.163.0.1",     // VK
		"212.188.0.1",    // MTS
		"178.176.0.1",    // Megafon
		"185.165.120.1",  // Sber
	}
	for _, ipStr := range positives {
		ip := net.ParseIP(ipStr)
		if !matcher.Match(ip) {
			t.Errorf("expected %s to match geoip:ru", ipStr)
		}
	}

	negatives := []string{
		"1.1.1.1",
		"8.8.4.4",
		"140.82.121.3", // GitHub
		"104.244.42.1", // Twitter/X
	}
	for _, ipStr := range negatives {
		ip := net.ParseIP(ipStr)
		if matcher.Match(ip) {
			t.Errorf("expected %s NOT to match geoip:ru", ipStr)
		}
	}
}

func BenchmarkIPMatcher_Match(b *testing.B) {
	matcher, _ := GetGeoIPMatcher("geoip:ru")
	ip := net.ParseIP("77.88.55.66")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = matcher.Match(ip)
	}
}
