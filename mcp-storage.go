package devbrowser

import (

	"webtyp.com/context"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetStorageTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_get_storage",
			Description: "Read localStorage, sessionStorage, or cookies from the current domain. Useful for debugging session state and user preferences.",
			Args: new(GetStorageArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}
				var args GetStorageArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				var js string
				switch args.Type {
				case "session":
					js = `JSON.stringify(Object.fromEntries(Object.entries(sessionStorage)))`
				case "cookies":
					js = `document.cookie`
				default: // "local"
					js = `JSON.stringify(Object.fromEntries(Object.entries(localStorage)))`
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
