---
PLAN: "feat: selection history (last 10) with alt+click marking for browser_get_selected_element"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 12970885000868296164
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — devbrowser: a history of selected elements

Phase **C** of the master plan
`SOURCE_SELECTION_MASTER_PLAN.md` (orchestration only — everything this plan needs is inline).
**Depends on phase A1:** `webtyp.com/filepath` at the tag exporting `Tilde`
(`https://github.com/webtyp/filepath/blob/main/docs/PLAN.md`), and on phase A3: `webtyp.com/lang`
(`https://github.com/webtyp/lang/blob/main/docs/PLAN.md`), which replaces `webtyp.com/fmt/lang`. Phase D (`webtyp/app`) waits for the
tag this plan produces.

Read [AGENTS.md](../AGENTS.md) first. Critical rules, repeated here:

- This repo is **backend tooling** and does NOT compile to WASM: `os`, `path/filepath`, `time`,
  `strings`, `encoding/json` are legitimate. Do NOT replace them with `webtyp.com/*` packages.
- Do not edit `chromedp/` or `cdproto/` (vendored).
- Messages to the developer go through `b.Logger` with `webtyp.com/lang` (`lang.Translate(...)`),
  like `devbrowser.go`.
- Every repeated string (JS global names, binding name, attribute names, tool messages) is a named
  constant.
- Tests that need Chrome call `t.Skip` when `devbrowser.ResolveChromeExecPath()` returns `""`.
- Tests live in `tests/` (`package devbrowser_test`, public API only). A root-level test is allowed
  only with a top-of-file justification of the unexported identifier it needs. **Never export a
  symbol so a test can reach it.**

## Why

`browser_get_selected_element` keeps exactly one selection: the inspect-pointer event overwrites
`LastInspectedBackendNodeID` (`inspect_capture.go`). When a developer — alone or in a conversation
with other devs — points at several things that must change, only the last one survives. Re-reading
nodes later is also fragile: `webtyp/dom` replaces nodes on re-render, so a stored node reference
goes stale. The tool needs a **history of snapshots taken at selection time**, plus a selection
gesture that works without DevTools open.

Today's fallbacks are also removed: `__webtyp_last_clicked` (every normal click), `window.$0` read
from the page, and `document.activeElement`. They made "the selection" mean "whatever was touched
last", which is wrong once there is a history. A selection is now only a deliberate act.

## Design gate

**1. Prior art.**
- Chrome DevTools: `$0`–`$4` keep the last five Elements-panel selections, but only in the
  DevTools console, not exposed over CDP, and as live node references.
- React DevTools / Vue DevTools: "select element in page" picker, one selection at a time.
- Figma / design-review tools (Markup.io, BugHerd): numbered pins dropped on the page, listed in
  order and kept as snapshots with a screenshot. That is what this plan builds.

We follow the review-tool family (numbered marks, snapshot per mark) because the consumer is an LLM
reading the list later, after the page may have re-rendered or navigated away.

**2. Novice-name test.**
- Tool arg `count`: "how many of the latest selections to return".
- Tool arg `clear`: "clear the selections".
- `SelectionSource` values: `"devtools inspect pointer"`, `"alt+click"`, `"devtools elements panel"`.
- `SourceKind` values: `"render"`, `"style"`, `"declaration"`, `"text"` — what the line is.

**3. Complexity ledger.**
```
Concepts the developer must learn   +1 (alt+click marks an element) / −1 (no more "last click wins")
Files they must touch to do X       +0 / −0
Lines at the call site              +0 (count defaults to 1: today's callers unchanged)
Ways to do the same thing           +0 / −3 (last-clicked, page $0, activeElement fallbacks deleted)
```

**4. Where it belongs.** The history is browser state captured from CDP events: devbrowser. The
source resolution stays behind the existing `SourceLocator` interface implemented by `webtyp/app`.

