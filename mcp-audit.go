package devbrowser

import (
	"fmt"
	"strings"

	"webtyp.com/context"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

// GetAuditMobileJS is the mobile compatibility audit script.
const GetAuditMobileJS = `
(selector) => {
	const report = {
		viewportMeta: { missing: false, missingFit: false },
		vhUnits: [],
		safeArea: [],
		inputZoom: [],
		tapTarget: [],
		fixedVh: []
	};

	const root = selector ? document.querySelector(selector) : document.body;
	if (!root) return report;

	// 1. viewport-meta check
	const meta = document.querySelector('meta[name="viewport"]');
	if (!meta) {
		report.viewportMeta.missing = true;
	} else {
		const content = meta.getAttribute('content') || '';
		if (!content.includes('viewport-fit=cover')) {
			report.viewportMeta.missingFit = true;
		}
	}

	// Helper to find selectors with safe-area-inset or vh
	const vhSelectors = [];
	const safeAreaSelectors = [];
	try {
		for (const sheet of document.styleSheets) {
			try {
				const rules = sheet.cssRules || sheet.rules;
				if (!rules) continue;
				for (let i = 0; i < rules.length; i++) {
					const rule = rules[i];
					if (rule.style) {
						const text = rule.style.cssText || '';
						if (text.includes('vh')) {
							vhSelectors.push(rule.selectorText);
						}
						if (text.includes('safe-area-inset')) {
							safeAreaSelectors.push(rule.selectorText);
						}
					}
				}
			} catch (e) {}
		}
	} catch (e) {}

	const hasVhUnit = (el) => {
		if (el.style && (String(el.style.height).includes('vh') || String(el.style.minHeight).includes('vh'))) {
			return true;
		}
		for (let i = 0; i < vhSelectors.length; i++) {
			try {
				if (vhSelectors[i] && el.matches(vhSelectors[i])) {
					return true;
				}
			} catch (e) {}
		}
		return false;
	};

	const usesSafeArea = (el) => {
		if (el.style && (String(el.style.padding).includes('safe-area-inset') ||
		                 String(el.style.margin).includes('safe-area-inset') ||
		                 String(el.style.top).includes('safe-area-inset') ||
		                 String(el.style.bottom).includes('safe-area-inset'))) {
			return true;
		}
		for (let i = 0; i < safeAreaSelectors.length; i++) {
			try {
				if (safeAreaSelectors[i] && el.matches(safeAreaSelectors[i])) {
					return true;
				}
			} catch (e) {}
		}
		return false;
	};

	const getSelector = (el) => {
		if (el.id) return '#' + el.id;
		if (el.className) {
			const firstClass = String(el.className).split(' ')[0];
			if (firstClass) return el.tagName.toLowerCase() + '.' + firstClass;
		}
		return el.tagName.toLowerCase();
	};

	// Walk tree
	const elements = root.querySelectorAll('*');
	const allElements = [root, ...Array.from(elements)];
	allElements.forEach((el) => {
		if (!el || el === document || el === window) return;
		const style = window.getComputedStyle(el);

		// 2. vh-units
		if (hasVhUnit(el)) {
			report.vhUnits.push(getSelector(el));
		}

		// 3. safe-area check
		const isFixedOrAbsolute = style.position === 'fixed' || style.position === 'absolute';
		const isAtEdge = parseFloat(style.top) === 0 || parseFloat(style.bottom) === 0;
		if (isFixedOrAbsolute && isAtEdge) {
			if (!usesSafeArea(el)) {
				report.safeArea.push(getSelector(el));
			}
		}

		// 4. input-zoom
		const isInput = ['INPUT', 'SELECT', 'TEXTAREA'].includes(el.tagName);
		if (isInput) {
			const fontSize = parseFloat(style.fontSize);
			if (fontSize && fontSize < 16) {
				report.inputZoom.push(getSelector(el) + ' (' + style.fontSize + ')');
			}
		}

		// 5. tap-target
		const isInteractive = ['A', 'BUTTON', 'INPUT', 'SELECT', 'TEXTAREA'].includes(el.tagName) ||
		                      el.getAttribute('role') === 'button' ||
		                      style.cursor === 'pointer';
		if (isInteractive) {
			const rect = el.getBoundingClientRect();
			if (rect.width > 0 && rect.height > 0 && (rect.width < 44 || rect.height < 44)) {
				report.tapTarget.push(getSelector(el) + ' (' + Math.round(rect.width) + 'x' + Math.round(rect.height) + 'px)');
			}
		}

		// 6. fixed-vh
		if (style.position === 'fixed' && hasVhUnit(el)) {
			report.fixedVh.push(getSelector(el));
		}
	});

	// Deduplicate arrays
	report.vhUnits = [...new Set(report.vhUnits)];
	report.safeArea = [...new Set(report.safeArea)];
	report.inputZoom = [...new Set(report.inputZoom)];
	report.tapTarget = [...new Set(report.tapTarget)];
	report.fixedVh = [...new Set(report.fixedVh)];

	return report;
}
`

type AuditViewportMeta struct {
	Missing    bool `json:"missing"`
	MissingFit bool `json:"missingFit"`
}

type AuditMobileReport struct {
	ViewportMeta AuditViewportMeta `json:"viewportMeta"`
	VhUnits      []string          `json:"vhUnits"`
	SafeArea     []string          `json:"safeArea"`
	InputZoom    []string          `json:"inputZoom"`
	TapTarget    []string          `json:"tapTarget"`
	FixedVh      []string          `json:"fixedVh"`
}

func FormatAuditMobileReport(pageURL string, r *AuditMobileReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Mobile Audit Report: %s\n\n", pageURL)

	// Viewport Meta
	sb.WriteString("1. Viewport Meta Check:\n")
	if r.ViewportMeta.Missing {
		sb.WriteString("   [FAIL] <meta name=\"viewport\"> is missing! iOS will render at 980px zoom, breaking layouts.\n")
	} else if r.ViewportMeta.MissingFit {
		sb.WriteString("   [FAIL] viewport-fit=cover is missing in viewport meta. Safe area insets (safe-area-inset-*) will resolve to 0.\n")
	} else {
		sb.WriteString("   [PASS] Viewport meta present with viewport-fit=cover.\n")
	}
	sb.WriteString("\n")

	// VH Units
	sb.WriteString("2. VH Units Check:\n")
	if len(r.VhUnits) > 0 {
		fmt.Fprintf(&sb, "   [FAIL] Found %d element(s) using 'vh' units. '100vh' does not shrink when mobile browser bars contract/expand; use dvh/svh instead.\n", len(r.VhUnits))
		for _, el := range r.VhUnits {
			fmt.Fprintf(&sb, "     - %s\n", el)
		}
	} else {
		sb.WriteString("   [PASS] No vh units found.\n")
	}
	sb.WriteString("\n")

	// Safe Area
	sb.WriteString("3. Safe Area Check:\n")
	if len(r.SafeArea) > 0 {
		fmt.Fprintf(&sb, "   [FAIL] Found %d fixed/absolute element(s) at edge without safe-area-inset padding/margin. Content might be covered by notches or home bars.\n", len(r.SafeArea))
		for _, el := range r.SafeArea {
			fmt.Fprintf(&sb, "     - %s\n", el)
		}
	} else {
		sb.WriteString("   [PASS] All edge fixed/absolute elements use safe-area-inset padding/margin.\n")
	}
	sb.WriteString("\n")

	// Input Zoom
	sb.WriteString("4. Input Zoom Check:\n")
	if len(r.InputZoom) > 0 {
		fmt.Fprintf(&sb, "   [FAIL] Found %d input element(s) with font-size < 16px. iOS Safari will trigger automatic layout-shifting zoom when focused.\n", len(r.InputZoom))
		for _, el := range r.InputZoom {
			fmt.Fprintf(&sb, "     - %s\n", el)
		}
	} else {
		sb.WriteString("   [PASS] All form inputs have font-size >= 16px.\n")
	}
	sb.WriteString("\n")

	// Tap Target
	sb.WriteString("5. Tap Target Check:\n")
	if len(r.TapTarget) > 0 {
		fmt.Fprintf(&sb, "   [FAIL] Found %d interactive element(s) with tap bounding box smaller than 44x44 px (Apple HIG recommendation).\n", len(r.TapTarget))
		for _, el := range r.TapTarget {
			fmt.Fprintf(&sb, "     - %s\n", el)
		}
	} else {
		sb.WriteString("   [PASS] All interactive elements have tap sizes >= 44x44 px.\n")
	}
	sb.WriteString("\n")

	// Fixed VH
	sb.WriteString("6. Fixed VH Check:\n")
	if len(r.FixedVh) > 0 {
		fmt.Fprintf(&sb, "   [FAIL] Found %d fixed position element(s) using 'vh' height units. This causes severe rendering jumpiness on mobile scrolling.\n", len(r.FixedVh))
		for _, el := range r.FixedVh {
			fmt.Fprintf(&sb, "     - %s\n", el)
		}
	} else {
		sb.WriteString("   [PASS] No fixed position elements are using 'vh' heights.\n")
	}

	return sb.String()
}

func (b *DevBrowser) GetAuditTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_audit_mobile",
			Description: "Run mobile compatibility audits on the page or a sub-tree to identify issues with missing viewport metas, unsafe areas under notch/home bar, auto-zooming inputs under 16px, or small tapping target sizes. Returns a compact text report.",
			Args:        new(AuditMobileArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(Ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args AuditMobileArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				var pageURL string
				var report AuditMobileReport

				js := fmt.Sprintf("(%s)(%q)", GetAuditMobileJS, args.Selector)
				err := chromedp.Run(b.toolCtx(),
					chromedp.Location(&pageURL),
					chromedp.Evaluate(js, &report),
				)
				if err != nil {
					return nil, fmt.Errorf("Failed to run mobile audit: %v", err)
				}

				reportStr := FormatAuditMobileReport(pageURL, &report)
				return mcp.Text(reportStr), nil
			},
		},
	}
}
