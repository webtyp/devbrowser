package devbrowser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	wtctx "webtyp.com/context"
	"webtyp.com/devbrowser/cdproto/cdp"
	"webtyp.com/devbrowser/cdproto/dom"
	"webtyp.com/devbrowser/cdproto/page"
	"webtyp.com/devbrowser/cdproto/runtime"
	"webtyp.com/devbrowser/cdproto/target"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

type selectedElementData struct {
	HasSelection  bool                   `json:"hasSelection"`
	Identity      selectedIdentity       `json:"identity"`
	Attributes    map[string]string      `json:"attributes"`
	Breadcrumbs   string                 `json:"breadcrumbs"`
	OuterHTML     string                 `json:"outerHTML"`
	BoxModel      selectedBoxModel       `json:"boxModel"`
	Position      selectedPosition       `json:"position"`
	Layout        map[string]interface{} `json:"layout"`
	Accessibility map[string]interface{} `json:"accessibility"`
	ViewportClip  selectedClip           `json:"viewportClip"`
}

type selectedIdentity struct {
	TagName       string  `json:"tagName"`
	ID            *string `json:"id"`
	ClassName     *string `json:"className"`
	Name          *string `json:"name"`
	DataKey       *string `json:"dataKey"`
	DataComponent *string `json:"dataComponent"`
	Text          string  `json:"text"`
}

type selectedBoxModel struct {
	Width   float64            `json:"width"`
	Height  float64            `json:"height"`
	Padding map[string]float64 `json:"padding"`
	Margin  map[string]float64 `json:"margin"`
	Border  map[string]float64 `json:"border"`
}

