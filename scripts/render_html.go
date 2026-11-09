package main

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/tui"
)

func ansiToHTML(ansiStr string) string {
	re := regexp.MustCompile(`\x1b\[([0-9;]*)m`)

	lines := strings.Split(ansiStr, "\n")
	var htmlLines []string

	colorMap := map[string]string{
		"30": "#1E293B",
		"31": "#F87171",
		"32": "#34D399",
		"33": "#FBBF24",
		"34": "#60A5FA",
		"35": "#E879F9",
		"36": "#22D3EE",
		"37": "#F8FAFC",
		"90": "#64748B",
		"91": "#EF4444",
		"92": "#10B981",
		"93": "#F59E0B",
		"94": "#3B82F6",
		"95": "#D946EF",
		"96": "#00E5FF",
		"97": "#FFFFFF",
	}

	for _, line := range lines {
		matches := re.FindAllStringSubmatchIndex(line, -1)
		if len(matches) == 0 {
			htmlLines = append(htmlLines, html.EscapeString(line))
			continue
		}

		var sb strings.Builder
		lastIdx := 0
		openSpans := 0

		for _, m := range matches {
			if m[0] > lastIdx {
				text := line[lastIdx:m[0]]
				sb.WriteString(html.EscapeString(text))
			}

			codeStr := line[m[2]:m[3]]
			codes := strings.Split(codeStr, ";")

			for _, c := range codes {
				if c == "0" || c == "" {
					for openSpans > 0 {
						sb.WriteString("</span>")
						openSpans--
					}
				} else if c == "1" {
					sb.WriteString(`<span style="font-weight:700;">`)
					openSpans++
				} else if color, ok := colorMap[c]; ok {
					sb.WriteString(fmt.Sprintf(`<span style="color:%s;">`, color))
					openSpans++
				}
			}

			lastIdx = m[1]
		}

		if lastIdx < len(line) {
			sb.WriteString(html.EscapeString(line[lastIdx:]))
		}

		for openSpans > 0 {
			sb.WriteString("</span>")
			openSpans--
		}

		htmlLines = append(htmlLines, sb.String())
	}

	return strings.Join(htmlLines, "\n")
}

