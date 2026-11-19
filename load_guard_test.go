// Root-level test (justified): exercises the unexported loadGuard — it decides from CDP events and an injected clock,
// and no exported API exposes that decision without a real Chrome.
package devbrowser

import (
	"testing"
	"time"
)

// guardHarness drives a loadGuard with a fake clock and a timer that only fires when the test says so.
type guardHarness struct {
	now     time.Time
	reloads int
	fire    []func()
	delays  []time.Duration
	logs    []string
	g       *loadGuard
}

func newGuardHarness() *guardHarness {
	h := &guardHarness{now: time.Unix(1000, 0)}
	h.g = newLoadGuard(loadGuardDeps{
		Now:    func() time.Time { return h.now },
		After:  func(d time.Duration, f func()) { h.delays = append(h.delays, d); h.fire = append(h.fire, f) },
		Reload: func() { h.reloads++ },
		Log:    func(m ...any) { h.logs = append(h.logs, m[0].(string)) },
	})
	h.g.SetOrigin("https://localhost:8080")
	return h
}

// run fires the pending timer, as the clock reaching it would.
func (h *guardHarness) run() {
	f := h.fire[0]
	h.fire = h.fire[1:]
	f()
}

// The relaunch case: the page loads, then browser.wasm is cut with ERR_NETWORK_CHANGED. Nothing reloaded the page,
// so the developer saw a blank screen and had to reload by hand.
func TestLoadGuard_ReloadsWhenTheAppResourceIsCutByANetworkError(t *testing.T) {
	h := newGuardHarness()
	h.g.Failed("https://localhost:8080/browser.wasm", "net::ERR_NETWORK_CHANGED")
	if len(h.fire) != 1 {
		t.Fatalf("a cut app resource must schedule one reload, scheduled %d", len(h.fire))
	}
	if h.reloads != 0 {
		t.Fatal("the reload must wait for its delay, the server may still be restarting")
	}
	h.run()
	if h.reloads != 1 {
		t.Fatalf("reloads = %d, want 1", h.reloads)
	}
}

func TestLoadGuard_IgnoresWhatIsNotATransientFailureOfTheApp(t *testing.T) {
	cases := []struct{ name, url, err string }{
		{"third-party origin", "https://cdn.example.com/x.js", "net::ERR_NETWORK_CHANGED"},
		{"blocked by policy, a reload cannot fix it", "https://localhost:8080/x.js", "net::ERR_BLOCKED_BY_CSP"},
		{"cancelled by a navigation", "https://localhost:8080/x.js", "net::ERR_ABORTED"},
		{"another port, another project", "https://localhost:8081/browser.wasm", "net::ERR_NETWORK_CHANGED"},
	}
	for _, c := range cases {
		h := newGuardHarness()
		h.g.Failed(c.url, c.err)
		if len(h.fire) != 0 {
			t.Errorf("%s: scheduled a reload", c.name)
		}
	}
}

// Several resources fail in the same instant (the server went away): one reload, not one per resource.
func TestLoadGuard_OneReloadForAFailureBurst(t *testing.T) {
	h := newGuardHarness()
	for _, p := range []string{"/browser.wasm", "/style.css", "/sprite.svg"} {
		h.g.Failed("https://localhost:8080"+p, "net::ERR_CONNECTION_REFUSED")
	}
	if len(h.fire) != 1 {
		t.Fatalf("burst scheduled %d reloads, want 1", len(h.fire))
	}
}

// A server that never comes back must not turn into an endless reload loop: the delay grows, and after
// loadGuardMaxAttempts the guard stops and says so.
func TestLoadGuard_GivesUpAndSaysSo(t *testing.T) {
	h := newGuardHarness()
	for i := 0; i < loadGuardMaxAttempts; i++ {
		h.g.Failed("https://localhost:8080/browser.wasm", "net::ERR_CONNECTION_REFUSED")
		h.run()
		h.now = h.now.Add(time.Second)
	}
	h.g.Failed("https://localhost:8080/browser.wasm", "net::ERR_CONNECTION_REFUSED")
	if len(h.fire) != 0 {
		t.Fatal("scheduled a reload past the attempt limit")
	}
	if len(h.logs) == 0 || h.logs[len(h.logs)-1] != msgLoadGuardGaveUp {
		t.Errorf("the last log must be %q, got %v", msgLoadGuardGaveUp, h.logs)
	}
	for i := 1; i < len(h.delays); i++ {
		if h.delays[i] <= h.delays[i-1] {
			t.Errorf("delays must grow: %v", h.delays)
		}
	}
}

// After a quiet period the budget is full again: a failure an hour later is a new incident, not the 6th attempt.
func TestLoadGuard_BudgetRefillsAfterAQuietPeriod(t *testing.T) {
	h := newGuardHarness()
	for i := 0; i < loadGuardMaxAttempts; i++ {
		h.g.Failed("https://localhost:8080/browser.wasm", "net::ERR_CONNECTION_REFUSED")
		h.run()
		h.now = h.now.Add(time.Second)
	}
	h.now = h.now.Add(loadGuardQuiet)
	h.g.Failed("https://localhost:8080/browser.wasm", "net::ERR_NETWORK_CHANGED")
	if len(h.fire) != 1 {
		t.Fatal("a failure after the quiet period must reload again")
	}
}
