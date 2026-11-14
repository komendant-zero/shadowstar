package routing

import (
	"net"
	"strings"
)

type compiledRule struct {
	action    Action
	domains   *DomainMatcher
	ipMatcher *IPMatcher
}

type engineImpl struct {
	cfg       *Config
	whitelist *DomainMatcher
	blacklist *DomainMatcher
	rules     []compiledRule
}

// NewEngine compiles and returns a high-performance routing Engine.
func NewEngine(cfg *Config) (Engine, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	eng := &engineImpl{
		cfg:       cfg,
		whitelist: NewDomainMatcher(),
		blacklist: NewDomainMatcher(),
	}

	// Populate whitelist (exact, suffix, keyword)
	for _, item := range cfg.Whitelist {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, ".") {
			eng.whitelist.AddSuffix(item)
		} else if strings.Contains(item, "*") {
			kw := strings.ReplaceAll(item, "*", "")
			eng.whitelist.AddKeyword(kw)
		} else {
			eng.whitelist.AddExact(item)
			eng.whitelist.AddSuffix(item)
		}
	}

	// Populate blacklist
	for _, item := range cfg.Blacklist {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, ".") {
			eng.blacklist.AddSuffix(item)
		} else if strings.Contains(item, "*") {
			kw := strings.ReplaceAll(item, "*", "")
			eng.blacklist.AddKeyword(kw)
		} else {
			eng.blacklist.AddExact(item)
			eng.blacklist.AddSuffix(item)
		}
	}

	// Compile rules
	for _, r := range cfg.Rules {
		cr := compiledRule{
			action:    r.Action,
			domains:   NewDomainMatcher(),
			ipMatcher: NewIPMatcher(),
		}

		for _, d := range r.Domains {
			cr.domains.AddExact(d)
		}
		for _, s := range r.DomainSuffix {
			s = strings.TrimSpace(s)
			if strings.HasPrefix(strings.ToLower(s), "geosite:") {
				if _, err := GetGeoSiteMatcher(s); err == nil {
					// Merge geosite entries into domain trie
					for _, tld := range ruTLDs {
						cr.domains.AddSuffix(tld)
					}
					for _, d := range ruDomains {
						cr.domains.AddSuffix(d)
					}
				}
			} else {
				cr.domains.AddSuffix(s)
			}
		}
		for _, k := range r.DomainKeyword {
			cr.domains.AddKeyword(k)
		}

		hasIP := false
		for _, ipr := range r.IPRanges {
			ipr = strings.TrimSpace(ipr)
			if strings.HasPrefix(strings.ToLower(ipr), "geoip:") {
				if gm, err := GetGeoIPMatcher(ipr); err == nil {
					hasIP = true
					for _, iv := range gm.rangesV4 {
						cr.ipMatcher.AddInterval(iv.Start, iv.End)
					}
					cr.ipMatcher.netsV6 = append(cr.ipMatcher.netsV6, gm.netsV6...)
				}
			} else {
				if err := cr.ipMatcher.AddCIDR(ipr); err == nil {
					hasIP = true
				}
			}
		}
		if hasIP {
			cr.ipMatcher.Optimize()
		}

		eng.rules = append(eng.rules, cr)
	}

	return eng, nil
}

// Match evaluates destination against routing configuration and returns Action.
func (e *engineImpl) Match(dest string) Action {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return e.cfg.DefaultAction
	}

	// 1. Mode short-circuits
	switch strings.ToLower(e.cfg.Mode) {
	case "global":
		return ActionProxy
	case "direct":
		return ActionDirect
	}

	// 2. Extract host from destination (strip port and brackets if present)
	host := dest
	if h, _, err := net.SplitHostPort(dest); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	hostLower := strings.ToLower(host)

	// 3. Loopback short-circuit
	if hostLower == "localhost" || hostLower == "127.0.0.1" || hostLower == "::1" || strings.HasPrefix(hostLower, "127.") {
		return ActionDirect
	}

	// 4. Whitelist check (always direct)
	if e.whitelist.Match(hostLower) {
		return ActionDirect
	}

	// 5. Blacklist check (always proxy)
	if e.blacklist.Match(hostLower) {
		return ActionProxy
	}

	// 6. Check whether host is an IP address
	parsedIP := net.ParseIP(host)
	if parsedIP != nil {
		// Destination is an IP
		for _, rule := range e.rules {
			if rule.ipMatcher != nil && rule.ipMatcher.Match(parsedIP) {
				return rule.action
			}
		}
		return e.cfg.DefaultAction
	}

	// 7. Destination is a Domain Name
	for _, rule := range e.rules {
		if rule.domains != nil && rule.domains.Match(hostLower) {
			return rule.action
		}
	}

	return e.cfg.DefaultAction
}
