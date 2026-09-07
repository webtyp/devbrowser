---
PLAN: "fix: device-emulation test asserts on the developer's monitor size"
EXECUTOR: jules
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

The intent is correct: `desktop` and `off` must be distinct branches. The
mechanism is not. It infers "the branches are distinct" from a viewport value
the **environment** controls — the real browser window after autofit. On a
machine where that window happens to be 1440×900, `off` correctly reports
1440×900 and the test reports a regression that did not happen.

It is not flaky in the usual sense. It is a correct implementation failing an
assertion about the developer's monitor. Skill **testing**: a test must be
deterministic and side-effect free; a result that depends on the display is
neither.

## Design gate

Not required — this changes no public API. It changes one test's assertion.

## The fix

Assert the invariant the comment states — *the two modes take different
branches* — by comparing the two replies to each other, not by hard-coding a
magic viewport.

In `tests/device_emulation_test.go`, the `desktop` reply is already captured
earlier in the same test. Replace the `1440x900` check with:

1. Extract the viewport substring from each reply with one helper in this test
   file:

   ```go
   // viewportOf returns the "WxH" reported in a device-emulation reply, or "" if
   // the reply carries none.
   func viewportOf(reply string) string
   ```

2. Assert `desktop` reports a viewport, `off` reports a viewport, and — **only
   when the real window differs from the pinned desktop size** — that the two
   differ. When the machine's window genuinely is 1440×900 the two are equal and
   that is correct behaviour, not a failure, so the branch assertion must not run.

3. Assert what actually distinguishes the branches and does not depend on the
   display: `desktop` reports the pinned size `1440x900` **always**, on every
   machine. That assertion is deterministic and catches the collapse the comment
   is worried about — if `off` collapsed into `desktop`, `desktop` would still
   pass, so keep assertion 2 as the conditional complement rather than deleting
   it.

Replace the misleading comment with one that says what is actually guaranteed:
`desktop` is deterministic, `off` is environment-dependent by design, and the
test may only assert the second relative to the first.

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
