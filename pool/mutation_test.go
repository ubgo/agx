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

// mutant is one break: the exact text in file and its replacement.
type mutant struct {
	name, file, find, replace string
}

// mutants mirror the selector's 41 (adapted to the Go code; the
// selector's checks of JSON types are agx's typed schema, broken here by
// dropping StrictKeys), plus the agx-only rules.
var mutants = []mutant{
	{"cap check becomes exclusive (5h)", "pool/pool.go", "rd.Short.Percent >= caps.Short", "rd.Short.Percent > caps.Short"},
	{"cap check becomes exclusive (week)", "pool/pool.go", "rd.Week.Percent >= caps.Week", "rd.Week.Percent > caps.Week"},
	{"5h safety cutoff becomes exclusive", "pool/pool.go", "rd.Short.Percent >= shortUnavailableAt", "rd.Short.Percent > shortUnavailableAt"},
	{"enabled: false ignored", "pool/pool.go", "reasons = append(reasons, reasonDisabled)", "_ = reasonDisabled"},
	{"status not checked", "pool/pool.go", "if rd.Status != StatusOK {", "if false {"},
	{"unreadable usage allowed", "pool/pool.go", "if w.win != nil && !readable(w.win) {", "if false {"},
	{"unreadable usage treated as readable", "pool/pool.go", "return w != nil && !math.IsNaN(w.Percent) && !math.IsInf(w.Percent, 0)", "return w != nil"},
	{"every window treated as not reported", "pool/pool.go", "return w != nil && !math.IsNaN(w.Percent) && !math.IsInf(w.Percent, 0)", "return false"},
	{"missing account not blocked", "pool/pool.go", "return append(reasons, reasonNoReading)", "return reasons"},
	{"borrow window boundary exclusive", "pool/pool.go", "until >= 0 && until <= cfg.BorrowWindow", "until >= 0 && until < cfg.BorrowWindow"},
	{"stale shared reset counts as inside window", "pool/pool.go", "until >= 0 && until <= cfg.BorrowWindow", "until <= cfg.BorrowWindow"},
	{"no fallback to shared when no own is usable", "pool/pool.go", "case c.Group == GroupOwn || !ownUsable || c.inWindow:", "case c.Group == GroupOwn || c.inWindow:"},
	{"borrow window ignored", "pool/pool.go", "case c.Group == GroupOwn || !ownUsable || c.inWindow:", "case true:"},
	{"shared accounts never borrowed early", "pool/pool.go", "case c.Group == GroupOwn || !ownUsable || c.inWindow:", "case c.Group == GroupOwn || !ownUsable:"},
	{"ownUsable looks at shared", "pool/pool.go", "return c.Eligible && c.Group == GroupOwn })", "return c.Eligible && c.Group == GroupShared })"},
	{"reset order ignored", "pool/pool.go", "da, db := deadline(a), deadline(b)", "da, db := time.Duration(0), time.Duration(0)"},
	{"reset order reversed", "pool/pool.go", "da, db := deadline(a), deadline(b)", "da, db := deadline(b), deadline(a)"},
	{"tie order reversed", "pool/pool.go", "return a.order - b.order", "return b.order - a.order"},
	{"stale reset sorts first", "pool/pool.go", "if !c.hasWeekReset || *c.UntilReset < 0 {", "if !c.hasWeekReset {"},
	{"unknown reset sorts first", "pool/pool.go", "return time.Duration(math.MaxInt64)", "return 0"},
	{"unreadable reset counts as a deadline", "pool/pool.go", "if rd.Week.ResetsAt != nil && rd.Week.ResetsAt.IsZero() {", "if false {"},
	{"last of pool picked", "pool/pool.go", "pick := ready[0]", "pick := ready[len(ready)-1]"},
	{"eligible with one reason", "pool/pool.go", "c.Eligible = len(c.Reasons) == 0", "c.Eligible = len(c.Reasons) <= 1"},
	{"enabled read wrongly", "pool/pool.go", "return !ok || a.Enabled == nil || *a.Enabled", "return ok && a.Enabled != nil && *a.Enabled"},
	{"room ignores the weekly cap", "pool/pool.go", "math.Max(0, c.Caps.Week-rd.Week.Percent)", "math.Max(0, 100-rd.Week.Percent)"},
	{"config force beats --force", "pool/pool.go", "force, by := o.Force, ForcedByFlag\n\tif force == \"\" {\n\t\tforce, by = cfg.Force, ForcedByConfig", "force, by := cfg.Force, ForcedByConfig\n\tif force == \"\" {\n\t\tforce, by = o.Force, ForcedByFlag"},
	{"force ignored", "pool/pool.go", "case force != \"\":", "case false:"},
	{"defaults override account caps (5h)", "pool/pool.go", "caps.Short = *a.Short", "caps.Short = c.Defaults.Short"},
	{"defaults override account caps (week)", "pool/pool.go", "caps.Week = *a.Week", "caps.Week = c.Defaults.Week"},
	{"defaults setting ignored", "pool/pool.go", "caps := c.Defaults", "caps := Caps{Short: DefaultShortCap, Week: DefaultWeekCap}"},
	{"default week cap changed", "pool/pool.go", "DefaultWeekCap = 70.0", "DefaultWeekCap = 80.0"},
	{"default 5h cap changed", "pool/pool.go", "DefaultShortCap = 50.0", "DefaultShortCap = 60.0"},
	{"default safety cutoff changed", "pool/pool.go", "DefaultShortUnavailableAt = 99.0", "DefaultShortUnavailableAt = 100.0"},
	{"default borrow window changed", "pool/pool.go", "DefaultBorrowWindow = 48 * time.Hour", "DefaultBorrowWindow = 24 * time.Hour"},
	{"cap range not checked", "pool/pool.go", "return v >= MinPercent && v <= MaxPercent && !math.IsNaN(v)", "return v >= MinPercent && v <= 10*MaxPercent && !math.IsNaN(v)"},
	{"NaN cap allowed", "pool/pool.go", "return v >= MinPercent && v <= MaxPercent && !math.IsNaN(v)", "return !(v < MinPercent) && !(v > MaxPercent)"},
	{"duplicate names allowed (pool)", "pool/pool.go", "if seen[name] {", "if false {"},
	{"accounts entry for an unlisted name allowed", "pool/pool.go", "if !seen[name] {", "if false {"},
	{"bad now allowed", "pool/pool.go", "if o.Now.IsZero() {", "if false {"},
	{"wait list joins the pool", "pool/pool.go", "waiting = append(waiting, c)", "ready = append(ready, c)"},
	{"unknown settings allowed (schema strict keys)", "config/load.go", "schema.Validate(doc, s, schema.StrictKeys(), schema.WithFileName(name))", "schema.Validate(doc, s, schema.WithFileName(name))"},
	{"duplicate names allowed (config)", "config/check.go", "if seen[m] {", "if false {"},
	{"empty force allowed", "config/check.go", "if p.Force != nil && *p.Force == \"\" {", "if false {"},
	{"unknown member allowed", "config/check.go", "\tcase !ok:\n\t\treturn \"unknown login \" + m", "\tcase !ok && false:\n\t\treturn \"unknown login \" + m"},
	{"member of another agent allowed", "config/check.go", "if !ml.Serves(id) {", "if false {"},
	{"null cap allowed (schema)", "config/schema.go", "n.Types = []string{k.typ}", "n.Types = []string{k.typ, SchemaNull}"},
	{"cap range left out of the schema", "config/schema.go", "n.Maximum = jsonNumber(pool.MaxPercent)", "n.Maximum = jsonNumber(10 * pool.MaxPercent)"},
}

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
		cmd := exec.Command("go", "test", "-count=1", "./pool/", "./config/")
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
