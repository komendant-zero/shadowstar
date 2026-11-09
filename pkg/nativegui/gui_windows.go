//go:build windows

package nativegui

import (
	"fmt"
	"math"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
	"github.com/shadowstar/shadowstar/pkg/i18n"
	"github.com/shadowstar/shadowstar/pkg/sysproxy"
	"github.com/shadowstar/shadowstar/pkg/updater"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	gdiplus  = syscall.NewLazyDLL("gdiplus.dll")

	pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	pGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	pGlobalLock                 = kernel32.NewProc("GlobalLock")
	pGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	pRegisterClassExW           = user32.NewProc("RegisterClassExW")
	pCreateWindowExW            = user32.NewProc("CreateWindowExW")
	pDefWindowProcW             = user32.NewProc("DefWindowProcW")
	pShowWindow                 = user32.NewProc("ShowWindow")
	pUpdateWindow               = user32.NewProc("UpdateWindow")
	pGetMessageW                = user32.NewProc("GetMessageW")
	pTranslateMessage           = user32.NewProc("TranslateMessage")
	pDispatchMessageW           = user32.NewProc("DispatchMessageW")
	pPostQuitMessage            = user32.NewProc("PostQuitMessage")
	pDestroyWindow              = user32.NewProc("DestroyWindow")
	pLoadCursorW                = user32.NewProc("LoadCursorW")
	pSetCursor                  = user32.NewProc("SetCursor")
	pBeginPaint                 = user32.NewProc("BeginPaint")
	pEndPaint                   = user32.NewProc("EndPaint")
	pInvalidateRect             = user32.NewProc("InvalidateRect")
	pGetClientRect              = user32.NewProc("GetClientRect")
	pDrawTextW                  = user32.NewProc("DrawTextW")
	pFillRect                   = user32.NewProc("FillRect")
	pSetTimer                   = user32.NewProc("SetTimer")
	pKillTimer                  = user32.NewProc("KillTimer")
	pOpenClipboard              = user32.NewProc("OpenClipboard")
	pEmptyClipboard             = user32.NewProc("EmptyClipboard")
	pSetClipboardData           = user32.NewProc("SetClipboardData")
	pGetClipboardData           = user32.NewProc("GetClipboardData")
	pCloseClipboard             = user32.NewProc("CloseClipboard")
	pIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	pGetSystemMetrics           = user32.NewProc("GetSystemMetrics")
	pCreatePopupMenu            = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                = user32.NewProc("AppendMenuW")
	pTrackPopupMenu             = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                = user32.NewProc("DestroyMenu")
	pClientToScreen             = user32.NewProc("ClientToScreen")
	pTrackMouseEvent            = user32.NewProc("TrackMouseEvent")
	pAdjustWindowRectEx         = user32.NewProc("AdjustWindowRectEx")
	pSetProcessDPIAware         = user32.NewProc("SetProcessDPIAware")
	pGetKeyState                = user32.NewProc("GetKeyState")

	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pSetBkColor             = gdi32.NewProc("SetBkColor")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pCreateFontW            = gdi32.NewProc("CreateFontW")
	pDwmSetWindowAttribute  = dwmapi.NewProc("DwmSetWindowAttribute")

	// GDI+ Procs for high-DPI vector graphics
	pGdiplusStartup       = gdiplus.NewProc("GdiplusStartup")
	pGdiplusShutdown      = gdiplus.NewProc("GdiplusShutdown")
	pGdipCreateFromHDC    = gdiplus.NewProc("GdipCreateFromHDC")
	pGdipDeleteGraphics   = gdiplus.NewProc("GdipDeleteGraphics")
	pGdipSetSmoothingMode = gdiplus.NewProc("GdipSetSmoothingMode")
	pGdipCreateSolidFill  = gdiplus.NewProc("GdipCreateSolidFill")
	pGdipDeleteBrush      = gdiplus.NewProc("GdipDeleteBrush")
	pGdipCreatePen1       = gdiplus.NewProc("GdipCreatePen1")
	pGdipDeletePen        = gdiplus.NewProc("GdipDeletePen")
	pGdipSetPenStartCap   = gdiplus.NewProc("GdipSetPenStartCap")
	pGdipSetPenEndCap     = gdiplus.NewProc("GdipSetPenEndCap")
	pGdipFillEllipseI     = gdiplus.NewProc("GdipFillEllipseI")
	pGdipDrawEllipseI     = gdiplus.NewProc("GdipDrawEllipseI")
	pGdipDrawLineI        = gdiplus.NewProc("GdipDrawLineI")
	pGdipDrawArcI         = gdiplus.NewProc("GdipDrawArcI")
	pGdipFillPolygonI     = gdiplus.NewProc("GdipFillPolygonI")
	pGdipCreatePath       = gdiplus.NewProc("GdipCreatePath")
	pGdipDeletePath       = gdiplus.NewProc("GdipDeletePath")
	pGdipAddPathArcI      = gdiplus.NewProc("GdipAddPathArcI")
	pGdipAddPathLineI     = gdiplus.NewProc("GdipAddPathLineI")
	pGdipClosePathFigure  = gdiplus.NewProc("GdipClosePathFigure")
	pGdipFillPath         = gdiplus.NewProc("GdipFillPath")
	pGdipDrawPath         = gdiplus.NewProc("GdipDrawPath")
)

type GdiplusStartupInput struct {
	GdiplusVersion           uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
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

type RECT struct {
	Left, Top, Right, Bottom int32
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

type TRACKMOUSEEVENT struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   uintptr
	DwHoverTime uint32
}

const (
	TabDashboard = 0
	TabServers   = 1
)

const (
	ModalNone = iota
	ModalSubscription
	ModalAddServer
	ModalEditServer
	ModalUpdate
)

const (
	HitNone = iota
	HitTabDashboard
	HitTabServers
	HitBtnSettings
	HitBtnLangToggle
	HitBtnUpdateBadge
	// Dashboard elements
	HitHeroButton
	HitActivePingBadge
	HitBtnSwitchServer
	HitSysProxyCard
	HitPortsCard
	// Servers tab elements
	HitBtnUpdateSub
	HitBtnImportClip
	HitBtnPingAll
	HitBtnAddServer
	HitBtnSetSubURL
	HitBtnAutoBest
	HitServerPagePrev
	HitServerPageNext
	// Modal elements
	HitModalBackdrop
	HitModalClose
	HitModalSubInput
	HitModalSubPaste
	HitModalSubSubmit
	HitModalCancel
	HitModalAddInput1
	HitModalAddInput2
	HitModalAddInput3
	HitModalAddInput4
	HitModalAddPaste
	HitModalAddSubmit
	HitModalUpdateDownload
	// Dynamic ranges
	HitServerBaseSelect = 1000
	HitServerBaseEdit   = 2000
	HitServerBaseDelete = 3000
	HitServerBasePing   = 4000
)

type AppState struct {
	client     *client.Client
	cfg        *client.Config
	cfgPath    string
	hwndMain   uintptr
	hCursorArr uintptr
	hCursorHnd uintptr

	// Reusable ClearType Fonts
	hFontBrand    uintptr
	hFontHeader   uintptr
	hFontTitle    uintptr
	hFontButton   uintptr
	hFontRegular  uintptr
	hFontSmall    uintptr
	hFontTiny     uintptr
	hFontCode     uintptr
	hFontCodeBold uintptr
	hFontBigSpeed uintptr

	// Dynamic Navigation & UI State
	currentTab      int
	connected       bool
	connecting      bool
	sysProxyEnabled bool
	hoverElement    int
	mouseTracking   bool
	toastMsg        string
	toastTimer      int
	serverPage      int

	// In-Window Modern Glass Modal State
	activeModal   int
	focusedField  int
	cursorVisible bool
	modalInput1   string // Sub URL / Name
	modalInput2   string // Server Addr
	modalInput3   string // Master Key
	modalInput4   string // SNI
	modalErrorMsg string
	editServerIdx int

	// Update State (GitHub Releases)
	hasUpdate      bool
	updateInfo     *updater.ReleaseInfo
	checkingUpdate bool

	// Telemetry
	speedDown     uint64
	speedUp       uint64
	bytesDown     uint64
	bytesUp       uint64
	activeConns   int64
	connectedTime int64
	pingStatus    string
	pingMs        int
	pingingAll    bool
	updatingSub   bool
	timerTicks    int

	mu sync.Mutex
}

var (
	gApp         *AppState
	gdiplusToken uintptr
)

func isAWGActive() bool {
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range interfaces {
		lower := strings.ToLower(iface.Name)
		if strings.Contains(lower, "amnezia") || strings.Contains(lower, "wireguard") {
			addrs, err := iface.Addrs()
			if err == nil && len(addrs) > 0 {
				return true
			}
		}
	}
	return false
}

func copyToClipboard(hwnd uintptr, text string) {
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return
	}
	size := len(utf16) * 2
	hMem, _, _ := pGlobalAlloc.Call(0x0002 /* GMEM_MOVEABLE */, uintptr(size))
	if hMem == 0 {
		return
	}
	ptr, _, _ := pGlobalLock.Call(hMem)
	if ptr == 0 {
		return
	}
	copy((*[1 << 20]byte)(unsafe.Pointer(ptr))[:size], (*[1 << 20]byte)(unsafe.Pointer(&utf16[0]))[:size])
	pGlobalUnlock.Call(hMem)

	pOpenClipboard.Call(hwnd)
	pEmptyClipboard.Call()
	pSetClipboardData.Call(13 /* CF_UNICODETEXT */, hMem)
	pCloseClipboard.Call()
}

func readClipboardText(hwnd uintptr) string {
	r, _, _ := pIsClipboardFormatAvailable.Call(13 /* CF_UNICODETEXT */)
	if r == 0 {
		return ""
	}
	r, _, _ = pOpenClipboard.Call(hwnd)
	if r == 0 {
		return ""
	}
	defer pCloseClipboard.Call()

	hMem, _, _ := pGetClipboardData.Call(13 /* CF_UNICODETEXT */)
	if hMem == 0 {
		return ""
	}
	ptr, _, _ := pGlobalLock.Call(hMem)
	if ptr == 0 {
		return ""
	}
	defer pGlobalUnlock.Call(hMem)

	var lenVal int
	for p := unsafe.Pointer(ptr); *(*uint16)(p) != 0; p = unsafe.Pointer(uintptr(p) + 2) {
		lenVal++
	}
	slice := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), lenVal)
	return syscall.UTF16ToString(slice)
}

func createFont(name string, size int, bold bool) uintptr {
	weight := 400
	if bold {
		weight = 700
	}
	fontName, _ := syscall.UTF16PtrFromString(name)
	hFont, _, _ := pCreateFontW.Call(
		uintptr(-size), 0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		1, // DEFAULT_CHARSET
		0, 0,
		5, // CLEARTYPE_QUALITY
		0,
		uintptr(unsafe.Pointer(fontName)),
	)
	return hFont
}

func formatBytesSpeed(bytes uint64) string {
	if bytes == 0 {
		return "0.0 KB/s"
	}
	val := float64(bytes)
	sizes := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	i := 0
	for val >= 1024 && i < len(sizes)-1 {
		val /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", val, sizes[i])
}

func formatTotalBytes(bytes uint64) string {
	if bytes == 0 {
		return "0.0 MB"
	}
	val := float64(bytes)
	sizes := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for val >= 1024 && i < len(sizes)-1 {
		val /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", val, sizes[i])
}

// GDI+ Drawing Helpers
func createRoundRectPath(x, y, w, h, r int32) uintptr {
	var path uintptr
	pGdipCreatePath.Call(0, uintptr(unsafe.Pointer(&path)))
	d := r * 2

	a180 := uintptr(math.Float32bits(180.0))
	a270 := uintptr(math.Float32bits(270.0))
	a0 := uintptr(math.Float32bits(0.0))
	a90 := uintptr(math.Float32bits(90.0))

	pGdipAddPathArcI.Call(path, uintptr(x), uintptr(y), uintptr(d), uintptr(d), a180, a90)
	pGdipAddPathArcI.Call(path, uintptr(x+w-d), uintptr(y), uintptr(d), uintptr(d), a270, a90)
	pGdipAddPathArcI.Call(path, uintptr(x+w-d), uintptr(y+h-d), uintptr(d), uintptr(d), a0, a90)
	pGdipAddPathArcI.Call(path, uintptr(x), uintptr(y+h-d), uintptr(d), uintptr(d), a90, a90)
	pGdipClosePathFigure.Call(path)

	return path
}

func drawSmoothCard(graphics uintptr, x, y, w, h, r int32, fillARGB, borderARGB uint32, borderWidth float32) {
	path := createRoundRectPath(x, y, w, h, r)
	defer pGdipDeletePath.Call(path)

	var brush uintptr
	pGdipCreateSolidFill.Call(uintptr(fillARGB), uintptr(unsafe.Pointer(&brush)))
	pGdipFillPath.Call(graphics, brush, path)
	pGdipDeleteBrush.Call(brush)

	if borderWidth > 0 && (borderARGB>>24) > 0 {
		var pen uintptr
		bwBits := uintptr(math.Float32bits(borderWidth))
		pGdipCreatePen1.Call(uintptr(borderARGB), bwBits, 2 /* UnitPixel */, uintptr(unsafe.Pointer(&pen)))
		pGdipDrawPath.Call(graphics, pen, path)
		pGdipDeletePen.Call(pen)
	}
}

func drawSmoothCircle(graphics uintptr, cx, cy, radius int32, fillARGB, borderARGB uint32, borderWidth float32) {
	d := radius * 2
	x := cx - radius
	y := cy - radius

	var brush uintptr
	pGdipCreateSolidFill.Call(uintptr(fillARGB), uintptr(unsafe.Pointer(&brush)))
	pGdipFillEllipseI.Call(graphics, brush, uintptr(x), uintptr(y), uintptr(d), uintptr(d))
	pGdipDeleteBrush.Call(brush)

	if borderWidth > 0 && (borderARGB>>24) > 0 {
		var pen uintptr
		bwBits := uintptr(math.Float32bits(borderWidth))
		pGdipCreatePen1.Call(uintptr(borderARGB), bwBits, 2 /* UnitPixel */, uintptr(unsafe.Pointer(&pen)))
		pGdipDrawEllipseI.Call(graphics, pen, uintptr(x), uintptr(y), uintptr(d), uintptr(d))
		pGdipDeletePen.Call(pen)
	}
}

func drawVectorPowerIcon(graphics uintptr, cx, cy, radius int32, colorARGB uint32, lineWidth float32) {
	var pen uintptr
	lwBits := uintptr(math.Float32bits(lineWidth))
	pGdipCreatePen1.Call(uintptr(colorARGB), lwBits, 2 /* UnitPixel */, uintptr(unsafe.Pointer(&pen)))
	defer pGdipDeletePen.Call(pen)

	pGdipSetPenStartCap.Call(pen, 2 /* LineCapRound */)
	pGdipSetPenEndCap.Call(pen, 2 /* LineCapRound */)

	d := radius * 2
	a300 := uintptr(math.Float32bits(300.0))
	aSweep := uintptr(math.Float32bits(300.0))
	pGdipDrawArcI.Call(graphics, pen, uintptr(cx-radius), uintptr(cy-radius), uintptr(d), uintptr(d), a300, aSweep)

	pGdipDrawLineI.Call(graphics, pen, uintptr(cx), uintptr(cy-radius-2), uintptr(cx), uintptr(cy-2))
}

func drawVectorShieldIcon(graphics uintptr, cx, cy, w, h int32, strokeARGB, fillARGB uint32, lineWidth float32) {
	var path uintptr
	pGdipCreatePath.Call(0, uintptr(unsafe.Pointer(&path)))
	defer pGdipDeletePath.Call(path)

	x := cx - w/2
	y := cy - h/2
	pGdipAddPathLineI.Call(path, uintptr(x), uintptr(y), uintptr(cx), uintptr(y+2))
	pGdipAddPathLineI.Call(path, uintptr(cx), uintptr(y+2), uintptr(x+w), uintptr(y))
	pGdipAddPathLineI.Call(path, uintptr(x+w), uintptr(y), uintptr(x+w), uintptr(y+h/2))
	pGdipAddPathLineI.Call(path, uintptr(x+w), uintptr(y+h/2), uintptr(cx), uintptr(y+h))
	pGdipAddPathLineI.Call(path, uintptr(cx), uintptr(y+h), uintptr(x), uintptr(y+h/2))
	pGdipAddPathLineI.Call(path, uintptr(x), uintptr(y+h/2), uintptr(x), uintptr(y))
	pGdipClosePathFigure.Call(path)

	if fillARGB > 0 {
		var brush uintptr
		pGdipCreateSolidFill.Call(uintptr(fillARGB), uintptr(unsafe.Pointer(&brush)))
		pGdipFillPath.Call(graphics, brush, path)
		pGdipDeleteBrush.Call(brush)
	}

	var pen uintptr
	lwBits := uintptr(math.Float32bits(lineWidth))
	pGdipCreatePen1.Call(uintptr(strokeARGB), lwBits, 2 /* UnitPixel */, uintptr(unsafe.Pointer(&pen)))
	pGdipDrawPath.Call(graphics, pen, path)
	pGdipDeletePen.Call(pen)
}

func drawVectorSettingsIcon(graphics uintptr, cx, cy int32, colorARGB uint32, lineWidth float32) {
	var pen uintptr
	lwBits := uintptr(math.Float32bits(lineWidth))
	pGdipCreatePen1.Call(uintptr(colorARGB), lwBits, 2 /* UnitPixel */, uintptr(unsafe.Pointer(&pen)))
	defer pGdipDeletePen.Call(pen)

	var brush uintptr
	pGdipCreateSolidFill.Call(uintptr(colorARGB), uintptr(unsafe.Pointer(&brush)))
	defer pGdipDeleteBrush.Call(brush)

	pGdipDrawLineI.Call(graphics, pen, uintptr(cx-6), uintptr(cy-4), uintptr(cx+6), uintptr(cy-4))
	pGdipFillEllipseI.Call(graphics, brush, uintptr(cx-3), uintptr(cy-6), 4, 4)

	pGdipDrawLineI.Call(graphics, pen, uintptr(cx-6), uintptr(cy+4), uintptr(cx+6), uintptr(cy+4))
	pGdipFillEllipseI.Call(graphics, brush, uintptr(cx+1), uintptr(cy+2), 4, 4)
}

func drawVectorLightningIcon(graphics uintptr, cx, cy int32, colorARGB uint32) {
	var brush uintptr
	pGdipCreateSolidFill.Call(uintptr(colorARGB), uintptr(unsafe.Pointer(&brush)))
	defer pGdipDeleteBrush.Call(brush)

	pts := []POINT{
		{X: cx + 1, Y: cy - 6},
		{X: cx - 4, Y: cy},
		{X: cx - 1, Y: cy},
		{X: cx - 2, Y: cy + 6},
		{X: cx + 4, Y: cy - 1},
		{X: cx + 1, Y: cy - 1},
	}
	pGdipFillPolygonI.Call(graphics, brush, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)), 0)
}

