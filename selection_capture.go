package devbrowser

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"webtyp.com/devbrowser/cdproto/cdp"
	"webtyp.com/devbrowser/cdproto/dom"
	"webtyp.com/devbrowser/cdproto/overlay"
	"webtyp.com/devbrowser/cdproto/page"
	"webtyp.com/devbrowser/cdproto/runtime"
	"webtyp.com/devbrowser/cdproto/target"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/lang"
)

const (
	selectionBindingName  = "__webtyp_mark"          // CDP binding the page calls on alt+click
	selectionQueueGlobal  = "__webtyp_capture_queue" // window array of elements waiting to be captured
	selectionTargetGlobal = "__webtyp_capture_target"
	selectionBadgeAttr    = "data-webtyp-mark"
)

// selectionPageScriptJS marks the alt+clicked element for capture and keeps the
// click away from the app. It is idempotent and installed on every new document.
var selectionPageScriptJS = fmt.Sprintf(`(() => {
	if (window.__webtyp_selection_installed) return;
	window.__webtyp_selection_installed = true;
	window.%[2]s = window.%[2]s || [];
	// The selection gesture lives in this one line. On desktops where Alt is the
	// window-move modifier (XFCE, KDE before Plasma 6) the click never reaches
	// the page; changing the gesture is changing this line.
	const isMark = (e) => e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey;
	const handle = (e) => {
		if (!isMark(e)) return;
		const t = e.target;
		if (!(t instanceof Element) || t === document.body || t === document.documentElement || t.hasAttribute('%[3]s')) return;
		e.preventDefault();
		e.stopImmediatePropagation();
		if (e.type !== 'click') return;
		window.%[2]s.push(t);
		if (typeof window.%[1]s === 'function') window.%[1]s('%[4]s');
	};
	// window, capture phase: runs before any listener the app adds on document.
	window.addEventListener('mousedown', handle, true);
	window.addEventListener('click', handle, true);
})()`, selectionBindingName, selectionQueueGlobal, selectionBadgeAttr, SelectionAltClick)

// enqueueSelectionJS is called on a resolved node (this) to queue it for capture.
var enqueueSelectionJS = fmt.Sprintf(`function() { (window.%[1]s = window.%[1]s || []).push(this); }`, selectionQueueGlobal)

// captureSelectionJS takes the next queued element, scrolls it into view and
// describes it.
//
// A click on an icon lands on an SVG <path> or <circle>, which nobody means to
// select: the target climbs out of the outermost <svg> to the control that owns
// the icon, or stays on that <svg> when no control does. Done here, for every
// selection source, so the duplicate check and the badge see the same node.
var captureSelectionJS = fmt.Sprintf(`(() => {
	%[1]s
	let target = (window.%[2]s || []).shift() || null;
	if (target instanceof Element && target.closest('svg')) {
		let svg = target.closest('svg');
		for (let p = svg.parentElement; p; p = p.parentElement) {
			if (p.tagName.toLowerCase() === 'svg') svg = p;
		}
		const control = svg.parentElement && svg.parentElement.closest('button, a[href], label, summary, [role="button"], [role="link"], [role="tab"], [role="menuitem"], [role="option"], [role="checkbox"], [role="switch"]');
		target = control || svg;
	}
	window.%[3]s = target;
	if (!target || !(target instanceof Element)) {
		return JSON.stringify({ hasSelection: false });
	}
	try {
		target.scrollIntoView({ block: 'nearest', inline: 'nearest' });
	} catch (e) {}
	const details = extractElementDetails(target);
	if (!details) {
		return JSON.stringify({ hasSelection: false });
	}
	return JSON.stringify({ hasSelection: true, ...details });
})()`, ExtractElementDetailsFunctionJS, selectionQueueGlobal, selectionTargetGlobal)

var applyHighlightJS = fmt.Sprintf(`(() => {
	const el = window.%s;
	if (!el || !(el instanceof Element)) return;
	window.__webtyp_prev_outline = el.style.outline;
	window.__webtyp_prev_outlineOffset = el.style.outlineOffset;
	window.__webtyp_prev_boxShadow = el.style.boxShadow;
	el.style.setProperty('outline', '3px solid #2563eb', 'important');
	el.style.setProperty('outline-offset', '2px', 'important');
	el.style.setProperty('box-shadow', '0 0 0 4px rgba(37, 99, 235, 0.35)', 'important');
})()`, selectionTargetGlobal)

