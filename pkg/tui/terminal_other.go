//go:build !windows

package tui

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

var initialized bool

// InitTerminal configures Unix terminal for raw mode and alternate screen
func InitTerminal() error {
	// Raw mode via stty (most portable across Linux / macOS / BSD)
	cmd := exec.Command("stty", "-echo", "-icanon", "min", "1", "time", "0")
	cmd.Stdin = os.Stdin
	_ = cmd.Run()

	initialized = true

	// Switch to alternate screen buffer, clear screen, hide cursor
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
	return nil
}

// RestoreTerminal cleans up console modes and restores normal terminal screen
func RestoreTerminal() {
	if !initialized {
		return
	}
	os.Stdout.WriteString("\x1b[?25h\x1b[?1049l")

	cmd := exec.Command("stty", "sane")
	cmd.Stdin = os.Stdin
	_ = cmd.Run()

	initialized = false
}

// GetTerminalSize returns width and height in columns and rows
func GetTerminalSize() (int, int) {
	ws := &winsize{}
	// TIOCGWINSZ ioctl (0x5413 on Linux, standard ioctl)
	retCode, _, _ := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(syscall.Stdout),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)),
	)
	if int(retCode) >= 0 && ws.Col > 0 && ws.Row > 0 {
		return int(ws.Col), int(ws.Row)
	}

	// Fallback: check stty size
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	if out, err := cmd.Output(); err == nil {
		parts := strings.Fields(string(out))
		if len(parts) == 2 {
			r, err1 := strconv.Atoi(parts[0])
			c, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil && c > 0 && r > 0 {
				return c, r
			}
		}
	}

	return 80, 24
}
