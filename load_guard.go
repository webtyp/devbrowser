package devbrowser

import (
	"net/url"
	"strings"
	"sync"
	"time"

	"webtyp.com/devbrowser/cdproto/network"
	"webtyp.com/devbrowser/chromedp"
)

const (
	// loadGuardMaxAttempts is how many reloads one incident gets before the guard stops and says so.
	loadGuardMaxAttempts = 5
	// loadGuardFirstDelay is the wait before the first reload; each attempt doubles it. A restarting dev server
	// answers within a second or two, so the first tries are quick and the last ones patient.
	loadGuardFirstDelay = 300 * time.Millisecond
	// loadGuardQuiet is how long without a failure ends an incident and refills the attempts.
	loadGuardQuiet = 10 * time.Second

	msgLoadGuardReload = "Reload: the page lost a resource to a network error, retrying"
	msgLoadGuardGaveUp = "Reload stopped: the page still fails to load, check the SERVER section"
)

// transientLoadErrors are the transport failures a restarting dev server causes and a reload fixes. A policy block
// (ERR_BLOCKED_BY_CSP) or a navigation's own cancel (ERR_ABORTED) is not in the list: reloading would only repeat it.
var transientLoadErrors = []string{
	"net::ERR_NETWORK_CHANGED",
	"net::ERR_CONNECTION_REFUSED",
	"net::ERR_CONNECTION_RESET",
	"net::ERR_CONNECTION_CLOSED",
	"net::ERR_CONNECTION_ABORTED",
	"net::ERR_EMPTY_RESPONSE",
	"net::ERR_INCOMPLETE_CHUNKED_ENCODING",
	"net::ERR_HTTP2_PROTOCOL_ERROR",
	"net::ERR_TIMED_OUT",
}

type loadGuardDeps struct {
	Now    func() time.Time
	After  func(time.Duration, func())
	Reload func()
	Log    func(...any)
}

// loadGuard reloads the development page when one of the app's own resources is cut by a transport error.
// Opening the browser is fire-and-forget and the server's readiness is a port probe, so a server that restarts
// while the page loads (a daemon relaunch, a rebuild) left a blank page that only a manual reload fixed.
type loadGuard struct {
	deps loadGuardDeps

	mu       sync.Mutex
	origin   string
	attempts int
	last     time.Time
	pending  bool
}

func newLoadGuard(deps loadGuardDeps) *loadGuard { return &loadGuard{deps: deps} }

// SetOrigin names the dev server the guard protects, from the URL the browser was sent to.
func (g *loadGuard) SetOrigin(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return
	}
	g.mu.Lock()
	g.origin = u.Scheme + "://" + u.Host
	g.mu.Unlock()
}

// Failed receives every failed request; only a transient failure of the app's own origin schedules a reload.
func (g *loadGuard) Failed(rawURL, errText string) {
	if !transient(errText) {
		return
	}
	g.mu.Lock()
	if g.origin == "" || !strings.HasPrefix(rawURL, g.origin+"/") {
		g.mu.Unlock()
		return
	}
	if g.pending { // a burst: the reload already scheduled covers it
		g.mu.Unlock()
		return
	}
	now := g.deps.Now()
	if now.Sub(g.last) > loadGuardQuiet {
		g.attempts = 0
	}
	g.last = now
	if g.attempts >= loadGuardMaxAttempts {
		g.mu.Unlock()
		g.deps.Log(msgLoadGuardGaveUp)
		return
	}
	delay := loadGuardFirstDelay << g.attempts
	g.attempts++
	g.pending = true
	g.mu.Unlock()

	g.deps.Log(msgLoadGuardReload)
	g.deps.After(delay, func() {
		g.mu.Lock()
		g.pending = false
		g.mu.Unlock()
		g.deps.Reload()
	})
}

func transient(errText string) bool {
	for _, e := range transientLoadErrors {
		if errText == e {
			return true
		}
	}
	return false
}

// installLoadGuard listens to the page's failed requests for the life of the browser context.
func (b *DevBrowser) installLoadGuard() {
	g := newLoadGuard(loadGuardDeps{
		Now:    time.Now,
		After:  func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		Reload: func() { _ = b.Reload() },
		Log:    func(m ...any) { b.Logger(m...) },
	})
	b.guard = g
	urls := make(map[network.RequestID]string)
	var mu sync.Mutex
	chromedp.ListenTarget(b.Ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			mu.Lock()
			urls[ev.RequestID] = ev.Request.URL
			mu.Unlock()
		case *network.EventLoadingFinished:
			mu.Lock()
			delete(urls, ev.RequestID)
			mu.Unlock()
		case *network.EventLoadingFailed:
			mu.Lock()
			u := urls[ev.RequestID]
			delete(urls, ev.RequestID)
			mu.Unlock()
			g.Failed(u, ev.ErrorText)
		}
	})
}
