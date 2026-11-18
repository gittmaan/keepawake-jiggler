# Vorssaint 3.4.4 — how "Move pointer slightly" works

Reverse engineering notes for `/Applications/Vorssaint.app` (3.4.4, build 106,
`com.vorssaint.utils`, arm64 native Mach-O, LSUIElement menu-bar app).

Claims below are marked **seen** (file:offset or symbol evidence) or
**measured** (behaviour observed while the app ran), or **inferred**.

## The two mechanisms behind Keep Awake

Keep Awake is built from two independent pieces:

1. **IOKit power assertions** — the actual sleep prevention. Seen in
   `KeepAwakeManager.applyAssertions`:
   - `IOPMAssertionCreateWithName("PreventUserIdleSystemSleep", level=0xff,
     "Vorssaint: keep the Mac awake", &id)` at `0x1006baff0`–`0x1006bb044`.
   - A second `PreventUserIdleDisplaySleep` / "Vorssaint: keep the display on"
     assertion at `0x1006bb114`–`0x1006bb160`, gated on the
     `keepAwakeAllowDisplaySleep` setting (live value `1` on this machine, and
     live `pmset -g assertions` shows Vorssaint holding **only** the system
     assertion, not the display one).
   - **No root required.** Assertions are per-process and ordinary processes
     can take them.
2. **The pointer nudge** ("Move pointer slightly") — a *supplementary* trick
   that feeds the system a real mouse-moved event, resetting the HID idle
   timer. Caption, seen in the binary's string table:
   > "During a session, moves the pointer a little at the chosen interval."

The only root-requiring part of Vorssaint is the separate **closed-lid**
(clamshell) feature, which installs a restricted sudoers rule
(`/etc/sudoers.d/vorss*-clamshell` granting `pmset disablesleep` only). Seen in
the binary's string table; **not** part of Keep Awake.

## `KeepAwakeManager.jiggleMousePointer()` — the nudge

Symbol (seen): `_$s9Vorssaint16KeepAwakeManagerC18jiggleMousePointer33_04DF8D…LLyyF`
at `0x1006c2b24`.

Algorithm, in order:

1. Read `keepAwakeMouseJiggleEnabled` from `UserDefaults` (`boolForKey:` at
   `0x1006c2c84`). If false → reschedule the timer and return
   (`0x1006c2c9c`).
2. Read the pointer position: `CGEventCreate(NULL)` (`0x1006c2ca4`) →
   `CGEventGetLocation` (`0x1006c2cb0`).
3. Find the display that contains the pointer: `CGGetActiveDisplayList`
   twice (count, then fill, `0x1006c2cd4`/`0x1006c2d14`), then for each
   display `CGDisplayBounds` (`0x1006c2d54`) and compare against
   `CGRectGetMinX/MaxX/MinY/MaxY`. A pointer outside a display's bounds
   skips that display (`b.hi` → next iteration at `0x1006c2d44`).
4. Shrink the containing display by **2 points** on every side:
   `CGRectInset(rect, 2.0, 2.0)` (`0x1006c2df0`, with `fmov d4/d5, #2.0` at
   `0x1006c2dd8`).
5. Clamp the pointer position into that safe area with two `fcsel` pairs
   (`0x1006c2e0c`, `0x1006c2e28`, `0x1006c2e44`, `0x1006c2e60`), then stash
   it on the stack (`stp d12, d0, [x29, #-0xe0]`, `0x1006c2e64`).
6. Pick the step, X axis first, each candidate tested against the safe area:
   - `clampedX + 1` if it is still ≤ the safe area's maxX (`0x1006c2e6c`,
     `b.ls` at `0x1006c2e88`);
   - else `clampedX - 1` if it is still ≥ minX (`0x1006c2e94`,
     `b.ls` at `0x2eb0`);
   - else `clampedY + 1` (`0x2ebc`, `b.ls` at `0x2ed8`);
   - else `clampedY - 1` (`0x2ee0`);
   - else give up and only reschedule (`0x2f00 → 0x2f24`).
7. Post it: `CGEventSourceCreate(1 /* kCGEventSourceStateHIDSystemState */)`
   (`0x1006c2f60`), `CGEventCreateMouseEvent(src, 5 /* kCGEventMouseMoved */,
   point, 0 /* kCGMouseButtonLeft */)` (`0x1006c2f78`), then
   `CGEventPost(0 /* kCGHIDEventTap */, event)` (`0x1006c2f8c`).

`CGEventPost(kCGHIDEventTap, …)` is what makes it a real, system-wide
mouse move: the HID layer treats it as genuine user activity, which is what
resets the idle timer.

## Settings involved

Seen as `DefaultsKey` symbols:
`keepAwakeMouseJiggleEnabled`, `keepAwakeMouseJiggleInterval`,
`keepAwakeAllowDisplaySleep`, `keepAwakeAutoStart`, `keepAwakeActiveIcon`.

The interval lives under the key `keepAwakeMouseJiggleIntervalMinutes`
(measured from this machine's `com.vorssaint.utils` defaults: `1`).
The interval picker offers 2/5/15/30 minutes as labels (seen in the string
table); the stored default on this machine is 1 minute.

## Measured behaviour of the running app

While a Keep Awake session was active (assertion held for 49h 18m per
`pmset -g assertions`), sampling
`CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateHIDSystemState,
kCGEventMouseMoved)` every 2 s for 150 s showed the HID idle timer reset at
`t = 52.1 s` and again at `t = 112.2 s` — exactly the configured 1-minute
cadence. That is the app posting a mouse-moved event to the HID tap.

High-frequency position polling (20 ms) over 200 s caught steps of exactly
`+1.0` point, and one pair at `t = 109.42 s` / `t = 169.43 s` — 60.0 s apart —
of `(+1.0, −1.5)` followed by `(−1.0, +1.5)`, i.e. a step and a matching
restore.

## The targeted variant — what changes

Posting the same event to a single process instead of the HID tap is the
per-window/per-app variant. Measured:

| | HID-tap post (`CGEventPost`) | Per-process post (`CGEventPostToPid`) |
|---|---|---|
| Reaches other applications | yes | no |
| Reaches the target app | yes | yes (a test app received 3/3 events at the exact coordinates posted) |
| Moves the physical pointer | yes, +1.0 point | **no** (measured `moved = (0.0, 0.0)`) |
| Resets the system HID idle timer | yes, `22.4 s → 0.79 s` | **no** (`22.4 s → 22.9 s`) |
| Needs Accessibility trust | yes | yes |

**The load-bearing consequence:** aiming the nudge at one app makes *that
app* think the pointer moved, but it does **not** reset the system idle
timer, so it does not keep the machine awake on its own. The machine is kept
awake by the power assertion, which is independent of the nudge. That is why
this tool always takes the assertion and treats the nudge as a separate,
optional signal.

Note also that per-process delivery goes to the **process**, not to one
window: the receiving app routes a mouse-moved event to whichever window is
under the posted coordinates, so aiming at a window means choosing
coordinates inside that window's bounds (this tool does that when the
window's bounds are known).

## Accessibility trust

Posting synthetic pointer events from a process macOS does not trust for
Accessibility is silently dropped. Measured: `AXIsProcessTrusted()` true in
this environment, and the nudge works; when untrusted, the event is dropped
with no error. This tool checks the state at startup and prints how to grant
it. It is a per-app permission grant, **not** root.
