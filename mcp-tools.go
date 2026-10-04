package devbrowser

import (
	"webtyp.com/fmt"
	"webtyp.com/mcp"
)

// ErrBrowserNotOpen es el error de precondición de todos los tools browser_*.
// El browser lo abre el daemon automáticamente al iniciar un proyecto.
var ErrBrowserNotOpen = fmt.Err(
	"browser is not open: start a project with the start_development tool (the browser opens automatically); if it was closed, call start_development again")

// GetMCPTools returns metadata for all DevBrowser MCP tools
func (b *DevBrowser) GetMCPTools() []mcp.Tool {
	tools := []mcp.Tool{}
	tools = append(tools, b.GetManagementTools()...)
	tools = append(tools, b.GetConsoleTools()...)
	tools = append(tools, b.GetScreenshotTools()...)
	tools = append(tools, b.GetStructureTools()...)
	tools = append(tools, b.GetEvaluateJsTools()...)
	tools = append(tools, b.GetNetworkTools()...)
	tools = append(tools, b.GetErrorTools()...)
	tools = append(tools, b.GetInteractionTools()...)
	tools = append(tools, b.GetNavigationTools()...)
	tools = append(tools, b.GetInspectTools()...)
	tools = append(tools, b.GetSelectedElementTools()...)
	tools = append(tools, b.GetPerformanceTools()...)
	tools = append(tools, b.GetAuditTools()...)
	tools = append(tools, b.GetSourceTools()...)
	tools = append(tools, b.GetStylesTools()...)
	tools = append(tools, b.GetStorageTools()...)
	tools = append(tools, b.GetAssetTools()...)
	tools = append(tools, b.GetInterceptTools()...)
	return tools
}
