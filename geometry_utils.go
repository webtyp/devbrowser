package devbrowser

import (
	"fmt"
)

// calculateConstrainedSize determines the best window size based on available monitor space
// and previous configuration.
//
// logic:
// 1. If monitor size is unknown (0x0), return requested size
// 2. If valid monitor size:
//   - If !sizeConfigured (first run or auto):
//     Accept requested size BUT ensure it fits within monitor (clamping)
//   - If sizeConfigured (user manually set size):
//     Check if it fits. If not, scale down or clamp. To be safe, we clamp to available area.
//
// In all cases, we ensure the window is not larger than the available screen area.
func (b *DevBrowser) CalculateConstrainedSize(reqW, reqH, monW, monH int) (int, int) {
	if monW <= 0 || monH <= 0 {
		return reqW, reqH
	}

	finalW, finalH := reqW, reqH

	// Constrain width
	if finalW > monW {
		finalW = monW
	}

	// Constrain height
	if finalH > monH {
		finalH = monH
	}

	// If we modified dimensions, we might want to log it (caller handles logging)
	return finalW, finalH
}

// StartWithDetectedSize updates the browser size to the monitor size
// if it hasn't been configured yet.
func (b *DevBrowser) StartWithDetectedSize() {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	// If user already has a saved config, respect it
	if b.SizeConfigured {
		return
	}

	// If no monitor detected, keep defaults
	if b.MonitorWidth == 0 || b.MonitorHeight == 0 {
		return
	}

	b.Width = b.MonitorWidth
	b.Height = b.MonitorHeight
	b.Log(fmt.Sprintf("Browser size auto-adjusted to monitor: %dx%d", b.Width, b.Height))
}

// DevToolsReservedWidth is a conservative, documented ESTIMATE of the
// horizontal space Chrome's docked DevTools panel occupies when auto-opened
// via auto-open-devtools-for-tabs (see context.go). CDP exposes no command to
// read the panel's actual bounds, so this value is fixed and deliberately
// generous rather than pixel-exact — it assumes DevTools docks to the right,
// which only happens for the wide windows that trigger the auto-open in the
// first place. DevTools docked to the bottom is deliberately NOT modelled:
// that layout costs height rather than width, and the auto-open condition
// never fires for the narrow windows where Chrome docks it there.
const DevToolsReservedWidth = 420

// RequiredWindowSize returns the physical window size needed to show a
// requested viewport of (reqW, reqH) without DevTools covering any of it,
// clamped to the detected monitor. It never returns less than the window's
// current size: growth is one-directional, the existing window size is a
// floor. If the monitor size has not been detected yet, it is detected now
// (mirrors the lazy-detect in GetPresetSize).
func (b *DevBrowser) RequiredWindowSize(reqW, reqH int) (int, int) {
	b.Mu.Lock()
	monW, monH := b.MonitorWidth, b.MonitorHeight
	b.Mu.Unlock()

	if monW == 0 || monH == 0 {
		b.DetectMonitorSize()
		b.Mu.Lock()
		monW, monH = b.MonitorWidth, b.MonitorHeight
		b.Mu.Unlock()
	}

	b.Mu.Lock()
	curW, curH := b.Width, b.Height
	reserved := b.DevToolsReserved
	b.Mu.Unlock()

	neededW := reqW
	if reserved {
		neededW += DevToolsReservedWidth
	}
	neededH := reqH

	if neededW < curW {
		neededW = curW
	}
	if neededH < curH {
		neededH = curH
	}

	return b.CalculateConstrainedSize(neededW, neededH, monW, monH)
}

// getPresetSize calculates the optimal dimensions for a requested preset mode.
// It uses predefined base sizes but ensures they fit within the current monitor constraints.
func (b *DevBrowser) GetPresetSize(mode string) (int, int, error) {
	b.Mu.Lock()
	monW := b.MonitorWidth
	monH := b.MonitorHeight
	b.Mu.Unlock()

	// If monitor size not detected yet (lazy load), try to detect it now
	if monW == 0 || monH == 0 {
		b.DetectMonitorSize()
		// Re-read
		b.Mu.Lock()
		monW = b.MonitorWidth
		monH = b.MonitorHeight
		b.Mu.Unlock()
	}

	var baseW, baseH int

	// Base definitions
	switch mode {
	case "desktop":
		if monW > 0 && monH > 0 {
			return monW, monH, nil
		}
		baseW, baseH = 1024, 768
	case "mobile":
		baseW, baseH = 375, 812
	case "tablet":
		baseW, baseH = 768, 1024
	default:
		return 0, 0, fmt.Errorf("unknown mode: %s", mode)
	}

	// If still not detected, return base size
	if monW == 0 || monH == 0 {
		return baseW, baseH, nil
	}

	// Calculate constrained size
	// We currently use clamping ("adjust to them") which is standard for maximizing space
	// while ensuring it fits.
	finalW, finalH := b.CalculateConstrainedSize(baseW, baseH, monW, monH)

	return finalW, finalH, nil
}
