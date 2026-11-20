package app

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/picker"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
)

// agx usage (spec 13.1): plan usage, one row per configured home, plus
// probed rows for stored logins on request. Cost classes (14.4): live is
// N (one request per home, never a loop, may 429), --cached is R (disk
// only), every login row is Q (a probe spends a little of that plan).
//
// Why rows carry their failure instead of failing the command: one home
// being rate-limited or logged out must not hide the others; the command
// then exits 3 (partial) or 1 (nothing answered), edge 57.

// UsageOptions are the parsed flags of agx usage.
type UsageOptions struct {
	// Profiles keeps only these profiles' homes (repeatable --profile).
	Profiles []string
	// Agent keeps only this agent (--agent).
	Agent agentio.ID
	// Plan adds the plan and subscription status (--plan).
	Plan bool
	// Cached reads the vendor's disk copy only, no network (--cached).
	Cached bool
	// LoginProbes adds one probed row per stored login (--login-probes).
	LoginProbes bool
	// Logins shows these logins instead of homes (repeatable --login).
	Logins []string
	// AllLogins shows every stored login instead of homes (--all-logins).
	AllLogins bool
	// LoginPicker ticks logins in a checklist first (--login-picker, TTY).
	LoginPicker bool
	// Sort orders the rows: keys of SortKeyValues, "-" prefix descending,
	// repeated for tie-breaks (--sort).
	Sort []string
	// TimeOptions are --tz, --time-format, --date-format and
	// --duration-format: how reset times read.
	TimeOptions
}

// RowKind says what a usage row measures. Closed set.
type RowKind string

// Row kinds.
const (
	// RowHome is a configured home read with its own stored login.
	RowHome RowKind = "home"
	// RowLogin is a stored login (logins.<name>) read by a token probe.
	RowLogin RowKind = "login"
)

// RowStatus is a row's outcome. Closed set: the table's STATUS column,
// the --sort status key and the JSON "status" all use it.
type RowStatus string

// Row statuses. The first group are answers; the second are failures
// that make the command partial (exit 3) or failed (exit 1).
const (
	// StatusOK: windows read, none at its cap.
	StatusOK RowStatus = "ok"
	// StatusLimit: a window is at its cap (100%).
	StatusLimit RowStatus = "limit-reached"
	// StatusPerToken: billed per token; no windows exist (an answer).
	StatusPerToken RowStatus = "per-token"
	// StatusUnsupported: the agent models no usage (agentio's reason).
	StatusUnsupported RowStatus = "unsupported"
	// StatusNoCache: --cached and the vendor left no reading on disk.
	StatusNoCache RowStatus = "no-cache"
	// StatusStale: --cached found a reading older than StaleAfter; the
	// windows are shown, but they may be far from the truth.
	StatusStale RowStatus = "stale"

	// StatusRateLimited: the vendor answered 429 without usage headers.
	StatusRateLimited RowStatus = "rate-limited"
	// StatusExpired: the stored login is past its expiry (never refreshed).
	StatusExpired RowStatus = "expired"
	// StatusNotLoggedIn: the home holds no usable login.
	StatusNotLoggedIn RowStatus = "not-logged-in"
	// StatusRejected: the server refused the credential (401/403).
	StatusRejected RowStatus = "rejected"
	// StatusScopeLimited: the credential cannot read usage (a setup-token
	// used as a home login); probe it as a stored login instead.
	StatusScopeLimited RowStatus = "scope-limited"
	// StatusHomeMissing: the profile's home folder does not exist.
	StatusHomeMissing RowStatus = "home-missing"
	// StatusSecret: the login's token did not resolve.
	StatusSecret RowStatus = "secret"
	// StatusNetwork: the request failed (no network, timeout, DNS).
	StatusNetwork RowStatus = "network"
	// StatusError: anything else (an unreadable file, a bad response).
	StatusError RowStatus = "error"
)

// RowStatusValues lists every status in rank order (the --sort status
// key orders by this position: answers first, then failures).
func RowStatusValues() []RowStatus {
	return []RowStatus{StatusOK, StatusLimit, StatusPerToken, StatusUnsupported, StatusNoCache, StatusStale,
		StatusRateLimited, StatusExpired, StatusNotLoggedIn, StatusRejected, StatusScopeLimited,
		StatusHomeMissing, StatusSecret, StatusNetwork, StatusError}
}

