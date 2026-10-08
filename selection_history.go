package devbrowser

import (
	"time"

	"webtyp.com/devbrowser/cdproto/cdp"
)

// selectionHistoryCap is how many selections are kept; the oldest is dropped first.
const selectionHistoryCap = 10

// SelectionSource is the gesture that produced a selection.
type SelectionSource string

const (
	SelectionInspectPointer SelectionSource = "devtools inspect pointer"
	SelectionAltClick       SelectionSource = "alt+click"
	SelectionDevToolsPanel  SelectionSource = "devtools elements panel"
)

// selection is a snapshot taken when the element was selected. It never
// references the live node again (webtyp/dom replaces nodes on re-render).
type selection struct {
	seq        int // 1, 2, 3… since the page was loaded or cleared; equals the badge number on the page
	source     SelectionSource
	at         time.Time
	pageURL    string
	nodeID     cdp.BackendNodeID // only to drop an immediate duplicate
	report     string            // formatSelectedElementReport output, source locations included
	screenshot []byte            // PNG with the highlight; nil when the element was not visible
}

// selectionHistory is guarded by DevBrowser.Mu.
type selectionHistory struct {
	items   []selection // oldest first, len <= selectionHistoryCap
	nextSeq int
}

// push assigns the next seq and appends s. When s.nodeID equals the newest
// item's nodeID (and is not 0) nothing is added and ok is false.
func (h *selectionHistory) push(s selection) (added selection, ok bool) {
	if n := len(h.items); n > 0 && s.nodeID != 0 && h.items[n-1].nodeID == s.nodeID {
		return selection{}, false
	}
	h.nextSeq++
	s.seq = h.nextSeq
	h.items = append(h.items, s)
	if len(h.items) > selectionHistoryCap {
		h.items = append(h.items[:0], h.items[len(h.items)-selectionHistoryCap:]...)
	}
	return s, true
}

// latest returns up to n items, NEWEST FIRST.
func (h *selectionHistory) latest(n int) []selection {
	if n > len(h.items) {
		n = len(h.items)
	}
	out := make([]selection, 0, n)
	for i := len(h.items) - 1; i >= len(h.items)-n; i-- {
		out = append(out, h.items[i])
	}
	return out
}

// len returns how many items are held.
func (h *selectionHistory) len() int { return len(h.items) }

// clear empties the history, restarts numbering at 1 and returns how many items it
// held. It runs whenever the badges disappear — an explicit clear removes them, a
// reload or navigation discards the document that held them — because a number
// that no longer marks anything on the page only misleads.
func (h *selectionHistory) clear() int {
	n := len(h.items)
	h.items = nil
	h.nextSeq = 0
	return n
}