**5. What it deletes.** `inspect_capture.go` (whole file), `injectInspectListenerJS`,
`DevBrowser.LastInspectedBackendNodeID`, the exported `GetSelectedElementJS`, the JS globals
`__webtyp_last_clicked`, `__webtyp_selected`, `__webtyp_target_for_screenshot`, the
`$0`/activeElement fallbacks, and `SourceLocation.Snippet` (always equal to `Match`).

## Contract shared with webtyp/app (phase D)

`source_locator.go` becomes exactly:

```go
package devbrowser

// ElementSourceQuery contains element selectors and attributes for source resolution.
type ElementSourceQuery struct {
	Tag         string            `json:"tag"`
	ID          string            `json:"id"`
	Classes     []string          `json:"classes"`
	DataKey     string            `json:"data_key"`
	Attributes  map[string]string `json:"attributes"`
	Breadcrumbs []string          `json:"breadcrumbs"`
}

// SourceKind says what a SourceLocation points at.
type SourceKind string

const (
	SourceRender      SourceKind = "render"      // the line that puts the class into the DOM
	SourceStyle       SourceKind = "style"       // the stylesheet line for the class (a css.go file)
	SourceDeclaration SourceKind = "declaration" // the widget identity: const NameX = widget.Name("x")
	SourceText        SourceKind = "text"        // a literal match of a class/id/key outside widget identity
)

// SourceLocation is one place in local source code related to a DOM element.
type SourceLocation struct {
	File   string     `json:"file"`   // as shown to a person: "~/..." (wpath.Tilde from webtyp.com/filepath)
	Line   int        `json:"line"`
	Kind   SourceKind `json:"kind"`
	Token  string     `json:"token"`  // the DOM token that led here, e.g. "composebar__row" or "ancestor composebar"
	Match  string     `json:"match"`  // the trimmed source line
	Origin string     `json:"origin"` // "project", "webtyp.com/components (local checkout; project pins v0.8.7)", ...
}

// SourceLocator resolves a DOM element's identifiers to local source code files.
type SourceLocator interface {
	LocateSource(query ElementSourceQuery) []SourceLocation
}
```

## Stage 1 — the history (new file `selection_history.go`)

```go
// selectionHistoryCap is how many selections are kept; the oldest is dropped first.
const selectionHistoryCap = 10

// SelectionSource is the gesture that produced a selection.
type SelectionSource string

const (
	SelectionInspectPointer SelectionSource = "devtools inspect pointer"
	SelectionAltClick       SelectionSource = "alt+click"
	SelectionDevToolsPanel  SelectionSource = "devtools elements panel"
)

// selection is a snapshot taken when the element was selected. It never
// references the live node again (webtyp/dom replaces nodes on re-render).
type selection struct {
	seq        int               // 1, 2, 3… for the browser session; equals the badge number on the page
	source     SelectionSource
	at         time.Time
	pageURL    string
	nodeID     cdp.BackendNodeID // only to drop an immediate duplicate
	report     string            // formatSelectedElementReport output, source locations included
	screenshot []byte            // PNG with the highlight; nil when the element was not visible
}

// selectionHistory is guarded by DevBrowser.Mu.
type selectionHistory struct {
	items   []selection // oldest first, len <= selectionHistoryCap
	nextSeq int
}

// push assigns the next seq and appends s. When s.nodeID equals the newest
// item's nodeID (and is not 0) nothing is added and ok is false.
func (h *selectionHistory) push(s selection) (added selection, ok bool)

// latest returns up to n items, NEWEST FIRST.
func (h *selectionHistory) latest(n int) []selection

// len returns how many items are held.
func (h *selectionHistory) len() int

// clear empties the history and returns how many items it held. nextSeq is NOT reset,
// so badge numbers never repeat within a browser session.
func (h *selectionHistory) clear() int
```

