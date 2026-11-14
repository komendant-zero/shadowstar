//go:build windows

package tray

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")
	modAdvapi32 = syscall.NewLazyDLL("advapi32.dll")

	procRegOpenKeyExW    = modAdvapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW = modAdvapi32.NewProc("RegQueryValueExW")
	procRegSetValueExW   = modAdvapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW  = modAdvapi32.NewProc("RegDeleteValueW")
	procRegCloseKey      = modAdvapi32.NewProc("RegCloseKey")

	procShell_NotifyIconW = modShell32.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu   = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW       = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu    = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu       = modUser32.NewProc("DestroyMenu")
	procGetCursorPos      = modUser32.NewProc("GetCursorPos")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	procPostMessageW      = modUser32.NewProc("PostMessageW")
	procDefWindowProcW    = modUser32.NewProc("DefWindowProcW")
	procRegisterClassExW  = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW   = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow     = modUser32.NewProc("DestroyWindow")
	procGetModuleHandleW  = modKernel32.NewProc("GetModuleHandleW")
	procLoadIconW         = modUser32.NewProc("LoadIconW")
	procGetMessageW       = modUser32.NewProc("GetMessageW")
	procTranslateMessage  = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW  = modUser32.NewProc("DispatchMessageW")
)

const (
	NIM_ADD        = 0x00000000
	NIM_MODIFY     = 0x00000001
	NIM_DELETE     = 0x00000002
	NIF_MESSAGE    = 0x00000001
	NIF_ICON       = 0x00000002
	NIF_TIP        = 0x00000004
	WM_USER        = 0x0400
	WM_TRAYICON    = WM_USER + 100
	WM_LBUTTONUP   = 0x0202
	WM_RBUTTONUP   = 0x0205
	WM_LBUTTONDBLCLK = 0x0203

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800
	MF_CHECKED   = 0x00000008
	MF_UNCHECKED = 0x00000000
	MF_GRAYED    = 0x00000001

	TPM_BOTTOMALIGN = 0x0020
	TPM_LEFTALIGN   = 0x0000

	IDM_STATUS     = 1001
	IDM_CONNECT    = 1002
	IDM_SYSPROXY   = 1003
	IDM_TUN        = 1004
	IDM_KILLSWITCH = 1005
	IDM_OPEN_WEB   = 1006
	IDM_AUTOSTART  = 1007
	IDM_EXIT       = 1008
)

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
}

type POINT struct {
	X int32
	Y int32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

// Handlers holds callback functions for tray actions
type Handlers struct {
	OnConnectToggle    func()
	OnSysProxyToggle   func()
	OnTunToggle        func()
	OnKillSwitchToggle func()
	OnOpenWeb          func()
	OnExit             func()

	IsConnected    func() bool
	IsSysProxy     func() bool
	IsTun          func() bool
	IsKillSwitch   func() bool
	GetActiveNode  func() string
}

// TrayApp manages the Windows system tray icon
type TrayApp struct {
	handlers  Handlers
	hwnd      uintptr
	nid       NOTIFYICONDATAW
	hicon     uintptr
	running   bool
	mu        sync.Mutex
}

var activeTrayApp *TrayApp

// New creates a new System Tray instance
func New(handlers Handlers) *TrayApp {
	app := &TrayApp{
		handlers: handlers,
	}
	activeTrayApp = app
	return app
}

// Start launches the system tray message loop in a background OS thread
func (t *TrayApp) Start() error {
	ready := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hInstance, _, _ := procGetModuleHandleW.Call(0)
		className, _ := syscall.UTF16PtrFromString("ShadowStarTrayWndClass")

		wndProc := syscall.NewCallback(trayWndProc)

		var wc WNDCLASSEXW
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		wc.LpfnWndProc = wndProc
		wc.HInstance = hInstance
		wc.LpszClassName = className

		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

		hwnd, _, err := procCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(className)),
			0, 0, 0, 0, 0,
			0, 0, hInstance, 0,
		)
		if hwnd == 0 {
			ready <- fmt.Errorf("failed to create tray window: %w", err)
			return
		}
		t.hwnd = hwnd

		// Load standard application icon (32512 = IDI_APPLICATION, 32516 = IDI_SHIELD)
		hIcon, _, _ := procLoadIconW.Call(0, 32516)
		if hIcon == 0 {
			hIcon, _, _ = procLoadIconW.Call(0, 32512)
		}
		t.hicon = hIcon

		t.nid.CbSize = uint32(unsafe.Sizeof(t.nid))
		t.nid.HWnd = hwnd
		t.nid.UID = 1
		t.nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
		t.nid.UCallbackMessage = WM_TRAYICON
		t.nid.HIcon = hIcon

		tip, _ := syscall.UTF16FromString("ShadowStar • Next-Gen Anti-DPI Proxy")
		copy(t.nid.SzTip[:], tip)

		procShell_NotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&t.nid)))
		t.running = true
		ready <- nil

		var msg MSG
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()

	return <-ready
}

