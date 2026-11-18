package cli_test

import (
	"io"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/cli"
	"github.com/ubgo/agx/internal/testkit"
)

// Secret values planted in every place a token can come from. None may
// ever appear in agx's own output (spec principle 12, CLAUDE.md).
const (
	leakPlain   = "fake-token-PLAINLEAK"
	leakRef     = "fake-token-REFLEAK"
	leakStored  = "fake-token-STOREDLEAK"
	leakEnvRef  = "fake-token-ENVREFLEAK"
	leakEnvFile = "fake-token-ENVFILELEAK"
	leakOneOff  = "fake-token-ONEOFFLEAK"
	leakGopass  = "fake-token-GOPASSLEAK"
)

// leaks lists every planted value.
var leaks = []string{leakPlain, leakRef, leakStored, leakEnvRef, leakEnvFile, leakOneOff, leakGopass}

// leakConfig uses each source: a plain token, an env reference, a stored
// value, a gopass reference, env_refs and an env file.
const leakConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true, "login": "plain",
      "env_refs": { "API_KEY": "${LEAK_ENVREF}" }, "env_file": "work.env" },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true }
  },
  "logins": {
    "plain": { "agents": ["claude"], "kind": "oauth-token", "token": "` + leakPlain + `" },
    "ref": { "agents": ["claude"], "kind": "oauth-token", "token": "${LEAK_REF}" },
    "stored": { "agents": ["claude"], "kind": "oauth-token", "stored": true },
    "gp": { "agents": ["claude"], "kind": "api-key", "token": "${gopass:x/y}" }
  },
  "projects": {},
  "aliases": { "w": "run --profile work" }
}`

// leakFixture builds the machine with every secret source.
func leakFixture(t *testing.T) *testkit.Fixture {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.HomeDir(".codex")
	f.FakeAgent("claude")
	f.FakeAgent("codex")
	f.WriteFile(f.Bin+"/gopass", "#!/bin/sh\necho "+leakGopass+"\n", 0o755)
	f.WriteConfig(leakConfig)
	f.WriteFile(f.ConfigDir+"/work.env", "FILE_SECRET="+leakEnvFile+"\n", 0o600)
	f.WriteFile(f.ConfigDir+"/tokens.jsonc", `{"tokens":{"stored":"`+leakStored+`"}}`, 0o600)
	f.Env["LEAK_REF"] = leakRef
	f.Env["LEAK_ENVREF"] = leakEnvRef
	return f
}

// leafCommands lists every leaf command path from the parser.
func leafCommands(t *testing.T) [][]string {
	root, _ := cli.NewRoot(cli.Env{Stdout: io.Discard, Stderr: io.Discard, Stdin: strings.NewReader("")})
	var out [][]string
	var walk func(h cli.HelpCommand)
	walk = func(h cli.HelpCommand) {
		if len(h.Commands) == 0 && h.Path != "" {
			out = append(out, strings.Fields(h.Path))
		}
		for _, s := range h.Commands {
			walk(s)
		}
	}
	walk(cli.HelpTree(root))
	if len(out) < 40 {
		t.Fatalf("only %d leaf commands found", len(out))
	}
	return out
}

// noLeakExtra are invocations that reach the secret paths with real
// arguments (a bare leaf mostly stops at a usage error).
var noLeakExtra = [][]string{
	{"run", "--dry-run"}, {"run", "--dry-run", "--resolved-secrets"},
	{"run", "--dry-run", "--resolved-secrets", "--login", "ref"},
	{"run", "--dry-run", "--resolved-secrets", "--login", "stored"},
	{"run", "--dry-run", "--resolved-secrets", "--login", "gp"},
	{"run", "--dry-run", "--resolved-secrets", "--token-ref", leakOneOff},
	{"run", "--dry-run", "--resolved-secrets", "--env", "X=${LEAK_REF}"},
	{"run", "--dry-run", "--resolved-secrets", "--env", "Y=$(printf %s ${LEAK_REF})"},
	{"run", "--dry-run", "--resolved-secrets", "--setting", "env.K=" + leakOneOff},
	{"run", "--dry-run", "--resolved-secrets", "--login", "ref", "--live-login"},
	{"new", "--dry-run", "--resolved-secrets"},
	{"login", "list"}, {"login", "show", "--id", "plain"}, {"login", "show", "--id", "stored"},
	{"login", "check", "--no-probe"}, {"login", "check", "--no-probe", "--id", "gp"},
	{"profile", "list"}, {"profile", "show", "--id", "work"},
	{"project", "show"}, {"config", "show"}, {"config", "check"},
	{"doctor"}, {"doctor", "--resolved-secrets"},
	{"usage", "--cached"}, {"whoami"}, {"shell-init", "--shell", "bash"},
	{"login", "use", "--id", "ref", "--terminal"},
}

// noLeakExempt are the only commands that print a token by design, so
// the sweep skips them: the hidden credential (the live-login helper the
// agent runs) and login token (prints one login's token when the user
// asks, for a pipe or an apiKeyHelper; its own tests cover what it may
// print and when). Every other command stays in the sweep.
var noLeakExempt = map[string]bool{"credential": true, "login token": true}

// TestNoCommandPrintsASecret runs every command (bare, and the extra
// invocations above) in human, --json and --json --field data modes and
// fails if any planted token value appears in stdout or stderr, skipping
// only noLeakExempt. A literal value typed on the command line (--env
// X=literal) is the user's own text and may be shown; every value that
// came from a reference must not be.
func TestNoCommandPrintsASecret(t *testing.T) {
	f := leakFixture(t)
	leaves := leafCommands(t)
	invocations := append(leaves, noLeakExtra...)
	swept := map[string]bool{}
	for _, inv := range invocations {
		if noLeakExempt[exemptKey(inv)] {
			continue
		}
		swept[strings.Join(inv, " ")] = true
		for _, mode := range [][]string{nil, {"--json"}, {"--json", "--field", "data"}} {
			args := append(append([]string{}, inv...), mode...)
			r := f.Run(args...)
			out := r.Stdout + r.Stderr
			if _, launched := r.Agent(); launched {
				// The (fake) agent printed its own environment after agx
				// exec'd it; only agx's stderr notes are agx's output.
				out = r.Stderr
			}
			for _, s := range leaks {
				if strings.Contains(out, s) {
					t.Errorf("agx %s printed a secret (%s…): exit %d", strings.Join(args, " "), s[:14], r.Code)
				}
			}
		}
	}
	// Every leaf outside noLeakExempt was run: the exemption is the only
	// way out of the sweep.
	for _, inv := range leaves {
		if k := strings.Join(inv, " "); !noLeakExempt[k] && !swept[k] {
			t.Errorf("agx %s was not swept", k)
		}
	}
}

// exemptKey is the command an invocation runs, as noLeakExempt names it:
// the leading words before the first flag.
func exemptKey(inv []string) string {
	var words []string
	for _, w := range inv {
		if strings.HasPrefix(w, "-") {
			break
		}
		words = append(words, w)
	}
	return strings.Join(words, " ")
}

// TestNoLeakSweepCoversEveryOtherCommand: the exemption list names only
// commands that exist and skips nothing else, so a new command is swept
// by default and a renamed exemption cannot silently widen the skip.
func TestNoLeakSweepCoversEveryOtherCommand(t *testing.T) {
	leaves := leafCommands(t)
	found := map[string]bool{}
	skipped := 0
	for _, inv := range leaves {
		k := strings.Join(inv, " ")
		if noLeakExempt[exemptKey(inv)] {
			found[k] = true
			skipped++
		}
	}
	for k := range noLeakExempt {
		if !found[k] {
			t.Errorf("noLeakExempt names %q, which is not a leaf command", k)
		}
	}
	if skipped != len(noLeakExempt) {
		t.Errorf("the sweep skips %d leaf commands, want exactly %d (noLeakExempt)", skipped, len(noLeakExempt))
	}
}
