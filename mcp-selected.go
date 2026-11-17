package devbrowser

import (
	"fmt"
	"strings"

	wtctx "webtyp.com/context"
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

// Tool messages of browser_get_selected_element.
const (
	msgSelectionCleared  = "Cleared %d selection(s)."
	msgNoSelection       = "No element selected. Alt+click an element in the page, or use the DevTools inspect pointer, then call this tool again."
	msgSelectionCountErr = "count must be between 1 and %d"
	msgSelectionHeader   = "Selection #%d (%s, %s, page %s)"
	msgSelectionMore     = "History holds %d selection(s); call with count=%d to see them all."
)

func (b *DevBrowser) GetSelectedElementTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_get_selected_element",
			Description: "Get the latest elements the developer selected in the browser — by Alt+click on the page (marked with a numbered badge), the DevTools inspect pointer, or the DevTools Elements panel. Keeps the last 10 as snapshots taken at selection time. Args: count (1-10, default 1) returns the N most recent, newest first; clear=true empties the history and removes the badges. Each selection includes WebTyp identifiers, hierarchy, outer HTML, geometry, source code locations and a cropped screenshot.",
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
				b.installSelectionCapture()

				var args GetSelectedElementArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				if args.Clear {
					b.Mu.Lock()
					n := b.selections.clear()
					b.Mu.Unlock()
					_ = chromedp.Run(b.Ctx, chromedp.Evaluate(clearSelectionBadgesJS, nil))
					return mcp.Text(fmt.Sprintf(msgSelectionCleared, n)), nil
				}

				count := int(args.Count)
				if count == 0 {
					count = 1
				}
				if count < 1 || count > selectionHistoryCap {
					return nil, fmt.Errorf(msgSelectionCountErr, selectionHistoryCap)
				}

				// Compared against the last panel node, not the newest history item,
				// so an old panel selection is not re-added after a newer alt+click.
				if id := b.devToolsPanelNodeID(); id != 0 {
					b.Mu.Lock()
					isNew := id != b.lastPanelNodeID
					b.lastPanelNodeID = id
					b.Mu.Unlock()
					if isNew && b.enqueueNode(b.Ctx, id) {
						b.captureSelection(SelectionDevToolsPanel)
					}
				}

				b.Mu.Lock()
				items := b.selections.latest(count)
				held := b.selections.len()
				b.Mu.Unlock()
				if len(items) == 0 {
					return mcp.Text(msgNoSelection), nil
				}

				var blocks []mcp.ContentBlock
				for _, s := range items {
					header := fmt.Sprintf(msgSelectionHeader, s.seq, s.source, s.at.Format("15:04:05"), s.pageURL)
					blocks = append(blocks, mcp.TextBlock(header+"\n"+s.report))
					if s.screenshot != nil {
						blocks = append(blocks, mcp.ImageBlock(s.screenshot, "image/png"))
					}
				}
				if held > count {
					blocks = append(blocks, mcp.TextBlock(fmt.Sprintf(msgSelectionMore, held, held)))
				}
				return mcp.NewResult(blocks...), nil
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

	fmt.Fprintf(&sb, "Selected Element: <%s", data.Identity.TagName)
	if idStr != "" {
		fmt.Fprintf(&sb, " id=%q", idStr)
	}
	if classStr != "" {
		fmt.Fprintf(&sb, " class=%q", classStr)
	}
	if dataKeyStr != "" {
		fmt.Fprintf(&sb, " data-key=%q", dataKeyStr)
	}
	sb.WriteString(">\n")

	if data.Breadcrumbs != "" {
		fmt.Fprintf(&sb, "Hierarchy (Breadcrumbs): %s\n", data.Breadcrumbs)
	}
	if data.Identity.Text != "" {
		fmt.Fprintf(&sb, "Visible Text: %q\n", data.Identity.Text)
	}

	sb.WriteString("\nWebTyp Identifiers:\n")
	fmt.Fprintf(&sb, "- Tag: %s\n", data.Identity.TagName)
	if idStr != "" {
		fmt.Fprintf(&sb, "- ID: %s\n", idStr)
	}
	if dataKeyStr != "" {
		fmt.Fprintf(&sb, "- Data-Key: %s\n", dataKeyStr)
	}
	if classStr != "" {
		fmt.Fprintf(&sb, "- Classes: %s\n", classStr)
	}
	if data.Identity.DataComponent != nil && *data.Identity.DataComponent != "" {
		fmt.Fprintf(&sb, "- Component: %s\n", *data.Identity.DataComponent)
	}

	sb.WriteString("\nGeometry & Layout:\n")
	fmt.Fprintf(&sb, "- Size: %.1f x %.1f px\n", data.BoxModel.Width, data.BoxModel.Height)
	if disp, ok := data.Layout["display"].(string); ok && disp != "" {
		fmt.Fprintf(&sb, "- Display: %s\n", disp)
	}
	fmt.Fprintf(&sb, "- Viewport Position: (x: %.1f, y: %.1f)\n", data.Position.Left, data.Position.Top)

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
				fmt.Fprintf(&sb, "- [%s] %s:%d", loc.Kind, loc.File, loc.Line)
				if loc.Token != "" {
					fmt.Fprintf(&sb, " (%s)", loc.Token)
				}
				sb.WriteString("\n")
				if loc.Match != "" {
					fmt.Fprintf(&sb, "  `%s`\n", loc.Match)
				}
				if loc.Origin != "" {
					fmt.Fprintf(&sb, "  origin: %s\n", loc.Origin)
				}
			}
		}
	}

	return sb.String()
}
