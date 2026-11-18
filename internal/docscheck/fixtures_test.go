package docscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio/claude"

	"github.com/ubgo/agx/internal/testkit"
)

// fixtureMarker is the line above a ```jsonc block that defines a named
// fixture's config.jsonc: <!-- fixture: demo -->. The config a reader sees
// is the one the runnable examples run against.
var fixtureMarker = regexp.MustCompile(`^\s*<!--\s*fixture:\s*(\S+)\s*-->\s*$`)

// fixtureLang is the fence info string of a fixture config block.
const fixtureLang = "jsonc"

// Fixture names. emptyFixture is built in: a machine with the agents
// installed and homes on disk but no config file.
const (
	defaultFixture = "demo"
	emptyFixture   = "empty"
)

// defaultDir is where examples run (relative to the fixture home) unless
// the marker says dir=…; an unbound folder.
const defaultDir = "projects/app"

// How docs spell the fixture home: "~" alone, and "~/" before a path
// relative to it (dir=~/projects/client, a "< ~/token.txt" redirect).
const (
	homeShown  = "~"
	homePrefix = homeShown + "/"
)

// commandDir is the absolute folder a block or session runs its commands
// in: dir as written in a marker or #options ("~/projects/client"), or
// defaultDir when dir is "".
func commandDir(fx *testkit.Fixture, dir string) string {
	rel := defaultDir
	if dir != "" {
		rel = strings.TrimPrefix(dir, homePrefix)
	}
	return filepath.Join(fx.Home, filepath.FromSlash(rel))
}

// How fixture paths are shown in docs: the fake agents' folder reads as a
// usual install folder, and the temporary root as a neutral path.
const (
	fixtureBinShown  = "/usr/local/bin"
	fixtureRootShown = "/tmp/agx-demo"
	// fixtureHomeKeyShown is the fixture home inside Claude's history
	// folder names, shown as a macOS user's home would be.
	fixtureHomeKeyShown = "-Users-you"
)

// fixtureAgents are the fake agent binaries every fixture installs.
var fixtureAgents = []string{"claude", "codex", "gemini", "pi"}

// fixtureHomes are the agent homes and folders every fixture creates
// (relative to the fixture home).
var fixtureHomes = []string{".claude", ".claude-work", ".codex", ".gemini", ".pi/agent", "projects/app", "projects/client"}

// fakeGopass stands in for gopass: it prints a fake secret named after the
// entry, so --resolved-secrets and launches resolve without a real store.
// The value never matches a secret-scan pattern.
const fakeGopass = `#!/bin/sh
printf 'fake-token-%s\n' "$(basename "$3")"
`

// fakeClaudeTranscript is a minimal Claude Code transcript: one prompt,
// one answer and a title. {{ID}}, {{CWD}}, {{TIME}} and {{TITLE}} are
// filled per conversation. Every value is plainly fake.
const fakeClaudeTranscript = `{"type":"user","sessionId":"{{ID}}","timestamp":"{{TIME}}","cwd":"{{CWD}}","version":"2.1.325","gitBranch":"main","uuid":"u1","message":{"role":"user","content":"fake prompt"}}
{"type":"assistant","sessionId":"{{ID}}","timestamp":"{{TIME}}","cwd":"{{CWD}}","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","model":"fake-model","content":[{"type":"text","text":"fake answer"}],"usage":{"input_tokens":3,"output_tokens":5}}}
{"type":"custom-title","sessionId":"{{ID}}","customTitle":"{{TITLE}}"}
`

// fixtureConversation is one Claude conversation every fixture holds, in
// ~/projects/client, so conversation, resume and move examples have data.
type fixtureConversation struct {
	home, id, title string
	at              time.Time
}

var fixtureConversations = []fixtureConversation{
	{".claude", "3f2a9c1e-0000-4000-8000-000000000001", "Fix the parser", time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)},
	{".claude-work", "7b1d4e20-0000-4000-8000-000000000002", "Review the API", time.Date(2026, 11, 12, 15, 30, 0, 0, time.UTC)},
}

// fixtureNow is the examples' clock (RFC 3339): after every fixture
// conversation and cached reading, and before any real file the run
// creates, so a folder made during the run reads "just now".
const fixtureNow = "2026-11-18T12:00:00Z"

// fixtureConversationDir is the folder the conversations ran in.
const fixtureConversationDir = "projects/client"

// docsColumns is the line width the runnable examples run at: wide enough
// that agx never wraps or truncates a line on the fixture's long temporary
// paths.
const docsColumns = "400"

