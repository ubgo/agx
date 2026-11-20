package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
	"github.com/ubgo/agx/pool"
)

// Login pools (kind "pool", docs/guides/login-pool.md): an optional
// login with no value of its own whose key, each time it is asked for,
// is the token of one member login. Who decides, first match wins: a
// one-run force (--pool-force), the pool's force, the pool's picker
// command, the built-in rules (package pool). The usage comes from one
// probe per member (cost Q), shared by every agx process through the
// login-usage cache and reused while it is younger than the pool's
// usage_max_age, so a dozen sessions asking for a key every minute still
// probe each member about once a minute.

// WarnPool is the warning code of every login-pool note: a forced pick,
// a probe that fell back to the cache, a picker that failed, a cache that
// could not be read or written.
const WarnPool = "pool"

// Usage sources, as the report prints them. sourceLive is the selector's
// own wording, which the report keeps line for line.
const (
	sourceLive  = "live agx result"
	sourceCache = "cache"
	sourceNone  = "no reading"
)

// Who picked the member (PoolSelection.PickedBy).
const (
	// PickedByRules is the built-in rules (package pool).
	PickedByRules = "rules"
	// PickedByForce is a force: --pool-force or the pool's force.
	PickedByForce = "force"
	// PickedByPicker is the pool's picker command.
	PickedByPicker = "picker"
	// PickedByOnly is a pin (--pool-only): its one member, caps enforced.
	PickedByOnly = "only"
)

// Force sources (PoolSelection.ForcedBy).
const (
	// ForcedByFlag is a one-run --pool-force (pool pick, login token, or
	// a launch whose run keeps it in live.jsonc).
	ForcedByFlag = "--pool-force"
	// ForcedByConfig is the pool's "force" key.
	ForcedByConfig = "config"
)

// envPoolName tells a picker command which pool it picks for.
const envPoolName = "AGX_POOL"

// PickerInputVersion is the version of the JSON a picker reads.
const PickerInputVersion = 1

// PoolPickOptions are agx pool pick's flags (agx credential and login
// token set Force from their own flag or the run).
type PoolPickOptions struct {
	// ID is the pool login.
	ID string
	// Cached reads the login-usage cache only: no probe, any age.
	Cached bool
	// MaxAge overrides the pool's usage_max_age when MaxAgeSet (0 =
	// always probe).
	MaxAge    time.Duration
	MaxAgeSet bool
	// Force picks this member for this one call, over every rule, the
	// pool's force and its picker.
	Force string
	// Only pins this call to one member with the pool's rules still
	// applied to it (--pool-only): no other member, no force, no picker.
	Only PoolOnly
	// Now replaces the clock (--now, for reading old caches); zero = now.
	Now time.Time
	// RunID is the live run asking for a key (agx credential); "" for a
	// pick outside a run. Sticky keeps that run on its member, and
	// max_sessions counts the other runs on each member.
	RunID string
}

// PoolSelection is the member to use and why.
type PoolSelection struct {
	Name  string     `json:"name"`
	Group pool.Group `json:"group"`
	// Forced is true for a force; ForcedBy says which.
	Forced   bool   `json:"forced"`
	ForcedBy string `json:"forced_by,omitempty"`
	// Only is true for a pin (--pool-only): the one member, caps enforced.
	Only bool `json:"only,omitempty"`
	// PickedBy is rules, force, picker or only.
	PickedBy string `json:"picked_by"`
	// Sticky is true when the run stayed on its member.
	Sticky bool `json:"sticky,omitempty"`
	// Sessions is how many other live runs use the member.
	Sessions int `json:"sessions"`
	// Reason is one sentence: what chose it.
	Reason string `json:"reason"`
	// Warnings are the rules a forced or picker-chosen member breaks.
	Warnings       []string   `json:"warnings"`
	WeeklyResetsAt *time.Time `json:"weekly_resets_at"`
	RoomPercent    *float64   `json:"room_percent"`
}

