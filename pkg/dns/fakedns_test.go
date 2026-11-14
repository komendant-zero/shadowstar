package dns

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"testing"
)

func TestFakeDNS_AllocationBounds(t *testing.T) {
	fd, err := NewFakeDNS("198.18.0.0/15")
	if err != nil {
		t.Fatalf("NewFakeDNS failed: %v", err)
	}

	ip := fd.Allocate("example.com")
	if ip == nil {
		t.Fatal("expected non-nil IP")
	}

	ip4 := ip.To4()
	if ip4 == nil {
		t.Fatal("expected IPv4 address")
	}

	val := binary.BigEndian.Uint32(ip4)
	start := binary.BigEndian.Uint32(net.ParseIP("198.18.0.2").To4())
	end := binary.BigEndian.Uint32(net.ParseIP("198.19.255.254").To4())
	gw := binary.BigEndian.Uint32(net.ParseIP("198.18.0.1").To4())

	if val < start || val > end {
		t.Errorf("allocated IP %s out of bounds [%s, %s]", ip, "198.18.0.2", "198.19.255.254")
	}
	if val == gw {
		t.Errorf("allocated IP %s cannot be gateway IP 198.18.0.1", ip)
	}
	if !fd.IsFakeIP(ip) {
		t.Errorf("IsFakeIP returned false for allocated IP %s", ip)
	}
}

func TestFakeDNS_IdempotencyAndNormalization(t *testing.T) {
	fd, err := NewFakeDNS("")
	if err != nil {
		t.Fatalf("NewFakeDNS failed: %v", err)
	}

	ip1 := fd.Allocate("test.com")
	ip2 := fd.Allocate("TEST.COM.")
	ip3 := fd.Allocate(" test.com ")

	if !ip1.Equal(ip2) || !ip1.Equal(ip3) {
		t.Errorf("expected idempotent allocation: ip1=%s, ip2=%s, ip3=%s", ip1, ip2, ip3)
	}

	dom, found := fd.LookupDomain(ip1)
	if !found || dom != "test.com" {
		t.Errorf("expected domain test.com, got %s (found=%v)", dom, found)
	}
}

func TestFakeDNS_LRUEviction(t *testing.T) {
	// Create small pool of 2 usable addresses
	fd, err := NewFakeDNS("198.18.0.0/30") // /30: 4 IPs: .0 net, .1 gw, .2 usable, .3 bcast -> too small!
	// Let's use /29: 8 IPs: .0 net, .1 gw, .2-.6 usable (5 IPs)
	fd, err = NewFakeDNS("198.18.0.0/29")
	if err != nil {
		t.Fatalf("NewFakeDNS failed: %v", err)
	}
	impl := fd.(*fakeDNSImpl)
	impl.maxCapacity = 2 // artifically set small capacity for testing eviction

	ipA := fd.Allocate("domain-a.com")
	ipB := fd.Allocate("domain-b.com")

	// Now allocate 3rd domain -> should evict domain-a
	ipC := fd.Allocate("domain-c.com")

	if _, found := fd.LookupDomain(ipA); found {
		t.Errorf("expected domain-a to be evicted from LRU pool")
	}
	if dom, found := fd.LookupDomain(ipB); !found || dom != "domain-b.com" {
		t.Errorf("expected domain-b to remain, got %s (found=%v)", dom, found)
	}
	if dom, found := fd.LookupDomain(ipC); !found || dom != "domain-c.com" {
		t.Errorf("expected domain-c to exist, got %s (found=%v)", dom, found)
	}
}

func TestFakeDNS_Concurrency(t *testing.T) {
	fd, err := NewFakeDNS("198.18.0.0/15")
	if err != nil {
		t.Fatalf("NewFakeDNS failed: %v", err)
	}

	var wg sync.WaitGroup
	workers := 100
	iterations := 200

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				dom := fmt.Sprintf("worker-%d-domain-%d.com", workerID, i%20)
				ip := fd.Allocate(dom)
				if ip == nil {
					t.Errorf("got nil ip for %s", dom)
					return
				}
				lookup, found := fd.LookupDomain(ip)
				if !found || lookup == "" {
					t.Errorf("lookup failed for %s -> %s", dom, ip)
					return
				}
				if !fd.IsFakeIP(ip) {
					t.Errorf("IsFakeIP failed for %s", ip)
					return
				}
			}
		}(w)
	}

	wg.Wait()
}

func BenchmarkFakeDNS_Allocate(b *testing.B) {
	fd, _ := NewFakeDNS("198.18.0.0/15")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = fd.Allocate("benchmark.example.com")
	}
}

func BenchmarkFakeDNS_LookupDomain(b *testing.B) {
	fd, _ := NewFakeDNS("198.18.0.0/15")
	ip := fd.Allocate("benchmark.example.com")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = fd.LookupDomain(ip)
	}
}

func BenchmarkFakeDNS_IsFakeIP(b *testing.B) {
	fd, _ := NewFakeDNS("198.18.0.0/15")
	ip := net.ParseIP("198.18.5.10")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = fd.IsFakeIP(ip)
	}
}
