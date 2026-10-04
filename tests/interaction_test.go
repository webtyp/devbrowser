package devbrowser_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webtyp.com/devbrowser/chromedp"
)

// TestBrowserInteraction verifies that we can click and fill elements
// using Chromedp, validating the logic used in the MCP tools.
func TestBrowserInteraction(t *testing.T) {
	// 1. Setup a test server with interactive elements
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `
			<!DOCTYPE html>
			<html>
			<body>
				<input id="test-input" type="text" value="">
				<button id="test-btn" onclick="document.getElementById('result').innerText = 'Clicked'">Click Me</button>
				<div id="result"></div>
			</body>
			</html>
		`)
	}))
	defer ts.Close()

	// 2. Setup browser context
	db, _ := NewTestBrowserWithContext(t)
	defer db.CloseBrowser()

	// 3. Test Fill (Typing)
	var inputValue string
	fillSelector := "#test-input"
	fillValue := "Hello World"

	err := chromedp.Run(db.Ctx,
		chromedp.Navigate(ts.URL),
		chromedp.WaitVisible(fillSelector, chromedp.ByQuery),
		chromedp.SendKeys(fillSelector, fillValue, chromedp.ByQuery),
		chromedp.Value(fillSelector, &inputValue, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("Failed to fill element: %v", err)
	}

	if inputValue != fillValue {
		t.Errorf("Expected input value '%s', got '%s'", fillValue, inputValue)
	}

	// 4. Test Click
	var resultText string
	clickSelector := "#test-btn"

	err = chromedp.Run(db.Ctx,
		chromedp.Click(clickSelector, chromedp.ByQuery),
		chromedp.Sleep(100*time.Millisecond), // Wait for JS execution
		chromedp.Text("#result", &resultText, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("Failed to click element: %v", err)
	}

	if resultText != "Clicked" {
		t.Errorf("Expected result text 'Clicked', got '%s'", resultText)
	}
}
