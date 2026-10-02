package devbrowser

import (
	"fmt"
)

// Store keys for browser configuration
const (
	StoreKeyBrowserAutostart   = "browser_autostart"
	StoreKeyBrowserPosition    = "browser_position"
	StoreKeyBrowserSize        = "browser_size"
	StoreKeyBrowserViewport    = "browser_viewport"
	StoreKeyLegacyViewportMode = "viewport_mode" // legacy fallback
)

// LoadConfig loads all browser configuration from the store
func (b *DevBrowser) LoadConfig() {
	// Load auto-start setting
	if val, err := b.DB.Get(StoreKeyBrowserAutostart); err == nil && val != "" {
		// Handle legacy values: convert "true"/"false" to "t"/"f"
		switch val {
		case "true", "t":
			b.AutoStart = true
			// Migrate legacy value if needed
			if val == "true" {
				b.DB.Set(StoreKeyBrowserAutostart, "t")
			}
		case "false", "f":
			b.AutoStart = false
			// Migrate legacy value if needed
			if val == "false" {
				b.DB.Set(StoreKeyBrowserAutostart, "f")
			}
		default:
			b.AutoStart = true // Default if unknown value
		}
	} else {
		b.AutoStart = true // Default: auto-start enabled
	}

	// Load position
	if pos, err := b.DB.Get(StoreKeyBrowserPosition); err == nil && pos != "" {
		b.Position = pos
	}

	// Load size (width,height)
	if size, err := b.DB.Get(StoreKeyBrowserSize); err == nil && size != "" {
		var w, h int
		if _, err := fmt.Sscanf(size, "%d,%d", &w, &h); err == nil {
			b.Width = w
			b.Height = h
			b.SizeConfigured = true
		}
	}

	// Load browser viewport (with fallback to legacy viewport_mode)
	if mode, err := b.DB.Get(StoreKeyBrowserViewport); err == nil && mode != "" {
		b.ViewportMode = mode
	} else if legacyMode, err := b.DB.Get(StoreKeyLegacyViewportMode); err == nil && legacyMode != "" {
		b.ViewportMode = legacyMode
		_ = b.DB.Set(StoreKeyBrowserViewport, legacyMode)
	}
}

// SaveConfig saves all browser configuration to the store
func (b *DevBrowser) SaveConfig() error {
	// Save position
	if err := b.DB.Set(StoreKeyBrowserPosition, b.Position); err != nil {
		return err
	}

	// Save size
	size := fmt.Sprintf("%d,%d", b.Width, b.Height)
	if err := b.DB.Set(StoreKeyBrowserSize, size); err != nil {
		return err
	}

	// Save browser viewport (desktop, mobile, tablet)
	mode := b.ViewportMode
	if mode == "" || mode == "off" {
		mode = "desktop"
	}
	if err := b.DB.Set(StoreKeyBrowserViewport, mode); err != nil {
		return err
	}

	return nil
}
