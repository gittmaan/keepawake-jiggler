//go:build windows

// Package windows implements the Windows side of the keep-awake process.
//
// Neither SetThreadExecutionState nor SendInput needs administrator rights,
// so the whole process runs as an ordinary user.
package platform

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"keepawake-jiggler/internal/core"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSetThreadExecutionState   = user32.NewProc("SetThreadExecutionState")
	procSendInput                 = user32.NewProc("SendInput")
	procPostMessage               = user32.NewProc("PostMessageW")
	procGetForegroundWindow       = user32.NewProc("GetForegroundWindow")
	procGetCursorPos              = user32.NewProc("GetCursorPos")
	procGetWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW            = user32.NewProc("GetWindowTextW")
	procEnumWindows               = user32.NewProc("EnumWindows")
	procIsWindowVisible           = user32.NewProc("IsWindowVisible")
	procMonitorFromPoint          = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW           = user32.NewProc("GetMonitorInfoW")
	procOpenProcess               = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle               = kernel32.NewProc("CloseHandle")
)

// SetThreadExecutionState flags.
const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

// SendInput flags.
const (
	inputMouse            = 0
	mouseEventMove        = 0x0001
	mouseEventAbsolute    = 0x8000
	mouseEventVirtualDesk = 0x4000

	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
)

func getSystemMetrics(index int) uint32 {
	n, _, _ := user32.NewProc("GetSystemMetrics").Call(uintptr(index))
	return uint32(n)
}

// Windows messages.
const wmMouseMove = 0x0200

// Process access rights.
const (
	processQueryLimitedInformation = 0x1000
)

type point struct{ X, Y int32 }

type rect struct {
	Left, Top, Right, Bottom int32
}

type mouseInput struct {
	Dx, Dy      int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type input struct {
	Type uint32
	Mi   mouseInput
}

type monitorInfo struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
}

// Backend is the Windows implementation of core.Backend.
type Backend struct {
	prevState uintptr
}

// Acquire asks the system to keep the machine awake. It needs no
// administrator rights: the request is per thread and per session.
func (b *Backend) Acquire(preventDisplaySleep bool) (func(), error) {
	flags := uintptr(esContinuous | esSystemRequired)
	if preventDisplaySleep {
		flags |= esDisplayRequired
	}
	prev, _, err := procSetThreadExecutionState.Call(flags)
	if prev == 0 {
		return nil, fmt.Errorf("SetThreadExecutionState failed: %w", err)
	}
	b.prevState = prev
	return b.release, nil
}

// release returns the system to its previous power state.
func (b *Backend) release() {
	if b.prevState != 0 {
		procSetThreadExecutionState.Call(b.prevState)
		b.prevState = 0
	}
}

// PointerLocation returns where the pointer is now.
func (b *Backend) PointerLocation() (core.Point, error) {
	var p point
	r, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		return core.Point{}, fmt.Errorf("GetCursorPos failed: %w", err)
	}
	return core.Point{X: float64(p.X), Y: float64(p.Y)}, nil
}

// DisplayBounds returns the bounds of the monitor holding the pointer,
// because the nudge is scoped to the display the pointer is on.
func (b *Backend) DisplayBounds() ([]core.Rect, error) {
	cur, err := b.PointerLocation()
	if err != nil {
		return nil, err
	}
	pt := point{X: int32(cur.X), Y: int32(cur.Y)}
	mon, _, _ := procMonitorFromPoint.Call(uintptr(unsafe.Pointer(&pt)), 2 /*MONITOR_DEFAULTTONEAREST*/)
	if mon == 0 {
		return nil, errors.New("no monitor for the pointer")
	}
	mi := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, err := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return nil, fmt.Errorf("GetMonitorInfo failed: %w", err)
	}
	return []core.Rect{{
		MinX: float64(mi.RcMonitor.Left),
		MinY: float64(mi.RcMonitor.Top),
		MaxX: float64(mi.RcMonitor.Right),
		MaxY: float64(mi.RcMonitor.Bottom),
	}}, nil
}

// JiggleGlobal moves the system pointer to the requested position. An
// absolute move is used because relative deltas pass through pointer
// acceleration and would not land exactly.
func (b *Backend) JiggleGlobal(at core.Point) error {
	vx := int32(getSystemMetrics(smXVirtualScreen))
	vy := int32(getSystemMetrics(smYVirtualScreen))
	vw := int32(getSystemMetrics(smCXVirtualScreen))
	vh := int32(getSystemMetrics(smCYVirtualScreen))
	if vw == 0 || vh == 0 {
		return errors.New("no virtual screen metrics")
	}
	nx := uint32((at.X - float64(vx)) * 65535.0 / float64(vw-1))
	ny := uint32((at.Y - float64(vy)) * 65535.0 / float64(vh-1))
	in := input{
		Type: inputMouse,
		Mi:   mouseInput{Dx: int32(nx), Dy: int32(ny), DwFlags: mouseEventMove | mouseEventAbsolute | mouseEventVirtualDesk},
	}
	n, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	if n != 1 {
		return fmt.Errorf("SendInput failed: %w", err)
	}
	return nil
}

// JiggleTarget delivers a mouse-moved message to one window only. The system
// pointer does not move and the session idle timer is not reset, so this is
// the "aim at one app" variant.
func (b *Backend) JiggleTarget(t core.Target, at core.Point) error {
	if t.Handle == 0 {
		return errors.New("target has no window handle")
	}
	lp := uintptr(int32(at.X)&0xffff | (int32(at.Y)&0xffff)<<16)
	r, _, err := procPostMessage.Call(t.Handle, wmMouseMove, 0, lp)
	if r == 0 {
		return fmt.Errorf("PostMessage failed: %w", err)
	}
	return nil
}

// ListTargets enumerates the visible windows a nudge can be aimed at.
func (b *Backend) ListTargets() ([]core.Target, error) {
	var out []core.Target
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		vis, _, _ := procIsWindowVisible.Call(hwnd)
		if vis == 0 {
			return 1
		}
		var pid uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == 0 {
			return 1
		}
		buf := make([]uint16, 512)
		procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		title := syscall.UTF16ToString(buf)

		t := core.Target{
			Handle: hwnd,
			PID:    int32(pid),
			Title:  title,
			App:    processName(pid),
			Known:  true,
		}
		out = append(out, t)
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return out, nil
}

// processName returns the executable name behind a pid.
func processName(pid uint32) string {
	h, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	n, _, _ := procQueryFullProcessImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if n == 0 {
		return ""
	}
	path := syscall.UTF16ToString(buf)
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

// String makes the backend printable.
func (b *Backend) String() string { return "windows backend" }
