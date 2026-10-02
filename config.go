package devbrowser

import (
	"fmt"
)

// Store keys for browser configuration
const (
	StoreKeyBrowserAutostart = "browser_autostart"
	StoreKeyBrowserPosition  = "browser_position"
	StoreKeyBrowserSize      = "browser_size"
	StoreKeyViewportMode     = "viewport_mode"
	StoreKeyViewportDevice   = "viewport_device"
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

	// Load viewport mode
	if mode, err := b.DB.Get(StoreKeyViewportMode); err == nil && mode != "" {
		b.ViewportMode = mode
	}

	// Load viewport device
	if device, err := b.DB.Get(StoreKeyViewportDevice); err == nil && device != "" {
		b.ViewportDevice = device
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

	// Save viewport mode only when non-empty
	if b.ViewportMode != "" {
		if err := b.DB.Set(StoreKeyViewportMode, b.ViewportMode); err != nil {
			return err
		}
	}

	// Save viewport device only when non-empty
	if b.ViewportDevice != "" {
		if err := b.DB.Set(StoreKeyViewportDevice, b.ViewportDevice); err != nil {
			return err
		}
	}

	return nil
}