// PoolCandidate is one member in the report, in the selector's --json
// shape (snake_case, as every agx envelope).
type PoolCandidate struct {
	Name     string     `json:"name"`
	Group    pool.Group `json:"group"`
	State    pool.State `json:"state"`
	Rank     *int       `json:"rank"`
	Eligible bool       `json:"eligible"`
	Enabled  bool       `json:"enabled"`
	// Status is the reading's status, or "missing".
	Status         string     `json:"status"`
	Account        string     `json:"account"`
	Short          *float64   `json:"short"`
	Week           *float64   `json:"week"`
	WeeklyResetsAt *time.Time `json:"weekly_resets_at"`
	// HoursToReset is the time to the weekly reset in hours (one decimal;
	// negative = passed); nil when unknown.
	HoursToReset *float64  `json:"hours_to_reset"`
	RoomPercent  *float64  `json:"room_percent"`
	Caps         pool.Caps `json:"caps"`
	// CapsSource says where each cap comes from: member, group, pool
	// (defaults) or default.
	CapsSource CapSources `json:"caps_source"`
	// WeekCapConfigured is the configured weekly cap when caps.week is
	// relaxed above it near the reset; nil otherwise.
	WeekCapConfigured *float64 `json:"week_cap_configured,omitempty"`
	// UrgencyPercentPerHour is room ÷ hours to the reset times the
	// capacity, the urgency order's key; nil when unknown.
	UrgencyPercentPerHour *float64 `json:"urgency_percent_per_hour,omitempty"`
	// Sessions is how many other live runs use the member.
	Sessions int      `json:"sessions"`
	Reasons  []string `json:"reasons"`
	Notes    []string `json:"notes"`
	// Source is "live agx result", "cache" or "no reading"; AgeSeconds
	// is a cached reading's age.
	Source     string   `json:"source"`
	AgeSeconds *float64 `json:"age_seconds"`
}

// CapSources are where a member's 5-hour and weekly caps come from.
type CapSources struct {
	Short string `json:"short"`
	Week  string `json:"week"`
}

// PoolReport is agx pool pick's result.
type PoolReport struct {
	Pool  string `json:"pool"`
	Agent string `json:"agent"`
	// Order is how the pool ranked its members: urgency or deadline.
	Order pool.Order `json:"order"`
	// Settings are the rule values in effect, each with its source.
	Settings []PoolSetting `json:"settings"`
	// Selected is nil when no member is usable.
	Selected *PoolSelection `json:"selected"`
	// Source sums up where the usage came from: "live agx result",
	// "cache (25s old)", "live agx result 2, cache 4 (up to 25s old)".
	Source             string          `json:"source"`
	Now                time.Time       `json:"now"`
	ShortUnavailableAt float64         `json:"short_unavailable_at_percent"`
	BorrowWindowHours  float64         `json:"shared_borrow_window_hours"`
	Warnings           []string        `json:"warnings"`
	Candidates         []PoolCandidate `json:"candidates"`
	// View is how the human report writes reset times.
	View TimeView `json:"-"`
	// Detected is the kind detected in the handed-out member's token
	// (poolMemberToken only); "" otherwise.
	Detected string `json:"-"`
}

// perUseNotProbed is the cached reason of a member skipped as a per-use
// API key: it was resolved but never probed.
const perUseNotProbed = "token is an API key (billed per use); not probed"

// warn adds one warning.
func (r *PoolReport) warn(msg string) { r.Warnings = append(r.Warnings, msg) }

// StderrWarnings are the report's warnings, and each rule a forced or
// picker-chosen member breaks, as stderr notes (login token, credential).
func (r PoolReport) StderrWarnings() []report.Warning {
	out := make([]report.Warning, 0, len(r.Warnings))
	if s := r.Selected; s != nil {
		verb := "forced " + s.Name
		if s.PickedBy == PickedByPicker {
			verb = "picker chose " + s.Name
		}
		for _, w := range s.Warnings {
			out = append(out, report.Warning{Code: WarnPool, Message: verb + ": " + w})
		}
	}
	for _, w := range r.Warnings {
		out = append(out, report.Warning{Code: WarnPool, Message: w})
	}
	return out
}

// PoolPick runs a pool's selection and returns the report (agx pool
// pick). Cost Q unless Cached: members whose cached reading is older than
// the max age are probed once. No member usable is a failure with code
// auto-pick (exit 1), the report still returned.
func (a *App) PoolPick(ctx context.Context, o PoolPickOptions, to TimeOptions) (PoolReport, error) {
	if err := checkTimeOptions(to); err != nil {
		return PoolReport{}, err
	}
	if o.MaxAgeSet && o.MaxAge < 0 {
		return PoolReport{}, apperr.Usage(fmt.Sprintf("--max-age %s is negative; 0 probes every member", o.MaxAge))
	}
	cfg, lg, err := a.namedLogin(o.ID)
	if err != nil {
		return PoolReport{}, err
	}
	if lg.Kind != config.KindPool {
		return PoolReport{}, apperr.Usage(a.notAPool(poolCmdPick, o.ID, lg)).WithHint(poolListHint)
	}
	view, err := a.timeView(cfg.Config, to)
	if err != nil {
		return PoolReport{}, err
	}
	rep, err := a.poolPick(ctx, o.ID, lg, o)
	rep.View = view
	if !o.Now.IsZero() {
		rep.View.Now = o.Now
	}
	_ = a.poolLog(lg, poolModePick, rep, err)
	return rep, err
}

