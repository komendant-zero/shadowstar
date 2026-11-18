package server

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	blockedCases := []string{
		"127.0.0.1",
		"127.0.0.2",
		"10.0.0.1",
		"10.255.255.254",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.1.1",
		"192.168.254.254",
		"169.254.169.254",
		"169.254.1.1",
		"100.64.0.1",
		"100.127.255.255",
		"100.100.100.200",
		"0.0.0.0",
		"::1",
		"fe80::1",
		"fc00::1",
		"fd00::1",
		"fd00:ec2::254",
		"::ffff:127.0.0.1",
		"::ffff:10.0.0.1",
		"::ffff:192.168.1.1",
		"::ffff:169.254.169.254",
	}

	for _, ipStr := range blockedCases {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("Failed to parse test IP: %s", ipStr)
		}
		if !isBlockedIP(ip) {
			t.Errorf("Expected IP %s to be blocked, but was allowed", ipStr)
		}
	}

	allowedCases := []string{
		"8.8.8.8",
		"1.1.1.1",
		"142.250.180.14",
		"20.112.52.29",
		"2606:4700:4700::1111",
	}

	for _, ipStr := range allowedCases {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("Failed to parse test IP: %s", ipStr)
		}
		if isBlockedIP(ip) {
			t.Errorf("Expected public IP %s to be allowed, but was blocked", ipStr)
		}
	}
}

func TestIsBlockedTarget(t *testing.T) {
	blockedHosts := []string{
		"",
		"localhost",
		"test.localhost",
		"my-host.local",
		"cluster.internal",
		"router.lan",
		"gateway.home.arpa",
		"127.0.0.1",
		"10.1.2.3",
		"192.168.0.1",
		"169.254.169.254",
	}

	for _, host := range blockedHosts {
		if !isBlockedTarget(host) {
			t.Errorf("Expected host %q to be blocked", host)
		}
	}
}
