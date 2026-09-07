---
PLAN: "fix: device-emulation test asserts on the developer's monitor size"
EXECUTOR: local
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

## Prerequisite — install the test runner

External agents run in isolated environments where `gotest` is not installed.
Run this **before anything else**; the acceptance criteria depend on it:

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

Then use `gotest` for the whole suite and `gotest -run TestName` for one test.
Never call `go test` directly.

# Plan — make `TestDeviceEmulation_ValidationAndDistinctModes` deterministic

## The defect

`tests/device_emulation_test.go:139` fails intermittently and has blocked three
consecutive publishes of this module:

```
device_emulation_test.go:139: off must not pin the desktop viewport; desktop and
off would be indistinguishable. off reply: Device emulation set to off (viewport 1440x900)
```

The assertion is:

```go
// desktop pins 1440x900; off clears the override and reports the real
// window size. Asserting the two reported viewports differ catches a
// regression where the two modes collapse into the same action list.
if strings.Contains(resultTextOff, "viewport 1440x900") {
    t.Errorf("off must not pin the desktop viewport; …")
}
```

### The real root cause — worse than "it depends on the monitor"

**The test's own previous step creates the condition this step forbids.**

Step 2 sets mode `desktop`. In `mcp-management.go` that path calls
`GrowWindowToFit(1440, 900)`, and `window_autofit.go` documents that it
"resizes the live physical browser window, in place, so it is at least
(reqW, reqH) … **and never shrinks it**".

Step 3 then sets mode `off`, which pins nothing and reads the viewport back with
`chromedp.Evaluate("window.innerWidth")` — the **real** window. That window was
just grown to fit exactly 1440×900 and never shrinks, so on most machines it now
reads exactly 1440×900.

The assertion "off must not report 1440x900" therefore fails on **correct
behaviour that the test itself caused two steps earlier**. It passed only when
the display, DPI or DevTools reservation happened to push the window past
1440×900 — which is why it looked intermittent.

The intent is right: `desktop` and `off` must be distinct branches. The
mechanism is not — a reported pixel size cannot carry that meaning here.

## Design gate

Not required — this changes no public API. It changes one test's assertion.

## The fix

Assert the invariant on the **stored mode**, which is what actually distinguishes
the branches and is deterministic on every machine.

`mcp-management.go` assigns `b.ViewportMode = args.Mode` before applying the
emulation: `desktop` pins an override, `off` clears it. Asserting that is exact,
needs no window, and cannot be defeated by autofit.

In `tests/device_emulation_test.go`, the `desktop` reply is already captured
earlier in the same test. Replace the `1440x900` check with:

1. After step 2, assert `db.ViewportMode == "desktop"`.
2. After step 3, assert `db.ViewportMode == "off"` — this replaces the
   `1440x900` negative check entirely.
3. Keep the positive assertions: `desktop` reports `viewport 1440x900` (it pins,
   so this is deterministic) and `off` still reports some `viewport `.
4. Replace the misleading comment with the mechanism above, so the next reader
   does not reintroduce the same assertion.

**Do not** fix this by pinning the window size in the test, by skipping the test
on some machines, or by deleting the assertion. The first makes `off` untestable
for what it does, the second hides the case, the third loses the regression
guard.

## Constraints

- One file changes: `tests/device_emulation_test.go`.
- No production code changes. If the fix appears to need one, stop and report —
  that would mean the defect is in the implementation, not the test, and this
  plan would be wrong.
- Standard library only in tests (skill: testing): no assertion libraries.

## Acceptance criteria

1. `gotest` passes.
2. `gotest -run TestDeviceEmulation_ValidationAndDistinctModes` passes when run
   ten times in a row (`for i in $(seq 10); do gotest -run … || break; done`).
3. `grep -n "1440x900" tests/device_emulation_test.go` → present only in the
   assertion about `desktop`, never in one about `off`.

## Stages

| # | Stage | File(s) | Gate |
|---|---|---|---|
| 1 | `viewportOf` helper + reworked assertions | `tests/device_emulation_test.go` | criteria 1, 2, 3 |

Single stage.
