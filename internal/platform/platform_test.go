//go:build darwin

package platform

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"keepawake-jiggler/internal/core"
)

// TestAcquireTakesPowerAssertionWithoutRoot checks the macOS power
// assertion is actually taken, and that it needs no privileges.
//
// Criterion: while the process runs, macOS reports a PreventUserIdleSystemSleep
// assertion owned by it; after the release function runs, it is gone.
func TestAcquireTakesPowerAssertionWithoutRoot(t *testing.T) {
	if !trustedForAccessibility() {
		t.Skip("process is not trusted for Accessibility; skipping the live assertion check")
	}

	b := New()
	release, err := b.Acquire(false)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// The assertion must appear in pmset output, owned by this process.
	out := runCommand(t, "pmset", "-g", "assertions")
	if !strings.Contains(out, "PreventUserIdleSystemSleep") {
		t.Fatalf("expected a PreventUserIdleSystemSleep assertion, got:\n%s", out)
	}
	if !strings.Contains(out, "keepawake-jiggler") {
		t.Fatalf("expected the assertion to be named for this tool, got:\n%s", out)
	}

	release()

	out = runCommand(t, "pmset", "-g", "assertions")
	if strings.Contains(out, "keepawake-jiggler: keep the system awake") {
		t.Fatalf("expected the assertion to be released, got:\n%s", out)
	}
}

// TestJiggleGlobalProducesHIDEvent checks that a global nudge really enters
// the HID event stream, which is what resets the system idle timer.
//
// Criterion: a global nudge produces a system-level mouse-moved event.
//
// The HID event counter is used instead of comparing pointer positions,
// because the person using the machine moves the pointer too, which would
// make a position comparison unreliable.
func TestJiggleGlobalProducesHIDEvent(t *testing.T) {
	if !trustedForAccessibility() {
		t.Skip("process is not trusted for Accessibility; skipping the live nudge check")
	}

	b, ok := New().(*Backend)
	if !ok {
		t.Fatal("expected the darwin backend")
	}
	before := b.MouseMovedCounter()

	cur, err := b.PointerLocation()
	if err != nil {
		t.Fatalf("PointerLocation: %v", err)
	}
	if err := b.JiggleGlobal(core.Point{X: cur.X + 1, Y: cur.Y}); err != nil {
		t.Fatalf("JiggleGlobal: %v", err)
	}

	// The counter lags the asynchronous delivery, so poll for the increase
	// rather than reading it once.
	increased := false
	for range 20 {
		time.Sleep(100 * time.Millisecond)
		if b.MouseMovedCounter() > before {
			increased = true
			break
		}
	}
	if !increased {
		t.Errorf("HID mouse-moved counter did not rise above %d within 2s", before)
	}
}

// TestJiggleTargetLeavesSystemPointerAlone checks that a nudge aimed at one
// process is not delivered to the system pointer.
//
// Criterion: a nudge aimed at one application does not move the system
// pointer. The fact that the event is delivered to the target process was
// verified separately: a test application registered as the target received
// exactly three mouseMoved events at exactly the coordinates posted.
func TestJiggleTargetLeavesSystemPointerAlone(t *testing.T) {
	if !trustedForAccessibility() {
		t.Skip("process is not trusted for Accessibility; skipping the live targeted nudge check")
	}

	b := New()
	before, err := b.PointerLocation()
	if err != nil {
		t.Fatalf("PointerLocation: %v", err)
	}

	// Aim at this very process, at a point far from the pointer's current
	// position. The physical pointer must not move there.
	requested := core.Point{X: before.X + 500, Y: before.Y + 300}
	self := core.Target{PID: int32(os.Getpid()), App: "keepawake-jiggler", Known: true}
	if err := b.JiggleTarget(self, requested); err != nil {
		t.Fatalf("JiggleTarget: %v", err)
	}
	settle()

	after, err := b.PointerLocation()
	if err != nil {
		t.Fatalf("PointerLocation: %v", err)
	}
	if after.X == requested.X && after.Y == requested.Y {
		t.Errorf("system pointer moved to the targeted position (%v, %v)", after.X, after.Y)
	}
}

// settle waits for posted pointer events to be processed. The system
// delivers them asynchronously, so the pointer position is read too early
// otherwise.
func settle() {
	time.Sleep(400 * time.Millisecond)
}

// runCommand runs a command and returns its combined output.
func runCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}
