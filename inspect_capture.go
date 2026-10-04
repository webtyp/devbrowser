package devbrowser

import (
	"webtyp.com/devbrowser/cdproto/cdp"
	"webtyp.com/devbrowser/cdproto/dom"
	"webtyp.com/devbrowser/cdproto/overlay"
	"webtyp.com/devbrowser/cdproto/runtime"
	"webtyp.com/devbrowser/chromedp"
)

const injectInspectListenerJS = `
(() => {
	if (window.__webtyp_inspect_installed) return;
	window.__webtyp_inspect_installed = true;
	const track = (e) => {
		if (e.target instanceof Element && e.target !== document.body && e.target !== document.documentElement) {
			window.__webtyp_last_clicked = e.target;
		}
	};
	document.addEventListener('pointerdown', track, true);
	document.addEventListener('mousedown', track, true);
	document.addEventListener('click', track, true);
	document.addEventListener('focusin', track, true);
})();
`

func (b *DevBrowser) initializeInspectCapture() {
	if b.Ctx == nil {
		return
	}

	// Listen for CDP Overlay.inspectNodeRequested events (fires when developer uses DevTools inspect pointer)
	chromedp.ListenTarget(b.Ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *overlay.EventInspectNodeRequested:
			b.Mu.Lock()
			b.LastInspectedBackendNodeID = e.BackendNodeID
			ctx := b.Ctx
			b.Mu.Unlock()

			if ctx != nil && e.BackendNodeID != 0 {
				go func(bID cdp.BackendNodeID) {
					obj, err := dom.ResolveNode().WithBackendNodeID(bID).Do(ctx)
					if err == nil && obj != nil && obj.ObjectID != "" {
						_, _, _ = runtime.CallFunctionOn("function() { window.__webtyp_selected = this; }").
							WithObjectID(obj.ObjectID).
							Do(ctx)
					}
				}(e.BackendNodeID)
			}
		}
	})

	// Enable DOM and Overlay domains so inspect events are dispatched
	go func() {
		_ = chromedp.Run(b.Ctx,
			dom.Enable(),
			overlay.Enable(),
		)
	}()
}
