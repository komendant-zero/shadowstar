package routing

import (
	"net"
	"strings"
)

type trieNode struct {
	children map[string]*trieNode
	isSuffix bool
}

// DomainMatcher performs fast exact, suffix, and keyword matching.
type DomainMatcher struct {
	root     *trieNode
	exact    map[string]struct{}
	keywords []string
}

// NewDomainMatcher initializes an empty DomainMatcher.
func NewDomainMatcher() *DomainMatcher {
	return &DomainMatcher{
		root:  &trieNode{children: make(map[string]*trieNode)},
		exact: make(map[string]struct{}),
	}
}

// AddExact registers an exact domain name match.
func (m *DomainMatcher) AddExact(domain string) {
	d := normalizeDomain(domain)
	if d != "" {
		m.exact[d] = struct{}{}
	}
}

// AddSuffix registers a domain suffix into the reverse label trie.
func (m *DomainMatcher) AddSuffix(suffix string) {
	s := normalizeDomain(suffix)
	if s == "" {
		return
	}
	parts := strings.Split(s, ".")
	node := m.root
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		child, exists := node.children[part]
		if !exists {
			child = &trieNode{children: make(map[string]*trieNode)}
			node.children[part] = child
		}
		node = child
	}
	node.isSuffix = true
}

// AddKeyword registers a substring keyword to match against domains.
func (m *DomainMatcher) AddKeyword(kw string) {
	k := strings.ToLower(strings.TrimSpace(kw))
	if k != "" {
		m.keywords = append(m.keywords, k)
	}
}

// Match checks whether a domain matches any exact, suffix, or keyword rule.
func (m *DomainMatcher) Match(domain string) bool {
	d := normalizeDomain(domain)
	if d == "" {
		return false
	}

	// 1. Exact match (O(1))
	if _, ok := m.exact[d]; ok {
		return true
	}

	// 2. Keyword match
	for _, kw := range m.keywords {
		if strings.Contains(d, kw) {
			return true
		}
	}

	// 3. Suffix match via reverse trie
	parts := strings.Split(d, ".")
	node := m.root
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		child, exists := node.children[part]
		if !exists {
			return false
		}
		if child.isSuffix {
			return true
		}
		node = child
	}

	return false
}

// normalizeDomain strips ports, whitespace, brackets, and dots, and lowercases.
func normalizeDomain(dest string) string {
	dest = strings.TrimSpace(strings.ToLower(dest))
	if dest == "" {
		return ""
	}

	// If dest contains a port (e.g. "example.com:443" or "[::1]:80")
	if host, _, err := net.SplitHostPort(dest); err == nil {
		dest = host
	}

	dest = strings.TrimPrefix(dest, "[")
	dest = strings.TrimSuffix(dest, "]")
	dest = strings.TrimPrefix(dest, ".")
	dest = strings.TrimSuffix(dest, ".")

	return dest
}
