package devbrowser_test

import (
	"errors"
	"sync"
	"testing"

	"webtyp.com/devbrowser"
	"webtyp.com/json"
	"webtyp.com/model"
)

// encodeArgs serializes an args struct to its wire JSON so tests can build
// mcp.CallToolParams.Arguments (a raw JSON string), without production code
// needing a JSON-schema-adjacent helper like the removed EncodeSchema.
func encodeArgs(f model.Encodable) string {
	var s string
	_ = json.Encode(f, &s)
	return s
}

// Utilities for tests: provide a single initializer that tests can call to
// create a trimmed-down devbrowser.DevBrowser suitable for unit tests. The function
// accepts variadic options so callers don't have to pass anything. If a
// logger function or a custom exit channel is provided it will be used.
//
// Usage examples:
//   db, exit := DefaultTestBrowser()
//   db, exit := DefaultTestBrowser(myLogger)
//   db, exit := DefaultTestBrowser(myLogger, myExitChan)

// Note: we provide simple local implementations of the required
// interfaces so tests don't need to construct or import fake types.

// defaultServerConfig implements serverConfig for tests.
type defaultServerConfig struct{}

func (defaultServerConfig) ServerPort() string { return "0" }

// defaultUI implements userInterface for tests.
type defaultUI struct{}

func (defaultUI) RefreshUI() {}

func (defaultUI) ReturnFocus() error { return nil }

// defaultStore implements store for tests (in-memory key-value)
type defaultStore struct {
	mu sync.RWMutex
	m  map[string]string
}

func (s *defaultStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.m == nil {
		return "", errors.New("key not found")
	}
	v, ok := s.m[key]
	if !ok {
		return "", errors.New("key not found")
	}
	return v, nil
}

func (s *defaultStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string]string)
	}
	s.m[key] = value
	return nil
}

// NewTestBrowser creates an isolated devbrowser.DevBrowser instance for tests,
// pre-configured with headless mode enabled, mock UI, in-memory store, and default logger.
//
// Accepted variadic options (in any order):
// - func(...any) : a logger function. If omitted a no-op logger is used.
// - chan bool    : a custom exit channel. If omitted a new channel is created.
// Any other option types are ignored.
func NewTestBrowser(opts ...any) (*devbrowser.DevBrowser, chan bool) {
	var logger func(message ...any)
	var exit chan bool

	for _, o := range opts {
		switch v := o.(type) {
		case func(...any):
			logger = v
		case chan bool:
			exit = v
		}
	}

	if logger == nil {
		logger = func(...any) {}
	}
	if exit == nil {
		exit = make(chan bool)
	}

	db := devbrowser.New(defaultUI{}, &defaultStore{m: make(map[string]string)}, exit)
	db.SetLog(logger)
	db.SetHeadless(true) // Test browsers run headless by default for CI/CD safety
	return db, exit
}

// NewTestBrowserWithContext creates a devbrowser.DevBrowser, initializes its
// headless browser context, and sets IsOpenFlag so tests can directly run actions on db.Ctx.
func NewTestBrowserWithContext(t testing.TB, opts ...any) (*devbrowser.DevBrowser, chan bool) {
	t.Helper()
	db, exit := NewTestBrowser(opts...)
	if err := db.CreateBrowserContext(); err != nil {
		t.Fatalf("failed to create test browser context: %v", err)
	}
	db.IsOpenFlag = true
	return db, exit
}

// DefaultTestBrowser is an alias for NewTestBrowser for backwards compatibility.
func DefaultTestBrowser(opts ...any) (*devbrowser.DevBrowser, chan bool) {
	return NewTestBrowser(opts...)
}