type selectedPosition struct {
	Type   string  `json:"type"`
	Top    float64 `json:"top"`
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

type selectedClip struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// GetSelectedElementJS evaluates the element selected by inspect pointer, $0, or developer click.
var GetSelectedElementJS = fmt.Sprintf(`
(() => {
	%s
	let target = null;
	if (typeof window.__webtyp_selected !== 'undefined' && window.__webtyp_selected !== null && (window.__webtyp_selected instanceof Element)) {
		target = window.__webtyp_selected;
	}
	if (!target && typeof window.$0 !== 'undefined' && window.$0 !== null && (window.$0 instanceof Element)) {
		target = window.$0;
	}
	try {
		if (!target && typeof $0 !== 'undefined' && $0 !== null && ($0 instanceof Element)) {
			target = $0;
		}
	} catch (e) {}
	if (!target && typeof window.__webtyp_last_clicked !== 'undefined' && window.__webtyp_last_clicked !== null && (window.__webtyp_last_clicked instanceof Element)) {
		target = window.__webtyp_last_clicked;
	}
	if (!target && document.activeElement && document.activeElement !== document.body && document.activeElement !== document.documentElement) {
		target = document.activeElement;
	}

	if (!target) {
		return JSON.stringify({ hasSelection: false });
	}
	window.__webtyp_target_for_screenshot = target;
	try {
		target.scrollIntoView({ block: 'nearest', inline: 'nearest' });
	} catch (e) {}
	const details = extractElementDetails(target);
	if (!details) {
		return JSON.stringify({ hasSelection: false });
	}
	return JSON.stringify({ hasSelection: true, ...details });
})()
`, ExtractElementDetailsFunctionJS)

const applyHighlightJS = `(() => {
	const el = window.__webtyp_target_for_screenshot;
	if (!el || !(el instanceof Element)) return;
	window.__webtyp_prev_outline = el.style.outline;
	window.__webtyp_prev_outlineOffset = el.style.outlineOffset;
	window.__webtyp_prev_boxShadow = el.style.boxShadow;
	el.style.setProperty('outline', '3px solid #2563eb', 'important');
	el.style.setProperty('outline-offset', '2px', 'important');
	el.style.setProperty('box-shadow', '0 0 0 4px rgba(37, 99, 235, 0.35)', 'important');
})()`

const removeHighlightJS = `(() => {
	const el = window.__webtyp_target_for_screenshot;
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
})()`

func (b *DevBrowser) GetSelectedElementTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_get_selected_element",
			Description: "Get the browser element currently selected by the developer in Chrome DevTools ($0). Returns WebTyp identifiers (tag, id, data-key, classes), hierarchy breadcrumbs, outer HTML, geometry, and a cropped visual screenshot. Returns guidance if no element is selected.",
			Args:        new(GetSelectedElementArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(ctx *wtctx.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}
				if b.Ctx == nil {
					return nil, fmt.Errorf("browser context is nil")
				}

				b.Mu.Lock()
				bNodeID := b.LastInspectedBackendNodeID
				b.Mu.Unlock()

				// If no node was captured via inspect pointer, check if DevTools Elements panel has a selected node
				if bNodeID == 0 {
					var targets []*target.Info
					_ = chromedp.Run(b.Ctx, chromedp.ActionFunc(func(c context.Context) error {
						var err error
						targets, err = target.GetTargets().Do(c)
						return err
					}))
					for _, tgt := range targets {
						if strings.Contains(tgt.URL, "devtools") {
							dtCtx, dtCancel := chromedp.NewContext(b.Ctx, chromedp.WithTargetID(tgt.TargetID))
							var dtDiag string
							_ = chromedp.Run(dtCtx,
								chromedp.Evaluate(`(() => {
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

									return JSON.stringify({
										bID: bID,
										panelKeys: elementsPanel ? Object.keys(elementsPanel).filter(k => !k.startsWith('_')).slice(0, 30) : [],
										nodeFound: !!node,
										nodeTag: node ? (node.nodeName ? node.nodeName() : node.tagName) : null
									});
								})()`, &dtDiag),
							)
							if dtC := chromedp.FromContext(dtCtx); dtC != nil && dtC.Target != nil {
								// Clear TargetID so chromedp cancels the session (DetachFromTarget)
								// without closing the DevTools tab (CloseTarget).
								dtC.Target.TargetID = ""
							}
							dtCancel()
							if dtDiag != "" {
								var parsed struct {
									BID int64 `json:"bID"`
								}
								_ = json.Unmarshal([]byte(dtDiag), &parsed)
								if parsed.BID != 0 {
									bNodeID = cdp.BackendNodeID(parsed.BID)
									break
								}
							}
						}
					}
				}

				if bNodeID != 0 {
					_ = chromedp.Run(b.Ctx,
						dom.Enable(),
						chromedp.ActionFunc(func(c context.Context) error {
							obj, err := dom.ResolveNode().WithBackendNodeID(bNodeID).Do(c)
							if err == nil && obj != nil && obj.ObjectID != "" {
								_, _, _ = runtime.CallFunctionOn("function() { window.__webtyp_selected = this; }").
									WithObjectID(obj.ObjectID).
									Do(c)
							}
							return nil
						}),
					)
				}

				var rawJSON string
				err := chromedp.Run(b.Ctx,
					chromedp.Evaluate(GetSelectedElementJS, &rawJSON, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
						return p.WithIncludeCommandLineAPI(true)
					}),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to evaluate selected element: %v", err)
				}

				var data selectedElementData
				if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
					return nil, fmt.Errorf("failed to parse selected element data: %v", err)
				}

				if !data.HasSelection {
					return mcp.Text("No element currently selected in DevTools ($0 is null or undefined)."), nil
				}

				// Build clear text report for the agent
				report := b.formatSelectedElementReport(&data)

				// Capture cropped screenshot with context and visual highlight if dimensions are visible
				if data.ViewportClip.Width > 0 && data.ViewportClip.Height > 0 {
					clip := &page.Viewport{
						X:      data.ViewportClip.X,
						Y:      data.ViewportClip.Y,
						Width:  data.ViewportClip.Width,
						Height: data.ViewportClip.Height,
						Scale:  1.0,
					}

					var imgBytes []byte
					_ = chromedp.Run(b.Ctx, chromedp.Evaluate(applyHighlightJS, nil))
					defer func() {
						_ = chromedp.Run(b.Ctx, chromedp.Evaluate(removeHighlightJS, nil))
					}()

					err := chromedp.Run(b.Ctx,
						chromedp.ActionFunc(func(c context.Context) error {
							buf, err := page.CaptureScreenshot().WithClip(clip).Do(c)
							if err != nil {
								return err
							}
							imgBytes = buf
							return nil
						}),
					)

					if err == nil && len(imgBytes) > 0 {
						return mcp.NewResult(mcp.TextBlock(report), mcp.ImageBlock(imgBytes, "image/png")), nil
					}
				}

				return mcp.Text(report), nil
			},
		},
	}
}