func main() {
	cfg := &client.Config{
		SocksAddr: "127.0.0.1:10808",
		HttpAddr:  "127.0.0.1:10809",
		Groups: []client.SubscriptionGroup{
			{ID: "grp-1", Name: "FastVPN Pro", URL: "https://fastvpn.org/sub/123", UpdatedAt: time.Now().Unix()},
			{ID: "grp-2", Name: "Личные VPS", UpdatedAt: time.Now().Unix()},
			{ID: "grp-3", Name: "Ручные", UpdatedAt: time.Now().Unix()},
		},
		ActiveGroupID: "grp-1",
		Servers: []client.ServerProfile{
			{
				ID:            "nl-01",
				Name:          "Frankfurt Ultra",
				ServerAddr:    "45.12.33.1:443",
				MasterKey:     "secret-key-12345678901234567890",
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
				Ping:          38,
				Country:       "NL",
				GroupID:       "grp-1",
			},
			{
				ID:            "fi-01",
				Name:          "Helsinki Edge",
				ServerAddr:    "82.118.2.90:443",
				MasterKey:     "secret-key-12345678901234567890",
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
				Ping:          29,
				Country:       "FI",
				GroupID:       "grp-1",
			},
			{
				ID:            "de-01",
				Name:          "Berlin Shield",
				ServerAddr:    "194.87.12.5:443",
				MasterKey:     "secret-key-12345678901234567890",
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
				Ping:          46,
				Country:       "DE",
				GroupID:       "grp-1",
			},
			{
				ID:            "us-01",
				Name:          "Ashburn Fast",
				ServerAddr:    "104.24.8.11:443",
				MasterKey:     "secret-key-12345678901234567890",
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
				Ping:          112,
				Country:       "US",
				GroupID:       "grp-2",
			},
			{
				ID:            "se-01",
				Name:          "Stockholm Cloud",
				ServerAddr:    "185.120.44.8:443",
				MasterKey:     "secret-key-12345678901234567890",
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
				Ping:          34,
				Country:       "SE",
				GroupID:       "grp-3",
			},
		},
		ActiveID: "nl-01",
	}

	c, _ := client.NewClient(cfg.Servers[0], cfg.SocksAddr, cfg.HttpAddr)
	app := tui.New("shadowstar-client.json", c, cfg)

	mockStats := client.Stats{
		Connected:     true,
		Latency:       38,
		SpeedUp:       1420000,
		SpeedDown:     8740000,
		BytesUp:       14200000,
		BytesDown:     89400000,
		ActiveConns:   4,
		ConnectedTime: time.Now().Unix() - 5420,
	}
	histUp := []float64{100, 200, 400, 800, 1100, 1300, 1420, 1400, 1380, 1420}
	histDown := []float64{1200, 3400, 5600, 7800, 8900, 8740, 9200, 8900, 8500, 8740}
	app.SetStateForTest(mockStats, "✓ Защищенный туннель запущен!", histUp, histDown)

	// Render frame with clean 84 cols x 22 lines
	frame := app.RenderFrame(84, 22)
	htmlContent := ansiToHTML(frame)

	page := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: #060911;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      padding: 24px;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    }
    .terminal-window {
      background: #080D1A;
      border: 1px solid #1E2D48;
      border-radius: 12px;
      box-shadow: 0 25px 60px rgba(0, 0, 0, 0.75), 0 0 40px rgba(0, 229, 255, 0.08);
      overflow: hidden;
      display: inline-block;
    }
    .titlebar {
      background: #0D1527;
      border-bottom: 1px solid #162238;
      height: 38px;
      display: flex;
      align-items: center;
      padding: 0 16px;
      position: relative;
    }
    .dots {
      display: flex;
      gap: 8px;
    }
    .dot {
      width: 12px;
      height: 12px;
      border-radius: 50%%;
    }
    .dot-red { background: #EF4444; }
    .dot-yellow { background: #F59E0B; }
    .dot-green { background: #10B981; }
    .window-title {
      position: absolute;
      left: 0;
      right: 0;
      text-align: center;
      font-size: 12.5px;
      font-weight: 600;
      color: #94A3B8;
      letter-spacing: 0.3px;
    }
    .terminal-body {
      padding: 16px 20px 20px 20px;
      font-family: "Cascadia Code", Consolas, "JetBrains Mono", monospace;
      font-size: 13.5px;
      line-height: 1.25;
      color: #94A3B8;
      white-space: pre;
      overflow-x: hidden;
      font-variant-ligatures: none;
      letter-spacing: 0px;
      -webkit-font-smoothing: antialiased;
    }
  </style>
</head>
<body>
  <div class="terminal-window">
    <div class="titlebar">
      <div class="dots">
        <div class="dot dot-red"></div>
        <div class="dot dot-yellow"></div>
        <div class="dot dot-green"></div>
      </div>
      <div class="window-title">◈ ShadowStar (SSP) Anti-DPI Stealth Proxy — TUI Client v1.4.0</div>
    </div>
    <div class="terminal-body">%s</div>
  </div>
</body>
</html>`, htmlContent)

	htmlFile := "scripts/tui_render.html"
	_ = os.WriteFile(htmlFile, []byte(page), 0644)
	absHTML, _ := filepath.Abs(htmlFile)
	htmlURI := "file:///" + filepath.ToSlash(absHTML)

	// Run Chrome / Edge headless screenshot
	browsers := []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	}

	var browserExe string
	for _, b := range browsers {
		if _, err := os.Stat(b); err == nil {
			browserExe = b
			break
		}
	}

	if browserExe == "" {
		fmt.Println("No Chrome or Edge found")
		return
	}

	outAssets, _ := filepath.Abs("assets/shadowstar-tui.png")
	outDocs, _ := filepath.Abs("docs/screenshots/shadowstar-tui.png")
	outArtifact := `C:\Users\atack\.gemini\antigravity\brain\7b6ba60d-2cc7-485d-9c8b-38b0ceeb4b33\shadowstar-tui.png`

	cmd := exec.Command(browserExe,
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-crash-reporter",
		"--hide-scrollbars",
		"--force-device-scale-factor=2",
		"--window-size=980,680",
		fmt.Sprintf("--screenshot=%s", outAssets),
		htmlURI,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Browser error: %v | %s\n", err, string(out))
		return
	}

	// Copy to docs and artifact
	data, _ := os.ReadFile(outAssets)
	_ = os.WriteFile(outDocs, data, 0644)
	_ = os.WriteFile(outArtifact, data, 0644)

	fmt.Println("GORGEOUS pixel-perfect screenshot generated successfully!")
}
