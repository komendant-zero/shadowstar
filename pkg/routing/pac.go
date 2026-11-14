package routing

import (
	"fmt"
	"strings"
)

// GeneratePAC generates an ES3/ES5 compatible PAC script for WinINet.
func (e *engineImpl) GeneratePAC(proxyAddr string) string {
	proxyAddr = strings.TrimSpace(proxyAddr)
	if proxyAddr == "" {
		proxyAddr = "127.0.0.1:10809"
	}

	proxySpec := "PROXY " + proxyAddr + "; DIRECT"
	if strings.HasPrefix(proxyAddr, "socks5://") {
		clean := strings.TrimPrefix(proxyAddr, "socks5://")
		proxySpec = "SOCKS5 " + clean + "; SOCKS " + clean + "; DIRECT"
	} else if strings.HasPrefix(proxyAddr, "http://") {
		clean := strings.TrimPrefix(proxyAddr, "http://")
		proxySpec = "PROXY " + clean + "; DIRECT"
	}

	var whitelistChecks []string
	for _, item := range e.cfg.Whitelist {
		item = strings.TrimSpace(item)
		if item != "" {
			item = strings.TrimPrefix(item, ".")
			item = strings.ReplaceAll(item, `\`, `\\`)
			item = strings.ReplaceAll(item, `"`, `\"`)
			whitelistChecks = append(whitelistChecks, fmt.Sprintf(`dnsDomainIs(host, "%s")`, item))
		}
	}

	var blacklistChecks []string
	for _, item := range e.cfg.Blacklist {
		item = strings.TrimSpace(item)
		if item != "" {
			item = strings.TrimPrefix(item, ".")
			item = strings.ReplaceAll(item, `\`, `\\`)
			item = strings.ReplaceAll(item, `"`, `\"`)
			blacklistChecks = append(blacklistChecks, fmt.Sprintf(`dnsDomainIs(host, "%s")`, item))
		}
	}

	wlCondition := "false"
	if len(whitelistChecks) > 0 {
		wlCondition = strings.Join(whitelistChecks, " || ")
	}

	blCondition := "false"
	if len(blacklistChecks) > 0 {
		blCondition = strings.Join(blacklistChecks, " || ")
	}

	return fmt.Sprintf(`// ShadowStar Dynamic PAC Script (WinINet ES3/ES5 Compatible)
function FindProxyForURL(url, host) {
    if (!host) {
        return "DIRECT";
    }

    // Strip port if present and handle bracketed IPv6 literals
    if (host.charAt(0) === "[") {
        var closeBracket = host.indexOf("]");
        if (closeBracket !== -1) {
            host = host.substring(1, closeBracket);
        }
    } else if (host.indexOf(":") === host.lastIndexOf(":") && host.indexOf(":") !== -1) {
        host = host.substring(0, host.indexOf(":"));
    }
    host = host.toLowerCase();

    // Plain hostnames (no dots and no colons) and loopback
    if ((isPlainHostName(host) && host.indexOf(":") === -1) || host === "127.0.0.1" || host === "::1" || host === "localhost") {
        return "DIRECT";
    }

    // RFC 1918 Private & Benchmark Subnets
    if (/^(10\.|127\.|169\.254\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|198\.1[89]\.)/.test(host)) {
        return "DIRECT";
    }

    // User Whitelist (always direct)
    if (%s) {
        return "DIRECT";
    }

    // User Blacklist (always proxy)
    if (%s) {
        return "%s";
    }

    // Russian TLDs and Domestic Services (Direct)
    if (/(^|\.)(ru|su|xn--p1ai|xn--80adxhks|xn--d1acj3b|moscow|tatar|yandex\.[a-z]+|ya\.ru|vk\.com|vk\.ru|mail\.ru|ok\.ru|sberbank\.ru|sber\.ru|tbank\.ru|tinkoff\.ru|vtb\.ru|alfabank\.ru|gosuslugi\.ru|mos\.ru|nalog\.ru|ozon\.ru|wildberries\.ru|wb\.ru|avito\.ru|rutube\.ru|rambler\.ru|2gis\.ru|kinopoisk\.ru|dzen\.ru)$/i.test(host)) {
        return "DIRECT";
    }

    // Default to Proxy
    return "%s";
}
`, wlCondition, blCondition, proxySpec, proxySpec)
}
