package devbrowser_test

import (
	"strings"
	"testing"
	"webtyp.com/devbrowser"
)

// TestErrBrowserNotOpenMessage guards that the precondition error of the
// browser_* tools never references a tool this package does not register,
// and always instructs the real flow (the daemon opens the browser via
// start_development).
func TestErrBrowserNotOpenMessage(t *testing.T) {
	msg := devbrowser.ErrBrowserNotOpen.Error()
	if strings.Contains(msg, "browser_open") {
		t.Fatalf("ErrBrowserNotOpen references nonexistent tool browser_open: %q", msg)
	}
	if !strings.Contains(msg, "start_development") {
		t.Fatalf("ErrBrowserNotOpen must instruct the real flow (start_development): %q", msg)
	}
}
