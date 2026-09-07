# devbrowser
<img src="docs/img/badges.svg">
<img src="docs/img/badges.svg">

A lightweight Go library for launching and controlling web browsers programmatically, designed for automation and development tools.

## Usage

The main entry point is the `New` function, which creates a new browser controller:

```go
import "webtyp.com/devbrowser"

type myServerConfig struct{}
func (myServerConfig) GetServerPort() string { return "8080" }

type myUI struct{}
func (myUI) ReturnFocus() {}

func main() {
	exitChan := make(chan bool)
	browser := devbrowser.New(myServerConfig{}, myUI{}, exitChan)
	err := browser.OpenBrowser()
	if err != nil {
		// handle error
	}
	// ... use browser ...
	browser.CloseBrowser()
}
```

## Public API

- `New(sc serverConfig, ui userInterface, exitChan chan bool) *DevBrowser`: Create a new DevBrowser instance.
- `(*DevBrowser) OpenBrowser() error`: Launch a new browser window.
- `(*DevBrowser) CloseBrowser() error`: Close the browser and clean up resources. Safe to call unconditionally (idempotent, returns `nil` if already closed).
- `(*DevBrowser) Reload() error`: Reload the current page in the browser.
- `(*DevBrowser) RestartBrowser() error`: Restart the browser (close and reopen).
- `(*DevBrowser) BrowserStartUrlChanged(fieldName, oldValue, newValue string) error`: Handle changes to the start URL and restart the browser if open.
- `(*DevBrowser) BrowserPositionAndSizeChanged(fieldName, oldValue, newValue string) error`: Change the browser window's position and size, and restart the browser.
- `(*DevBrowser) Name() string` and `(*DevBrowser) Label() string`: For UI integration, returns the component name and label.
- `(*DevBrowser) Execute(progress func(msgs ...any))`: For UI integration, toggles browser open/close and reports progress.

- `TrustDevCertSPKI string` (field): the base64-encoded SHA-256 of a development
  certificate's SubjectPublicKeyInfo. When set, the launched browser trusts
  **exactly that public key**; certificate verification stays on for every other
  origin. Empty means no pin.

	- Obtain it from `webtyp.com/server/httpd.DevCertSPKI()`. Set it before
	  `OpenBrowser()`.
	- It launches Chrome with `--ignore-certificate-errors-spki-list`. That is
	  **not** `--ignore-certificate-errors`: this browser visits arbitrary pages
	  while debugging, and a blanket bypass would also trust a genuinely broken
	  certificate on any site. `--ignore-certificate-errors`,
	  `--allow-insecure-localhost` and `--disable-web-security` are forbidden here
	  and a test fails if any of them appears.
	- Without it, a dev server on HTTPS fails to open with
	  `net::ERR_CERT_AUTHORITY_INVALID`.
	- This is why no certificate authority is installed into the operating
	  system: the tool launches the browser it needs to convince, so it configures
	  it instead. No elevated privileges, identical on Linux, macOS and Windows,
	  and nothing left behind. A phone on the LAN is a different client and cannot
	  be given a flag — it installs the CA served at `/__webtyp/ca` instead.
	- Example:

```go
spki, err := httpd.DevCertSPKI()
if err != nil {
	log.Println("browser will not trust the dev server:", err)
}
db := devbrowser.New(myServerConfig{}, myUI{}, exitChan)
db.TrustDevCertSPKI = spki
err = db.OpenBrowser()
```

- `(*DevBrowser) SetHeadless(headless bool)`: Configure whether the browser runs in headless mode (without a visible UI).
	- Signature: `func (b *DevBrowser) SetHeadless(headless bool)`
	- Default: `false` (shows the browser window). This is convenient for local development and debugging.
	- Tests: the test helper `DefaultTestBrowser()` configures the returned `DevBrowser` with `headless = true` so unit tests run without requiring a graphical display.
	- Notes: Call this before `OpenBrowser()` (or before the browser context is created) to ensure the headless flag is applied when launching Chrome/Chromium.
	- Headed launches suppress Chrome's "controlled by automated test software" infobar by overriding chromedp's default `--enable-automation` (CDP automation is unchanged).
	- Example:

