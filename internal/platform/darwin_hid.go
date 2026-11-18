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
static int post_key_event(unsigned short code, int down) {
	CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (src == NULL) {
		return 0;
	}
	CGEventRef e = CGEventCreateKeyboardEvent(src, (CGKeyCode)code, down != 0);
	CFRelease(src);
	if (e == NULL) {
		return 0;
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

// Virtual key codes (Carbon kVK_*). F15 and Shift do nothing visible in
// Windows, but both count as keyboard activity for its idle timer.
const (
	keyShift = 56
	keyF15   = 113
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

func keyCode(name string) (uint16, error) {
	switch strings.ToLower(name) {
	case "", "f15":
		return keyF15, nil
	case "shift":
		return keyShift, nil
	}
	return 0, fmt.Errorf("unknown -key %q (use f15 or shift)", name)
}

// CheckKey validates a -key value before any nudge is attempted.
func (b *Backend) CheckKey(key string) error {
	_, err := keyCode(key)
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
	code, err := keyCode(key)
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

	if C.post_key_event(C.ushort(code), 1) == 0 {
		return errors.New("could not post the key event")
	}
	time.Sleep(50 * time.Millisecond)
	C.post_key_event(C.ushort(code), 0)
	time.Sleep(focusSettle)
	return nil
}
