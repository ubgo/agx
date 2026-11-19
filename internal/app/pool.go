package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/login"
	"github.com/ubgo/agx/pool"
)

// Login pools (kind "pool", docs/guides/login-pool.md): a login with no
// value of its own whose key, each time it is asked for, is the token of
// one member login picked by package pool from the members' usage. The
// usage comes from one probe per member (cost Q), shared by every agx
// process through the login-usage cache and reused while it is younger
// than the pool's usage_max_age, so a dozen sessions asking for a key
// every minute still probe each member about once a minute.

// WarnPool is the warning code of every login-pool note: a forced pick,
// a probe that failed and fell back to the cache, a cache that could not
// be read or written.
const WarnPool = "pool"

// Usage sources of a pool member, as the report prints them.
const (
	poolSourceProbe   = "live probe"
	poolSourceCache   = "cache"
	poolSourceNone    = "no reading"
	poolSourceSkipped = "not probed (disabled)"
)

// PoolPickOptions are agx login pick's flags (and what agx credential and
// login token use: all zero).
type PoolPickOptions struct {
	// ID is the pool login.
	ID string
	// Cached reads the login-usage cache only: no probe, any age.
	Cached bool
	// MaxAge overrides the pool's usage_max_age when > 0.
	MaxAge time.Duration
	// Force picks this member for this one call, over every rule and the
	// pool's own force.
	Force string
}

// PoolCandidate is one member in the report: the selection fields plus
// its account label and where its usage came from.
type PoolCandidate struct {
	pool.Candidate
	// Account is the member login's display label.
	Account string `json:"account"`
	// Source is "live probe", "cache" or "no reading".
	Source string `json:"source"`
	// Age is how old a cached reading is; 0 for a live probe.
	Age time.Duration `json:"-"`
}

// PoolReport is agx login pick's result.
type PoolReport struct {
	// Pool is the pool login's name; Agent the agent it serves.
	Pool  string `json:"pool"`
	Agent string `json:"agent"`
	// Selected is the member to use; nil when none is usable.
	Selected *pool.Selection `json:"selected"`
	// Candidates are every member in report order.
	Candidates []PoolCandidate `json:"candidates"`
	// Now is the time the rules measured from.
	Now time.Time `json:"now"`
	// Source sums up where the usage came from ("live probe", "cache
	// (25s old)", "live probe 2, cache 4").
	Source string `json:"source"`
	// BorrowWindow, MaxAge and KeyTTL are the pool's numbers.
	BorrowWindow       string  `json:"shared_borrow_window"`
	ShortUnavailableAt float64 `json:"short_unavailable_at_percent"`
	MaxAge             string  `json:"usage_max_age"`
	KeyTTL             string  `json:"key_ttl"`
	// Warnings are the notes (also printed on stderr).
	Warnings []report.Warning `json:"-"`
}

// LoginPick runs a pool's selection and returns the report (agx login
// pick). Cost Q unless Cached: the members whose cached reading is older
// than the max age are probed once. No member usable is a failure with
// code auto-pick (exit 1), the report still returned.
func (a *App) LoginPick(ctx context.Context, o PoolPickOptions) (PoolReport, error) {
	_, lg, err := a.namedLogin(o.ID)
	if err != nil {
		return PoolReport{}, err
	}
	if lg.Kind != config.KindPool {
		return PoolReport{}, apperr.Usage(fmt.Sprintf("login %s is %s, not a pool; agx login pick reads a pool login", o.ID, lg.Kind)).WithHint("agx login list")
	}
	return a.poolPick(ctx, o.ID, lg, o)
}

