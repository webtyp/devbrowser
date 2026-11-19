package devbrowser

import (
	"fmt"
	"strings"

	"webtyp.com/context"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

// GetPerformanceJS extracts page performance metrics optimized for LLM consumption.
const GetPerformanceJS = `
(() => {
	const m = {};

	// JS Heap (Chrome only)
	if (performance.memory) {
		m.heapUsed = performance.memory.usedJSHeapSize;
		m.heapTotal = performance.memory.totalJSHeapSize;
		m.heapLimit = performance.memory.jsHeapSizeLimit;
	}

	// Navigation timing
	const nav = performance.getEntriesByType('navigation')[0];
	if (nav) {
		m.domInteractive = Math.round(nav.domInteractive - nav.startTime);
		m.domLoaded = Math.round(nav.domContentLoadedEventEnd - nav.startTime);
		m.fullLoad = Math.round(nav.loadEventEnd - nav.startTime);
	}

	// Paint timing
	const paints = performance.getEntriesByType('paint');
	for (let i = 0; i < paints.length; i++) {
		if (paints[i].name === 'first-paint') m.fp = Math.round(paints[i].startTime);
		if (paints[i].name === 'first-contentful-paint') m.fcp = Math.round(paints[i].startTime);
	}

	// DOM stats
	m.domNodes = document.querySelectorAll('*').length;
	let maxDepth = 0;
	const walk = (el, d) => {
		if (d > maxDepth) maxDepth = d;
		const children = el.children;
		for (let i = 0; i < children.length; i++) walk(children[i], d + 1);
	};
	if (document.body) walk(document.body, 0);
	m.domDepth = maxDepth;

	// Resources summary
	const resources = performance.getEntriesByType('resource');
	m.resourceCount = resources.length;
	let totalTransfer = 0;
	const wasmFiles = [];
	for (let i = 0; i < resources.length; i++) {
		const r = resources[i];
		totalTransfer += r.transferSize || 0;
		if (r.name.endsWith('.wasm')) {
			wasmFiles.push({
				name: r.name.split('/').pop(),
				size: Math.round(r.transferSize / 1024),
				duration: Math.round(r.duration)
			});
		}
	}
	m.totalTransferKB = Math.round(totalTransfer / 1024);
	m.wasmFiles = wasmFiles;

	return m;
})()
`

// FormatPerformanceReport builds a compact text report from raw JS metrics.
func FormatPerformanceReport(pageURL string, metrics map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Performance: %s\n", pageURL)

	// Memory
	if heapUsed, ok := metrics["heapUsed"].(float64); ok {
		heapTotal, _ := metrics["heapTotal"].(float64)
		heapLimit, _ := metrics["heapLimit"].(float64)
		fmt.Fprintf(&b, "Memory:    JS Heap %.1f/%.1f MB (limit %d MB)\n",
			heapUsed/1048576, heapTotal/1048576, int(heapLimit/1048576))
	}

	// Timing
	domInteractive, hasInteractive := metrics["domInteractive"].(float64)
	domLoaded, hasLoaded := metrics["domLoaded"].(float64)
	fullLoad, hasFullLoad := metrics["fullLoad"].(float64)
	if hasInteractive || hasLoaded || hasFullLoad {
		b.WriteString("Timing:    ")
		sep := ""
		if hasInteractive {
			fmt.Fprintf(&b, "Interactive %dms", int(domInteractive))
			sep = " | "
		}
		if hasLoaded {
			fmt.Fprintf(&b, "%sDOM Loaded %dms", sep, int(domLoaded))
			sep = " | "
		}
		if hasFullLoad {
			fmt.Fprintf(&b, "%sFull Load %dms", sep, int(fullLoad))
		}
		b.WriteString("\n")
	}

	// Paint
	fp, hasFP := metrics["fp"].(float64)
	fcp, hasFCP := metrics["fcp"].(float64)
	if hasFP || hasFCP {
		b.WriteString("Paint:     ")
		sep := ""
		if hasFP {
			fmt.Fprintf(&b, "FP %dms", int(fp))
			sep = " | "
		}
		if hasFCP {
			fmt.Fprintf(&b, "%sFCP %dms", sep, int(fcp))
		}
		b.WriteString("\n")
	}

	// DOM
	if nodes, ok := metrics["domNodes"].(float64); ok {
		depth, _ := metrics["domDepth"].(float64)
		fmt.Fprintf(&b, "DOM:       %d nodes | max depth %d\n", int(nodes), int(depth))
	}

	// WASM files
	if wasmFiles, ok := metrics["wasmFiles"].([]any); ok && len(wasmFiles) > 0 {
		for _, wf := range wasmFiles {
			if entry, ok := wf.(map[string]any); ok {
				name, _ := entry["name"].(string)
				size, _ := entry["size"].(float64)
				duration, _ := entry["duration"].(float64)
				fmt.Fprintf(&b, "WASM:      %s %d KB (loaded in %dms)\n", name, int(size), int(duration))
			}
		}
	}

	// Resources total
	if count, ok := metrics["resourceCount"].(float64); ok {
		totalKB, _ := metrics["totalTransferKB"].(float64)
		fmt.Fprintf(&b, "Resources: %d total | %d KB transferred\n", int(count), int(totalKB))
	}

	return b.String()
}

func (b *DevBrowser) GetPerformanceTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_get_performance",
			Description: "Get page performance metrics (memory, timing, DOM stats, WASM resources) to diagnose excessive RAM usage, slow loads, or rendering issues. Returns a compact text report optimized for minimal token usage.",
			Args: new(GetPerformanceArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(Ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args GetPerformanceArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				var pageURL string
				var metrics map[string]any

				err := chromedp.Run(b.toolCtx(),
					chromedp.Location(&pageURL),
					chromedp.Evaluate(GetPerformanceJS, &metrics),
				)
				if err != nil {
					return nil, fmt.Errorf("Failed to get performance metrics: %v", err)
				}

				report := FormatPerformanceReport(pageURL, metrics)
				return mcp.Text(report), nil
			},
		},
	}
}
