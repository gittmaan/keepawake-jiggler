# keepawake-jiggler — agent guide

Single-binary tool that keeps a macOS/Windows machine awake **without root** and
optionally nudges the pointer. Faithful rebuild of Vorssaint 3.4.4's "Move
pointer slightly", with the nudge extendable to one application/window.

## Layout

```
cmd/keepawake-jiggler/main.go   # flags, target resolution, run loop wiring
internal/core/                  # platform-independent: nudge geometry, Runner, Backend interface
internal/platform/              # per-OS backends (darwin, windows, unsupported)
  darwin_backend.go             # cgo: IOPMAssertion, CGEvent, CGWindowListCopyWindowInfo
  darwin_hid.go                # focus-nudge HID input (Citrix/VDI)
  windows_backend.go           # SetThreadExecutionState, SendInput, PostMessageW
docs/vorssaint-reverse-engineering.md   # the evidence behind the nudge geometry
```

## How it works

- **Sleep prevention** = OS power assertions. macOS: `IOPMAssertionCreateWithName`
  (`PreventUserIdleSystemSleep`). Windows: `SetThreadExecutionState(ES_CONTINUOUS |
  ES_SYSTEM_REQUIRED)`. Neither needs privileges; released on exit.
- **Pointer nudge** = post a `kCGEventMouseMoved` from `kCGEventSourceStateHIDSystemState`
  to `kCGHIDEventTap` (macOS) or `SendInput` absolute move (Windows). This is what
  resets the system idle timer.
- **Aimed at one app** = `CGEventPostToPid` (macOS) / `PostMessageW(WM_MOUSEMOVE)`
  (Windows). Delivers to that process only; does **not** reset the system idle
  timer. So the assertion always runs; the nudge is supplementary.

## Nudge geometry (the part that matters)

`core.NudgeTarget` — clamp pointer into display bounds inset by 2 points, then
step 1 point preferring X, falling back to Y (up then down). Each candidate
tested against the safe area first; never leaves it. When `limit` is set the
pointer is confined to that window instead of the display.

## Rules

- Go 1.24, cgo on macOS only (`//go:build darwin` files). Windows cross-compiles
  with `CGO_ENABLED=0`.
- `runtime.LockOSThread()` in main — `SetThreadExecutionState` is per-thread.
- Build: `go build ./cmd/keepawake-jiggler`. Test: `go test ./...`.
- Probe files at root (`probe_*.go`) are throwaway — they conflict on `main`,
  exclude from builds.

## Known limits

- macOS pointer events need Accessibility trust (per-app, not root). Warned at startup.
- Windows targeted nudge via `PostMessageW` is best-effort; many apps ignore it.
- No Windows host here — Windows path is cross-compile + vet only, not runtime-tested.