func drawSmoothToggle(graphics uintptr, x, y, w, h int32, on bool) {
	bgARGB := uint32(0xFF202A3D)
	borderARGB := uint32(0xFF334155)
	thumbARGB := uint32(0xFFFFFFFF)
	thumbRadius := (h - 6) / 2
	thumbX := x + 3 + thumbRadius

	if on {
		bgARGB = 0xFF10B981 // Emerald
		borderARGB = 0xFF059669
		thumbARGB = 0xFFFFFFFF
		thumbX = x + w - 3 - thumbRadius
	}

	drawSmoothCard(graphics, x, y, w, h, h/2, bgARGB, borderARGB, 1.2)
	drawSmoothCircle(graphics, thumbX, y+h/2, thumbRadius, thumbARGB, 0, 0)
}

func drawText(hdc uintptr, text string, x, y, w, h int32, font uintptr, colorBGR uint32, flags uint32) {
	pSelectObject.Call(hdc, font)
	pSetTextColor.Call(hdc, uintptr(colorBGR))
	pSetBkMode.Call(hdc, 1) // TRANSPARENT
	rc := RECT{Left: x, Top: y, Right: x + w, Bottom: y + h}
	ptr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(ptr)), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), uintptr(flags))
}

// Main Window Procedure
func mainWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case 0x0002: // WM_DESTROY
		pKillTimer.Call(hwnd, 1)
		_ = sysproxy.ClearSystemProxy()
		if gApp != nil && gApp.client != nil {
			_ = gApp.client.Stop()
		}
		pPostQuitMessage.Call(0)
		return 0

	case 0x0014: // WM_ERASEBKGND
		return 1 // Double buffer

	case 0x0102: // WM_CHAR
		if gApp != nil && gApp.activeModal != ModalNone {
			r := rune(wParam)
			if r >= 32 && r != 127 {
				gApp.handleModalChar(r)
				pInvalidateRect.Call(hwnd, 0, 0)
				return 0
			}
		}

	case 0x0100: // WM_KEYDOWN
		if gApp != nil && gApp.activeModal != ModalNone {
			vk := int(wParam)
			switch vk {
			case 0x1B: // VK_ESCAPE
				gApp.closeModal()
				pInvalidateRect.Call(hwnd, 0, 0)
				return 0
			case 0x0D: // VK_RETURN
				gApp.submitModal()
				pInvalidateRect.Call(hwnd, 0, 0)
				return 0
			case 0x08: // VK_BACK
				gApp.handleModalBackspace()
				pInvalidateRect.Call(hwnd, 0, 0)
				return 0
			case 0x09: // VK_TAB
				gApp.cycleModalFocus()
				pInvalidateRect.Call(hwnd, 0, 0)
				return 0
			case 0x56: // 'V' for paste
				ctrl, _, _ := pGetKeyState.Call(0x11 /* VK_CONTROL */)
				if int16(ctrl) < 0 {
					gApp.handleModalPaste()
					pInvalidateRect.Call(hwnd, 0, 0)
					return 0
				}
			}
		}

	case 0x0200: // WM_MOUSEMOVE
		x := int32(lParam & 0xFFFF)
		y := int32((lParam >> 16) & 0xFFFF)
		hit := hitTest(hwnd, x, y)

		if !gApp.mouseTracking {
			var tme TRACKMOUSEEVENT
			tme.CbSize = uint32(unsafe.Sizeof(tme))
			tme.DwFlags = 0x00000002 // TME_LEAVE
			tme.HwndTrack = hwnd
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			gApp.mouseTracking = true
		}

		if hit != HitNone && hit != HitModalBackdrop {
			pSetCursor.Call(gApp.hCursorHnd)
		} else {
			pSetCursor.Call(gApp.hCursorArr)
		}

		if hit != gApp.hoverElement {
			gApp.hoverElement = hit
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0

	case 0x02A3: // WM_MOUSELEAVE
		gApp.mouseTracking = false
		gApp.hoverElement = HitNone
		pInvalidateRect.Call(hwnd, 0, 0)
		return 0

	case 0x0202: // WM_LBUTTONUP
		x := int32(lParam & 0xFFFF)
		y := int32((lParam >> 16) & 0xFFFF)
		hit := hitTest(hwnd, x, y)

		gApp.handleHitClick(hwnd, hit, POINT{X: x, Y: y})
		return 0

	case 0x0113: // WM_TIMER
		if gApp != nil {
			gApp.cursorVisible = !gApp.cursorVisible
			gApp.timerTicks++
			if gApp.timerTicks%2 == 0 {
				gApp.updateTelemetry()
			} else if gApp.activeModal != ModalNone {
				pInvalidateRect.Call(hwnd, 0, 0)
			}
		}
		return 0

	case 0x000F: // WM_PAINT
		var ps PAINTSTRUCT
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var rc RECT
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		w := rc.Right - rc.Left
		h := rc.Bottom - rc.Top

		memDC, _, _ := pCreateCompatibleDC.Call(hdc)
		memBmp, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
		oldBmp, _, _ := pSelectObject.Call(memDC, memBmp)

		// 1. Fill Deep Obsidian Space Background #090D16
		bgBrush, _, _ := pCreateSolidBrush.Call(0x00160D09)
		pFillRect.Call(memDC, uintptr(unsafe.Pointer(&rc)), bgBrush)
		pDeleteObject.Call(bgBrush)

		// 2. Setup GDI+
		var graphics uintptr
		pGdipCreateFromHDC.Call(memDC, uintptr(unsafe.Pointer(&graphics)))
		pGdipSetSmoothingMode.Call(graphics, 2 /* SmoothingModeAntiAlias */)

		// Render Modern Desktop UI
		gApp.renderWindow(memDC, graphics, w, h)

		pGdipDeleteGraphics.Call(graphics)

		// Blit to screen
		pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), memDC, 0, 0, 0x00CC0020)
		pSelectObject.Call(memDC, oldBmp)
		pDeleteObject.Call(memBmp)
		pDeleteDC.Call(memDC)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}

	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func (app *AppState) renderWindow(hdc, graphics uintptr, w, h int32) {
	// ==========================================
	// 1. TOP HEADER & NAVIGATION BAR (y: 12..52)
	// ==========================================
	// Brand Star Icon
	drawSmoothCard(graphics, 20, 14, 28, 28, 7, 0xFF141C2B, 0xFF00E5FF, 1.2)
	drawText(hdc, "★", 20, 15, 28, 26, app.hFontHeader, 0x0000E5FF, 0x0025)

	// Brand Title
	drawText(hdc, "SHADOWSTAR", 56, 17, 120, 24, app.hFontBrand, 0x00FCFAF8, 0x0020)

	// Navigation Tabs (Centered Pills with comfortable margins)
	tabX := int32(180)
	tabW := int32(130)
	tabH := int32(32)
	tabY := int32(12)

	// Tab 1: Dashboard
	tab1Bg := uint32(0xFF131A28)
	tab1Border := uint32(0xFF222E42)
	tab1TextColor := uint32(0x0094A3B8)
	if app.currentTab == TabDashboard {
		tab1Bg = 0xFF1C283C
		tab1Border = 0xFF00E5FF
		tab1TextColor = 0x00FCFAF8
	} else if app.hoverElement == HitTabDashboard {
		tab1Bg = 0xFF182234
		tab1Border = 0xFF3B82F6
	}
	drawSmoothCard(graphics, tabX, tabY, tabW, tabH, 8, tab1Bg, tab1Border, 1.0)
	tab1Title := i18n.T("⚡ Главная", "⚡ Dashboard")
	drawText(hdc, tab1Title, tabX, tabY, tabW, tabH, app.hFontButton, tab1TextColor, 0x0025)

	// Tab 2: Servers & Subscriptions
	tab2X := tabX + tabW + 10
	tab2W := int32(165)
	tab2Bg := uint32(0xFF131A28)
	tab2Border := uint32(0xFF222E42)
	tab2TextColor := uint32(0x0094A3B8)
	if app.currentTab == TabServers {
		tab2Bg = 0xFF1C283C
		tab2Border = 0xFF00E5FF
		tab2TextColor = 0x00FCFAF8
	} else if app.hoverElement == HitTabServers {
		tab2Bg = 0xFF182234
		tab2Border = 0xFF3B82F6
	}
	drawSmoothCard(graphics, tab2X, tabY, tab2W, tabH, 8, tab2Bg, tab2Border, 1.0)
	app.mu.Lock()
	srvCount := len(app.cfg.Servers)
	hasUpd := app.hasUpdate
	app.mu.Unlock()
	tab2Label := fmt.Sprintf("%s (%d)", i18n.T("🌐 Серверы", "🌐 Servers"), srvCount)
	drawText(hdc, tab2Label, tab2X, tabY, tab2W, tabH, app.hFontButton, tab2TextColor, 0x0025)

	// Settings Gear Button (Top Right)
	setX := w - 20 - 36
	setY := int32(12)
	setBg := uint32(0xFF131A28)
	setBorder := uint32(0xFF222E42)
	if app.hoverElement == HitBtnSettings {
		setBg = 0xFF1C283C
		setBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, setX, setY, 36, tabH, 8, setBg, setBorder, 1.0)
	drawVectorSettingsIcon(graphics, setX+18, setY+16, 0xFF94A3B8, 1.5)

	// Language Switcher Button [ RU | EN ]
	langX := setX - 8 - 46
	langBg := uint32(0xFF131A28)
	langBorder := uint32(0xFF222E42)
	langColor := uint32(0x00CBD5E1)
	if app.hoverElement == HitBtnLangToggle {
		langBg = 0xFF1C283C
		langBorder = 0xFF00E5FF
		langColor = 0x0000E5FF
	}
	drawSmoothCard(graphics, langX, setY, 46, tabH, 8, langBg, langBorder, 1.0)
	langLabel := "RU"
	if i18n.CurrentLang == i18n.LangEN {
		langLabel = "EN"
	}
	drawText(hdc, langLabel, langX, setY, 46, tabH, app.hFontButton, langColor, 0x0025)

	// Update Badge (if new release on GitHub)
	if hasUpd {
		updW := int32(84)
		updX := langX - 8 - updW
		updBg := uint32(0xFF2A1C12)
		updBorder := uint32(0xFFF59E0B)
		if app.hoverElement == HitBtnUpdateBadge {
			updBg = 0xFF3D2716
			updBorder = 0xFF00E5FF
		}
		drawSmoothCard(graphics, updX, setY, updW, tabH, 8, updBg, updBorder, 1.0)
		drawText(hdc, "⚡ Update", updX, setY, updW, tabH, app.hFontSmall, 0x000B9EF5, 0x0025)
	}

	// Subtle Divider line
	var divPen uintptr
	pGdipCreatePen1.Call(0xFF1B2436, uintptr(math.Float32bits(1.0)), 2, uintptr(unsafe.Pointer(&divPen)))
	pGdipDrawLineI.Call(graphics, divPen, 20, 52, uintptr(w-20), 52)
	pGdipDeletePen.Call(divPen)

	// ==========================================
	// 2. BODY CONTENT (Based on active tab)
	// ==========================================
	if app.currentTab == TabDashboard {
		app.renderDashboard(hdc, graphics, w, h)
	} else {
		app.renderServersTab(hdc, graphics, w, h)
	}

	// ==========================================
	// 3. IN-WINDOW MODERN GLASS MODALS
	// ==========================================
	if app.activeModal != ModalNone {
		// Dim backdrop across whole window with smooth dark glass acrylic
		drawSmoothCard(graphics, 0, 0, w, h, 0, 0xD8060A14, 0, 0)

		if app.activeModal == ModalSubscription {
			app.renderSubscriptionModal(hdc, graphics, w, h)
		} else if app.activeModal == ModalAddServer {
			app.renderAddServerModal(hdc, graphics, w, h)
		} else if app.activeModal == ModalEditServer {
			app.renderEditServerModal(hdc, graphics, w, h)
		} else if app.activeModal == ModalUpdate {
			app.renderUpdateModal(hdc, graphics, w, h)
		}
	}

	// ==========================================
	// 4. FLOATING TOAST NOTIFICATION
	// ==========================================
	app.mu.Lock()
	toast := app.toastMsg
	app.mu.Unlock()
	if toast != "" {
		toastW := int32(400)
		toastH := int32(36)
		toastX := (w - toastW) / 2
		drawSmoothCard(graphics, toastX, 10, toastW, toastH, 18, 0xEE10B981, 0xFF059669, 1.2)
		drawText(hdc, toast, toastX, 10, toastW, toastH, app.hFontButton, 0x00FCFAF8, 0x0025)
	}
}