```go
db := devbrowser.New(myServerConfig{}, myUI{}, exitChan)
// run with no UI (useful in CI/tests)
db.SetHeadless(true)
err := db.OpenBrowser()
if err != nil {
		// handle error
}
```

## Profile Cleanup & Idempotent Shutdown

### ProfileCleaner

Chrome automation creates temporary user profile directories (`chromedp-runner*`) on each browser instance launch. If a process exits abruptly without closing the browser, these temporary directories can accumulate over time.

`ProfileCleaner` inspects the temporary profile directory and removes stale profile directories left behind by processes that are no longer running:

```go
cleaner := devbrowser.NewProfileCleaner()
removed, freedBytes, err := cleaner.CleanStale()
if err == nil && removed > 0 {
    fmt.Printf("Cleaned %d stale browser profile(s), freed %d bytes\n", removed, freedBytes)
}
```

Consumers are encouraged to call `CleanStale()` at application startup. `CleanStale()` checks for active process locks (`SingletonLock`) to avoid disturbing live browser instances and respects a configurable grace period for newly initializing profiles.

### Idempotent `CloseBrowser`

`(*DevBrowser) CloseBrowser()` is idempotent and safe to call unconditionally (e.g. in `defer` statements or defensive shutdown paths). If the browser is already closed, it returns `nil` without error.

## Browser engine support

`devbrowser` drives Chromium through CDP. It cannot emulate WebKit/Safari, and
device emulation reproduces viewport metrics — not the iOS rendering environment.
Before reopening that discussion, read
[docs/WEBKIT_PLAN_SUPPORT.md](docs/WEBKIT_PLAN_SUPPORT.md): it covers why CDP and
WebKit are incompatible, how to tell engine differences apart from device and
document ones, which library in the ecosystem owns each fix, and what a real
WebKit backend would cost.

## MCP Tools

The following Model Context Protocol (MCP) tools are available for browser automation:

| Tool | Description |
|---|---|
| `browser_get_console` | Capture console messages from the loaded page |
| `browser_emulate_device` | Emulate a mobile, tablet, or custom device (with real DPR, UA, viewport, and touch emulation) |
| `browser_audit_mobile` | Run mobile compatibility audits (notch safe-areas, DVH/SVH units, auto-zoom, tap sizes) |
| `browser_screenshot` | Take a screenshot of the current page |
| `browser_save_screenshot` | Capture a screenshot and write it as a durable PNG file on disk (with path validation, overwrite prevention, and mutual exclusivity) |
| `browser_get_content` | Get simplified semantic HTML of the page |
| `browser_click_element` | Click on an element specified by a selector |
| `browser_fill_element` | Fill an input field with a value |
| `browser_navigate` | Navigate to a specific URL or relative path |
| `browser_swipe_element` | Perform a swipe gesture on an element |
| `browser_inspect_element` | Get detailed information about a DOM element |
| `browser_get_performance` | Get page performance metrics |
| `browser_get_network_logs` | Get network requests and responses metadata |
| `browser_evaluate_js` | Execute JavaScript in the browser context |
| `browser_get_errors` | Get captured JavaScript errors |
| `browser_get_source` | Get the raw HTML (outerHTML) of the entire page or a specific element by selector |
| `browser_get_styles` | Extract CSS rules from loaded stylesheets, with an optional selector filter |
| `browser_get_storage` | Read localStorage, sessionStorage, or cookies from the current domain |
| `browser_get_asset` | Download the content of a JS or CSS file by URL using the active session |
| `browser_intercept_request` | Capture bodies of requests and responses XHR/fetch calls (CDP Fetch domain) |

- `(*DevBrowser) GetConsoleLogs() ([]string, error)`: Capture console messages from the loaded page.
	- Signature: `func (b *DevBrowser) GetConsoleLogs() ([]string, error)`
	- Behavior: injects a small script into the page that maintains `window.__consoleLogs` and returns its contents as a slice of strings. Captures `console.log`, `console.error`, `console.warn`, and `console.info` messages.
	- Requirements: the browser context must be initialized (`OpenBrowser()` called and context created). Returns an error if the context is not ready or the evaluation fails.
	- Example:

```go
logs, err := db.GetConsoleLogs()
if err != nil {
		// handle error
}
for _, l := range logs {
		fmt.Println(l)
}
```