// The agx pool subcommands, as a refusal names them ("pool show is for
// pool logins").
const (
	poolCmdPick = "pick"
	poolCmdShow = "show"
)

// poolListHint is the hint of a refused pool command: the pools there are.
const poolListHint = "agx pool list"

// notAPool says agx pool CMD is for pool logins, which agents can have
// one (from the registry's capabilities, never a fixed name), and why
// this login's own agents cannot when they cannot.
func (a *App) notAPool(cmd, name string, lg config.Login) string {
	var can []string
	if reg, err := a.Registry(); err == nil {
		for _, id := range reg.IDs() {
			if agents.PoolReason(id) == "" {
				can = append(can, string(id))
			}
		}
	}
	msg := fmt.Sprintf("pool %s is for pool logins; login %s is %s. Pools serve %s today", cmd, name, lg.Kind, strings.Join(can, ", "))
	var why []string
	for _, id := range lg.Agents {
		if r := agents.PoolReason(id); r != "" {
			why = append(why, string(id)+": "+r)
		}
	}
	if len(why) > 0 {
		msg += " (" + strings.Join(why, "; ") + ")"
	}
	return msg
}

// poolPick reads the members' usage and decides. The error is nil only
// when a member was selected.
func (a *App) poolPick(ctx context.Context, name string, lg config.Login, o PoolPickOptions) (PoolReport, error) {
	rep := PoolReport{Pool: name, Warnings: []string{}, Candidates: []PoolCandidate{}, Settings: []PoolSetting{}}
	if lg.Pool == nil || len(lg.Agents) != 1 {
		return rep, apperr.Failure(apperr.CodeConfig, "login "+name+" is a pool without a pool block or one agent (agx config check)")
	}
	rules, err := lg.Pool.Rules()
	if err != nil {
		return rep, apperr.Failure(apperr.CodeConfig, "login "+name+": "+err.Error())
	}
	maxAge, _ := lg.Pool.MaxAge() // Rules checked it
	if o.MaxAgeSet {
		maxAge = o.MaxAge
	}
	cfg, err := a.Config()
	if err != nil {
		return rep, err
	}
	ag, err := a.loginAgent(lg.Agents[0])
	if err != nil {
		return rep, err
	}
	now := o.Now
	if now.IsZero() {
		now = a.Now()
	}
	rep.Agent, rep.Now, rep.ShortUnavailableAt = string(ag.ID()), now, rules.ShortUnavailableAt
	rep.BorrowWindowHours = rules.BorrowWindow.Hours()
	rep.Order = rules.Rules().Order
	rep.Settings = poolRuleSettings(*lg.Pool)

	if err := checkPoolOnly(name, lg.Pool, rules, o.Only, o.Force, false); err != nil {
		// A refused flag is not a pick: no report to print.
		return PoolReport{}, err
	}
	force, forcedBy := o.Force, ForcedByFlag
	if force == "" && o.Only.Member == "" {
		// A pin outranks the pool's standing force: the force would
		// ignore the very caps the pin keeps.
		force, forcedBy = rules.Force, ForcedByConfig
	}
	if force != "" {
		if err := a.checkForce(cfg.Config, force, ag.ID()); err != nil {
			return rep, err
		}
	}
	read := []string{}
	for _, m := range lg.Pool.Members() {
		pinnedOut := o.Only.Member != "" && m != o.Only.Member
		if (rules.Enabled(m) || m == force) && !pinnedOut {
			read = append(read, m)
		}
	}
	if force != "" && !slices.Contains(read, force) {
		read = append(read, force)
	}

	cache, cerr := login.ReadUsageCache(a.paths)
	if cerr != nil {
		rep.warn("login-usage cache: " + cerr.Error())
	}
	entries, sources, err := a.poolReadings(ctx, cfg.Config, ag, read, cache, maxAge, o.Cached, now, rep.warn)
	if err != nil {
		return rep, err
	}
	readings := map[string]pool.Reading{}
	for m, e := range entries {
		readings[m] = readingOf(e)
		readings[m] = withPerUse(readings[m], cfg.Config.Logins[m], e)
	}
	sel := pool.Options{Now: now, Force: o.Force, Only: o.Only.Member, OnlyCaps: o.Only.Caps}
	if rules.MaxSessions > 0 || rules.Sticky {
		sel.Load, sel.Current, sel.CurrentAhead = a.poolLoad(name, lg, o.RunID, now, rep.warn)
	}
	res, err := pool.Select(rules, readings, sel)
	if errors.Is(err, pool.ErrForcedMissing) {
		return rep, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("forced login %s has no usage reading (run without --cached)", force))
	}
	if err != nil {
		return rep, apperr.Wrap(err, apperr.CodeAutoPick, apperr.ClassFailure, err.Error())
	}
	for _, c := range res.Candidates {
		pc := candidateOf(c, cfg.Config.Logins[c.Name].Account, sources[c.Name])
		_, pc.CapsSource.Short, _ = memberValue(*lg.Pool, c.Name, c.Group, memberShort)
		_, pc.CapsSource.Week, _ = memberValue(*lg.Pool, c.Name, c.Group, memberWeek)
		if c.Name == o.Only.Member && o.Only.Caps.Short != nil {
			pc.CapsSource.Short = flagPoolOnlyShort
		}
		if c.Name == o.Only.Member && o.Only.Caps.Week != nil {
			pc.CapsSource.Week = flagPoolOnlyWeek
		}
		rep.Candidates = append(rep.Candidates, pc)
	}
	rep.Source = sourceSummary(sources)
	switch s := res.Selected; {
	case s != nil && s.Forced:
		rep.Selected = selectionOf(s, PickedByForce)
		rep.Selected.ForcedBy = forcedBy
	case lg.Pool.Picker != nil && o.Only.Member == "":
		// A pin is not the picker's to change: it keeps one member.
		rep.Selected = a.runPicker(ctx, name, lg, rules, readings, res, &rep)
	case s != nil && s.Only:
		rep.Selected = selectionOf(s, PickedByOnly)
	case s != nil:
		rep.Selected = selectionOf(s, PickedByRules)
	}
	if rep.Selected == nil && o.Only.Member != "" {
		return rep, onlyFailure(o.Only.Member, res)
	}
	if rep.Selected == nil {
		parts := make([]string, 0, len(res.Candidates))
		for _, c := range res.Candidates {
			parts = append(parts, c.Name+" ("+strings.Join(c.Reasons, "; ")+")")
		}
		return rep, apperr.Failure(apperr.CodeAutoPick, "pool "+name+": no member is usable: "+strings.Join(parts, ", ")).
			WithHint("agx pool pick --id " + name + " --pool-force NAME")
	}
	return rep, nil
}

