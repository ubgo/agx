// Package pool picks one account out of a pool of logins by the rules of
// a "login pool" (agx login kind pool): waste as little plan quota as
// possible. Weekly quota that is not used before the weekly reset is
// lost, so the account whose week ends soonest goes first; your own
// accounts are used up almost fully, teammates' shared accounts only up
// to their caps, and a shared account goes before a usable own account
// only when its week ends within the borrow window.
//
// Select is a pure function of a Config, one Reading per account and the
// time: no I/O, no clock, no agent knowledge, so every rule is
// table-tested. agx reads usage (a token probe per member, cached) and
// hands the readings in.
//
// The rules, in order:
//
//  1. force (Options.Force, else Config.Force) wins outright; every rule
//     is ignored and each rule it would have broken is a warning.
//  2. An account is blocked when it is disabled, has no reading, its
//     status is not "ok", its short window is at or over
//     ShortUnavailableAt, or any window is at or over its cap (caps are
//     exclusive: usage < cap is usable). Every reason is listed.
//  3. While any own account is usable, the pool is the usable own
//     accounts plus the usable shared accounts whose weekly reset is
//     within BorrowWindow; other usable shared accounts wait. With no own
//     account usable, the pool is every usable shared account.
//  4. The pool is sorted by weekly reset, soonest first; an unknown or
//     already-passed reset sorts last (a passed reset means stale data);
//     ties keep config order, own before shared.
//  5. The first in the pool is selected; an empty pool selects nothing.
package pool

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

// Defaults of a pool that does not set a value.
const (
	// DefaultShortCap is the short (5-hour) window cap of an account
	// without its own entry, in percent.
	DefaultShortCap = 50.0
	// DefaultWeekCap is the weekly window cap of an account without its
	// own entry, in percent.
	DefaultWeekCap = 70.0
	// DefaultShortUnavailableAt is the short-window safety cutoff for
	// every account: at or over it the account is skipped even when its
	// status is ok, because the next request would be refused.
	DefaultShortUnavailableAt = 99.0
	// DefaultBorrowWindow is how close a shared account's weekly reset
	// must be for it to go before a usable own account.
	DefaultBorrowWindow = 48 * time.Hour
)

// The range of every cap and of the safety cutoff, in percent of a
// window. config publishes it in the editor schema, and agx config set
// and the pool flags name it in their messages, so the bound lives here
// once.
const (
	MinPercent = 0.0
	MaxPercent = 100.0
)

// StatusOK is the only reading status that leaves an account usable;
// any other status ("limit-reached", "rejected", "network") blocks it.
const StatusOK = "ok"

// Group says whose account a candidate is.
type Group string

// Groups.
const (
	// GroupOwn is one of your accounts: used up to its (high) caps.
	GroupOwn Group = "own"
	// GroupShared is a teammate's account: borrowed up to its caps.
	GroupShared Group = "shared"
	// GroupUnlisted is a forced account listed in neither group.
	GroupUnlisted Group = "unlisted"
)

// State is a candidate's place in the report.
type State string

// States.
const (
	// StateReady is in the pool, in pick order (Rank 1 is selected).
	StateReady State = "ready"
	// StateWaiting is usable but held back by the borrow window.
	StateWaiting State = "waiting"
	// StateBlocked is not usable; Reasons says why.
	StateBlocked State = "blocked"
)

// ForcedBy says where a force came from.
type ForcedBy string

// Force sources.
const (
	// ForcedByFlag is a one-run force (agx pool pick --pool-force).
	ForcedByFlag ForcedBy = "--pool-force"
	// ForcedByConfig is the pool's "force" key.
	ForcedByConfig ForcedBy = "config"
)

// ErrForcedMissing is a forced account that has no reading: there is
// nothing to hand out, so it is an error rather than a pick.
var ErrForcedMissing = errors.New("forced account has no usage reading")

// ErrBadNow is a selection without a clock: every reset would be
// measured from year 1, a silently wrong order.
var ErrBadNow = errors.New("pool: Options.Now is not set")

// Caps are an account's window caps in percent (exclusive).
type Caps struct {
	Short float64 `json:"short"`
	Week  float64 `json:"week"`
}

