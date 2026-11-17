package devbrowser

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	twcontext "webtyp.com/context"
	"webtyp.com/devbrowser/cdproto/fetch"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetInterceptTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_intercept_request",
			Description: "Capture request and response bodies for XHR/fetch calls. Use 'start' to begin, 'stop' to end, and 'get' to retrieve captured data.",
			Args: new(InterceptRequestArgs),
			Resource:    "browser",
			Action:      'u',
			Execute: func(ctx *twcontext.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}
				var args InterceptRequestArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				switch args.Action {
				case "start":
					return b.startInterception(args.Filter)
				case "stop":
					return b.stopInterception()
				case "get":
					return b.getInterceptedRequests(args.Filter, int(args.Limit))
				default:
					return nil, fmt.Errorf("Unknown action: %s. Use 'start', 'stop', or 'get'", args.Action)
				}
			},
		},
	}
}

func (b *DevBrowser) startInterception(filter string) (*mcp.Result, error) {
	b.InterceptMutex.Lock()
	if b.InterceptActive {
		b.InterceptMutex.Unlock()
		return mcp.Text("Interception is already active"), nil
	}
	b.InterceptActive = true
	b.InterceptedReqs = nil
	b.InterceptMutex.Unlock()

	// Enable with RequestStageResponse to get bodies
	err := chromedp.Run(b.Ctx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{
		{URLPattern: "*", RequestStage: fetch.RequestStageResponse},
	}))
	if err != nil {
		return nil, err
	}

	return mcp.Text("Request interception started"), nil
}

func (b *DevBrowser) stopInterception() (*mcp.Result, error) {
	b.InterceptMutex.Lock()
	b.InterceptActive = false
	b.InterceptMutex.Unlock()

	err := chromedp.Run(b.Ctx, fetch.Disable())
	if err != nil {
		return nil, err
	}

	return mcp.Text("Request interception stopped"), nil
}

func (b *DevBrowser) getInterceptedRequests(filter string, limit int) (*mcp.Result, error) {
	b.InterceptMutex.Lock()
	defer b.InterceptMutex.Unlock()

	var filtered []InterceptedRequest
	for _, r := range b.InterceptedReqs {
		if filter == "" || strings.Contains(r.URL, filter) {
			filtered = append(filtered, r)
		}
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	if len(filtered) == 0 {
		return mcp.Text("No requests intercepted"), nil
	}

	var res strings.Builder
	for i, r := range filtered {
		if i > 0 {
			res.WriteString("\n---\n")
		}
		fmt.Fprintf(&res, "%d %s %s\n", r.Status, r.Method, r.URL)
		if r.RequestBody != "" {
			fmt.Fprintf(&res, "Request Body: %s\n", r.RequestBody)
		}
		if r.ResponseBody != "" {
			fmt.Fprintf(&res, "Response Body: %s\n", r.ResponseBody)
		}
	}

	return mcp.Text(res.String()), nil
}

func (b *DevBrowser) initializeInterceptCapture() {
	chromedp.ListenTarget(b.Ctx, func(ev interface{}) {
		b.InterceptMutex.Lock()
		active := b.InterceptActive
		b.InterceptMutex.Unlock()
		if !active {
			return
		}

		switch ev := ev.(type) {
		case *fetch.EventRequestPaused:
			go func() {
				if ev.ResponseStatusCode != 0 {
					intercepted := InterceptedRequest{
						URL:    ev.Request.URL,
						Method: ev.Request.Method,
						Status: int(ev.ResponseStatusCode),
					}

					if ev.Request.HasPostData && len(ev.Request.PostDataEntries) > 0 {
						var postData strings.Builder
						for _, entry := range ev.Request.PostDataEntries {
							decoded, err := base64.StdEncoding.DecodeString(entry.Bytes)
							if err == nil {
								postData.Write(decoded)
							} else {
								postData.WriteString(entry.Bytes)
							}
						}
						intercepted.RequestBody = postData.String()
					}

					// Use b.Ctx (allocated) for GetResponseBody
					var body []byte
					err := chromedp.Run(b.Ctx, chromedp.ActionFunc(func(ctx context.Context) error {
						var err error
						body, err = fetch.GetResponseBody(ev.RequestID).Do(ctx)
						return err
					}))

					if err == nil {
						intercepted.ResponseBody = string(body)
					}

					b.InterceptMutex.Lock()
					if len(b.InterceptedReqs) >= 100 {
						b.InterceptedReqs = b.InterceptedReqs[1:]
					}
					b.InterceptedReqs = append(b.InterceptedReqs, intercepted)
					b.InterceptMutex.Unlock()
				}

				// Always continue
				chromedp.Run(b.Ctx, fetch.ContinueRequest(ev.RequestID))
			}()
		}
	})
}
