package devbrowser_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"webtyp.com/devbrowser/chromedp"
)

// The daemon relaunch cut browser.wasm mid-download and the developer was left with a blank page that only a manual
// reload fixed. Real Chrome against a real server that drops the first request for the app script: the page must
// finish loading by itself.
func TestOpen_PageRecoversWhenTheServerCutsAResource(t *testing.T) {
	var cuts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app.js":
			if cuts.Add(1) == 1 { // the first download dies half way, as when the server restarts
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
				return
			}
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, `document.title = "loaded"`)
		default:
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><title>loading</title><body><script src="/app.js"></script></body>`)
		}
	}))
	defer ts.Close()

	db, _ := DefaultTestBrowser()
	if err := db.Open(ts.URL + "/"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.CloseBrowser()

	deadline := time.Now().Add(15 * time.Second)
	title := ""
	for time.Now().Before(deadline) && title != "loaded" {
		time.Sleep(200 * time.Millisecond)
		_ = chromedp.Run(db.Ctx, chromedp.Title(&title))
	}
	if title != "loaded" {
		t.Fatalf("the page never recovered, title = %q after %d requests for app.js", title, cuts.Load())
	}
}
