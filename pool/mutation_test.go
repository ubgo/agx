//go:build mutation

package pool_test

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The mutation check, the agx side of the selector's
// scripts/mutation-check.mjs: it breaks the rules on purpose, one way at
// a time, on a temporary copy of the repository, and expects the tests of
// pool (and config, for the pool validation that lives there) to fail
// for every break. A break the tests do not notice means a rule could
// change without anyone knowing: add the missing test.
//
//	task mutation        (go test -tags mutation -run TestMutation -v ./pool/)
//
// Opt-in: it is not part of task check (each mutant compiles and runs the
// tests, a few minutes in all).

// mutant is one break: the exact text in file and its replacement. The
// tests of pool and config run for every break; args, when set, replace
// them (an app-level rule runs the internal/app tests that cover it).
type mutant struct {
	name, file, find, replace string
	args                      []string
}

// appPin runs the internal/app tests of --pool-only (the whole package
// takes too long to run once per break).
var appPin = []string{"-run", "PoolOnly", "./internal/app/"}

// mutants mirror the selector's 41 (adapted to the Go code; the
// selector's checks of JSON types are agx's typed schema, broken here by
// dropping StrictKeys), plus the agx-only rules.
var mutants = []mutant{
	{"cap check becomes exclusive (5h)", "pool/pool.go", "rd.Short.Percent >= caps.Short", "rd.Short.Percent > caps.Short", nil},
	{"cap check becomes exclusive (week)", "pool/pool.go", "rd.Week.Percent >= caps.Week", "rd.Week.Percent > caps.Week", nil},
	{"5h safety cutoff becomes exclusive", "pool/pool.go", "rd.Short.Percent >= shortUnavailableAt", "rd.Short.Percent > shortUnavailableAt", nil},
	{"enabled: false ignored", "pool/pool.go", "reasons = append(reasons, reasonDisabled)", "_ = reasonDisabled", nil},
	{"status not checked", "pool/pool.go", "if rd.Status != StatusOK {", "if false {", nil},
	{"unreadable usage allowed", "pool/pool.go", "if w.win != nil && !readable(w.win) {", "if false {", nil},
	{"unreadable usage treated as readable", "pool/pool.go", "return w != nil && !math.IsNaN(w.Percent) && !math.IsInf(w.Percent, 0)", "return w != nil", nil},
	{"every window treated as not reported", "pool/pool.go", "return w != nil && !math.IsNaN(w.Percent) && !math.IsInf(w.Percent, 0)", "return false", nil},
	{"missing account not blocked", "pool/pool.go", "return append(reasons, reasonNoReading)", "return reasons", nil},
	{"borrow window boundary exclusive", "pool/pool.go", "until >= 0 && until <= cfg.BorrowWindow", "until >= 0 && until < cfg.BorrowWindow", nil},
	{"stale shared reset counts as inside window", "pool/pool.go", "until >= 0 && until <= cfg.BorrowWindow", "until <= cfg.BorrowWindow", nil},
	{"no fallback to shared when no own is usable", "pool/pool.go", "case c.Group == GroupOwn || !ownUsable || c.inWindow:", "case c.Group == GroupOwn || c.inWindow:", nil},
	{"borrow window ignored", "pool/pool.go", "case c.Group == GroupOwn || !ownUsable || c.inWindow:", "case true:", nil},
	{"shared accounts never borrowed early", "pool/pool.go", "case c.Group == GroupOwn || !ownUsable || c.inWindow:", "case c.Group == GroupOwn || !ownUsable:", nil},
	{"ownUsable looks at shared", "pool/pool.go", "return c.Eligible && c.Group == GroupOwn })", "return c.Eligible && c.Group == GroupShared })", nil},
	{"reset order ignored", "pool/pool.go", "da, db := deadline(a), deadline(b)", "da, db := time.Duration(0), time.Duration(0)", nil},
	{"reset order reversed", "pool/pool.go", "da, db := deadline(a), deadline(b)", "da, db := deadline(b), deadline(a)", nil},
	{"tie order reversed", "pool/pool.go", "return a.order - b.order", "return b.order - a.order", nil},
	{"stale reset sorts first", "pool/pool.go", "if !c.hasWeekReset || *c.UntilReset < 0 {", "if !c.hasWeekReset {", nil},
	{"unknown reset sorts first", "pool/pool.go", "return time.Duration(math.MaxInt64)", "return 0", nil},
	{"unreadable reset counts as a deadline", "pool/pool.go", "if rd.Week.ResetsAt != nil && rd.Week.ResetsAt.IsZero() {", "if false {", nil},
	{"last of pool picked", "pool/rules.go", "for _, c := range list {", "for _, c := range slices.Backward(list) {", nil},
	{"eligible with one reason", "pool/pool.go", "c.Eligible = len(c.Reasons) == 0", "c.Eligible = len(c.Reasons) <= 1", nil},
	{"enabled read wrongly", "pool/pool.go", "return !ok || a.Enabled == nil || *a.Enabled", "return ok && a.Enabled != nil && *a.Enabled", nil},
	{"room ignores the weekly cap", "pool/rules.go", "capNow := cand.Caps.Week", "capNow := 100.0", nil},
	{"config force beats --force", "pool/pool.go", "force, by := o.Force, ForcedByFlag\n\tif force == \"\" {\n\t\tforce, by = cfg.Force, ForcedByConfig", "force, by := cfg.Force, ForcedByConfig\n\tif force == \"\" {\n\t\tforce, by = o.Force, ForcedByFlag", nil},
	{"force ignored", "pool/pool.go", "case force != \"\":", "case false:", nil},
	{"defaults override account caps (5h)", "pool/pool.go", "caps.Short = *a.Short", "caps.Short = c.Defaults.Short", nil},
	{"defaults override account caps (week)", "pool/pool.go", "caps.Week = *a.Week", "caps.Week = c.Defaults.Week", nil},
	{"defaults setting ignored", "pool/pool.go", "caps := c.Defaults", "caps := Caps{Short: DefaultShortCap, Week: DefaultWeekCap}", nil},
	{"default week cap changed", "pool/pool.go", "DefaultWeekCap = 70.0", "DefaultWeekCap = 80.0", nil},
	{"default 5h cap changed", "pool/pool.go", "DefaultShortCap = 50.0", "DefaultShortCap = 60.0", nil},
	{"default safety cutoff changed", "pool/pool.go", "DefaultShortUnavailableAt = 99.0", "DefaultShortUnavailableAt = 100.0", nil},
	{"default borrow window changed", "pool/pool.go", "DefaultBorrowWindow = 48 * time.Hour", "DefaultBorrowWindow = 24 * time.Hour", nil},
	{"cap range not checked", "pool/pool.go", "return v >= MinPercent && v <= MaxPercent && !math.IsNaN(v)", "return v >= MinPercent && v <= 10*MaxPercent && !math.IsNaN(v)", nil},
	{"NaN cap allowed", "pool/pool.go", "return v >= MinPercent && v <= MaxPercent && !math.IsNaN(v)", "return !(v < MinPercent) && !(v > MaxPercent)", nil},
	{"duplicate names allowed (pool)", "pool/pool.go", "if seen[name] {", "if false {", nil},
	{"accounts entry for an unlisted name allowed", "pool/pool.go", "if !seen[name] {", "if false {", nil},
	{"bad now allowed", "pool/pool.go", "if o.Now.IsZero() {", "if false {", nil},
	{"wait list joins the pool", "pool/pool.go", "waiting = append(waiting, c)", "ready = append(ready, c)", nil},
	{"unknown settings allowed (schema strict keys)", "config/load.go", "schema.Validate(doc, s, schema.StrictKeys(), schema.WithFileName(name))", "schema.Validate(doc, s, schema.WithFileName(name))", nil},
	{"duplicate names allowed (config)", "config/check.go", "if seen[m] {", "if false {", nil},
	{"empty force allowed", "config/check.go", "if p.Force != nil && *p.Force == \"\" {", "if false {", nil},
	{"unknown member allowed", "config/check.go", "\tcase !ok:\n\t\treturn \"unknown login \" + m", "\tcase !ok && false:\n\t\treturn \"unknown login \" + m", nil},
	{"member of another agent allowed", "config/check.go", "if !ml.Serves(id) {", "if false {", nil},
	{"null cap allowed (schema)", "config/schema.go", "n.Types = []string{k.typ}", "n.Types = []string{k.typ, SchemaNull}", nil},
	{"cap range left out of the schema", "config/schema.go", "n.Maximum = jsonNumber(pool.MaxPercent)", "n.Maximum = jsonNumber(10 * pool.MaxPercent)", nil},
	// --pool-only: one member, its caps enforced.
	{"only-set ignored", "pool/pool.go", "if o.Only != \"\" {\n\t\tpinned, err := onlyConfig(cfg, o)", "if false {\n\t\tpinned, err := onlyConfig(cfg, o)", nil},
	{"caps not enforced under only", "pool/pool.go", "{name: {Short: &caps.Short, Week: &caps.Week,", "{name: {Short: ptr(MaxPercent), Week: ptr(MaxPercent),", nil},
	{"disabled ignored under only", "pool/pool.go", "Week: &caps.Week, Enabled: c.Accounts[name].Enabled, WeekRelaxMax:", "Week: &caps.Week, Enabled: nil, WeekRelaxMax:", nil},
	{"config force wins under only", "pool/pool.go", "out.Own, out.Shared, out.Force = nil, nil, \"\"", "out.Own, out.Shared = nil, nil", nil},
	{"only and force allowed together", "pool/pool.go", "if o.Force != \"\" {\n\t\treturn Config{}, ErrOnlyAndForce", "if false {\n\t\treturn Config{}, ErrOnlyAndForce", nil},
	{"only allows a non-member", "pool/pool.go", "if !slices.Contains(cfg.Own, o.Only) && !slices.Contains(cfg.Shared, o.Only) {", "if false {", nil},
	{"tighter caps ignored", "pool/pool.go", "*w.cap = *w.v", "_ = *w.v", nil},
	{"tighter cap may raise", "pool/pool.go", "case *w.v > *w.cap:", "case false:", nil},
	{"tighter caps without only allowed", "pool/pool.go", "} else if o.OnlyCaps.Given() {", "} else if false {", nil},
	{"picker used under a pin", "internal/app/pool.go", "case lg.Pool.Picker != nil && o.Only.Member == \"\":", "case lg.Pool.Picker != nil:", appPin},
	{"--pool-only with --pool-force allowed", "internal/app/pool_only.go", "if force != \"\" {\n\t\treturn apperr.Conflict(", "if false {\n\t\treturn apperr.Conflict(", appPin},
	{"pin dropped by agx credential", "internal/app/livecmd.go", "poolModeCredential, st.PoolForce, only, f.ID)", "poolModeCredential, st.PoolForce, PoolOnly{}, f.ID)", appPin},
	{"higher run cap allowed at launch", "internal/app/pool_only.go", "case *c.v > c.cap:", "case false:", appPin},
	// The rule values (pool/rules.go) and their wiring.
	{"weekly relax ignored", "pool/rules.go", "cand.Caps.Week = week", "cand.Caps.Week = configured", nil},
	{"relax not linear", "pool/rules.go", "gone := 1 - until.Seconds()/window.Seconds()", "gone := 1.0", nil},
	{"stale reset relaxed", "pool/rules.go", "if window <= 0 || until < 0 || until >= window", "if window <= 0 || until >= window", nil},
	{"relax may lower a higher cap", "pool/rules.go", "until >= window || relaxMax <= capWeek {", "until >= window {", nil},
	{"relax outside the window", "pool/rules.go", "until >= window || relaxMax", "until >= 10*window || relaxMax", nil},
	{"member relax max ignored", "pool/rules.go", "if a, ok := c.Accounts[name]; ok && a.WeekRelaxMax != nil {", "if a, ok := c.Accounts[name]; false && ok && a.WeekRelaxMax != nil {", nil},
	{"group relax max ignored", "pool/rules.go", "if g, ok := c.Groups[c.groupOf(name)]; ok && g.WeekRelaxMax != nil {", "if g, ok := c.Groups[c.groupOf(name)]; false && ok && g.WeekRelaxMax != nil {", nil},
	{"group caps ignored", "pool/pool.go", "if g, ok := c.Groups[group]; ok {", "if g, ok := c.Groups[GroupUnlisted]; ok {", nil},
	{"urgency order reversed", "pool/rules.go", "case ua > ub:\n\t\treturn -1", "case ua < ub:\n\t\treturn -1", nil},
	{"unknown urgency sorts first", "pool/rules.go", "if !c.hasUrgency {\n\t\treturn -1", "if !c.hasUrgency {\n\t\treturn math.MaxFloat64", nil},
	{"minutes to the reset divide by zero", "pool/rules.go", "math.Max(window.until.Hours(), minUrgencyHours)", "window.until.Hours()", nil},
	{"urgency ignores the relaxed cap", "pool/rules.go", "capNow := cand.Caps.Week", "capNow := 70.0", nil},
	{"urgency order ignored", "pool/pool.go", "order := byUrgency", "order := byDeadline", nil},
	{"deadline order ignored", "pool/pool.go", "if res.Order == OrderDeadline {\n\t\torder = byDeadline", "if false {\n\t\torder = byDeadline", nil},
	{"no pick per session", "pool/pool.go", "case len(ready) > 0 && o.Only == \"\":", "case false:", nil},
	{"sticky ignored", "pool/rules.go", "if cfg.Sticky && o.Current != \"\" {", "if false {", nil},
	{"sticky ignores max_sessions", "pool/rules.go", "(limit == 0 || o.CurrentAhead < limit)", "true", nil},
	{"sticky keeps a waiting member", "pool/rules.go", "if i := slices.IndexFunc(ready, func(c *Candidate) bool { return c.Name == o.Current });", "if i := slices.IndexFunc(append(ready, waiting...), func(c *Candidate) bool { return c.Name == o.Current });", nil},
	{"max_sessions ignored", "pool/rules.go", "return limit > 0 && c.Load >= limit", "return false", nil},
	{"max_sessions exclusive", "pool/rules.go", "c.Load >= limit", "c.Load > limit", nil},
	{"no overflow to waiting members", "pool/rules.go", "[][]*Candidate{ready, waiting}", "[][]*Candidate{ready}", nil},
	{"most loaded when all are full", "pool/rules.go", "if c.Load < least.Load {", "if c.Load > least.Load {", nil},
	{"one-run weekly cap relaxed", "pool/pool.go", "a.WeekRelaxMax = o.OnlyCaps.Week", "a.WeekRelaxMax = nil", nil},
	{"sticky never leaves", "pool/rules.go", "if !outranked(cfg, ready[j], ready[i]) {", "if true {", nil},
	{"sticky leave ratio ignored", "pool/rules.go", "return best.urgency >= cfg.StickyLeaveRatio*curUrgency", "_ = curUrgency; return true", nil},
	{"negative leave ratio allowed", "pool/rules.go", "if c.StickyLeaveRatio < 0 || math.IsNaN(c.StickyLeaveRatio) || math.IsInf(c.StickyLeaveRatio, 0) {", "if false {", nil},
	{"config leave ratio ignored", "config/pool.go", "c.StickyLeaveRatio = *p.StickyLeaveRatio", "_ = p.StickyLeaveRatio", nil},
	{"default leave ratio changed", "pool/pool.go", "DefaultStickyLeaveRatio = 3.0", "DefaultStickyLeaveRatio = 0.0", nil},
	{"capacity ignored", "pool/rules.go", "cand.urgency = needs * c.CapacityFor(cand.Name)", "cand.urgency = needs", nil},
	{"unset capacity is not 1", "pool/rules.go", "return *a.Capacity\n\t}\n\treturn 1", "return *a.Capacity\n\t}\n\treturn 2", nil},
	{"zero capacity allowed", "pool/pool.go", "if a.Capacity != nil && (*a.Capacity <= 0 ||", "if a.Capacity != nil && (*a.Capacity < 0 ||", nil},
	{"config capacity ignored", "config/pool.go", "WeekRelaxMax: a.WeekRelaxMax, Capacity: a.Capacity}", "WeekRelaxMax: a.WeekRelaxMax}", nil},
	{"unknown order allowed", "pool/rules.go", "if c.Order != \"\" && !slices.Contains(OrderValues(), string(c.Order)) {", "if false {", nil},
	{"negative max_sessions allowed", "pool/rules.go", "if c.MaxSessions < 0 {", "if false {", nil},
	{"bad group allowed", "pool/rules.go", "if g != GroupOwn && g != GroupShared {", "if false {", nil},
	{"default order changed", "pool/pool.go", "DefaultOrder = OrderUrgency", "DefaultOrder = OrderDeadline", nil},
	{"default own short cap changed", "pool/pool.go", "DefaultOwnShortCap = 95.0", "DefaultOwnShortCap = 50.0", nil},
	{"default own week cap changed", "pool/pool.go", "DefaultOwnWeekCap  = 98.0", "DefaultOwnWeekCap  = 70.0", nil},
	{"own members take the shared defaults", "pool/pool.go", "caps = c.OwnDefaults", "caps = c.Defaults", nil},
	{"classic preset is not classic", "pool/rules.go", "classic.Order, classic.WeekRelaxWindow, classic.MaxSessions, classic.Sticky, classic.StickyLeaveRatio = OrderDeadline, 0, 0, false, 0", "classic.Order = OrderDeadline", nil},
	{"default relax window changed", "pool/pool.go", "DefaultWeekRelaxWindow = 24 * time.Hour", "DefaultWeekRelaxWindow = 48 * time.Hour", nil},
	{"default relax max changed", "pool/pool.go", "DefaultWeekRelaxMax = 95.0", "DefaultWeekRelaxMax = 90.0", nil},
	{"default max_sessions changed", "pool/pool.go", "DefaultMaxSessions = 4", "DefaultMaxSessions = 3", nil},
	{"default sticky changed", "pool/pool.go", "DefaultSticky = true", "DefaultSticky = false", nil},
	{"config order ignored", "config/pool.go", "c.Order = pool.Order(p.Order)", "_ = p.Order", nil},
	{"empty probe model allowed", "config/pool.go", "if strings.TrimSpace(m) == \"\" {", "if _ = m; false {", appModels},
	// Per-model windows (pool/scoped.go) and their wiring.
	{"scoped window ignored", "pool/pool.go", "c.Reasons = append(c.Reasons, scopedReasons(c.Scoped)...)", "_ = scopedReasons", nil},
	{"scoped cap exclusive", "pool/scoped.go", "case *s.Percent >= s.Cap:", "case *s.Percent > s.Cap:", nil},
	{"rejected scoped window usable", "pool/scoped.go", "case s.Rejected:", "case false && s.Rejected:", nil},
	{"stale scoped window blocks", "pool/scoped.go", "s.Stale = until < 0", "s.Stale = false", nil},
	{"scoped window not relaxed", "pool/scoped.go", "s.Cap = RelaxedWeek(configured, relaxMax, c.WeekRelaxWindow, until)", "s.Cap = configured; _ = relaxMax", nil},
	{"unreadable scoped window usable", "pool/scoped.go", "out = append(out, fmt.Sprintf(unreadableFmt, s.Name))", "_ = s.Name", nil},
	{"scoped room ignored", "pool/rules.go", "if r := math.Max(0, w.upTo-*s.Percent); r < ahead {", "if r := math.Max(0, w.upTo-*s.Percent); false && r < ahead {", nil},
	{"scoped room left out of the room now", "pool/rules.go", "room = math.Min(room, s.Room)", "_ = s.Room", nil},
	{"probe model ignored", "internal/app/pool_models.go", "u, err := mp.TokenUsageModel(ctx, cred, model)", "_ = mp; u, err := pr.TokenUsage(ctx, cred)", appModels},
	{"run model not read", "internal/app/pool.go", "o.Model = st.Model", "_ = st.Model", appModels},
	{"probe_models ignored", "internal/app/pool_models.go", "return slices.Clone(p.ProbeModels), modelSrcPool", "return nil, modelSrcPool", appModels},
	{"default profile model ignored", "internal/app/pool_models.go", "return p.Model, modelSrcProfile + names[0]", "return \"\", \"\"", appModels},
	{"scoped windows not handed to the rules", "internal/app/pool_models.go", "r.Scoped = append(r.Scoped,", "_ = append(r.Scoped,", appModels},
	{"refused model blocks the pool", "internal/app/pool_models.go", "if errors.Is(err, agentio.ErrModelRefused) {", "if _ = errors.Is; false {", appModels},
	{"full scoped window limits every model", "internal/app/usage_read.go", "full = full || (w.Scope == \"\" && w.Percent >= percentFull)", "full = full || w.Percent >= percentFull", appModels},
	{"cache drops other models' windows", "login/usagecache.go", "for k, v := range old.Models {", "for k, v := range map[string]ModelWindows{} {", appModels},
	{"launch model not recorded", "internal/app/live.go", "p.Live.Model, p.liveModelSrc = runModel(o, res)", "_, p.liveModelSrc = runModel(o, res)", appModels},
	// The usage history (login/usagehistory.go).
	{"history gap ignored", "login/usagehistory.go", "(p.At.Sub(last.At) < HistoryMinGap && !resetsChanged(last, p))", "false", loginHistory},
	{"history age unbounded", "login/usagehistory.go", "cut < len(pts) && p.At.Sub(pts[cut].At) > HistoryMaxAge", "false", loginHistory},
	{"history count unbounded", "login/usagehistory.go", "cut = max(cut, len(pts)-HistoryMaxPoints)", "_ = HistoryMaxPoints", loginHistory},
	{"history records failures", "login/usagehistory.go", "if len(e.Windows) == 0 {", "if false {", loginHistory},
	{"history not written", "login/usagecache.go", "_ = recordHistory(paths, entries)", "_ = entries", loginHistory},
	// Presets and the reported sources.
	{"preset writes no order", "internal/app/pool_preset.go", "Order:            str(string(r.Order)),", "Order:            nil,", appPreset},
	{"group source reported as pool", "internal/app/pool_info.go", "{group.Short, SourceGroup, groupKey + \".short\"}", "{group.Short, SourcePool, groupKey + \".short\"}", appPreset},
	{"own members show the pool defaults", "internal/app/pool_info.go", "\t\t\tdef = pool.DefaultOwnShortCap\n\t\t} else {", "\t\t\tdef = pool.DefaultOwnShortCap\n\t\t}\n\t\t{", appPreset},
	{"config max_sessions ignored", "config/pool.go", "c.MaxSessions = *p.MaxSessions", "_ = p.MaxSessions", nil},
	{"config group caps ignored", "config/pool.go", "c.Groups[g] = pool.GroupCaps{Short: gc.Short, Week: gc.Week, WeekRelaxMax: gc.WeekRelaxMax}", "_, _ = gc, g", nil},
	{"live runs not counted", "internal/app/pool.go", "sel.Load, sel.Current, sel.CurrentAhead = a.poolLoad(ctx, name, lg, o.RunID, now, rep.warn)", "_, _, _ = a.poolLoad(ctx, name, lg, o.RunID, now, rep.warn)", appRules},
	{"idle runs counted", "internal/app/pool.go", "if now.Sub(st.InUseAt) <= activeRunTTLs*ttl {", "if _ = ttl; true {", appRules},
	{"credential run not passed", "internal/app/livecmd.go", "poolModeCredential, st.PoolForce, only, f.ID)", "poolModeCredential, st.PoolForce, only, \"\")", appRules},
	// The simulator's regression: the defaults must keep their gains.
	{"spread off in the simulated pool", "pool/rules.go", "full := func(c *Candidate) bool { return limit > 0 && c.Load >= limit }", "full := func(c *Candidate) bool { return false }", simRegression},
	{"relax off in the simulated pool", "pool/pool.go", "if found {\n\t\tcfg.relax(c, rd, now)", "if found {\n\t\t_ = cfg.relax", simRegression},
	{"per-model windows off in the simulated pool", "internal/poolsim/policy.go", "if pp.blind || !s.Scoped {", "if true {", simRegression},
	// Schedules and overrides (schedule.go).
	{"Select ignores schedules and overrides", "pool/pool.go", "cfg = cfg.At(o.Now)", "cfg = cfg.At(time.Time{})", nil},
	{"group schedule beats member schedule", "pool/schedule.go", "[][3]timedCap{ovr, byName, byGroup}", "[][3]timedCap{ovr, byGroup, byName}", nil},
	{"schedule beats override", "pool/schedule.go", "[][3]timedCap{ovr, byName, byGroup}", "[][3]timedCap{byName, byGroup, ovr}", nil},
	{"least permissive schedule wins", "pool/schedule.go", "if v == nil || (t.v != nil && *v <= *t.v) {", "if v == nil || (t.v != nil && *v >= *t.v) {", nil},
	{"ended override still applies", "pool/schedule.go", "if o.Active(now) {", "if true {", nil},
	{"override end inclusive", "pool/schedule.go", "func (o Override) Active(now time.Time) bool { return now.Before(o.Until) }", "func (o Override) Active(now time.Time) bool { return !now.After(o.Until) }", nil},
	{"release_before ignored", "pool/schedule.go", "until := sp.to.Add(-s.ReleaseBefore)", "until := sp.to", nil},
	{"window across midnight ends the same day", "pool/schedule.go", "if s.End <= s.Start {\n\t\t\tendDay++", "if false {\n\t\t\tendDay++", nil},
	{"schedule zone ignored", "pool/schedule.go", "if s.Location != nil {\n\t\tloc = s.Location", "if false {\n\t\tloc = s.Location", nil},
	{"back-to-back windows not joined", "pool/schedule.go", "if !sp.from.After(last.to) {", "if sp.from.Before(last.to) {", nil},
	{"group schedule applies to every group", "pool/schedule.go", "byGroup = byGroup || Group(m) == g", "byGroup = true", nil},
	{"schedule names an unknown member", "pool/schedule.go", "if m != string(GroupOwn) && m != string(GroupShared) && !members[m] {", "if false {", nil},
	{"schedule caps unchecked", "pool/schedule.go", "if err := validateCaps(label+\" caps\", s.Caps); err != nil {", "if err := validateCaps(label+\" caps\", s.Caps); false && err != nil {", nil},
	{"--until in the past allowed", "pool/schedule.go", "if !t.After(now) {", "if false {", nil},
	{"default release_before dropped", "config/pool.go", "release = pool.DefaultScheduleReleaseBefore", "release = 0", nil},
	{"schedule timezone ignored", "config/pool.go", "c.Location = loc", "_ = loc", nil},
	// The order of v0.4.20 (rules.go): the room up to the cap at the
	// reset, the deadline group, the rate shown, and the review's fixes.
	{"urgency counts the cap now only", "pool/rules.go", "upTo = math.Max(capNow, final)", "upTo = capNow", nil},
	{"rise counted before the relax window opens", "pool/rules.go", "if window <= 0 || until < 0 || until > window || relaxMax <= capWeek {", "if window <= 0 || until < 0 || relaxMax <= capWeek {", nil},
	{"relax window's end exclusive for the count", "pool/rules.go", "if window <= 0 || until < 0 || until > window || relaxMax <= capWeek {", "if window <= 0 || until < 0 || until >= window || relaxMax <= capWeek {", nil},
	{"scoped window counted to its cap now", "pool/scoped.go", "s.finalCap = c.finalCap(name, *w.ResetsAt, until)", "s.finalCap = s.Cap", nil},
	{"timed caps at the reset ignored", "pool/pool.go", "cfg.atReset = base.capsAt(o)", "cfg.atReset = nil; _ = base", nil},
	{"pinned weekly cap lifted at the reset", "pool/pool.go", "week, relaxMax = math.Min(week, *limit), math.Min(relaxMax, *limit)", "_ = limit", nil},
	{"deadline group ignored", "pool/rules.go", "cand.Deadline = cand.Eligible && c.order() == OrderUrgency", "cand.Deadline = false && cand.Eligible && c.order() == OrderUrgency", nil},
	{"blocked member in the deadline group", "pool/rules.go", "cand.Deadline = cand.Eligible && c.order() == OrderUrgency", "cand.Deadline = c.order() == OrderUrgency", nil},
	{"deadline group under the deadline order", "pool/rules.go", "cand.Deadline = cand.Eligible && c.order() == OrderUrgency", "cand.Deadline = cand.Eligible", nil},
	{"deadline window 0 not off", "pool/rules.go", "c.DeadlineWindow > 0 && cand.hasUrgency", "cand.hasUrgency", nil},
	{"deadline window exclusive", "pool/rules.go", "*cand.window.until <= c.DeadlineWindow", "*cand.window.until < c.DeadlineWindow", nil},
	{"deadline min room ignored", "pool/rules.go", "cand.urgencyRoom >= c.DeadlineMinRoom", "cand.urgencyRoom >= 0", nil},
	{"deadline min room exclusive", "pool/rules.go", "cand.urgencyRoom >= c.DeadlineMinRoom", "cand.urgencyRoom > c.DeadlineMinRoom", nil},
	{"deadline group sorted latest first", "pool/rules.go", "cmp.Compare(*a.window.until, *b.window.until)", "cmp.Compare(*b.window.until, *a.window.until)", nil},
	{"deadline group after the rest", "pool/rules.go", "case a.Deadline && !b.Deadline:\n\t\treturn -1", "case a.Deadline && !b.Deadline:\n\t\treturn 1", nil},
	{"deadline tie order reversed", "pool/rules.go", "c != 0 {\n\t\t\treturn c\n\t\t}\n\t\treturn a.order - b.order", "c != 0 {\n\t\t\treturn c\n\t\t}\n\t\treturn b.order - a.order", nil},
	{"negative deadline window allowed", "pool/rules.go", "if c.DeadlineWindow < 0 {", "if false {", nil},
	{"deadline min room range unchecked", "pool/pool.go", ", {\"deadline_min_room\", c.DeadlineMinRoom}", "", nil},
	{"default deadline window changed", "pool/pool.go", "DefaultDeadlineWindow = 24 * time.Hour", "DefaultDeadlineWindow = 12 * time.Hour", nil},
	{"default deadline min room changed", "pool/pool.go", "DefaultDeadlineMinRoom = 2.0", "DefaultDeadlineMinRoom = 0.0", nil},
	{"classic preset keeps a deadline window", "pool/rules.go", "classic.DeadlineWindow = 0", "_ = 0", nil},
	{"config deadline window ignored", "config/pool.go", "c.DeadlineWindow = d", "_ = d", nil},
	{"config deadline min room ignored", "config/pool.go", "c.DeadlineMinRoom = *p.DeadlineMinRoom", "_ = p.DeadlineMinRoom", nil},
	{"needs rate weighed by capacity", "pool/rules.go", "cand.NeedsPerHour = ptr(perHour(needs))", "cand.NeedsPerHour = ptr(perHour(cand.urgency))", nil},
	{"real room shown as 0%/h", "pool/rules.go", "if v > 0 && r < minShownRate {", "if false {", nil},
	{"sticky leaves for a full leader", "pool/rules.go", "j := slices.IndexFunc(ready, func(c *Candidate) bool { return c == ready[i] || !full(c) })", "j := 0", nil},
	{"scoped window without a reset left out", "pool/rules.go", "if s.Stale || s.unreadable {\n\t\t\t// It has reset", "if s.Stale || s.unreadable || s.until == nil {\n\t\t\t// It has reset", nil},
	{"picker warnings without the relax", "pool/pool.go", "return candidate(at, name, at.groupOf(name), 0, rd, found, now).Reasons", "_ = at; return blockReasons(rd, found, c.Enabled(name), c.CapsFor(name), c.ShortUnavailableAt)", nil},
	{"unknown reset after a full member says none has one", "pool/rules.go", "case c.Urgency == nil && first:", "case c.Urgency == nil:", nil},
	{"folded config refolds its timed caps", "pool/schedule.go", "if c.normal != nil {\n\t\tc = *c.normal", "if false {\n\t\tc = *c.normal", nil},
	{"higher cap ending before the reset ignored", "pool/rules.go", "if end, src, ok := cand.timedWeekEnd(); ok && capNow > final {", "if end, src, ok := cand.timedWeekEnd(); false && ok && capNow > final {", nil},
	{"deadline not in the report", "internal/app/pool.go", "NeedsPercentPerHour: c.NeedsPerHour, Deadline: c.Deadline,", "NeedsPercentPerHour: c.NeedsPerHour,", appDeadline},
	{"room at the reset not in the report", "internal/app/pool.go", "RoomAtResetPercent: c.RoomAtReset,", "", appDeadline},
	{"config deadline keys not shown", "internal/app/pool_info.go", "addDuration(poolKeyDeadline, p.DeadlineWindow, pool.DefaultDeadlineWindow)", "addDuration(poolKeyDeadline, \"\", pool.DefaultDeadlineWindow)", appPreset},
	{"preset writes no deadline window", "internal/app/pool_preset.go", "DeadlineWindow:   str(configDuration(r.DeadlineWindow)),", "DeadlineWindow:   nil,", appPreset},
	// A schedule's week_before_reset (schedule.go, rules.go, scoped.go).
	{"week_before_reset ignored", "pool/rules.go", "ok && v > week {", "false && ok && v > week {", nil},
	{"week_before_reset lowers a higher cap", "pool/rules.go", "ok && v > week {", "ok {", nil},
	{"week_before_reset note dropped", "pool/rules.go", "cand.Notes = append(cand.Notes, fmt.Sprintf(noteBeforeReset, num(v), src.Name))", "_ = noteBeforeReset", nil},
	{"week_before_reset not named as the weekly cap's source", "pool/rules.go", "t.Week = &src", "_ = src", nil},
	{"week_before_reset source not marked", "pool/schedule.go", "src.BeforeReset = true", "_ = 0", nil},
	{"reset after the schedule's end covered", "pool/schedule.go", "return !reset.Before(now) && (src.Until == nil || !reset.After(*src.Until))", "return !reset.Before(now)", nil},
	{"reset at the schedule's end not covered", "pool/schedule.go", "(src.Until == nil || !reset.After(*src.Until))", "(src.Until == nil || reset.Before(*src.Until))", nil},
	{"passed reset covered", "pool/schedule.go", "return !reset.Before(now) && (src.Until == nil", "return (src.Until == nil", nil},
	{"never-ending schedule covers nothing", "pool/schedule.go", "(src.Until == nil || !reset.After(*src.Until))", "(src.Until != nil && !reset.After(*src.Until))", nil},
	{"group level beats member level", "pool/schedule.go", "for _, level := range [][]resetCap{r.byName, r.byGroup} {\n\t\tbest := -1", "for _, level := range [][]resetCap{r.byGroup, r.byName} {\n\t\tbest := -1", nil},
	{"lowest value wins at one level", "pool/schedule.go", "(best < 0 || f.v > level[best].v)", "(best < 0 || f.v < level[best].v)", nil},
	{"tie goes to the last schedule", "pool/schedule.go", "(best < 0 || f.v > level[best].v)", "(best < 0 || f.v >= level[best].v)", nil},
	{"override's weekly cap does not win", "pool/schedule.go", "if ovr[1].v == nil && floors.given() {", "if floors.given() {", nil},
	{"week_before_reset kept with no schedule naming", "pool/schedule.go", "level, floorLevel := &byGroup, &floors.byGroup", "level, floorLevel := &byGroup, &floors.byName", nil},
	{"per-model window not raised", "pool/scoped.go", "s.Cap = math.Max(s.Cap, v)", "_ = v", nil},
	{"pinned weekly cap raised by week_before_reset", "pool/pool.go", "cfg.limitBeforeReset(o.Only, *o.OnlyCaps.Week)", "_ = o.OnlyCaps.Week", nil},
	{"pinned limit raises instead of lowering", "pool/schedule.go", "level[i].v = math.Min(level[i].v, limit)", "level[i].v = math.Max(level[i].v, limit)", nil},
	{"week_before_reset range unchecked", "pool/schedule.go", "case s.WeekBeforeReset != nil && !validPercent(*s.WeekBeforeReset):", "case false:", nil},
	{"schedule with only week_before_reset refused", "pool/schedule.go", "case !s.Caps.given() && s.WeekBeforeReset == nil:", "case !s.Caps.given():", nil},
	{"config drops week_before_reset", "config/pool.go", "WeekBeforeReset: s.Caps.WeekBeforeReset,", "", nil},
	{"schema takes a null week_before_reset", "config/schema.go", "{[]string{\"schedules\", schemaItems, \"caps\", \"week_before_reset\"}, SchemaNumber, true},", "", nil},
	{"--pool-schedule drops week_before_reset", "internal/app/login_pool_timed.go", "capField(schedKeyBeforeReset, func(s *config.PoolSchedule) **float64 { return &s.Caps.WeekBeforeReset }),", "", appSchedule},
	{"--pool-schedule change line without the caps", "internal/app/login_pool_timed.go", "text: func(s *config.PoolSchedule) string { return optPercent(*slot(s)) },", "text: func(*config.PoolSchedule) string { return \"\" },", appSchedule},
	// --pool-schedule merges into the schedule of that name (v0.4.22).
	{"--pool-schedule replaces instead of merging", "internal/app/login_pool_timed.go", "\t\t\tbase = next[i]", "\t\t\t_ = next[i]", appSchedule},
	{"--pool-schedule-replace merges", "internal/app/login_pool_timed.go", "if i >= 0 && e.flag == flagPoolSchedule {", "if i >= 0 {", appSchedule},
	{"cap= keeps the cap", "internal/app/login_pool_timed.go", "\t\t\tif v == \"\" {\n\t\t\t\t*slot(s) = nil\n\t\t\t\treturn nil", "\t\t\tif v == \"\" {\n\t\t\t\treturn nil", appSchedule},
	{"list key= keeps the list", "internal/app/login_pool_timed.go", "\t\t\t*slot(s) = nil\n\t\t\tif v != \"\" {", "\t\t\tif v != \"\" {", appSchedule},
	{"merged schedule not checked", "internal/app/login_pool_timed.go", "if err := e.checkSchedule(sc, members); err != nil {", "if err := e.checkSchedule(sc, members); false && err != nil {", appSchedule},
	{"removal not named in the error", "internal/app/login_pool_timed.go", "if e.removes(key) {", "if false {", appSchedule},
	{"cap removal not named in the error", "internal/app/login_pool_timed.go", "if e.removes(k) {", "if false {", appSchedule},
	{"schedule named twice allowed", "internal/app/login_pool_timed.go", "if first, ok := seen[name]; ok {", "if first, ok := seen[name]; false && ok {", appSchedule},
	{"removed schedule has no change lines", "internal/app/login_pool_timed.go", "range [][]config.PoolSchedule{before, after} {", "range [][]config.PoolSchedule{after} {", appSchedule},
	{"merged schedules not written", "internal/app/login_pool_timed.go", "p.edits = append(p.edits, setValue(p.at(keyPoolSchedules), next))", "_ = next", appSchedule},
	{"--pool-schedule-remove keeps the schedule", "internal/app/login_pool_timed.go", "\t\tnext = slices.Delete(next, i, i+1)", "\t\t_ = i", appTimed},
	{"--pool-schedule-remove of an unknown name allowed", "internal/app/login_pool_timed.go", "\t\tif i < 0 {\n\t\t\treturn apperr.Usage(fmt.Sprintf(\"%s %s: pool %s has no schedule", "\t\tif false {\n\t\t\treturn apperr.Usage(fmt.Sprintf(\"%s %s: pool %s has no schedule", appTimed},
	// Overrides that end, --pool-clear-override and agx pool prune.
	{"--pool-clear-override keeps the override", "internal/app/login_pool_settings.go", "a := touch(n)\n\t\ta.Override = nil", "a := touch(n)\n\t\t_ = a.Override", appTimed},
	{"prune removes overrides that still apply", "internal/app/pool_prune.go", "err == nil && !r.Active(now) {", "err == nil && (true || !r.Active(now)) {", appTimed},
	{"prune keeps ended overrides", "internal/app/pool_prune.go", "ended = append(ended, m)", "_ = m", appTimed},
	{"active override starts over on --until", "internal/app/login_pool_timed.go", "r.Active(p.now) {\n\t\t\to = *a.Override", "r.Active(p.now) {\n\t\t\t_ = *a.Override", appTimed},
	// The pool log line (pool.log).
	{"pool log drops the picked usage", "internal/app/pool.go", "line += \" usage=\" + quote(usage)", "_ = usage", appModels},
	{"pool show leaves probe_models at its default", "internal/app/pool_info.go", "if len(p.ProbeModels) > 0 {\n\t\tprobe, probeSrc", "if false {\n\t\tprobe, probeSrc", appPreset},
	{"pool log drops the probe models", "internal/app/pool.go", "line += \" models=\" + quote(strings.Join(rep.ProbeModels, \",\"))", "_ = rep.ProbeModels", appModels},
	// Live sessions (run_liveness.go).
	{"ended runs counted as sessions", "internal/app/pool.go", "if lives[s.id].liveness == agentio.LivenessEnded {", "if _ = lives; false {", appLiveness},
	{"one process-table read per run folder", "internal/app/run_liveness.go", "if lives, err := a.d.Host.ProcessesAlive(ctx, refs); err == nil {", "if lives, err := a.d.Host.ProcessesAlive(ctx, refs); false && err == nil {", appLiveness},
	// No usable key (v0.4.23): when_none_usable, the refusal record, the
	// status line and the run's label.
	{"stop answered like keep", "internal/app/livecmd.go", "if rf.mode == config.WhenNoneUsableKeep {", "if true {", appNoKey},
	{"keep answered like stop", "internal/app/livecmd.go", "if rf.mode == config.WhenNoneUsableKeep {", "if false {", appNoKey},
	{"refusal not recorded", "internal/app/livecmd.go", "a.recordNoKey(ctx, rf.dir, why, rf.brief, rf.mode)", "_ = why", appNoKey},
	{"pool when_none_usable ignored", "internal/app/livecmd.go", "ok && lg.Pool != nil && lg.Pool.WhenNoneUsable != \"\" {", "ok && lg.Pool != nil && false {", appNoKey},
	{"live.when_none_usable ignored", "internal/app/livecmd.go", "if c.Live.WhenNoneUsable != \"\" {", "if false {", appNoKey},
	{"default becomes keep", "internal/app/livecmd.go", "return config.WhenNoneUsableStop, SourceDefault", "return config.WhenNoneUsableKeep, SourceDefault", appNoKey},
	{"unknown setting answered with keep", "internal/app/livecmd.go", "mode: config.WhenNoneUsableStop, brief: briefRequestFailed}", "mode: config.WhenNoneUsableKeep, brief: briefRequestFailed}", appNoKey},
	{"all over caps said of a mixed pool", "internal/app/livecmd.go", "if !slices.ContainsFunc(c.Reasons, isCapReason) {", "if _ = slices.ContainsFunc(c.Reasons, isCapReason); false {", appNoKey},
	{"refusal brief by code dropped", "internal/app/livecmd.go", "case apperr.CodePerUseKey:\n\t\treturn briefAPIKey", "case apperr.CodePerUseKey:\n\t\treturn briefRequestFailed", appNoKey},
	{"no-key since moves with every refusal", "live/live.go", "if s.NoKeySince.IsZero() {", "if true {", liveState},
	{"stop leaves the old login in use", "live/live.go", "if mode != config.WhenNoneUsableKeep && s.InUse != \"\" {", "if false {", liveState},
	{"keep clears the login in use", "live/live.go", "if mode != config.WhenNoneUsableKeep && s.InUse != \"\" {", "if s.InUse != \"\" {", liveState},
	{"a key does not end the refusal", "live/live.go", "s.NoKey, s.NoKeyBrief, s.NoKeySince, s.WhenNoneUsable = \"\", \"\", time.Time{}, \"\"", "s.NoKeyBrief = \"\"", liveState},
	{"a refusal moves the last key time", "live/live.go", "s.InUseAt, s.NoKey, s.NoKeyBrief, s.WhenNoneUsable = now, why, brief, mode", "s.InUseAt, s.NoKey, s.NoKeyBrief, s.WhenNoneUsable, s.KeyAt = now, why, brief, mode, now", liveState},
	{"same login moves in_use_since", "live/live.go", "if s.InUse != login || s.InUseSince.IsZero() {", "if true {", liveState},
	{"in use text drops the since", "internal/app/pool_only.go", "since = \" since \" + clockText(t, now)", "since = \"\"", appNoKey},
	{"keep shown as none", "internal/app/pool_only.go", "\tif st.InUse == \"\" {\n\t\treturn \"none (\" + brief + \")\" + since", "\tif true {\n\t\treturn \"none (\" + brief + \")\" + since", appNoKey},
	{"placeholder never printed", "internal/cli/resume_cmd.go", "value = app.NoKeyPlaceholder", "value = \"\"", cliNoKey},
	{"user's status line replaced", "internal/app/statusline.go", "dir, spec.Key); path != \"\" {", "dir, spec.Key); false && path != \"\" {", appNoKey},
	{"run settings' status line replaced", "internal/app/statusline.go", "if _, set := m[spec.Key]; set {", "if _, set := m[spec.Key]; false && set {", appNoKey},
	{"live.status_line false ignored", "internal/app/statusline.go", "if !cfg.Config.Live.StatusLineOn() {", "if false {", appNoKey},
	{"status line shows no usage", "internal/app/statusline.go", "\tif s.Short != nil {\n\t\tline +=", "\tif false {\n\t\tline +=", appNoKey},
	{"AGX_RUN_LABEL ignored", "internal/app/launch.go", "label, _ = a.d.Lookup(run.EnvRunLabel)", "label = \"\"", appNoKey},
	{"AGX_RUN_LABEL left in the agent's env", "internal/app/launch.go", "out = append(out, agentio.EnvUnset(run.EnvRunLabel))", "_ = out", appNoKey},
	{"record_env not recorded", "internal/app/launch.go", "\t\tout[n] = v\n", "\t\t_ = v\n", appNoKey},
	{"conversation not matched by pid", "internal/app/livecmd.go", "if s.PID != pid || s.ConversationID == \"\" {", "if true {", appNoKey},
}

