package devbrowser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Root test: markProfileExitedCleanly is unexported; tests/ can only reach the public API.
func TestMarkProfileExitedCleanly(t *testing.T) {
	t.Run("Existing Preferences with Crashed", func(t *testing.T) {
		dir := t.TempDir()
		defaultDir := filepath.Join(dir, "Default")
		if err := os.Mkdir(defaultDir, 0700); err != nil {
			t.Fatal(err)
		}
		prefsPath := filepath.Join(defaultDir, "Preferences")

		initialPrefs := `{"profile":{"exit_type":"Crashed","exited_cleanly":false},"other":1}`
		if err := os.WriteFile(prefsPath, []byte(initialPrefs), 0600); err != nil {
			t.Fatal(err)
		}

		if err := markProfileExitedCleanly(dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(prefsPath)
		if err != nil {
			t.Fatal(err)
		}

		var prefs map[string]any
		if err := json.Unmarshal(data, &prefs); err != nil {
			t.Fatal(err)
		}

		profile := prefs["profile"].(map[string]any)
		if profile["exit_type"] != "Normal" {
			t.Errorf("expected exit_type Normal, got %v", profile["exit_type"])
		}
		if profile["exited_cleanly"] != true {
			t.Errorf("expected exited_cleanly true, got %v", profile["exited_cleanly"])
		}
		if prefs["other"] != float64(1) { // JSON unmarshals numbers to float64
			t.Errorf("expected other 1, got %v", prefs["other"])
		}
	})

	t.Run("Missing File", func(t *testing.T) {
		dir := t.TempDir()
		if err := markProfileExitedCleanly(dir); err != nil {
			t.Fatalf("expected nil for missing file, got %v", err)
		}
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		defaultDir := filepath.Join(dir, "Default")
		if err := os.Mkdir(defaultDir, 0700); err != nil {
			t.Fatal(err)
		}
		prefsPath := filepath.Join(defaultDir, "Preferences")

		invalidJSON := `{"profile": { "exit_type": `
		if err := os.WriteFile(prefsPath, []byte(invalidJSON), 0600); err != nil {
			t.Fatal(err)
		}

		err := markProfileExitedCleanly(dir)
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}

		// Ensure file remains unchanged
		data, err := os.ReadFile(prefsPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != invalidJSON {
			t.Errorf("expected file to remain unchanged, got %s", string(data))
		}
	})

	t.Run("Large integers survive the rewrite", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "Default"), 0700); err != nil {
			t.Fatal(err)
		}
		prefsPath := filepath.Join(dir, "Default", "Preferences")
		const big = "13370000000000000123" // > 2^53: a float64 round trip changes it
		if err := os.WriteFile(prefsPath, []byte(`{"profile":{"exit_type":"Crashed"},"ts":`+big+`}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := markProfileExitedCleanly(dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(prefsPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"ts":`+big) {
			t.Errorf("large integer was rewritten: %s", data)
		}
	})
}