- `(*DevBrowser) ClearConsoleLogs() error`: Clear the in-page captured console log buffer.
	- Signature: `func (b *DevBrowser) ClearConsoleLogs() error`
	- Behavior: executes a small script that resets `window.__consoleLogs = []` if present.
	- Requirements: the browser context must be initialized. Returns an error if the evaluation fails.
	- Example:

```go
err := db.ClearConsoleLogs()
if err != nil {
		// handle error
}
```

### Device Emulation & Mobile Auditing

`devbrowser` provides robust device emulation to bridge the gap between emulated views and physical devices.

#### Precise Device Emulation & Live Window Auto-fit

The `browser_emulate_device` tool accepts two main arguments:
- `mode`: `"mobile"` (defaults to iPhone 15 Pro Max), `"tablet"` (defaults to iPad Pro), `"desktop"` (pins to `1440x900`), or `"off"` (clears overrides).
- `device`: Optional custom device name (e.g., `"iPhone 14 Pro Max"`, `"Pixel 5"`). This matches the name in lowercase and without spaces/symbols. If a name is unknown, the tool returns an error listing all available device names from the 131-device catalog.

When emulating a specific device from the catalog, `devbrowser` automatically configures:
1. **Device Pixel Ratio (DPR)** (e.g., `3.0` for iPhone 15 Pro Max) for crisp screenshots, `@media` query matching, and correct `srcset` selection.
2. **Real User-Agent (UA) Override** to ensure server-side or client-side UA routing works correctly.
3. **Viewport visible dimensions** (the height takes active browser bars into account).
4. **Touch Emulation** to correctly enable mobile gesture handlers.
5. **Live Window Auto-fit**: If the requested viewport exceeds the current physical window bounds, `devbrowser` resizes the browser window live using CDP `Browser.setWindowBounds` (without restarting Chrome). Position and existing size are preserved as a floor (the window never shrinks automatically). When DevTools is docked on the side (auto-opened for launch widths > 1200px), an estimated reserved width of 420px is included so DevTools does not obscure the emulated viewport.

#### What Cannot Be Emulated (And How to Audit)

Due to protocol limits, certain iOS/Safari behaviors cannot be directly simulated in Chromium:
- `safe-area-inset-*` (notch and home gestural bars) cannot be emulated.
- Dynamic viewport height shrinkage (`100vh` behavior) and Safari-specific fonts/inputs are not available.

To detect these and other common mobile layout defects, use **`browser_audit_mobile`**. It scans the DOM and stylesheets to check for:
- **`viewport-meta`**: Missing `<meta name="viewport">` or missing `viewport-fit=cover`.
- **`vh-units`**: Elements using raw `vh` units instead of dynamic viewports like `dvh` or `svh` (which can overflow or shift layout as browser controls hide).
- **`safe-area`**: Fixed elements at edges lacking safe-area insets.
- **`input-zoom`**: Input/Select fields with font-size `< 16px` (which triggers iOS Safari's annoying layout-shifting zoom on focus).
- **`tap-target`**: Interactive elements smaller than the standard `44x44` px touch target.
- **`fixed-vh`**: Fixed position elements using `vh` heights (the worst layout shifter on scroll).

### Saving screenshots directly to disk

The `browser_save_screenshot` tool is designed to produce durable documentation artifacts (such as widget and component reference images) directly on the local filesystem. This tool is isolated to the separate `browser_file` resource type for maximum security and access control.

#### Worked Example: Documenting a Widget Component

1. **Emulate the desired viewport (e.g., desktop):**
   ```json
   {
     "mode": "desktop"
   }
   ```
   *Result:* Sets the emulated device viewport to exactly `1440x900`.

2. **Capture and save a specific component by CSS selector:**
   ```json
   {
     "dir": "./docs/img",
     "name": "badges",
     "selector": ".badge-container",
     "overwrite": true
   }
   ```
   *Result:* Captures only the `.badge-container` element, cleans and resolves the destination directory to an absolute path, creates `docs/img/` if missing, and safely writes the PNG file to `docs/img/badges.png`.

3. **Reference the file in your documentation:**
   ```markdown
   <img src="docs/img/badges.png">
   ```
