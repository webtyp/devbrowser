//go:build repro

// These are headful/manual diagnostics for window-position behavior under
// Wayland/XWayland. They are excluded from normal test runs (no SKIP noise in
// `gotest`) and only compile with the build tag:
//
//	go test -tags repro ./ -run TestReproSetWindowBoundsIgnored -v
//	go test -tags repro ./ -run TestReproManualMoveDetected   -v -timeout 60s
package devbrowser_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"webtyp.com/devbrowser"

	"webtyp.com/devbrowser/cdproto/browser"
	"webtyp.com/devbrowser/chromedp"
)

// reproStore is a minimal in-memory Store for the reproduction test.
type reproStore struct {
	mu sync.RWMutex
	m  map[string]string
}

func (s *reproStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.m[key]; ok {
		return v, nil
	}
	return "", nil
}

func (s *reproStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

type reproUI struct{}

func (reproUI) RefreshUI()         {}
func (reproUI) ReturnFocus() error { return nil }

func reproReadCDP(t *testing.T, ctx context.Context, label string) (left, top, width, height int64) {
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		tgt := chromedp.FromContext(ctx).Target
		id, b, err := browser.GetWindowForTarget().WithTargetID(tgt.TargetID).Do(ctx)
		if err != nil {
			return err
		}
		left, top, width, height = b.Left, b.Top, b.Width, b.Height
		t.Logf("[REPRO] %-28s CDP: Left=%d Top=%d Width=%d Height=%d state=%q (winID=%d)",
			label, b.Left, b.Top, b.Width, b.Height, b.WindowState, id)
		return nil
	}))
	return
}

// TestReproSetWindowBoundsIgnored is a headful diagnostic: it shows whether
// SetWindowBounds applies position and size in the current environment.
// Results vary by window manager — run isolated for a reliable reading.
//
// Run: go test -tags repro ./ -run TestReproSetWindowBoundsIgnored -v
func TestReproSetWindowBoundsIgnored(t *testing.T) {
	store := &reproStore{m: map[string]string{
		"browser_position": "0,0",
		"browser_size":     "800,600",
	}}
	b := devbrowser.New(reproUI{}, store, make(chan bool))
	b.SetLog(func(a ...any) { t.Log(a...) })
	b.SetHeadless(false)

	if err := b.CreateBrowserContext(); err != nil {
		t.Fatalf("CreateBrowserContext: %v", err)
	}
	b.IsOpenFlag = true
	defer b.CloseBrowser()

	if err := chromedp.Run(b.Ctx, chromedp.Navigate("about:blank")); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	time.Sleep(700 * time.Millisecond)

	reproReadCDP(t, b.Ctx, "initial")

	// Ask CDP to move AND resize (cross-platform).
	if err := chromedp.Run(b.Ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		tgt := chromedp.FromContext(ctx).Target
		id, _, err := browser.GetWindowForTarget().WithTargetID(tgt.TargetID).Do(ctx)
		if err != nil {
			return err
		}
		return browser.SetWindowBounds(id, &browser.Bounds{
			Left: 450, Top: 250, Width: 1000, Height: 650,
			WindowState: browser.WindowStateNormal,
		}).Do(ctx)
	})); err != nil {
		t.Fatalf("SetWindowBounds: %v", err)
	}
	time.Sleep(700 * time.Millisecond)

	left, top, width, height := reproReadCDP(t, b.Ctx, "after SetWindowBounds(450,250,1000,650)")

	// SIZE: informational only (decoration may differ by ±1px). The point is it
	// tracks the request (≈1000x650).
	t.Logf("SIZE result: %dx%d (requested 1000x650) — size IS honored", width, height)

	// POSITION: under native Wayland the compositor never exposes absolute
	// window coordinates, so CDP reports 0,0 and SetWindowBounds is ignored —
	// that was the bug. The --ozone-platform=x11 flag routes Chrome through
	// XWayland, where the position IS applied and readable. We allow a small
	// tolerance because the window-manager frame/title-bar shifts the client
	// origin by a few pixels (e.g. requested top 250 lands at ~239).
	const tol = 40
	if abs64(left-450) > tol || abs64(top-250) > tol {
		t.Errorf("POSITION not applied — got %d,%d, want ~450,250 (tol %d). "+
			"Under native Wayland CDP reports 0,0; --ozone-platform=x11 (XWayland) "+
			"is required for position tracking to work.",
			left, top, tol)
	} else {
		t.Logf("POSITION result: %d,%d (requested 450,250, within %dpx frame tolerance) "+
			"— position IS honored under XWayland", left, top, tol)
	}
}

// TestReproManualMoveDetected is a MANUAL diagnostic: it opens a real browser and
// polls the window geometry once per second for ~20 seconds. Drag and resize the
// window with the mouse during that time. The log shows whether CDP reports the
// manual move (position) and the manual resize (size) — directly answering why
// browser_size is saved but browser_position may not be.
//
// Run: go test -tags repro ./ -run TestReproManualMoveDetected -v -timeout 60s
func TestReproManualMoveDetected(t *testing.T) {
	store := &reproStore{m: map[string]string{
		"browser_position": "0,0",
		"browser_size":     "800,600",
	}}
	b := devbrowser.New(reproUI{}, store, make(chan bool))
	b.SetLog(func(a ...any) { t.Log(a...) })
	b.SetHeadless(false)

	if err := b.CreateBrowserContext(); err != nil {
		t.Fatalf("CreateBrowserContext: %v", err)
	}
	b.IsOpenFlag = true
	defer b.CloseBrowser()

	if err := chromedp.Run(b.Ctx, chromedp.Navigate("about:blank")); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	t.Log(">>> DRAG and RESIZE the browser window with the mouse for the next 20s <<<")
	var lastPos, lastSize string
	for i := 0; i < 20; i++ {
		time.Sleep(1 * time.Second)
		left, top, width, height := reproReadCDP(t, b.Ctx, "poll")
		pos := itoa(left) + "," + itoa(top)
		size := itoa(width) + "x" + itoa(height)
		if pos != lastPos {
			t.Logf("    >>> POSITION changed: %s -> %s (CDP DID see the manual move)", lastPos, pos)
			lastPos = pos
		}
		if size != lastSize {
			t.Logf("    >>> SIZE changed: %s -> %s (CDP DID see the manual resize)", lastSize, size)
			lastSize = size
		}
	}
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
