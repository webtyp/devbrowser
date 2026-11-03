package devbrowser

import (
	"errors"
	"fmt"

	"webtyp.com/devbrowser/cdproto/audits"
	"webtyp.com/devbrowser/cdproto/log"
	"webtyp.com/devbrowser/cdproto/runtime"
	"webtyp.com/devbrowser/chromedp"
)

// initializeConsoleCapture sets up the console log capturing system using Chrome DevTools Protocol.
// This captures ALL console messages including those from page load, using runtime events.
func (b *DevBrowser) initializeConsoleCapture() error {
	if b.Ctx == nil {
		return errors.New("browser context not initialized")
	}

	// Initialize the console logs slice
	b.ConsoleLogs = []string{}

	// Listen for console API called events and console cleared events
	chromedp.ListenTarget(b.Ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			b.LogsMutex.Lock()
			defer b.LogsMutex.Unlock()

			// Format the arguments without prefix
			var message string
			for i, arg := range ev.Args {
				if i > 0 {
					message += " "
				}
				// Get the string value from the RemoteObject
				if arg.Value != nil {
					// Extract the raw value without JSON encoding
					val := string(arg.Value)
					// Remove surrounding quotes if it's a string value
					if len(val) > 2 && val[0] == '"' && val[len(val)-1] == '"' {
						val = val[1 : len(val)-1]
					}
					message += val
				} else if arg.Description != "" {
					message += arg.Description
				}
			}

			// Add to logs without type prefix to save tokens
			b.ConsoleLogs = append(b.ConsoleLogs, message)

		case *runtime.EventExceptionThrown:
			b.LogsMutex.Lock()
			defer b.LogsMutex.Unlock()

			// Capture uncaught exceptions
			msg := fmt.Sprintf("[Exception] %s", ev.ExceptionDetails.Text)
			if ev.ExceptionDetails.Exception != nil && ev.ExceptionDetails.Exception.Description != "" {
				msg += ": " + ev.ExceptionDetails.Exception.Description
			}
			b.ConsoleLogs = append(b.ConsoleLogs, msg)

		case *log.EventEntryAdded:
			b.LogsMutex.Lock()
			defer b.LogsMutex.Unlock()

			// Capture browser logs (network errors, security warnings, etc.)
			// Format: [Level] Source: Text
			msg := fmt.Sprintf("[%s] %s: %s", ev.Entry.Level, ev.Entry.Source, ev.Entry.Text)
			if ev.Entry.URL != "" {
				msg += fmt.Sprintf(" (%s)", ev.Entry.URL)
			}
			b.ConsoleLogs = append(b.ConsoleLogs, msg)

		case *audits.EventIssueAdded:
			b.LogsMutex.Lock()
			defer b.LogsMutex.Unlock()

			// Capture Audit Issues (Cookie warnings, Mixed Content, etc.)
			b.ConsoleLogs = append(b.ConsoleLogs, issueMessage(ev.Issue))

		case *runtime.EventExecutionContextsCleared:
			// Clear logs when execution contexts are cleared (page reload/navigation)
			b.LogsMutex.Lock()
			b.ConsoleLogs = []string{}
			b.LogsMutex.Unlock()
		}
	})

	// Enable console, log, and audits domains to start receiving events
	err := chromedp.Run(b.Ctx,
		runtime.Enable(),
		log.Enable(),
		audits.Enable(),
	)
	if err != nil {
		return errors.New("initializeConsoleCapture: " + err.Error())
	}

	return nil
}

// GetConsoleLogs returns captured console logs from the browser.
// Returns an error if the browser context is not initialized.
func (b *DevBrowser) GetConsoleLogs() ([]string, error) {
	if b.Ctx == nil {
		return nil, errors.New("browser context not initialized")
	}

	b.LogsMutex.Lock()
	defer b.LogsMutex.Unlock()

	// Return a copy of the logs to avoid race conditions
	logsCopy := make([]string, len(b.ConsoleLogs))
	copy(logsCopy, b.ConsoleLogs)

	return logsCopy, nil
}

// ClearConsoleLogs clears the captured console logs.
func (b *DevBrowser) ClearConsoleLogs() error {
	if b.Ctx == nil {
		return errors.New("browser context not initialized")
	}

	b.LogsMutex.Lock()
	defer b.LogsMutex.Unlock()

	b.ConsoleLogs = []string{}
	return nil
}

// issueMessage is one console line for an audit issue: its code and, for a
// generic issue (forms, ORB), what is wrong and on which attribute — the code
// alone ("GenericIssue") says nothing.
func issueMessage(issue *audits.InspectorIssue) string {
	msg := "[Issue] " + issue.Code.String()
	if d := issue.Details; d != nil && d.GenericIssueDetails != nil {
		g := d.GenericIssueDetails
		msg += " " + g.ErrorType.String()
		if g.ViolatingNodeAttribute != "" {
			msg += " (" + g.ViolatingNodeAttribute + ")"
		}
	}
	return msg
}
