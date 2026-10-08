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
// cancel below still runs, so a hung Chrome is killed and no second window
// survives (the about:blank "double window" bug).
func (h *DevBrowser) shutdownChrome() {
	// Only a context that actually started Chrome can close it gracefully.
	// Without a browser, chromedp's "allocated" channel is a one-token
	// semaphore: chromedp.Cancel would take the token and h.Cancel below would
	// then wait for a second one forever.
	if h.Ctx != nil {
		if c := chromedp.FromContext(h.Ctx); c != nil && c.Browser != nil {
			closeCtx, closeCancel := context.WithTimeout(h.Ctx, gracefulCloseTimeout)
			_ = chromedp.Cancel(closeCtx)
			closeCancel()
		}
		h.Ctx = nil
	}
	// The allocator goes BEFORE the context cancel. When the graceful close
	// fails, chromedp has already marked the browser as closing itself and
	// returns without killing it; the context cancel then waits for the
	// process to exit, forever. Cancelling the allocator kills a Chrome that is
	// still alive (a no-op when it already exited), so the wait below ends.
	if h.AllocCancel != nil {
		h.AllocCancel()
		h.AllocCancel = nil
	}
	if h.Cancel != nil {
		h.Cancel()
		h.Cancel = nil
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
