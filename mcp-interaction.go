package devbrowser

import (
	stdctx "context"
	"fmt"
	"strconv"
	"time"

	"webtyp.com/context"
	"webtyp.com/devbrowser/cdproto/input"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetInteractionTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_click_element",
			Description: "Click DOM element by CSS selector to test interactions, trigger events, or simulate user actions. Useful for testing buttons, links, and interactive components.",
			Args: new(ClickElementArgs),
			Resource:    "browser",
			Action:      'u',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args ClickElementArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				waitAfter := args.WaitAfter
				if waitAfter == 0 {
					waitAfter = 100
				}

				timeout := args.Timeout
				if timeout == 0 {
					timeout = 5000
				}

				tctx, cancel := stdctx.WithTimeout(b.toolCtx(), time.Duration(timeout)*time.Millisecond)
				defer cancel()

				// 1. Wait for element to be present in DOM (WaitReady)
				err := chromedp.Run(tctx, chromedp.WaitReady(args.Selector, chromedp.ByQuery))
				if err != nil {
					return nil, fmt.Errorf("Error waiting for element %s: %v", args.Selector, err)
				}

				// 2. Attempt standard click
				// Use chromedp.MouseClickNode if we want more robust clicking?
				// No, let's just use Click but maybe it's being blocked.
				err = chromedp.Run(tctx, chromedp.Click(args.Selector, chromedp.ByQuery))

				var msg string
				if err == nil {
					msg = fmt.Sprintf("Clicked element: %s", args.Selector)
				} else {
					// 3. Fallback: JavaScript click
					b.Logger(fmt.Sprintf("Standard click failed (%v), attempting JS fallback for: %s", err, args.Selector))

					// Use strconv.Quote to safely escape the selector for JS string
					jsClick := fmt.Sprintf("document.querySelector(%s).click()", strconv.Quote(args.Selector))

					if err := chromedp.Run(tctx, chromedp.Evaluate(jsClick, nil)); err != nil {
						return nil, fmt.Errorf("JS click fallback failed for %s: %v", args.Selector, err)
					}
					msg = fmt.Sprintf("Clicked element (JS fallback): %s", args.Selector)
				}

				// Wait after action
				if waitAfter > 0 {
					chromedp.Run(tctx, chromedp.Sleep(time.Duration(waitAfter)*time.Millisecond))
				}

				return mcp.Text(msg), nil
			},
		},
		{
			Name:        "browser_fill_element",
			Description: "Fill a form field (input, textarea) with text. Simulates typing.",
			Args: new(FillElementArgs),
			Resource:    "browser",
			Action:      'u',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args FillElementArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				waitAfter := args.WaitAfter
				if waitAfter == 0 {
					waitAfter = 100
				}

				timeout := args.Timeout
				if timeout == 0 {
					timeout = 5000
				}

				tctx, cancel := stdctx.WithTimeout(b.toolCtx(), time.Duration(timeout)*time.Millisecond)
				defer cancel()

				err := chromedp.Run(tctx,
					chromedp.WaitVisible(args.Selector, chromedp.ByQuery),
					chromedp.SendKeys(args.Selector, args.Value, chromedp.ByQuery),
					chromedp.Sleep(time.Duration(waitAfter)*time.Millisecond),
				)

				if err != nil {
					return nil, fmt.Errorf("Error filling element %s: %v", args.Selector, err)
				}

				return mcp.Text(fmt.Sprintf("Filled element %s with '%s'", args.Selector, args.Value)), nil
			},
		},
		{
			Name:        "browser_swipe_element",
			Description: "Simulate a swipe gesture on an element (up, down, left, right).",
			Args: new(SwipeElementArgs),
			Resource:    "browser",
			Action:      'u',
			Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args SwipeElementArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				// Swipe Logic
				err := chromedp.Run(b.toolCtx(),
					chromedp.WaitVisible(args.Selector, chromedp.ByQuery),
					chromedp.ActionFunc(func(ctx stdctx.Context) error {
						// 1. Get element dimensions to find center
						// We use javascript to get bounding client rect as it is reliable
						script := fmt.Sprintf(`
							(function() {
								const el = document.querySelector('%s');
								if (!el) return null;
								const rect = el.getBoundingClientRect();
								return {x: rect.left + rect.width/2, y: rect.top + rect.height/2};
							})()
						`, args.Selector)

						var res map[string]float64
						// Need to pass standard context.Context here
						if err := chromedp.Evaluate(script, &res).Do(ctx); err != nil {
							return err
						}

						startX := res["x"]
						startY := res["y"]
						endX := startX
						endY := startY

						switch args.Direction {
						case "up":
							endY -= float64(args.Distance)
						case "down":
							endY += float64(args.Distance)
						case "left":
							endX -= float64(args.Distance)
						case "right":
							endX += float64(args.Distance)
						}

						// Perform Mouse sequence using cdproto/input
						// Move to start
						p1 := input.DispatchMouseEvent(input.MouseMoved, startX, startY)
						if err := p1.Do(ctx); err != nil {
							return err
						}
						// Mouse Down
						p2 := input.DispatchMouseEvent(input.MousePressed, startX, startY).WithButton("left").WithClickCount(1)
						if err := p2.Do(ctx); err != nil {
							return err
						}
						// Move to end (swipe)
						p3 := input.DispatchMouseEvent(input.MouseMoved, endX, endY).WithButton("left")
						if err := p3.Do(ctx); err != nil {
							return err
						}
						// Mouse Up
						p4 := input.DispatchMouseEvent(input.MouseReleased, endX, endY).WithButton("left").WithClickCount(1)
						if err := p4.Do(ctx); err != nil {
							return err
						}
						return nil
					}),
				)

				if err != nil {
					return nil, fmt.Errorf("Error swiping element %s: %v", args.Selector, err)
				}

				return mcp.Text(fmt.Sprintf("Swiped %s on %s by %dpx", args.Direction, args.Selector, args.Distance)), nil
			},
		},
	}
}
