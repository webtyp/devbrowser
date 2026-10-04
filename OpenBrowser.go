package devbrowser

import (
	"fmt"
	"time"

	"webtyp.com/devbrowser/chromedp"
)

// Open idempotently opens the browser at rawURL.
// If the browser is already open and responsive, it navigates to rawURL without spawning a new window.
func (h *DevBrowser) Open(rawURL string) error {
	h.Mu.Lock()
	isFirst := h.FirstCall
	h.FirstCall = false
	h.LastURL = rawURL

	// If already open and alive, just navigate to the requested URL (idempotent)
	if h.IsOpenFlag && h.Ctx != nil && h.Ctx.Err() == nil {
		h.Mu.Unlock()
		return h.NavigateToURL(rawURL)
	}

	// Logic: on first call, only open if autoStart is true.
	// On subsequent calls (e.g. user action), always open.
	if isFirst && !h.AutoStart {
		h.Mu.Unlock()
		return nil
	}

	if h.TestMode {
		h.OpenedOnce = true
		h.Mu.Unlock()
		h.Logger("Skipping browser open in TestMode")
		return nil
	}

	h.IsOpenFlag = true
	h.OpenedOnce = true
	h.Mu.Unlock()

	// Add listener for exit signal (only once per open session)
	go func() {
		<-h.ExitChan
		h.CloseBrowser()
	}()

	go func() {
		// Detect monitor size and apply constraints ONLY if not already configured.
		// If configured, we respect the user's stored preferences (which might be manual resized).
		if !h.SizeConfigured {
			h.DetectMonitorSize()
			h.StartWithDetectedSize()
		}

		err := h.CreateBrowserContext()
		if err != nil {
			h.ErrChan <- err
			return
		}

		// Initialize console log capturing BEFORE navigating to the page
		// This ensures all console.log statements from page load are captured
		if err := h.initializeConsoleCapture(); err != nil {
			h.Logger("Warning: failed to initialize console capture:", err)
			// Continue anyway - capture is optional
		}
		h.initializeNetworkCapture()
		h.initializeErrorCapture()
		h.initializeInterceptCapture()
		h.initializeInspectCapture()

		if err := chromedp.Run(h.Ctx,
			chromedp.Navigate(rawURL),
			chromedp.WaitReady("body"),
			chromedp.Evaluate(injectInspectListenerJS, nil),
		); err != nil {
			h.ErrChan <- fmt.Errorf("error navigating to %s: %v", rawURL, err)
			return
		}

		// Wait an extra moment to ensure rendering completes
		time.Sleep(100 * time.Millisecond)

		// Restore device emulation if set
		h.Mu.Lock()
		vMode := h.ViewportMode
		h.Mu.Unlock()
		if vMode != "" && vMode != "off" && vMode != "desktop" {
			if err := h.applyDeviceEmulation(); err != nil {
				h.Logger(fmt.Sprintf("Failed to restore emulation: %v", err))
			}
		}

		// Mark the browser as fully ready: the context now has an allocated
		// browser (the Navigate above forced allocation). Only now is it safe
		// for other goroutines (e.g. the file watcher's Reload) to issue
		// chromedp actions without triggering a second allocation.
		h.Mu.Lock()
		h.ready = true
		h.Mu.Unlock()

		h.ProcessPendingReload()

		h.ReadyChan <- true

		// Monitor browser context for manual close
		go h.monitorBrowserClose()
	}()

	// Wait for start signal or error
	select {
	case err := <-h.ErrChan:
		h.Logger("Error opening DevBrowser: ", err)
		h.CloseBrowser()
		return err
	case <-h.ReadyChan:
		h.Logger(h.StatusMessage())

		// Start monitoring browser geometry changes
		go h.monitorBrowserGeometry()

		h.UI.RefreshUI()
		return nil
	}
}

// OpenBrowser opens the browser on the given port and scheme.
// It formats the URL and delegates to Open, preserving backward compatibility.
func (h *DevBrowser) OpenBrowser(port string, https bool) {
	if port == "" {
		return
	}
	h.Mu.Lock()
	h.LastPort = port
	h.LastHttps = https
	h.Mu.Unlock()

	protocol := "http"
	if https {
		protocol = "https"
	}
	url := protocol + "://localhost:" + port + "/"
	_ = h.Open(url)
}