// UpdateTooltip refreshes the tray hover tooltip text
func (t *TrayApp) UpdateTooltip(tipText string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}
	tip, _ := syscall.UTF16FromString(tipText)
	copy(t.nid.SzTip[:], tip)
	procShell_NotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&t.nid)))
}

// Stop removes the tray icon and terminates the window
func (t *TrayApp) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}
	t.running = false
	procShell_NotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&t.nid)))
	if t.hwnd != 0 {
		procPostMessageW.Call(t.hwnd, 0x0010 /* WM_CLOSE */, 0, 0)
	}
}

func trayWndProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	if msg == WM_TRAYICON {
		switch lParam {
		case WM_RBUTTONUP:
			if activeTrayApp != nil {
				activeTrayApp.showContextMenu()
			}
			return 0
		case WM_LBUTTONUP, WM_LBUTTONDBLCLK:
			if activeTrayApp != nil && activeTrayApp.handlers.OnOpenWeb != nil {
				activeTrayApp.handlers.OnOpenWeb()
			}
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func (t *TrayApp) showContextMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	connected := t.handlers.IsConnected != nil && t.handlers.IsConnected()
	sysproxy := t.handlers.IsSysProxy != nil && t.handlers.IsSysProxy()
	tunActive := t.handlers.IsTun != nil && t.handlers.IsTun()
	ksActive := t.handlers.IsKillSwitch != nil && t.handlers.IsKillSwitch()
	activeNode := "None"
	if t.handlers.GetActiveNode != nil {
		activeNode = t.handlers.GetActiveNode()
	}

	// 1. Status header
	statusText := fmt.Sprintf("⭐ ShadowStar: %s (%s)", map[bool]string{true: "CONNECTED", false: "IDLE"}[connected], activeNode)
	appendMenuItem(hMenu, MF_STRING|MF_GRAYED, IDM_STATUS, statusText)
	appendMenuItem(hMenu, MF_SEPARATOR, 0, "")

	// 2. Connect / Disconnect toggle
	connAction := "Подключить туннель (Connect)"
	if connected {
		connAction = "Отключить туннель (Disconnect)"
	}
	appendMenuItem(hMenu, MF_STRING, IDM_CONNECT, connAction)

	// 3. System Proxy toggle
	sysProxyFlags := uintptr(MF_STRING)
	if sysproxy {
		sysProxyFlags |= MF_CHECKED
	}
	appendMenuItem(hMenu, sysProxyFlags, IDM_SYSPROXY, "Системный прокси (WinINet)")

	// 4. L3 TUN mode toggle
	tunFlags := uintptr(MF_STRING)
	if tunActive {
		tunFlags |= MF_CHECKED
	}
	appendMenuItem(hMenu, tunFlags, IDM_TUN, "L3 TUN режим (Wintun)")

	// 5. Kill Switch toggle
	ksFlags := uintptr(MF_STRING)
	if ksActive {
		ksFlags |= MF_CHECKED
	}
	appendMenuItem(hMenu, ksFlags, IDM_KILLSWITCH, "Аварийный выключатель (Kill Switch)")

	appendMenuItem(hMenu, MF_SEPARATOR, 0, "")

	// 6. Open Web UI
	appendMenuItem(hMenu, MF_STRING, IDM_OPEN_WEB, "Открыть веб-панель (Web UI)")

	// 7. Auto-start on boot
	autoStartFlags := uintptr(MF_STRING)
	if IsAutoStartEnabled() {
		autoStartFlags |= MF_CHECKED
	}
	appendMenuItem(hMenu, autoStartFlags, IDM_AUTOSTART, "Запускать вместе с Windows")

	appendMenuItem(hMenu, MF_SEPARATOR, 0, "")

	// 8. Exit
	appendMenuItem(hMenu, MF_STRING, IDM_EXIT, "Выход (Exit)")

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(t.hwnd)

	cmd, _, _ := procTrackPopupMenu.Call(
		hMenu,
		TPM_BOTTOMALIGN|TPM_LEFTALIGN|0x0100 /* TPM_RETURNCMD */,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		t.hwnd,
		0,
	)

	switch int(cmd) {
	case IDM_CONNECT:
		if t.handlers.OnConnectToggle != nil {
			go t.handlers.OnConnectToggle()
		}
	case IDM_SYSPROXY:
		if t.handlers.OnSysProxyToggle != nil {
			go t.handlers.OnSysProxyToggle()
		}
	case IDM_TUN:
		if t.handlers.OnTunToggle != nil {
			go t.handlers.OnTunToggle()
		}
	case IDM_KILLSWITCH:
		if t.handlers.OnKillSwitchToggle != nil {
			go t.handlers.OnKillSwitchToggle()
		}
	case IDM_OPEN_WEB:
		if t.handlers.OnOpenWeb != nil {
			go t.handlers.OnOpenWeb()
		}
	case IDM_AUTOSTART:
		ToggleAutoStart()
	case IDM_EXIT:
		if t.handlers.OnExit != nil {
			t.handlers.OnExit()
		} else {
			os.Exit(0)
		}
	}
}

func appendMenuItem(hMenu uintptr, flags uintptr, id uintptr, text string) {
	strPtr, _ := syscall.UTF16PtrFromString(text)
	procAppendMenuW.Call(hMenu, flags, id, uintptr(unsafe.Pointer(strPtr)))
}

// Registry Auto-start helpers

const HKEY_CURRENT_USER = uintptr(0x80000001)
const KEY_READ = uintptr(0x20019)
const KEY_WRITE = uintptr(0x20006)
const REG_SZ = uintptr(1)

const runRegistryKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runAppName = "ShadowStar"

// IsAutoStartEnabled checks if ShadowStar is set to start on Windows logon
func IsAutoStartEnabled() bool {
	subKeyPtr, _ := syscall.UTF16PtrFromString(runRegistryKey)
	var hKey uintptr
	r, _, _ := procRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(subKeyPtr)), 0, KEY_READ, uintptr(unsafe.Pointer(&hKey)))
	if r != 0 {
		return false
	}
	defer procRegCloseKey.Call(hKey)

	valNamePtr, _ := syscall.UTF16PtrFromString(runAppName)
	r, _, _ = procRegQueryValueExW.Call(hKey, uintptr(unsafe.Pointer(valNamePtr)), 0, 0, 0, 0)
	return r == 0
}

