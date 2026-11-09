package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ANSI color escape sequences
const (
	Reset       = "\x1b[0m"
	BoldSeq     = "\x1b[1m"
	DimSeq      = "\x1b[2m"
	ItalicSeq   = "\x1b[3m"
	UnderlineSeq= "\x1b[4m"
	InverseSeq  = "\x1b[7m"

	FgBlack   = "\x1b[30m"
	FgRed     = "\x1b[31m"
	FgGreen   = "\x1b[32m"
	FgYellow  = "\x1b[33m"
	FgBlue    = "\x1b[34m"
	FgMagenta = "\x1b[35m"
	FgCyan    = "\x1b[36m"
	FgWhite   = "\x1b[37m"

	FgBrightBlack   = "\x1b[90m" // Gray
	FgBrightRed     = "\x1b[91m"
	FgBrightGreen   = "\x1b[92m"
	FgBrightYellow  = "\x1b[93m"
	FgBrightBlue    = "\x1b[94m"
	FgBrightMagenta = "\x1b[95m"
	FgBrightCyan    = "\x1b[96m"
	FgBrightWhite   = "\x1b[97m"

	BgDarkNavy = "\x1b[48;5;234m"
	BgHighlight= "\x1b[48;5;237m"
	BgCyan     = "\x1b[48;5;30m"
	BgActive   = "\x1b[48;5;22m"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func StripANSI(str string) string {
	return ansiRegexp.ReplaceAllString(str, "")
}

func RuneWidth(r rune) int {
	if r == 0xFE0F || r == 0xFE0E {
		return 0 // Variation selector (zero width)
	}
	// Wide emoji / East Asian characters
	if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2E80 && r <= 0x9FFF) {
		return 2
	}
	return 1
}

func VisibleLen(str string) int {
	clean := StripANSI(str)
	w := 0
	for _, r := range clean {
		w += RuneWidth(r)
	}
	return w
}

// FitLine ensures the string s has exactly visible width 'width',
// properly handling ANSI escape sequences, UTF-8 runes, and wide characters.
func FitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}

	var b strings.Builder
	curW := 0
	inEsc := false
	truncated := false

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// Check for ANSI escape start
		if r == 0x1b {
			inEsc = true
			b.WriteRune(r)
			continue
		}

		if inEsc {
			b.WriteRune(r)
			// ANSI CSI ends at any byte between 0x40 and 0x7E (e.g. 'm', 'H', 'J', etc.)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}

		rw := RuneWidth(r)
		if curW+rw > width {
			truncated = true
			break
		}

		b.WriteRune(r)
		curW += rw
	}

	if truncated {
		// Reset styles if truncated in the middle of styled text
		b.WriteString(Reset)
	}

	// Pad if shorter than width
	if curW < width {
		b.WriteString(strings.Repeat(" ", width-curW))
	}

	return b.String()
}

func Cyan(s string) string          { return FgCyan + s + Reset }
func BrightCyan(s string) string    { return FgBrightCyan + s + Reset }
func Green(s string) string         { return FgGreen + s + Reset }
func BrightGreen(s string) string   { return FgBrightGreen + s + Reset }
func Yellow(s string) string        { return FgYellow + s + Reset }
func BrightYellow(s string) string  { return FgBrightYellow + s + Reset }
func Red(s string) string           { return FgRed + s + Reset }
func BrightRed(s string) string     { return FgBrightRed + s + Reset }
func Magenta(s string) string       { return FgMagenta + s + Reset }
func BrightMagenta(s string) string { return FgBrightMagenta + s + Reset }
func Gray(s string) string          { return FgBrightBlack + s + Reset }
func White(s string) string         { return FgBrightWhite + s + Reset }
func Bold(s string) string          { return BoldSeq + s + Reset }
func Dim(s string) string           { return DimSeq + s + Reset }

func PadRight(s string, width int) string {
	vl := VisibleLen(s)
	if vl >= width {
		return FitLine(s, width)
	}
	return s + strings.Repeat(" ", width-vl)
}

func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	// Simple rune truncation
	var b strings.Builder
	count := 0
	for _, r := range s {
		if count >= width-1 && utf8.RuneCountInString(s) > width {
			b.WriteRune('…')
			break
		}
		b.WriteRune(r)
		count++
		if count >= width {
			break
		}
	}
	return b.String()
}

func Center(s string, width int) string {
	vl := VisibleLen(s)
	if vl >= width {
		return FitLine(s, width)
	}
	pad := (width - vl) / 2
	rightPad := width - vl - pad
	return strings.Repeat(" ", pad) + s + strings.Repeat(" ", rightPad)
}

func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func FormatSpeed(speed float64) string {
	if speed < 1024 {
		return fmt.Sprintf("%.0f B/s", speed)
	} else if speed < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", speed/1024)
	}
	return fmt.Sprintf("%.2f MB/s", speed/(1024*1024))
}

func FormatDuration(seconds int64) string {
	if seconds <= 0 {
		return "00:00:00"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// Sparkline generation using UTF-8 blocks:   ▂ ▃ ▄ ▅ ▆ ▇ █
var sparkBlocks = []rune{' ', ' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func RenderSparkline(history []float64, maxVal float64, width int) string {
	if width <= 0 {
		return ""
	}
	if len(history) == 0 {
		return strings.Repeat(" ", width)
	}

	// Slice last `width` values
	start := 0
	if len(history) > width {
		start = len(history) - width
	}
	sub := history[start:]

	// Find local maximum if not provided or zero
	if maxVal <= 0 {
		for _, v := range sub {
			if v > maxVal {
				maxVal = v
			}
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	var sb strings.Builder
	// Pad left if fewer values than width
	if len(sub) < width {
		sb.WriteString(strings.Repeat(" ", width-len(sub)))
	}

	for _, v := range sub {
		idx := int((v / maxVal) * float64(len(sparkBlocks)-1))
		if idx < 0 {
			idx = 0
		} else if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		sb.WriteRune(sparkBlocks[idx])
	}

	return sb.String()
}
