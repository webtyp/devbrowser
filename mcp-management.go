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
			Description: "Emulate a mobile device, tablet, or named device viewport. Also grows the physical window live (never shrinks) so the requested viewport is fully visible instead of being covered by DevTools — window position and any pre-existing size are preserved as a floor. This toggle affects rendering and touch events. This change is persisted.",
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
					if reqW, reqH, _ := EmulationViewportSize(args.Mode, args.Device); reqW > 0 && reqH > 0 {
						if _, err := b.GrowWindowToFit(reqW, reqH); err != nil {
							b.Logger(fmt.Sprintf("Failed to grow window for emulation: %v", err))
						}
					}

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

	var actions []chromedp.Action

	if devName != "" {
		d, _, err := resolveDevice(devName)
		if err != nil {
			return err
		}
		actions = append(actions, chromedp.Emulate(d))
	} else {
		switch mode {
		case "mobile":
			actions = append(actions, chromedp.Emulate(device.IPhone15ProMax))
		case "tablet":
			actions = append(actions, chromedp.Emulate(device.IPadPro))
		case "desktop", "off", "":
			// Clear overrides by resetting device metrics and user agent,
			// allowing the browser layout to adjust naturally to the window size and DevTools.
			actions = append(actions,
				emulation.ClearDeviceMetricsOverride(),
				emulation.SetTouchEmulationEnabled(false),
				emulation.SetUserAgentOverride(""),
			)
		default:
			return fmt.Errorf("unsupported mode: %s", mode)
		}
	}

	return chromedp.Run(b.Ctx, actions...)
}

// EmulationViewportSize returns the CSS pixel viewport size that mode/devName
// will render at once applied by applyDeviceEmulation, so the physical
// window can be grown to fit BEFORE the CDP emulation override is issued.
// Keep this in sync with the switch in applyDeviceEmulation — same modes,
// same device shortcuts.
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
		info := device.IPhone15ProMax.Device()
		return int(info.Width), int(info.Height), nil
	case "tablet":
		info := device.IPadPro.Device()
		return int(info.Width), int(info.Height), nil
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