Add to `DevBrowser` (in `devbrowser.go`): `selections selectionHistory` (guarded by `Mu`),
`lastPanelNodeID cdp.BackendNodeID` (guarded by `Mu`), `captureMu sync.Mutex` (serializes
captures), `selectionCaptureInstalled bool` (guarded by `Mu`). Delete the field
`LastInspectedBackendNodeID`.

In `CloseBrowser.go`, under `Mu`, set `selectionCaptureInstalled = false` and
`lastPanelNodeID = 0` (the next browser context needs its listeners again). Do NOT clear
`selections`: the snapshots are self-contained and stay valid across a browser restart, and
`nextSeq` keeps counting so a number is never reused.

The ring is unexported, so it is tested through the tool, in `tests/` (see Stage 5,
`TestSelectedElement_HistoryCap`). Do NOT add a `_test.go` file at the repo root: every new test
in this plan goes in `tests/` as `package devbrowser_test`.

## Stage 2 — capturing (new file `selection_capture.go`, replaces `inspect_capture.go`)

Delete `inspect_capture.go`. In `OpenBrowser.go`, replace the call `h.initializeInspectCapture()`
with `h.installSelectionCapture()` and remove `chromedp.Evaluate(injectInspectListenerJS, nil)` from
the navigation `chromedp.Run`.

Constants:

```go
const (
	selectionBindingName  = "__webtyp_mark"          // CDP binding the page calls on alt+click
	selectionQueueGlobal  = "__webtyp_capture_queue" // window array of elements waiting to be captured
	selectionTargetGlobal = "__webtyp_capture_target"
	selectionBadgeAttr    = "data-webtyp-mark"
)
```

**Page script** `selectionPageScriptJS` (string built with `fmt.Sprintf` from the constants). It must
be idempotent (`if (window.__webtyp_selection_installed) return;`) and:
- define `window[selectionQueueGlobal] = []` if absent;
- add **capture-phase** listeners on `document` for `mousedown` and `click`. When
  `e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey` and `e.target instanceof Element` and the
  target is not `document.body`, not `document.documentElement` and has no `selectionBadgeAttr`:
  call `e.preventDefault()` and `e.stopImmediatePropagation()` (both events, so the app's own
  handlers do not fire); on `click` only, push `e.target` onto the queue and call
  `window[selectionBindingName]('alt+click')` if it is a function.
- The modifier lives in ONE place in the script (a `const isMark = (e) => …` helper) with a comment:
  on desktops where Alt is the window-move modifier (XFCE, KDE before Plasma 6) the click never
  reaches the page; changing the gesture is this one line.

**`func (b *DevBrowser) installSelectionCapture()`**, idempotent via `selectionCaptureInstalled`.
If `b.Ctx == nil` it returns. Otherwise it:
1. Registers `chromedp.ListenTarget(b.Ctx, …)` handling:
   - `*overlay.EventInspectNodeRequested` with `BackendNodeID != 0` → in a goroutine: resolve the
     node (`dom.ResolveNode().WithBackendNodeID`), push it onto the queue with
     `runtime.CallFunctionOn("function(){ (window.__webtyp_capture_queue ||= []).push(this); }")`
     (build the string from the constant), then `b.captureSelection(SelectionInspectPointer)`.
   - `*runtime.EventBindingCalled` with `Name == selectionBindingName` → in a goroutine:
     `b.captureSelection(SelectionAltClick)`.
2. Runs (in a goroutine, like the old code) `dom.Enable()`, `overlay.Enable()`,
   `runtime.AddBinding(selectionBindingName)`,
   `page.AddScriptToEvaluateOnNewDocument(selectionPageScriptJS)` (survives reloads and
   navigations — the old listener was evaluated once and was lost on every reload), and
   `chromedp.Evaluate(selectionPageScriptJS, nil)` for the document already loaded.