// claudeHistoryKey is Claude Code's history folder name for a folder, from
// agentio's claude.EncodePath: a local copy missed the cut at 200
// characters plus hash suffix, which a long TMPDIR reaches.
func claudeHistoryKey(dir string) string {
	return claude.EncodePath(dir)
}

// seedConversations writes fixtureConversations into the fixture homes.
func seedConversations(f *testkit.Fixture) error {
	dir := filepath.Join(f.Home, filepath.FromSlash(fixtureConversationDir))
	for _, c := range fixtureConversations {
		body := strings.NewReplacer("{{ID}}", c.id, "{{CWD}}", dir, "{{TITLE}}", c.title, "{{TIME}}", c.at.Format(time.RFC3339)).Replace(fakeClaudeTranscript)
		p := f.WriteFile(filepath.Join(f.Home, c.home, "projects", claudeHistoryKey(dir), c.id+".jsonl"), body, transcriptMode)
		if err := os.Chtimes(p, c.at, c.at); err != nil {
			return err
		}
	}
	return nil
}

// transcriptMode is the mode Claude Code gives its transcripts.
const transcriptMode = 0o600

// findFixtureConfigs collects every fixture config block of the public
// markdown, keyed by name; a name defined twice is a problem.
func findFixtureConfigs(t *testing.T, root string, mds []string) (map[string]string, []string) {
	t.Helper()
	out := map[string]string{}
	where := map[string]string{}
	var probs []string
	for _, md := range mds {
		lines := readLines(t, root, md)
		fenced := fencedLines(lines)
		for i, l := range lines {
			if fenced[i] {
				continue
			}
			m := fixtureMarker.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			at := fmt.Sprintf("%s:%d", md, i+1)
			body, err := fencedBody(lines, i+1, fixtureLang)
			if err != nil {
				probs = append(probs, at+": "+err.Error())
				continue
			}
			if prev, dup := where[m[1]]; dup {
				probs = append(probs, fmt.Sprintf("%s: fixture %q already defined at %s", at, m[1], prev))
				continue
			}
			if m[1] == emptyFixture {
				probs = append(probs, fmt.Sprintf("%s: fixture %q is built in (no config)", at, m[1]))
				continue
			}
			out[m[1]], where[m[1]] = body, at
		}
	}
	return out, probs
}

// fencedBody returns the content of the fenced block opening at line i
// (0-based), which must have info string lang.
func fencedBody(lines []string, i int, lang string) (string, error) {
	if i >= len(lines) {
		return "", fmt.Errorf("marker not followed by a fenced %s block", lang)
	}
	f, ok := openFence(lines[i])
	if !ok || strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(lines[i]), string(f.char))) != lang {
		return "", fmt.Errorf("marker not followed by a fenced %s block", lang)
	}
	for j := i + 1; j < len(lines); j++ {
		if f.closes(lines[j]) {
			return strings.Join(lines[i+1:j], "\n") + "\n", nil
		}
	}
	return "", fmt.Errorf("fenced %s block is never closed", lang)
}

// newDocFixture builds the named fixture machine: fake agents and gopass
// on PATH, the usual homes and project folders, and the config (none for
// the empty fixture).
func newDocFixture(t *testing.T, configs map[string]string, name string) (*testkit.Fixture, error) {
	cfg, ok := configs[name]
	if !ok && name != emptyFixture {
		names := []string{emptyFixture}
		for n := range configs {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("unknown fixture %q (have %s)", name, strings.Join(names, ", "))
	}
	f := testkit.New(t)
	// The fixture's real paths are long temp paths that the shown output
	// spells "~/…" after normalization; a wide line keeps the layout of
	// the real run independent of the temp path's length.
	f.Env["COLUMNS"] = docsColumns
	// A fixed clock, so the relative times agx prints ("5 d ago", "cached
	// 4 d ago") read the same in the docs on every day they are checked.
	f.Env[testkit.EnvTestNow] = fixtureNow
	for _, a := range fixtureAgents {
		f.FakeAgent(a)
	}
	f.WriteFile(filepath.Join(f.Bin, "gopass"), fakeGopass, 0o755)
	for _, h := range fixtureHomes {
		f.HomeDir(filepath.FromSlash(h))
	}
	if err := seedConversations(f); err != nil {
		return nil, err
	}
	if ok {
		f.WriteConfig(cfg)
	}
	return f, nil
}