// Account is one account's own entry: caps that override the defaults,
// and Enabled false to leave it out. A nil field keeps the default.
type Account struct {
	Short   *float64
	Week    *float64
	Enabled *bool
}

// Config is a pool: who is in it and the rules' numbers.
type Config struct {
	// Own and Shared are account names in config order; a name appears
	// once across both.
	Own    []string
	Shared []string
	// Force is an account to always use; "" = off.
	Force string
	// BorrowWindow: a shared account goes before a usable own account
	// only when its weekly reset is within it; 0 = never.
	BorrowWindow time.Duration
	// ShortUnavailableAt is the short-window safety cutoff (percent).
	ShortUnavailableAt float64
	// Defaults are the caps of accounts without their own entry.
	Defaults Caps
	// Accounts are per-account entries keyed by name.
	Accounts map[string]Account
}

// New returns a Config with the documented defaults, for callers that
// fill only the lists.
func New(own, shared []string) Config {
	return Config{
		Own: own, Shared: shared, BorrowWindow: DefaultBorrowWindow,
		ShortUnavailableAt: DefaultShortUnavailableAt,
		Defaults:           Caps{Short: DefaultShortCap, Week: DefaultWeekCap},
	}
}

// Validate reports the first shape problem: an empty or repeated name, a
// negative window, a cap or cutoff outside 0..100, an account entry for
// a name in neither list.
func (c Config) Validate() error {
	seen := map[string]bool{}
	for _, name := range slices.Concat(c.Own, c.Shared) {
		if name == "" {
			return errors.New("pool: an account name is empty")
		}
		if seen[name] {
			return fmt.Errorf("pool: account %q is listed more than once in own/shared", name)
		}
		seen[name] = true
	}
	if c.BorrowWindow < 0 {
		return errors.New("pool: the borrow window is negative")
	}
	for _, p := range []struct {
		label string
		v     float64
	}{{"short_unavailable_at_percent", c.ShortUnavailableAt}, {"defaults.short", c.Defaults.Short}, {"defaults.week", c.Defaults.Week}} {
		if !validPercent(p.v) {
			return fmt.Errorf("pool: %s %v is outside 0..100", p.label, p.v)
		}
	}
	names := make([]string, 0, len(c.Accounts))
	for name := range c.Accounts {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		a := c.Accounts[name]
		if !seen[name] {
			return fmt.Errorf("pool: accounts.%s is not listed in own or shared", name)
		}
		for _, p := range []struct {
			label string
			v     *float64
		}{{"short", a.Short}, {"week", a.Week}} {
			if p.v != nil && !validPercent(*p.v) {
				return fmt.Errorf("pool: accounts.%s.%s %v is outside 0..100", name, p.label, *p.v)
			}
		}
	}
	return nil
}

// validPercent is a finite value in 0..100.
func validPercent(v float64) bool { return v >= MinPercent && v <= MaxPercent && !math.IsNaN(v) }

// CapsFor is one account's caps: the defaults, then its own entry.
func (c Config) CapsFor(name string) Caps {
	caps := c.Defaults
	if a, ok := c.Accounts[name]; ok {
		if a.Short != nil {
			caps.Short = *a.Short
		}
		if a.Week != nil {
			caps.Week = *a.Week
		}
	}
	return caps
}

// Enabled reports whether an account takes part (default true).
func (c Config) Enabled(name string) bool {
	a, ok := c.Accounts[name]
	return !ok || a.Enabled == nil || *a.Enabled
}

// Window is one usage window of a reading.
type Window struct {
	// Percent is the account's total use of the window.
	Percent float64
	// ResetsAt is nil when the window has no scheduled reset.
	ResetsAt *time.Time
}

// Reading is one account's usage. A window the source does not report is
// nil and is not checked (an account without a short window is caught
// only by its status).
type Reading struct {
	// Status is StatusOK or the reason it is not usable.
	Status string
	Short  *Window
	Week   *Window
}

// Options are one selection's inputs besides the config.
type Options struct {
	// Now is the time to measure resets from.
	Now time.Time
	// Force overrides Config.Force for this selection; "" = none.
	Force string
}

