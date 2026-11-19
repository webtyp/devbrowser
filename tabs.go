package devbrowser

import (
	"context"
	"errors"
	"fmt"

	"webtyp.com/devbrowser/cdproto/inspector"
	"webtyp.com/devbrowser/cdproto/page"
	"webtyp.com/devbrowser/cdproto/runtime"
	"webtyp.com/devbrowser/chromedp"
)

// browserTab is one tab of the browser: the main one (Ctx) or one OpenTab
// added. While a tab is NOT the active one, what it logs waits in its own
// buffers; the active tab's buffers are b.ConsoleLogs, b.NetworkLogs and
// b.JsErrors, which every MCP tool reads.
type browserTab struct {
	ctx    context.Context
	cancel context.CancelFunc // nil for the main tab: Ctx owns the browser
	url    string             // what OpenTab opened, to reopen it after RestartBrowser
	// selectionInstalled: the selection gestures are wired on this tab (guarded by b.Mu)
	selectionInstalled bool
	console            []string
	network            []NetworkLogEntry
	errors             []JSError
}

const (
	tabActiveBinding = "webtypTabActive"
	// tabActiveJS tells devbrowser when its tab comes to the front: switching
	// tabs fires visibilitychange (to visible), clicking into the page fires
	// focus — only the tab in front can receive focus.
	tabActiveJS = `(() => {
	const tell = () => {
		if (typeof window.` + tabActiveBinding + ` === 'function') {
			window.` + tabActiveBinding + `('');
		}
	};
	document.addEventListener('visibilitychange', () => {
		if (document.visibilityState === 'visible') tell();
	});
	window.addEventListener('focus', tell);
	if (document.visibilityState === 'visible') tell();
})()`
)

var errBrowserNotReady = errors.New("open tab: the browser is not open yet")

// OpenTab opens rawURL in a new tab of the browser that is already open, next
// to the main tab. The new tab gets its own console, network and error
// capture; MCP tools act on whichever tab the developer has in front
// (activeTab). It is reloaded together with the main one on every rebuild,
// and closes with the browser.
//
//	b.OpenBrowser("6060", false)
//	b.OpenTab("http://user2.localhost:6060/")
func (b *DevBrowser) OpenTab(rawURL string) error {
	b.Mu.Lock()
	if !b.ready || b.Ctx == nil {
		b.Mu.Unlock()
		return errBrowserNotReady
	}
	parent := b.Ctx
	b.Mu.Unlock()

	ctx, cancel := chromedp.NewContext(parent) // a new target in the same browser
	tab := &browserTab{ctx: ctx, cancel: cancel, url: rawURL}
	if err := b.captureConsole(tab); err != nil {
		cancel()
		return errors.New("open tab " + rawURL + ": " + err.Error())
	}
	b.captureNetwork(tab)
	b.captureErrors(tab)
	b.captureIntercept(tab)
	b.installSelectionCaptureOn(tab)
	b.watchTab(tab)
	if err := b.applyDeviceEmulationOn(ctx); err != nil {
		b.Logger(fmt.Sprintf("Failed to apply emulation to tab %s: %v", rawURL, err))
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(rawURL)); err != nil {
		b.forgetTab(tab)
		cancel()
		return errors.New("open tab " + rawURL + ": " + err.Error())
	}
	b.Mu.Lock()
	b.extraTabs = append(b.extraTabs, tab)
	b.Mu.Unlock()
	return nil
}

// toolCtx is the context every MCP tool acts on: the tab the developer has
// in front, or the main tab when that one is gone.
func (b *DevBrowser) toolCtx() context.Context {
	if t := b.toolTab(); t != nil {
		return t.ctx
	}
	return b.Ctx
}

// toolTab is the tab the developer has in front, or the main tab; nil while
// the browser is closed.
func (b *DevBrowser) toolTab() *browserTab {
	if t := b.active.Load(); t != nil && t.ctx != nil && t.ctx.Err() == nil {
		return t
	}
	b.Mu.Lock()
	defer b.Mu.Unlock()
	return b.mainTabLocked()
}

// mainTabLocked is the tab of Ctx, whoever set Ctx (CreateBrowserContext, or
// a caller assigning the exported field). Call with b.Mu held.
func (b *DevBrowser) mainTabLocked() *browserTab {
	if b.Ctx == nil {
		return nil
	}
	if b.mainTab == nil || b.mainTab.ctx != b.Ctx {
		b.mainTab = &browserTab{ctx: b.Ctx}
	}
	return b.mainTab
}

