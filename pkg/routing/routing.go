package routing

import (
	"strings"
)

// Action defines the routing decision for a destination.
type Action int

const (
	ActionProxy Action = iota
	ActionDirect
	ActionBlock
)

func (a Action) String() string {
	switch a {
	case ActionProxy:
		return "proxy"
	case ActionDirect:
		return "direct"
	case ActionBlock:
		return "block"
	default:
		return "unknown"
	}
}

// ParseAction converts a string to an Action.
func ParseAction(s string) Action {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "direct":
		return ActionDirect
	case "block":
		return ActionBlock
	case "proxy":
		fallthrough
	default:
		return ActionProxy
	}
}

// Engine evaluates destination endpoints against routing rules.
type Engine interface {
	Match(dest string) Action // dest can be host:port, domain, or IP
	GeneratePAC(proxyAddr string) string
}

// Rule defines a set of match criteria and the corresponding action.
type Rule struct {
	Action        Action   `json:"action"`
	Domains       []string `json:"domains,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	IPRanges      []string `json:"ip_ranges,omitempty"`
}

// Config specifies the overall routing configuration.
type Config struct {
	Mode          string   `json:"mode"`           // "rule", "global", "direct"
	DefaultAction Action   `json:"default_action"` // ActionProxy or ActionDirect
	Rules         []Rule   `json:"rules"`
	Whitelist     []string `json:"whitelist"`      // Always direct
	Blacklist     []string `json:"blacklist"`      // Always proxy
}

// DefaultConfig provides sensible defaults routing domestic and private traffic directly.
func DefaultConfig() *Config {
	return &Config{
		Mode:          "rule",
		DefaultAction: ActionProxy,
		Rules: []Rule{
			{
				Action:   ActionDirect,
				IPRanges: []string{"geoip:private", "geoip:ru"},
				DomainSuffix: []string{"geosite:category-ru"},
			},
			{
				Action:   ActionProxy,
				IPRanges: []string{"0.0.0.0/0", "::/0"},
			},
		},
	}
}