**`func (b *DevBrowser) captureSelection(source SelectionSource)`** — holds `b.captureMu` for its
whole body:
1. Evaluate JS that does
   `window.__webtyp_capture_target = (window.__webtyp_capture_queue || []).shift() || null;` and,
   if non-null, `scrollIntoView({block:'nearest', inline:'nearest'})` and returns
   `JSON.stringify({hasSelection:true, ...extractElementDetails(target)})`, else
   `JSON.stringify({hasSelection:false})`. Reuse `ExtractElementDetailsFunctionJS` exactly as
   `GetSelectedElementJS` did. `hasSelection:false` → return silently.
2. Get the node's BackendNodeID: `runtime.Evaluate("window.__webtyp_capture_target")` →
   `ObjectID` → `dom.DescribeNode().WithObjectID(id)` → `node.BackendNodeID`. Failure → `0`.
3. `report := b.formatSelectedElementReport(&data)` (Stage 4 shape).
4. Screenshot: move the existing clip + highlight + `page.CaptureScreenshot().WithClip` block from
   `mcp-selected.go` into `func (b *DevBrowser) captureHighlighted(clip selectedClip) []byte`, with
   `applyHighlightJS`/`removeHighlightJS` operating on `window.__webtyp_capture_target` (rename
   from `__webtyp_target_for_screenshot`). Width or height `<= 0` → nil.
5. Read the page URL (`chromedp.Location`).
6. Under `b.Mu`: `added, ok := b.selections.push(selection{…, at: time.Now()})`.
7. If `ok`: evaluate the badge script for `added.seq` **after** the screenshot (the capture must not
   contain the badge): a `<div>` appended to `document.body` with `selectionBadgeAttr="<seq>"`, text
   `<seq>`, inline style `position:absolute; left:<rect.left+scrollX>px; top:<rect.top+scrollY>px;
   z-index:2147483647; background:#2563eb; color:#fff; font:600 11px/16px system-ui,sans-serif;
   padding:0 5px; border-radius:8px; pointer-events:none;`. Then log
   `b.Logger(lang.Translate("Selection", "#"+seq, "captured", "(", source, "):", breadcrumbTail).String())`
   where `breadcrumbTail` is the last breadcrumb segment.

**DevTools Elements panel.** Move the existing polling of the DevTools target (the block in
`mcp-selected.go` that evaluates `window.UI?.panels?.elements…` and returns `bID`) unchanged into
`func (b *DevBrowser) devToolsPanelNodeID() cdp.BackendNodeID`. Delete its diagnostic fields
(`panelKeys`, `nodeFound`, `nodeTag`); the script returns only `bID`.

## Stage 3 — the tool (`mcp-selected.go`, `models.go`, `models_orm.go`)

`models.go`:

```go
var GetSelectedElementArgsModel = model.Definition{
	Name: "get_selected_element_args",
	Fields: model.Fields{
		{Name: "count", Type: model.Int()},
		{Name: "clear", Type: model.Bool()},
	},
}
```

