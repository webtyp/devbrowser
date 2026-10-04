package devbrowser_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"webtyp.com/devbrowser"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

// TestBrowserSwipe verifies that the browser_swipe_element tool performs drag actions.
func TestBrowserSwipe(t *testing.T) {
	// 1. Setup a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `
			<!DOCTYPE html>
			<html>
			<style>
				#slider {
					Width: 300px;
					Height: 20px;
					background: #ccc;
					position: relative;
				}
				#handle {
					Width: 20px;
					Height: 20px;
					background: red;
					position: absolute;
					left: 0;
					top: 0;
					cursor: pointer;
				}
			</style>
			<body>
				<div id="slider">
					<div id="handle"></div>
				</div>
				<script>
					const handle = document.getElementById('handle');
					let isDragging = false;
					
					handle.addEventListener('mousedown', (e) => {
						isDragging = true;
					});
					
					document.addEventListener('mousemove', (e) => {
						if (isDragging) {
							// Simple horizontal drag logic
							let newLeft = e.clientX - 10; // offset
							if (newLeft < 0) newLeft = 0;
							if (newLeft > 280) newLeft = 280;
							handle.style.left = newLeft + 'px';
						}
					});
					
					document.addEventListener('mouseup', () => {
						isDragging = false;
					});
				</script>
			</body>
			</html>
		`)
	}))
	defer ts.Close()

	// 2. Setup devbrowser with live browser context
	db, _ := NewTestBrowserWithContext(t)
	defer db.CloseBrowser()
	db.Width = 1024
	db.Height = 768

	// Navigate
	if err := chromedp.Run(db.Ctx, chromedp.Navigate(ts.URL)); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	// 4. Get the swipe tool
	tools := db.GetInteractionTools()
	var swipeTool *mcp.Tool
	for i := range tools {
		if tools[i].Name == "browser_swipe_element" {
			swipeTool = &tools[i]
			break
		}
	}

	if swipeTool == nil {
		t.Fatal("browser_swipe_element tool not found")
	}

	// 5. Execute swipe: Swipe right by 100px on the handle
	args := devbrowser.SwipeElementArgs{Selector: "#handle", Direction: "right", Distance: 100}
	req := mcp.Request{
		Params: mcp.CallToolParams{
			Name:      "browser_swipe_element",
			Arguments: encodeArgs(&args),
		},
		Action: 'u',
	}
	_, err := swipeTool.Execute(nil, req)
	if err != nil {
		t.Fatalf("Swipe failed: %v", err)
	}

	// 6. Verify handle moved
	var leftValue string
	err = chromedp.Run(db.Ctx,
		chromedp.Evaluate(`document.getElementById('handle').style.left`, &leftValue),
	)
	if err != nil {
		t.Fatalf("Failed to check handle position: %v", err)
	}

	if leftValue == "0px" || leftValue == "" {
		t.Errorf("Expected handle to move, but left is %s", leftValue)
	}
}
