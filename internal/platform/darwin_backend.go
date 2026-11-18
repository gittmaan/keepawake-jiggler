//go:build darwin

// Package darwin implements the macOS side of the keep-awake process.
//
// The power assertion needs no privileges at all. The pointer nudge uses
// CoreGraphics, which requires the app to be trusted for Accessibility; see
// README.md for how to grant that.
package platform

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework IOKit -framework CoreFoundation

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>


// frontmost_app_pid returns the pid of the frontmost application.
//
// CGWindowListCopyWindowInfo returns windows front to back, so the first
// on-screen window on layer 0 belongs to the frontmost application.
static int frontmost_app_pid(void) {
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (list == NULL) {
		return 0;
	}
	int pid = 0;
	CFIndex count = CFArrayGetCount(list);
	for (CFIndex i = 0; i < count; i++) {
		CFDictionaryRef info = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
		if (info == NULL) {
			continue;
		}
		CFNumberRef layerRef = (CFNumberRef)CFDictionaryGetValue(info, kCGWindowLayer);
		int layer = -1;
		if (layerRef != NULL) {
			CFNumberGetValue(layerRef, kCFNumberIntType, &layer);
		}
		if (layer != 0) {
			continue;
		}
		CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(info, kCGWindowOwnerPID);
		if (pidRef != NULL) {
			CFNumberGetValue(pidRef, kCFNumberIntType, &pid);
		}
		break;
	}
	CFRelease(list);
	return pid;
}


// mouse_moved_counter returns how many mouse-moved events the HID system
// has seen. Reading it before and after a post proves the event reached the
// HID stream, which is what resets the system idle timer.
static int64_t mouse_moved_counter(void) {
	return (int64_t)CGEventSourceCounterForEventType(kCGEventSourceStateHIDSystemState, kCGEventMouseMoved);
}

// grab_assertion takes an IOKit power assertion. Returns the assertion id
// or 0 on failure. The assertion is owned by the calling process, so it is
// released when the process exits even if we never release it explicitly.
static unsigned int grab_assertion(const char *type, const char *reason) {
	CFStringRef t = CFStringCreateWithCString(NULL, type, kCFStringEncodingUTF8);
	CFStringRef r = CFStringCreateWithCString(NULL, reason, kCFStringEncodingUTF8);
	IOPMAssertionID id = 0;
	IOReturn rc = IOPMAssertionCreateWithName(t, kIOPMAssertionLevelOn, r, &id);
	CFRelease(t);
	CFRelease(r);
	if (rc != kIOReturnSuccess) {
		return 0;
	}
	return (unsigned int)id;
}

static void drop_assertion(unsigned int id) {
	IOPMAssertionRelease((IOPMAssertionID)id);
}

// current_pointer returns the current pointer location.
static CGPoint current_pointer(void) {
	CGEventRef e = CGEventCreate(NULL);
	if (e == NULL) {
		return CGPointZero;
	}
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	return p;
}

// display_count and display_bounds enumerate the attached displays.
static uint32_t display_count(void) {
	uint32_t n = 0;
	if (CGGetActiveDisplayList(0, NULL, &n) != kCGErrorSuccess) {
		return 0;
	}
	return n;
}

static CGRect display_bounds(uint32_t index) {
	CGDirectDisplayID ids[32];
	uint32_t n = 32;
	if (CGGetActiveDisplayList(n, ids, &n) != kCGErrorSuccess || index >= n) {
		return CGRectZero;
	}
	return CGDisplayBounds(ids[index]);
}

// post_mouse_moved posts a mouse-moved event to the HID event tap, which is
// what makes the system treat it as real user activity.
static int post_mouse_moved(CGPoint p) {
	CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (src == NULL) {
		return 0;
	}
	CGEventRef e = CGEventCreateMouseEvent(src, kCGEventMouseMoved, p, kCGMouseButtonLeft);
	CFRelease(src);
	if (e == NULL) {
		return 0;
	}
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
	return 1;
}

