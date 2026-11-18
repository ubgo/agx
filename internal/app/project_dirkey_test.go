package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ubgo/agentio/agenttest"
)

// macOSFoldOS is the GOOS whose case-insensitive folder rule the test
// forces, so the macOS behaviour is checked on every machine.
const macOSFoldOS = "darwin"

// symlinkedHomeApp is an App whose user home is reached through a
// symlink (as macOS's /var/folders → /private/var/folders) and whose
// folder comparison uses the macOS case-folding rule. The config binds
// one folder for claude, under the key the caller spells (keyFor gets
// the real, symlink-resolved home). It returns the App and that key.
func symlinkedHomeApp(t *testing.T, keyFor func(realHome string) string) (*App, string) {
	t.Helper()
	root := t.TempDir()
	real := filepath.Join(root, "Real")
	home := filepath.Join(root, "Link", "Home")
	for _, d := range []string{filepath.Join(real, "Home", "projects", "app"), filepath.Join(real, "Home", "projects", "Client"), filepath.Join(real, "Home", ".agx")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(real, filepath.Join(root, "Link")); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(home, ".agx", "config.jsonc")
	// The real home resolved all the way: TMPDIR may itself be a symlink
	// (macOS /var/folders → /private/var/folders).
	realHome, err := filepath.EvalSymlinks(filepath.Join(real, "Home"))
	if err != nil {
		t.Fatal(err)
	}
	key := keyFor(realHome)
	cfg := `{"version": 1, "profiles": {"personal": {"agent": "claude", "home": "~/.claude"}}, "projects": {` + strconv.Quote(key) + `: {"claude": {"profile": "personal"}}}}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	h := agenttest.NewHost(t, agenttest.WithUserHome(home), agenttest.WithEnv("HOME="+home), agenttest.WithHTTP(agenttest.FailingHTTP(t)))
	a := New(Deps{
		Host:       h,
		Lookup:     func(string) (string, bool) { return "", false },
		Cwd:        filepath.Join(home, "projects", "app"),
		ConfigFlag: cfgPath,
	})
	a.foldOS = macOSFoldOS
	return a, key
}

// TestProjectRemoveShowsTheFolderAsGiven: a folder with no binding is
// named as the user gave it (~/projects/app), never as the comparison
// key, which on macOS is symlink-resolved and lower-cased. A real macOS
// run printed /private/var/folders/…/home/projects/app.
func TestProjectRemoveShowsTheFolderAsGiven(t *testing.T) {
	a, _ := symlinkedHomeApp(t, func(string) string { return "~/projects/Client" })
	_, err := a.ProjectRemove(context.Background(), "~/projects/app", "")
	if err == nil {
		t.Fatal("removing a folder without a binding succeeded")
	}
	if got, want := err.Error(), "no binding for ~/projects/app"; got != want {
		t.Errorf("message %q, want %q", got, want)
	}
}

// TestProjectRemoveMatchesOtherCaseKeepsStoredSpelling: under the macOS
// rule a binding stored in another case, under the symlink-resolved home
// (as written by hand), is found from ~/projects/Client, and the result
// names the stored key as written, never the folded comparison key.
func TestProjectRemoveMatchesOtherCaseKeepsStoredSpelling(t *testing.T) {
	a, key := symlinkedHomeApp(t, func(realHome string) string {
		return strings.ToUpper(filepath.Join(realHome, "projects", "client"))
	})
	res, err := a.ProjectRemove(context.Background(), "~/projects/Client", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Dir != key {
		t.Errorf("Dir %q, want the stored key %q", res.Dir, key)
	}
}
