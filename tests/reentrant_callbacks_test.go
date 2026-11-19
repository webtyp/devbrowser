package devbrowser_test

import (
	"testing"
	"time"

	"webtyp.com/devbrowser"
)

// reentrantUI calls back into the browser on every refresh, as a real TUI does when it redraws the BROWSER line.
type reentrantUI struct{ b *devbrowser.DevBrowser }

func (u *reentrantUI) RefreshUI()         { _ = u.b.IsOpen(); _ = u.b.GetLastOperationID() }
func (u *reentrantUI) ReturnFocus() error { return nil }

// newReentrantBrowser wires a logger and a UI that re-enter the browser, the way devtui and the daemon's
// HeadlessTUI do: they read GetLastOperationID on every log line to update the BROWSER line in place.
func newReentrantBrowser() *devbrowser.DevBrowser {
	ui := &reentrantUI{}
	b := devbrowser.New(ui, &defaultStore{m: make(map[string]string)}, make(chan bool))
	ui.b = b
	b.SetLog(func(...any) { _ = b.GetLastOperationID(); _ = b.IsOpen() })
	b.SetHeadless(true)
	return b
}

// returnsWithin fails the test when f is still blocked after d: a self-deadlock never returns.
func returnsWithin(t *testing.T, name string, d time.Duration, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s never returned: it calls the logger or the UI while holding the browser lock", name)
	}
}

// The daemon relaunch hung for good: Shutdown -> CloseBrowser held the lock and logged, the TUI's logger asked
// GetLastOperationID for the same lock, the daemon never exited and stayed half shut down with no browser.
func TestCloseBrowser_ReturnsWhenTheLoggerReentersTheBrowser(t *testing.T) {
	b := newReentrantBrowser()
	b.IsOpenFlag = true // an open browser is what makes CloseBrowser log
	returnsWithin(t, "CloseBrowser", 2*time.Second, func() { _ = b.CloseBrowser() })
}

func TestStartWithDetectedSize_ReturnsWhenTheLoggerReentersTheBrowser(t *testing.T) {
	b := newReentrantBrowser()
	b.SizeConfigured = false
	b.MonitorWidth, b.MonitorHeight = 1920, 1080 // a detected monitor is what makes it log
	returnsWithin(t, "StartWithDetectedSize", 2*time.Second, b.StartWithDetectedSize)
}
