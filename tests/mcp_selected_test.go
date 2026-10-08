package devbrowser_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webtyp.com/devbrowser"
	"webtyp.com/devbrowser/cdproto/cdp"
	"webtyp.com/devbrowser/cdproto/input"
	"webtyp.com/devbrowser/cdproto/runtime"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

func TestSelectedElement_Metadata(t *testing.T) {
	db, _ := DefaultTestBrowser()
	defer db.CloseBrowser()

	tools := db.GetSelectedElementTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 selected element tool, got %d", len(tools))
	}

	tool := tools[0]
	if tool.Name != "browser_get_selected_element" {
		t.Errorf("expected tool name 'browser_get_selected_element', got %q", tool.Name)
	}
	if tool.Resource != "browser" {
		t.Errorf("expected resource 'browser', got %q", tool.Resource)
	}
	if tool.Action != 'r' {
		t.Errorf("expected action 'r', got %c", tool.Action)
	}
}

func TestSelectedElement_BrowserNotOpen(t *testing.T) {
	db, _ := DefaultTestBrowser()
	defer db.CloseBrowser()

	tools := db.GetSelectedElementTools()
	tool := tools[0]

	req := mcp.Request{
		Params: mcp.CallToolParams{
			Name:      "browser_get_selected_element",
			Arguments: "{}",
		},
		Action: 'r',
	}
	_, err := tool.Execute(nil, req)
	if err != devbrowser.ErrBrowserNotOpen {
		t.Errorf("expected ErrBrowserNotOpen, got %v", err)
	}
}

// selectedHarness is a headless page wired to a DevBrowser and its
// browser_get_selected_element tool.
type selectedHarness struct {
	t    *testing.T
	ctx  context.Context
	tool mcp.Tool
}

func newSelectedHarness(t *testing.T, body string) *selectedHarness {
	t.Helper()
	execPath := devbrowser.ResolveChromeExecPath()
	if execPath == "" {
		t.Skip("Chrome not available")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>WebTyp Selected Test</title></head><body>`+body+`</body></html>`)
	}))
	t.Cleanup(ts.Close)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.DisableGPU,
		chromedp.ExecPath(execPath),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(allocCancel)
	ctx, cancel := chromedp.NewContext(allocCtx)
	t.Cleanup(cancel)

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Skipf("headless Chrome unavailable: %v", err)
	}

	db, _ := DefaultTestBrowser()
	db.IsOpenFlag = true
	db.Ctx = ctx
	t.Cleanup(func() { db.CloseBrowser() })

	return &selectedHarness{t: t, ctx: ctx, tool: db.GetSelectedElementTools()[0]}
}

func (h *selectedHarness) call(args string) (string, error) {
	res, err := h.tool.Execute(nil, mcp.Request{
		Params: mcp.CallToolParams{Name: "browser_get_selected_element", Arguments: args},
		Action: 'r',
	})
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

func (h *selectedHarness) mustCall(args string) string {
	h.t.Helper()
	content, err := h.call(args)
	if err != nil {
		h.t.Fatalf("tool call %s: %v", args, err)
	}
	return content
}

// altClick clicks the element matching sel with the Alt key held, as a person would.
func (h *selectedHarness) altClick(sel string) {
	h.t.Helper()
	err := chromedp.Run(h.ctx, chromedp.QueryAfter(sel, func(c context.Context, _ runtime.ExecutionContextID, nodes ...*cdp.Node) error {
		if len(nodes) == 0 {
			return fmt.Errorf("no node for %s", sel)
		}
		return chromedp.MouseClickNode(nodes[0], chromedp.ButtonModifiers(input.ModifierAlt)).Do(c)
	}, chromedp.ByQuery))
	if err != nil {
		h.t.Fatalf("alt+click %s: %v", sel, err)
	}
}

// waitFor polls the tool (count 10) until its content contains want.
func (h *selectedHarness) waitFor(want string) string {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var content string
	for time.Now().Before(deadline) {
		content = h.mustCall(`{"count":10}`)
		if strings.Contains(content, want) {
			return content
		}
		time.Sleep(100 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %q; last content: %s", want, content)
	return ""
}

func (h *selectedHarness) eval(js string, out any) {
	h.t.Helper()
	if err := chromedp.Run(h.ctx, chromedp.Evaluate(js, out)); err != nil {
		h.t.Fatalf("evaluate %s: %v", js, err)
	}
}

func containsAttr(content, name, value string) bool {
	return strings.Contains(content, name+`=\"`+value+`\"`) || strings.Contains(content, name+`="`+value+`"`)
}

func TestSelectedElement_Execution(t *testing.T) {
	h := newSelectedHarness(t, `
		<div id="app">
			<nav>
				<button id="18" class="stepindicator__step" type="button" data-key="18">3 Artefactos</button>
			</nav>
		</div>
		<script>document.getElementById("18").addEventListener("click", () => { window.__clicked = true; });</script>`)

	// 1. Nothing selected yet (this call also installs the capture).
	if content := h.mustCall(`{}`); !strings.Contains(content, "No element selected") {
		t.Fatalf("expected guidance message, got: %s", content)
	}

	// 2–3. Alt+click is captured as Selection #1.
	h.altClick("[id='18']")
	content := h.waitFor("Selection #1")
	for _, want := range []string{"alt+click", "stepindicator__step", "3 Artefactos", "image/png"} {
		if !strings.Contains(content, want) {
			t.Errorf("content lacks %q: %s", want, content)
		}
	}
	if !containsAttr(content, "id", "18") || !containsAttr(content, "data-key", "18") {
		t.Errorf("content lacks id/data-key 18: %s", content)
	}

	// 4. Exactly one badge numbered 1.
	var badges int
	h.eval(`document.querySelectorAll('[data-webtyp-mark="1"]').length`, &badges)
	if badges != 1 {
		t.Errorf("expected 1 badge numbered 1, got %d", badges)
	}

	// 9. The app's own click handler did not fire.
	var clicked bool
	h.eval(`window.__clicked === true`, &clicked)
	if clicked {
		t.Error("the app click handler fired on alt+click")
	}

	// 5. A second element becomes Selection #2, listed first.
	h.eval(`(() => { const b = document.createElement('button'); b.id = '19'; b.className = 'stepindicator__step'; b.textContent = 'Otro'; document.querySelector('nav').appendChild(b); })()`, nil)
	h.altClick("[id='19']")
	h.waitFor("Selection #2")
	content = h.mustCall(`{"count":2}`)
	i2, i1 := strings.Index(content, "Selection #2"), strings.Index(content, "Selection #1")
	if i2 < 0 || i1 < 0 || i2 > i1 {
		t.Errorf("expected Selection #2 before Selection #1: %s", content)
	}

	// 6. Selecting the same element again adds nothing.
	h.altClick("[id='19']")
	time.Sleep(500 * time.Millisecond)
	if content := h.mustCall(`{"count":10}`); strings.Contains(content, "Selection #3") {
		t.Errorf("duplicate selection was added: %s", content)
	}

	// 7. count out of range.
	if _, err := h.call(`{"count":11}`); err == nil || !strings.Contains(err.Error(), "count must be between 1 and 10") {
		t.Errorf("expected count range error, got %v", err)
	}

	// 8. clear empties the history and removes the badges.
	if content := h.mustCall(`{"clear":true}`); !strings.Contains(content, "Cleared 2 selection(s).") {
		t.Errorf("unexpected clear result: %s", content)
	}
	var left int
	h.eval(`document.querySelectorAll('[data-webtyp-mark]').length`, &left)
	if left != 0 {
		t.Errorf("expected no badges after clear, got %d", left)
	}
	if content := h.mustCall(`{}`); !strings.Contains(content, "No element selected") {
		t.Errorf("expected empty history after clear, got: %s", content)
	}
}

func TestSelectedElement_HistoryCap(t *testing.T) {
	var body strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&body, `<button id="b%d">Button %d</button>`, i, i)
	}
	h := newSelectedHarness(t, body.String())
	h.mustCall(`{}`)

	for i := 1; i <= 12; i++ {
		h.altClick(fmt.Sprintf("#b%d", i))
		h.waitFor(fmt.Sprintf("Selection #%d ", i))
	}

	content := h.mustCall(`{"count":10}`)
	last := -1
	for n := 12; n >= 3; n-- {
		idx := strings.Index(content, fmt.Sprintf("Selection #%d ", n))
		if idx < 0 || idx < last {
			t.Fatalf("Selection #%d missing or out of order: %s", n, content)
		}
		last = idx
	}
	for _, gone := range []string{"Selection #2 ", "Selection #1 "} {
		if strings.Contains(content, gone) {
			t.Errorf("%q should have been dropped: %s", gone, content)
		}
	}

	if content := h.mustCall(`{"clear":true}`); !strings.Contains(content, "Cleared 10 selection(s).") {
		t.Errorf("unexpected clear result: %s", content)
	}

	// clear removes the badges too, so numbering starts over.
	h.altClick("#b1")
	h.waitFor("Selection #1 ")
}

