package devbrowser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"webtyp.com/context"
	wpath "webtyp.com/filepath"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetScreenshotTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_screenshot",
			Description: "Capture screenshot of current browser viewport to verify visual rendering, layout correctness, or UI state. Returns PNG image as MCP resource (binary efficient format).",
			Args:        new(ScreenshotArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(Ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				var args ScreenshotArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				res, err := b.CaptureScreenshot(args.Fullpage)
				if err != nil {
					return nil, err
				}

				if len(res.ImageData) == 0 {
					return nil, fmt.Errorf("Screenshot capture returned empty buffer")
				}

				// Write PNG image to clipboard
				if err := writeToClipboard(res.ImageData); err != nil {
					b.Logger(fmt.Sprintf("Failed to copy screenshot to clipboard: %v", err))
				}

				// Build visual context report (what AI "sees" without image bytes)
				contextReport := fmt.Sprintf(
					"Screenshot captured (%d KB)\n"+
						"Url: %s | Title: %s | Viewport: %dx%d\n"+
						"\n"+
						"%s",
					len(res.ImageData)/1024,
					res.PageURL,
					res.PageTitle,
					res.Width, res.Height,
					res.HTMLStructure,
				)

				// Send text context + image to MCP client (not to TUI)
				return mcp.NewResult(mcp.TextBlock(contextReport), mcp.ImageBlock(res.ImageData, "image/png")), nil
			},
		},
		{
			Name:        "browser_save_screenshot",
			Description: "Capture a screenshot and write it as a durable PNG file on disk. Useful for documenting widgets or components.",
			Args:        new(SaveScreenshotArgs),
			Resource:    "browser_file",
			Action:      'c',
			Execute: func(Ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				var args SaveScreenshotArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				if args.Selector != "" && args.Fullpage {
					return nil, fmt.Errorf("selector and fullpage are mutually exclusive")
				}

				fullPath, err := cleanAndValidatePath(args.Dir, args.Name)
				if err != nil {
					return nil, err
				}

				if !args.Overwrite {
					if _, err := os.Stat(fullPath); err == nil {
						return nil, fmt.Errorf("file already exists: %s", wpath.Tilde(fullPath))
					}
				}

				// Verify browser is open
				if !b.IsOpen() || b.Ctx == nil {
					return nil, ErrBrowserNotOpen
				}

				var res *ScreenshotResult
				if args.Selector != "" {
					res, err = b.CaptureElementScreenshot(args.Selector)
				} else {
					res, err = b.CaptureScreenshot(args.Fullpage)
				}
				if err != nil {
					return nil, err
				}

				if len(res.ImageData) == 0 {
					return nil, fmt.Errorf("screenshot capture returned empty buffer")
				}

				// Ensure directory exists
				absDir := filepath.Dir(fullPath)
				if err := os.MkdirAll(absDir, 0755); err != nil {
					return nil, fmt.Errorf("failed to create directory %s: %v", absDir, err)
				}

				// Write PNG file
				if err := os.WriteFile(fullPath, res.ImageData, 0644); err != nil {
					return nil, fmt.Errorf("failed to write screenshot file: %v", err)
				}

				b.Mu.Lock()
				emulationMode := b.ViewportMode
				b.Mu.Unlock()
				if emulationMode == "" {
					emulationMode = "off"
				}

				// Return path and dimensions
				statusReport := fmt.Sprintf(
					"Screenshot saved to: %s\n"+
						"Dimensions: %dx%d\n"+
						"Emulation Mode: %s\n",
					wpath.Tilde(fullPath),
					res.Width, res.Height,
					emulationMode,
				)

				return mcp.Text(statusReport), nil
			},
		},
	}
}

// cleanAndValidatePath cleans the directory and filename, resolves the absolute path, and performs safety checks.
func cleanAndValidatePath(dir, name string) (string, error) {
	// First, reject any separator in name
	if strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("file name cannot contain path separators: %s", name)
	}

	// Clean the directory path
	cleanDir := filepath.Clean(dir)

	// After cleaning, we must reject any segment of ".." to prevent traversal
	parts := strings.Split(cleanDir, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return "", fmt.Errorf("path traversal (..) is not allowed: %s", dir)
		}
	}

	// Resolve to an absolute path
	absDir, err := filepath.Abs(cleanDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for directory: %v", err)
	}

	fullName := name
	if !strings.HasSuffix(strings.ToLower(name), ".png") {
		fullName = name + ".png"
	}
	fullPath := filepath.Join(absDir, fullName)

	if filepath.Dir(fullPath) != absDir {
		return "", fmt.Errorf("path traversal detected: %s", name)
	}

	return fullPath, nil
}
