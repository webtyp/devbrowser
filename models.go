package devbrowser

import "webtyp.com/model"

// Whitelists explícitas (model ≥ v0.0.8: reemplazan el piso default de
// Text() — ver model/docs/ARCHITECTURE.md §8). El encoding de salida es
// responsabilidad de quien renderice estos valores en HTML.
var (
	// permittedSelector: selectores CSS (#btn, .card > a[href^='x'],
	// div:nth-child(2n+1), [data-x="y"]).
	permittedSelector = model.Permitted{Letters: true, Numbers: true, Spaces: true,
		Extra: []rune(`#.-_[]()>~+*:,='"^$|`)}
	// permittedURL: RFC 3986 (unreserved + reserved + %).
	permittedURL = model.Permitted{Letters: true, Numbers: true,
		Extra: []rune(`:/?#[]@!$&'()*+,;=-._~%`)}
	// permittedFree: JS a evaluar, valores arbitrarios a tipear en inputs,
	// filtros de red — todo ASCII imprimible + saltos de línea/tab.
	permittedFree = model.Permitted{Letters: true, Numbers: true, Spaces: true,
		Tilde: true, BreakLine: true, Tab: true,
		Extra: []rune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~")}
	// permittedPath: paths on the filesystem (alphanumeric, spaces, separators, dots, hyphens, underscores).
	permittedPath = model.Permitted{Letters: true, Numbers: true, Spaces: true,
		Extra: []rune(`/._-\`)}
)

var ScreenshotArgsModel = model.Definition{
	Name: "screenshot_args",
	Fields: model.Fields{
		{Name: "fullpage", Type: model.Bool()},
	},
}

var ClickElementArgsModel = model.Definition{
	Name: "click_element_args",
	Fields: model.Fields{
		{Name: "selector", Type: model.Text(), NotNull: true, Permitted: permittedSelector},
		{Name: "wait_after", Type: model.Int()},
		{Name: "timeout", Type: model.Int()},
	},
}

var NavigateArgsModel = model.Definition{
	Name: "navigate_args",
	Fields: model.Fields{
		{Name: "url", Type: model.Text(), NotNull: true, Permitted: permittedURL},
	},
}

var EmulateDeviceArgsModel = model.Definition{
	Name: "emulate_device_args",
	Fields: model.Fields{
		{Name: "mode", Type: model.Text()},
		{Name: "device", Type: model.Text(), Permitted: permittedSelector},
		{Name: "capture", Type: model.Bool()},
		{Name: "selector", Type: model.Text(), Permitted: permittedSelector},
	},
}

var AuditMobileArgsModel = model.Definition{
	Name: "audit_mobile_args",
	Fields: model.Fields{
		{Name: "selector", Type: model.Text(), Permitted: permittedSelector},
	},
}

var GetConsoleArgsModel = model.Definition{
	Name: "get_console_args",
	Fields: model.Fields{
		{Name: "lines", Type: model.Int()},
	},
}

var FillElementArgsModel = model.Definition{
	Name: "fill_element_args",
	Fields: model.Fields{
		{Name: "selector", Type: model.Text(), NotNull: true, Permitted: permittedSelector},
		{Name: "value", Type: model.Text(), NotNull: true, Permitted: permittedFree},
		{Name: "wait_after", Type: model.Int()},
		{Name: "timeout", Type: model.Int()},
	},
}

var SwipeElementArgsModel = model.Definition{
	Name: "swipe_element_args",
	Fields: model.Fields{
		{Name: "selector", Type: model.Text(), NotNull: true, Permitted: permittedSelector},
		{Name: "direction", Type: model.Text(), NotNull: true},
		{Name: "distance", Type: model.Int()},
	},
}

var EvaluateJSArgsModel = model.Definition{
	Name: "evaluate_js_args",
	Fields: model.Fields{
		{Name: "script", Type: model.Text(), NotNull: true, Permitted: permittedFree},
		{Name: "await_promise", Type: model.Bool()},
	},
}

var GetNetworkLogsArgsModel = model.Definition{
	Name: "get_network_logs_args",
	Fields: model.Fields{
		{Name: "filter", Type: model.Text(), Permitted: permittedFree},
		{Name: "limit", Type: model.Int()},
	},
}

var GetErrorsArgsModel = model.Definition{
	Name: "get_errors_args",
	Fields: model.Fields{
		{Name: "limit", Type: model.Int()},
	},
}

var GetPerformanceArgsModel = model.Definition{
	Name: "get_performance_args",
	Fields: model.Fields{},
}

var GetContentArgsModel = model.Definition{
	Name: "get_content_args",
	Fields: model.Fields{},
}

var GetSourceArgsModel = model.Definition{
	Name: "get_source_args",
	Fields: model.Fields{
		{Name: "selector", Type: model.Text(), Permitted: permittedSelector},
	},
}

var InspectElementArgsModel = model.Definition{
	Name: "inspect_element_args",
	Fields: model.Fields{
		{Name: "selector", Type: model.Text(), NotNull: true, Permitted: permittedSelector},
	},
}

var GetStylesArgsModel = model.Definition{
	Name: "get_styles_args",
	Fields: model.Fields{
		{Name: "selector", Type: model.Text(), Permitted: permittedSelector},
		{Name: "sheet", Type: model.Int()},
	},
}

var GetStorageArgsModel = model.Definition{
	Name: "get_storage_args",
	Fields: model.Fields{
		{Name: "type", Type: model.Text()},
	},
}

var GetAssetArgsModel = model.Definition{
	Name: "get_asset_args",
	Fields: model.Fields{
		{Name: "url", Type: model.Text(), NotNull: true, Permitted: permittedURL},
	},
}

var InterceptRequestArgsModel = model.Definition{
	Name: "intercept_request_args",
	Fields: model.Fields{
		{Name: "action", Type: model.Text(), NotNull: true},
		{Name: "filter", Type: model.Text(), Permitted: permittedFree},
		{Name: "limit", Type: model.Int()},
	},
}

var OpenBrowserArgsModel = model.Definition{
	Name: "open_browser_args",
	Fields: model.Fields{
		{Name: "port", Type: model.Text()},
		{Name: "https", Type: model.Bool()},
	},
}

var CloseBrowserArgsModel = model.Definition{
	Name: "close_browser_args",
	Fields: model.Fields{},
}

var SaveScreenshotArgsModel = model.Definition{
	Name: "save_screenshot_args",
	Fields: model.Fields{
		{Name: "dir", Type: model.Text(), NotNull: true, Permitted: permittedPath},
		{Name: "name", Type: model.Text(), NotNull: true, Permitted: permittedPath},
		{Name: "selector", Type: model.Text(), Permitted: permittedSelector},
		{Name: "fullpage", Type: model.Bool()},
		{Name: "overwrite", Type: model.Bool()},
	},
}

var GetSelectedElementArgsModel = model.Definition{
	Name: "get_selected_element_args",
	Fields: model.Fields{
		{Name: "count", Type: model.Int()},
		{Name: "clear", Type: model.Bool()},
	},
}

type InterceptedRequest struct {
	URL          string
	Method       string
	RequestBody  string
	ResponseBody string
	Status       int
}