// A reload wipes the badges, so the numbers already handed out no longer
// point at anything: the history empties and numbering starts over at 1. The
// capture itself survives (the page script is registered for every document).
func TestSelectedElement_ReloadStartsOver(t *testing.T) {
	h := newSelectedHarness(t, `<button id="a">A</button><button id="b">B</button>`)
	h.mustCall(`{}`)

	h.altClick("#a")
	h.waitFor("Selection #1 ")
	h.altClick("#b")
	h.waitFor("Selection #2 ")

	if err := chromedp.Run(h.ctx, chromedp.Reload(), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("reload: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(h.mustCall(`{}`), "No element selected") {
		if time.Now().After(deadline) {
			t.Fatalf("history must be empty after a reload: %s", h.mustCall(`{"count":10}`))
		}
		time.Sleep(100 * time.Millisecond)
	}

	h.altClick("#b")
	content := h.waitFor("Selection #1 ")
	if strings.Contains(content, "Selection #2 ") {
		t.Errorf("only the selection made after the reload must remain: %s", content)
	}
}

// Alt+clicking an icon lands on an SVG <path>, which no developer means: the
// selection climbs to the control that owns the icon, or to the <svg> itself
// when no control does.
func TestSelectedElement_IconSelectsItsControl(t *testing.T) {
	icon := `<svg viewBox="0 0 24 24" width="48" height="48"><path d="M0 0h24v24H0z"></path></svg>`
	h := newSelectedHarness(t, `<button id="eye" type="button">`+icon+`</button><div id="logo">`+icon+`</div>`)
	h.mustCall(`{}`)

	h.altClick("#eye path")
	content := h.waitFor("Selection #1 ")
	if !strings.Contains(content, "Selected Element: <button") {
		t.Errorf("alt+click on a button's icon must select the button: %s", content)
	}

	h.altClick("#logo path")
	content = h.waitFor("Selection #2 ")
	if !strings.Contains(content[:strings.Index(content, "Selection #1 ")], "Selected Element: <svg") {
		t.Errorf("alt+click on a bare icon must select the <svg>: %s", content)
	}
}