func (app *AppState) renderDashboard(hdc, graphics uintptr, w, h int32) {
	app.mu.Lock()
	isConnected := app.connected
	isConnecting := app.connecting
	connTime := app.connectedTime
	activeProfile := app.client.GetProfile()
	pingTxt := app.pingStatus
	if pingTxt == "" {
		pingTxt = i18n.T("Тест", "Ping")
	}
	pingMs := app.pingMs
	app.mu.Unlock()

	colW := (w - 50) / 2 // ~315 px

	// LEFT: Connection Power Button Card
	cardLeftX := int32(20)
	cardLeftY := int32(64)
	cardH1 := int32(216)
	drawSmoothCard(graphics, cardLeftX, cardLeftY, colW, cardH1, 14, 0xFF101726, 0xFF1D293E, 1.0)

	btnCX := cardLeftX + colW/2
	btnCY := cardLeftY + 80

	if isConnected {
		// Connected Aura (Emerald)
		drawSmoothCircle(graphics, btnCX, btnCY, 58, 0x1A10B981, 0xFF10B981, 2.0)
		drawSmoothCircle(graphics, btnCX, btnCY, 46, 0xFF10B981, 0xFF059669, 1.5)
		drawVectorPowerIcon(graphics, btnCX, btnCY, 20, 0xFFFFFFFF, 3.4)

		drawSmoothCircle(graphics, btnCX-56, cardLeftY+156, 4, 0xFF10B981, 0xFF10B981, 0)
		drawText(hdc, i18n.T("ПОДКЛЮЧЕНО", "CONNECTED"), cardLeftX, cardLeftY+146, colW, 22, app.hFontHeader, 0x0081B910, 0x0025)

		sec := time.Now().Unix() - connTime
		if sec < 0 {
			sec = 0
		}
		timeStr := fmt.Sprintf(i18n.T("Анти-DPI туннель активен • %02d:%02d:%02d", "Anti-DPI tunnel active • %02d:%02d:%02d"), sec/3600, (sec%3600)/60, sec%60)
		drawText(hdc, timeStr, cardLeftX, cardLeftY+174, colW, 18, app.hFontSmall, 0x0094A3B8, 0x0025)

	} else if isConnecting {
		// Connecting Aura (Amber)
		drawSmoothCircle(graphics, btnCX, btnCY, 58, 0x22F59E0B, 0xFFF59E0B, 2.0)
		drawSmoothCircle(graphics, btnCX, btnCY, 46, 0xFFF59E0B, 0xFFD97706, 1.5)
		drawVectorPowerIcon(graphics, btnCX, btnCY, 20, 0xFF0F141C, 3.4)

		drawSmoothCircle(graphics, btnCX-68, cardLeftY+156, 4, 0xFFF59E0B, 0xFFF59E0B, 0)
		drawText(hdc, i18n.T("ПОДКЛЮЧЕНИЕ...", "CONNECTING..."), cardLeftX, cardLeftY+146, colW, 22, app.hFontHeader, 0x000B9EF5, 0x0025)
		drawText(hdc, i18n.T("Согласование TLS 1.3 Reality...", "Handshaking TLS 1.3 Reality..."), cardLeftX, cardLeftY+174, colW, 18, app.hFontSmall, 0x00CBD5E1, 0x0025)

	} else {
		// Disconnected (Stealth Titanium)
		ringBorder := uint32(0xFF222F45)
		btnFill := uint32(0xFF141C2C)
		btnBorder := uint32(0xFF2A3A54)
		iconColor := uint32(0xFF94A3B8)
		if app.hoverElement == HitHeroButton {
			ringBorder = 0xFF00E5FF
			btnFill = 0xFF1D283E
			btnBorder = 0xFF00E5FF
			iconColor = 0xFFFFFFFF
		}
		drawSmoothCircle(graphics, btnCX, btnCY, 56, 0x00000000, ringBorder, 1.0)
		drawSmoothCircle(graphics, btnCX, btnCY, 46, btnFill, btnBorder, 1.5)
		drawVectorPowerIcon(graphics, btnCX, btnCY, 20, iconColor, 3.2)

		drawSmoothCircle(graphics, btnCX-50, cardLeftY+156, 4, 0xFF64748B, 0xFF64748B, 0)
		drawText(hdc, i18n.T("ОТКЛЮЧЕНО", "DISCONNECTED"), cardLeftX, cardLeftY+146, colW, 22, app.hFontHeader, 0x008B7464, 0x0025)
		drawText(hdc, i18n.T("Нажмите для включения защиты", "Click power button to connect"), cardLeftX, cardLeftY+174, colW, 18, app.hFontSmall, 0x0064748B, 0x0025)
	}

	// RIGHT: Active Server Overview Card
	cardRightX := cardLeftX + colW + 10
	drawSmoothCard(graphics, cardRightX, cardLeftY, colW, cardH1, 14, 0xFF101726, 0xFF1D293E, 1.0)

	// Active Server Card Title & Ping Badge
	drawText(hdc, i18n.T("ТЕКУЩИЙ УЗЕЛ", "CURRENT NODE"), cardRightX+16, cardLeftY+16, 140, 18, app.hFontTiny, 0x0064748B, 0x0020)

	// Ping Badge (Clickable with comfortable padding)
	badgeW := int32(80)
	badgeH := int32(26)
	badgeX := cardRightX + colW - 16 - badgeW
	badgeY := cardLeftY + 12
	pingBg := uint32(0xFF162032)
	pingBorder := uint32(0xFF283954)
	pingColorARGB := uint32(0xFF10B981)
	pingColorBGR := uint32(0x0081B910)
	if pingMs > 200 {
		pingColorARGB = 0xFFF59E0B
		pingColorBGR = 0x000B9EF5
	} else if pingMs == -1 {
		pingColorARGB = 0xFFEF4444
		pingColorBGR = 0x004444EF
	}
	if app.hoverElement == HitActivePingBadge {
		pingBg = 0xFF223048
		pingBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, badgeX, badgeY, badgeW, badgeH, 6, pingBg, pingBorder, 1.0)
	drawVectorLightningIcon(graphics, badgeX+12, badgeY+13, pingColorARGB)
	drawText(hdc, pingTxt, badgeX+18, badgeY, badgeW-20, badgeH, app.hFontButton, pingColorBGR, 0x0025)

	// Country Flag Pill + Server Name
	countryCode := "--"
	srvName := i18n.T("Нет доступных узлов", "No nodes configured")
	srvAddr := i18n.T("Узлы не добавлены", "Server list is empty")
	sni := "--"
	if len(app.cfg.Servers) > 0 && activeProfile.ServerAddr != "" {
		cCode, _ := config.DetectCountry(activeProfile.Name, activeProfile.ServerAddr)
		if activeProfile.Country != "" {
			cCode = activeProfile.Country
		}
		countryCode = cCode
		if activeProfile.Name != "" {
			srvName = activeProfile.Name
		} else {
			srvName = activeProfile.ServerAddr
		}
		srvAddr = activeProfile.ServerAddr
		sni = activeProfile.SNI
	}

	pillW := int32(42)
	drawSmoothCard(graphics, cardRightX+16, cardLeftY+44, pillW, 24, 6, 0x2600E5FF, 0x6600E5FF, 1.0)
	drawText(hdc, countryCode, cardRightX+16, cardLeftY+44, pillW, 24, app.hFontButton, 0x0000E5FF, 0x0025)

	drawText(hdc, srvName, cardRightX+16+pillW+10, cardLeftY+45, colW-80-pillW, 24, app.hFontHeader, 0x00FCFAF8, 0x0020)

	// Server Spec details
	drawText(hdc, i18n.T("Адрес сервера:", "Server address:"), cardRightX+16, cardLeftY+76, 105, 16, app.hFontSmall, 0x0064748B, 0x0020)
	drawText(hdc, srvAddr, cardRightX+122, cardLeftY+76, colW-137, 16, app.hFontCode, 0x00CBD5E1, 0x0020)

	drawText(hdc, i18n.T("Маскировка:", "Masking / SNI:"), cardRightX+16, cardLeftY+96, 105, 16, app.hFontSmall, 0x0064748B, 0x0020)
	drawText(hdc, sni, cardRightX+122, cardLeftY+96, colW-137, 16, app.hFontSmall, 0x0000E5FF, 0x0020)

	drawText(hdc, i18n.T("Защита:", "Protection:"), cardRightX+16, cardLeftY+116, 105, 16, app.hFontSmall, 0x0064748B, 0x0020)
	drawText(hdc, "TLS 1.3 Reality + Chunker", cardRightX+122, cardLeftY+116, colW-137, 16, app.hFontSmall, 0x0010B981, 0x0020)

	// "Выбрать другой узел" Button
	swBtnY := cardLeftY + 158
	swBtnW := colW - 32
	swBtnH := int32(38)
	swBtnBg := uint32(0xFF162032)
	swBtnBorder := uint32(0xFF283954)
	if app.hoverElement == HitBtnSwitchServer {
		swBtnBg = 0xFF223048
		swBtnBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, cardRightX+16, swBtnY, swBtnW, swBtnH, 8, swBtnBg, swBtnBorder, 1.0)
	drawText(hdc, i18n.T("🌐 Выбрать другой узел →", "🌐 Switch active node →"), cardRightX+16, swBtnY, swBtnW, swBtnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	// --- ROW 2: Telemetry Speed Cards (y: 290) ---
	row2Y := int32(290)
	row2H := int32(82)

	// Left: Download Card
	drawSmoothCard(graphics, cardLeftX, row2Y, colW, row2H, 12, 0xFF101726, 0xFF1D293E, 1.0)
	drawText(hdc, i18n.T("СКОРОСТЬ ЗАГРУЗКИ", "DOWNLOAD SPEED"), cardLeftX+16, row2Y+12, 160, 16, app.hFontTiny, 0x0064748B, 0x0020)
	speedDownStr := formatBytesSpeed(app.speedDown)
	drawText(hdc, "↓ "+speedDownStr, cardLeftX+16, row2Y+28, colW-32, 30, app.hFontBigSpeed, 0x0000E5FF, 0x0020)
	totDownStr := fmt.Sprintf(i18n.T("Всего: %s  •  Потоков: %d", "Total: %s  •  Streams: %d"), formatTotalBytes(app.bytesDown), app.activeConns)
	drawText(hdc, totDownStr, cardLeftX+16, row2Y+58, colW-32, 16, app.hFontTiny, 0x0094A3B8, 0x0020)

	// Right: Upload Card
	drawSmoothCard(graphics, cardRightX, row2Y, colW, row2H, 12, 0xFF101726, 0xFF1D293E, 1.0)
	drawText(hdc, i18n.T("СКОРОСТЬ ОТДАЧИ", "UPLOAD SPEED"), cardRightX+16, row2Y+12, 160, 16, app.hFontTiny, 0x0064748B, 0x0020)
	speedUpStr := formatBytesSpeed(app.speedUp)
	drawText(hdc, "↑ "+speedUpStr, cardRightX+16, row2Y+28, colW-32, 30, app.hFontBigSpeed, 0x0081B910, 0x0020)
	totUpStr := fmt.Sprintf(i18n.T("Всего: %s  •  RFC 7685 Anti-DPI", "Total: %s  •  RFC 7685 Anti-DPI"), formatTotalBytes(app.bytesUp))
	drawText(hdc, totUpStr, cardRightX+16, row2Y+58, colW-32, 16, app.hFontTiny, 0x0094A3B8, 0x0020)

	// --- ROW 3: System Proxy & Local Ports (y: 382) ---
	row3Y := int32(382)
	row3H := int32(76)

	// Left: System Proxy Card
	proxyBg := uint32(0xFF101726)
	proxyBorder := uint32(0xFF1D293E)
	if app.hoverElement == HitSysProxyCard {
		proxyBg = 0xFF162032
		proxyBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, cardLeftX, row3Y, colW, row3H, 12, proxyBg, proxyBorder, 1.0)
	drawVectorShieldIcon(graphics, cardLeftX+28, row3Y+38, 18, 20, 0xFF00E5FF, 0x2600E5FF, 1.4)
	drawText(hdc, i18n.T("Системный прокси Windows", "Windows System Proxy"), cardLeftX+48, row3Y+16, colW-110, 20, app.hFontTitle, 0x00FCFAF8, 0x0020)
	if isAWGActive() {
		drawText(hdc, i18n.T("⚠️ AmneziaWG активен (без конфликта)", "⚠️ AmneziaWG active (no conflict)"), cardLeftX+48, row3Y+40, colW-110, 16, app.hFontSmall, 0x000B9EF5, 0x0020)
	} else {
		drawText(hdc, i18n.T("Маршрутизация браузеров и программ", "Route browsers and system traffic"), cardLeftX+48, row3Y+40, colW-110, 16, app.hFontSmall, 0x0094A3B8, 0x0020)
	}
	drawSmoothToggle(graphics, cardLeftX+colW-58, row3Y+26, 44, 24, app.sysProxyEnabled)

	// Right: Local Ports Card
	portsBg := uint32(0xFF0C121E)
	portsBorder := uint32(0xFF1A263A)
	if app.hoverElement == HitPortsCard {
		portsBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, cardRightX, row3Y, colW, row3H, 12, portsBg, portsBorder, 1.0)
	drawText(hdc, "SOCKS5: 127.0.0.1:10808 (TCP / UDP)", cardRightX+16, row3Y+14, colW-32, 18, app.hFontCodeBold, 0x00CBD5E1, 0x0020)
	drawText(hdc, "HTTP:   127.0.0.1:10809 (IDE / System)", cardRightX+16, row3Y+32, colW-32, 18, app.hFontCodeBold, 0x00CBD5E1, 0x0020)
	drawText(hdc, i18n.T("Нажмите, чтобы скопировать адрес SOCKS5", "Click to copy local SOCKS5 address"), cardRightX+16, row3Y+52, colW-32, 14, app.hFontTiny, 0x0064748B, 0x0020)

	// --- ROW 4: Security Badges Footer (y: 472) ---
	drawText(hdc, "✓ TCP Chunker Active   •   ✓ Decoy Shield 200 OK   •   ✓ Frame Morphing   •   ✓ Zero-Logs", 20, 474, w-40, 20, app.hFontSmall, 0x0064748B, 0x0025)
}

func (app *AppState) renderServersTab(hdc, graphics uintptr, w, h int32) {
	app.mu.Lock()
	servers := make([]client.ServerProfile, len(app.cfg.Servers))
	copy(servers, app.cfg.Servers)
	activeID := app.cfg.ActiveID
	subURL := app.cfg.SubscriptionURL
	isUpdatingSub := app.updatingSub
	isPingingAll := app.pingingAll
	app.mu.Unlock()

	// --- 1. SUBSCRIPTION & ACTIONS TOOLBAR (y: 64, h: 74) ---
	barX := int32(20)
	barY := int32(64)
	barW := w - 40
	drawSmoothCard(graphics, barX, barY, barW, 74, 12, 0xFF101726, 0xFF1D293E, 1.0)

	subDisplay := subURL
	if subDisplay == "" {
		subDisplay = i18n.T("Подписка не настроена (нажмите «⚙️ URL подписки»)", "No subscription (click «⚙️ Sub URL»)")
	}
	drawText(hdc, i18n.T("Ссылка на подписку:", "Subscription URL:"), barX+16, barY+10, 130, 16, app.hFontTiny, 0x0064748B, 0x0020)
	drawText(hdc, subDisplay, barX+150, barY+10, barW-170, 16, app.hFontCode, 0x0000E5FF, 0x0020)

	// 5 Clean, evenly spaced action buttons
	btnY := barY + 32
	btnH := int32(30)

	// Button 1: Update Subscription
	b1X := barX + 10
	b1W := int32(118)
	b1Bg := uint32(0xFF162032)
	b1Border := uint32(0xFF283954)
	if app.hoverElement == HitBtnUpdateSub {
		b1Bg = 0xFF223048
		b1Border = 0xFF00E5FF
	}
	drawSmoothCard(graphics, b1X, btnY, b1W, btnH, 6, b1Bg, b1Border, 1.0)
	b1Txt := i18n.T("🔄 Обновить", "🔄 Update")
	if isUpdatingSub {
		b1Txt = i18n.T("⏳ Загрузка...", "⏳ Loading...")
	}
	drawText(hdc, b1Txt, b1X, btnY, b1W, btnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	// Button 2: Import from Clipboard
	b2X := b1X + b1W + 8
	b2W := int32(118)
	b2Bg := uint32(0xFF162032)
	b2Border := uint32(0xFF283954)
	if app.hoverElement == HitBtnImportClip {
		b2Bg = 0xFF223048
		b2Border = 0xFF00E5FF
	}
	drawSmoothCard(graphics, b2X, btnY, b2W, btnH, 6, b2Bg, b2Border, 1.0)
	drawText(hdc, i18n.T("📋 Из буфера", "📋 Paste URI"), b2X, btnY, b2W, btnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	// Button 3: Ping All Servers
	b3X := b2X + b2W + 8
	b3W := int32(112)
	b3Bg := uint32(0xFF162032)
	b3Border := uint32(0xFF283954)
	if app.hoverElement == HitBtnPingAll {
		b3Bg = 0xFF223048
		b3Border = 0xFF00E5FF
	}
	drawSmoothCard(graphics, b3X, btnY, b3W, btnH, 6, b3Bg, b3Border, 1.0)
	b3Txt := i18n.T("⚡ Пинг всех", "⚡ Ping All")
	if isPingingAll {
		b3Txt = i18n.T("⚡ Тест...", "⚡ Testing...")
	}
	drawText(hdc, b3Txt, b3X, btnY, b3W, btnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	// Button 4: Add Single Node Manually
	b4X := b3X + b3W + 8
	b4W := int32(118)
	b4Bg := uint32(0xFF162032)
	b4Border := uint32(0xFF283954)
	if app.hoverElement == HitBtnAddServer {
		b4Bg = 0xFF223048
		b4Border = 0xFF00E5FF
	}
	drawSmoothCard(graphics, b4X, btnY, b4W, btnH, 6, b4Bg, b4Border, 1.0)
	drawText(hdc, i18n.T("➕ Новый узел", "➕ New Node"), b4X, btnY, b4W, btnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	// Button 5: Setup Subscription URL
	b5X := b4X + b4W + 8
	b5W := barW - 10 - (b5X - barX)
	b5Bg := uint32(0xFF162032)
	b5Border := uint32(0xFF283954)
	if app.hoverElement == HitBtnSetSubURL {
		b5Bg = 0xFF223048
		b5Border = 0xFF00E5FF
	}
	drawSmoothCard(graphics, b5X, btnY, b5W, btnH, 6, b5Bg, b5Border, 1.0)
	drawText(hdc, i18n.T("⚙️ URL подписки", "⚙️ Sub URL"), b5X, btnY, b5W, btnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	// --- 2. SERVERS LIST (y: 148) ---
	listY := int32(148)
	cardH := int32(66)
	gap := int32(8)
	itemsPerPage := 4

	startIdx := app.serverPage * itemsPerPage
	if startIdx >= len(servers) {
		startIdx = 0
		app.serverPage = 0
	}
	endIdx := startIdx + itemsPerPage
	if endIdx > len(servers) {
		endIdx = len(servers)
	}

	for i := startIdx; i < endIdx; i++ {
		s := servers[i]
		rowY := listY + int32(i-startIdx)*(cardH+gap)
		isActive := (s.ID == activeID)

		cardBg := uint32(0xFF101726)
		cardBorder := uint32(0xFF1D293E)
		if isActive {
			cardBg = 0xFF142232
			cardBorder = 0xFF00E5FF
		} else if app.hoverElement == HitServerBaseSelect+i {
			cardBg = 0xFF162032
			cardBorder = 0xFF3B82F6
		}
		drawSmoothCard(graphics, barX, rowY, barW, cardH, 10, cardBg, cardBorder, 1.2)

		// Country Pill
		cCode, _ := config.DetectCountry(s.Name, s.ServerAddr)
		if s.Country != "" {
			cCode = s.Country
		}
		pillW := int32(42)
		drawSmoothCard(graphics, barX+14, rowY+18, pillW, 28, 6, 0x2600E5FF, 0x6600E5FF, 1.0)
		drawText(hdc, cCode, barX+14, rowY+18, pillW, 28, app.hFontButton, 0x0000E5FF, 0x0025)

		// Right-aligned Action Buttons: [ ✕ ] [ ✎ ] [ ⚡ Ping ] [ ✓ Активен / Выбрать ]
		actBW := int32(96)
		actBH := int32(32)
		actBX := barX + barW - 14 - actBW
		actBY := rowY + 17

		pingBW := int32(76)
		pingBH := int32(32)
		pingBX := actBX - 8 - pingBW
		pingBY := rowY + 17

		editBW := int32(34)
		editBH := int32(32)
		editBX := pingBX - 8 - editBW
		editBY := rowY + 17

		delBW := int32(34)
		delBH := int32(32)
		delBX := editBX - 8 - delBW
		delBY := rowY + 17

		// Name & Address with calculated bounds
		nameX := barX + 14 + pillW + 12
		maxTextW := delBX - 10 - nameX
		drawText(hdc, s.Name, nameX, rowY+13, maxTextW, 20, app.hFontTitle, 0x00FCFAF8, 0x0020)
		subAddr := fmt.Sprintf("%s • SNI: %s", s.ServerAddr, s.SNI)
		drawText(hdc, subAddr, nameX, rowY+36, maxTextW, 16, app.hFontCode, 0x0094A3B8, 0x0020)

		// 1. Selection / Active Button
		if isActive {
			drawSmoothCard(graphics, actBX, actBY, actBW, actBH, 6, 0xFF10B981, 0xFF059669, 1.0)
			drawText(hdc, i18n.T("✓ Активен", "✓ Active"), actBX, actBY, actBW, actBH, app.hFontButton, 0x00FCFAF8, 0x0025)
		} else {
			selBg := uint32(0xFF1E2A3E)
			selBorder := uint32(0xFF334664)
			if app.hoverElement == HitServerBaseSelect+i {
				selBg = 0xFF2A3C58
				selBorder = 0xFF00E5FF
			}
			drawSmoothCard(graphics, actBX, actBY, actBW, actBH, 6, selBg, selBorder, 1.0)
			drawText(hdc, i18n.T("Выбрать", "Select"), actBX, actBY, actBW, actBH, app.hFontButton, 0x00CBD5E1, 0x0025)
		}

		// 2. Ping Badge Button
		pBg := uint32(0xFF162032)
		pBorder := uint32(0xFF283954)
		pColorARGB := uint32(0xFF10B981)
		pColorBGR := uint32(0x0081B910)
		pingTxt := fmt.Sprintf("%d ms", s.Ping)
		if s.Ping == 0 {
			pingTxt = i18n.T("Тест", "Ping")
		} else if s.Ping > 200 {
			pColorARGB = 0xFFF59E0B
			pColorBGR = 0x000B9EF5
		} else if s.Ping == -1 {
			pColorARGB = 0xFFEF4444
			pColorBGR = 0x004444EF
			pingTxt = i18n.T("Таймаут", "Timeout")
		}
		if app.hoverElement == HitServerBasePing+i {
			pBg = 0xFF223048
			pBorder = 0xFF00E5FF
		}
		drawSmoothCard(graphics, pingBX, pingBY, pingBW, pingBH, 6, pBg, pBorder, 1.0)
		drawVectorLightningIcon(graphics, pingBX+10, pingBY+16, pColorARGB)
		drawText(hdc, pingTxt, pingBX+18, pingBY, pingBW-20, pingBH, app.hFontButton, pColorBGR, 0x0025)

		// 3. Edit Button [ ✎ ]
		editBg := uint32(0xFF162032)
		editBorder := uint32(0xFF283954)
		editColor := uint32(0x00CBD5E1)
		if app.hoverElement == HitServerBaseEdit+i {
			editBg = 0xFF223048
			editBorder = 0xFF00E5FF
			editColor = 0x0000E5FF
		}
		drawSmoothCard(graphics, editBX, editBY, editBW, editBH, 6, editBg, editBorder, 1.0)
		drawText(hdc, "✎", editBX, editBY, editBW, editBH, app.hFontButton, editColor, 0x0025)

		// 4. Delete Button [ ✕ ]
		delBg := uint32(0xFF162032)
		delBorder := uint32(0xFF283954)
		delColor := uint32(0x0094A3B8)
		if app.hoverElement == HitServerBaseDelete+i {
			delBg = 0xFF2A1520
			delBorder = 0xFFEF4444
			delColor = 0x004444EF
		}
		drawSmoothCard(graphics, delBX, delBY, delBW, delBH, 6, delBg, delBorder, 1.0)
		drawText(hdc, "✕", delBX, delBY, delBW, delBH, app.hFontButton, delColor, 0x0025)
	}

	if len(servers) == 0 {
		emptyH := int32(200)
		drawSmoothCard(graphics, barX, listY, barW, emptyH, 12, 0xFF101726, 0xFF1D293E, 1.0)
		drawText(hdc, "🌐", barX, listY+35, barW, 36, app.hFontBigSpeed, 0x0000E5FF, 0x0025)
		drawText(hdc, i18n.T("Список узлов пуст", "Node list is empty"), barX, listY+80, barW, 22, app.hFontHeader, 0x00FCFAF8, 0x0025)
		drawText(hdc, i18n.T("Нажмите «+ Новый узел» или укажите ссылку на подписку", "Click «+ New Node» or enter subscription URL"), barX, listY+112, barW, 18, app.hFontSmall, 0x0094A3B8, 0x0025)
	}

	// --- 3. BOTTOM FOOTER TOOLBAR (y: 450) ---
	footY := int32(450)

	// Button: Auto-Select Fastest Node (Width 225px, text fits easily)
	autoBW := int32(225)
	autoBH := int32(36)
	autoBg := uint32(0xFF162032)
	autoBorder := uint32(0xFF283954)
	if app.hoverElement == HitBtnAutoBest {
		autoBg = 0xFF223048
		autoBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, barX, footY, autoBW, autoBH, 8, autoBg, autoBorder, 1.0)
	drawText(hdc, i18n.T("⚡ Выбрать самый быстрый", "⚡ Auto-Select Fastest"), barX, footY, autoBW, autoBH, app.hFontButton, 0x0000E5FF, 0x0025)

	// Pagination if > 4 servers
	if len(servers) > itemsPerPage {
		maxPages := (len(servers) + itemsPerPage - 1) / itemsPerPage
		pageStr := fmt.Sprintf(i18n.T("Страница %d / %d", "Page %d / %d"), app.serverPage+1, maxPages)

		prevX := barX + autoBW + 15
		drawSmoothCard(graphics, prevX, footY, 36, autoBH, 6, 0xFF162032, 0xFF283954, 1.0)
		drawText(hdc, "◄", prevX, footY, 36, autoBH, app.hFontTitle, 0x00FCFAF8, 0x0025)

		drawText(hdc, pageStr, prevX+40, footY, 110, autoBH, app.hFontSmall, 0x0094A3B8, 0x0025)

		nextX := prevX + 40 + 110
		drawSmoothCard(graphics, nextX, footY, 36, autoBH, 6, 0xFF162032, 0xFF283954, 1.0)
		drawText(hdc, "►", nextX, footY, 36, autoBH, app.hFontTitle, 0x00FCFAF8, 0x0025)
	}

	// Server count label right-aligned
	countStr := fmt.Sprintf(i18n.T("Всего узлов: %d", "Total nodes: %d"), len(servers))
	drawText(hdc, countStr, barX+barW-180, footY, 180, autoBH, app.hFontSmall, 0x0064748B, 0x0026)
}

// =========================================================================
// 3. IN-WINDOW MODERN GLASS MODALS (SUBSCRIPTION & ADD SERVER)
// =========================================================================

func (app *AppState) renderSubscriptionModal(hdc, graphics uintptr, w, h int32) {
	mW := int32(520)
	mH := int32(260)
	mX := (w - mW) / 2
	mY := (h - mH) / 2

	// Glass Modal Card
	drawSmoothCard(graphics, mX, mY, mW, mH, 14, 0xFF101726, 0xFF00E5FF, 1.2)

	// Header: Icon + Title
	drawText(hdc, i18n.T("🌐 Ссылка на подписку", "🌐 Subscription URL"), mX+24, mY+20, 320, 24, app.hFontHeader, 0x00FCFAF8, 0x0020)

	// Close Button ✕
	closeX := mX + mW - 44
	closeY := mY + 16
	closeColor := uint32(0x0094A3B8)
	if app.hoverElement == HitModalClose {
		closeColor = 0x004444EF
	}
	drawText(hdc, "✕", closeX, closeY, 28, 28, app.hFontHeader, closeColor, 0x0025)

	// Subtitle
	drawText(hdc, i18n.T("Введите HTTPS-ссылку для регулярного автообновления списка узлов:", "Enter HTTPS subscription URL for automatic node updates:"), mX+24, mY+52, mW-48, 18, app.hFontSmall, 0x0094A3B8, 0x0020)

	// Input Box
	inX := mX + 24
	inY := mY + 76
	inW := mW - 48
	inH := int32(38)
	inBorder := uint32(0xFF283954)
	inBg := uint32(0xFF141C2B)
	if app.focusedField == 1 {
		inBorder = 0xFF00E5FF
		inBg = 0xFF182234
	}
	drawSmoothCard(graphics, inX, inY, inW, inH, 8, inBg, inBorder, 1.2)

	dispText := app.modalInput1
	if dispText == "" && app.focusedField != 1 {
		drawText(hdc, "https://your-domain.com/sub/nodes.txt", inX+14, inY, inW-28, inH, app.hFontSmall, 0x0064748B, 0x0024)
	} else {
		if app.focusedField == 1 && app.cursorVisible {
			dispText += "|"
		}
		drawText(hdc, dispText, inX+14, inY, inW-28, inH, app.hFontCode, 0x0000E5FF, 0x0024)
	}

	// Button: Paste from Clipboard
	pasteX := inX
	pasteY := inY + inH + 12
	pasteW := int32(185)
	pasteH := int32(28)
	pasteBg := uint32(0xFF162032)
	pasteBorder := uint32(0xFF283954)
	if app.hoverElement == HitModalSubPaste {
		pasteBg = 0xFF223048
		pasteBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, pasteX, pasteY, pasteW, pasteH, 6, pasteBg, pasteBorder, 1.0)
	drawText(hdc, i18n.T("📋 Вставить из буфера", "📋 Paste from clipboard"), pasteX, pasteY, pasteW, pasteH, app.hFontSmall, 0x00CBD5E1, 0x0025)

	// Error message if any
	if app.modalErrorMsg != "" {
		drawText(hdc, app.modalErrorMsg, pasteX+pasteW+15, pasteY+4, inW-pasteW-20, 22, app.hFontSmall, 0x004444EF, 0x0020)
	}

	// Bottom Buttons
	btnRowY := mY + mH - 52
	subBtnW := int32(210)
	subBtnH := int32(38)
	subBtnX := mX + mW - 24 - subBtnW
	drawSmoothCard(graphics, subBtnX, btnRowY, subBtnW, subBtnH, 8, 0xFF10B981, 0xFF059669, 1.2)
	drawText(hdc, i18n.T("✓ Загрузить и обновить", "✓ Save & Update"), subBtnX, btnRowY, subBtnW, subBtnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	cancelW := int32(100)
	cancelX := subBtnX - 10 - cancelW
	cancelBg := uint32(0xFF162032)
	cancelBorder := uint32(0xFF283954)
	if app.hoverElement == HitModalCancel {
		cancelBg = 0xFF223048
		cancelBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, cancelX, btnRowY, cancelW, subBtnH, 8, cancelBg, cancelBorder, 1.0)
	drawText(hdc, i18n.T("Отмена", "Cancel"), cancelX, btnRowY, cancelW, subBtnH, app.hFontButton, 0x0094A3B8, 0x0025)
}

func (app *AppState) renderAddServerModal(hdc, graphics uintptr, w, h int32) {
	mW := int32(520)
	mH := int32(430)
	mX := (w - mW) / 2
	mY := (h - mH) / 2

	// Glass Modal Card
	drawSmoothCard(graphics, mX, mY, mW, mH, 14, 0xFF101726, 0xFF00E5FF, 1.2)

	// Header: Icon + Title
	drawText(hdc, i18n.T("➕ Добавление узла ShadowStar", "➕ Add ShadowStar Node"), mX+24, mY+18, 380, 24, app.hFontHeader, 0x00FCFAF8, 0x0020)

	// Close Button ✕
	closeX := mX + mW - 44
	closeY := mY + 16
	closeColor := uint32(0x0094A3B8)
	if app.hoverElement == HitModalClose {
		closeColor = 0x004444EF
	}
	drawText(hdc, "✕", closeX, closeY, 28, 28, app.hFontHeader, closeColor, 0x0025)

	inW := mW - 48
	inH := int32(32)

	renderInput := func(label string, val string, fIdx int, y int32, placeholder string) {
		drawText(hdc, label, mX+24, y-18, inW, 16, app.hFontTiny, 0x0094A3B8, 0x0020)
		inBorder := uint32(0xFF283954)
		inBg := uint32(0xFF141C2B)
		if app.focusedField == fIdx {
			inBorder = 0xFF00E5FF
			inBg = 0xFF182234
		}
		drawSmoothCard(graphics, mX+24, y, inW, inH, 6, inBg, inBorder, 1.0)

		disp := val
		if disp == "" && app.focusedField != fIdx {
			drawText(hdc, placeholder, mX+36, y, inW-24, inH, app.hFontSmall, 0x0064748B, 0x0024)
		} else {
			if app.focusedField == fIdx && app.cursorVisible {
				disp += "|"
			}
			drawText(hdc, disp, mX+36, y, inW-24, inH, app.hFontCode, 0x00FCFAF8, 0x0024)
		}
	}

	renderInput(i18n.T("Название узла:", "Node Name:"), app.modalInput1, 1, mY+68, i18n.T("Например: VPS Нидерланды #1", "e.g.: VPS Netherlands #1"))
	renderInput(i18n.T("Адрес и порт сервера (IP:Port):", "Server Address (IP:Port):"), app.modalInput2, 2, mY+132, "185.120.40.10:4433")
	renderInput(i18n.T("Мастер-ключ безопасности:", "Security Master Key:"), app.modalInput3, 3, mY+196, i18n.T("Секретный токен узла", "Secret node master key"))
	renderInput(i18n.T("Маскировка SNI (Decoy Domain):", "SNI Masking (Decoy Domain):"), app.modalInput4, 4, mY+260, "www.microsoft.com")

	// Quick action: paste shadowstar:// link
	pasteX := mX + 24
	pasteY := mY + 306
	pasteW := int32(245)
	pasteH := int32(28)
	pasteBg := uint32(0xFF162032)
	pasteBorder := uint32(0xFF283954)
	if app.hoverElement == HitModalAddPaste {
		pasteBg = 0xFF223048
		pasteBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, pasteX, pasteY, pasteW, pasteH, 6, pasteBg, pasteBorder, 1.0)
	drawText(hdc, i18n.T("📋 Вставить ссылку shadowstar://", "📋 Paste shadowstar:// URI"), pasteX, pasteY, pasteW, pasteH, app.hFontSmall, 0x00CBD5E1, 0x0025)

	if app.modalErrorMsg != "" {
		drawText(hdc, app.modalErrorMsg, pasteX+pasteW+15, pasteY+4, inW-pasteW-20, 22, app.hFontSmall, 0x004444EF, 0x0020)
	}

	// Bottom Buttons
	btnRowY := mY + mH - 52
	saveBtnW := int32(180)
	saveBtnH := int32(38)
	saveBtnX := mX + mW - 24 - saveBtnW
	drawSmoothCard(graphics, saveBtnX, btnRowY, saveBtnW, saveBtnH, 8, 0xFF10B981, 0xFF059669, 1.2)
	drawText(hdc, i18n.T("✓ Сохранить узел", "✓ Add Server"), saveBtnX, btnRowY, saveBtnW, saveBtnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	cancelW := int32(100)
	cancelX := saveBtnX - 10 - cancelW
	cancelBg := uint32(0xFF162032)
	cancelBorder := uint32(0xFF283954)
	if app.hoverElement == HitModalCancel {
		cancelBg = 0xFF223048
		cancelBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, cancelX, btnRowY, cancelW, saveBtnH, 8, cancelBg, cancelBorder, 1.0)
	drawText(hdc, i18n.T("Отмена", "Cancel"), cancelX, btnRowY, cancelW, saveBtnH, app.hFontButton, 0x0094A3B8, 0x0025)
}

func (app *AppState) renderEditServerModal(hdc, graphics uintptr, w, h int32) {
	mW := int32(520)
	mH := int32(430)
	mX := (w - mW) / 2
	mY := (h - mH) / 2

	// Glass Modal Card
	drawSmoothCard(graphics, mX, mY, mW, mH, 14, 0xFF101726, 0xFF00E5FF, 1.2)

	// Header: Icon + Title
	drawText(hdc, i18n.T("✏️ Редактирование узла ShadowStar", "✏️ Edit ShadowStar Node"), mX+24, mY+18, 380, 24, app.hFontHeader, 0x00FCFAF8, 0x0020)

	// Close Button ✕
	closeX := mX + mW - 44
	closeY := mY + 16
	closeColor := uint32(0x0094A3B8)
	if app.hoverElement == HitModalClose {
		closeColor = 0x004444EF
	}
	drawText(hdc, "✕", closeX, closeY, 28, 28, app.hFontHeader, closeColor, 0x0025)

	inW := mW - 48
	inH := int32(32)

	renderInput := func(label string, val string, fIdx int, y int32, placeholder string) {
		drawText(hdc, label, mX+24, y-18, inW, 16, app.hFontTiny, 0x0094A3B8, 0x0020)
		inBorder := uint32(0xFF283954)
		inBg := uint32(0xFF141C2B)
		if app.focusedField == fIdx {
			inBorder = 0xFF00E5FF
			inBg = 0xFF182234
		}
		drawSmoothCard(graphics, mX+24, y, inW, inH, 6, inBg, inBorder, 1.0)

		disp := val
		if disp == "" && app.focusedField != fIdx {
			drawText(hdc, placeholder, mX+36, y, inW-24, inH, app.hFontSmall, 0x0064748B, 0x0024)
		} else {
			if app.focusedField == fIdx && app.cursorVisible {
				disp += "|"
			}
			drawText(hdc, disp, mX+36, y, inW-24, inH, app.hFontCode, 0x00FCFAF8, 0x0024)
		}
	}

	renderInput(i18n.T("Название узла:", "Node Name:"), app.modalInput1, 1, mY+68, i18n.T("Например: VPS Нидерланды #1", "e.g.: VPS Netherlands #1"))
	renderInput(i18n.T("Адрес и порт сервера (IP:Port):", "Server Address (IP:Port):"), app.modalInput2, 2, mY+132, "185.120.40.10:4433")
	renderInput(i18n.T("Мастер-ключ безопасности:", "Security Master Key:"), app.modalInput3, 3, mY+196, i18n.T("Секретный токен узла", "Secret node master key"))
	renderInput(i18n.T("Маскировка SNI (Decoy Domain):", "SNI Masking (Decoy Domain):"), app.modalInput4, 4, mY+260, "www.microsoft.com")

	// Quick action: paste shadowstar:// link
	pasteX := mX + 24
	pasteY := mY + 306
	pasteW := int32(245)
	pasteH := int32(28)
	pasteBg := uint32(0xFF162032)
	pasteBorder := uint32(0xFF283954)
	if app.hoverElement == HitModalAddPaste {
		pasteBg = 0xFF223048
		pasteBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, pasteX, pasteY, pasteW, pasteH, 6, pasteBg, pasteBorder, 1.0)
	drawText(hdc, i18n.T("📋 Вставить ссылку shadowstar://", "📋 Paste shadowstar:// URI"), pasteX, pasteY, pasteW, pasteH, app.hFontSmall, 0x00CBD5E1, 0x0025)

	if app.modalErrorMsg != "" {
		drawText(hdc, app.modalErrorMsg, pasteX+pasteW+15, pasteY+4, inW-pasteW-20, 22, app.hFontSmall, 0x004444EF, 0x0020)
	}

	// Bottom Buttons
	btnRowY := mY + mH - 52
	saveBtnW := int32(190)
	saveBtnH := int32(38)
	saveBtnX := mX + mW - 24 - saveBtnW
	drawSmoothCard(graphics, saveBtnX, btnRowY, saveBtnW, saveBtnH, 8, 0xFF10B981, 0xFF059669, 1.2)
	drawText(hdc, i18n.T("✓ Сохранить изменения", "✓ Save Changes"), saveBtnX, btnRowY, saveBtnW, saveBtnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	cancelW := int32(100)
	cancelX := saveBtnX - 10 - cancelW
	cancelBg := uint32(0xFF162032)
	cancelBorder := uint32(0xFF283954)
	if app.hoverElement == HitModalCancel {
		cancelBg = 0xFF223048
		cancelBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, cancelX, btnRowY, cancelW, saveBtnH, 8, cancelBg, cancelBorder, 1.0)
	drawText(hdc, i18n.T("Отмена", "Cancel"), cancelX, btnRowY, cancelW, saveBtnH, app.hFontButton, 0x0094A3B8, 0x0025)
}

func (app *AppState) renderUpdateModal(hdc, graphics uintptr, w, h int32) {
	mW := int32(520)
	mH := int32(320)
	mX := (w - mW) / 2
	mY := (h - mH) / 2

	// Glass Modal Card with Amber/Golden Accent
	drawSmoothCard(graphics, mX, mY, mW, mH, 14, 0xFF101726, 0xFFF59E0B, 1.4)

	// Header: Icon + Title
	title := i18n.T("⚡ Доступно обновление ShadowStar!", "⚡ ShadowStar Update Available!")
	drawText(hdc, title, mX+24, mY+20, 420, 24, app.hFontHeader, 0x000B9EF5, 0x0020)

	// Close Button ✕
	closeX := mX + mW - 44
	closeY := mY + 16
	closeColor := uint32(0x0094A3B8)
	if app.hoverElement == HitModalClose {
		closeColor = 0x004444EF
	}
	drawText(hdc, "✕", closeX, closeY, 28, 28, app.hFontHeader, closeColor, 0x0025)

	app.mu.Lock()
	info := app.updateInfo
	app.mu.Unlock()

	tag := "v1.3.0"
	pub := "GitHub Releases"
	notes := i18n.T("Вышла новая версия приложения с улучшениями стабильности и новыми функциями.", "A new version with performance and stability improvements is available.")
	if info != nil {
		if info.TagName != "" {
			tag = info.TagName
		}
		if info.PublishedAt != "" {
			pub = info.PublishedAt
			if len(pub) > 10 {
				pub = pub[:10]
			}
		}
		if info.Body != "" {
			notes = strings.ReplaceAll(info.Body, "\r\n", " ")
			notes = strings.ReplaceAll(notes, "\n", " ")
			if len(notes) > 180 {
				notes = notes[:177] + "..."
			}
		}
	}

	boxY := mY + 58
	boxH := int32(160)
	drawSmoothCard(graphics, mX+24, boxY, mW-48, boxH, 8, 0xFF141C2B, 0xFF283954, 1.0)

	curVerStr := fmt.Sprintf("%s: %s   ➔   %s: %s",
		i18n.T("Текущая версия", "Current version"), updater.CurrentVersion,
		i18n.T("Новая версия", "New version"), tag)
	drawText(hdc, curVerStr, mX+38, boxY+14, mW-76, 20, app.hFontTitle, 0x0000E5FF, 0x0020)

	pubStr := fmt.Sprintf("%s: %s", i18n.T("Дата релиза", "Release date"), pub)
	drawText(hdc, pubStr, mX+38, boxY+40, mW-76, 16, app.hFontSmall, 0x0094A3B8, 0x0020)

	drawText(hdc, i18n.T("Описание изменений:", "Changelog:"), mX+38, boxY+70, mW-76, 16, app.hFontSmall, 0x00CBD5E1, 0x0020)
	drawText(hdc, notes, mX+38, boxY+92, mW-76, 50, app.hFontSmall, 0x0094A3B8, 0x0020)

	// Buttons
	btnRowY := mY + mH - 52
	dlBtnW := int32(220)
	dlBtnH := int32(38)
	dlBtnX := mX + mW - 24 - dlBtnW
	dlBg := uint32(0xFF10B981)
	dlBorder := uint32(0xFF059669)
	if app.hoverElement == HitModalUpdateDownload {
		dlBg = 0xFF059669
		dlBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, dlBtnX, btnRowY, dlBtnW, dlBtnH, 8, dlBg, dlBorder, 1.2)
	drawText(hdc, i18n.T("⬇ Скачать обновление", "⬇ Download Update"), dlBtnX, btnRowY, dlBtnW, dlBtnH, app.hFontButton, 0x00FCFAF8, 0x0025)

	cancelW := int32(110)
	cancelX := dlBtnX - 12 - cancelW
	cancelBg := uint32(0xFF162032)
	cancelBorder := uint32(0xFF283954)
	if app.hoverElement == HitModalCancel {
		cancelBg = 0xFF223048
		cancelBorder = 0xFF00E5FF
	}
	drawSmoothCard(graphics, cancelX, btnRowY, cancelW, dlBtnH, 8, cancelBg, cancelBorder, 1.0)
	drawText(hdc, i18n.T("Позже", "Later"), cancelX, btnRowY, cancelW, dlBtnH, app.hFontButton, 0x0094A3B8, 0x0025)
}

func (app *AppState) openUpdateModal() {
	app.mu.Lock()
	app.activeModal = ModalUpdate
	app.mu.Unlock()
	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) checkGitHubUpdate(showToast bool) {
	app.mu.Lock()
	if app.checkingUpdate {
		app.mu.Unlock()
		return
	}
	app.checkingUpdate = true
	app.mu.Unlock()

	defer func() {
		app.mu.Lock()
		app.checkingUpdate = false
		app.mu.Unlock()
		pInvalidateRect.Call(app.hwndMain, 0, 0)
	}()

	rel, err := updater.CheckUpdate("")
	if err != nil {
		if showToast {
			app.showToast(i18n.T("Не удалось проверить обновления", "Failed to check for updates"))
		}
		return
	}

	app.mu.Lock()
	app.updateInfo = rel
	app.hasUpdate = (rel != nil)
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)

	if rel != nil {
		app.showToast(fmt.Sprintf(i18n.T("Доступна новая версия %s!", "New version %s is available!"), rel.TagName))
		app.openUpdateModal()
	} else if showToast {
		app.showToast(i18n.T("У вас последняя версия!", "You have the latest version!"))
	}
}

func (app *AppState) openSubscriptionModal() {
	app.mu.Lock()
	app.activeModal = ModalSubscription
	app.focusedField = 1
	app.modalInput1 = app.cfg.SubscriptionURL
	app.modalErrorMsg = ""
	app.mu.Unlock()
	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) openAddServerModal() {
	app.mu.Lock()
	app.activeModal = ModalAddServer
	app.focusedField = 1
	app.modalInput1 = ""
	app.modalInput2 = ""
	app.modalInput3 = ""
	app.modalInput4 = "www.microsoft.com"
	app.modalErrorMsg = ""
	app.mu.Unlock()
	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) openEditServerModal(idx int) {
	app.mu.Lock()
	if idx < 0 || idx >= len(app.cfg.Servers) {
		app.mu.Unlock()
		return
	}
	s := app.cfg.Servers[idx]
	app.activeModal = ModalEditServer
	app.editServerIdx = idx
	app.focusedField = 1
	app.modalInput1 = s.Name
	app.modalInput2 = s.ServerAddr
	app.modalInput3 = s.MasterKey
	app.modalInput4 = s.SNI
	app.modalErrorMsg = ""
	app.mu.Unlock()
	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) closeModal() {
	app.mu.Lock()
	app.activeModal = ModalNone
	app.modalErrorMsg = ""
	app.mu.Unlock()
	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) handleModalChar(r rune) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.activeModal == ModalSubscription {
		app.modalInput1 += string(r)
	} else if app.activeModal == ModalAddServer || app.activeModal == ModalEditServer {
		switch app.focusedField {
		case 1:
			app.modalInput1 += string(r)
		case 2:
			app.modalInput2 += string(r)
		case 3:
			app.modalInput3 += string(r)
		case 4:
			app.modalInput4 += string(r)
		}
	}
}

func (app *AppState) handleModalBackspace() {
	app.mu.Lock()
	defer app.mu.Unlock()
	trimLast := func(s string) string {
		runes := []rune(s)
		if len(runes) > 0 {
			return string(runes[:len(runes)-1])
		}
		return ""
	}
	if app.activeModal == ModalSubscription {
		app.modalInput1 = trimLast(app.modalInput1)
	} else if app.activeModal == ModalAddServer || app.activeModal == ModalEditServer {
		switch app.focusedField {
		case 1:
			app.modalInput1 = trimLast(app.modalInput1)
		case 2:
			app.modalInput2 = trimLast(app.modalInput2)
		case 3:
			app.modalInput3 = trimLast(app.modalInput3)
		case 4:
			app.modalInput4 = trimLast(app.modalInput4)
		}
	}
}

func (app *AppState) cycleModalFocus() {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.activeModal == ModalSubscription {
		app.focusedField = 1
	} else if app.activeModal == ModalAddServer || app.activeModal == ModalEditServer {
		app.focusedField++
		if app.focusedField > 4 {
			app.focusedField = 1
		}
	}
}

func (app *AppState) handleModalPaste() {
	text := readClipboardText(app.hwndMain)
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.activeModal == ModalSubscription {
		app.modalInput1 = text
	} else if app.activeModal == ModalAddServer || app.activeModal == ModalEditServer {
		// If pasted full shadowstar:// link, auto-populate all fields!
		if strings.HasPrefix(text, "shadowstar://") {
			prof, err := config.DecodeURIToProfile(text)
			if err == nil && prof != nil {
				app.modalInput1 = prof.Name
				app.modalInput2 = prof.ServerAddr
				app.modalInput3 = prof.MasterKey
				if prof.SNI != "" {
					app.modalInput4 = prof.SNI
				}
				return
			}
		}
		switch app.focusedField {
		case 1:
			app.modalInput1 = text
		case 2:
			app.modalInput2 = text
		case 3:
			app.modalInput3 = text
		case 4:
			app.modalInput4 = text
		}
	}
}

func (app *AppState) submitModal() {
	app.mu.Lock()
	if app.activeModal == ModalSubscription {
		val := strings.TrimSpace(app.modalInput1)
		if val == "" {
			app.modalErrorMsg = "Введите корректный URL!"
			app.mu.Unlock()
			return
		}
		app.cfg.SubscriptionURL = val
		_ = config.SaveClientConfig(app.cfgPath, app.cfg)
		app.activeModal = ModalNone
		app.mu.Unlock()
		app.showToast("✓ URL сохранен! Загружаем...")
		go app.updateSubscriptionFromURL()
		return
	} else if app.activeModal == ModalAddServer {
		name := strings.TrimSpace(app.modalInput1)
		addr := strings.TrimSpace(app.modalInput2)
		key := strings.TrimSpace(app.modalInput3)
		sni := strings.TrimSpace(app.modalInput4)

		if addr == "" || key == "" {
			app.modalErrorMsg = "Заполните адрес и мастер-ключ!"
			app.mu.Unlock()
			return
		}
		if name == "" {
			name = addr
		}
		if sni == "" {
			sni = "www.microsoft.com"
		}
		cCode, _ := config.DetectCountry(name, addr)

		newProf := client.ServerProfile{
			ID:            fmt.Sprintf("node-%d", time.Now().UnixNano()),
			Name:          name,
			ServerAddr:    addr,
			MasterKey:     key,
			SNI:           sni,
			EnableChunker: true,
			Country:       cCode,
		}

		app.cfg.Servers = append(app.cfg.Servers, newProf)
		app.cfg.ActiveID = newProf.ID
		app.client.UpdateProfile(newProf)
		_ = config.SaveClientConfig(app.cfgPath, app.cfg)
		app.activeModal = ModalNone
		app.toastMsg = "✓ Новый узел добавлен и выбран!"
		app.toastTimer = 3
		app.mu.Unlock()

		pInvalidateRect.Call(app.hwndMain, 0, 0)
		return
	} else if app.activeModal == ModalEditServer {
		name := strings.TrimSpace(app.modalInput1)
		addr := strings.TrimSpace(app.modalInput2)
		key := strings.TrimSpace(app.modalInput3)
		sni := strings.TrimSpace(app.modalInput4)

		if addr == "" || key == "" {
			app.modalErrorMsg = "Заполните адрес и мастер-ключ!"
			app.mu.Unlock()
			return
		}
		if name == "" {
			name = addr
		}
		if sni == "" {
			sni = "www.microsoft.com"
		}
		cCode, _ := config.DetectCountry(name, addr)

		idx := app.editServerIdx
		if idx >= 0 && idx < len(app.cfg.Servers) {
			prof := &app.cfg.Servers[idx]
			prof.Name = name
			prof.ServerAddr = addr
			prof.MasterKey = key
			prof.SNI = sni
			prof.Country = cCode
			prof.Ping = 0 // reset for retest

			if app.cfg.ActiveID == prof.ID {
				app.client.UpdateProfile(*prof)
			}
			_ = config.SaveClientConfig(app.cfgPath, app.cfg)
			app.activeModal = ModalNone
			app.toastMsg = "✓ Параметры узла обновлены!"
			app.toastTimer = 3
			app.mu.Unlock()

			pInvalidateRect.Call(app.hwndMain, 0, 0)
			go app.measureActiveLatency()
			return
		}
		app.activeModal = ModalNone
		app.mu.Unlock()
		pInvalidateRect.Call(app.hwndMain, 0, 0)
		return
	}
	app.mu.Unlock()
}

func hitTest(hwnd uintptr, x, y int32) int {
	var rc RECT
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top

	// Check Modal Hit Testing First
	if gApp.activeModal != ModalNone {
		if gApp.activeModal == ModalSubscription {
			mW := int32(520)
			mH := int32(260)
			mX := (w - mW) / 2
			mY := (h - mH) / 2

			// Close button ✕
			if x >= mX+mW-44 && x <= mX+mW-16 && y >= mY+16 && y <= mY+44 {
				return HitModalClose
			}
			// Input Box
			if x >= mX+24 && x <= mX+mW-24 && y >= mY+76 && y <= mY+114 {
				return HitModalSubInput
			}
			// Paste button
			if x >= mX+24 && x <= mX+24+175 && y >= mY+126 && y <= mY+154 {
				return HitModalSubPaste
			}
			// Submit button
			subBtnW := int32(210)
			subBtnX := mX + mW - 24 - subBtnW
			if x >= subBtnX && x <= subBtnX+subBtnW && y >= mY+mH-52 && y <= mY+mH-14 {
				return HitModalSubSubmit
			}
			// Cancel button
			cancelW := int32(100)
			cancelX := subBtnX - 10 - cancelW
			if x >= cancelX && x <= cancelX+cancelW && y >= mY+mH-52 && y <= mY+mH-14 {
				return HitModalCancel
			}
			// Inside modal card but not on control
			if x >= mX && x <= mX+mW && y >= mY && y <= mY+mH {
				return HitNone
			}
			return HitModalBackdrop
		}

		if gApp.activeModal == ModalAddServer || gApp.activeModal == ModalEditServer {
			mW := int32(520)
			mH := int32(430)
			mX := (w - mW) / 2
			mY := (h - mH) / 2

			// Close button ✕
			if x >= mX+mW-44 && x <= mX+mW-16 && y >= mY+16 && y <= mY+44 {
				return HitModalClose
			}
			inW := mW - 48
			inH := int32(32)
			// Input 1
			if x >= mX+24 && x <= mX+24+inW && y >= mY+68 && y <= mY+68+inH {
				return HitModalAddInput1
			}
			// Input 2
			if x >= mX+24 && x <= mX+24+inW && y >= mY+132 && y <= mY+132+inH {
				return HitModalAddInput2
			}
			// Input 3
			if x >= mX+24 && x <= mX+24+inW && y >= mY+196 && y <= mY+196+inH {
				return HitModalAddInput3
			}
			// Input 4
			if x >= mX+24 && x <= mX+24+inW && y >= mY+260 && y <= mY+260+inH {
				return HitModalAddInput4
			}
			// Paste shadowstar:// button
			if x >= mX+24 && x <= mX+24+230 && y >= mY+306 && y <= mY+334 {
				return HitModalAddPaste
			}
			// Submit button
			saveBtnW := int32(190)
			saveBtnX := mX + mW - 24 - saveBtnW
			if x >= saveBtnX && x <= saveBtnX+saveBtnW && y >= mY+mH-52 && y <= mY+mH-14 {
				return HitModalAddSubmit
			}
			// Cancel button
			cancelW := int32(100)
			cancelX := saveBtnX - 10 - cancelW
			if x >= cancelX && x <= cancelX+cancelW && y >= mY+mH-52 && y <= mY+mH-14 {
				return HitModalCancel
			}
			if x >= mX && x <= mX+mW && y >= mY && y <= mY+mH {
				return HitNone
			}
			return HitModalBackdrop
		}

		if gApp.activeModal == ModalUpdate {
			mW := int32(520)
			mH := int32(320)
			mX := (w - mW) / 2
			mY := (h - mH) / 2

			// Close button ✕
			if x >= mX+mW-44 && x <= mX+mW-16 && y >= mY+16 && y <= mY+44 {
				return HitModalClose
			}
			// Download button
			dlBtnW := int32(220)
			dlBtnX := mX + mW - 24 - dlBtnW
			if x >= dlBtnX && x <= dlBtnX+dlBtnW && y >= mY+mH-52 && y <= mY+mH-14 {
				return HitModalUpdateDownload
			}
			// Cancel button
			cancelW := int32(110)
			cancelX := dlBtnX - 12 - cancelW
			if x >= cancelX && x <= cancelX+cancelW && y >= mY+mH-52 && y <= mY+mH-14 {
				return HitModalCancel
			}
			if x >= mX && x <= mX+mW && y >= mY && y <= mY+mH {
				return HitNone
			}
			return HitModalBackdrop
		}
	}

	// Settings button: x: w-56..w-20, y: 12..44
	setX := w - 20 - 36
	if x >= setX && x <= setX+36 && y >= 12 && y <= 44 {
		return HitBtnSettings
	}

	// Language Switcher: [RU | EN]
	langX := setX - 8 - 46
	if x >= langX && x <= langX+46 && y >= 12 && y <= 44 {
		return HitBtnLangToggle
	}

	// Update Badge (if update available)
	gApp.mu.Lock()
	hasUpd := gApp.hasUpdate
	gApp.mu.Unlock()
	if hasUpd {
		updW := int32(84)
		updX := langX - 8 - updW
		if x >= updX && x <= updX+updW && y >= 12 && y <= 44 {
			return HitBtnUpdateBadge
		}
	}

	// Tabs:
	tabX := int32(180)
	tabW := int32(130)
	if x >= tabX && x <= tabX+tabW && y >= 12 && y <= 44 {
		return HitTabDashboard
	}
	tab2X := tabX + tabW + 10
	tab2W := int32(165)
	if x >= tab2X && x <= tab2X+tab2W && y >= 12 && y <= 44 {
		return HitTabServers
	}

	if gApp.currentTab == TabDashboard {
		colW := (w - 50) / 2
		cardLeftX := int32(20)
		cardLeftY := int32(64)

		btnCX := cardLeftX + colW/2
		btnCY := cardLeftY + 80
		dx := x - btnCX
		dy := y - btnCY
		if dx*dx+dy*dy <= 48*48 {
			return HitHeroButton
		}

		cardRightX := cardLeftX + colW + 10
		badgeW := int32(80)
		badgeX := cardRightX + colW - 16 - badgeW
		badgeY := cardLeftY + 12
		if x >= badgeX && x <= badgeX+badgeW && y >= badgeY && y <= badgeY+26 {
			return HitActivePingBadge
		}

		swBtnY := cardLeftY + 158
		swBtnW := colW - 32
		if x >= cardRightX+16 && x <= cardRightX+16+swBtnW && y >= swBtnY && y <= swBtnY+38 {
			return HitBtnSwitchServer
		}

		row3Y := int32(382)
		row3H := int32(76)
		if x >= cardLeftX && x <= cardLeftX+colW && y >= row3Y && y <= row3Y+row3H {
			return HitSysProxyCard
		}

		if x >= cardRightX && x <= cardRightX+colW && y >= row3Y && y <= row3Y+row3H {
			return HitPortsCard
		}

	} else {
		// SERVERS TAB HIT-TESTING
		barX := int32(20)
		barY := int32(64)
		barW := w - 40
		btnY := barY + 32
		btnH := int32(30)

		b1X := barX + 10
		b1W := int32(118)
		if x >= b1X && x <= b1X+b1W && y >= btnY && y <= btnY+btnH {
			return HitBtnUpdateSub
		}

		b2X := b1X + b1W + 8
		b2W := int32(118)
		if x >= b2X && x <= b2X+b2W && y >= btnY && y <= btnY+btnH {
			return HitBtnImportClip
		}

		b3X := b2X + b2W + 8
		b3W := int32(112)
		if x >= b3X && x <= b3X+b3W && y >= btnY && y <= btnY+btnH {
			return HitBtnPingAll
		}

		b4X := b3X + b3W + 8
		b4W := int32(118)
		if x >= b4X && x <= b4X+b4W && y >= btnY && y <= btnY+btnH {
			return HitBtnAddServer
		}

		b5X := b4X + b4W + 8
		b5W := barW - 10 - (b5X - barX)
		if x >= b5X && x <= b5X+b5W && y >= btnY && y <= btnY+btnH {
			return HitBtnSetSubURL
		}

		listY := int32(148)
		cardH := int32(66)
		gap := int32(8)
		itemsPerPage := 4

		gApp.mu.Lock()
		srvCount := len(gApp.cfg.Servers)
		gApp.mu.Unlock()

		startIdx := gApp.serverPage * itemsPerPage
		endIdx := startIdx + itemsPerPage
		if endIdx > srvCount {
			endIdx = srvCount
		}

		for i := startIdx; i < endIdx; i++ {
			rowY := listY + int32(i-startIdx)*(cardH+gap)
			if y >= rowY && y <= rowY+cardH && x >= barX && x <= barX+barW {
				actBW := int32(96)
				actBX := barX + barW - 14 - actBW
				pingBW := int32(76)
				pingBX := actBX - 8 - pingBW
				editBW := int32(34)
				editBX := pingBX - 8 - editBW
				delBW := int32(34)
				delBX := editBX - 8 - delBW

				if y >= rowY+17 && y <= rowY+49 {
					if x >= delBX && x <= delBX+delBW {
						return HitServerBaseDelete + i
					}
					if x >= editBX && x <= editBX+editBW {
						return HitServerBaseEdit + i
					}
					if x >= pingBX && x <= pingBX+pingBW {
						return HitServerBasePing + i
					}
					if x >= actBX && x <= actBX+actBW {
						return HitServerBaseSelect + i
					}
				}
				return HitServerBaseSelect + i
			}
		}

		footY := int32(450)
		autoBW := int32(225)
		if x >= barX && x <= barX+autoBW && y >= footY && y <= footY+36 {
			return HitBtnAutoBest
		}

		if srvCount > itemsPerPage {
			prevX := barX + autoBW + 15
			if x >= prevX && x <= prevX+36 && y >= footY && y <= footY+36 {
				return HitServerPagePrev
			}
			nextX := prevX + 40 + 110
			if x >= nextX && x <= nextX+36 && y >= footY && y <= footY+36 {
				return HitServerPageNext
			}
		}
	}

	return HitNone
}

func (app *AppState) handleHitClick(hwnd uintptr, hit int, pt POINT) {
	// Modal click interactions
	if app.activeModal != ModalNone {
		switch hit {
		case HitModalClose, HitModalCancel, HitModalBackdrop:
			app.closeModal()
		case HitModalSubInput:
			app.mu.Lock()
			app.focusedField = 1
			app.mu.Unlock()
			pInvalidateRect.Call(hwnd, 0, 0)
		case HitModalSubPaste:
			app.handleModalPaste()
			pInvalidateRect.Call(hwnd, 0, 0)
		case HitModalSubSubmit:
			app.submitModal()
		case HitModalAddInput1:
			app.mu.Lock()
			app.focusedField = 1
			app.mu.Unlock()
			pInvalidateRect.Call(hwnd, 0, 0)
		case HitModalAddInput2:
			app.mu.Lock()
			app.focusedField = 2
			app.mu.Unlock()
			pInvalidateRect.Call(hwnd, 0, 0)
		case HitModalAddInput3:
			app.mu.Lock()
			app.focusedField = 3
			app.mu.Unlock()
			pInvalidateRect.Call(hwnd, 0, 0)
		case HitModalAddInput4:
			app.mu.Lock()
			app.focusedField = 4
			app.mu.Unlock()
			pInvalidateRect.Call(hwnd, 0, 0)
		case HitModalAddPaste:
			app.handleModalPaste()
			pInvalidateRect.Call(hwnd, 0, 0)
		case HitModalAddSubmit:
			app.submitModal()
		case HitModalUpdateDownload:
			app.mu.Lock()
			url := ""
			if app.updateInfo != nil {
				url = app.updateInfo.HTMLURL
			}
			app.mu.Unlock()
			if url == "" {
				url = "https://github.com/komendant-zero/shadowstar/releases/latest"
			}
			_ = updater.OpenURL(url)
			app.closeModal()
		}
		return
	}

	switch hit {
	case HitBtnLangToggle:
		i18n.ToggleLang()
		pInvalidateRect.Call(hwnd, 0, 0)
	case HitBtnUpdateBadge:
		app.openUpdateModal()
	case HitTabDashboard:
		app.currentTab = TabDashboard
		pInvalidateRect.Call(hwnd, 0, 0)
	case HitTabServers:
		app.currentTab = TabServers
		pInvalidateRect.Call(hwnd, 0, 0)
	case HitBtnSwitchServer:
		app.currentTab = TabServers
		pInvalidateRect.Call(hwnd, 0, 0)
	case HitHeroButton:
		app.toggleConnect()
	case HitActivePingBadge:
		go app.measureActiveLatency()
	case HitSysProxyCard:
		app.toggleSysProxy(hwnd)
	case HitPortsCard:
		copyToClipboard(hwnd, "127.0.0.1:10808")
		app.showToast("✓ SOCKS5 скопирован: 127.0.0.1:10808")
	case HitBtnSettings:
		app.showSettingsMenu(hwnd, pt)
	case HitBtnUpdateSub:
		go app.updateSubscriptionFromURL()
	case HitBtnImportClip:
		app.importFromClipboard(hwnd)
	case HitBtnPingAll:
		go app.pingAllServersConcurrently()
	case HitBtnAddServer:
		app.openAddServerModal()
	case HitBtnSetSubURL:
		app.openSubscriptionModal()
	case HitBtnAutoBest:
		app.selectFastestServer()
	case HitServerPagePrev:
		if app.serverPage > 0 {
			app.serverPage--
			pInvalidateRect.Call(hwnd, 0, 0)
		}
	case HitServerPageNext:
		app.mu.Lock()
		maxPages := (len(app.cfg.Servers) + 3) / 4
		app.mu.Unlock()
		if app.serverPage < maxPages-1 {
			app.serverPage++
			pInvalidateRect.Call(hwnd, 0, 0)
		}
	default:
		if hit >= HitServerBaseSelect && hit < HitServerBaseEdit {
			idx := hit - HitServerBaseSelect
			app.selectServerByIndex(idx)
		} else if hit >= HitServerBaseEdit && hit < HitServerBaseDelete {
			idx := hit - HitServerBaseEdit
			app.openEditServerModal(idx)
		} else if hit >= HitServerBaseDelete && hit < HitServerBasePing {
			idx := hit - HitServerBaseDelete
			app.deleteServerByIndex(idx)
		} else if hit >= HitServerBasePing {
			idx := hit - HitServerBasePing
			go app.pingSingleServer(idx)
		}
	}
}

func (app *AppState) showToast(msg string) {
	app.mu.Lock()
	app.toastMsg = msg
	app.toastTimer = 3
	app.mu.Unlock()
	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) toggleConnect() {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.connected {
		if app.sysProxyEnabled {
			_ = sysproxy.ClearSystemProxy()
			app.sysProxyEnabled = false
		}
		_ = app.client.Stop()
		app.connected = false
		app.connecting = false
		app.toastMsg = "Туннель отключён"
		app.toastTimer = 2
	} else {
		if len(app.cfg.Servers) == 0 {
			app.toastMsg = "⚠️ Нет узлов для подключения! Добавьте узел"
			app.toastTimer = 3
			pInvalidateRect.Call(app.hwndMain, 0, 0)
			return
		}
		app.connecting = true
		pInvalidateRect.Call(app.hwndMain, 0, 0)

		if err := app.client.Start(); err != nil {
			app.connected = false
			app.connecting = false
			app.toastMsg = "Ошибка подключения: " + err.Error()
			app.toastTimer = 3
			return
		}
		app.connected = true
		app.connecting = false
		app.toastMsg = "✓ Анти-DPI туннель успешно подключён!"
		app.toastTimer = 2

		if !isAWGActive() {
			_ = sysproxy.SetSystemProxy(app.cfg.HttpAddr)
			app.sysProxyEnabled = true
		}
	}
	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) toggleSysProxy(hwnd uintptr) {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.sysProxyEnabled {
		_ = sysproxy.ClearSystemProxy()
		app.sysProxyEnabled = false
		app.toastMsg = "Системный прокси отключён"
		app.toastTimer = 2
	} else {
		if isAWGActive() {
			app.toastMsg = "⚠️ AmneziaWG активен: оставлен локальный SOCKS5"
			app.toastTimer = 3
			return
		}
		_ = sysproxy.SetSystemProxy(app.cfg.HttpAddr)
		app.sysProxyEnabled = true
		app.toastMsg = "✓ Системный прокси Windows активирован!"
		app.toastTimer = 2
	}
	pInvalidateRect.Call(hwnd, 0, 0)
}

func (app *AppState) measureActiveLatency() {
	defer func() { _ = recover() }()
	app.mu.Lock()
	active := app.client.GetProfile()
	app.pingStatus = "..."
	app.mu.Unlock()
	pInvalidateRect.Call(app.hwndMain, 0, 0)

	lat, err := config.PingServer(active.ServerAddr, 3*time.Second)

	app.mu.Lock()
	if err != nil {
		app.pingStatus = "Таймаут"
		app.pingMs = -1
	} else {
		app.pingMs = lat
		app.pingStatus = fmt.Sprintf("%d ms", lat)
	}
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) updateTelemetry() {
	stats := app.client.GetStats()
	app.mu.Lock()
	app.connected = stats.Connected
	app.speedDown = uint64(stats.SpeedDown)
	app.speedUp = uint64(stats.SpeedUp)
	app.bytesDown = stats.BytesDown
	app.bytesUp = stats.BytesUp
	app.activeConns = stats.ActiveConns
	app.connectedTime = stats.ConnectedTime

	if app.toastTimer > 0 {
		app.toastTimer--
		if app.toastTimer == 0 {
			app.toastMsg = ""
		}
	}
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) selectServerByIndex(idx int) {
	app.mu.Lock()
	if idx < 0 || idx >= len(app.cfg.Servers) {
		app.mu.Unlock()
		return
	}
	selected := app.cfg.Servers[idx]
	app.cfg.ActiveID = selected.ID
	app.client.UpdateProfile(selected)
	_ = config.SaveClientConfig(app.cfgPath, app.cfg)
	app.toastMsg = "✓ Выбран узел: " + selected.Name
	app.toastTimer = 2
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)
	go app.measureActiveLatency()
}

func (app *AppState) deleteServerByIndex(idx int) {
	app.mu.Lock()
	if idx < 0 || idx >= len(app.cfg.Servers) {
		app.mu.Unlock()
		return
	}
	deletedName := app.cfg.Servers[idx].Name
	deletedID := app.cfg.Servers[idx].ID

	app.cfg.Servers = append(app.cfg.Servers[:idx], app.cfg.Servers[idx+1:]...)
	if len(app.cfg.Servers) == 0 {
		app.cfg.ActiveID = ""
		app.client.UpdateProfile(client.ServerProfile{})
		if app.connected {
			if app.sysProxyEnabled {
				_ = sysproxy.ClearSystemProxy()
				app.sysProxyEnabled = false
			}
			_ = app.client.Stop()
			app.connected = false
			app.connecting = false
		}
	} else if app.cfg.ActiveID == deletedID {
		app.cfg.ActiveID = app.cfg.Servers[0].ID
		app.client.UpdateProfile(app.cfg.Servers[0])
	}
	itemsPerPage := 4
	maxPages := (len(app.cfg.Servers) + itemsPerPage - 1) / itemsPerPage
	if app.serverPage >= maxPages && app.serverPage > 0 {
		app.serverPage = maxPages - 1
	}
	_ = config.SaveClientConfig(app.cfgPath, app.cfg)
	app.toastMsg = "✓ Удалён узел: " + deletedName
	app.toastTimer = 2
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) pingSingleServer(idx int) {
	defer func() { _ = recover() }()
	app.mu.Lock()
	if idx < 0 || idx >= len(app.cfg.Servers) {
		app.mu.Unlock()
		return
	}
	targetAddr := app.cfg.Servers[idx].ServerAddr
	app.mu.Unlock()

	lat, err := config.PingServer(targetAddr, 3*time.Second)

	app.mu.Lock()
	if idx < len(app.cfg.Servers) {
		if err != nil {
			app.cfg.Servers[idx].Ping = -1
		} else {
			app.cfg.Servers[idx].Ping = lat
		}
	}
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) pingAllServersConcurrently() {
	defer func() { _ = recover() }()
	app.mu.Lock()
	if app.pingingAll {
		app.mu.Unlock()
		return
	}
	app.pingingAll = true
	serversCopy := make([]client.ServerProfile, len(app.cfg.Servers))
	copy(serversCopy, app.cfg.Servers)
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)

	results := config.PingAllConcurrently(serversCopy, 3*time.Second)

	app.mu.Lock()
	app.pingingAll = false
	for i := range app.cfg.Servers {
		if lat, ok := results[app.cfg.Servers[i].ID]; ok {
			app.cfg.Servers[i].Ping = lat
		}
	}
	_ = config.SaveClientConfig(app.cfgPath, app.cfg)
	app.toastMsg = "✓ Проверка пинга завершена!"
	app.toastTimer = 2
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) selectFastestServer() {
	app.mu.Lock()
	defer app.mu.Unlock()

	var bestIdx = -1
	var minPing = 999999

	for i, s := range app.cfg.Servers {
		if s.Ping > 0 && s.Ping < minPing {
			minPing = s.Ping
			bestIdx = i
		}
	}

	if bestIdx == -1 {
		app.toastMsg = "Сначала проверьте пинг серверов!"
		app.toastTimer = 2
		pInvalidateRect.Call(app.hwndMain, 0, 0)
		return
	}

	best := app.cfg.Servers[bestIdx]
	app.cfg.ActiveID = best.ID
	app.client.UpdateProfile(best)
	_ = config.SaveClientConfig(app.cfgPath, app.cfg)
	app.toastMsg = fmt.Sprintf("✓ Выбран лучший: %s (%d ms)", best.Name, best.Ping)
	app.toastTimer = 3
	pInvalidateRect.Call(app.hwndMain, 0, 0)
}

func (app *AppState) updateSubscriptionFromURL() {
	defer func() {
		if r := recover(); r != nil {
			app.mu.Lock()
			app.updatingSub = false
			app.toastMsg = "Ошибка при обновлении"
			app.toastTimer = 3
			app.mu.Unlock()
			pInvalidateRect.Call(app.hwndMain, 0, 0)
		}
	}()

	app.mu.Lock()
	subURL := app.cfg.SubscriptionURL
	if subURL == "" {
		app.mu.Unlock()
		app.openSubscriptionModal()
		return
	}
	if app.updatingSub {
		app.mu.Unlock()
		return
	}
	app.updatingSub = true
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)

	profiles, err := config.FetchSubscription(subURL)

	app.mu.Lock()
	app.updatingSub = false
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no such host") || strings.Contains(errMsg, "lookup") {
			app.toastMsg = "Ошибка: не найден сервер подписки"
		} else if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline") {
			app.toastMsg = "Ошибка: таймаут связи с сервером"
		} else if strings.Contains(errMsg, "404") {
			app.toastMsg = "Ошибка 404: файл подписки не найден"
		} else if len(errMsg) > 36 {
			app.toastMsg = "Ошибка: " + errMsg[:32] + "..."
		} else {
			app.toastMsg = "Ошибка: " + errMsg
		}
		app.toastTimer = 4
		app.mu.Unlock()
		pInvalidateRect.Call(app.hwndMain, 0, 0)
		return
	}

	if len(profiles) == 0 {
		app.toastMsg = "Подписка не содержит серверов"
		app.toastTimer = 3
		app.mu.Unlock()
		pInvalidateRect.Call(app.hwndMain, 0, 0)
		return
	}

	existingMap := make(map[string]bool)
	for _, p := range profiles {
		existingMap[p.ServerAddr] = true
	}

	var merged []client.ServerProfile
	merged = append(merged, profiles...)

	for _, s := range app.cfg.Servers {
		if !existingMap[s.ServerAddr] {
			merged = append(merged, s)
		}
	}

	app.cfg.Servers = merged
	if len(app.cfg.Servers) > 0 {
		var activeFound bool
		for _, s := range app.cfg.Servers {
			if s.ID == app.cfg.ActiveID {
				activeFound = true
				break
			}
		}
		if !activeFound {
			app.cfg.ActiveID = app.cfg.Servers[0].ID
			app.client.UpdateProfile(app.cfg.Servers[0])
		}
	}

	_ = config.SaveClientConfig(app.cfgPath, app.cfg)
	app.toastMsg = fmt.Sprintf("✓ Подписка обновлена! Серверов: %d", len(merged))
	app.toastTimer = 3
	app.mu.Unlock()

	pInvalidateRect.Call(app.hwndMain, 0, 0)
	go app.pingAllServersConcurrently()
}

func (app *AppState) importFromClipboard(hwnd uintptr) {
	text := readClipboardText(hwnd)
	text = strings.TrimSpace(text)
	if text == "" {
		app.showToast("Буфер обмена пуст!")
		return
	}

	if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
		app.mu.Lock()
		app.cfg.SubscriptionURL = text
		_ = config.SaveClientConfig(app.cfgPath, app.cfg)
		app.mu.Unlock()
		app.showToast("✓ URL сохранен! Загружаем...")
		go app.updateSubscriptionFromURL()
		return
	}

	if strings.HasPrefix(text, "shadowstar://") {
		prof, err := config.DecodeURIToProfile(text)
		if err != nil {
			app.showToast("Неверный формат ссылки: " + err.Error())
			return
		}
		app.mu.Lock()
		app.cfg.Servers = append(app.cfg.Servers, *prof)
		app.cfg.ActiveID = prof.ID
		app.client.UpdateProfile(*prof)
		_ = config.SaveClientConfig(app.cfgPath, app.cfg)
		app.toastMsg = "✓ Узел импортирован: " + prof.Name
		app.toastTimer = 3
		app.mu.Unlock()

		pInvalidateRect.Call(hwnd, 0, 0)
		return
	}

	profiles, err := config.ParseSubscriptionContent([]byte(text))
	if err == nil && len(profiles) > 0 {
		app.mu.Lock()
		app.cfg.Servers = append(app.cfg.Servers, profiles...)
		app.cfg.ActiveID = profiles[0].ID
		app.client.UpdateProfile(profiles[0])
		_ = config.SaveClientConfig(app.cfgPath, app.cfg)
		app.toastMsg = fmt.Sprintf("✓ Импортировано %d узлов из буфера!", len(profiles))
		app.toastTimer = 3
		app.mu.Unlock()

		pInvalidateRect.Call(hwnd, 0, 0)
		go app.pingAllServersConcurrently()
		return
	}

	app.showToast("В буфере не найдено ссылок ShadowStar!")
}

