package devbrowser

import (
	"fmt"
	"time"

	"webtyp.com/devbrowser/chromedp"
)

func (h *DevBrowser) OpenBrowser(port string, https bool) {
	h.Mu.Lock()
	isFirst := h.FirstCall
	h.FirstCall = false
	h.LastPort = port
	h.LastHttps = https

	//h.Logger(fmt.Sprintf("DEBUG: OpenBrowser called. port=%s, https=%v, isOpen=%v, firstCall=%v", port, https, h.IsOpenFlag, isFirst))

	// Logic: on first call, only open if autoStart is true.
	// On subsequent calls (e.g. user action), always open.
	if isFirst && !h.AutoStart {
		//h.Logger("DEBUG: OpenBrowser skipped on first call (autoStart=false)")
		h.Mu.Unlock()
		return
	}

	if h.IsOpenFlag {
		h.Mu.Unlock()
		return
	}

	if h.TestMode {
		h.OpenedOnce = true
		h.Mu.Unlock()
		h.Logger("Skipping browser open in TestMode")
		return
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

		protocol := "http"
		if https {
			protocol = "https"
		}
		url := protocol + `://localhost:` + port + "/"

		// Initialize console log capturing BEFORE navigating to the page
		// This ensures all console.log statements from page load are captured
		if err := h.initializeConsoleCapture(); err != nil {
			h.Logger("Warning: failed to initialize console capture:", err)
			// Continue anyway - capture is optional
		}
		h.initializeNetworkCapture()
		h.initializeErrorCapture()
		h.initializeInterceptCapture()

		if err := chromedp.Run(h.Ctx,
			chromedp.Navigate(url),
			chromedp.WaitReady("body"),
		); err != nil {
			h.ErrChan <- fmt.Errorf("error navigating to %s: %v", url, err)
			return
		}

		// Esperar un momento adicional para asegurar que todo esté cargado
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

	// Esperar señal de inicio o error
	select {
	case err := <-h.ErrChan:
		// use helper to ensure logging goes through configured logger
		h.Logger("Error opening DevBrowser: ", err)
		h.CloseBrowser()
		return
	case <-h.ReadyChan:
		h.Logger(h.StatusMessage())

		// Start monitoring browser geometry changes
		go h.monitorBrowserGeometry()

		h.UI.RefreshUI()
		return
	}
}
