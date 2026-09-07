package devbrowser

import (
	"context"
	"strings"

	"webtyp.com/devbrowser/chromedp"
)

// FlagSPKIList trusts the listed SubjectPublicKeyInfo hashes and nothing else.
// It is NOT --ignore-certificate-errors: verification stays on everywhere else.
const FlagSPKIList = "ignore-certificate-errors-spki-list"

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

	// Conditionally add devtools flag
	h.DevToolsReserved = h.Width > 1200
	if h.DevToolsReserved {
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
		opts = append(opts, chromedp.Flag(FlagSPKIList, h.TrustDevCertSPKI))
	}

	// Resolve the Chrome executable path
	chromePath := ResolveChromeExecPath()
	opts = append(opts, chromedp.ExecPath(chromePath))

	return opts
}

func (h *DevBrowser) CreateBrowserContext() error {
	opts := h.buildAllocatorOptions()

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
