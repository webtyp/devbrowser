package devbrowser_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
    "time"
    "net/url"

	"webtyp.com/devbrowser"
)

type dummyUI struct{}

func (d *dummyUI) RefreshUI() {}
func (d *dummyUI) ReturnFocus() error { return nil }

type dummyStore struct{}

func (d *dummyStore) Set(key string, value string) error { return nil }
func (d *dummyStore) Get(key string) (string, error)     { return "", nil }

func TestProfileExitedCleanlyIntegration(t *testing.T) {
	if os.Getenv("CHROME_EXECPATH") == "" && devbrowser.ResolveChromeExecPath() == "" {
		t.Skip("skipping test: chrome not available")
	}

	tempHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempHome)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer ts.Close()

	parsedURL, _ := url.Parse(ts.URL)
	port := parsedURL.Port()

	projectRoot := filepath.Join(tempHome, "myproject")

	exitChan := make(chan bool, 1)

	b := devbrowser.New(&dummyUI{}, &dummyStore{}, exitChan, devbrowser.WithProfile(projectRoot))
	b.SetHeadless(true)

	b.OpenBrowser(port, false)

    time.Sleep(1 * time.Second)

	if err := b.NavigateToURL(ts.URL); err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	if err := b.CloseBrowser(); err != nil {
		t.Fatalf("failed to close browser: %v", err)
	}

	profileDir, err := devbrowser.ProfileDir(projectRoot)
	if err != nil {
		t.Fatalf("failed to get profile dir: %v", err)
	}

	prefsPath := filepath.Join(profileDir, "Default", "Preferences")
	data, err := os.ReadFile(prefsPath)
	if err != nil {
		t.Fatalf("Preferences file missing after close: %v", err)
	}

	var prefs map[string]any
	if err := json.Unmarshal(data, &prefs); err != nil {
		t.Fatalf("failed to parse Preferences: %v", err)
	}

	profile, ok := prefs["profile"].(map[string]any)
	if !ok {
		t.Fatalf("profile object not found in Preferences")
	}

	if profile["exit_type"] != "Normal" {
		t.Errorf("expected exit_type Normal, got %v", profile["exit_type"])
	}
}
