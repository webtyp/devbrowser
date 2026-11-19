package devbrowser

import (
	"fmt"
	"net/url"
	"strings"
	"webtyp.com/devbrowser/chromedp"

	"webtyp.com/context"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetNavigationTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_navigate",
			Description: "Navigate the browser to a specific Url or relative path (e.g. /login)",
			Args:        new(NavigateArgs),
			Resource:    "browser",
			Action:      'u',
			Execute: func(Ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args NavigateArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				targetURL, err := b.resolveToolURL(args.Url)
				if err != nil {
					return nil, err
				}

				if err := b.navigateTool(targetURL); err != nil {
					return nil, fmt.Errorf("Error navigating to %s: %v", targetURL, err)
				}

				var current string
				if ctx := b.toolCtx(); ctx == nil {
					return mcp.Text(fmt.Sprintf("Navigated to %s", targetURL)), nil
				} else if err := chromedp.Run(ctx, chromedp.Location(&current)); err != nil {
					return mcp.Text(fmt.Sprintf("Navigated to %s (could not get current Url: %v)", targetURL, err)), nil
				}

				return mcp.Text(fmt.Sprintf("Navigated to %s (current: %s)", targetURL, current)), nil
			},
		},
	}
}

// resolveToolURL makes a relative path absolute against the tab the developer
// has in front ("/login" on user3.localhost stays on user3.localhost); with
// no page loaded yet, against the app's localhost port.
func (b *DevBrowser) resolveToolURL(target string) (string, error) {
	if strings.Contains(target, "://") {
		return target, nil
	}
	var baseURL string
	var current string
	if ctx := b.toolCtx(); ctx != nil && chromedp.Run(ctx, chromedp.Location(&current)) == nil {
		if u, err := url.Parse(current); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
			baseURL = u.Scheme + "://" + u.Host
		}
	}
	if baseURL == "" {
		if b.LastPort == "" {
			return "", fmt.Errorf("browser has no active app port; open the app first")
		}
		scheme := "http"
		if b.LastHttps {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://localhost:%s", scheme, b.LastPort)
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base Url: %v", err)
	}
	rel, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("failed to parse target path: %v", err)
	}
	return base.ResolveReference(rel).String(), nil
}

// navigateTool navigates the tab the developer has in front; the main tab
// goes through NavigateToURL, which also keeps the load guard's origin.
func (b *DevBrowser) navigateTool(target string) error {
	ctx := b.toolCtx()
	if ctx == nil || ctx == b.Ctx {
		return b.NavigateToURL(target)
	}
	return chromedp.Run(ctx, chromedp.Navigate(target))
}