// ToggleAutoStart toggles Windows logon autorun
func ToggleAutoStart() bool {
	subKeyPtr, _ := syscall.UTF16PtrFromString(runRegistryKey)
	valNamePtr, _ := syscall.UTF16PtrFromString(runAppName)

	if IsAutoStartEnabled() {
		var hKey uintptr
		r, _, _ := procRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(subKeyPtr)), 0, KEY_WRITE, uintptr(unsafe.Pointer(&hKey)))
		if r == 0 {
			_, _, _ = procRegDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(valNamePtr)))
			_, _, _ = procRegCloseKey.Call(hKey)
		}
		return false
	}

	exe, err := os.Executable()
	if err != nil {
		return false
	}
	cmd := fmt.Sprintf(`"%s" -headless -connect`, filepath.Clean(exe))
	cmdPtr, _ := syscall.UTF16FromString(cmd)

	var hKey uintptr
	r, _, _ := procRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(subKeyPtr)), 0, KEY_WRITE, uintptr(unsafe.Pointer(&hKey)))
	if r == 0 {
		byteLen := uintptr(len(cmdPtr) * 2)
		_, _, _ = procRegSetValueExW.Call(hKey, uintptr(unsafe.Pointer(valNamePtr)), 0, REG_SZ, uintptr(unsafe.Pointer(&cmdPtr[0])), byteLen)
		_, _, _ = procRegCloseKey.Call(hKey)
		return true
	}
	return false
}

// OpenBrowser opens the URL in the default user web browser
func OpenBrowser(url string) {
	_ = exec.Command("cmd", "/C", "start", "", url).Start()
}
