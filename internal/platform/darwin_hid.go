//go:build darwin

package platform

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework CoreFoundation

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

// activate_pid brings an application to the front through the Accessibility
// API, which this tool already needs to be trusted for.
static int activate_pid(int pid) {
	AXUIElementRef app = AXUIElementCreateApplication((pid_t)pid);
	if (app == NULL) {
		return 0;
	}
	AXError err = AXUIElementSetAttributeValue(app, kAXFrontmostAttribute, kCFBooleanTrue);
	CFRelease(app);
	return err == kAXErrorSuccess;
}

// post_key_event posts one key press or release to the HID tap, so the
// frontmost application (the Citrix viewer) sees it as real keyboard input.
//
// Modifier keys (Shift, Control) are not ordinary key-down events on a real
// keyboard: the hardware reports a "flags changed" event whose flags say
// whether the modifier is now held. modifier_mask is that flag for modifier
// keys and 0 for ordinary keys, so modifiers are posted the way hardware
// posts them.
static int post_key_event(unsigned short code, int down, unsigned long long modifier_mask) {
	CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (src == NULL) {
		return 0;
	}
	CGEventRef e = CGEventCreateKeyboardEvent(src, (CGKeyCode)code, down != 0);
	CFRelease(src);
	if (e == NULL) {
		return 0;
	}
	if (modifier_mask != 0) {
		CGEventSetType(e, kCGEventFlagsChanged);
		CGEventSetFlags(e, down ? (CGEventFlags)modifier_mask : (CGEventFlags)0);
	}
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
	return 1;
}

// idle_seconds is how long ago the last real input event arrived.
static double idle_seconds(void) {
	return CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateHIDSystemState, (CGEventType)~0);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"keepawake-jiggler/internal/core"
)

// keySpec is one key the nudge can tap: its Carbon virtual key code and, for
// modifier keys, the CGEventFlags mask that goes with it.
type keySpec struct {
	code uint16
	mask uint64
}

// The keys below count as keyboard activity in Windows but do nothing
// visible on either side. Keys that are deliberately NOT offered:
//   - F14 / F15: Brightness Down / Up on Mac keyboards (F15 raised the
//     screen brightness on every nudge).
//   - F13: Print Screen in Windows, which overwrites the clipboard.
//   - F14: Scroll Lock in Windows.
var (
	keyControl = keySpec{code: 59, mask: 0x40000} // kVK_Control, kCGEventFlagMaskControl
	keyShift   = keySpec{code: 56, mask: 0x20000} // kVK_Shift,   kCGEventFlagMaskShift
	keyF16     = keySpec{code: 106}
	keyF17     = keySpec{code: 64}
	keyF18     = keySpec{code: 79}
	keyF19     = keySpec{code: 80}
)

const (
	// userActiveWindow: if the target is already frontmost and a real input
	// event arrived this recently, real input is already reaching the VDI.
	userActiveWindow = 30 * time.Second
	// quietNeeded / quietWait: do not steal focus while the person is
	// typing; wait for a pause, and skip this round if none comes.
	quietNeeded = 1500 * time.Millisecond
	quietWait   = 10 * time.Second
	// settle gives the window server time to apply each step.
	focusSettle = 150 * time.Millisecond
)

func keyFor(name string) (keySpec, error) {
	switch strings.ToLower(name) {
	case "", "ctrl", "control":
		return keyControl, nil
	case "shift":
		return keyShift, nil
	case "f16":
		return keyF16, nil
	case "f17":
		return keyF17, nil
	case "f18":
		return keyF18, nil
	case "f19":
		return keyF19, nil
	case "f13", "f14", "f15":
		return keySpec{}, fmt.Errorf("-key %q is not allowed: F14/F15 are the Mac brightness keys, F13 is Print Screen and F14 is Scroll Lock in Windows (use ctrl, shift or f16-f19)", name)
	}
	return keySpec{}, fmt.Errorf("unknown -key %q (use ctrl, shift or f16-f19)", name)
}

// CheckKey validates a -key value before any nudge is attempted.
func (b *Backend) CheckKey(key string) error {
	_, err := keyFor(key)
	return err
}

func idleSeconds() time.Duration {
	return time.Duration(float64(C.idle_seconds()) * float64(time.Second))
}

// largestWindow returns the biggest on-screen window of the target's
// process. A Citrix desktop session is one large window; the launcher and
// small helper windows are ignored by taking the largest.
func (b *Backend) largestWindow(t core.Target) (core.Rect, bool) {
	targets, err := b.ListTargets()
	if err != nil {
		return t.Bounds, !t.Bounds.IsEmpty()
	}
	var best core.Rect
	var bestArea float64
	for _, w := range targets {
		if w.PID != t.PID {
			continue
		}
		area := (w.Bounds.MaxX - w.Bounds.MinX) * (w.Bounds.MaxY - w.Bounds.MinY)
		if area > bestArea {
			best, bestArea = w.Bounds, area
		}
	}
	if bestArea == 0 {
		return t.Bounds, !t.Bounds.IsEmpty()
	}
	return best, true
}

// FocusNudge sends real, HID-level input to the target's window: it brings
// the application forward, moves the pointer inside its window, taps a
// harmless key, then puts the pointer and the previously active application
// back. Because the events come from the real event stream while the viewer
// is frontmost, the Citrix viewer forwards them to the remote session.
func (b *Backend) FocusNudge(t core.Target, key string) error {
	spec, err := keyFor(key)
	if err != nil {
		return err
	}
	if t.PID <= 0 {
		return errors.New("target has no process id")
	}

	prevPID, _ := frontmostAppPID()
	if prevPID == t.PID && idleSeconds() < userActiveWindow {
		return nil // real input is already flowing into the target
	}

	// Wait for the person to pause so a key tap never lands in their work.
	deadline := time.Now().Add(quietWait)
	for idleSeconds() < quietNeeded {
		if time.Now().After(deadline) {
			return nil // busy; try again at the next interval
		}
		time.Sleep(250 * time.Millisecond)
	}

	win, ok := b.largestWindow(t)
	if !ok {
		return errors.New("could not find a window for the target")
	}
	center := core.Point{X: (win.MinX + win.MaxX) / 2, Y: (win.MinY + win.MaxY) / 2}
	prevPointer, _ := b.PointerLocation()

	if C.activate_pid(C.int(t.PID)) == 0 {
		return errors.New("could not bring the target forward (is Accessibility granted?)")
	}
	defer func() {
		_ = b.JiggleGlobal(prevPointer)
		if prevPID != 0 && prevPID != t.PID {
			C.activate_pid(C.int(prevPID))
		}
	}()
	time.Sleep(focusSettle)

	// Two distinct positions, so the viewer sees a move, not a repeat.
	if err := b.JiggleGlobal(center); err != nil {
		return err
	}
	time.Sleep(focusSettle)
	if err := b.JiggleGlobal(core.Point{X: center.X + 1, Y: center.Y}); err != nil {
		return err
	}
	time.Sleep(focusSettle)

	mask := C.ulonglong(spec.mask)
	if C.post_key_event(C.ushort(spec.code), 1, mask) == 0 {
		return errors.New("could not post the key event")
	}
	time.Sleep(50 * time.Millisecond)
	// Always release, so a modifier is never left held down.
	C.post_key_event(C.ushort(spec.code), 0, mask)
	time.Sleep(focusSettle)
	return nil
}
