package devbrowser

import (
	"context"
	"fmt"

	"webtyp.com/devbrowser/cdproto/browser"
	"webtyp.com/devbrowser/chromedp"
)

// GrowWindowToFit resizes the live physical browser window, in place, so it
// is at least (reqW, reqH) plus any DevTools reservation — see
// RequiredWindowSize — and never shrinks it. It is the single place that
// keeps what a human sees in the window in sync with what an MCP-driven
// device emulation call renders, so both are looking at the same content
// instead of the emulated viewport overflowing a window sized for something
// else. Returns false, nil when no resize was necessary.
func (b *DevBrowser) GrowWindowToFit(reqW, reqH int) (bool, error) {
	b.Mu.Lock()
	ctx := b.Ctx
	curW, curH := b.Width, b.Height
	b.Mu.Unlock()

	if ctx == nil {
		return false, nil
	}

	newW, newH := b.RequiredWindowSize(reqW, reqH)
	if newW <= curW && newH <= curH {
		return false, nil
	}

	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			t := chromedp.FromContext(ctx).Target
			windowID, _, err := browser.GetWindowForTarget().WithTargetID(t.TargetID).Do(ctx)
			if err != nil {
				return err
			}
			bounds := &browser.Bounds{
				Width:  int64(newW),
				Height: int64(newH),
			}
			return browser.SetWindowBounds(windowID, bounds).Do(ctx)
		}),
	)
	if err != nil {
		return false, fmt.Errorf("GrowWindowToFit: %w", err)
	}

	b.Mu.Lock()
	b.Width = newW
	b.Height = newH
	b.Mu.Unlock()

	return true, nil
}