// Failed reports a status that counts as a failed read for the exit code.
func (s RowStatus) Failed() bool {
	switch s {
	case StatusOK, StatusLimit, StatusPerToken, StatusUnsupported, StatusNoCache, StatusStale:
		return false
	}
	return true
}

// RowSource says where a row's windows came from. Closed set.
type RowSource string

// Row sources.
const (
	// SourceLive is the vendor's usage endpoint, now (cost N).
	SourceLive RowSource = "live"
	// SourceCached is the copy the vendor CLI left on disk (cost R).
	SourceCached RowSource = "cached"
	// SourceProbe is a token probe's rate-limit headers (cost Q).
	SourceProbe RowSource = "probe"
	// SourceNone is a row nothing was read for (unsupported, per-token
	// profile, missing home).
	SourceNone RowSource = "none"
)

// UsageRow is one row of agx usage: a home (once, with all its profiles,
// spec 6.2) or a probed login. --json data is a list of these.
type UsageRow struct {
	// Kind is home or login.
	Kind RowKind `json:"kind"`
	// Index is the row's number in the table's # column: a login's
	// 1-based position in the config file's logins, a home's the
	// position of its first profile in the file's profiles. It stays the
	// same whatever subset or --sort is shown, as in agx v0.3.
	Index int `json:"index"`
	// Name labels the row: the home's profile (the shared-home winner, or
	// the profiles joined when none wins), or the login name.
	Name string `json:"name"`
	// Agent is the agent read.
	Agent agentio.ID `json:"agent"`
	// Home is the home folder ("~/…"); "" for a login row.
	Home string `json:"home,omitempty"`
	// Profile is the shared-home winner (6.2): the default profile, else
	// the only plan profile; "" when the home has several and no winner.
	Profile string `json:"profile,omitempty"`
	// Profiles are every profile on this home, sorted.
	Profiles []string `json:"profiles,omitempty"`
	// Login is the stored login of a login row.
	Login string `json:"login,omitempty"`
	// Account is who the row reads: a login's label from config (never
	// verified), or the email of a home's login on disk (read locally,
	// shown as agx v0.3 did).
	Account string `json:"account,omitempty"`
	// PlanLabel is a home's plan as its login on disk names it ("Max
	// (20x)"); "" for a login row or when unknown.
	PlanLabel string `json:"plan_label,omitempty"`
	// Source is where the windows came from.
	Source RowSource `json:"source"`
	// ObservedAt is when the reading was taken (server time for live,
	// the vendor's fetch time for cached); nil when nothing was read.
	ObservedAt *time.Time `json:"observed_at,omitempty"`
	// Age is how old a cached reading is ("4m", "2h", "3d"); "" for live.
	Age string `json:"age,omitempty"`
	// Status is the outcome; Reason says more for anything but ok.
	Status RowStatus `json:"status"`
	Reason string    `json:"reason,omitempty"`
	// Left is 100 minus the highest window percent: what is left before
	// the first cap; nil when there are no windows (never a fake 100).
	Left *float64 `json:"left,omitempty"`
	// Windows are the usage limits, in the vendor's order.
	Windows []UsageWindow `json:"windows"`
	// Plan is the plan and subscription (--plan); nil without the flag.
	Plan *UsagePlan `json:"plan,omitempty"`
}

// UsageWindow is one usage limit.
type UsageWindow struct {
	// Class buckets the window by length: short (the 5-hour session),
	// day, week, month or unknown (agentio.WindowClass names).
	Class string `json:"class"`
	// Label is the vendor's or agentio's label ("5h", "7d", "session").
	Label string `json:"label,omitempty"`
	// Scope is the model or surface the limit is for; "" = all usage.
	Scope string `json:"scope,omitempty"`
	// Percent is how much is used.
	Percent float64 `json:"percent"`
	// ResetsAt is when it resets; nil when it has no scheduled reset.
	ResetsAt *time.Time `json:"resets_at,omitempty"`
	// Reset is ResetsAt in the --tz zone ("Tue 3 Nov 14:00").
	Reset string `json:"reset,omitempty"`
	// ID is the vendor's name of the window ("5h", "7d", "7d_oi"); ""
	// when the source names none.
	ID string `json:"id,omitempty"`
	// Status is the vendor's status of the window ("allowed",
	// "rejected"); "" when the source gives none.
	Status string `json:"status,omitempty"`
	// Binding is true when the vendor names it the window that limits
	// the account now.
	Binding bool `json:"binding,omitempty"`
}

