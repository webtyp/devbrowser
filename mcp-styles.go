package devbrowser

import (
	"fmt"

	"webtyp.com/context"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetStylesTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_get_styles",
			Description: "Extract CSS rules from loaded stylesheets. Can filter by selector and stylesheet index. Useful for replicating design and understanding styling.",
			Args: new(GetStylesArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}
				var args GetStylesArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				var js string
				if args.Selector == "" {
					js = fmt.Sprintf(`(function() {
  const sheetIdx = %d;
  const sheets = sheetIdx >= 0 ? [document.styleSheets[sheetIdx]].filter(Boolean) : [...document.styleSheets];
  return sheets.flatMap(s => {
    try { return [...s.cssRules].map(r => r.cssText) }
    catch(e) { return ['/* cross-origin: ' + s.href + ' */'] }
  }).join('\n')
})()`, args.Sheet)
				} else {
					js = fmt.Sprintf(`(function() {
  const SELECTOR = %q;
  const sheetIdx = %d;
  const sheets = sheetIdx >= 0 ? [document.styleSheets[sheetIdx]].filter(Boolean) : [...document.styleSheets];
  return sheets.flatMap(s => {
    try { return [...s.cssRules] }
    catch(e) { return [] }
  }).filter(r => r.selectorText && r.selectorText.includes(SELECTOR))
    .map(r => r.cssText).join('\n')
})()`, args.Selector, args.Sheet)
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
