package cli_test

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/cli"
	"github.com/ubgo/agx/internal/testkit"
)

// versionLine is the generated-from line; the golden pins it to a fixed
// version so a rebuild does not change it.
var versionLine = regexp.MustCompile(`from agx [^ ]+\. Regenerate`)

// TestSkillGolden pins agx skill's output (the version stamped fixed).
func TestSkillGolden(t *testing.T) {
	f := testkit.New(t)
	r := f.Run("skill")
	if r.Code != 0 || r.Stderr != "" {
		t.Fatalf("%d %s", r.Code, r.Stderr)
	}
	if !strings.HasPrefix(r.Stdout, "---\nname: agx\ndescription: Use when ") {
		t.Errorf("frontmatter:\n%s", r.Stdout[:200])
	}
	testkit.GoldenText(t, "skill", versionLine.ReplaceAllString(r.Stdout, "from agx v0.0.0-test. Regenerate"))
}

// TestSkillCoversTheTree: every visible command of the parser's tree has
// its section, and every visible flag of that command appears in it;
// hidden commands and flags do not.
func TestSkillCoversTheTree(t *testing.T) {
	root, _ := cli.NewRoot(cli.Env{Stdout: io.Discard, Stderr: io.Discard, Stdin: strings.NewReader("")})
	tree := cli.HelpTree(root)
	md := cli.SkillMarkdown(tree, "v0.0.0-test")
	sections := map[string]string{}
	parts := strings.Split(md, "\n### agx ")
	for _, p := range parts[1:] {
		head, body, _ := strings.Cut(p, "\n")
		sections[head] = body
	}
	var walk func(h cli.HelpCommand)
	walk = func(h cli.HelpCommand) {
		for _, s := range h.Commands {
			body, ok := sections[s.Path]
			switch {
			case s.Hidden && ok:
				t.Errorf("hidden command %s is in the skill", s.Path)
			case s.Hidden:
				continue
			case !ok:
				t.Errorf("command %s has no section", s.Path)
			}
			for _, fl := range s.Flags {
				has := strings.Contains(body, "`--"+fl.Name+" ") || strings.Contains(body, "`--"+fl.Name+"`")
				if fl.Hidden && has {
					t.Errorf("hidden flag --%s of %s is in the skill", fl.Name, s.Path)
				}
				if !fl.Hidden && !has {
					t.Errorf("flag --%s of %s is missing", fl.Name, s.Path)
				}
			}
			walk(s)
		}
	}
	walk(tree)
	for _, fl := range tree.Flags {
		if !strings.Contains(md, "`--"+fl.Name) {
			t.Errorf("global flag --%s missing", fl.Name)
		}
	}
	if len(sections) < 40 {
		t.Errorf("only %d sections", len(sections))
	}
}

// TestSkillInstall: --install --dir writes <dir>/agx/SKILL.md (0755,
// 0644), the second install backs the first up; --dir without --install,
// --json and --field are refused.
func TestSkillInstall(t *testing.T) {
	f := testkit.New(t)
	f.Env[testkit.EnvTestNow] = "2026-11-19T10:00:00Z"
	dir := filepath.Join(f.Root, "skills")
	r := f.Run("skill", "--install", "--dir", dir)
	path := filepath.Join(dir, "agx", "SKILL.md")
	if r.Code != 0 || strings.TrimSpace(r.Stdout) != "wrote "+path {
		t.Fatalf("%d %q %s", r.Code, r.Stdout, r.Stderr)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o644 {
		t.Fatalf("%v %v", st, err)
	}
	if ds, _ := os.Stat(filepath.Dir(path)); ds.Mode().Perm() != 0o755 {
		t.Errorf("dir mode %v", ds.Mode())
	}
	if err := os.WriteFile(path, []byte("my edits\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r = f.Run("skill", "--install", "--dir", dir)
	backup := path + ".bak-20261119T100000Z"
	if r.Code != 0 || !strings.Contains(r.Stdout, "backed up "+backup) {
		t.Fatalf("%d %q %s", r.Code, r.Stdout, r.Stderr)
	}
	if b, _ := os.ReadFile(backup); string(b) != "my edits\n" {
		t.Errorf("backup %q", b)
	}
	if b, _ := os.ReadFile(path); !strings.HasPrefix(string(b), "---\nname: agx") {
		t.Errorf("new file %q", b[:20])
	}
	r = f.Run("skill", "--install")
	if want := filepath.Join(f.Home, ".claude", "skills", "agx", "SKILL.md"); r.Code != 0 || !strings.Contains(r.Stdout, want) {
		t.Errorf("default dir: %d %q", r.Code, r.Stdout)
	}
	for _, args := range [][]string{{"skill", "--dir", dir}, {"skill", "--json"}, {"skill", "--json", "--field", "data"}} {
		if r := f.Run(args...); r.Code != 2 || r.Stdout != "" {
			t.Errorf("%v: %d %q", args, r.Code, r.Stdout)
		}
	}
}