// UsagePlan is what --plan adds to a home row.
type UsagePlan struct {
	// Plan is the plan label ("Max (20x)").
	Plan string `json:"plan,omitempty"`
	// Status is the vendor's subscription status ("active").
	Status string `json:"status,omitempty"`
	// BillingType is the vendor's billing type.
	BillingType string `json:"billing_type,omitempty"`
	// StartedAt is when the subscription started; nil when unknown.
	StartedAt *time.Time `json:"started_at,omitempty"`
	// Source is live (the vendor's account endpoint) or cached (the
	// login on disk, with --cached).
	Source RowSource `json:"source"`
	// Error is why the plan could not be read; the row's usage stands.
	Error string `json:"error,omitempty"`
}

// UsageReport is the result of agx usage.
type UsageReport struct {
	// Rows are the homes and logins, in config order or --sort order.
	Rows []UsageRow
	// Warnings are the footer: homes found on disk that no profile uses
	// (6.2), and "no profiles configured" (edge 2).
	Warnings []report.Warning
	// Loc is the --tz zone the Reset texts are in (the table's DAY column).
	Loc *time.Location
	// TimeFormat is how reset times read: --time-format, else config
	// usage.time_format, else relative (config.TimeFormatValues).
	TimeFormat string
	// DateFormat writes the clock and date parts of reset times (day,
	// absolute, both, RESETS ON): --date-format, else config
	// usage.date_format, else date_format, else the long preset.
	DateFormat datefmt.Format
	// DurationFormat is how relative times of a day or more read:
	// --duration-format, else usage.duration_format, else truncate.
	DurationFormat string
	// Now is the clock the relative times are measured from (the Host
	// clock, fixed in tests).
	Now time.Time
}

// Warning codes agx usage adds.
const (
	// WarnUnconfiguredHome names a home on disk that no profile uses.
	WarnUnconfiguredHome = "unconfigured-home"
	// WarnNoProfiles is edge 2 for a read command: nothing to show.
	WarnNoProfiles = "no-profiles"
)

// Usage reads plan usage for every configured home in scope (or the
// chosen logins). The report is returned with a partial error (exit 3)
// when some rows failed and some answered, and with a failure (exit 1)
// when every row that was read failed; a refused combination of flags is
// returned before anything is read.
func (a *App) Usage(ctx context.Context, o UsageOptions) (UsageReport, error) {
	rep := UsageReport{}
	keys, err := ParseSortKeys(o.Sort)
	if err != nil {
		return rep, err
	}
	if err := checkTimeOptions(o.TimeOptions); err != nil {
		return rep, err
	}
	if err := checkUsageFlags(o); err != nil {
		return rep, err
	}
	cfg, err := a.Config()
	if err != nil {
		return rep, err
	}
	tv, err := a.timeView(cfg.Config, o.TimeOptions)
	if err != nil {
		return rep, err
	}
	rep.Loc, rep.Now, rep.TimeFormat, rep.DateFormat, rep.DurationFormat = tv.Loc, tv.Now, tv.TimeFormat, tv.DateFormat, tv.DurationFormat
	reg, _ := a.Registry()
	ags, err := scopeAgents(reg.All(), cfg.Config, o)
	if err != nil {
		return rep, err
	}
	if len(cfg.Config.Profiles) == 0 {
		rep.Warnings = append(rep.Warnings, report.Warning{Code: WarnNoProfiles,
			Message: "no profiles configured; agx config init or agx profile import --discovered --yes"})
	}

	var jobs []usageJob
	loginsOnly := len(o.Logins) > 0 || o.AllLogins || o.LoginPicker
	if !loginsOnly {
		homeJobs, err := a.homeJobs(cfg, ags, o)
		if err != nil {
			return rep, err
		}
		jobs = append(jobs, homeJobs...)
	}
	if loginsOnly || o.LoginProbes {
		names, err := a.chosenLogins(ctx, cfg, ags, o)
		if err != nil {
			return rep, err
		}
		jobs = append(jobs, a.loginJobs(cfg, ags, names)...)
	}

	rep.Rows = a.runJobs(ctx, jobs, o)
	for i := range rep.Rows {
		finishRow(&rep.Rows[i], rep.Loc, a.Now())
	}
	SortRows(rep.Rows, keys, rep.Loc)
	rep.Warnings = append(rep.Warnings, a.unconfiguredHomes(cfg.Config, ags, o)...)
	return rep, outcome(rep.Rows)
}

