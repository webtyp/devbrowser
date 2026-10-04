package devbrowser

import (
	"encoding/json"
	"fmt"

	"webtyp.com/context"
	"webtyp.com/devbrowser/cdproto/runtime"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetEvaluateJsTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_evaluate_js",
			Description: "Execute JavaScript code in browser context to inspect DOM, call WASM exports, test functions, or debug application state. Returns execution result or error.",
			Args:        new(EvaluateJSArgs),
			Resource:    "browser",
			Action:      'u',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args EvaluateJSArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				var res interface{}
				err := chromedp.Run(b.Ctx,
					chromedp.Evaluate(args.Script, &res, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
						return p.WithAwaitPromise(args.AwaitPromise).WithIncludeCommandLineAPI(true)
					}),
				)

				if err != nil {
					return nil, err
				}

				switch v := res.(type) {
				case nil:
					return mcp.Text("undefined"), nil
				case string:
					return mcp.Text(v), nil
				case float64, bool:
					return mcp.Text(fmt.Sprintf("%v", v)), nil
				default:
					jsonRes, err := json.Marshal(v)
					if err != nil {
						return nil, fmt.Errorf("failed to serialize result: %v", err)
					}
					return mcp.Text(string(jsonRes)), nil
				}
			},
		},
	}
}
