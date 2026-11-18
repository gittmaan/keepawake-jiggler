# keepawake-jiggler

A single-binary process that keeps a macOS or Windows machine awake without
administrator/root rights, and optionally nudges the pointer a little at a
chosen interval — a faithful rebuild of Vorssaint's Keep Awake "Move pointer
slightly" feature, with the pointer nudge extended so it can be aimed at one
application or window instead of the whole screen.

The engineering notes for agents live in [`AGENTS.md`](AGENTS.md); the evidence
behind the nudge geometry is in
[`docs/vorssaint-reverse-engineering.md`](docs/vorssaint-reverse-engineering.md).

## Why it needs no root

The sleep prevention uses the OS's own mechanism, which ordinary processes may
use:

| | macOS | Windows |
|---|---|---|
| API | `IOPMAssertionCreateWithName` | `SetThreadExecutionState` |
| Asserts | `PreventUserIdleSystemSleep` (+ `PreventUserIdleDisplaySleep` with `-display-sleep`) | `ES_CONTINUOUS \| ES_SYSTEM_REQUIRED` (+ `ES_DISPLAY_REQUIRED`) |
| Needs privileges | none | none |

Nothing here touches `sudo`, `pmset`, or an installer. Assertions are released
when the process exits.

## Pointer nudge

Reproduces Vorssaint's `KeepAwakeManager.jiggleMousePointer` exactly: read the
pointer position, find the display containing it, inset that display's bounds
by 2 points, clamp the position into the safe area, then step one point along
X if that stays inside the safe area, otherwise one point along Y. The event
is a `kCGEventMouseMoved` from a `kCGEventSourceStateHIDSystemState` source
posted to `kCGHIDEventTap`, which is what makes the system treat it as genuine
user activity.

### Aiming it at one application

`-target-pid` or `-target-app` delivers the same event to that process only
(`CGEventPostToPid` on macOS, `PostMessageW(WM_MOUSEMOVE)` on Windows) and
confines the posted coordinates to that window's bounds when they are known.

**What that does and does not achieve.** Measured on macOS:

| | global nudge | nudge aimed at one app |
|---|---|---|
| Target app sees the pointer move | yes | yes |
| Other apps see it | yes | no |
| Physical pointer moves | yes, 1 point | no |
| Resets the system idle timer | yes | **no** |

So aiming the nudge at one app keeps *that app* from thinking the user went
idle, but it does **not** keep the machine awake by itself — the idle timer is
fed by system-level activity, which a per-process post never touches. That is
why the power assertion is always taken, regardless of the nudge. Use `-jiggle`
without a target when the nudge itself has to be the keep-awake signal; use a
target when you want only one app to stay awake while the assertion does the
real work.

On macOS, per-process delivery goes to the **process**: the receiving app
routes a mouse-moved event to whichever window is under the posted
coordinates, so a window target works by choosing coordinates inside it. Window
titles are only reported by macOS to processes with Screen Recording
permission; without it, match on the application name or PID instead.

## Usage

```
keepawake-jiggler [flags]
```

| flag | meaning |
|---|---|
| `-jiggle` | nudge the pointer at each interval |
| `-interval 1m` | how often to nudge (default 1m, like Vorssaint's default) |
| `-target-pid 1234` | aim the nudge at this process |
| `-target-app Code` | aim the nudge at the window whose app or title contains this text |
| `-watch-target` | hold the assertion only while the target application is in the foreground |
| `-display-sleep` | also keep the display awake, not just the system |
| `-list-targets` | list aimable windows, then exit |
| `-hid-focus` | macOS: bring the target forward, send real pointer and key input, then restore focus (needed for Citrix/VDI) |
| `-key f15` | key tapped by `-hid-focus`: `f15` or `shift` |

The process runs until interrupted; Ctrl-C releases the assertions.

## Accessibility on macOS

Posting synthetic pointer events requires the app to be trusted for
Accessibility. macOS silently drops the events otherwise. The tool checks and
warns at startup; grant it under System Settings → Privacy & Security →
Accessibility. This is a per-app permission grant, not root. The power
assertion itself needs no permission at all.

## Building

```
go build -o keepawake-jiggler ./cmd/keepawake-jiggler          # host (macOS)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -o keepawake-jiggler.exe ./cmd/keepawake-jiggler   # Windows
```

The macOS side uses CoreGraphics and IOKit through cgo; the Windows side is
pure Go against `user32`/`kernel32`, so it cross-compiles with no Windows
toolchain present.

## Tests

```
go test ./...
```

`internal/core` tests the nudge geometry (edge cases: pointer at the display
edge, one-point-wide display, clamp from outside, window-bounded nudge) and
display selection. `internal/platform` has live macOS tests: the power
assertion appearing in and disappearing from `pmset -g assertions`, a global
nudge producing a HID-level mouse-moved event (counted, so it is immune to a
person moving the mouse), and a targeted nudge leaving the system pointer
alone.