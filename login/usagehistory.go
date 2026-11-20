package login

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"time"

	"github.com/ubgo/textdata/atomicfile"

	"github.com/ubgo/agx/config"
)

// The login-usage history (config.Paths.LoginUsageHistory): a short,
// bounded record of what the probes already returned, per login: when,
// with which model, and every window's percent and reset. It costs no
// API call (it is written with the cache, under the cache's lock) and
// feeds later calibration of the pool's assumptions (how fast a session
// burns a week, how many 5-hour windows a week holds), a conditional
// sticky leave and a pool simulation of the user's own pool. Like the
// cache it is rebuilt at any time, owner-only, and never holds a token.

// UsageHistoryVersion is the history file's format version; a file with
// another version is ignored and rewritten.
const UsageHistoryVersion = 1

// Bounds of the history: a point is kept only when HistoryMinGap has
// passed since the login's last point or a window's reset changed, and
// points older than HistoryMaxAge or beyond HistoryMaxPoints are
// dropped (8 days of a point every 10 minutes is 1152 points).
const (
	HistoryMinGap    = 10 * time.Minute
	HistoryMaxAge    = 8 * 24 * time.Hour
	HistoryMaxPoints = 1200
)

// UsageHistory is the history file.
type UsageHistory struct {
	Version int `json:"version"`
	// Logins maps a login name to its points, oldest first.
	Logins map[string][]UsagePoint `json:"logins"`
}

// UsagePoint is one probe's reading of one login.
type UsagePoint struct {
	At time.Time `json:"at"`
	// Model is the model the probe asked with ("" = the default probe).
	Model   string        `json:"model,omitempty"`
	Windows []PointWindow `json:"windows"`
}

// PointWindow is one window of a point.
type PointWindow struct {
	// ID is the vendor's window name ("5h", "7d", "7d_oi"), else the
	// class and scope ("week", "week/oi").
	ID       string     `json:"id"`
	Percent  float64    `json:"percent"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
}

// pointOf is entry e as a history point; ok is false for an entry
// without windows (a failure or an API key: nothing was read).
func pointOf(e UsageEntry) (UsagePoint, bool) {
	if len(e.Windows) == 0 {
		return UsagePoint{}, false
	}
	p := UsagePoint{At: e.ProbedAt, Model: e.Model, Windows: make([]PointWindow, 0, len(e.Windows))}
	for _, w := range e.Windows {
		id := w.ID
		if id == "" {
			id = w.Class
			if w.Scope != "" {
				id += "/" + w.Scope
			}
		}
		p.Windows = append(p.Windows, PointWindow{ID: id, Percent: w.Percent, ResetsAt: w.ResetsAt})
	}
	return p, true
}

// resetsChanged reports a window of p whose reset differs from the same
// window in last (a reset happened, or a new window appeared).
func resetsChanged(last, p UsagePoint) bool {
	for _, w := range p.Windows {
		i := slices.IndexFunc(last.Windows, func(o PointWindow) bool { return o.ID == w.ID })
		if i < 0 {
			return true
		}
		o := last.Windows[i]
		if (o.ResetsAt == nil) != (w.ResetsAt == nil) || (o.ResetsAt != nil && !o.ResetsAt.Equal(*w.ResetsAt)) {
			return true
		}
	}
	return false
}

// Add records p for name under the bounds and reports whether it was
// kept.
func (h *UsageHistory) Add(name string, p UsagePoint) bool {
	if h.Logins == nil {
		h.Logins = map[string][]UsagePoint{}
	}
	pts := h.Logins[name]
	if n := len(pts); n > 0 {
		last := pts[n-1]
		if !p.At.After(last.At) || (p.At.Sub(last.At) < HistoryMinGap && !resetsChanged(last, p)) {
			return false
		}
	}
	pts = append(pts, p)
	cut := 0
	for cut < len(pts) && p.At.Sub(pts[cut].At) > HistoryMaxAge {
		cut++
	}
	cut = max(cut, len(pts)-HistoryMaxPoints)
	h.Logins[name] = slices.Clone(pts[cut:])
	return true
}

// ReadUsageHistory reads the history. A missing file is an empty
// history; an unreadable or foreign one is an empty history plus the
// error (the history is never needed to pick).
func ReadUsageHistory(paths config.Paths) (UsageHistory, error) {
	empty := UsageHistory{Version: UsageHistoryVersion, Logins: map[string][]UsagePoint{}}
	b, err := os.ReadFile(paths.LoginUsageHistory())
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	var h UsageHistory
	if err := json.Unmarshal(b, &h); err != nil {
		return empty, fmt.Errorf("%s: %w", config.LoginUsageHistoryFileName, err)
	}
	if h.Version != UsageHistoryVersion {
		return empty, fmt.Errorf("%s: version %d, want %d; it is rebuilt", config.LoginUsageHistoryFileName, h.Version, UsageHistoryVersion)
	}
	if h.Logins == nil {
		h.Logins = map[string][]UsagePoint{}
	}
	return h, nil
}

// recordHistory adds the entries' readings to the history file; the
// caller holds the cache lock. Nothing is written when no point is kept.
func recordHistory(paths config.Paths, entries map[string]UsageEntry) error {
	h, _ := ReadUsageHistory(paths) // a broken history is replaced
	kept := false
	for name, e := range entries {
		if p, ok := pointOf(e); ok && h.Add(name, p) {
			kept = true
		}
	}
	if !kept {
		return nil
	}
	b, err := json.MarshalIndent(h, "", " ")
	if err != nil {
		return err
	}
	return atomicfile.Write(paths.LoginUsageHistory(), append(b, '\n'), atomicfile.Options{Mode: config.TokensFileMode})
}
