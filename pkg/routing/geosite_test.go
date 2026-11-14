package routing

import (
	"testing"
)

func TestDomainMatcher_Basic(t *testing.T) {
	m := NewDomainMatcher()
	m.AddExact("exact.com")
	m.AddSuffix("suffix.org")
	m.AddKeyword("keyword")

	tests := []struct {
		domain string
		expect bool
	}{
		{"exact.com", true},
		{"sub.exact.com", false}, // Exact does not match subdomain
		{"suffix.org", true},
		{"deep.sub.suffix.org", true},
		{"notsuffix.org", false},
		{"my-keyword-domain.net", true},
		{"other.org", false},
	}

	for _, tt := range tests {
		got := m.Match(tt.domain)
		if got != tt.expect {
			t.Errorf("domain %q: expected %v, got %v", tt.domain, tt.expect, got)
		}
	}
}

func TestGeoSite_CategoryRU(t *testing.T) {
	matcher, err := GetGeoSiteMatcher("geosite:category-ru")
	if err != nil {
		t.Fatalf("failed to get geosite:category-ru: %v", err)
	}

	positives := []string{
		"yandex.ru",
		"music.yandex.ru",
		"disk.yandex.com",
		"vk.com",
		"api.vk.com",
		"mail.ru",
		"gosuslugi.ru",
		"lk.gosuslugi.ru",
		"sberbank.ru",
		"tbank.ru",
		"ozon.ru",
		"wildberries.ru",
		"habr.com",
		"rutube.ru",
		"kremlin.ru",
		"президент.рф",
		"test.xn--p1ai",
		"my-site.su",
		"moscow.su",
	}

	for _, dom := range positives {
		if !matcher.Match(dom) {
			t.Errorf("expected %s to match geosite:category-ru", dom)
		}
	}

	negatives := []string{
		"google.com",
		"youtube.com",
		"facebook.com",
		"wikipedia.org",
		"not-yandex.com",
		"github.com",
	}

	for _, dom := range negatives {
		if matcher.Match(dom) {
			t.Errorf("expected %s NOT to match geosite:category-ru", dom)
		}
	}
}

func BenchmarkDomainMatcher_Match(b *testing.B) {
	matcher, _ := GetGeoSiteMatcher("geosite:category-ru")
	dom := "music.sub.yandex.ru"
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = matcher.Match(dom)
	}
}
