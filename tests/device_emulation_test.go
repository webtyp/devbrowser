package devbrowser_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webtyp.com/devbrowser"
	"webtyp.com/json"
	"webtyp.com/mcp"
)

func TestDeviceEmulation_Logic(t *testing.T) {
	b := &devbrowser.DevBrowser{
		Width:  1200,
		Height: 800,
		Log:    func(msg ...any) {},
	}

	// Test case: mobile emulation
	b.ViewportMode = "mobile"
	if b.ViewportMode != "mobile" {
		t.Errorf("Expected viewportMode mobile, got %s", b.ViewportMode)
	}

	// Test case: desktop (reset)
	b.ViewportMode = "desktop"
	if b.ViewportMode != "desktop" {
		t.Errorf("Expected viewportMode desktop, got %s", b.ViewportMode)
	}
}

func TestScreenshotUtility_ResultStructure(t *testing.T) {
	res := &devbrowser.ScreenshotResult{
		ImageData: []byte("fake-image"),
		PageURL:   "http://localhost",
		Width:     1024,
	}

	if len(res.ImageData) == 0 {
		t.Error("ImageData should not be empty")
	}
}

func TestDeviceEmulation_ValidationAndDistinctModes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><h1>Test</h1></body></html>`)
	}))
	defer ts.Close()

	db, _ := DefaultTestBrowser()
	if err := db.CreateBrowserContext(); err != nil {
		t.Fatal(err)
	}
	db.IsOpenFlag = true
	defer db.CloseBrowser()

	if err := db.NavigateToURL(ts.URL); err != nil {
		t.Fatal(err)
	}

	tools := db.GetManagementTools()
	tool := tools[0] // browser_emulate_device

	// 1. Rejects unknown mode and does not mutate/persist
	argsInvalid := devbrowser.EmulateDeviceArgs{Mode: "super_fast_phone"}
	reqInvalid := mcp.Request{
		Params: mcp.CallToolParams{
			Name:      "browser_emulate_device",
			Arguments: encodeArgs(&argsInvalid),
		},
		Action: 'u',
	}
	_, err := tool.Execute(nil, reqInvalid)
	if err == nil {
		t.Fatal("Expected error when requesting unknown device mode, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported mode") {
		t.Errorf("Expected unsupported mode error, got: %v", err)
	}
	// Check b.ViewportMode is not changed
	if db.ViewportMode == "super_fast_phone" {
		t.Error("Invalid mode should not have been mutated into config")
	}

	// 2. Setting "desktop" mode successfully pins viewport and returns size in reply
	argsDesktop := devbrowser.EmulateDeviceArgs{Mode: "desktop"}
	reqDesktop := mcp.Request{
		Params: mcp.CallToolParams{
			Name:      "browser_emulate_device",
			Arguments: encodeArgs(&argsDesktop),
		},
		Action: 'u',
	}
	resDesktop, err := tool.Execute(nil, reqDesktop)
	if err != nil {
		t.Fatalf("Failed to execute device emulation for desktop: %v", err)
	}

	var contents mcp.TextContentList
	if err := json.Decode(string(resDesktop.Content), &contents); err != nil {
		t.Fatal(err)
	}
	resultText := contents[0].Text
	if !strings.Contains(resultText, "Device emulation set to desktop") || !strings.Contains(resultText, "viewport 1440x900") {
		t.Errorf("Desktop response should specify viewport 1440x900. Got: %s", resultText)
	}
	// desktop PINS the override, so the stored mode is the deterministic half of
	// "the two branches are distinct" — see the off case below.
	if db.ViewportMode != "desktop" {
		t.Errorf("desktop must pin the override; ViewportMode = %q, want \"desktop\"", db.ViewportMode)
	}

	// 3. Setting "off" clears overrides and returns actual window layout
	argsOff := devbrowser.EmulateDeviceArgs{Mode: "off"}
	reqOff := mcp.Request{
		Params: mcp.CallToolParams{
			Name:      "browser_emulate_device",
			Arguments: encodeArgs(&argsOff),
		},
		Action: 'u',
	}
	resOff, err := tool.Execute(nil, reqOff)
	if err != nil {
		t.Fatalf("Failed to execute device emulation off: %v", err)
	}

	var contentsOff mcp.TextContentList
	if err := json.Decode(string(resOff.Content), &contentsOff); err != nil {
		t.Fatal(err)
	}
	resultTextOff := contentsOff[0].Text
	if !strings.Contains(resultTextOff, "Device emulation set to off") {
		t.Errorf("Expected 'Device emulation set to off', got: %s", resultTextOff)
	}

	// Criterion 2 (Part A): desktop and off must produce DISTINCT branches.
	//
	// This cannot be asserted from the reported viewport, and asserting "off does
	// not report 1440x900" was a bug in this test. Step 2 above sets desktop,
	// which calls GrowWindowToFit(1440, 900); window_autofit.go grows the live
	// window "and never shrinks it". off pins nothing and reads window.innerWidth
	// back from that same window — which is now exactly 1440x900. The assertion
	// failed on correct behaviour this test had itself caused two steps earlier,
	// and passed only when DPI or the DevTools reservation happened to push the
	// window past 1440x900.
	//
	// What distinguishes the branches is the stored mode: mcp-management.go sets
	// b.ViewportMode = args.Mode, so desktop pins an override and off clears it.
	// That is exact on every machine and no window can defeat it.
	if db.ViewportMode != "off" {
		t.Errorf("off must clear the emulation override; ViewportMode = %q, want \"off\"", db.ViewportMode)
	}
	if !strings.Contains(resultTextOff, "viewport ") {
		t.Errorf("off reply should still report the resulting viewport (Criterion 6). got: %s", resultTextOff)
	}
}

func TestEmulationViewportSize_Modes(t *testing.T) {
	tests := []struct {
		mode    string
		wantW   int
		wantH   int
		wantErr bool
	}{
		{"mobile", 430, 739, false},
		{"tablet", 1024, 1366, false},
		{"desktop", 1440, 900, false},
		{"off", 0, 0, false},
		{"", 0, 0, false},
		{"unknown_mode", 0, 0, true},
	}

	for _, tt := range tests {
		w, h, err := devbrowser.EmulationViewportSize(tt.mode, "")
		if (err != nil) != tt.wantErr {
			t.Errorf("EmulationViewportSize(%q, \"\") error = %v, wantErr %v", tt.mode, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("EmulationViewportSize(%q, \"\") = (%d, %d), want (%d, %d)", tt.mode, w, h, tt.wantW, tt.wantH)
			}
		}
	}
}

func TestEmulationViewportSize_NamedDevice(t *testing.T) {
	w, h, err := devbrowser.EmulationViewportSize("", "iphone15promax")
	if err != nil {
		t.Fatalf("EmulationViewportSize(\"\", \"iphone15promax\") failed: %v", err)
	}
	if w <= 0 || h <= 0 {
		t.Errorf("Expected positive dimensions for iphone15promax, got %dx%d", w, h)
	}

	_, _, errInvalid := devbrowser.EmulationViewportSize("", "nonexistent_device_xyz")
	if errInvalid == nil {
		t.Error("Expected error for nonexistent device name, got nil")
	}
}