// checkUsageFlags refuses two flags that choose the same thing (spec 5.1
// rule 9): which logins to show, and whether to spend quota with --cached.
func checkUsageFlags(o UsageOptions) error {
	var chosen []string
	if len(o.Logins) > 0 {
		chosen = append(chosen, "--login")
	}
	if o.AllLogins {
		chosen = append(chosen, "--all-logins")
	}
	if o.LoginPicker {
		chosen = append(chosen, "--login-picker")
	}
	if o.LoginProbes {
		chosen = append(chosen, "--login-probes")
	}
	if len(chosen) > 1 {
		return apperr.Conflict(strings.Join(chosen, " and ") + " all choose which logins to probe; pick one")
	}
	if o.Cached && len(chosen) == 1 {
		return apperr.Conflict("--cached reads disk only and " + chosen[0] + " probes logins (spends quota); pick one")
	}
	if len(o.Profiles) > 0 && len(chosen) == 1 && chosen[0] != "--login-probes" {
		return apperr.Conflict("--profile picks homes and " + chosen[0] + " shows logins instead of homes; pick one")
	}
	return nil
}

// scopeAgents is the agents in scope: --agent, else every agent. It
// refuses an unknown agent and a --profile of another agent than --agent.
func scopeAgents(all []agentio.Agent, c config.Config, o UsageOptions) ([]agentio.Agent, error) {
	for _, name := range o.Profiles {
		p, ok := c.Profiles[name]
		if !ok {
			return nil, apperr.Failure(apperr.CodeNotFound, "unknown profile "+name).WithHint("agx profile list")
		}
		if o.Agent != "" && p.Agent != o.Agent {
			return nil, apperr.Conflict(fmt.Sprintf("profile %s is a %s profile and --agent is %s; pick one", name, p.Agent, o.Agent))
		}
	}
	if o.Agent == "" {
		return all, nil
	}
	for _, ag := range all {
		if ag.ID() == o.Agent {
			return []agentio.Agent{ag}, nil
		}
	}
	return nil, apperr.Failure(apperr.CodeNotFound, "unknown agent "+string(o.Agent)).WithHint("agx agent list")
}

// usageJob is one row to read: a home with its own login, or a login.
type usageJob struct {
	row   UsageRow
	agent agentio.Agent
	// home is set for a home row.
	home agentio.Home
	// perToken: every profile on the home bills per token, so nothing is
	// requested (the answer is known).
	perToken bool
	// missing: the home folder does not exist.
	missing bool
	// login is set for a login row.
	login config.Login
}

