package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAliasAddListRemove(t *testing.T) {
	f := fixtureProfiles(t)
	for _, args := range []string{"run --profile personal", "run -p work --model opus -- --verbose", "resume", "usage --cached --json", "run --dry-run", "profile list --agent=claude"} {
		r := f.Run("alias", "add", "--name", "a"+string(rune('a'+len(args)%26)), "--args", args)
		if r.Code != 0 {
			t.Errorf("valid alias %q refused: %s", args, r.Stderr)
		}
	}
	wantCode(t, f.Run("alias", "add", "--name", "cl", "--args", "run  --profile personal"), 0)
	r := f.Run("alias", "add", "--name", "cl", "--args", "run --profile work", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "alias-add-replace", r.Stdout)
	r = f.Run("alias", "list", "--json", "--field", "data")
	if !strings.Contains(r.Stdout, `"run --profile work"`) {
		t.Errorf("list: %s", r.Stdout)
	}
	r = f.Run("alias", "list")
	if !strings.Contains(r.Stdout, "NAME") || !strings.Contains(r.Stdout, "cl") {
		t.Errorf("list table: %s", r.Stdout)
	}
	wantCode(t, f.Run("alias", "remove", "--id", "cl"), 0)
	wantErr(t, f.Run("alias", "remove", "--id", "cl", "--json"), 1, "not-found", "unknown alias cl")
	if !strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc")), "// keep this comment") {
		t.Error("comment lost")
	}
}

func TestAliasRefusesInvalidValues(t *testing.T) {
	f := fixtureProfiles(t)
	for args, msg := range map[string]string{
		"run personal":               `unexpected word "personal"`,
		"runn --profile work":        `"runn" is not an agx command`,
		"profile":                    "agx profile needs a verb",
		"run --nope":                 "agx run has no flag --nope",
		"run --profile":              "--profile needs a value",
		"run --profile $(rm -rf ~)":  "has a character outside",
		"run --profile work; rm -rf": "has a character outside",
		"--profile work":             "starts with an agx command",
		"launch --plan-file x":       "internal",
		"":                           "needs agx arguments",
	} {
		r := f.Run("alias", "add", "--name", "x", "--args", args, "--json")
		if r.Code != 2 {
			t.Errorf("%q: exit %d", args, r.Code)
			continue
		}
		if e := pfParse(t, r); e.Error == nil || !strings.Contains(e.Error.Message, msg) {
			t.Errorf("%q: error %+v, want %q", args, e.Error, msg)
		}
	}
	wantErr(t, f.Run("alias", "add", "--name", "1x", "--args", "resume", "--json"), 2, "usage", "--name")
	// shell-init's own rules apply at add: a reserved name, or a word a
	// shell expands (zsh =word, pwsh @word), is refused here instead of
	// being stored and then skipped by shell-init.
	for _, name := range []string{"agx", "command", "builtin", "cd", "set", "function"} {
		wantErr(t, f.Run("alias", "add", "--name", name, "--args", "resume", "--json"), 2, "usage", "a name may not be agx, command, builtin, cd, set, function")
	}
	wantErr(t, f.Run("alias", "add", "--name", "x", "--args", "run --profile =work", "--json"), 2, "usage", `--args: word "=work" may not start with = or @`)
	wantErr(t, f.Run("alias", "add", "--name", "x", "--args", "run --model @m", "--json"), 2, "usage", `--args: word "@m" may not start with = or @`)
	if r := f.Run("alias", "list", "--json", "--field", "data.*.name"); strings.Contains(r.Stdout, "agx") {
		t.Fatalf("a refused alias was stored: %s", r.Stdout)
	}
}

// TestAliasCheckedAtLoad: an alias written by hand that is not a valid
// command is a config problem (spec 15.3 cross-check).
func TestAliasCheckedAtLoad(t *testing.T) {
	f := fixtureProfiles(t)
	f.WriteConfig(strings.Replace(profilesConfig, `"projects": {}`, `"projects": {}, "aliases": { "bad": "run personal" }`, 1))
	r := f.Run("config", "check")
	wantCode(t, r, 2)
	if !strings.Contains(r.Stderr, "aliases.bad") || !strings.Contains(r.Stderr, `unexpected word "personal"`) {
		t.Errorf("check: %s", r.Stderr)
	}
}
