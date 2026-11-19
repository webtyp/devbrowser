package devbrowser

import (
	"fmt"

	"webtyp.com/devbrowser/chromedp"
)

// ScreenshotResult contains the image data and metadata about the captured page.
type ScreenshotResult struct {
	ImageData     []byte
	PageTitle     string
	PageURL       string
	Width         int
	Height        int
	HTMLStructure string
}

// CaptureScreenshot captures a screenshot of the current page.
func (b *DevBrowser) CaptureScreenshot(fullpage bool) (*ScreenshotResult, error) {
	if !b.IsOpenFlag || b.Ctx == nil {
		return nil, fmt.Errorf("browser is not open")
	}

	var buf []byte
	var err error

	if fullpage {
		err = chromedp.Run(b.toolCtx(),
			chromedp.FullScreenshot(&buf, 90),
		)
	} else {
		err = chromedp.Run(b.toolCtx(),
			chromedp.CaptureScreenshot(&buf),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to capture screenshot: %v", err)
	}

	res := &ScreenshotResult{
		ImageData: buf,
	}

	// Capture comprehensive page context
	err = chromedp.Run(b.toolCtx(),
		chromedp.Title(&res.PageTitle),
		chromedp.Location(&res.PageURL),
		chromedp.Evaluate(`window.innerWidth`, &res.Width),
		chromedp.Evaluate(`window.innerHeight`, &res.Height),
		chromedp.Evaluate(GetStructureJS, &res.HTMLStructure),
	)

	if err != nil {
		// Non-fatal, return what we have
		b.Logger(fmt.Sprintf("Warning: failed to capture context metadata: %v", err))
	}

	return res, nil
}

// CaptureElementScreenshot captures a screenshot of a specific element.
func (b *DevBrowser) CaptureElementScreenshot(selector string) (*ScreenshotResult, error) {
	if !b.IsOpenFlag || b.Ctx == nil {
		return nil, fmt.Errorf("browser is not open")
	}

	var buf []byte
	err := chromedp.Run(b.toolCtx(),
		chromedp.Screenshot(selector, &buf),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to capture element screenshot: %v", err)
	}

	res := &ScreenshotResult{
		ImageData: buf,
	}

	// Capture basic context
	_ = chromedp.Run(b.toolCtx(),
		chromedp.Title(&res.PageTitle),
		chromedp.Location(&res.PageURL),
		chromedp.Evaluate(`window.innerWidth`, &res.Width),
		chromedp.Evaluate(`window.innerHeight`, &res.Height),
	)

	return res, nil
}
