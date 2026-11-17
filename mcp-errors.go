package devbrowser

import (
	"fmt"
	"strings"
	"time"

	"webtyp.com/context"
	"webtyp.com/devbrowser/cdproto/runtime"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetErrorTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_get_errors",
			Description: "Get JavaScript runtime errors and uncaught exceptions to quickly identify crashes, bugs, or WASM panics. Returns error messages with stack traces.",
			Args: new(GetErrorsArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args GetErrorsArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				limit := args.Limit
				if limit == 0 {
					limit = 20
				}

				b.ErrorsMutex.Lock()
				defer b.ErrorsMutex.Unlock()

				if len(b.JsErrors) == 0 {
					return mcp.Text("No JavaScript errors captured"), nil
				}

				start := 0
				if len(b.JsErrors) > int(limit) {
					start = len(b.JsErrors) - int(limit)
				}

				var result strings.Builder
				for i, err := range b.JsErrors[start:] {
					if i > 0 {
						result.WriteString("\n---\n")
					}
					fmt.Fprintf(&result, "Error: %s\n  at %s:%d:%d\n%s", err.Message, err.Source, err.LineNumber, err.ColumnNumber, err.StackTrace)
				}

				return mcp.Text(result.String()), nil
			},
		},
	}
}

func (b *DevBrowser) initializeErrorCapture() {
	chromedp.ListenTarget(b.Ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventExceptionThrown:
			b.ErrorsMutex.Lock()
			defer b.ErrorsMutex.Unlock()

			exception := ev.ExceptionDetails
			jsErr := JSError{
				Message:      exception.Text,
				Source:       exception.URL,
				LineNumber:   int(exception.LineNumber),
				ColumnNumber: int(exception.ColumnNumber),
				Timestamp:    time.Now(),
			}

			if exception.Exception != nil && exception.Exception.Description != "" {
				jsErr.StackTrace = exception.Exception.Description
			}

			b.JsErrors = append(b.JsErrors, jsErr)
		case *runtime.EventExecutionContextsCleared:
			b.ErrorsMutex.Lock()
			b.JsErrors = []JSError{}
			b.ErrorsMutex.Unlock()
		}
	})
}
