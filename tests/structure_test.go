package devbrowser_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webtyp.com/devbrowser"
	"webtyp.com/devbrowser/chromedp"
)

// TestPageStructureExtraction verifies that the JavaScript logic
// correctly extracts semantic structure, attributes, and layout info.
func TestPageStructureExtraction(t *testing.T) {
	// 1. Setup a test server with comprehensive HTML
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `
			<!DOCTYPE html>
			<html>
			<head><title>Test Page</title></head>
			<body>
				<header style="display: flex;">
					<h1>Site Title</h1>
					<nav>
						<a href="/home" title="Go Home">Home</a>
						<a href="/about">About</a>
					</nav>
				</header>
				<main style="display: grid; gap: 20px;">
					<section>
						<h2>Content Area</h2>
						<p>Some text.</p>
						<img src="logo.png" alt="Company Logo">
					</section>
					<div id="sidebar" style="position: absolute; top: 0; left: 0;">
						<button aria-label="Close">X</button>
					</div>
				</main>
			</body>
			</html>
		`)
	}))
	defer ts.Close()

	// 2. Setup browser context using standard test helper
	db, _ := NewTestBrowserWithContext(t)
	defer db.CloseBrowser()

	// 3. Navigate and Extract
	var structure string
	err := chromedp.Run(db.Ctx,
		chromedp.Navigate(ts.URL),
		chromedp.Sleep(500*time.Millisecond), // Wait for render
		chromedp.Evaluate(devbrowser.GetStructureJS, &structure),
	)

	if err != nil {
		t.Fatalf("Failed to extract structure: %v", err)
	}

	// 4. Verify Content
	t.Logf("Extracted Structure:\n%s", structure)

	expectedSubstrings := []string{
		"Site Title",         // Text content
		"display:flex",       // Flex detection
		"FLEX",               // Flex label
		"display:grid",       // Grid detection
		"GRID",               // Grid label
		`href="/home"`,       // Link attribute
		`title="Go Home"`,    // Link title
		`src="logo.png"`,     // Image src
		`alt="Company Logo"`, // Image alt
		`position:absolute`,  // Positioning
		`aria-label="Close"`, // ARIA
		"clickable",          // Interactive heuristic
	}

	for _, s := range expectedSubstrings {
		if !strings.Contains(structure, s) {
			t.Errorf("Expected structure to contain '%s', but it didn't.", s)
		}
	}
}