// Candidate is one listed account in the report.
type Candidate struct {
	Name  string `json:"name"`
	Group Group  `json:"group"`
	State State  `json:"state"`
	// Rank is the 1-based pick order in the pool; 0 when not ready.
	Rank     int  `json:"rank,omitempty"`
	Eligible bool `json:"eligible"`
	Enabled  bool `json:"enabled"`
	// Status is the reading's status, or "missing" without one.
	Status string   `json:"status"`
	Short  *float64 `json:"short"`
	Week   *float64 `json:"week"`
	// WeeklyResetsAt is the weekly window's reset; nil when unknown.
	WeeklyResetsAt *time.Time `json:"weekly_resets_at"`
	// UntilReset is the time from Now to WeeklyResetsAt (negative when
	// it has passed); nil when unknown.
	UntilReset *time.Duration `json:"-"`
	// Room is the weekly cap minus weekly use, never below 0; nil when
	// the weekly window is unknown.
	Room    *float64 `json:"room"`
	Caps    Caps     `json:"caps"`
	Reasons []string `json:"reasons"`
	Notes   []string `json:"notes"`

	order        int
	inWindow     bool
	hasWeekReset bool
}

// Selection is the account to use and why.
type Selection struct {
	Name   string `json:"name"`
	Group  Group  `json:"group"`
	Forced bool   `json:"forced"`
	// ForcedBy is set when Forced.
	ForcedBy ForcedBy `json:"forced_by,omitempty"`
	// Reason is one sentence: the rule that chose it.
	Reason string `json:"reason"`
	// Warnings are the rules a forced account breaks.
	Warnings       []string   `json:"warnings"`
	WeeklyResetsAt *time.Time `json:"weekly_resets_at"`
	Room           *float64   `json:"room"`
}

// Result is a whole selection: the pick (nil when nothing is usable) and
// every listed account in report order (ready by rank, then waiting,
// then blocked).
type Result struct {
	Selected           *Selection    `json:"selected"`
	Now                time.Time     `json:"now"`
	ShortUnavailableAt float64       `json:"short_unavailable_at_percent"`
	BorrowWindow       time.Duration `json:"-"`
	Candidates         []Candidate   `json:"candidates"`
}

// Reasons and notes, as the report prints them.
const (
	reasonDisabled  = "disabled in config"
	reasonNoReading = "no usage reading"
	unreadableFmt   = "%s usage is unreadable"
	noteBadReset    = "weekly reset time is unreadable; treated as unknown"
	reasonUnlisted  = "not listed in own or shared"
	noteStaleReset  = "weekly reset time has passed; usage data is stale"
	statusMissing   = "missing"
	shortLabel      = "5h"
	weekLabel       = "week"
)