// post_mouse_moved_to_pid delivers a mouse-moved event to one process only.
// The system pointer does not move and the HID idle timer is not reset, so
// this is the "aim at one app" variant.
static int post_mouse_moved_to_pid(int pid, CGPoint p) {
	CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStatePrivate);
	if (src == NULL) {
		return 0;
	}
	CGEventRef e = CGEventCreateMouseEvent(src, kCGEventMouseMoved, p, kCGMouseButtonLeft);
	CFRelease(src);
	if (e == NULL) {
		return 0;
	}
	CGEventPostToPid((pid_t)pid, e);
	CFRelease(e);
	return 1;
}

// window_target describes one on-screen window.
struct window_target {
	int pid;
	int has_bounds;
	double x, y, w, h;
	char owner[256];
	char title[256];
};

// list_windows enumerates on-screen windows that can be aimed at.
static int list_windows(struct window_target *out, int max) {
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (list == NULL) {
		return 0;
	}
	CFIndex count = CFArrayGetCount(list);
	int n = 0;
	for (CFIndex i = 0; i < count && n < max; i++) {
		CFDictionaryRef info = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
		if (info == NULL) {
			continue;
		}
		CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(info, kCGWindowOwnerPID);
		if (pidRef == NULL) {
			continue;
		}
		int pid = 0;
		CFNumberGetValue(pidRef, kCFNumberIntType, &pid);
		if (pid <= 0) {
			continue;
		}
		struct window_target *w = &out[n];
		memset(w, 0, sizeof(*w));
		w->pid = pid;

		CFStringRef owner = (CFStringRef)CFDictionaryGetValue(info, kCGWindowOwnerName);
		if (owner != NULL) {
			CFStringGetCString(owner, w->owner, sizeof(w->owner), kCFStringEncodingUTF8);
		}
		CFStringRef title = (CFStringRef)CFDictionaryGetValue(info, kCGWindowName);
		if (title != NULL) {
			CFStringGetCString(title, w->title, sizeof(w->title), kCFStringEncodingUTF8);
		}
		CFDictionaryRef b = (CFDictionaryRef)CFDictionaryGetValue(info, kCGWindowBounds);
		if (b != NULL) {
			CGRect r = CGRectZero;
			// Bounds dictionary holds X, Y, Width, Height.
			CFNumberRef bx = (CFNumberRef)CFDictionaryGetValue(b, CFSTR("X"));
			CFNumberRef by = (CFNumberRef)CFDictionaryGetValue(b, CFSTR("Y"));
			CFNumberRef bw = (CFNumberRef)CFDictionaryGetValue(b, CFSTR("Width"));
			CFNumberRef bh = (CFNumberRef)CFDictionaryGetValue(b, CFSTR("Height"));
			double x = 0, y = 0, ww = 0, hh = 0;
			if (bx) CFNumberGetValue(bx, kCFNumberDoubleType, &x);
			if (by) CFNumberGetValue(by, kCFNumberDoubleType, &y);
			if (bw) CFNumberGetValue(bw, kCFNumberDoubleType, &ww);
			if (bh) CFNumberGetValue(bh, kCFNumberDoubleType, &hh);
			r = CGRectMake(x, y, ww, hh);
			w->x = CGRectGetMinX(r);
			w->y = CGRectGetMinY(r);
			w->w = CGRectGetWidth(r);
			w->h = CGRectGetHeight(r);
			w->has_bounds = 1;
		}
		n++;
	}
	CFRelease(list);
	return n;
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"keepawake-jiggler/internal/core"
)

// assertion names shown in `pmset -g assertions`.
const (
	assertionSystem  = "PreventUserIdleSystemSleep"
	assertionDisplay = "PreventUserIdleDisplaySleep"
	reasonSystem     = "keepawake-jiggler: keep the system awake"
	reasonDisplay    = "keepawake-jiggler: keep the display awake"
)

// Backend is the macOS implementation of core.Backend.
type Backend struct {
	systemID  uintptr
	displayID uintptr
}

// Acquire takes the power assertions that keep the Mac awake. It needs no
// privileges: IOPMAssertionCreateWithName works for any ordinary process.
func (b *Backend) Acquire(preventDisplaySleep bool) (func(), error) {
	sysName := C.CString(assertionSystem)
	sysReason := C.CString(reasonSystem)
	sysID := C.grab_assertion(sysName, sysReason)
	C.free(unsafe.Pointer(sysName))
	C.free(unsafe.Pointer(sysReason))
	if sysID == 0 {
		return nil, errors.New("could not take the system sleep assertion")
	}
	b.systemID = uintptr(sysID)

	if preventDisplaySleep {
		dispName := C.CString(assertionDisplay)
		dispReason := C.CString(reasonDisplay)
		d := C.grab_assertion(dispName, dispReason)
		C.free(unsafe.Pointer(dispName))
		C.free(unsafe.Pointer(dispReason))
		if d == 0 {
			b.release()
			return nil, errors.New("could not take the display sleep assertion")
		}
		b.displayID = uintptr(d)
	}

	return b.release, nil
}