// homeJobs builds one job per configured home in scope, each home once
// with every profile on it (6.2), in the order the config file lists the
// profiles (a home sits where its first profile does), as agx v0.3 did.
func (a *App) homeJobs(l *config.Loaded, ags []agentio.Agent, o UsageOptions) ([]usageJob, error) {
	c := l.Config
	rs := a.resolver(c)
	type homeKey struct {
		agent agentio.ID
		dir   string
	}
	byHome := map[homeKey][]string{}
	first := map[homeKey]int{}
	var homes []homeKey
	for i, name := range l.ProfileOrder() {
		p := c.Profiles[name]
		if !slices.ContainsFunc(ags, func(ag agentio.Agent) bool { return ag.ID() == p.Agent }) {
			continue
		}
		k := homeKey{p.Agent, config.ExpandPath(p.Home, a.d.Lookup, a.d.Host.UserHome)}
		if _, seen := byHome[k]; !seen {
			homes, first[k] = append(homes, k), i+1
		}
		byHome[k] = append(byHome[k], name)
	}
	var jobs []usageJob
	for _, k := range homes {
		names := slices.Sorted(slices.Values(byHome[k]))
		if len(o.Profiles) > 0 && !slices.ContainsFunc(names, func(n string) bool { return slices.Contains(o.Profiles, n) }) {
			continue
		}
		ag := agentByID(ags, k.agent)
		winner, _, err := rs.ProfileForHome(k.agent, k.dir, nil)
		if err != nil && apperr.As(err).Code != apperr.CodeAmbiguous {
			return nil, err
		}
		label := winner
		if label == "" {
			label = strings.Join(names, ",")
		}
		allAPI := true
		for _, n := range names {
			allAPI = allAPI && c.Profiles[n].Billing == config.BillingAPI
		}
		jobs = append(jobs, usageJob{
			row: UsageRow{Kind: RowHome, Index: first[k], Name: label, Agent: k.agent, Home: a.tilde(k.dir), Profile: winner, Profiles: names},
			// The home's own name is its winning profile, so pick and
			// messages name homes the way config does.
			agent: ag, home: resolve.HomeOf(ag, k.dir, label), perToken: allAPI, missing: !statDir(k.dir),
		})
	}
	return jobs, nil
}

// agentByID is the agent of ags with id (the caller took id from ags).
func agentByID(ags []agentio.Agent, id agentio.ID) agentio.Agent {
	for _, ag := range ags {
		if ag.ID() == id {
			return ag
		}
	}
	return nil
}

// resolver is a resolver over c for the shared-home rule.
func (a *App) resolver(c config.Config) *resolve.Resolver {
	return &resolve.Resolver{Cfg: &config.Loaded{Config: c}, Lookup: a.d.Lookup, UserHome: a.d.Host.UserHome, FoldOS: a.foldOS}
}

// chosenLogins are the logins to probe: --login names (checked), every
// stored login serving an agent in scope (--all-logins, --login-probes),
// or the ones ticked in the checklist (--login-picker).
func (a *App) chosenLogins(ctx context.Context, l *config.Loaded, ags []agentio.Agent, o UsageOptions) ([]string, error) {
	c := l.Config
	inScope := func(l config.Login) bool {
		return slices.ContainsFunc(ags, func(ag agentio.Agent) bool { return l.Serves(ag.ID()) })
	}
	if len(o.Logins) > 0 {
		for _, name := range o.Logins {
			l, ok := c.Logins[name]
			if !ok {
				return nil, apperr.Failure(apperr.CodeNotFound, "unknown login "+name).WithHint("agx login list")
			}
			if !inScope(l) {
				return nil, apperr.New(apperr.CodeWrongAgent, apperr.ClassUsage,
					fmt.Sprintf("login %s serves %s; --agent is %s", name, joinAgentIDs(l.Agents), o.Agent))
			}
			if l.Kind == config.KindPool {
				return nil, apperr.Usage(fmt.Sprintf("login %s is a pool: it has no token to probe; its members do", name)).
					WithHint("agx pool pick --id " + name)
			}
		}
		// Shown in config order, each once, as agx v0.3's table was.
		var out []string
		for _, name := range l.LoginOrder() {
			if slices.Contains(o.Logins, name) {
				out = append(out, name)
			}
		}
		return out, nil
	}
	var all []string
	for _, name := range l.LoginOrder() {
		// A pool has no token of its own; its members are rows already.
		if inScope(c.Logins[name]) && c.Logins[name].Kind != config.KindPool {
			all = append(all, name)
		}
	}
	if !o.LoginPicker {
		return all, nil
	}
	if a.d.Picker == nil {
		return nil, picker.NoTTY("pass --login NAME (repeat) or --all-logins")
	}
	if len(all) == 0 {
		return nil, apperr.Failure(apperr.CodeNotFound, "no stored logins to pick from").WithHint("agx login add --name NAME --agent AGENT --kind oauth-token --token-ref REF")
	}
	items := make([]picker.Item, len(all))
	for i, name := range all {
		l := c.Logins[name]
		items[i] = picker.Item{Label: name, Detail: strings.TrimSpace(joinAgentIDs(l.Agents) + " " + l.Account)}
	}
	idx, err := a.d.Picker.Checklist(ctx, "Probe which logins? (each probe spends a little of that plan)", items)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(idx))
	for _, i := range idx {
		out = append(out, all[i])
	}
	return out, nil
}

