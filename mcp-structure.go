package devbrowser

import (
	"fmt"

	"webtyp.com/context"
	"webtyp.com/devbrowser/chromedp"
	"webtyp.com/mcp"
)

// GetStructureJS is the JavaScript used to extract the page structure for LLM understanding.
const GetStructureJS = `
(() => {
	const getStructure = (el, depth = 0) => {
		if (depth > 12 || !el) return ''; 
		
		const tag = el.tagName.toLowerCase();
		const indent = '  '.repeat(depth);
		const style = window.getComputedStyle(el);
		
		// Skip invisible elements
		if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return '';
		
		// Get direct text content
		const directText = Array.from(el.childNodes)
			.filter(n => n.nodeType === 3)
			.map(n => n.textContent.trim())
			.filter(t => t)
			.join(' ');
		
		let result = indent + '<' + tag;
		if (el.id) result += ' id="' + el.id + '"';
		
		// Handle class names more cleanly
		if (el.className && typeof el.className === 'string') {
			const classes = el.className.split(/\s+/).filter(c => c).join(' ');
			if (classes) result += ' class="' + classes + '"';
		}
		
		// Add ARIA roles and labels which are critical for LLMs
		const role = el.getAttribute('role');
		const ariaLabel = el.getAttribute('aria-label');
		const placeholder = el.getAttribute('placeholder');
		const value = el.value;
		const name = el.getAttribute('name');
		const type = el.getAttribute('type');
		// NEW: Add critical navigation and media attributes
		const href = el.getAttribute('href');
		const src = el.getAttribute('src');
		const alt = el.getAttribute('alt');
		const title = el.getAttribute('title');

		if (role) result += ' role="' + role + '"';
		if (ariaLabel) result += ' aria-label="' + ariaLabel + '"';
		if (placeholder) result += ' placeholder="' + placeholder + '"';
		if (name) result += ' name="' + name + '"';
		if (type) result += ' type="' + type + '"';
		if (value && tag !== 'body') result += ' value="' + value + '"';
		
		if (href) result += ' href="' + href + '"';
		if (src) result += ' src="' + src + '"';
		if (alt) result += ' alt="' + alt + '"';
		if (title) result += ' title="' + title + '"';

		// Add critical visual styles (only if non-default)
		const styles = [];
		if (style.display !== 'block' && style.display !== 'inline' && style.display !== 'inline-block') {
			styles.push('display:' + style.display);
			if (style.display === 'flex') styles.push('FLEX');
			if (style.display === 'grid') styles.push('GRID');
		}
		if (style.position !== 'static') styles.push('position:' + style.position);
		if (style.position === 'absolute' || style.position === 'fixed') {
             styles.push('top:' + style.top);
             styles.push('left:' + style.left);
        }
		
		// Indicate interactive items
		const isClickable = window.getComputedStyle(el).cursor === 'pointer' || 
						   tag === 'button' || tag === 'a' || tag === 'input' || tag === 'select';
		if (isClickable) styles.push('clickable');

		if (styles.length > 0) result += ' [' + styles.join(' ') + ']';
		result += '>';
		
		if (directText) result += ' ' + directText;
		result += '\n';
		
		// Recurse for children
		Array.from(el.children).forEach(child => {
			result += getStructure(child, depth + 1);
		});
		
		return result;
	};
	return getStructure(document.body);
})()
`

func (b *DevBrowser) GetStructureTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "browser_get_content",
			Description: "Get a text-based representation of the page content, optimized for LLM reading. Reduced token count compared to screenshots.",
			Args: new(GetContentArgs),
			Resource:    "browser",
			Action:      'r',
			Execute: func(Ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
				if !b.IsOpenFlag {
					return nil, ErrBrowserNotOpen
				}

				var args GetContentArgs
				if err := req.Bind(&args); err != nil {
					return nil, err
				}

				var pageTitle, pageURL, structure string
				var windowWidth, windowHeight int

				err := chromedp.Run(b.toolCtx(),
					chromedp.Title(&pageTitle),
					chromedp.Location(&pageURL),
					chromedp.Evaluate(`window.innerWidth`, &windowWidth),
					chromedp.Evaluate(`window.innerHeight`, &windowHeight),
					chromedp.Evaluate(GetStructureJS, &structure),
				)

				if err != nil {
					return nil, fmt.Errorf("Failed to get page structure: %v", err)
				}

				report := fmt.Sprintf(
					"Url: %s\nTitle: %s\nViewport: %dx%d\n\n%s",
					pageURL,
					pageTitle,
					windowWidth, windowHeight,
					structure,
				)

				return mcp.Text(report), nil
			},
		},
	}
}
