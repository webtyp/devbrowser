package devbrowser

import (
	"fmt"
	"reflect"
	"strings"

	"webtyp.com/context"
	"webtyp.com/devbrowser/cdproto/emulation"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/devbrowser/chromedp/device"
	"webtyp.com/mcp"
)

func (b *DevBrowser) GetManagementTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_emulate_device",
			Description: "Emulate mobile, tablet, or desktop viewport. Adapts naturally to the current browser window and Chrome DevTools without forcing artificial window resizing. This change is persisted in browser_viewport.",
			Args:        new(EmulateDeviceArgs),
			Resource:    "browser",
			Action:      'u',
			Execute: func(Ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				var args EmulateDeviceArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				// Validate the mode and device *before* assigning and saving
				var avail []string
				var err error

				if args.Device != "" {
					_, avail, err = resolveDevice(args.Device)
					if err != nil {
						return nil, fmt.Errorf("unsupported device: %s. Available devices: %s", args.Device, strings.Join(avail, ", "))
					}
				}

				// Validate the mode *before* assigning and saving
				switch args.Mode {
				case "mobile", "tablet", "desktop", "off", "":
					// valid modes
				default:
					return nil, fmt.Errorf("unsupported mode: %s", args.Mode)
				}

				b.Mu.Lock()
				b.ViewportMode = args.Mode
				b.ViewportDevice = args.Device
				b.Mu.Unlock()

				if err := b.SaveConfig(); err != nil {
					b.Logger(fmt.Sprintf("Error saving emulation config: %v", err))
				}

				var actualW, actualH int
				if b.IsOpen() && b.Ctx != nil {
					if err := b.applyDeviceEmulation(); err != nil {
						return nil, err
					}
					b.UI.RefreshUI()

					// Read back dimensions dynamically
					if err := chromedp.Run(b.Ctx,
						chromedp.Evaluate(`window.innerWidth`, &actualW),
						chromedp.Evaluate(`window.innerHeight`, &actualH),
					); err != nil {
						b.Logger(fmt.Sprintf("Failed to read back viewport: %v", err))
					}
				}

				emulationName := args.Mode
				if args.Device != "" {
					emulationName = args.Device
				}
				statusMsg := fmt.Sprintf("Device emulation set to %s", emulationName)
				if b.IsOpen() && b.Ctx != nil && actualW > 0 && actualH > 0 {
					statusMsg = fmt.Sprintf("Device emulation set to %s (viewport %dx%d)", emulationName, actualW, actualH)
				}

				if args.Capture {
					var res *ScreenshotResult
					var err error
					if args.Selector != "" {
						res, err = b.CaptureElementScreenshot(args.Selector)
					} else {
						res, err = b.CaptureScreenshot(false)
					}

					if err != nil {
						return nil, fmt.Errorf("%s. Failed to capture screenshot: %v", statusMsg, err)
					}

					// Build visual context report
					contextReport := fmt.Sprintf(
						"%s\nURL: %s | Title: %s | Viewport: %dx%d\n\n%s",
						statusMsg,
						res.PageURL,
						res.PageTitle,
						res.Width, res.Height,
						res.HTMLStructure,
					)

					return mcp.Text(contextReport), nil
				} else {
					return mcp.Text(statusMsg), nil
				}
			},
		},
	}
}

// applyDeviceEmulation applies the current b.ViewportMode or b.ViewportDevice using CDP emulation commands.
func (b *DevBrowser) applyDeviceEmulation() error {
	b.Mu.Lock()
	mode := b.ViewportMode
	devName := b.ViewportDevice
	b.Mu.Unlock()

	// Clear any previous metrics override to measure unconstrained window
	if err := chromedp.Run(b.Ctx,
		emulation.ClearDeviceMetricsOverride(),
		emulation.SetTouchEmulationEnabled(false),
		emulation.SetUserAgentOverride(""),
	); err != nil {
		return err
	}

	if devName != "" {
		d, _, err := resolveDevice(devName)
		if err != nil {
			return err
		}
		return chromedp.Run(b.Ctx, chromedp.Emulate(d))
	}

	switch mode {
	case "desktop", "off", "":
		// Clear overrides: layout adjusts naturally to window size and DevTools
		return nil

	case "mobile", "tablet":
		// Read available inner dimensions inside Chrome (respects DevTools docked on right/bottom)
		var availW, availH int
		if err := chromedp.Run(b.Ctx,
			chromedp.Evaluate(`window.innerWidth`, &availW),
			chromedp.Evaluate(`window.innerHeight`, &availH),
		); err != nil {
			return err
		}

		if availW <= 0 || availH <= 0 {
			return nil
		}

		var targetW int
		var ua string

		if mode == "mobile" {
			targetW = 375
			if availW < targetW {
				targetW = availW
			}
			ua = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
		} else { // tablet
			targetW = 768
			if availW < targetW {
				targetW = availW
			}
			ua = "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
		}

		// Apply override: target width + 0 height (0 disables height override so height
		// tracks the available window & Chrome DevTools naturally without overflowing or cutting off)
		return chromedp.Run(b.Ctx,
			emulation.SetDeviceMetricsOverride(int64(targetW), 0, 1.0, true),
			emulation.SetTouchEmulationEnabled(true),
			emulation.SetUserAgentOverride(ua),
		)

	default:
		return fmt.Errorf("unsupported mode: %s", mode)
	}
}

// EmulationViewportSize returns the CSS pixel viewport size for a given mode or device name.
func EmulationViewportSize(mode, devName string) (int, int, error) {
	if devName != "" {
		d, _, err := resolveDevice(devName)
		if err != nil {
			return 0, 0, err
		}
		info := d.Device()
		return int(info.Width), int(info.Height), nil
	}

	switch mode {
	case "mobile":
		return 375, 0, nil
	case "tablet":
		return 768, 0, nil
	case "desktop", "off", "":
		return 0, 0, nil
	default:
		return 0, 0, fmt.Errorf("unsupported mode: %s", mode)
	}
}

// normalizeName removes spaces, dashes, parentheses, underscores, and lowercase the string.
func normalizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "")
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "(", "")
	name = strings.ReplaceAll(name, ")", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "+", "")
	return name
}

// resolveDevice finds a device by normalising its name.
// If not found, it returns a list of all available device names.
func resolveDevice(target string) (chromedp.Device, []string, error) {
	normTarget := normalizeName(target)
	rt := reflect.TypeOf(device.Reset)
	var available []string

	for i := 1; i <= 131; i++ {
		v := reflect.ValueOf(i).Convert(rt)
		d := v.Interface().(chromedp.Device)
		info := d.Device()
		if info.Name == "" {
			continue
		}
		available = append(available, info.Name)
		if normalizeName(info.Name) == normTarget {
			return d, nil, nil
		}
	}

	return nil, available, fmt.Errorf("unsupported device: %s", target)
}
