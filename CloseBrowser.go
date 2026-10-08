package devbrowser

import (
	"context"
	"time"

	"webtyp.com/devbrowser/chromedp"
)

const gracefulCloseTimeout = 3 * time.Second

// shutdownChrome asks Chrome to close itself first (Browser.close): a process
// killed by the allocator records exit_type "Crashed" in the profile, and the
// next start shows the "Restore pages?" bubble over DevTools. The allocator
// cancel below stays as the last step, so a hung Chrome is still killed and no
// second window survives (the about:blank "double window" bug).
func (h *DevBrowser) shutdownChrome() {
	if h.Ctx != nil {
		closeCtx, closeCancel := context.WithTimeout(h.Ctx, gracefulCloseTimeout)
		_ = chromedp.Cancel(closeCtx)
		closeCancel()
		h.Ctx = nil
	}
	if h.Cancel != nil {
		h.Cancel()
		h.Cancel = nil
	}
	if h.AllocCancel != nil {
		h.AllocCancel()
		h.AllocCancel = nil
	}
}

func (h *DevBrowser) CloseBrowser() error {
	h.Mu.Lock()
	defer h.Mu.Unlock()

	wasOpen := h.IsOpenFlag
	h.IsOpenFlag = false
	h.ready = false
	h.pendingReload = false
	// The next browser context needs its listeners again; the history is cleared
	// by the first navigation of the new context.
	h.selectionCaptureInstalled = false
	h.lastPanelNodeID = 0

	h.shutdownChrome()

	if wasOpen {
		h.Logger(h.StatusMessage())
		h.UI.RefreshUI()
	}
	return nil
}