// poolPick reads the members' usage and applies the rules. The error is
// nil only when a member was selected.
func (a *App) poolPick(ctx context.Context, name string, lg config.Login, o PoolPickOptions) (PoolReport, error) {
	if lg.Pool == nil || len(lg.Agents) != 1 {
		return PoolReport{}, apperr.Failure(apperr.CodeConfig, "login "+name+" is a pool without a pool block or one agent (agx config check)")
	}
	rules, err := lg.Pool.Rules()
	if err != nil {
		return PoolReport{}, apperr.Failure(apperr.CodeConfig, "login "+name+": "+err.Error())
	}
	maxAge, _ := lg.Pool.MaxAge() // Rules checked it
	ttl, _ := lg.Pool.TTL()
	if o.MaxAge > 0 {
		maxAge = o.MaxAge
	}
	cfg, err := a.Config()
	if err != nil {
		return PoolReport{}, err
	}
	ag, err := a.loginAgent(lg.Agents[0])
	if err != nil {
		return PoolReport{}, err
	}
	now := a.Now()
	rep := PoolReport{
		Pool: name, Agent: string(ag.ID()), Now: now, BorrowWindow: pool.Hours(rules.BorrowWindow),
		ShortUnavailableAt: rules.ShortUnavailableAt, MaxAge: maxAge.String(), KeyTTL: ttl.String(),
	}
	warn := func(msg string) { rep.Warnings = append(rep.Warnings, report.Warning{Code: WarnPool, Message: msg}) }

	force := o.Force
	if force == "" {
		force = rules.Force
	}
	if force != "" {
		if _, err := liveLogin(cfg.Config, force, string(ag.ID())); err != nil {
			return rep, err
		}
		if cfg.Config.Logins[force].Kind == config.KindPool {
			return rep, apperr.Usage(fmt.Sprintf("forced login %s is a pool; force a token login", force))
		}
	}
	read := []string{}
	for _, m := range lg.Pool.Members() {
		if rules.Enabled(m) || m == force {
			read = append(read, m)
		}
	}
	if force != "" && !slices.Contains(read, force) {
		read = append(read, force)
	}

	cache, cerr := login.ReadUsageCache(a.paths)
	if cerr != nil {
		warn("login-usage cache: " + cerr.Error())
	}
	entries, sources, err := a.poolReadings(ctx, cfg.Config, ag, read, cache, maxAge, o.Cached, now, warn)
	if err != nil {
		return rep, err
	}
	readings := map[string]pool.Reading{}
	for m, e := range entries {
		readings[m] = readingOf(e)
	}
	res, err := pool.Select(rules, readings, pool.Options{Now: now, Force: o.Force})
	if errors.Is(err, pool.ErrForcedMissing) {
		return rep, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("forced login %s has no usage reading (run without --cached)", force))
	}
	if err != nil {
		return rep, apperr.Wrap(err, apperr.CodeAutoPick, apperr.ClassFailure, err.Error())
	}
	rep.Selected = res.Selected
	for _, c := range res.Candidates {
		pc := PoolCandidate{Candidate: c, Account: cfg.Config.Logins[c.Name].Account, Source: poolSourceNone}
		switch src, ok := sources[c.Name]; {
		case ok:
			pc.Source, pc.Age = src.label, src.age
		case !c.Enabled:
			pc.Source = poolSourceSkipped
		}
		rep.Candidates = append(rep.Candidates, pc)
	}
	rep.Source = sourceSummary(sources)
	if s := rep.Selected; s != nil && s.Forced {
		for _, w := range s.Warnings {
			warn(fmt.Sprintf("forced %s: %s", s.Name, w))
		}
	}
	if rep.Selected == nil {
		parts := make([]string, 0, len(res.Candidates))
		for _, c := range res.Candidates {
			parts = append(parts, c.Name+" ("+strings.Join(c.Reasons, "; ")+")")
		}
		return rep, apperr.Failure(apperr.CodeAutoPick, "pool "+name+": no member is usable: "+strings.Join(parts, ", ")).
			WithHint("agx login pick --id " + name + " --force NAME")
	}
	return rep, nil
}

// memberSource is where one member's reading came from.
type memberSource struct {
	label string
	age   time.Duration
}