// CloseTabs closes every tab OpenTab added and keeps the main one, e.g. when
// the browser switches to another project whose users are other hosts.
func (b *DevBrowser) CloseTabs() {
	b.Mu.Lock()
	tabs := b.extraTabs
	b.extraTabs = nil
	main := b.mainTab
	b.Mu.Unlock()
	for _, t := range tabs {
		t.cancel()
	}
	if main != nil {
		b.activateTab(main)
	}
}

// extraTabURLs lists what the extra tabs opened, in order.
func (b *DevBrowser) extraTabURLs() []string {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	urls := make([]string, len(b.extraTabs))
	for i, t := range b.extraTabs {
		urls[i] = t.url
	}
	return urls
}

// allTabCtxs is the main tab and every extra tab, for settings that apply to
// the whole browser (device emulation).
func (b *DevBrowser) allTabCtxs() []context.Context {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	var out []context.Context
	if b.Ctx != nil {
		out = append(out, b.Ctx)
	}
	for _, t := range b.extraTabs {
		out = append(out, t.ctx)
	}
	return out
}

// watchTab makes tab report when it comes to the front (tabActiveJS) and
// when it goes away.
func (b *DevBrowser) watchTab(tab *browserTab) {
	chromedp.ListenTarget(tab.ctx, func(ev any) {
		switch e := ev.(type) {
		case *runtime.EventBindingCalled:
			if e.Name == tabActiveBinding {
				go b.activateTab(tab) // never wait on a lock on chromedp's event loop
			}
		case *inspector.EventDetached:
			go b.forgetTab(tab)
		}
	})
	_ = chromedp.Run(tab.ctx,
		runtime.Enable(),
		page.Enable(),
		runtime.AddBinding(tabActiveBinding),
		chromedp.ActionFunc(func(c context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(tabActiveJS).Do(c)
			return err
		}),
	)
}

// activateTab makes tab the one MCP tools read and act on: the buffers of
// the tab that was active go back into it, and tab's come out.
func (b *DevBrowser) activateTab(tab *browserTab) {
	b.LogsMutex.Lock()
	b.NetworkMutex.Lock()
	b.ErrorsMutex.Lock()
	old := b.active.Load()
	if old != tab {
		if old != nil {
			old.console, old.network, old.errors = b.ConsoleLogs, b.NetworkLogs, b.JsErrors
		}
		b.ConsoleLogs, b.NetworkLogs, b.JsErrors = orEmpty(tab.console), orEmptyNet(tab.network), orEmptyErr(tab.errors)
		tab.console, tab.network, tab.errors = nil, nil, nil
		b.active.Store(tab)
	}
	b.ErrorsMutex.Unlock()
	b.NetworkMutex.Unlock()
	b.LogsMutex.Unlock()
}

// forgetTab drops a tab that closed; if it was the active one, the main tab
// takes over.
func (b *DevBrowser) forgetTab(tab *browserTab) {
	b.Mu.Lock()
	kept := b.extraTabs[:0]
	for _, t := range b.extraTabs {
		if t != tab {
			kept = append(kept, t)
		}
	}
	b.extraTabs = kept
	main := b.mainTab
	b.Mu.Unlock()
	if b.active.Load() == tab && main != nil && main != tab {
		b.activateTab(main)
	}
}

// consoleOf, networkOf and errorsOf return the buffer tab writes to: the
// shared one while tab is active, its own otherwise. Call with the matching
// mutex held (LogsMutex, NetworkMutex, ErrorsMutex).
func (b *DevBrowser) consoleOf(tab *browserTab) *[]string {
	if b.active.Load() == tab {
		return &b.ConsoleLogs
	}
	return &tab.console
}

func (b *DevBrowser) networkOf(tab *browserTab) *[]NetworkLogEntry {
	if b.active.Load() == tab {
		return &b.NetworkLogs
	}
	return &tab.network
}

func (b *DevBrowser) errorsOf(tab *browserTab) *[]JSError {
	if b.active.Load() == tab {
		return &b.JsErrors
	}
	return &tab.errors
}

func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func orEmptyNet(v []NetworkLogEntry) []NetworkLogEntry {
	if v == nil {
		return []NetworkLogEntry{}
	}
	return v
}

func orEmptyErr(v []JSError) []JSError {
	if v == nil {
		return []JSError{}
	}
	return v
}

// reloadExtraTabs reloads every tab OpenTab added; one the user closed by hand
// fails to reload and is forgotten.
func (b *DevBrowser) reloadExtraTabs() {
	b.Mu.Lock()
	tabs := append([]*browserTab(nil), b.extraTabs...)
	b.Mu.Unlock()
	for _, t := range tabs {
		if err := chromedp.Run(t.ctx, chromedp.Reload()); err != nil {
			t.cancel()
			b.forgetTab(t)
		}
	}
}
