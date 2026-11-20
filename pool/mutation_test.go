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
	{"last of pool picked", "pool/pool.go", "pick := ready[0]", "pick := ready[len(ready)-1]", nil},
	{"eligible with one reason", "pool/pool.go", "c.Eligible = len(c.Reasons) == 0", "c.Eligible = len(c.Reasons) <= 1", nil},
	{"enabled read wrongly", "pool/pool.go", "return !ok || a.Enabled == nil || *a.Enabled", "return ok && a.Enabled != nil && *a.Enabled", nil},
	{"room ignores the weekly cap", "pool/pool.go", "math.Max(0, c.Caps.Week-rd.Week.Percent)", "math.Max(0, 100-rd.Week.Percent)", nil},
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
	// The expiring strategy (pool/expiring.go) and its wiring.
	{"weekly relax ignored", "pool/expiring.go", "cand.Caps.Week = week", "cand.Caps.Week = configured", nil},
	{"relax not linear", "pool/expiring.go", "gone := 1 - until.Seconds()/window.Seconds()", "gone := 1.0", nil},
	{"stale reset relaxed", "pool/expiring.go", "if window <= 0 || until < 0 ||", "if window <= 0 ||", nil},
	{"relax may lower a higher cap", "pool/expiring.go", "until >= window || relaxMax <= capWeek {", "until >= window {", nil},
	{"relax outside the window", "pool/expiring.go", "until >= window || relaxMax", "until >= 10*window || relaxMax", nil},
	{"member relax max ignored", "pool/expiring.go", "if a, ok := c.Accounts[name]; ok && a.WeekRelaxMax != nil {", "if a, ok := c.Accounts[name]; false && ok && a.WeekRelaxMax != nil {", nil},
	{"group relax max ignored", "pool/expiring.go", "if g, ok := c.Groups[c.groupOf(name)]; ok && g.WeekRelaxMax != nil {", "if g, ok := c.Groups[c.groupOf(name)]; false && ok && g.WeekRelaxMax != nil {", nil},
	{"group caps ignored", "pool/pool.go", "if g, ok := c.Groups[c.groupOf(name)]; ok {", "if g, ok := c.Groups[GroupUnlisted]; ok {", nil},
	{"urgency order reversed", "pool/expiring.go", "case ua > ub:\n\t\treturn -1", "case ua < ub:\n\t\treturn -1", nil},
	{"unknown urgency sorts first", "pool/expiring.go", "if !c.hasUrgency {\n\t\treturn -1", "if !c.hasUrgency {\n\t\treturn math.MaxFloat64", nil},
	{"minutes to the reset divide by zero", "pool/expiring.go", "math.Max(c.UntilReset.Hours(), minUrgencyHours)", "c.UntilReset.Hours()", nil},
	{"urgency ignores the relaxed cap", "pool/expiring.go", "math.Max(0, c.Caps.Week-rd.Week.Percent)", "math.Max(0, 70-rd.Week.Percent)", nil},
	{"expiring sorts by deadline", "pool/pool.go", "order = byUrgency", "order = byDeadline", nil},
	{"expiring never assigns", "pool/pool.go", "case len(ready) > 0 && res.Strategy == StrategyExpiring && o.Only == \"\":", "case false:", nil},
	{"sticky ignored", "pool/expiring.go", "if cfg.Sticky && o.Current != \"\" {", "if false {", nil},
	{"sticky ignores max_sessions", "pool/expiring.go", "(limit == 0 || o.CurrentAhead < limit)", "true", nil},
	{"sticky keeps a waiting member", "pool/expiring.go", "if i := slices.IndexFunc(ready, func(c *Candidate) bool { return c.Name == o.Current });", "if i := slices.IndexFunc(append(ready, waiting...), func(c *Candidate) bool { return c.Name == o.Current });", nil},
	{"max_sessions ignored", "pool/expiring.go", "return limit > 0 && c.Load >= limit", "return false", nil},
	{"max_sessions exclusive", "pool/expiring.go", "c.Load >= limit", "c.Load > limit", nil},
	{"no overflow to waiting members", "pool/expiring.go", "[][]*Candidate{ready, waiting}", "[][]*Candidate{ready}", nil},
	{"most loaded when all are full", "pool/expiring.go", "if c.Load < least.Load {", "if c.Load > least.Load {", nil},
	{"one-run weekly cap relaxed", "pool/pool.go", "a.WeekRelaxMax = o.OnlyCaps.Week", "a.WeekRelaxMax = nil", nil},
	{"unknown strategy allowed", "pool/expiring.go", "if c.Strategy != \"\" && !slices.Contains(StrategyValues(), string(c.Strategy)) {", "if false {", nil},
	{"negative max_sessions allowed", "pool/expiring.go", "if c.MaxSessions < 0 {", "if false {", nil},
	{"bad group allowed", "pool/expiring.go", "if g != GroupOwn && g != GroupShared {", "if false {", nil},
	{"default strategy changed", "pool/pool.go", "DefaultStrategy = StrategyDeadline", "DefaultStrategy = StrategyExpiring", nil},
	{"default relax window changed", "pool/pool.go", "DefaultWeekRelaxWindow = 24 * time.Hour", "DefaultWeekRelaxWindow = 48 * time.Hour", nil},
	{"default relax max changed", "pool/pool.go", "DefaultWeekRelaxMax = 95.0", "DefaultWeekRelaxMax = 90.0", nil},
	{"default max_sessions changed", "pool/pool.go", "DefaultMaxSessions = 3", "DefaultMaxSessions = 4", nil},
	{"default sticky changed", "pool/pool.go", "DefaultSticky = true", "DefaultSticky = false", nil},
	{"config strategy ignored", "config/pool.go", "c.Strategy = pool.Strategy(p.Strategy)", "_ = p.Strategy", nil},
	{"config max_sessions ignored", "config/pool.go", "c.MaxSessions = *p.MaxSessions", "_ = p.MaxSessions", nil},
	{"config group caps ignored", "config/pool.go", "c.Groups[g] = pool.GroupCaps{Short: gc.Short, Week: gc.Week, WeekRelaxMax: gc.WeekRelaxMax}", "_ = gc", nil},
	{"live runs not counted", "internal/app/pool.go", "sel.Load, sel.Current, sel.CurrentAhead = a.poolLoad(name, lg, o.RunID, now, rep.warn)", "_, _, _ = a.poolLoad(name, lg, o.RunID, now, rep.warn)", appExpiring},
	{"idle runs counted", "internal/app/pool.go", "if now.Sub(st.InUseAt) <= activeRunTTLs*ttl {", "if true {", appExpiring},
	{"credential run not passed", "internal/app/livecmd.go", "poolModeCredential, st.PoolForce, only, f.ID)", "poolModeCredential, st.PoolForce, only, \"\")", appExpiring},
	// The simulator's regression: the shipped strategy must keep its gains.
	{"spread off in the simulated pool", "pool/expiring.go", "full := func(c *Candidate) bool { return limit > 0 && c.Load >= limit }", "full := func(c *Candidate) bool { return false }", simRegression},
	{"relax off in the simulated pool", "pool/pool.go", "if found && cfg.strategy() == StrategyExpiring {\n\t\tcfg.relax(c, rd, now)", "if false {\n\t\tcfg.relax(c, rd, now)", simRegression},
}

// appExpiring runs the internal/app tests of the expiring strategy.
var appExpiring = []string{"-run", "PoolExpiring|PoolDeadline", "./internal/app/"}

// simRegression runs the simulator's regression of the shipped rules.
var simRegression = []string{"-run", "Regression|PolicyMatchesPool", "./internal/poolsim/"}

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
	var survived []string
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
		caught++
		why := "test failed"
		if strings.Contains(string(out), "[build failed]") {
			why = "does not compile"
		}
		t.Logf("caught    %s (%s)", m.name, why)
	}
	t.Logf("%d/%d breaks caught by the tests", caught, len(mutants)-len(missing))
	if len(missing) > 0 {
		t.Errorf("mutation text no longer found (update the list): %s", strings.Join(missing, "; "))
	}
	if len(survived) > 0 {
		t.Errorf("not caught: %s", strings.Join(survived, "; "))
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