func (b *DevBrowser) formatSelectedElementReport(data *selectedElementData) string {
	var sb strings.Builder
	idStr := ""
	if data.Identity.ID != nil {
		idStr = *data.Identity.ID
	}
	classStr := ""
	if data.Identity.ClassName != nil {
		classStr = *data.Identity.ClassName
	}
	dataKeyStr := ""
	if data.Identity.DataKey != nil {
		dataKeyStr = *data.Identity.DataKey
	}

	sb.WriteString(fmt.Sprintf("Selected Element: <%s", data.Identity.TagName))
	if idStr != "" {
		sb.WriteString(fmt.Sprintf(" id=%q", idStr))
	}
	if classStr != "" {
		sb.WriteString(fmt.Sprintf(" class=%q", classStr))
	}
	if dataKeyStr != "" {
		sb.WriteString(fmt.Sprintf(" data-key=%q", dataKeyStr))
	}
	sb.WriteString(">\n")

	if data.Breadcrumbs != "" {
		sb.WriteString(fmt.Sprintf("Hierarchy (Breadcrumbs): %s\n", data.Breadcrumbs))
	}
	if data.Identity.Text != "" {
		sb.WriteString(fmt.Sprintf("Visible Text: %q\n", data.Identity.Text))
	}

	sb.WriteString("\nWebTyp Identifiers:\n")
	sb.WriteString(fmt.Sprintf("- Tag: %s\n", data.Identity.TagName))
	if idStr != "" {
		sb.WriteString(fmt.Sprintf("- ID: %s\n", idStr))
	}
	if dataKeyStr != "" {
		sb.WriteString(fmt.Sprintf("- Data-Key: %s\n", dataKeyStr))
	}
	if classStr != "" {
		sb.WriteString(fmt.Sprintf("- Classes: %s\n", classStr))
	}
	if data.Identity.DataComponent != nil && *data.Identity.DataComponent != "" {
		sb.WriteString(fmt.Sprintf("- Component: %s\n", *data.Identity.DataComponent))
	}

	sb.WriteString("\nGeometry & Layout:\n")
	sb.WriteString(fmt.Sprintf("- Size: %.1f x %.1f px\n", data.BoxModel.Width, data.BoxModel.Height))
	if disp, ok := data.Layout["display"].(string); ok && disp != "" {
		sb.WriteString(fmt.Sprintf("- Display: %s\n", disp))
	}
	sb.WriteString(fmt.Sprintf("- Viewport Position: (x: %.1f, y: %.1f)\n", data.Position.Left, data.Position.Top))

	if data.OuterHTML != "" {
		sb.WriteString("\nOuter HTML:\n")
		sb.WriteString(data.OuterHTML)
		sb.WriteString("\n")
	}

	b.Mu.Lock()
	locator := b.SourceLocator
	b.Mu.Unlock()

	if locator != nil {
		var crumbs []string
		if data.Breadcrumbs != "" {
			crumbs = strings.Split(data.Breadcrumbs, " > ")
		}
		var classes []string
		if classStr != "" {
			classes = strings.Fields(classStr)
		}
		locations := locator.LocateSource(ElementSourceQuery{
			Tag:         data.Identity.TagName,
			ID:          idStr,
			Classes:     classes,
			DataKey:     dataKeyStr,
			Attributes:  data.Attributes,
			Breadcrumbs: crumbs,
		})
		if len(locations) > 0 {
			sb.WriteString("\nSource Code Location:\n")
			for _, loc := range locations {
				sb.WriteString(fmt.Sprintf("- File: %s:%d\n", loc.File, loc.Line))
				if loc.Match != "" {
					sb.WriteString(fmt.Sprintf("  Match: `%s`\n", loc.Match))
				}
			}
		}
	}

	return sb.String()
}
