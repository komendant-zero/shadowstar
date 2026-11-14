package dns

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"
)

// FakeDNS provides synthetic IP address allocation in 198.18.0.0/15 and bidirectional mapping.
type FakeDNS interface {
	Allocate(domain string) net.IP
	LookupDomain(ip net.IP) (string, bool)
	IsFakeIP(ip net.IP) bool
}

type lruNode struct {
	domain string
	ip     uint32
	prev   *lruNode
	next   *lruNode
}

type fakeDNSImpl struct {
	mu          sync.RWMutex
	network     uint32
	mask        uint32
	startIP     uint32
	endIP       uint32
	gatewayIP   uint32
	currentIP   uint32
	maxCapacity int

	domainToNode map[string]*lruNode
	ipToNode     map[uint32]*lruNode

	head *lruNode // MRU
	tail *lruNode // LRU
}

// NewFakeDNS initializes a FakeDNS allocator with the given IPv4 CIDR (defaults to 198.18.0.0/15).
func NewFakeDNS(cidr string) (FakeDNS, error) {
	if strings.TrimSpace(cidr) == "" {
		cidr = "198.18.0.0/15"
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return nil, errors.New("fakedns requires IPv4 CIDR")
	}

	network := binary.BigEndian.Uint32(ip4)
	mask := binary.BigEndian.Uint32(ipNet.Mask)
	broadcast := network | (^mask)

	gatewayIP := network + 1 // 198.18.0.1 (TUN virtual DNS)
	startIP := network + 2   // 198.18.0.2 (First allocatable)
	endIP := broadcast - 1   // 198.19.255.254 (Last allocatable)

	if endIP <= startIP {
		return nil, errors.New("CIDR subnet too small for fakedns pool")
	}

	usableCount := int(endIP - startIP + 1)
	capacity := usableCount
	if capacity > 65534 {
		capacity = 65534
	}

	return &fakeDNSImpl{
		network:      network,
		mask:         mask,
		startIP:      startIP,
		endIP:        endIP,
		gatewayIP:    gatewayIP,
		currentIP:    startIP,
		maxCapacity:  capacity,
		domainToNode: make(map[string]*lruNode, 1024),
		ipToNode:     make(map[uint32]*lruNode, 1024),
	}, nil
}

// Allocate returns a synthetic FakeIP for the domain, maintaining bidirectional LRU mappings.
func (f *fakeDNSImpl) Allocate(domain string) net.IP {
	domain = normalizeDomainName(domain)
	if domain == "" {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// 1. Existing mapping hit: bump to MRU
	if node, ok := f.domainToNode[domain]; ok {
		f.moveToHead(node)
		return uint32ToIP(node.ip)
	}

	// 2. If pool capacity full: evict LRU tail first
	if len(f.domainToNode) >= f.maxCapacity {
		lru := f.tail
		delete(f.domainToNode, lru.domain)
		delete(f.ipToNode, lru.ip)
		f.removeNode(lru)
	}

	// 3. Find next available circular IP
	var ip uint32
	for {
		cand := f.currentIP
		f.currentIP++
		if f.currentIP > f.endIP {
			f.currentIP = f.startIP
		}
		if cand == f.gatewayIP {
			continue
		}
		if _, exists := f.ipToNode[cand]; !exists {
			ip = cand
			break
		}
	}

	node := &lruNode{domain: domain, ip: ip}
	f.addToHead(node)
	f.domainToNode[domain] = node
	f.ipToNode[ip] = node

	return uint32ToIP(ip)
}

// LookupDomain resolves a FakeIP back to its original domain name.
func (f *fakeDNSImpl) LookupDomain(ip net.IP) (string, bool) {
	ip4 := ip.To4()
	if ip4 == nil {
		return "", false
	}
	val := binary.BigEndian.Uint32(ip4)

	f.mu.RLock()
	defer f.mu.RUnlock()

	node, ok := f.ipToNode[val]
	if !ok {
		return "", false
	}
	return node.domain, true
}

// IsFakeIP performs an O(1) lockless bitwise subnet check.
func (f *fakeDNSImpl) IsFakeIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	val := binary.BigEndian.Uint32(ip4)
	return (val & f.mask) == f.network
}

func uint32ToIP(v uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, v)
	return ip
}

func normalizeDomainName(d string) string {
	d = strings.TrimSpace(strings.ToLower(d))
	d = strings.TrimPrefix(d, ".")
	d = strings.TrimSuffix(d, ".")
	return d
}

func (f *fakeDNSImpl) addToHead(node *lruNode) {
	node.next = f.head
	node.prev = nil
	if f.head != nil {
		f.head.prev = node
	}
	f.head = node
	if f.tail == nil {
		f.tail = node
	}
}

func (f *fakeDNSImpl) removeNode(node *lruNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		f.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		f.tail = node.prev
	}
	node.prev = nil
	node.next = nil
}

func (f *fakeDNSImpl) moveToHead(node *lruNode) {
	if f.head == node {
		return
	}
	f.removeNode(node)
	f.addToHead(node)
}
