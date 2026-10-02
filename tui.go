package devbrowser

import "sync/atomic"

const (
	shortcutBrowserToggle = "B"
)

func (h *DevBrowser) Name() string {
	return "BROWSER"
}

func (h *DevBrowser) Label() string {
	if h.IsOpenFlag {
		return "Hide Browser"
	}
	return "Show Browser"
}

// StatusMessage returns formatted browser status for logging
func (h *DevBrowser) StatusMessage() string {
	state := "Closed"
	if h.IsOpenFlag {
		state = "Open"
	}
	return state + " | Shortcut B"
}

// Execute handles action button activation or shortcut trigger to toggle browser state
func (h *DevBrowser) Execute() {
	go func() {
		if !atomic.CompareAndSwapInt32(&h.Busy, 0, 1) {
			// Prevent spamming / re-entrant calls
			return
		}
		defer atomic.StoreInt32(&h.Busy, 0)

		if h.IsOpenFlag {
			if err := h.CloseBrowser(); err != nil {
				h.Logger("Close error:", err.Error())
			}
		} else {
			h.OpenBrowser(h.LastPort, h.LastHttps)
		}
		// Note: OpenBrowser and CloseBrowser log StatusMessage internally and call RefreshUI
	}()
}

// Shortcuts registers "B" for browser toggle
func (h *DevBrowser) Shortcuts() []map[string]string {
	return []map[string]string{
		{"B": "toggle browser"},
	}
}