// appNoKey runs the internal/app tests of a refused key request
// (when_none_usable, live.jsonc's record, the in-use text), the status
// line and the run's label and records.
var appNoKey = []string{"-run", "PortFlow|PoolOnlyCredentialFlow|PoolOnlyWhoami|PoolCredentialNoneUsable|WhenNoneUsable|RefusalBriefs|CredentialTokenLogin|CredentialBefore|CredentialBroken|StatusLine|LaunchLabel|LiveListConversation", "./internal/app/"}

// liveState runs the live package's tests (live.jsonc's state changes).
var liveState = []string{"./live/"}

// cliNoKey runs the CLI tests of agx credential's placeholder.
var cliNoKey = []string{"-run", "CredentialNeverEmpty|CredentialPerUse|LiveListWideAndRefusal", "./internal/cli/"}

// appSchedule runs the internal/app tests of --pool-schedule.
var appSchedule = []string{"-run", "PoolSchedule", "./internal/app/"}

// appTimed runs the internal/app tests of schedules, overrides and
// agx pool prune.
var appTimed = []string{"-run", "PoolSchedule|PoolTimed|PoolUntil|PoolPrune", "./internal/app/"}

// appLiveness runs the internal/app tests of live-run liveness.
var appLiveness = []string{"-run", "PoolLoadSkipsEndedRuns|LiveListOneTableRead", "./internal/app/"}