// checkForce refuses a force that names no token login of the pool's
// agent: there would be no token to hand out.
func (a *App) checkForce(c config.Config, force string, agent agentio.ID) error {
	if _, err := liveLogin(c, force, string(agent)); err != nil {
		return err
	}
	if c.Logins[force].Kind == config.KindPool {
		return apperr.Usage(fmt.Sprintf("forced login %s is a pool; force a token login", force))
	}
	return nil
}

// selectionOf copies the rules' selection.
func selectionOf(s *pool.Selection, by string) *PoolSelection {
	return &PoolSelection{Name: s.Name, Group: s.Group, Forced: s.Forced, Only: s.Only, PickedBy: by, Reason: s.Reason,
		Sticky: s.Sticky, Sessions: s.Load,
		Warnings: append([]string{}, s.Warnings...), WeeklyResetsAt: s.WeeklyResetsAt, RoomPercent: s.Room}
}

// candidateOf is one member in the report shape.
func candidateOf(c pool.Candidate, account string, src memberSource) PoolCandidate {
	pc := PoolCandidate{
		Name: c.Name, Group: c.Group, State: c.State, Eligible: c.Eligible, Enabled: c.Enabled, Status: c.Status,
		Account: account, Short: c.Short, Week: c.Week, WeeklyResetsAt: c.WeeklyResetsAt, RoomPercent: c.Room,
		Caps: c.Caps, Reasons: c.Reasons, Notes: c.Notes, Source: sourceNone,
		WeekCapConfigured: c.WeekCapConfigured, UrgencyPercentPerHour: c.Urgency, Sessions: c.Load,
	}
	if c.Rank > 0 {
		r := c.Rank
		pc.Rank = &r
	}
	if c.UntilReset != nil {
		h := math.Round(c.UntilReset.Hours()*10) / 10
		pc.HoursToReset = &h
	}
	if src.label != "" {
		pc.Source = src.label
		if src.label == sourceCache {
			s := math.Round(src.age.Seconds())
			pc.AgeSeconds = &s
		}
	}
	return pc
}

