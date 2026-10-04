package devbrowser_test

import (
	"context"
	"testing"
	"time"

	"webtyp.com/devbrowser/cdproto/target"
	"webtyp.com/devbrowser/chromedp"
)

func TestDiscoverDevToolsTarget(t *testing.T) {
	db, _ := NewTestBrowserWithContext(t)
	defer db.CloseBrowser()

	if err := chromedp.Run(db.Ctx,
		chromedp.Navigate("https://example.com"),
		chromedp.Sleep(500*time.Millisecond),
	); err != nil {
		t.Skip("skipping in headless/container environment without network")
	}

	var targets []*target.Info
	if err := chromedp.Run(db.Ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			var err error
			targets, err = target.GetTargets().Do(c)
			return err
		}),
	); err != nil {
		t.Fatalf("failed to get targets: %v", err)
	}

	t.Logf("Found %d targets:", len(targets))
	for i, tgt := range targets {
		t.Logf("[%d] ID=%s Type=%s Title=%q URL=%s", i, tgt.TargetID, tgt.Type, tgt.Title, tgt.URL)
	}
}
