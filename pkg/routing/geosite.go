package routing

import (
	"fmt"
	"strings"
	"sync"
)

var (
	geositeCacheMu sync.RWMutex
	geositeCache   = make(map[string]*DomainMatcher)
)

// GetGeoSiteMatcher returns a cached, compiled DomainMatcher for the given geosite tag (e.g. "geosite:category-ru", "geosite:ru").
func GetGeoSiteMatcher(tag string) (*DomainMatcher, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))

	geositeCacheMu.RLock()
	if m, ok := geositeCache[tag]; ok {
		geositeCacheMu.RUnlock()
		return m, nil
	}
	geositeCacheMu.RUnlock()

	geositeCacheMu.Lock()
	defer geositeCacheMu.Unlock()

	if m, ok := geositeCache[tag]; ok {
		return m, nil
	}

	matcher := NewDomainMatcher()

	switch tag {
	case "geosite:category-ru", "geosite:ru", "category-ru", "ru":
		for _, tld := range ruTLDs {
			matcher.AddSuffix(tld)
		}
		for _, domain := range ruDomains {
			matcher.AddSuffix(domain)
		}
	default:
		return nil, fmt.Errorf("unknown geosite tag: %s", tag)
	}

	geositeCache[tag] = matcher
	return matcher, nil
}
