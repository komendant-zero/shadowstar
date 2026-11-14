package routing

import (
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strings"
)

// IPRange represents a contiguous inclusive IPv4 range [Start, End].
type IPRange struct {
	Start uint32
	End   uint32
}

// IPMatcher performs fast O(log N) IP matching using binary interval search.
type IPMatcher struct {
	rangesV4 []IPRange
	netsV6   []*net.IPNet
}

// NewIPMatcher initializes an empty IPMatcher.
func NewIPMatcher() *IPMatcher {
	return &IPMatcher{}
}

// AddInterval directly appends an IPv4 range interval.
func (m *IPMatcher) AddInterval(start, end uint32) {
	if start <= end {
		m.rangesV4 = append(m.rangesV4, IPRange{Start: start, End: end})
	}
}

// AddCIDR parses and adds an IPv4 or IPv6 CIDR/IP string.
func (m *IPMatcher) AddCIDR(cidrStr string) error {
	cidrStr = strings.TrimSpace(cidrStr)
	if cidrStr == "" {
		return nil
	}

	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		ip := net.ParseIP(cidrStr)
		if ip == nil {
			return fmt.Errorf("invalid CIDR or IP address: %s", cidrStr)
		}
		if ip4 := ip.To4(); ip4 != nil {
			ipNet = &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}
		} else {
			ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		}
	}

	if ip4 := ipNet.IP.To4(); ip4 != nil {
		mask := binary.BigEndian.Uint32(ipNet.Mask)
		start := binary.BigEndian.Uint32(ip4) & mask
		end := start | (^mask)
		m.rangesV4 = append(m.rangesV4, IPRange{Start: start, End: end})
	} else {
		m.netsV6 = append(m.netsV6, ipNet)
	}
	return nil
}

// Optimize sorts and merges overlapping or adjacent IPv4 intervals for fast binary searching.
func (m *IPMatcher) Optimize() {
	if len(m.rangesV4) == 0 {
		return
	}

	sort.Slice(m.rangesV4, func(i, j int) bool {
		return m.rangesV4[i].Start < m.rangesV4[j].Start
	})

	merged := make([]IPRange, 0, len(m.rangesV4))
	curr := m.rangesV4[0]

	for _, next := range m.rangesV4[1:] {
		if curr.End == 0xFFFFFFFF || next.Start <= curr.End+1 {
			if next.End > curr.End {
				curr.End = next.End
			}
		} else {
			merged = append(merged, curr)
			curr = next
		}
	}
	merged = append(merged, curr)
	m.rangesV4 = merged
}

// Match checks if the given IP address falls within any registered range.
func (m *IPMatcher) Match(ip net.IP) bool {
	if ip == nil {
		return false
	}

	if ip4 := ip.To4(); ip4 != nil {
		val := binary.BigEndian.Uint32(ip4)
		idx := sort.Search(len(m.rangesV4), func(i int) bool {
			return m.rangesV4[i].End >= val
		})
		if idx < len(m.rangesV4) && m.rangesV4[idx].Start <= val {
			return true
		}
		return false
	}

	for _, n := range m.netsV6 {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// MatchString parses an IP string (with optional port) and evaluates Match.
func (m *IPMatcher) MatchString(dest string) bool {
	dest = strings.TrimSpace(dest)
	if host, _, err := net.SplitHostPort(dest); err == nil {
		dest = host
	}
	dest = strings.TrimPrefix(dest, "[")
	dest = strings.TrimSuffix(dest, "]")

	ip := net.ParseIP(dest)
	if ip == nil {
		return false
	}
	return m.Match(ip)
}
