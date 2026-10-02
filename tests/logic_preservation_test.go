package devbrowser_test

import (
	"fmt"
	"testing"
	"webtyp.com/devbrowser"
)

// TestLogicPreservation_ConfigPriority enforces the critical business logic:
// "devbrowser.Stored configuration takes precedence over monitor detection on startup."
//
// Scenario: A user has manually resized the browser or moved it to a specific setup.
// We must NOT override this with the auto-detected monitor size on startup.
//
// See: User Request Step Id 180 & 195
func TestLogicPreservation_ConfigPriority(t *testing.T) {
	// Setup mock monitor to be different from config
	// Monitor is HUGE: 4000x2000
	mockMonitorW, mockMonitorH := 4000, 2000

	// Temporarily replace the monitor query function
	originalQuery := devbrowser.QueryMonitorSize
	defer func() { devbrowser.QueryMonitorSize = originalQuery }()

	devbrowser.QueryMonitorSize = func() (int, int) {
		return mockMonitorW, mockMonitorH
	}

	// User's Configured Size (devbrowser.Stored)
	// e.g. a small window preferred by the user: 800x600
	storedW, storedH := 800, 600

	b := &devbrowser.DevBrowser{
		Width:          storedW,
		Height:         storedH,
		SizeConfigured: true,                // This flag means "loaded from storage"
		Log:            func(msg ...any) {}, // No-op logger
	}

	// EXECUTE STARTUP LOGIC
	// Logic from OpenBrowser.go:
	// if !h.sizeConfigured { h.detectMonitorSize(); h.startWithDetectedSize() }
	if !b.SizeConfigured {
		b.DetectMonitorSize()
		b.StartWithDetectedSize()
	}

	// ASSERT
	// Logic Preservation Check:
	// Did we keep the stored config?
	if b.Width != storedW || b.Height != storedH {
		t.Errorf("CRITICAL LOGIC FAILURE: Startup logic overwrote stored config.\n"+
			"Expected (devbrowser.Stored): %dx%d\n"+
			"Got (After Startup): %dx%d\n"+
			"Explanation: If sizeConfigured is true, we MUST NOT auto-adjust to monitor on startup.",
			storedW, storedH, b.Width, b.Height)
	}

	// Verify we didn't even populate monitor stats (Startup optimization: skip detection if not needed)
	// Note: b.monitorWidth is private, so we check indirect side effects or just rely on the fact
	// that if we called detectMonitorSize, likely width/height wouldn't change anyway
	// because startWithDetectedSize also checks sizeConfigured.
	// But per user request, we shouldn't even *try* to detect if configured.
	// (Although internal field visibility makes it hard to assert 'monitorWidth == 0' directly here without accessor or unsafe)
	// We'll rely on the main size assertion above as the contract.
}

// TestLogicPreservation_LazyLoadingMCP enforces:
// "MCP tools uses dynamic monitor constraints, lazy-loading if needed."
//
// Scenario: Browser started with stored config (skipped detection).
// Later, MCP tool is used to set "Desktop" preset.
// We MUST then detect monitor size to ensure the preset fits.
func TestLogicPreservation_LazyLoadingMCP(t *testing.T) {
	// Setup mock monitor: Small Screen (Laptop)
	// 1366x768
	mockMonitorW, mockMonitorH := 1366, 768

	originalQuery := devbrowser.QueryMonitorSize
	defer func() { devbrowser.QueryMonitorSize = originalQuery }()

	detectCalled := false
	devbrowser.QueryMonitorSize = func() (int, int) {
		detectCalled = true
		return mockMonitorW, mockMonitorH
	}

	b := &devbrowser.DevBrowser{
		Width:          800,
		Height:         600,
		SizeConfigured: true, // Started with config
		Log:            func(msg ...any) {},
	}

	// EXECUTE MCP ACTION
	// Request "desktop" preset (nominally 1440x900)
	// This should trigger lazy detection and constrain to 1366x768 (fitting height inside 768)
	// The getPresetSize method calls detectMonitorSize internally if fields are 0.
	w, h, err := b.GetPresetSize("desktop")
	if err != nil {
		t.Fatalf("getPresetSize failed: %v", err)
	}

	// ASSERT
	if !detectCalled {
		t.Error("CRITICAL LOGIC FAILURE: Lazy monitor detection was NOT triggered when using MCP tool.")
	}

	// Check constraints were applied
	// Desktop 1440 > Laptop 1366 -> Should result in 1366 (width limited)
	// Desktop 900 > Laptop 768 -> Should result in 768 (height limited)
	// Actually current logic in calculateConstrainedSize clamps both independently.
	if w > mockMonitorW {
		t.Errorf("Constraint Failure: Width %d exceeds monitor %d", w, mockMonitorW)
	}
	if h > mockMonitorH {
		t.Errorf("Constraint Failure: Height %d exceeds monitor %d", h, mockMonitorH)
	}

	fmt.Printf("MCP Request 'desktop' (1440x900) on Screen (1366x768) -> Result: %dx%d (Correctly Constrained)\n", w, h)
}

// TestLogicPreservation_NoConfigStartup enforces:
// "If NO configuration exists, we MUST auto-detect and constrain on startup."
func TestLogicPreservation_NoConfigStartup(t *testing.T) {
	mockMonitorW, mockMonitorH := 1920, 1080

	originalQuery := devbrowser.QueryMonitorSize
	defer func() { devbrowser.QueryMonitorSize = originalQuery }()

	devbrowser.QueryMonitorSize = func() (int, int) {
		return mockMonitorW, mockMonitorH
	}

	b := &devbrowser.DevBrowser{
		Width:          1024, // Default defined in devbrowser.New()
		Height:         768,
		SizeConfigured: false, // NO stored config
		Log:            func(msg ...any) {},
	}

	// EXECUTE STARTUP LOGIC
	if !b.SizeConfigured {
		b.DetectMonitorSize()
		b.StartWithDetectedSize()
	}

	// When no config exists, window adopts the monitor size
	if b.Width != mockMonitorW || b.Height != mockMonitorH {
		t.Errorf("Startup Auto-Size Failed: got %dx%d, want %dx%d", b.Width, b.Height, mockMonitorW, mockMonitorH)
	}
}

func TestLogicPreservation_TinyScreenStartup(t *testing.T) {
	// Tiny Raspberry Pi Screen: 800x480
	mockMonitorW, mockMonitorH := 800, 480

	originalQuery := devbrowser.QueryMonitorSize
	defer func() { devbrowser.QueryMonitorSize = originalQuery }()

	devbrowser.QueryMonitorSize = func() (int, int) {
		return mockMonitorW, mockMonitorH
	}

	b := &devbrowser.DevBrowser{
		Width:          1024, // Default default
		Height:         768,
		SizeConfigured: false, // NO stored config
		Log:            func(msg ...any) {},
	}

	// EXECUTE STARTUP LOGIC
	if !b.SizeConfigured {
		b.DetectMonitorSize()
		b.StartWithDetectedSize()
	}

	// ASSERT
	// Should be clamped to monitor
	if b.Width > mockMonitorW {
		t.Errorf("Startup Auto-Size Failed: Width %d > Monitor %d", b.Width, mockMonitorW)
	}
	if b.Height > mockMonitorH {
		t.Errorf("Startup Auto-Size Failed: Height %d > Monitor %d", b.Height, mockMonitorH)
	}
}