func (app *AppState) showSettingsMenu(hwnd uintptr, pt POINT) {
	hMenu, _, _ := pCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer pDestroyMenu.Call(hMenu)

	p1, _ := syscall.UTF16PtrFromString(i18n.T("⚡ Проверить пинг всех серверов", "⚡ Ping all servers"))
	pAppendMenuW.Call(hMenu, 0, 101, uintptr(unsafe.Pointer(p1)))

	p2, _ := syscall.UTF16PtrFromString(i18n.T("🔗 Скопировать ссылку текущего узла", "🔗 Copy active server link"))
	pAppendMenuW.Call(hMenu, 0, 102, uintptr(unsafe.Pointer(p2)))

	p3, _ := syscall.UTF16PtrFromString(i18n.T("➕ Добавить узел вручную...", "➕ Add server manually..."))
	pAppendMenuW.Call(hMenu, 0, 103, uintptr(unsafe.Pointer(p3)))

	p4, _ := syscall.UTF16PtrFromString(i18n.T("🌐 Настроить URL подписки...", "🌐 Configure subscription URL..."))
	pAppendMenuW.Call(hMenu, 0, 104, uintptr(unsafe.Pointer(p4)))

	pAppendMenuW.Call(hMenu, 0x0800 /* MF_SEPARATOR */, 0, 0)

	p5, _ := syscall.UTF16PtrFromString(i18n.T("🔄 Проверить обновления (GitHub)", "🔄 Check for updates (GitHub)"))
	pAppendMenuW.Call(hMenu, 0, 107, uintptr(unsafe.Pointer(p5)))

	p6, _ := syscall.UTF16PtrFromString(i18n.T("🌐 Сменить язык (RU / EN)", "🌐 Switch Language (RU / EN)"))
	pAppendMenuW.Call(hMenu, 0, 108, uintptr(unsafe.Pointer(p6)))

	p7, _ := syscall.UTF16PtrFromString(i18n.T("⭐ Открыть GitHub репозиторий", "⭐ Open GitHub Repository"))
	pAppendMenuW.Call(hMenu, 0, 109, uintptr(unsafe.Pointer(p7)))

	p8, _ := syscall.UTF16PtrFromString(i18n.T("📝 Открыть config.json в Блокноте", "📝 Open config.json in Notepad"))
	pAppendMenuW.Call(hMenu, 0, 105, uintptr(unsafe.Pointer(p8)))

	pAppendMenuW.Call(hMenu, 0x0800 /* MF_SEPARATOR */, 0, 0)

	pQuit, _ := syscall.UTF16PtrFromString(i18n.T("✕ Закрыть ShadowStar", "✕ Exit ShadowStar"))
	pAppendMenuW.Call(hMenu, 0, 106, uintptr(unsafe.Pointer(pQuit)))

	var scrPt POINT = pt
	pClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&scrPt)))

	cmd, _, _ := pTrackPopupMenu.Call(hMenu, 0x0100 /* TPM_RETURNCMD */, uintptr(scrPt.X), uintptr(scrPt.Y), 0, hwnd, 0)
	switch cmd {
	case 101:
		go app.pingAllServersConcurrently()
	case 102:
		app.copyProfileLink(hwnd)
	case 103:
		app.openAddServerModal()
	case 104:
		app.openSubscriptionModal()
	case 105:
		_ = exec.Command("notepad.exe", app.cfgPath).Start()
	case 106:
		pDestroyWindow.Call(hwnd)
	case 107:
		go app.checkGitHubUpdate(true)
	case 108:
		i18n.ToggleLang()
		pInvalidateRect.Call(hwnd, 0, 0)
	case 109:
		_ = updater.OpenURL("https://github.com/komendant-zero/shadowstar")
	}
}

