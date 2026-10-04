package devbrowser

import (
	"fmt"

	"webtyp.com/context"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

// InspectElementJS extracts detailed element information like Chrome DevTools.
// Returns a JSON-like string with box model, position, styles, and accessibility info.
var InspectElementJS = fmt.Sprintf(`
(selector) => {
	%s
	const el = document.querySelector(selector);
	if (!el) return JSON.stringify({ error: 'Element not found: ' + selector });
	return JSON.stringify(extractElementDetails(el), null, 2);
}
`, ExtractElementDetailsFunctionJS)

func (b *DevBrowser) GetInspectTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_inspect_element",
			Description: "Inspect a specific element to get detailed CSS properties like Chrome DevTools. Returns box model (width, height, padding, margin, border), position (top, left, offset), layout (display, flex, grid), typography (font, color), and accessibility info.",
			Args: new(InspectElementArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args InspectElementArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				var result string
				js := fmt.Sprintf("(%s)(%q)", InspectElementJS, args.Selector)

				err := chromedp.Run(b.Ctx,
					chromedp.Evaluate(js, &result),
				)

				if err != nil {
					return nil, fmt.Errorf("Failed to inspect element: %v", err)
				}

				return mcp.Text(fmt.Sprintf("Inspect Element: %s\n%s", args.Selector, result)), nil
			},
		},
	}
}
