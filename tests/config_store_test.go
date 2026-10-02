package devbrowser_test

import (
	"testing"

	"webtyp.com/devbrowser"
)

func TestConfig_BrowserViewportSaveAndLoad(t *testing.T) {
	store := &defaultStore{m: map[string]string{
		"browser_position": "100,100",
		"browser_size":     "1280,720",
	}}
	exitChan := make(chan bool)
	b := devbrowser.New(defaultUI{}, store, exitChan)

	// Default ViewportMode is empty, SaveConfig should save it as "desktop"
	if err := b.SaveConfig(); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	val, err := store.Get("browser_viewport")
	if err != nil || val != "desktop" {
		t.Errorf("Expected browser_viewport=desktop, got %q (err=%v)", val, err)
	}

	// Change to mobile and save
	b.ViewportMode = "mobile"
	if err := b.SaveConfig(); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	val, err = store.Get("browser_viewport")
	if err != nil || val != "mobile" {
		t.Errorf("Expected browser_viewport=mobile, got %q (err=%v)", val, err)
	}

	// Create new DevBrowser instance and LoadConfig
	b2 := devbrowser.New(defaultUI{}, store, exitChan)
	b2.LoadConfig()
	if b2.ViewportMode != "mobile" {
		t.Errorf("Expected loaded ViewportMode to be mobile, got %q", b2.ViewportMode)
	}
}

func TestConfig_LegacyViewportModeMigration(t *testing.T) {
	// Store with legacy viewport_mode key
	store := &defaultStore{m: map[string]string{
		"viewport_mode": "tablet",
	}}
	exitChan := make(chan bool)
	b := devbrowser.New(defaultUI{}, store, exitChan)

	b.LoadConfig()
	if b.ViewportMode != "tablet" {
		t.Errorf("Expected legacy viewport_mode to be loaded as tablet, got %q", b.ViewportMode)
	}

	// Check that it was migrated to browser_viewport
	val, err := store.Get("browser_viewport")
	if err != nil || val != "tablet" {
		t.Errorf("Expected migrated browser_viewport=tablet, got %q (err=%v)", val, err)
	}
}
