package devbrowser_test

import (
	"testing"
)

func TestGetMCPToolsMetadata_AllToolsRegistered(t *testing.T) {
	db, _ := DefaultTestBrowser()
	defer db.CloseBrowser()

	tools := db.GetMCPTools()

	expectedToolNames := []string{
		"browser_get_console",
		"browser_emulate_device",
		"browser_screenshot",
		"browser_get_content",
		"browser_click_element",
		"browser_fill_element",
		"browser_navigate",
		"browser_swipe_element",
		"browser_inspect_element",
		"browser_get_selected_element",
		"browser_get_performance",
		"browser_get_network_logs",
		"browser_evaluate_js",
		"browser_get_errors",
		"browser_get_source",
		"browser_get_styles",
		"browser_get_storage",
		"browser_get_asset",
		"browser_intercept_request",
		"browser_save_screenshot",
		"browser_audit_mobile",
	}

	if len(tools) != len(expectedToolNames) {
		t.Errorf("Expected %d tools, but got %d", len(expectedToolNames), len(tools))
	}

	registeredTools := make(map[string]bool)
	for _, tool := range tools {
		registeredTools[tool.Name] = true
	}

	for _, name := range expectedToolNames {
		if !registeredTools[name] {
			t.Errorf("Expected tool '%s' to be registered, but it was not", name)
		}
	}
}
