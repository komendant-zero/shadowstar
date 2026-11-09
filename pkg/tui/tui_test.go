package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shadowstar/shadowstar/pkg/client"
)

func TestStripANSIAndVisibleLen(t *testing.T) {
	colored := Bold(Cyan("Hello")) + " " + BrightGreen("World!")
	clean := StripANSI(colored)
	if clean != "Hello World!" {
		t.Fatalf("expected 'Hello World!', got %q", clean)
	}

	vl := VisibleLen(colored)
	if vl != 12 {
		t.Fatalf("expected visible len 12, got %d", vl)
	}
}

func TestFormatHelpers(t *testing.T) {
	if FormatBytes(500) != "500 B" {
		t.Fatalf("unexpected format bytes: %s", FormatBytes(500))
	}
	if FormatBytes(1024*1024*5) != "5.0 MB" {
		t.Fatalf("unexpected format bytes: %s", FormatBytes(1024*1024*5))
	}

	if FormatSpeed(1024*512) != "512.0 KB/s" {
		t.Fatalf("unexpected format speed: %s", FormatSpeed(1024*512))
	}

	if FormatDuration(3665) != "01:01:05" {
		t.Fatalf("unexpected format duration: %s", FormatDuration(3665))
	}
}

func TestRenderSparkline(t *testing.T) {
	history := []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	spark := RenderSparkline(history, 100, 10)
	if len([]rune(spark)) != 10 {
		t.Fatalf("expected sparkline rune length 10, got %d", len([]rune(spark)))
	}
}

func TestKeyReader(t *testing.T) {
	// Simulate input: 'c', Up arrow (\x1b[A), Enter (\r), Ctrl+C (\x03)
	inputData := []byte{'c', 0x1B, '[', 'A', '\r', 0x03}
	kr := NewKeyReader(bytes.NewReader(inputData))

	k1, err := kr.ReadKey()
	if err != nil || k1.Type != KeyRune || k1.Rune != 'c' {
		t.Fatalf("expected rune 'c', got %+v", k1)
	}

	k2, err := kr.ReadKey()
	if err != nil || k2.Type != KeyUp {
		t.Fatalf("expected KeyUp, got %+v", k2)
	}

	k3, err := kr.ReadKey()
	if err != nil || k3.Type != KeyEnter {
		t.Fatalf("expected KeyEnter, got %+v", k3)
	}

	k4, err := kr.ReadKey()
	if err != nil || k4.Type != KeyCtrlC {
		t.Fatalf("expected KeyCtrlC, got %+v", k4)
	}
}

func TestRenderFrameSmoke(t *testing.T) {
	cfg := &client.Config{
		SocksAddr: "127.0.0.1:10808",
		HttpAddr:  "127.0.0.1:10809",
		Servers: []client.ServerProfile{
			{
				ID:            "srv-1",
				Name:          "Frankfurt Ultra",
				ServerAddr:    "45.12.33.1:443",
				MasterKey:     "secret-key-12345678901234567890",
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
				Ping:          38,
				Country:       "NL",
			},
			{
				ID:            "srv-2",
				Name:          "Helsinki Edge",
				ServerAddr:    "82.118.2.90:443",
				MasterKey:     "secret-key-12345678901234567890",
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
				Ping:          29,
				Country:       "FI",
			},
		},
		ActiveID: "srv-1",
	}

	c, err := client.NewClient(cfg.Servers[0], cfg.SocksAddr, cfg.HttpAddr)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	app := New("test-cfg.json", c, cfg)

	// Test different dimensions
	dimensions := [][2]int{
		{80, 24},
		{100, 30},
		{120, 40},
		{70, 18}, // minimum clamped
	}

	for _, dim := range dimensions {
		frame := app.RenderFrame(dim[0], dim[1])
		if !strings.Contains(frame, "SHADOWSTAR") {
			t.Fatalf("frame missing title for dim %v", dim)
		}
		if !strings.Contains(frame, "Frankfurt Ultra") {
			t.Fatalf("frame missing server name for dim %v", dim)
		}
	}

	// Test switching tabs
	app.activeTab = TabLogs
	logsFrame := app.RenderFrame(80, 24)
	if !strings.Contains(logsFrame, "SHADOWSTAR") {
		t.Fatalf("logs frame failed")
	}

	app.activeTab = TabDiagnostics
	diagFrame := app.RenderFrame(80, 24)
	if !strings.Contains(diagFrame, "AES-256-GCM") {
		t.Fatalf("diag frame missing cipher")
	}

	// Test Modal
	app.modal = ModalAddNode
	modalFrame := app.RenderFrame(80, 24)
	if !strings.Contains(modalFrame, "ADD SHADOWSTAR NODE") && !strings.Contains(modalFrame, "ДОБАВЛЕНИЕ УЗЛА") {
		t.Fatalf("modal frame missing title")
	}
}

func TestExactLineWidthAlignment(t *testing.T) {
	cfg := &client.Config{
		SocksAddr: "127.0.0.1:10808",
		HttpAddr:  "127.0.0.1:10809",
		Servers: []client.ServerProfile{
			{
				ID:         "srv-1",
				Name:       "Frankfurt Ultra",
				ServerAddr: "45.12.33.1:443",
				SNI:        "www.microsoft.com",
				Country:    "NL",
				Ping:       38,
			},
			{
				ID:         "srv-2",
				Name:       "Helsinki Edge",
				ServerAddr: "82.118.2.90:443",
				SNI:        "www.microsoft.com",
				Country:    "FI",
				Ping:       29,
			},
		},
		ActiveID: "srv-1",
	}

	c, _ := client.NewClient(cfg.Servers[0], cfg.SocksAddr, cfg.HttpAddr)
	app := New("test.json", c, cfg)

	widths := []int{72, 80, 84, 90, 100, 120}
	tabs := []int{TabServers, TabLogs, TabDiagnostics}

	for _, w := range widths {
		for _, tab := range tabs {
			app.activeTab = tab
			frame := app.RenderFrame(w, 22)
			lines := strings.Split(frame, "\n")
			for lineIdx, line := range lines {
				vl := VisibleLen(line)
				if vl != w {
					t.Errorf("Width %d, Tab %d, Line %d has visible length %d, expected %d: %q",
						w, tab, lineIdx, vl, w, StripANSI(line))
				}
			}
		}
	}
}