// pickerInput is what a picker command reads on stdin (version 1, docs:
// login-pool.md "Your own picker"). It holds no token.
type pickerInput struct {
	Version int    `json:"version"`
	Pool    string `json:"pool"`
	Agent   string `json:"agent"`
	Now     string `json:"now"`
	// Builtin is the member the built-in rules pick; null when none is
	// usable.
	Builtin *string         `json:"builtin"`
	Rules   pickerRules     `json:"rules"`
	Members []PoolCandidate `json:"members"`
}

// pickerRules are the pool's numbers, for a picker that wants them.
type pickerRules struct {
	SharedBorrowWindowHours   float64             `json:"shared_borrow_window_hours"`
	ShortUnavailableAtPercent float64             `json:"short_unavailable_at_percent"`
	Defaults                  pool.Caps           `json:"defaults"`
	Own                       []string            `json:"own"`
	Shared                    []string            `json:"shared"`
	Accounts                  map[string]pickCaps `json:"accounts"`
}

// pickCaps is one member's caps and enabled flag after defaults.
type pickCaps struct {
	Short   float64 `json:"short"`
	Week    float64 `json:"week"`
	Enabled bool    `json:"enabled"`
}

// runPicker asks the pool's picker command for the member. It reads the
// members and their usage as JSON on stdin and prints one login name. A
// picker that fails (no start, non-zero exit, timeout, no line or more
// than one, a name that is not a member) falls back to the built-in
// rules with a warning: a key request that fails stops Claude's session,
// so a broken picker must not take the session down with it.
func (a *App) runPicker(ctx context.Context, name string, lg config.Login, rules pool.Config, readings map[string]pool.Reading, res pool.Result, rep *PoolReport) *PoolSelection {
	builtin := func() *PoolSelection {
		if res.Selected == nil {
			return nil
		}
		return selectionOf(res.Selected, PickedByRules)
	}
	in := pickerInput{Version: PickerInputVersion, Pool: name, Agent: rep.Agent, Now: rep.Now.UTC().Format(time.RFC3339), Members: rep.Candidates,
		Rules: pickerRules{SharedBorrowWindowHours: rules.BorrowWindow.Hours(), ShortUnavailableAtPercent: rules.ShortUnavailableAt,
			Defaults: rules.Defaults, Own: append([]string{}, rules.Own...), Shared: append([]string{}, rules.Shared...), Accounts: map[string]pickCaps{}}}
	for _, m := range lg.Pool.Members() {
		caps := rules.CapsFor(m)
		in.Rules.Accounts[m] = pickCaps{Short: caps.Short, Week: caps.Week, Enabled: rules.Enabled(m)}
	}
	if res.Selected != nil {
		b := res.Selected.Name
		in.Builtin = &b
	}
	body, err := json.Marshal(in)
	if err != nil {
		rep.warn("picker: " + err.Error() + "; using the built-in rules")
		return builtin()
	}
	timeout, _ := lg.Pool.PickerTimeout() // Rules checked it
	env := append(append([]string{}, a.d.Host.Env...), envPoolName+"="+name)
	out, err := run.Filter(ctx, lg.Pool.Picker.Command, env, body, timeout)
	if err == nil {
		var choice string
		if choice, err = pickerChoice(out, lg.Pool.Members()); err == nil {
			rd, found := readings[choice]
			sel := &PoolSelection{Name: choice, Group: groupOf(lg.Pool, choice), PickedBy: PickedByPicker,
				Reason: "picked by the picker command (" + firstWord(lg.Pool.Picker.Command) + ")", Warnings: rules.Reasons(choice, rd, found)}
			for _, c := range rep.Candidates {
				if c.Name == choice {
					sel.WeeklyResetsAt, sel.RoomPercent = c.WeeklyResetsAt, c.RoomPercent
				}
			}
			return sel
		}
	}
	rep.warn("picker " + firstWord(lg.Pool.Picker.Command) + " failed (" + err.Error() + "); using the built-in rules")
	return builtin()
}

// pickerChoice is the one login name a picker printed, which must be a
// member: exactly one non-empty line, trimmed.
func pickerChoice(out string, members []string) (string, error) {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}
	switch {
	case len(lines) == 0:
		return "", errors.New("printed no login name")
	case len(lines) > 1:
		return "", fmt.Errorf("printed %d lines; print one login name", len(lines))
	case !slices.Contains(members, lines[0]):
		return "", fmt.Errorf("printed %q, which is not a member of the pool", trimTo(lines[0], maxEcho))
	}
	return lines[0], nil
}

