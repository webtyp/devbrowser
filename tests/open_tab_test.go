package devbrowser_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"webtyp.com/devbrowser"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
	"webtyp.com/model"
)

func TestOpenTab_RefusesBeforeTheBrowserIsOpen(t *testing.T) {
	db, _ := DefaultTestBrowser()
	if err := db.OpenTab("http://user2.localhost:6060/"); err == nil {
		t.Fatal("OpenTab on a closed browser returned nil, want an error")
	}
}

func TestOpenTab_ExtraTabLoadsAndReloadsWithTheMainOne(t *testing.T) {
	var extraHits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/extra" {
			extraHits.Add(1)
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!doctype html><body>ok</body>")
	}))
	defer ts.Close()

	db, _ := DefaultTestBrowser()
	if err := db.Open(ts.URL + "/"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.CloseBrowser()

	if err := db.OpenTab(ts.URL + "/extra"); err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	if got := extraHits.Load(); got != 1 {
		t.Fatalf("extra tab requests after OpenTab = %d, want 1", got)
	}
	if err := db.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for extraHits.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := extraHits.Load(); got != 2 {
		t.Errorf("extra tab requests after Reload = %d, want 2", got)
	}
}

// callTool runs the MCP tool name with args on db and returns its text.
func callTool(t *testing.T, db *devbrowser.DevBrowser, name string, args model.Encodable) string {
	t.Helper()
	for _, tool := range db.GetMCPTools() {
		if tool.Name != name {
			continue
		}
		res, err := tool.Execute(nil, mcp.Request{Params: mcp.CallToolParams{Name: name, Arguments: encodeArgs(args)}, Action: 'u'})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return res.Content
	}
	t.Fatalf("tool %s not found", name)
	return ""
}

// waitTool polls the MCP tool until its text contains want.
func waitTool(t *testing.T, db *devbrowser.DevBrowser, name string, args model.Encodable, want string) string {
	t.Helper()
	var got string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got = callTool(t, db, name, args); strings.Contains(got, want) {
			return got
		}
		time.Sleep(100 * time.Millisecond)
	}
	return got
}

func TestOpenTab_ToolsFollowTheTabInFront(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Each page logs its name; /extra then comes to the front on its own
		// (a focus event, what clicking into a tab fires).
		fmt.Fprintf(w, `<!doctype html><body><script>console.log("from%s");`, r.URL.Path)
		if r.URL.Path == "/extra" {
			fmt.Fprint(w, `setTimeout(() => window.dispatchEvent(new Event("focus")), 200);`)
		}
		fmt.Fprint(w, `</script></body>`)
	}))
	defer ts.Close()

	db, _ := DefaultTestBrowser()
	if err := db.Open(ts.URL + "/main"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.CloseBrowser()
	if err := db.OpenTab(ts.URL + "/extra"); err != nil {
		t.Fatalf("OpenTab: %v", err)
	}

	where := &devbrowser.EvaluateJSArgs{Script: "location.pathname"}
	if got := waitTool(t, db, "browser_evaluate_js", where, "/extra"); !strings.Contains(got, "/extra") {
		t.Fatalf("with /extra in front, evaluate ran on %q", got)
	}
	logs := callTool(t, db, "browser_get_console", &devbrowser.GetConsoleArgs{})
	if !strings.Contains(logs, "from/extra") || strings.Contains(logs, "from/main") {
		t.Errorf("with /extra in front, console = %q; want only the /extra tab's lines", logs)
	}

	// The developer clicks back into the main tab.
	if err := chromedp.Run(db.Ctx, chromedp.Evaluate(`window.dispatchEvent(new Event("focus"))`, nil)); err != nil {
		t.Fatal(err)
	}
	if got := waitTool(t, db, "browser_evaluate_js", where, "/main"); !strings.Contains(got, "/main") {
		t.Fatalf("with /main in front, evaluate ran on %q", got)
	}
	logs = callTool(t, db, "browser_get_console", &devbrowser.GetConsoleArgs{})
	if !strings.Contains(logs, "from/main") || strings.Contains(logs, "from/extra") {
		t.Errorf("with /main in front, console = %q; want only the main tab's lines", logs)
	}
}

// tabServer serves a page per path and counts the requests to each path.
func tabServer(t *testing.T) (*httptest.Server, func(path string) int32) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int32{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == "/api" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!doctype html><body>%s</body>`, r.URL.Path)
	}))
	t.Cleanup(ts.Close)
	return ts, func(path string) int32 {
		mu.Lock()
		defer mu.Unlock()
		return hits[path]
	}
}

func waitHits(hits func(string) int32, path string, want int32) int32 {
	deadline := time.Now().Add(5 * time.Second)
	for hits(path) < want && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	return hits(path)
}

func TestOpenTab_RestartBrowserReopensTheTabs(t *testing.T) {
	ts, hits := tabServer(t)
	db, _ := DefaultTestBrowser()
	if err := db.Open(ts.URL + "/main"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.CloseBrowser()
	if err := db.OpenTab(ts.URL + "/extra"); err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	if err := db.RestartBrowser(); err != nil {
		t.Fatalf("RestartBrowser: %v", err)
	}
	if got := waitHits(hits, "/extra", 2); got != 2 {
		t.Errorf("/extra requested %d times after a restart, want 2 (the tab comes back)", got)
	}
}

func TestOpenTab_CloseTabsKeepsOnlyTheMainOne(t *testing.T) {
	ts, hits := tabServer(t)
	db, _ := DefaultTestBrowser()
	if err := db.Open(ts.URL + "/main"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.CloseBrowser()
	if err := db.OpenTab(ts.URL + "/extra"); err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	db.CloseTabs()
	if err := db.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := waitHits(hits, "/main", 2); got != 2 {
		t.Fatalf("/main requested %d times, want 2 (reloaded)", got)
	}
	if got := hits("/extra"); got != 1 {
		t.Errorf("/extra requested %d times after CloseTabs and a reload, want 1 (the tab is gone)", got)
	}
}

func TestOpenTab_InterceptionRunsOnTheTabInFront(t *testing.T) {
	ts, _ := tabServer(t)
	db, _ := DefaultTestBrowser()
	if err := db.Open(ts.URL + "/main"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.CloseBrowser()
	if err := db.OpenTab(ts.URL + "/extra"); err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	where := &devbrowser.EvaluateJSArgs{Script: `(window.dispatchEvent(new Event("focus")), location.pathname)`}
	if got := waitTool(t, db, "browser_evaluate_js", where, "/extra"); !strings.Contains(got, "/extra") {
		t.Fatalf("tools are not on /extra: %q", got)
	}

	callTool(t, db, "browser_intercept_request", &devbrowser.InterceptRequestArgs{Action: "start"})
	callTool(t, db, "browser_evaluate_js", &devbrowser.EvaluateJSArgs{Script: `fetch("/api").then(r => r.text())`, AwaitPromise: true})
	got := waitTool(t, db, "browser_intercept_request", &devbrowser.InterceptRequestArgs{Action: "get"}, "/api")
	callTool(t, db, "browser_intercept_request", &devbrowser.InterceptRequestArgs{Action: "stop"})
	if !strings.Contains(got, "200 GET") || !strings.Contains(got, "/api") || !strings.Contains(got, "Response Body") {
		t.Errorf("interception on the tab in front saw %q; want the /api request with its body", got)
	}
}
