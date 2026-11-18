// Command keepawake-jiggler keeps the machine awake without administrator
// rights and, optionally, nudges the pointer a little at a chosen interval.
//
// The pointer nudge reproduces the "Move pointer slightly" feature of
// Vorssaint 3.4.4. It can be aimed at one window or application instead of
// the whole screen; see README.md for what that does and does not achieve.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"keepawake-jiggler/internal/core"
	"keepawake-jiggler/internal/platform"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintln(os.Stderr, "keepawake-jiggler:", err)
		os.Exit(1)
	}
}

func run() error {
	// SetThreadExecutionState is per-thread, so the goroutine that takes and
	// releases the assertion must stay on one OS thread.
	runtime.LockOSThread()

	var (
		jiggle          = flag.Bool("jiggle", false, "nudge the pointer slightly at each interval")
		interval        = flag.Duration("interval", time.Minute, "how often to nudge the pointer")
		targetPID       = flag.Int("target-pid", 0, "aim the nudge at this process id instead of the whole screen")
		targetApp       = flag.String("target-app", "", "aim the nudge at the window whose application or title contains this text")
		watchTarget     = flag.Bool("watch-target", false, "keep the machine awake only while the target application is in the foreground")
		displaySlee     = flag.Bool("display-sleep", false, "also keep the display awake, not just the system")
		listTargetsFlag = flag.Bool("list-targets", false, "list the windows a nudge can be aimed at, then exit")
		hidFocus        = flag.Bool("hid-focus", false, "macOS: bring the target forward, send real pointer and key input, then restore focus (needed for Citrix/VDI)")
		hidKey          = flag.String("key", "ctrl", "key tapped by -hid-focus: ctrl, shift or f16-f19 (F13-F15 are refused: brightness / Print Screen)")
	)
	flag.Parse()

	backend := platform.New()

	if *listTargetsFlag {
		return printTargets(backend)
	}

	if *interval <= 0 {
		return errors.New("-interval must be positive")
	}

	opts := core.Options{
		Jiggle:              *jiggle,
		Interval:            *interval,
		PreventDisplaySleep: *displaySlee,
	}

	if *targetPID != 0 || *targetApp != "" {
		target, err := resolveTarget(backend, *targetPID, *targetApp)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "aiming the nudge at %s\n", describeTarget(target))
		opts.Target = &target
	}

	if *hidFocus {
		if opts.Target == nil {
			return errors.New("-hid-focus needs -target-pid or -target-app")
		}
		wrapped, err := platform.WithFocusNudge(backend, *hidKey)
		if err != nil {
			return err
		}
		backend = wrapped
		opts.Jiggle = true
		fmt.Fprintf(os.Stderr, "focus mode: every %s the target is brought forward, nudged with the pointer and the %s key, then focus is restored\n",
			opts.Interval, *hidKey)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ticks := make(chan time.Time)
	if opts.Jiggle {
		ticker := time.NewTicker(opts.Interval)
		defer ticker.Stop()
		go func() {
			for t := range ticker.C {
				select {
				case ticks <- t:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	runner := &core.Runner{Backend: backend}

	if *watchTarget {
		if opts.Target == nil {
			return errors.New("-watch-target needs -target-pid or -target-app")
		}
		fmt.Fprintf(os.Stderr, "keeping the machine awake only while %s is in the foreground\n", describeTarget(*opts.Target))
		return runner.RunWhile(ctx, opts, ticks, func() (bool, error) {
			return platform.IsFocused(*opts.Target)
		})
	}

	fmt.Fprintln(os.Stderr, "keeping the machine awake; press Ctrl-C to stop")
	return runner.Run(ctx, opts, ticks)
}

// describeTarget renders a target for the log line.
func describeTarget(t core.Target) string {
	switch {
	case t.Title != "" && t.App != "":
		return t.App + " — " + t.Title
	case t.Title != "":
		return t.Title
	case t.App != "":
		return t.App
	default:
		return fmt.Sprintf("pid %d", t.PID)
	}
}

// resolveTarget finds the window to aim at. When several windows match a
// name (a launcher plus the desktop session, say) the largest one wins, so
// "Citrix Viewer" picks the session window rather than a helper.
func resolveTarget(backend core.Backend, pid int, app string) (core.Target, error) {
	if pid != 0 {
		t, ok := platform.TargetByPID(backend, int32(pid))
		if !ok {
			return core.Target{}, fmt.Errorf("no window found for process %d", pid)
		}
		return t, nil
	}
	targets, err := backend.ListTargets()
	if err != nil {
		return core.Target{}, err
	}
	var best core.Target
	var bestArea float64
	found := false
	for _, t := range targets {
		if !platform.MatchesApp(t, app) {
			continue
		}
		area := (t.Bounds.MaxX - t.Bounds.MinX) * (t.Bounds.MaxY - t.Bounds.MinY)
		if !found || area > bestArea {
			best, bestArea, found = t, area, true
		}
	}
	if !found {
		return core.Target{}, fmt.Errorf("no window matched %q", app)
	}
	return best, nil
}

// printTargets prints the windows a nudge can be aimed at.
func printTargets(backend core.Backend) error {
	targets, err := backend.ListTargets()
	if err != nil {
		return err
	}
	for _, t := range targets {
		fmt.Printf("%-8d %-28s %s\n", t.PID, t.App, t.Title)
	}
	return nil
}

func init() {
	// Warn early when the process cannot post synthetic pointer events.
	platform.WarnIfNotTrusted()
}
