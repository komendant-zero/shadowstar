//go:build windows

package tun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

var (
	modWintun                 *syscall.LazyDLL
	procWintunCreateAdapter   *syscall.LazyProc
	procWintunOpenAdapter     *syscall.LazyProc
	procWintunCloseAdapter    *syscall.LazyProc
	procWintunStartSession    *syscall.LazyProc
	procWintunEndSession      *syscall.LazyProc
	procWintunGetReadWaitEvent *syscall.LazyProc
	procWintunReceivePacket   *syscall.LazyProc
	procWintunReleaseReceivePacket *syscall.LazyProc
	procWintunAllocateSendPacket *syscall.LazyProc
	procWintunSendPacket      *syscall.LazyProc

	initWintunOnce sync.Once
	initWintunErr  error
)

// InitWintun loads wintun.dll from exe dir, System32 or current directory
func InitWintun() error {
	initWintunOnce.Do(func() {
		candidates := []string{
			"wintun.dll",
		}
		if exe, err := os.Executable(); err == nil {
			candidates = append([]string{filepath.Join(filepath.Dir(exe), "wintun.dll")}, candidates...)
		}

		var dllPath string
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				dllPath = cand
				break
			}
		}
		if dllPath == "" {
			dllPath = "wintun.dll"
		}

		mod := syscall.NewLazyDLL(dllPath)
		if err := mod.Load(); err != nil {
			initWintunErr = fmt.Errorf("wintun.dll not found: %w", err)
			return
		}

		modWintun = mod
		procWintunCreateAdapter = mod.NewProc("WintunCreateAdapter")
		procWintunOpenAdapter = mod.NewProc("WintunOpenAdapter")
		procWintunCloseAdapter = mod.NewProc("WintunCloseAdapter")
		procWintunStartSession = mod.NewProc("WintunStartSession")
		procWintunEndSession = mod.NewProc("WintunEndSession")
		procWintunGetReadWaitEvent = mod.NewProc("WintunGetReadWaitEvent")
		procWintunReceivePacket = mod.NewProc("WintunReceivePacket")
		procWintunReleaseReceivePacket = mod.NewProc("WintunReleaseReceivePacket")
		procWintunAllocateSendPacket = mod.NewProc("WintunAllocateSendPacket")
		procWintunSendPacket = mod.NewProc("WintunSendPacket")
	})
	return initWintunErr
}

// WintunDevice represents an active Wintun session
type WintunDevice struct {
	adapterHandle uintptr
	sessionHandle uintptr
	readEvent     syscall.Handle
	name          string
	mu            sync.Mutex
	closed        bool
}

// CreateWintunDevice creates or opens the named Wintun adapter and starts a session
func CreateWintunDevice(name string) (*WintunDevice, error) {
	if err := InitWintun(); err != nil {
		return nil, err
	}

	nameUTF16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	poolUTF16, _ := syscall.UTF16PtrFromString("ShadowStar")

	// Try creating adapter
	r1, _, err := procWintunCreateAdapter.Call(
		uintptr(unsafe.Pointer(poolUTF16)),
		uintptr(unsafe.Pointer(nameUTF16)),
		0,
	)
	var adapter uintptr
	if r1 != 0 {
		adapter = r1
	} else {
		// Try opening existing adapter
		r1Open, _, errOpen := procWintunOpenAdapter.Call(
			uintptr(unsafe.Pointer(poolUTF16)),
			uintptr(unsafe.Pointer(nameUTF16)),
		)
		if r1Open == 0 {
			return nil, fmt.Errorf("failed to create or open Wintun adapter %q: create_err=%v, open_err=%v", name, err, errOpen)
		}
		adapter = r1Open
	}

	// Start Wintun session (capacity ring buffer: 0x400000 = 4MB)
	capacity := uint32(0x400000)
	session, _, sessErr := procWintunStartSession.Call(adapter, uintptr(capacity))
	if session == 0 {
		_, _, _ = procWintunCloseAdapter.Call(adapter)
		return nil, fmt.Errorf("failed to start Wintun session: %v", sessErr)
	}

	readEvent, _, _ := procWintunGetReadWaitEvent.Call(session)

	return &WintunDevice{
		adapterHandle: adapter,
		sessionHandle: session,
		readEvent:     syscall.Handle(readEvent),
		name:          name,
	}, nil
}

func (w *WintunDevice) Name() string {
	return w.name
}

func (w *WintunDevice) Read(packet []byte) (int, error) {
	for {
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return 0, errors.New("wintun device closed")
		}
		session := w.sessionHandle
		event := w.readEvent
		w.mu.Unlock()

		var size uint32
		ptr, _, _ := procWintunReceivePacket.Call(session, uintptr(unsafe.Pointer(&size)))
		if ptr != 0 && size > 0 {
			byteSlice := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)
			n := copy(packet, byteSlice)
			_, _, _ = procWintunReleaseReceivePacket.Call(session, ptr)
			return n, nil
		}

		// Wait for packet ready event or timeout
		_, _ = syscall.WaitForSingleObject(event, 250)
	}
}

func (w *WintunDevice) Write(packet []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("wintun device closed")
	}

	size := uint32(len(packet))
	ptr, _, _ := procWintunAllocateSendPacket.Call(w.sessionHandle, uintptr(size))
	if ptr == 0 {
		return 0, errors.New("wintun send packet allocation failed (ring buffer full)")
	}

	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)
	copy(dst, packet)

	_, _, _ = procWintunSendPacket.Call(w.sessionHandle, ptr)
	return len(packet), nil
}

func (w *WintunDevice) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true

	if w.sessionHandle != 0 {
		_, _, _ = procWintunEndSession.Call(w.sessionHandle)
		w.sessionHandle = 0
	}
	if w.adapterHandle != 0 {
		_, _, _ = procWintunCloseAdapter.Call(w.adapterHandle)
		w.adapterHandle = 0
	}
	return nil
}