// maxEcho bounds how much of a picker's output a warning repeats.
const maxEcho = 40

// trimTo cuts s to n bytes with "…".
func trimTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// groupOf is a member's group.
func groupOf(p *config.Pool, name string) pool.Group {
	if slices.Contains(p.Own, name) {
		return pool.GroupOwn
	}
	return pool.GroupShared
}

// memberSource is where one member's reading came from.
type memberSource struct {
	label string
	age   time.Duration
}

// poolReadings returns a usage entry per member in names: the cache when
// it is younger than maxAge (or with cached, any age), else a probe.
// Members are probed in parallel; a probe that fails for a passing reason
// (network, 429 without usage, an unexpected answer) falls back to the
// cached entry with a warning, and the new answers are written back.
func (a *App) poolReadings(ctx context.Context, c config.Config, ag agentio.Agent, names []string, cache login.UsageCache,
	maxAge time.Duration, cached bool, now time.Time, warn func(string)) (map[string]login.UsageEntry, map[string]memberSource, error) {
	entries := map[string]login.UsageEntry{}
	sources := map[string]memberSource{}
	fromCache := func(m string, e login.UsageEntry) {
		entries[m] = e
		sources[m] = memberSource{label: sourceCache, age: now.Sub(e.ProbedAt)}
	}
	var probe []string
	for _, m := range names {
		e, ok := cache.Logins[m]
		ok = ok && e.Agent == string(ag.ID())
		switch {
		case cached && ok:
			fromCache(m, e)
		case cached:
		case ok && now.Sub(e.ProbedAt) < maxAge && !e.ProbedAt.After(now):
			fromCache(m, e)
		default:
			probe = append(probe, m)
		}
	}
	if len(probe) == 0 {
		return entries, sources, nil
	}
	pr, ok := agentio.TokenProberOf(ag)
	if !ok {
		return nil, nil, apperr.Unsupported(fmt.Sprintf("%s cannot probe a login's usage (%s)", ag.ID(), usageUnsupportedReason(ag, agentio.CapTokenProber, reasonNoProbe)))
	}
	probed := make([]login.UsageEntry, len(probe))
	var wg sync.WaitGroup
	for i, m := range probe {
		wg.Add(1)
		go func() {
			defer wg.Done()
			probed[i] = a.probeMember(ctx, pr, ag, m, c.Logins[m], now)
		}()
	}
	wg.Wait()
	if err := ctxErr(ctx); err != nil {
		return nil, nil, err
	}
	fresh := map[string]login.UsageEntry{}
	for i, m := range probe {
		e := probed[i]
		old, hasOld := cache.Logins[m]
		if passingFailure(RowStatus(e.Status)) && hasOld && old.Agent == string(ag.ID()) {
			warn(fmt.Sprintf("probe of %s failed (%s); using its cached usage from %s ago", m, failureText(e), roundAge(now.Sub(old.ProbedAt))))
			fromCache(m, old)
			continue
		}
		entries[m] = e
		sources[m] = memberSource{label: sourceLive}
		if !passingFailure(RowStatus(e.Status)) {
			// A failed request is not an answer: caching it would block
			// the member until max age passes instead of asking again.
			fresh[m] = e
		}
	}
	if len(fresh) > 0 {
		if _, err := login.UpdateUsageCache(ctx, a.paths, fresh); err != nil {
			warn("login-usage cache not written: " + apperr.Text(err))
		}
	}
	return entries, sources, nil
}

// probeMember resolves one member's token and probes it once (cost Q,
// plus X for a reference that runs a command). The token is never kept.
func (a *App) probeMember(ctx context.Context, pr agentio.TokenProber, ag agentio.Agent, name string, lg config.Login, now time.Time) login.UsageEntry {
	e := login.UsageEntry{Agent: string(ag.ID()), Account: lg.Account, ProbedAt: now, Windows: []login.UsageCacheWindow{}}
	cred, err := login.Credential(ctx, a.exp.Brief(), a.paths, name, lg)
	if err != nil {
		e.Status, e.Reason = string(StatusSecret), apperr.As(err).Message
		return e
	}
	d := login.DetectToken(cred.Token)
	e.Detected = string(d)
	if login.PerUseRefused(lg, d, false) {
		// Probing an API key spends money on the account that owns it,
		// for a member the rules will skip anyway.
		e.Status, e.Reason = string(StatusPerToken), perUseNotProbed
		return e
	}
	rctx, cancel := context.WithTimeout(ctx, a.d.Timeout)
	defer cancel()
	u, err := pr.TokenUsage(rctx, cred)
	var r UsageRow
	setUsage(&r, u, err, SourceProbe)
	e.Status, e.Reason = string(r.Status), r.Reason
	for _, w := range r.Windows {
		e.Windows = append(e.Windows, login.UsageCacheWindow{Class: w.Class, Scope: w.Scope, Percent: w.Percent, ResetsAt: w.ResetsAt})
	}
	return e
}