// loginJobs builds one job per login: probed through the first agent in
// scope it serves that can probe a token; when none can, the row says
// why (agentio's unsupported reason) and nothing is resolved.
func (a *App) loginJobs(lc *config.Loaded, ags []agentio.Agent, names []string) []usageJob {
	c := lc.Config
	order := lc.LoginOrder()
	jobs := make([]usageJob, 0, len(names))
	for _, name := range names {
		l := c.Logins[name]
		var serving []agentio.Agent
		for _, ag := range ags {
			if l.Serves(ag.ID()) {
				serving = append(serving, ag)
			}
		}
		ag := serving[0]
		for _, s := range serving {
			if _, ok := agentio.TokenProberOf(s); ok {
				ag = s
				break
			}
		}
		jobs = append(jobs, usageJob{
			row:   UsageRow{Kind: RowLogin, Index: slices.Index(order, name) + 1, Name: name, Agent: ag.ID(), Login: name, Account: l.Account},
			agent: ag, login: l,
		})
	}
	return jobs
}

// runJobs reads every job concurrently (one request per home or login,
// each bounded by --timeout) and returns the rows in job order. Each
// goroutine writes only its own slot and ends when its read returns.
func (a *App) runJobs(ctx context.Context, jobs []usageJob, o UsageOptions) []UsageRow {
	rows := make([]UsageRow, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Go(func() {
			rows[i] = a.readJob(ctx, j, o)
		})
	}
	wg.Wait()
	return rows
}

// outcome maps the rows to the command's error: nil when no read failed,
// partial (exit 3) when some answered, a failure (exit 1) when none did.
func outcome(rows []UsageRow) error {
	var failed []string
	network := true
	for _, r := range rows {
		if r.Status.Failed() {
			failed = append(failed, r.Name+" ("+string(r.Status)+")")
			network = network && (r.Status == StatusNetwork || r.Status == StatusRateLimited)
		}
	}
	switch {
	case len(failed) == 0:
		return nil
	case len(failed) < len(rows):
		return apperr.New(apperr.CodePartial, apperr.ClassPartial,
			fmt.Sprintf("%d of %d rows failed: %s", len(failed), len(rows), strings.Join(failed, "; ")))
	}
	code := apperr.CodeIO
	if network {
		code = apperr.CodeNetwork
	}
	return apperr.Failure(code, "no row answered: "+strings.Join(failed, "; "))
}

// unconfiguredHomes is the footer (6.2): homes of the agents in scope that
// agentio finds on disk (default home, home patterns, the home variable)
// and no profile uses. Disk only; a discovery error is skipped, since the
// footer is a hint and never decides anything.
func (a *App) unconfiguredHomes(c config.Config, ags []agentio.Agent, o UsageOptions) []report.Warning {
	if len(o.Profiles) > 0 {
		return nil
	}
	used := a.profileHomeDirs(c)
	var out []report.Warning
	for _, ag := range ags {
		homes, err := ag.Homes()
		if err != nil {
			continue
		}
		for _, h := range homes {
			if used[filepath.Clean(h.Dir())] {
				continue
			}
			out = append(out, report.Warning{Code: WarnUnconfiguredHome, Message: fmt.Sprintf(
				"%s home %s is not in config; agx profile import --discovered", ag.ID(), a.tilde(h.Dir()))})
		}
	}
	return out
}

// joinAgentIDs is "claude, pi".
func joinAgentIDs(ids []agentio.ID) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = string(id)
	}
	return strings.Join(s, ", ")
}
