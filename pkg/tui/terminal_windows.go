//go:build windows

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	pGetStdHandle               = kernel32.NewProc("GetStdHandle")
	pGetConsoleMode              = kernel32.NewProc("GetConsoleMode")
	pSetConsoleMode              = kernel32.NewProc("SetConsoleMode")
	pGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

const (
	STD_INPUT_HANDLE  = ^uintptr(9)  // -10
	STD_OUTPUT_HANDLE = ^uintptr(10) // -11

	ENABLE_PROCESSED_OUTPUT            = 0x0001
	ENABLE_WRAP_AT_EOL_OUTPUT          = 0x0002
	ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004

	ENABLE_PROCESSED_INPUT        = 0x0001
	ENABLE_LINE_INPUT             = 0x0002
	ENABLE_ECHO_INPUT             = 0x0004
	ENABLE_WINDOW_INPUT           = 0x0008
	ENABLE_MOUSE_INPUT            = 0x0010
	ENABLE_INSERT_MODE            = 0x0020
	ENABLE_QUICK_EDIT_MODE        = 0x0040
	ENABLE_EXTENDED_FLAGS         = 0x0080
	ENABLE_AUTO_POSITION          = 0x0100
	ENABLE_VIRTUAL_TERMINAL_INPUT = 0x0200
)

type coord struct {
	X int16
	Y int16
}

type smallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

var (
	origInMode  uint32
	origOutMode uint32
	hStdin      uintptr
	hStdout     uintptr
	initialized bool
)

// InitTerminal configures the Windows console for ANSI VT100 and unbuffered input
func InitTerminal() error {
	inH, _, _ := pGetStdHandle.Call(STD_INPUT_HANDLE)
	outH, _, _ := pGetStdHandle.Call(STD_OUTPUT_HANDLE)
	hStdin = inH
	hStdout = outH

	pGetConsoleMode.Call(hStdin, uintptr(unsafe.Pointer(&origInMode)))
	pGetConsoleMode.Call(hStdout, uintptr(unsafe.Pointer(&origOutMode)))

	// Enable Virtual Terminal Processing for rich ANSI output & colors
	outMode := origOutMode | ENABLE_PROCESSED_OUTPUT | ENABLE_VIRTUAL_TERMINAL_PROCESSING
	pSetConsoleMode.Call(hStdout, uintptr(outMode))

	// Raw input: disable line buffering, disable echo, enable VT input
	inMode := (origInMode &^ (ENABLE_LINE_INPUT | ENABLE_ECHO_INPUT)) | ENABLE_VIRTUAL_TERMINAL_INPUT | ENABLE_PROCESSED_INPUT
	pSetConsoleMode.Call(hStdin, uintptr(inMode))

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
	// Show cursor, leave alternate buffer
	os.Stdout.WriteString("\x1b[?25h\x1b[?1049l")

	if hStdout != 0 && origOutMode != 0 {
		pSetConsoleMode.Call(hStdout, uintptr(origOutMode))
	}
	if hStdin != 0 && origInMode != 0 {
		pSetConsoleMode.Call(hStdin, uintptr(origInMode))
	}
	initialized = false
}

// GetTerminalSize returns width and height in columns and rows
func GetTerminalSize() (int, int) {
	if hStdout == 0 {
		return 80, 24
	}
	var info consoleScreenBufferInfo
	ret, _, _ := pGetConsoleScreenBufferInfo.Call(hStdout, uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return 80, 24
	}
	w := int(info.Window.Right - info.Window.Left + 1)
	h := int(info.Window.Bottom - info.Window.Top + 1)
	if w <= 10 || h <= 5 {
		return 80, 24
	}
	return w, h
}