var removeHighlightJS = fmt.Sprintf(`(() => {
	const el = window.%s;
	if (!el || !(el instanceof Element)) return;
	if (typeof window.__webtyp_prev_outline === 'string' && window.__webtyp_prev_outline !== '') {
		el.style.outline = window.__webtyp_prev_outline;
	} else {
		el.style.removeProperty('outline');
	}
	if (typeof window.__webtyp_prev_outlineOffset === 'string' && window.__webtyp_prev_outlineOffset !== '') {
		el.style.outlineOffset = window.__webtyp_prev_outlineOffset;
	} else {
		el.style.removeProperty('outline-offset');
	}
	if (typeof window.__webtyp_prev_boxShadow === 'string' && window.__webtyp_prev_boxShadow !== '') {
		el.style.boxShadow = window.__webtyp_prev_boxShadow;
	} else {
		el.style.removeProperty('box-shadow');
	}
	delete window.__webtyp_prev_outline;
	delete window.__webtyp_prev_outlineOffset;
	delete window.__webtyp_prev_boxShadow;
})()`, selectionTargetGlobal)

// selectionBadgeJS pins the numbered badge (%[3]d) over the captured element.
const selectionBadgeJS = `(() => {
	const el = window.%[1]s;
	if (!el || !(el instanceof Element) || !document.body) return;
	const r = el.getBoundingClientRect();
	const b = document.createElement('div');
	b.setAttribute('%[2]s', '%[3]d');
	b.textContent = '%[3]d';
	b.style.cssText = 'position:absolute; left:' + (r.left + window.scrollX) + 'px; top:' + (r.top + window.scrollY) + 'px; z-index:2147483647; background:#2563eb; color:#fff; font:600 11px/16px system-ui,sans-serif; padding:0 5px; border-radius:8px; pointer-events:none;';
	document.body.appendChild(b);
})()`

// clearSelectionBadgesJS removes every badge from the page.
var clearSelectionBadgesJS = fmt.Sprintf(`document.querySelectorAll('[%s]').forEach(e => e.remove())`, selectionBadgeAttr)

// devToolsPanelNodeJS reads the node selected in the DevTools Elements panel.
const devToolsPanelNodeJS = `(() => {
	let bID = 0;
	const elementsPanel = window.UI?.panels?.elements;
	let node = null;
	try {
		if (elementsPanel) {
			if (typeof elementsPanel.selectedDOMNode === 'function') {
				node = elementsPanel.selectedDOMNode();
			} else if (elementsPanel.treeOutline && typeof elementsPanel.treeOutline.selectedDOMNode === 'function') {
				node = elementsPanel.treeOutline.selectedDOMNode();
			}
		}
	} catch (e) {}
	if (node && typeof node.backendNodeId === 'function') {
		bID = node.backendNodeId();
	}
	return JSON.stringify({ bID: bID });
})()`

// installSelectionCapture listens for the three selection gestures. It is
// idempotent per browser context and synchronous, so a selection made right
// after it returns is captured.
func (b *DevBrowser) installSelectionCapture() {
	b.Mu.Lock()
	ctx := b.Ctx
	if ctx == nil || b.selectionCaptureInstalled {
		b.Mu.Unlock()
		return
	}
	b.selectionCaptureInstalled = true
	b.Mu.Unlock()

	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *overlay.EventInspectNodeRequested:
			if e.BackendNodeID != 0 {
				go func(id cdp.BackendNodeID) {
					if b.enqueueNode(ctx, id) {
						b.captureSelection(SelectionInspectPointer)
					}
				}(e.BackendNodeID)
			}
		case *runtime.EventBindingCalled:
			if e.Name == selectionBindingName {
				go b.captureSelection(SelectionAltClick)
			}
		case *page.EventFrameNavigated:
			// The main frame got a new document (reload or navigation): its
			// badges are gone, so the history they numbered goes with them.
			// An SPA route change (pushState) keeps the document and does not
			// fire this.
			if e.Frame != nil && e.Frame.ParentID == "" {
				b.Mu.Lock()
				b.selections.clear()
				b.Mu.Unlock()
			}
		}
	})

	// The binding must exist before the page script runs, and the script is
	// registered for every new document: a reload or a navigation keeps it.
	_ = chromedp.Run(ctx,
		dom.Enable(),
		overlay.Enable(),
		page.Enable(), // EventFrameNavigated, which clears the history on reload
		runtime.AddBinding(selectionBindingName),
		chromedp.ActionFunc(func(c context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(selectionPageScriptJS).Do(c)
			return err
		}),
		chromedp.Evaluate(selectionPageScriptJS, nil),
	)
}