func (app *AppState) copyProfileLink(hwnd uintptr) {
	app.mu.Lock()
	activeProfile := app.client.GetProfile()
	app.mu.Unlock()
	uri := config.EncodeProfileToURI(activeProfile)
	copyToClipboard(hwnd, uri)
	app.showToast("✓ Ссылка shadowstar:// скопирована!")
}

// Run launches the Modern Native Windows GUI
func Run(cfgPath string, c *client.Client, cfg *client.Config, autoConnect bool) error {
	runtime.LockOSThread()

	if pSetProcessDPIAware.Find() == nil {
		pSetProcessDPIAware.Call()
	}

	var input GdiplusStartupInput
	input.GdiplusVersion = 1
	pGdiplusStartup.Call(uintptr(unsafe.Pointer(&gdiplusToken)), uintptr(unsafe.Pointer(&input)), 0)
	defer pGdiplusShutdown.Call(gdiplusToken)

	hInstance, _, _ := pGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("ShadowStarModernWindow")
	hCursorArr, _, _ := pLoadCursorW.Call(0, uintptr(32512)) // IDC_ARROW
	hCursorHnd, _, _ := pLoadCursorW.Call(0, uintptr(32649)) // IDC_HAND
	hBrushBg, _, _ := pCreateSolidBrush.Call(0x00160D09)    // Obsidian space

	gApp = &AppState{
		client:        c,
		cfg:           cfg,
		cfgPath:       cfgPath,
		hCursorArr:    hCursorArr,
		hCursorHnd:    hCursorHnd,
		hFontBrand:    createFont("Segoe UI", 15, true),
		hFontHeader:   createFont("Segoe UI", 13, true),
		hFontTitle:    createFont("Segoe UI", 11, true),
		hFontButton:   createFont("Segoe UI", 10, true),
		hFontRegular:  createFont("Segoe UI", 10, false),
		hFontSmall:    createFont("Segoe UI", 9, false),
		hFontTiny:     createFont("Segoe UI", 8, false),
		hFontCode:     createFont("Consolas", 10, false),
		hFontCodeBold: createFont("Consolas", 10, true),
		hFontBigSpeed: createFont("Segoe UI", 18, true),
	}

	for i := range gApp.cfg.Servers {
		if gApp.cfg.Servers[i].Country == "" {
			cCode, _ := config.DetectCountry(gApp.cfg.Servers[i].Name, gApp.cfg.Servers[i].ServerAddr)
			gApp.cfg.Servers[i].Country = cCode
		}
	}

	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         0x0002 | 0x0001,
		LpfnWndProc:   syscall.NewCallback(mainWndProc),
		HInstance:     hInstance,
		HCursor:       hCursorArr,
		HbrBackground: hBrushBg,
		LpszClassName: className,
	}

	atom, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassExW failed: %w", err)
	}

	clientW := int32(680)
	clientH := int32(530)
	var wr RECT
	wr.Left = 0
	wr.Top = 0
	wr.Right = clientW
	wr.Bottom = clientH
	winStyle := uint32(0x00CA0000)
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&wr)), uintptr(winStyle), 0, 0)
	winW := wr.Right - wr.Left
	winH := wr.Bottom - wr.Top

	scrW, _, _ := pGetSystemMetrics.Call(0)
	scrH, _, _ := pGetSystemMetrics.Call(1)
	posX := (int32(scrW) - winW) / 2
	posY := (int32(scrH) - winH) / 2

	winTitle, _ := syscall.UTF16PtrFromString("ShadowStar • Anti-DPI Stealth Client")
	hwnd, _, err := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(winTitle)),
		uintptr(winStyle),
		uintptr(posX), uintptr(posY), uintptr(winW), uintptr(winH),
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed: %w", err)
	}
	gApp.hwndMain = hwnd

	darkMode := int32(1)
	pDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&darkMode)), 4)

	pShowWindow.Call(hwnd, 5)
	pUpdateWindow.Call(hwnd)

	if autoConnect {
		gApp.toggleConnect()
	}

	go gApp.measureActiveLatency()
	go gApp.checkGitHubUpdate(false)

	// 500ms timer for telemetry and blinking cursor
	pSetTimer.Call(hwnd, 1, 500, 0)

	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	return nil
}