// poolReadings returns a usage entry per member in names: the cache when
// it is fresh enough (or with cached, any age), else a probe. Members are
// probed in parallel; a probe that fails for a passing reason (network,
// 429 without usage, an unexpected answer) falls back to the cached entry
// with a warning, and the new answers are written back to the cache.
func (a *App) poolReadings(ctx context.Context, c config.Config, ag agentio.Agent, names []string, cache login.UsageCache,
	maxAge time.Duration, cached bool, now time.Time, warn func(string)) (map[string]login.UsageEntry, map[string]memberSource, error) {
	entries := map[string]login.UsageEntry{}
	sources := map[string]memberSource{}
	fromCache := func(m string, e login.UsageEntry) {
		entries[m] = e
		sources[m] = memberSource{label: poolSourceCache, age: now.Sub(e.ProbedAt)}
	}
	var probe []string
	for _, m := range names {
		e, ok := cache.Logins[m]
		ok = ok && e.Agent == string(ag.ID())
		switch {
		case cached && ok:
			fromCache(m, e)
		case cached:
		case ok && now.Sub(e.ProbedAt) <= maxAge && !e.ProbedAt.After(now):
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
		sources[m] = memberSource{label: poolSourceProbe}
		fresh[m] = e
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

// readingOf turns a cached entry into the rules' reading: its status
// ("limit-reached" blocks by itself, as in the usage table) and the
// first all-usage short and weekly windows (a per-model window does
// not decide whether the account can take a request).
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
		if s.label == poolSourceProbe {
			probes++
			continue
		}
		caches++
		oldest = max(oldest, s.age)
	}
	switch {
	case probes == 0 && caches == 0:
		return poolSourceNone
	case caches == 0:
		return poolSourceProbe
	case probes == 0:
		return fmt.Sprintf("%s (%s old)", poolSourceCache, roundAge(oldest))
	}
	return fmt.Sprintf("%s %d, %s %d (up to %s old)", poolSourceProbe, probes, poolSourceCache, caches, roundAge(oldest))
}

// roundAge renders an age to the second.
func roundAge(d time.Duration) string { return d.Round(time.Second).String() }

// poolLogFormat is one line of a pool's log: time, mode, pick, source.
// Values are login names and agx's own words, never a token.
const poolLogFormat = "%s mode=%s pool=%s picked=%s group=%s forced=%t source=%q"

// poolLog appends one line to the pool's log when it sets one; a write
// failure is a warning, never a failed key request.
func (a *App) poolLog(lg config.Login, mode string, rep PoolReport) []report.Warning {
	if lg.Pool == nil || lg.Pool.Log == "" || rep.Selected == nil {
		return nil
	}
	path := config.ExpandPath(lg.Pool.Log, a.d.Lookup, a.d.Host.UserHome)
	s := rep.Selected
	line := fmt.Sprintf(poolLogFormat, a.Now().UTC().Format(time.RFC3339), mode, rep.Pool, s.Name, s.Group, s.Forced, rep.Source)
	if err := login.AppendPoolLog(path, line); err != nil {
		return []report.Warning{{Code: WarnPool, Message: "pool log " + a.tilde(path) + ": " + err.Error()}}
	}
	return nil
}

// Pool log modes: who asked for the key.
const (
	poolModeCredential = "credential"
	poolModeToken      = "token"
)

// poolMemberToken picks a pool member and resolves its token (agx
// credential and agx login token). The warnings go to stderr.
func (a *App) poolMemberToken(ctx context.Context, name string, lg config.Login, mode string) (string, agentio.Token, []report.Warning, error) {
	rep, err := a.poolPick(ctx, name, lg, PoolPickOptions{})
	if err != nil {
		return "", agentio.Token{}, rep.Warnings, err
	}
	cfg, err := a.Config()
	if err != nil {
		return "", agentio.Token{}, rep.Warnings, err
	}
	member := rep.Selected.Name
	cred, err := login.Credential(ctx, a.exp.Brief(), a.paths, member, cfg.Config.Logins[member])
	if err != nil {
		return member, agentio.Token{}, rep.Warnings, err
	}
	return member, cred.Token, append(rep.Warnings, a.poolLog(lg, mode, rep)...), nil
}