// enqueueNode queues the node with backend id for the next capture.
func (b *DevBrowser) enqueueNode(ctx context.Context, id cdp.BackendNodeID) bool {
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		obj, err := dom.ResolveNode().WithBackendNodeID(id).Do(c)
		if err != nil {
			return err
		}
		if obj == nil || obj.ObjectID == "" {
			return fmt.Errorf("node %d not resolved", id)
		}
		_, _, err = runtime.CallFunctionOn(enqueueSelectionJS).WithObjectID(obj.ObjectID).Do(c)
		return err
	}))
	return err == nil
}

// captureSelection snapshots the next queued element into the history and
// marks it with its number on the page.
func (b *DevBrowser) captureSelection(source SelectionSource) {
	b.captureMu.Lock()
	defer b.captureMu.Unlock()

	b.Mu.Lock()
	ctx := b.Ctx
	b.Mu.Unlock()
	if ctx == nil {
		return
	}

	var rawJSON string
	if err := chromedp.Run(ctx, chromedp.Evaluate(captureSelectionJS, &rawJSON)); err != nil {
		return
	}
	var data selectedElementData
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil || !data.HasSelection {
		return
	}

	var nodeID cdp.BackendNodeID
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		obj, _, err := runtime.Evaluate("window." + selectionTargetGlobal).Do(c)
		if err != nil || obj == nil || obj.ObjectID == "" {
			return err
		}
		node, err := dom.DescribeNode().WithObjectID(obj.ObjectID).Do(c)
		if err == nil && node != nil {
			nodeID = node.BackendNodeID
		}
		return err
	}))

	report := b.formatSelectedElementReport(&data)
	screenshot := b.captureHighlighted(ctx, data.ViewportClip)

	var pageURL string
	_ = chromedp.Run(ctx, chromedp.Location(&pageURL))

	b.Mu.Lock()
	added, ok := b.selections.push(selection{
		source:     source,
		at:         time.Now(),
		pageURL:    pageURL,
		nodeID:     nodeID,
		report:     report,
		screenshot: screenshot,
	})
	b.Mu.Unlock()
	if !ok {
		return
	}

	// After the screenshot: the capture must not contain the badge.
	_ = chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(selectionBadgeJS, selectionTargetGlobal, selectionBadgeAttr, added.seq), nil))

	tail := data.Breadcrumbs
	if i := strings.LastIndex(tail, " > "); i >= 0 {
		tail = tail[i+len(" > "):]
	}
	b.Logger(lang.Translate("Selection", "#"+strconv.Itoa(added.seq), "captured", "(", string(source), "):", tail).String())
}

// captureHighlighted returns a PNG of clip with the capture target outlined,
// or nil when the element is not visible.
func (b *DevBrowser) captureHighlighted(ctx context.Context, clip selectedClip) []byte {
	if clip.Width <= 0 || clip.Height <= 0 {
		return nil
	}
	viewport := &page.Viewport{X: clip.X, Y: clip.Y, Width: clip.Width, Height: clip.Height, Scale: 1.0}

	_ = chromedp.Run(ctx, chromedp.Evaluate(applyHighlightJS, nil))
	defer func() { _ = chromedp.Run(ctx, chromedp.Evaluate(removeHighlightJS, nil)) }()

	var img []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		buf, err := page.CaptureScreenshot().WithClip(viewport).Do(c)
		img = buf
		return err
	}))
	if err != nil || len(img) == 0 {
		return nil
	}
	return img
}

// devToolsPanelNodeID returns the node selected in an open DevTools Elements
// panel, or 0.
func (b *DevBrowser) devToolsPanelNodeID() cdp.BackendNodeID {
	b.Mu.Lock()
	ctx := b.Ctx
	b.Mu.Unlock()
	if ctx == nil {
		return 0
	}

	var targets []*target.Info
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		var err error
		targets, err = target.GetTargets().Do(c)
		return err
	}))
	for _, tgt := range targets {
		if !strings.Contains(tgt.URL, "devtools") {
			continue
		}
		dtCtx, dtCancel := chromedp.NewContext(ctx, chromedp.WithTargetID(tgt.TargetID))
		var raw string
		_ = chromedp.Run(dtCtx, chromedp.Evaluate(devToolsPanelNodeJS, &raw))
		if dtC := chromedp.FromContext(dtCtx); dtC != nil && dtC.Target != nil {
			// Clear TargetID so chromedp cancels the session (DetachFromTarget)
			// without closing the DevTools tab (CloseTarget).
			dtC.Target.TargetID = ""
		}
		dtCancel()
		var parsed struct {
			BID int64 `json:"bID"`
		}
		if raw != "" && json.Unmarshal([]byte(raw), &parsed) == nil && parsed.BID != 0 {
			return cdp.BackendNodeID(parsed.BID)
		}
	}
	return 0
}
