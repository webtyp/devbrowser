package devbrowser

import (
	"fmt"

	"webtyp.com/context"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetSourceTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_get_source",
			Description: "Get the raw HTML (outerHTML) of the entire page or a specific element by selector. Useful for faithful reverse engineering of the DOM structure.",
			Args: new(GetSourceArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}
				var args GetSourceArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				var js string
				if args.Selector == "" {
					js = `document.documentElement.outerHTML`
				} else {
					js = fmt.Sprintf(`document.querySelector(%q)?.outerHTML ?? "Element not found"`, args.Selector)
				}

				var result string
				err := chromedp.Run(b.toolCtx(), chromedp.Evaluate(js, &result))
				if err != nil {
					return nil, err
				}

				return mcp.Text(result), nil
			},
		},
	}
}