Regenerate `models_orm.go` by running `ormc` at the repo root (header
`DO NOT EDIT. generated by webtyp.com/ormc`). The diff must touch only `GetSelectedElementArgs`
(fields `Count int64`/`Clear bool` or whatever ormc emits for `model.Int()`/`model.Bool()` — match
`ClickElementArgs`'s `WaitAfter`). If `ormc` is unavailable, write that struct and its methods by hand,
mirroring `ClickElementArgs` exactly, and say so in the PR.

Tool messages (constants):

```go
const (
	msgSelectionCleared  = "Cleared %d selection(s)."
	msgNoSelection       = "No element selected. Alt+click an element in the page, or use the DevTools inspect pointer, then call this tool again."
	msgSelectionCountErr = "count must be between 1 and %d"
	msgSelectionHeader   = "Selection #%d (%s, %s, page %s)"
	msgSelectionMore     = "History holds %d selection(s); call with count=%d to see them all."
)
```

`Execute`:
1. `ErrBrowserNotOpen` / nil-context checks unchanged. `b.installSelectionCapture()` (idempotent).
2. `req.Bind(&args)`.
3. `args.Clear` → under `Mu` `n := b.selections.clear()`; evaluate
   `document.querySelectorAll('[data-webtyp-mark]').forEach(e => e.remove())` (built from the
   constant); return `mcp.Text(fmt.Sprintf(msgSelectionCleared, n))`.
4. `count := args.Count`; `0` → `1`; `< 1 || > selectionHistoryCap` →
   `return nil, fmt.Errorf(msgSelectionCountErr, selectionHistoryCap)`.
5. Panel: `id := b.devToolsPanelNodeID()`. If `id != 0 && id != b.lastPanelNodeID`: set
   `lastPanelNodeID = id`, push the node onto the queue (same `CallFunctionOn` as Stage 2) and call
   `b.captureSelection(SelectionDevToolsPanel)` synchronously. (Comparing against
   `lastPanelNodeID`, not against the newest history item, stops an old panel selection from being
   re-added after a newer alt+click.)
6. `items := b.selections.latest(count)` under `Mu`. Empty → `mcp.Text(msgNoSelection)`.
7. Build `mcp.NewResult(blocks...)`: for each item, newest first, a `mcp.TextBlock` with
   `fmt.Sprintf(msgSelectionHeader, seq, source, at.Format("15:04:05"), pageURL)` + `"\n"` +
   `report`, followed by `mcp.ImageBlock(screenshot, "image/png")` when `screenshot != nil`. If
   `history.len() > count`, append a final `mcp.TextBlock(fmt.Sprintf(msgSelectionMore, len, len))`.

New tool `Description`:
`"Get the latest elements the developer selected in the browser — by Alt+click on the page (marked with a numbered badge), the DevTools inspect pointer, or the DevTools Elements panel. Keeps the last 10 as snapshots taken at selection time. Args: count (1-10, default 1) returns the N most recent, newest first; clear=true empties the history and removes the badges. Each selection includes WebTyp identifiers, hierarchy, outer HTML, geometry, source code locations and a cropped screenshot."`

Delete from `mcp-selected.go`: `GetSelectedElementJS`, the `bNodeID`/`LastInspectedBackendNodeID`
logic, the `__webtyp_selected` assignment, and the screenshot block (moved to Stage 2).

## Stage 4 — report and paths

1. `formatSelectedElementReport`: replace the "Source Code Location" block with:
   ```
   Source Code Location:
   - [render] ~/Dev/Project/webtyp/components/composebar/composebar.go:88 (composebar__row)
     `html.Div().Class(NameComposeBar.Class(partRow))`
     origin: webtyp.com/components (local checkout; project pins v0.8.7)
   ```
   i.e. per location: `- [<Kind>] <File>:<Line> (<Token>)`, then the match in backticks when not
   empty, then `origin: <Origin>` when not empty. `File` is printed as received (the locator already
   abbreviates it).
2. `mcp-screenshot.go`: the `"Screenshot saved to: %s"` report prints
   `wpath.Tilde(fullPath)`, where `wpath` is the import alias for `webtyp.com/filepath` (the
   repo also uses the stdlib `path/filepath`; always use the alias `wpath`).
   `go get webtyp.com/filepath@latest`.
3. Every import of `webtyp.com/fmt/lang` (2 files) becomes `webtyp.com/lang`; identifiers do not
   change. `go get webtyp.com/lang@latest`.
4. `grep -rn "Saved\|saved to\|Path:\|Dir:" --include='*.go' . | grep -v "chromedp/\|cdproto/"` and
   apply `wpath.Tilde` the same way to any other absolute filesystem path a tool returns to the MCP client.

## Stage 5 — tests (`tests/mcp_selected_test.go`)

Keep `TestSelectedElement_Metadata` and `TestSelectedElement_BrowserNotOpen` as they are.

Rewrite `TestSelectedElement_Execution` (same httptest page and headless setup; skip without
Chrome). Steps:
1. Call the tool with `{}` → content contains `No element selected`.
2. `chromedp.Click("[id='18']", chromedp.ByQuery, chromedp.ButtonModifiers(input.ModifierAlt))`
   (`input` = `webtyp.com/devbrowser/cdproto/input`). Poll the tool every 100 ms up to 3 s until the
   content contains `Selection #1`.
3. Assert content contains `alt+click`, `id="18"`, `data-key="18"`, `stepindicator__step`,
   `3 Artefactos`, and `image/png`.
4. Assert the page has exactly one `[data-webtyp-mark="1"]` element.
5. Add to the page a second button `<button id="19" class="stepindicator__step">Otro</button>`;
   alt+click it, poll for `Selection #2`. Call with `{"count":2}` → `Selection #2` appears before
   `Selection #1` in the content.
6. Alt+click button 19 again → history still holds 2 (call with `{"count":10}` shows no `#3`).
7. Call with `{"count":11}` → error containing `count must be between 1 and 10`.
8. Call with `{"clear":true}` → `Cleared 2 selection(s).`; the page has no `[data-webtyp-mark]`;
   calling with `{}` → `No element selected`.
9. A page click handler on button 18 that sets `window.__clicked = true`: after the alt+click,
   `window.__clicked` is still undefined (the app handler did not fire).

Add `TestSelectedElement_HistoryCap`: a page with 12 buttons (`id="b1"`…`id="b12"`); alt+click them in
order (poll for `Selection #N` after each). Call with `{"count":10}`: the content holds
`Selection #12` … `Selection #3` in that order, and neither `Selection #2` nor `Selection #1`. Then
`{"clear":true}` returns `Cleared 10 selection(s).`. One more alt+click produces `Selection #13`
(numbers are not reused).

Add `TestSelectedElement_SurvivesReload`: alt+click, reload the page (`chromedp.Reload()`),
alt+click again → `Selection #2` exists (the page script was re-installed by
`AddScriptToEvaluateOnNewDocument`).

## Stage 6 — root-level tests

Five tests sit at the repo root as `package devbrowser`: `context_test.go`, `execpath_test.go`,
`mcp_errors_guard_test.go`, `position_repro_test.go`, `profile_dir_test.go`. Apply this criterion
to each file (split a file if only some of its tests need internals):
- it only uses exported identifiers → `git mv` it to `tests/`, as `package devbrowser_test`;
- it needs an unexported identifier (e.g. `buildAllocatorOptions`, the profile helpers) → it stays
  at the root and gets this comment as its first lines:
  `// Root-level test (justified): exercises <unexported identifiers> — <why the behaviour is not
  observable through the exported API>.` For example, the SPKI guard in `context_test.go` checks the
  Chrome flags built before launch, which no public API returns.

**Never export a function so a test can reach it.** List the outcome per file in the PR
description.

## Acceptance

- `gotest` passes.
- `grep -rn "LastInspectedBackendNodeID\|__webtyp_last_clicked\|__webtyp_selected\|__webtyp_target_for_screenshot\|GetSelectedElementJS\|injectInspectListenerJS\|Snippet" --include='*.go' . | grep -v "chromedp/\|cdproto/"` → empty.
- `inspect_capture.go` does not exist.
- `README.md`: the tool's row/section describes Alt+click, `count`, `clear`, and the 10-item cap.

## Stages

| # | Stage | Files |
|---|---|---|
| 1 | History | `selection_history.go`, `devbrowser.go`, `CloseBrowser.go` |
| 2 | Capture | `selection_capture.go` (new), `inspect_capture.go` (deleted), `OpenBrowser.go` |
| 3 | Tool | `mcp-selected.go`, `models.go`, `models_orm.go` |
| 4 | Report and paths | `mcp-selected.go`, `source_locator.go`, `mcp-screenshot.go`, `go.mod`, `go.sum` |
| 5 | Tests and docs | `tests/mcp_selected_test.go`, `README.md` |
| 6 | Root-level tests | the five root `*_test.go` files |
