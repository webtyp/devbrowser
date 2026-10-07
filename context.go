package devbrowser

import (
	"context"
	"path/filepath"
	"strings"

	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/lang"
)

// FlagSPKIList trusts the listed SubjectPublicKeyInfo hashes and nothing else.
// It is NOT --ignore-certificate-errors: verification stays on everywhere else.
const FlagSPKIList = "ignore-certificate-errors-spki-list"

// flagTestType hides Chrome's "You are using an unsupported command-line flag"
// infobar, which FlagSPKIList would otherwise trigger in every dev window. It
// changes UI only: certificate verification and every other check stay on.
const flagTestType = "test-type"

func (h *DevBrowser) buildAllocatorOptions() []chromedp.ExecAllocatorOption {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", h.Headless),
		// chromedp defaults enable-automation=true, which shows the headed
		// infobar "Chrome is being controlled by automated test software".
		// false omits the switch (map overwrite); CDP still works via
		// remote-debugging-port.
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("use-fake-ui-for-media-stream", true),
		// Force the X11 ozone backend. On Linux/Wayland this routes Chrome
		// through XWayland, which is the ONLY way the window position is
		// readable/settable via CDP (native Wayland never exposes absolute
		// window coordinates, so GetWindowForTarget always reports 0,0).
		// Ignored as a no-op on Windows and macOS, so it stays cross-platform.
		chromedp.Flag("ozone-platform", "x11"),
		chromedp.Flag("window-position", h.Position),
		chromedp.WindowSize(h.Width, h.Height),
	)

	// Always enable DevTools for non-headless sessions
	h.DevToolsReserved = true
	if !h.Headless {
		opts = append(opts, chromedp.Flag("auto-open-devtools-for-tabs", true))
	}

	// Disable cache by default unless explicitly enabled
	// Note: disk-cache-size and media-cache-size flags cause "invalid exec pool flag" errors
	// Use --disable-cache instead
	if !h.CacheEnabled {
		opts = append(opts,
			chromedp.Flag("disable-cache", true),
			chromedp.Flag("disable-gpu-shader-disk-cache", true),
		)
	}

	if h.TrustDevCertSPKI != "" {
		opts = append(opts,
			chromedp.Flag(FlagSPKIList, h.TrustDevCertSPKI),
			chromedp.Flag(flagTestType, true),
		)
	}

	if h.ProfileDir != "" {
		if profileLocked(h.ProfileDir) {
			h.profileFallbackReason = "locked by another process"
		} else {
			opts = append(opts, chromedp.UserDataDir(h.ProfileDir))
		}
	}

	// Resolve the Chrome executable path
	chromePath := ResolveChromeExecPath()
	opts = append(opts, chromedp.ExecPath(chromePath))

	return opts
}

// profileLocked reports whether a running Chrome holds the profile at dir. A lock whose owner
// cannot be read counts as held.
func profileLocked(dir string) bool {
	pid, hasLock, readable := lockOwner(filepath.Join(dir, singletonLockName))
	if !hasLock {
		return false
	}
	return !readable || processAlive(pid)
}

func (h *DevBrowser) CreateBrowserContext() error {
	if h.Cancel != nil {
		h.Cancel()
		h.Cancel = nil
	}
	if h.AllocCancel != nil {
		h.AllocCancel()
		h.AllocCancel = nil
	}

	opts := h.buildAllocatorOptions()

	if h.profileFallbackReason != "" {
		h.Logger(lang.Translate("browser", "profile", "unavailable,", "using", "a", "temporary", "one:", h.profileFallbackReason).String())
		// Clear it so we only log it once per session failure
		h.profileFallbackReason = ""
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithErrorf(func(format string, args ...any) {
			// Chrome sends new CDP enum values before cdproto is updated.
			// These unmarshal errors are harmless — suppress them to avoid
			// corrupting the TUI via stderr.
			if strings.HasPrefix(format, "could not unmarshal event") {
				return
			}
			errorArgs := append([]any{"ERROR: "}, args...)
			// Forward error to devbrowser log
			h.Log(errorArgs...)
		}),
	)
	h.Ctx = ctx
	h.Cancel = cancel
	h.AllocCancel = allocCancel

	return nil
}
