package devbrowser

func (h *DevBrowser) CloseBrowser() error {
	h.Mu.Lock()
	defer h.Mu.Unlock()

	wasOpen := h.IsOpenFlag
	h.IsOpenFlag = false
	h.ready = false
	h.pendingReload = false
	// The next browser context needs its listeners again. The selection
	// history stays: its snapshots are self-contained, and nextSeq keeps
	// counting so a badge number is never reused.
	h.selectionCaptureInstalled = false
	h.lastPanelNodeID = 0

	if h.Cancel != nil {
		h.Cancel()
		h.Cancel = nil
	}

	// Cancel the exec allocator too, otherwise the Chrome OS process/window
	// survives (only the tab/target is closed) and a restart spawns a second
	// window -> the about:blank "double window" bug.
	if h.AllocCancel != nil {
		h.AllocCancel()
		h.AllocCancel = nil
	}

	// Limpiar recursos
	h.Ctx = nil

	if wasOpen {
		h.Logger(h.StatusMessage())
		h.UI.RefreshUI()
	}
	return nil
}
