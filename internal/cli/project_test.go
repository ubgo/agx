package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectUseListShowRemove(t *testing.T) {
	f := fixtureProfiles(t)
	client := f.Mkdir("work/client")
	r := f.Run("project", "use", "--dir", client, "--profile", "work", "--json")
	wantCode(t, r, 0)
	// --agent with --login alone keeps the bound profile.
	wantCode(t, f.Run("project", "use", "--dir", client, "--agent", "claude", "--login", "carol"), 0)
	wantCode(t, f.Run("project", "use", "--dir", client, "--profile", "codex"), 0)
	r = f.Run("project", "list", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "project-list", r.Stdout)

	r = f.RunIn(client, "", "project", "show", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "project-show", r.Stdout)
	r = f.RunIn(client, "", "project", "show")
	wantCode(t, r, 0)
	for _, want := range []string{"profile  work", "from folder binding", "login    carol"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("show text lacks %q:\n%s", want, r.Stdout)
		}
	}
	// The same decisions as run --dry-run for those concerns.
	dry := pfParse(t, f.RunIn(client, "", "run", "--dry-run", "--json"))
	show := pfParse(t, f.RunIn(client, "", "project", "show", "--json"))
	for i := range 4 {
		d, s := dry.Decisions[i], show.Decisions[i]
		// The login value differs only by run's "(not resolved: --dry-run)".
		if d.Name != s.Name || d.Source != s.Source || !strings.HasPrefix(d.Value, s.Value) {
			t.Errorf("decision %d: run %+v, show %+v", i, dry.Decisions[i], show.Decisions[i])
		}
	}
	// --agent codex picks the codex binding.
	r = f.RunIn(client, "", "project", "show", "--agent", "codex", "--json", "--field", "data.profile")
	if strings.TrimSpace(r.Stdout) != "codex" {
		t.Errorf("codex binding: %s", r.Stdout)
	}

	wantCode(t, f.Run("project", "remove", "--dir", client, "--agent", "codex"), 0)
	r = f.Run("project", "list", "--json", "--field", "data.*.agent")
	if strings.TrimSpace(r.Stdout) != `["claude"]` {
		t.Errorf("after remove --agent codex: %s", r.Stdout)
	}
	wantCode(t, f.Run("project", "remove", "--dir", client), 0)
	wantErr(t, f.Run("project", "remove", "--dir", client, "--json"), 1, "not-found", "no binding")
	wantCode(t, f.Run("config", "check"), 0)
	if !strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc")), "// keep this comment") {
		t.Error("comment lost")
	}
}

// TestProjectUseHomeLoginText: the text names the home login once
// ("home login"), never "login home login".
func TestProjectUseHomeLoginText(t *testing.T) {
	f := fixtureProfiles(t)
	r := f.Run("project", "use", "--agent", "claude", "--home-login")
	wantCode(t, r, 0)
	if !strings.HasSuffix(strings.TrimSpace(r.Stdout), ": claude home login") {
		t.Errorf("text %q", r.Stdout)
	}
	r = f.Run("project", "use", "--agent", "claude", "--login", "carol")
	if !strings.HasSuffix(strings.TrimSpace(r.Stdout), ": claude login carol") {
		t.Errorf("named login text %q", r.Stdout)
	}
}

func TestProjectUseRefusals(t *testing.T) {
	f := fixtureProfiles(t)
	wantErr(t, f.Run("project", "use", "--json"), 2, "usage", "give --profile, --login or --home-login")
	wantErr(t, f.Run("project", "use", "--login", "carol", "--json"), 2, "usage", "--agent is required")
	wantErr(t, f.Run("project", "use", "--profile", "work", "--agent", "codex", "--json"), 2, "conflict", "disagree")
	wantErr(t, f.Run("project", "use", "--agent", "codex", "--login", "carol", "--json"), 2, "wrong-agent", "serves claude")
	wantErr(t, f.Run("project", "use", "--profile", "work", "--login", "carol", "--home-login", "--json"), 2, "conflict", "pick one")
	wantErr(t, f.Run("project", "use", "--profile", "nope", "--json"), 1, "not-found", "unknown profile nope")
	wantErr(t, f.Run("project", "use", "--profile", "work", "--dir", filepath.Join(f.Root, "gone"), "--json"), 1, "folder-gone", "does not exist")
}

// TestEdge29NestedBindings: the nearest bound folder for the agent wins,
// with a stderr note.
func TestEdge29NestedBindings(t *testing.T) {
	f := fixtureProfiles(t)
	outer := f.Mkdir("work/client")
	inner := f.Mkdir("work/client/api/v2")
	wantCode(t, f.Run("project", "use", "--dir", outer, "--profile", "personal"), 0)
	wantCode(t, f.Run("project", "use", "--dir", filepath.Dir(inner), "--profile", "work"), 0)
	r := f.RunIn(inner, "", "project", "show", "--json", "--field", "data.profile")
	wantCode(t, r, 0)
	if strings.TrimSpace(r.Stdout) != "work" {
		t.Errorf("nearest binding: %s", r.Stdout)
	}
	if !strings.Contains(r.Stderr, "agx: profile work (folder binding ~/") && !strings.Contains(r.Stderr, "agx: profile work (folder binding "+f.Root) {
		t.Errorf("no stderr note: %q", r.Stderr)
	}
	r = f.RunIn(inner, "", "project", "show", "--quiet", "--json")
	if r.Stderr != "" {
		t.Errorf("--quiet printed a note: %q", r.Stderr)
	}
}

// TestEdge30SymlinkedFolder: a folder bound through a symlink is stored
// and matched by its real path.
func TestEdge30SymlinkedFolder(t *testing.T) {
	f := fixtureProfiles(t)
	real := f.Mkdir("real/client")
	link := filepath.Join(f.Root, "link")
	if err := os.Symlink(filepath.Join(f.Root, "real"), link); err != nil {
		t.Skip("symlinks unavailable: " + err.Error())
	}
	wantCode(t, f.Run("project", "use", "--dir", filepath.Join(link, "client"), "--profile", "work"), 0)
	cfg := f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	if !strings.Contains(cfg, `"`+real+`"`) || strings.Contains(cfg, link+"/client") {
		t.Errorf("stored path is not the real one:\n%s", cfg)
	}
	for _, dir := range []string{real, filepath.Join(link, "client")} {
		r := f.RunIn(dir, "", "project", "show", "--json", "--field", "data.profile")
		if strings.TrimSpace(r.Stdout) != "work" {
			t.Errorf("show in %s: %s %s", dir, r.Stdout, r.Stderr)
		}
	}
	// Removing through the link finds the same binding.
	wantCode(t, f.Run("project", "remove", "--dir", filepath.Join(link, "client")), 0)
}