// passingFailure is a probe failure that says nothing about the account
// (the request failed), so a cached answer is better than none.
func passingFailure(s RowStatus) bool {
	return s == StatusNetwork || s == StatusRateLimited || s == StatusError
}

// failureText is a failed entry's status and reason.
func failureText(e login.UsageEntry) string {
	if e.Reason == "" {
		return e.Status
	}
	return e.Status + ": " + e.Reason
}

// withPerUse marks a member whose cached token kind is an API key under a
// login declared oauth-token that does not allow it.
func withPerUse(r pool.Reading, lg config.Login, e login.UsageEntry) pool.Reading {
	r.PerUseKey = login.PerUseRefused(lg, login.Detected(e.Detected), false)
	return r
}

// readingOf turns a cached entry into the rules' reading: its status
// ("limit-reached" blocks by itself, as in the usage table) and the
// first all-usage short and weekly windows (a per-model window does not
// decide whether the account can take a request).
func readingOf(e login.UsageEntry) pool.Reading {
	r := pool.Reading{Status: e.Status}
	for _, w := range e.Windows {
		if w.Scope != "" {
			continue
		}
		win := &pool.Window{Percent: w.Percent, ResetsAt: w.ResetsAt}
		switch w.Class {
		case agentio.WindowShort.String():
			if r.Short == nil {
				r.Short = win
			}
		case agentio.WindowWeek.String():
			if r.Week == nil {
				r.Week = win
			}
		}
	}
	return r
}

// sourceSummary is the report's one-line usage source.
func sourceSummary(sources map[string]memberSource) string {
	probes, caches := 0, 0
	var oldest time.Duration
	for _, s := range sources {
		if s.label == sourceLive {
			probes++
			continue
		}
		caches++
		oldest = max(oldest, s.age)
	}
	switch {
	case probes == 0 && caches == 0:
		return sourceNone
	case caches == 0:
		return sourceLive
	case probes == 0:
		return fmt.Sprintf("%s (%s old)", sourceCache, roundAge(oldest))
	}
	return fmt.Sprintf("%s %d, %s %d (up to %s old)", sourceLive, probes, sourceCache, caches, roundAge(oldest))
}

// roundAge renders an age to the second.
func roundAge(d time.Duration) string { return d.Round(time.Second).String() }

// Pool log modes: who asked for the key.
const (
	poolModeCredential = "credential"
	poolModeToken      = "token"
	poolModePick       = "pick"
)

// poolLog appends one line per pick to the pool's log when it sets one:
// time, mode, pool, pick (null when none), group, who picked, source, and
// the error's first words when it failed. Login names and agx's own words
// only, never a token. A write failure is a warning in the result.
func (a *App) poolLog(lg config.Login, mode string, rep PoolReport, pickErr error) []report.Warning {
	if lg.Pool == nil || lg.Pool.Log == "" {
		return nil
	}
	path := config.ExpandPath(lg.Pool.Log, a.d.Lookup, a.d.Host.UserHome)
	picked, group, by, forced := "null", "null", "null", false
	if s := rep.Selected; s != nil {
		picked, group, by, forced = quote(s.Name), quote(string(s.Group)), quote(s.PickedBy), s.Forced
	}
	line := fmt.Sprintf("%s mode=%s pool=%s picked=%s group=%s forced=%t by=%s source=%q",
		a.Now().UTC().Format(time.RFC3339), mode, quote(rep.Pool), picked, group, forced, by, rep.Source)
	if pickErr != nil {
		line += " error=" + quote(trimTo(apperr.As(pickErr).Message, maxLogError))
	}
	if err := login.AppendPoolLog(path, line); err != nil {
		return []report.Warning{{Code: WarnPool, Message: "pool log " + a.tilde(path) + ": " + err.Error()}}
	}
	return nil
}

// maxLogError bounds the error text in a log line.
const maxLogError = 200

