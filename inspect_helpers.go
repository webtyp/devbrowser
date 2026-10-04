package devbrowser

// ExtractElementDetailsFunctionJS is the shared JavaScript function used by both
// browser_inspect_element and browser_get_selected_element to extract comprehensive
// DOM, style, accessibility, and WebTyp-specific metadata from any Element.
const ExtractElementDetailsFunctionJS = `
function extractElementDetails(el) {
	if (!el || !(el instanceof Element)) return null;

	const rect = el.getBoundingClientRect();
	const style = window.getComputedStyle(el);

	// Box Model
	const boxModel = {
		width: Math.round(rect.width * 100) / 100,
		height: Math.round(rect.height * 100) / 100,
		padding: {
			top: parseFloat(style.paddingTop) || 0,
			right: parseFloat(style.paddingRight) || 0,
			bottom: parseFloat(style.paddingBottom) || 0,
			left: parseFloat(style.paddingLeft) || 0
		},
		margin: {
			top: parseFloat(style.marginTop) || 0,
			right: parseFloat(style.marginRight) || 0,
			bottom: parseFloat(style.marginBottom) || 0,
			left: parseFloat(style.marginLeft) || 0
		},
		border: {
			top: parseFloat(style.borderTopWidth) || 0,
			right: parseFloat(style.borderRightWidth) || 0,
			bottom: parseFloat(style.borderBottomWidth) || 0,
			left: parseFloat(style.borderLeftWidth) || 0
		}
	};

	// Position
	const position = {
		type: style.position,
		top: Math.round(rect.top * 100) / 100,
		left: Math.round(rect.left * 100) / 100,
		right: Math.round(rect.right * 100) / 100,
		bottom: Math.round(rect.bottom * 100) / 100,
		offsetTop: el.offsetTop,
		offsetLeft: el.offsetLeft,
		scrollTop: el.scrollTop,
		scrollLeft: el.scrollLeft
	};

	// Layout
	const layout = {
		display: style.display,
		flexDirection: style.flexDirection,
		justifyContent: style.justifyContent,
		alignItems: style.alignItems,
		gridTemplateColumns: style.gridTemplateColumns,
		gridTemplateRows: style.gridTemplateRows,
		gap: style.gap,
		overflow: style.overflow,
		zIndex: style.zIndex
	};

	// Typography
	const typography = {
		fontFamily: style.fontFamily,
		fontSize: style.fontSize,
		fontWeight: style.fontWeight,
		lineHeight: style.lineHeight,
		textAlign: style.textAlign,
		color: style.color
	};

	// Background
	const background = {
		color: style.backgroundColor,
		image: style.backgroundImage !== 'none' ? style.backgroundImage : null
	};

	// Accessibility
	const accessibility = {
		role: el.getAttribute('role'),
		ariaLabel: el.getAttribute('aria-label'),
		ariaDescribedBy: el.getAttribute('aria-describedby'),
		tabIndex: el.tabIndex,
		isKeyboardFocusable: el.tabIndex >= 0 || ['A', 'BUTTON', 'INPUT', 'SELECT', 'TEXTAREA'].includes(el.tagName)
	};

	// Attributes (WebTyp key, id, class, etc.)
	const attributes = {};
	if (el.attributes) {
		for (let i = 0; i < el.attributes.length; i++) {
			const attr = el.attributes[i];
			attributes[attr.name] = attr.value;
		}
	}

	// WebTyp Identifiers & Identity
	let classVal = null;
	if (typeof el.className === 'string') {
		classVal = el.className;
	} else if (typeof el.getAttribute === 'function') {
		classVal = el.getAttribute('class');
	}
	if (classVal) classVal = classVal.trim();

	let idVal = null;
	if (typeof el.id === 'string' && el.id !== '') {
		idVal = el.id;
	} else if (typeof el.getAttribute === 'function') {
		idVal = el.getAttribute('id');
	}

	const identity = {
		tagName: el.tagName.toLowerCase(),
		id: idVal || null,
		className: classVal || null,
		name: typeof el.getAttribute === 'function' ? el.getAttribute('name') : null,
		dataKey: typeof el.getAttribute === 'function' ? el.getAttribute('data-key') : null,
		dataComponent: typeof el.getAttribute === 'function' ? el.getAttribute('data-component') : null,
		text: (el.innerText || el.textContent || '').trim().slice(0, 300)
	};

	// Build breadcrumb path
	const breadcrumbs = [];
	let curr = el;
	while (curr && curr.nodeType === Node.ELEMENT_NODE && curr !== document.documentElement) {
		let desc = curr.tagName.toLowerCase();
		let currId = null;
		if (typeof curr.id === 'string' && curr.id !== '') {
			currId = curr.id;
		} else if (typeof curr.getAttribute === 'function') {
			currId = curr.getAttribute('id');
		}

		if (currId) {
			if (/^[0-9]/.test(currId)) {
				desc += '[id="' + currId + '"]';
			} else {
				desc += '#' + currId;
			}
		} else {
			let currClass = null;
			if (typeof curr.className === 'string') {
				currClass = curr.className;
			} else if (typeof curr.getAttribute === 'function') {
				currClass = curr.getAttribute('class');
			}
			if (currClass && currClass.trim()) {
				const primaryClass = currClass.trim().split(/\s+/)[0];
				if (primaryClass) desc += '.' + primaryClass;
			}
		}
		breadcrumbs.unshift(desc);
		curr = curr.parentElement;
	}

	// Viewport clip with contextual padding for human and AI inspection
	const padding = 80;
	const viewW = window.innerWidth || (document.documentElement ? document.documentElement.clientWidth : 1920);
	const viewH = window.innerHeight || (document.documentElement ? document.documentElement.clientHeight : 1080);

	const clipX = Math.max(0, rect.left - padding);
	const clipY = Math.max(0, rect.top - padding);
	const clipRight = Math.min(viewW, rect.right + padding);
	const clipBottom = Math.min(viewH, rect.bottom + padding);

	const viewportClip = {
		x: Math.round(clipX * 100) / 100,
		y: Math.round(clipY * 100) / 100,
		width: Math.round(Math.max(1, clipRight - clipX) * 100) / 100,
		height: Math.round(Math.max(1, clipBottom - clipY) * 100) / 100
	};

	// Outer HTML (truncated to safe size)
	let outerHTML = el.outerHTML || '';
	if (outerHTML.length > 2500) {
		outerHTML = outerHTML.slice(0, 2500) + '... <!-- truncated -->';
	}

	return {
		identity,
		attributes,
		breadcrumbs: breadcrumbs.join(' > '),
		outerHTML,
		boxModel,
		position,
		layout,
		typography,
		background,
		accessibility,
		viewportClip
	};
}
`
