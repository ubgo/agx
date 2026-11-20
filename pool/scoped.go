package pool

import (
	"fmt"
	"math"
	"time"
)

// Per-model windows. A plan can limit one model on its own (Fable has a
// weekly window of its own on top of the all-models week; Opus has none
// today), and the vendor reports such a window only when it is asked
// with that model. The caller hands in the scoped windows of the models
// the session uses (Reading.Scoped); each is an extra weekly cap with the
// member's weekly cap and relax rules, measured to its own reset. A
// member is blocked when any of them is at or over its cap, or when the
// vendor refuses requests on it now (Rejected) and its reset has not
// passed; its room is the least room over the weekly window and these.

// ScopedWindow is one per-model window of a reading.
type ScopedWindow struct {
	// Name labels it in reasons and the report ("fable week").
	Name string
	// Percent is the account's use of the window.
	Percent float64
	// ResetsAt is nil when the window has no known reset.
	ResetsAt *time.Time
	// Rejected is true when the vendor refuses requests on this window
	// now (its status is rejected, or it is the window a refused answer
	// named).
	Rejected bool
}

// ScopedState is one per-model window in the report: its use, the cap it
// gets now and the room it leaves.
type ScopedState struct {
	Name     string     `json:"name"`
	Percent  *float64   `json:"percent"`
	Cap      float64    `json:"cap"`
	Room     float64    `json:"room"`
	ResetsAt *time.Time `json:"resets_at"`
	Rejected bool       `json:"rejected,omitempty"`
	// Stale is true when its reset has passed: it has reset since the
	// reading, so it neither blocks nor limits the room.
	Stale bool `json:"stale,omitempty"`

	until      *time.Duration
	unreadable bool
}

// scopedStates are member name's scoped windows under c at now: each
// window's cap is the member's configured weekly cap relaxed toward its
// relax max by the window's own reset. A zero now (a check without a
// clock) neither relaxes nor marks anything stale.
func (c Config) scopedStates(name string, rd Reading, now time.Time) []ScopedState {
	if len(rd.Scoped) == 0 {
		return nil
	}
	configured := c.CapsFor(name).Week
	relaxMax := c.RelaxMaxFor(name)
	out := make([]ScopedState, 0, len(rd.Scoped))
	for _, w := range rd.Scoped {
		s := ScopedState{Name: w.Name, Cap: configured, ResetsAt: w.ResetsAt, Rejected: w.Rejected}
		if w.ResetsAt != nil && !w.ResetsAt.IsZero() && !now.IsZero() {
			until := w.ResetsAt.Sub(now)
			s.until = &until
			s.Stale = until < 0
			s.Cap = RelaxedWeek(configured, relaxMax, c.WeekRelaxWindow, until)
		}
		if math.IsNaN(w.Percent) || math.IsInf(w.Percent, 0) {
			s.unreadable = true
		} else {
			s.Percent = ptr(round(w.Percent))
			s.Room = math.Max(0, s.Cap-w.Percent)
		}
		if s.Stale || s.unreadable {
			// A window that has reset since the reading, or whose use is
			// unknown, says nothing about the room now.
			s.until, s.Room = nil, math.Max(0, s.Cap)
		}
		out = append(out, s)
	}
	return out
}

// scopedReasons are the block reasons of scoped windows: unreadable,
// refused by the vendor until its reset, or at or over its cap.
func scopedReasons(states []ScopedState) []string {
	var out []string
	for _, s := range states {
		switch {
		case s.Stale:
		case s.unreadable:
			out = append(out, fmt.Sprintf(unreadableFmt, s.Name))
		case s.Rejected:
			out = append(out, fmt.Sprintf("%s is refused by the vendor until its reset (%s%%)", s.Name, num(*s.Percent)))
		case *s.Percent >= s.Cap:
			out = append(out, fmt.Sprintf("%s is %s%% (cap %s%%)", s.Name, num(*s.Percent), num(s.Cap)))
		}
	}
	return out
}