// release drops the assertions taken by Acquire.
func (b *Backend) release() {
	if b.displayID != 0 {
		C.drop_assertion(C.uint(b.displayID))
		b.displayID = 0
	}
	if b.systemID != 0 {
		C.drop_assertion(C.uint(b.systemID))
		b.systemID = 0
	}
}

// PointerLocation returns where the pointer is now.
func (b *Backend) PointerLocation() (core.Point, error) {
	p := C.current_pointer()
	return core.Point{X: float64(p.x), Y: float64(p.y)}, nil
}

// DisplayBounds returns the bounds of every attached display.
func (b *Backend) DisplayBounds() ([]core.Rect, error) {
	const maxDisplays = 32
	n := C.display_count()
	if n == 0 {
		return nil, errors.New("no active displays")
	}
	if n > maxDisplays {
		n = maxDisplays
	}
	out := make([]core.Rect, 0, n)
	for i := range n {
		r := C.display_bounds(C.uint32_t(i))
		out = append(out, core.Rect{
			MinX: float64(C.CGRectGetMinX(r)),
			MinY: float64(C.CGRectGetMinY(r)),
			MaxX: float64(C.CGRectGetMaxX(r)),
			MaxY: float64(C.CGRectGetMaxY(r)),
		})
	}
	return out, nil
}

// JiggleGlobal moves the system pointer.
func (b *Backend) JiggleGlobal(at core.Point) error {
	p := C.CGPointMake(C.double(at.X), C.double(at.Y))
	if C.post_mouse_moved(p) == 0 {
		return errors.New("could not post the mouse-moved event")
	}
	return nil
}

// JiggleTarget delivers a mouse-moved event to one application only.
func (b *Backend) JiggleTarget(t core.Target, at core.Point) error {
	if t.PID <= 0 {
		return errors.New("target has no process id")
	}
	p := C.CGPointMake(C.double(at.X), C.double(at.Y))
	if C.post_mouse_moved_to_pid(C.int(t.PID), p) == 0 {
		return errors.New("could not post the mouse-moved event to the target")
	}
	return nil
}

// ListTargets enumerates the on-screen windows a nudge can be aimed at.
func (b *Backend) ListTargets() ([]core.Target, error) {
	const max = 512
	buf := make([]C.struct_window_target, max)
	n := C.list_windows((*C.struct_window_target)(unsafe.Pointer(&buf[0])), C.int(max))
	if n <= 0 {
		return nil, errors.New("no windows found")
	}
	out := make([]core.Target, 0, n)
	for i := 0; i < int(n); i++ {
		w := buf[i]
		t := core.Target{
			PID:   int32(w.pid),
			App:   C.GoString(&w.owner[0]),
			Title: C.GoString(&w.title[0]),
			Known: true,
		}
		if w.has_bounds != 0 {
			t.Bounds = core.Rect{
				MinX: float64(w.x), MinY: float64(w.y),
				MaxX: float64(w.x + w.w), MaxY: float64(w.y + w.h),
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// TrustedForAccessibility reports whether the app may post synthetic events.
// Posting mouse events from a process that is not trusted for Accessibility
// is silently dropped by the system.
func trustedForAccessibility() bool {
	return C.AXIsProcessTrusted() != 0
}

// frontmostAppPID returns the pid of the frontmost application.
func frontmostAppPID() (int32, error) {
	pid := C.frontmost_app_pid()
	if pid == 0 {
		return 0, errors.New("no frontmost application")
	}
	return int32(pid), nil
}

// MouseMovedCounter returns how many mouse-moved events the HID system has
// recorded. It is the interference-immune signal that a nudge really
// entered the HID stream.
func (b *Backend) MouseMovedCounter() int64 {
	return int64(C.mouse_moved_counter())
}