// quote is a JSON string (names may hold only safe runes, but the
// format stays parseable whatever they hold).
func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// poolMemberToken picks a pool member and resolves its token (agx
// credential and agx login token), with a one-run force or pin. The
// report's warnings go to stderr.
//
// The member's token kind is detected as it is resolved: a member
// declared oauth-token whose token is an API key (billed per use) is
// never handed out unless its allow_api_key is set. When the cache did
// not know that yet (an entry from before detection), the kind is
// recorded there and the pool picks once more, now skipping it; a forced,
// pinned or picker-chosen member is refused instead.
func (a *App) poolMemberToken(ctx context.Context, name string, lg config.Login, mode, force string, only PoolOnly, runID string) (string, agentio.Token, PoolReport, error) {
	for attempt := 0; ; attempt++ {
		rep, err := a.poolPick(ctx, name, lg, PoolPickOptions{Force: force, Only: only, RunID: runID})
		if err != nil {
			a.logPoolInto(&rep, lg, mode, err)
			return "", agentio.Token{}, rep, err
		}
		cfg, err := a.Config()
		if err != nil {
			return "", agentio.Token{}, rep, err
		}
		member := rep.Selected.Name
		mlg := cfg.Config.Logins[member]
		cred, err := login.Credential(ctx, a.exp.Brief(), a.paths, member, mlg)
		if err == nil {
			d := login.DetectToken(cred.Token)
			learned := a.noteDetected(ctx, member, d)
			if login.PerUseRefused(mlg, d, false) {
				if learned && attempt == 0 && rep.Selected.PickedBy == PickedByRules {
					continue
				}
				err = login.CheckPerUse(member, mlg, d, false, false)
			}
			rep.Detected = string(d)
		}
		a.logPoolInto(&rep, lg, mode, err)
		if err != nil {
			return member, agentio.Token{}, rep, err
		}
		return member, cred.Token, rep, nil
	}
}

// noteDetected records member's detected token kind in its login-usage
// cache entry when the entry lacks it or says otherwise, and reports
// whether it changed. A write failure is dropped: the pick goes on with
// what it knows.
func (a *App) noteDetected(ctx context.Context, member string, d login.Detected) bool {
	cache, err := login.ReadUsageCache(a.paths)
	if err != nil {
		return false
	}
	e, ok := cache.Logins[member]
	if !ok || e.Detected == string(d) {
		return false
	}
	e.Detected = string(d)
	_, err = login.UpdateUsageCache(ctx, a.paths, map[string]login.UsageEntry{member: e})
	return err == nil
}

// logPoolInto logs one pick and keeps a log write failure as a warning.
func (a *App) logPoolInto(rep *PoolReport, lg config.Login, mode string, err error) {
	for _, w := range a.poolLog(lg, mode, *rep, err) {
		rep.warn(w.Message)
	}
}

// activeRunTTLs is how many key_ttls a run may go without asking for a
// key and still count as a session on its member: a busy run asks every
// key_ttl, an idle or ended one stops asking.
const activeRunTTLs = 2

// poolLoad reads the live runs of pool name (max_sessions, sticky): how many
// runs other than runID use each member now (they asked within
// activeRunTTLs key_ttls), runID's own member, and how many of the runs
// on that member started before runID (run ids begin with their start
// time, so they compare in start order). A run folder that cannot be
// read is skipped; a runs folder that cannot be read is one warning and
// no load.
func (a *App) poolLoad(name string, lg config.Login, runID string, now time.Time, warn func(string)) (map[string]int, string, int) {
	folders, err := run.List(a.paths.Runs)
	if err != nil {
		warn("live runs: " + err.Error() + "; sessions per member not counted")
		return nil, "", 0
	}
	ttl, _ := lg.Pool.TTL() // Rules checked it
	members := lg.Pool.Members()
	type seat struct {
		id, member string
	}
	var seats []seat
	current := ""
	for _, f := range folders {
		st, err := live.Read(f.Dir)
		if err != nil || st.Want() != name || !slices.Contains(members, st.InUse) {
			continue
		}
		if f.ID == runID {
			current = st.InUse
			continue
		}
		if now.Sub(st.InUseAt) <= activeRunTTLs*ttl {
			seats = append(seats, seat{f.ID, st.InUse})
		}
	}
	load := map[string]int{}
	ahead := 0
	for _, s := range seats {
		load[s.member]++
		if s.member == current && s.id < runID {
			ahead++
		}
	}
	return load, current, ahead
}