// Select applies the rules (package comment) and returns the report. The
// errors are ErrForcedMissing (a forced account without a reading) and
// ErrBadNow (no clock). Select does not validate cfg; call Validate.
func Select(cfg Config, readings map[string]Reading, o Options) (Result, error) {
	if o.Now.IsZero() {
		return Result{}, ErrBadNow
	}
	res := Result{Now: o.Now, ShortUnavailableAt: cfg.ShortUnavailableAt, BorrowWindow: cfg.BorrowWindow}
	cands := make([]*Candidate, 0, len(cfg.Own)+len(cfg.Shared))
	add := func(names []string, g Group) {
		for _, name := range names {
			rd, found := readings[name]
			cands = append(cands, candidate(cfg, name, g, len(cands), rd, found, o.Now))
		}
	}
	add(cfg.Own, GroupOwn)
	add(cfg.Shared, GroupShared)

	ownUsable := slices.ContainsFunc(cands, func(c *Candidate) bool { return c.Eligible && c.Group == GroupOwn })
	var ready, waiting, blocked []*Candidate
	for _, c := range cands {
		switch {
		case !c.Eligible:
			blocked = append(blocked, c)
		case c.Group == GroupOwn || !ownUsable || c.inWindow:
			ready = append(ready, c)
		default:
			waiting = append(waiting, c)
		}
	}
	slices.SortStableFunc(ready, byDeadline)
	slices.SortStableFunc(waiting, byDeadline)
	for i, c := range ready {
		c.State, c.Rank = StateReady, i+1
	}
	waitNote := fmt.Sprintf("shared reset not within %s while an own account is usable", Hours(cfg.BorrowWindow))
	for _, c := range waiting {
		c.State = StateWaiting
		c.Notes = append(c.Notes, waitNote)
	}
	for _, c := range blocked {
		c.State = StateBlocked
	}
	for _, list := range [][]*Candidate{ready, waiting, blocked} {
		for _, c := range list {
			res.Candidates = append(res.Candidates, *c)
		}
	}

	force, by := o.Force, ForcedByFlag
	if force == "" {
		force, by = cfg.Force, ForcedByConfig
	}
	switch {
	case force != "":
		sel, err := forced(cfg, readings, cands, force, by)
		if err != nil {
			return res, err
		}
		res.Selected = &sel
	case len(ready) > 0:
		pick := ready[0]
		res.Selected = &Selection{
			Name: pick.Name, Group: pick.Group, Reason: pickReason(pick.Group, ownUsable, cfg.BorrowWindow),
			Warnings: []string{}, WeeklyResetsAt: pick.WeeklyResetsAt, Room: pick.Room,
		}
	}
	return res, nil
}

// candidate builds one listed account from its reading.
func candidate(cfg Config, name string, g Group, order int, rd Reading, found bool, now time.Time) *Candidate {
	c := &Candidate{
		Name: name, Group: g, order: order, Enabled: cfg.Enabled(name), Caps: cfg.CapsFor(name),
		Status: statusMissing, Reasons: []string{}, Notes: []string{},
	}
	c.Reasons = blockReasons(rd, found, c.Enabled, c.Caps, cfg.ShortUnavailableAt)
	c.Eligible = len(c.Reasons) == 0
	if !found {
		return c
	}
	c.Status = rd.Status
	if readable(rd.Short) {
		c.Short = ptr(round(rd.Short.Percent))
	}
	if rd.Week == nil {
		return c
	}
	if readable(rd.Week) {
		c.Week = ptr(round(rd.Week.Percent))
		c.Room = ptr(round(math.Max(0, c.Caps.Week-rd.Week.Percent)))
	}
	if rd.Week.ResetsAt != nil && rd.Week.ResetsAt.IsZero() {
		// A reset that could not be read carries no deadline.
		c.Notes = append(c.Notes, noteBadReset)
	} else if rd.Week.ResetsAt != nil {
		at := *rd.Week.ResetsAt
		until := at.Sub(now)
		c.WeeklyResetsAt, c.UntilReset, c.hasWeekReset = &at, &until, true
		if until < 0 {
			c.Notes = append(c.Notes, noteStaleReset)
		}
		c.inWindow = g == GroupShared && until >= 0 && until <= cfg.BorrowWindow
	}
	return c
}

// Reasons is every reason account name cannot be used now under cfg,
// given its reading (found false = none); empty = usable. A caller that
// lets something other than the rules choose (a picker command) reports
// these as warnings.
func (c Config) Reasons(name string, rd Reading, found bool) []string {
	return blockReasons(rd, found, c.Enabled(name), c.CapsFor(name), c.ShortUnavailableAt)
}

