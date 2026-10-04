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

func TestSelectedElement_Execution(t *testing.T) {
	// 1. Setup a test server serving an element with WebTyp attributes
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `
			<!DOCTYPE html>
			<html>
			<head><title>WebTyp Selected Test</title></head>
			<body>
				<div id="app">
					<nav>
						<button id="18" class="stepindicator__step" type="button" data-key="18">3 Artefactos</button>
					</nav>
				</div>
			</body>
			</html>
		`)
	}))
	defer ts.Close()

	// 2. Setup headless Chromedp browser
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.DisableGPU,
		chromedp.ExecPath(devbrowser.ResolveChromeExecPath()),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	if err := chromedp.Run(ctx,
		chromedp.Navigate(ts.URL),
		chromedp.WaitVisible("[id='18']"),
	); err != nil {
		t.Fatalf("failed to navigate to test server: %v", err)
	}

	db, _ := DefaultTestBrowser()
	db.IsOpenFlag = true
	db.Ctx = ctx
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

	// Step A: Initially no element is selected ($0 is undefined)
	t.Log("Before Step A Execute")
	resNoSel, err := tool.Execute(nil, req)
	t.Logf("After Step A Execute, err=%v", err)
	if err != nil {
		t.Fatalf("unexpected error when no selection: %v", err)
	}
	if !strings.Contains(resNoSel.Content, "No element currently selected in DevTools") {
		t.Errorf("expected guidance message, got: %s", resNoSel.Content)
	}

	// Step B: Simulate developer selecting the button in DevTools ($0 = element)
	t.Log("Before Step B Evaluate")
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`window.$0 = document.getElementById("18")`, nil),
	)
	t.Logf("After Step B Evaluate, err=%v", err)
	if err != nil {
		t.Fatalf("failed to simulate $0 selection: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Step C: Execute tool again with $0 assigned
	t.Log("Before Step C Execute")
	resSel, err := tool.Execute(nil, req)
	t.Logf("After Step C Execute, err=%v", err)
	if err != nil {
		t.Fatalf("unexpected error when $0 is selected: %v", err)
	}

	content := resSel.Content
	// Verify WebTyp attributes are extracted
	if !strings.Contains(content, "id=\\\"18\\\"") && !strings.Contains(content, `id="18"`) {
		t.Errorf("report does not contain id='18': %s", content)
	}
	if !strings.Contains(content, "data-key=\\\"18\\\"") && !strings.Contains(content, `data-key="18"`) {
		t.Errorf("report does not contain data-key='18': %s", content)
	}
	if !strings.Contains(content, "stepindicator__step") {
		t.Errorf("report does not contain stepindicator__step class: %s", content)
	}
	if !strings.Contains(content, "3 Artefactos") {
		t.Errorf("report does not contain visible text: %s", content)
	}

	// Verify image screenshot is present
	if !strings.Contains(content, "image/png") {
		t.Errorf("expected image/png in result content, got: %s", content)
	}
}
