package devbrowser_test

import (
	"webtyp.com/devbrowser"
	"testing"
)

type mockUI struct {
	refreshed bool
}

func (m *mockUI) RefreshUI() {
	m.refreshed = true
}

func (m *mockUI) ReturnFocus() error {
	return nil
}

func TestTUI_HandlerExecution(t *testing.T) {
	b := &devbrowser.DevBrowser{
		IsOpenFlag: false,
		Log:        func(msg ...any) {},
		UI:         &mockUI{},
		DB:         &mockStore{data: make(map[string]string)},
	}

	// 1. Name
	if b.Name() != "BROWSER" {
		t.Errorf("expected Name 'BROWSER', got %q", b.Name())
	}

	// 2. Initial Label when closed
	if b.Label() != "Show Browser" {
		t.Errorf("expected Label 'Show Browser' when closed, got %q", b.Label())
	}

	// 3. Label when open
	b.IsOpenFlag = true
	if b.Label() != "Hide Browser" {
		t.Errorf("expected Label 'Hide Browser' when open, got %q", b.Label())
	}

	// 4. StatusMessage
	b.IsOpenFlag = false
	if msg := b.StatusMessage(); msg != "Closed | Shortcut B" {
		t.Errorf("expected 'Closed | Shortcut B', got %q", msg)
	}
	b.IsOpenFlag = true
	if msg := b.StatusMessage(); msg != "Open | Shortcut B" {
		t.Errorf("expected 'Open | Shortcut B', got %q", msg)
	}

	// 5. Shortcuts
	shortcuts := b.Shortcuts()
	if len(shortcuts) != 1 || shortcuts[0]["B"] != "toggle browser" {
		t.Errorf("expected shortcut [B: toggle browser], got %+v", shortcuts)
	}
}
