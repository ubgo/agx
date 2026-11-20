package pool_test

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// withScoped is read() plus one per-model window "fable week".
func withScoped(short, week float64, reset time.Duration, scoped float64, scopedReset time.Duration, rejected bool) pool.Reading {
	r := read(short, week, reset)
	r.Scoped = []pool.ScopedWindow{{Name: "fable week", Percent: scoped, ResetsAt: in(scopedReset), Rejected: rejected}}
	return r
}

// TestScopedWindowCaps: a per-model window is an extra weekly cap with
// the member's weekly cap and relax rules, measured to its own reset; it
// blocks at or over its cap, when the vendor refuses it until its reset,
// or when its use is unreadable; a passed reset neither blocks nor
// limits; the room is the least room over the windows.
func TestScopedWindowCaps(t *testing.T) {
	cases := []struct {
		name    string
		rd      pool.Reading
		usable  bool
		reason  string
		room    float64
		urgency float64
	}{
		{"under its cap: room is the least", withScoped(10, 20, 100*time.Hour, 60, 50*time.Hour, false), true, "", 10, 0.2},
		{"weekly window leaves less", withScoped(10, 65, 100*time.Hour, 20, 50*time.Hour, false), true, "", 5, 0.05},
		{"at its cap", withScoped(10, 20, 100*time.Hour, 70, 50*time.Hour, false), false, "fable week is 70% (cap 70%)", 0, -1},
		{"relaxed by its own reset", withScoped(10, 20, 100*time.Hour, 72, 12*time.Hour, false), true, "", 10.5, 0.875},
		{"refused until its reset", withScoped(10, 20, 100*time.Hour, 40, 30*time.Hour, true), false, "fable week is refused by the vendor until its reset (40%)", 30, -1},
		{"refused but reset passed", withScoped(10, 20, 100*time.Hour, 100, -time.Hour, true), true, "", 50, 0.5},
		{"unreadable", withScoped(10, 20, 100*time.Hour, math.NaN(), 30*time.Hour, false), false, "fable week usage is unreadable", 50, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := expiring()
			got, r := pick(t, cfg, map[string]pool.Reading{"derek": c.rd}, pool.Options{})
			d := candidateOf(t, r, "derek")
			if (got == "derek") != c.usable || (c.reason != "" && !slices.Contains(d.Reasons, c.reason)) {
				t.Fatalf("picked %q, reasons %v; want usable %v, reason %q", got, d.Reasons, c.usable, c.reason)
			}
			if d.Room == nil || math.Abs(*d.Room-c.room) > 0.05 {
				t.Errorf("room %v, want %v", d.Room, c.room)
			}
			if c.usable && (d.Urgency == nil || math.Abs(*d.Urgency-c.urgency) > 0.01) {
				t.Errorf("urgency %v, want %v", d.Urgency, c.urgency)
			}
			if len(d.Scoped) != 1 || d.Scoped[0].Name != "fable week" {
				t.Errorf("scoped %+v", d.Scoped)
			}
		})
	}
}

// TestScopedWindowReasons: Config.Reasons (a picker's or a force's
// warnings) names a scoped window over its cap too; a member without
// scoped windows is unchanged.
func TestScopedWindowReasons(t *testing.T) {
	cfg := expiring()
	got := cfg.Reasons("derek", withScoped(10, 20, 100*time.Hour, 80, 50*time.Hour, false), true)
	if !slices.Contains(got, "fable week is 80% (cap 70%)") {
		t.Errorf("reasons %v", got)
	}
	if got := cfg.Reasons("derek", read(10, 20, 100*time.Hour), true); len(got) != 0 {
		t.Errorf("no scoped window: %v", got)
	}
	// A scoped window full on one member moves the pick to another.
	readings := map[string]pool.Reading{
		"alice": withScoped(5, 10, 20*time.Hour, 99, 20*time.Hour, false),
		"bob":   read(5, 10, 100*time.Hour),
	}
	name, r := pick(t, cfg, readings, pool.Options{})
	if name != "bob" || !strings.Contains(strings.Join(candidateOf(t, r, "alice").Reasons, ";"), "fable week is 99% (cap 98%)") {
		t.Errorf("picked %s; alice %v", name, candidateOf(t, r, "alice").Reasons)
	}
}