// appDeadline runs the internal/app test of the deadline group's report.
var appDeadline = []string{"-run", "PoolDeadline", "./internal/app/"}

// appRules runs the internal/app tests of the session rules.
var appRules = []string{"-run", "PoolSessions|PoolClassic", "./internal/app/"}

// appModels runs the internal/app tests of per-model windows.
var appModels = []string{"-run", "PoolProbeModel|PoolFable|PoolRunModel", "./internal/app/"}

// appPreset runs the internal/app tests of presets and sources.
var appPreset = []string{"-run", "PoolPreset|PoolInfo|PoolListAndShow", "./internal/app/"}

// loginHistory runs the usage history tests.
var loginHistory = []string{"-run", "History", "./login/"}

// simRegression runs the simulator's regression of the shipped rules.
var simRegression = []string{"-run", "SimGate|PolicyMatchesPool", "./internal/poolsim/"}

// TestMutation runs every mutant and fails unless each is caught.
func TestMutation(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := copyTree(root, work); err != nil {
		t.Fatal(err)
	}
	caught, missing := 0, []string{}
	var survived, broken []string
	for _, m := range mutants {
		path := filepath.Join(work, filepath.FromSlash(m.file))
		orig, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(orig), m.find) != 1 {
			missing = append(missing, fmt.Sprintf("%s (%d matches)", m.name, strings.Count(string(orig), m.find)))
			continue
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(orig), m.find, m.replace, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		args := m.args
		if args == nil {
			args = []string{"./pool/", "./config/"}
		}
		cmd := exec.Command("go", append([]string{"test", "-count=1"}, args...)...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "CASES=1500", "GOWORK=off")
		out, runErr := cmd.CombinedOutput()
		if err := os.WriteFile(path, orig, 0o644); err != nil {
			t.Fatal(err)
		}
		if runErr == nil {
			survived = append(survived, m.name)
			t.Logf("SURVIVED  %s", m.name)
			continue
		}
		// A break that does not compile tests nothing: the replacement
		// must keep the package building (use "_ = x" for a variable it
		// leaves unused), so that the tests, not the compiler, catch it.
		if strings.Contains(string(out), "[build failed]") || strings.Contains(string(out), "[setup failed]") {
			broken = append(broken, m.name)
			t.Logf("BROKEN    %s (does not compile)", m.name)
			continue
		}
		caught++
		t.Logf("caught    %s (test failed)", m.name)
	}
	t.Logf("%d/%d breaks caught by the tests", caught, len(mutants)-len(missing))
	if len(missing) > 0 {
		t.Errorf("mutation text no longer found (update the list): %s", strings.Join(missing, "; "))
	}
	if len(survived) > 0 {
		t.Errorf("not caught: %s", strings.Join(survived, "; "))
	}
	if len(broken) > 0 {
		t.Errorf("does not compile, so no test ran (keep the replacement building): %s", strings.Join(broken, "; "))
	}
}

// copyTree copies the repository into dst, without .git and dist.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "dist") && p != src {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode().Perm())
	})
}