// blockReasons is every reason an account cannot be used now; empty =
// usable. Every reason is listed, a disabled account without a reading
// included ("disabled in config", "no usage reading").
func blockReasons(rd Reading, found, enabled bool, caps Caps, shortUnavailableAt float64) []string {
	reasons := []string{}
	if !enabled {
		reasons = append(reasons, reasonDisabled)
	}
	if !found {
		return append(reasons, reasonNoReading)
	}
	if rd.Status != StatusOK {
		reasons = append(reasons, "status is "+rd.Status)
	}
	// A reported window whose usage is not a finite number cannot be
	// checked against its cap, so it blocks the account instead of
	// passing as if it were fine.
	for _, w := range []struct {
		label string
		win   *Window
	}{{shortLabel, rd.Short}, {weekLabel, rd.Week}} {
		if w.win != nil && !readable(w.win) {
			reasons = append(reasons, fmt.Sprintf(unreadableFmt, w.label))
		}
	}
	if readable(rd.Short) && rd.Short.Percent >= shortUnavailableAt {
		reasons = append(reasons, fmt.Sprintf("%s is %s%% (unavailable at %s%%)", shortLabel, num(rd.Short.Percent), num(shortUnavailableAt)))
	}
	if readable(rd.Short) && rd.Short.Percent >= caps.Short {
		reasons = append(reasons, fmt.Sprintf("%s is %s%% (cap %s%%)", shortLabel, num(rd.Short.Percent), num(caps.Short)))
	}
	if readable(rd.Week) && rd.Week.Percent >= caps.Week {
		reasons = append(reasons, fmt.Sprintf("%s is %s%% (cap %s%%)", weekLabel, num(rd.Week.Percent), num(caps.Week)))
	}
	return reasons
}

// readable reports a window whose usage is a finite number.
func readable(w *Window) bool {
	return w != nil && !math.IsNaN(w.Percent) && !math.IsInf(w.Percent, 0)
}

// forced builds the selection of a forced account: listed accounts carry
// their block reasons as warnings, an unlisted one is checked against
// the default caps and says it is unlisted.
func forced(cfg Config, readings map[string]Reading, cands []*Candidate, name string, by ForcedBy) (Selection, error) {
	rd, found := readings[name]
	if !found {
		return Selection{}, fmt.Errorf("%w: %s", ErrForcedMissing, name)
	}
	sel := Selection{Name: name, Group: GroupUnlisted, Forced: true, ForcedBy: by}
	if i := slices.IndexFunc(cands, func(c *Candidate) bool { return c.Name == name }); i >= 0 {
		c := cands[i]
		sel.Group, sel.Warnings, sel.WeeklyResetsAt, sel.Room = c.Group, slices.Clone(c.Reasons), c.WeeklyResetsAt, c.Room
	} else {
		sel.Warnings = append([]string{reasonUnlisted}, blockReasons(rd, true, true, cfg.CapsFor(name), cfg.ShortUnavailableAt)...)
		if rd.Week != nil {
			sel.WeeklyResetsAt = rd.Week.ResetsAt
		}
	}
	src := string(ForcedByFlag)
	if by == ForcedByConfig {
		src = `config "force"`
	}
	sel.Reason = "forced by " + src + "; all rules ignored"
	return sel, nil
}

// pickReason is the rule that chose the first account of the pool.
func pickReason(g Group, ownUsable bool, window time.Duration) string {
	switch {
	case g == GroupOwn:
		return "soonest weekly reset among usable accounts"
	case ownUsable:
		return fmt.Sprintf("shared account resets within %s, before any usable own account; using its room before it is lost", Hours(window))
	}
	return "no own account is usable; soonest weekly reset among shared accounts"
}

// byDeadline orders by weekly reset, soonest first; an unknown or passed
// reset (stale data carries no deadline) sorts last; ties keep config
// order, which lists own before shared.
func byDeadline(a, b *Candidate) int {
	da, db := deadline(a), deadline(b)
	switch {
	case da < db:
		return -1
	case da > db:
		return 1
	}
	return a.order - b.order
}

// deadline is the time to the weekly reset, or the largest duration when
// it is unknown or has passed.
func deadline(c *Candidate) time.Duration {
	if !c.hasWeekReset || *c.UntilReset < 0 {
		return time.Duration(math.MaxInt64)
	}
	return *c.UntilReset
}

// Hours renders a window as whole hours ("48h"), or Go's duration form
// when it is not a whole number of hours ("90m0s").
func Hours(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	return d.String()
}

// round keeps one decimal, as the report prints it.
func round(v float64) float64 { return math.Round(v*10) / 10 }

// num prints a percent with at most one decimal and no trailing zero.
func num(v float64) string { return fmt.Sprintf("%g", round(v)) }

// ptr returns a pointer to v.
func ptr(v float64) *float64 { return &v }
