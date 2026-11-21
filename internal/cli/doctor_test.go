package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// Fake values used across the doctor tests; none match a secret-scan
// pattern.
const (
	plainTokenValue = "fake-token-ALPHA"
	refTokenValue   = "fake-token-BRAVO"
	// newerClaude is above agentio's tested claude range (edge 8).
	newerClaude = "2.1.400"
	// deadPID is a pid no test machine runs (above every pid_max).
	deadPID = 2147483646
)

// versionScript is a fake binary that answers --version like Claude Code
// and prints nothing else; it never runs a real vendor binary.
const versionScript = "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"" + newerClaude + " (Claude Code)\"; exit 0; fi\nexit 0\n"

// claudeConfig is one claude profile on ~/.claude-work.
const claudeConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": { "work": { "agent": "claude", "home": "~/.claude-work", "default": true } }
}`

// nativeClaude installs a fake native Claude Code of version v: the
// versions/<v> file and the PATH symlink the native installer makes, so
// agentio reads the version from disk without running it.
func nativeClaude(t *testing.T, f *testkit.Fixture, v string) {
	t.Helper()
	target := f.WriteFile(filepath.Join(f.Home, ".local", "share", "claude", "versions", v), versionScript, 0o755)
	if err := os.Symlink(target, filepath.Join(f.Bin, "claude")); err != nil {
		t.Fatal(err)
	}
}

// doctorEnv is the doctor --json envelope.
type doctorEnv struct {
	OK    bool `json:"ok"`
	Data  []doctorRow
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// doctorRow is one check of doctor --json.
type doctorRow struct {
	Area    string `json:"area"`
	Agent   string `json:"agent"`
	Home    string `json:"home"`
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// runDoctor runs doctor --json and decodes it.
func runDoctor(t *testing.T, f *testkit.Fixture, args ...string) (doctorEnv, testkit.Result) {
	t.Helper()
	r := f.Run(append([]string{"doctor", "--json"}, args...)...)
	var env doctorEnv
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
		t.Fatalf("doctor output is not JSON: %v\n%s\n%s", err, r.Stdout, r.Stderr)
	}
	return env, r
}

// find returns the first check with id whose message contains sub.
func find(rows []doctorRow, id, sub string) (doctorRow, bool) {
	for _, c := range rows {
		if c.ID == id && strings.Contains(c.Message, sub) {
			return c, true
		}
	}
	return doctorRow{}, false
}

// TestEdge07DoctorBinaryMissing: a configured agent whose binary is not
// on PATH fails doctor (exit 1) with the install hint; an unused agent's
// missing binary is information only.
func TestEdge07DoctorBinaryMissing(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.WriteConfig(claudeConfig)
	env, r := runDoctor(t, f, "--agent", "claude")
	if r.Code != 1 || env.OK || env.Error == nil || env.Error.Code != "checks-failed" {
		t.Fatalf("code %d env %+v", r.Code, env)
	}
	c, ok := find(env.Data, "binary-found", "claude not found on PATH")
	if !ok || c.Status != "fail" {
		t.Fatalf("no failing binary check: %+v", env.Data)
	}
	// agentio names an npm package for gemini (not for claude, whose
	// spec has no Packages): the hint comes from there.
	env, _ = runDoctor(t, f, "--agent", "gemini")
	if c, ok := find(env.Data, "binary-found", "gemini not found on PATH; install: npm i -g @google/gemini-cli"); !ok || c.Status != "fail" {
		t.Errorf("--agent names gemini, so its missing binary fails with the hint: %+v", env.Data)
	}
	env, _ = runDoctor(t, f)
	if c, ok := find(env.Data, "binary-found", "gemini not found on PATH"); !ok || c.Status != "info" {
		t.Errorf("unused gemini should be info: %+v", c)
	}
}

// TestEdge08DoctorVersionNewer: a version above the tested range warns
// with the spec wording and does not fail doctor on its own.
func TestEdge08DoctorVersionNewer(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	nativeClaude(t, f, newerClaude)
	f.WriteConfig(claudeConfig)
	env, _ := runDoctor(t, f, "--agent", "claude")
	c, ok := find(env.Data, "version-tested", "claude "+newerClaude+" is newer than tested (2.1.330)")
	if !ok || c.Status != "warn" {
		t.Fatalf("no newer warning: %+v", env.Data)
	}
	if _, ok := find(env.Data, "binary-found", filepath.Join("~", "bin")); ok {
		t.Errorf("binary path should be the fixture bin, not ~/bin")
	}
}

// TestEdge09DoctorVersionUnknown: a binary without install metadata has
// an unknown version: doctor warns, agent list says "unknown".
func TestEdge09DoctorVersionUnknown(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.FakeAgent("claude")
	f.WriteConfig(claudeConfig)
	env, _ := runDoctor(t, f, "--agent", "claude")
	if c, ok := find(env.Data, "version-tested", "claude version unknown (no install metadata)"); !ok || c.Status != "warn" {
		t.Fatalf("no unknown-version warning: %+v", env.Data)
	}
	r := f.Run("agent", "list", "--json", "--field", "data.0.version")
	if r.Code != 0 || strings.TrimSpace(r.Stdout) != "unknown" {
		t.Errorf("agent list version: code %d %q %s", r.Code, r.Stdout, r.Stderr)
	}
}

// TestEdge23DoctorListsPlainTokens: a plain token in config is allowed and
// listed by key as information; its value is never printed.
func TestEdge23DoctorListsPlainTokens(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.WriteConfig(`{
  "version": 1,
  "profiles": { "kimi": { "agent": "claude", "home": "~/.claude-work", "env": { "ANTHROPIC_AUTH_TOKEN": "` + plainTokenValue + `", "DEBUG": "1" } } },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "` + plainTokenValue + `" } }
}`)
	for _, args := range [][]string{{"doctor"}, {"doctor", "--json"}} {
		r := f.Run(args...)
		if strings.Contains(r.Stdout+r.Stderr, plainTokenValue) {
			t.Fatalf("%v printed the token value", args)
		}
	}
	env, _ := runDoctor(t, f)
	for _, key := range []string{"logins.carol.token", "profiles.kimi.env.ANTHROPIC_AUTH_TOKEN"} {
		if c, ok := find(env.Data, "plain-token", key); !ok || c.Status != "info" {
			t.Errorf("plain token %s not listed as info: %+v", key, env.Data)
		}
	}
	if _, ok := find(env.Data, "plain-token", "DEBUG"); ok {
		t.Errorf("DEBUG is not a credential")
	}
}

// TestEdge47DoctorReportsEndedRun: a run folder whose agent ended is
// reported and kept (agx clean deletes, never doctor).
func TestEdge47DoctorReportsEndedRun(t *testing.T) {
	f := testkit.New(t)
	id := "20261114-101500-k3x9"
	dir := filepath.Join(f.ConfigDir, "runs", id)
	f.WriteFile(filepath.Join(dir, "run.jsonc"), `{"id":"`+id+`","pid":`+strconv.Itoa(deadPID)+`,"started_at":"2026-11-14T10:15:00Z","agent":"claude","profile":"work"}`, 0o600)
	env, r := runDoctor(t, f)
	c, ok := find(env.Data, "run-folder", "run "+id+" ended (profile work); kept until agx clean")
	if !ok || c.Status != "info" || c.Fix != "agx clean" {
		t.Fatalf("ended run not reported: code %d %+v", r.Code, env.Data)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("doctor deleted the run folder: %v", err)
	}
	f.WriteFile(filepath.Join(f.ConfigDir, "runs", "20261114-101501-zzzz", "run.jsonc"), "not json", 0o600)
	env, _ = runDoctor(t, f)
	if _, ok := find(env.Data, "run-folder", "run 20261114-101501-zzzz: cannot verify"); !ok {
		t.Errorf("unreadable run not listed: %+v", env.Data)
	}
}

// TestDoctorReferences: references are parsed, never resolved: an unknown
// scheme and a missing $(…) program fail, an unset variable warns, a
// gopass reference parses (gopass is never run).
func TestDoctorReferences(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	nativeClaude(t, f, "2.1.325")
	f.WriteFile(filepath.Join(f.Bin, "gopass"), "#!/bin/sh\necho ran > \"$HOME/gopass-ran\"\n", 0o755)
	f.WriteConfig(`{
  "version": 1,
  "profiles": { "work": { "agent": "claude", "home": "~/.claude-work",
    "env_refs": { "A": "${gopas:x/y}", "B": "$(no-such-program-xyz --flag)", "C": "${UNSET_FOR_DOCTOR}", "D": "${gopass:team/key}", "E": "$(gopass show -o team/key)" } } }
}`)
	env, r := runDoctor(t, f)
	if r.Code != 1 {
		t.Fatalf("code %d, want 1", r.Code)
	}
	want := map[string]string{
		"profiles.work.env_refs.A: no plugin for scheme gopas":              "fail",
		"profiles.work.env_refs.B: program no-such-program-xyz not on PATH": "fail",
		"profiles.work.env_refs.C: UNSET_FOR_DOCTOR is not set":             "warn",
		"profiles.work.env_refs.D: parses":                                  "ok",
		"profiles.work.env_refs.E: program gopass found":                    "ok",
	}
	for sub, st := range want {
		found := false
		for _, c := range env.Data {
			if strings.Contains(c.Message, sub) && c.Status == st {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s %q in %+v", st, sub, env.Data)
		}
	}
	if _, err := os.Stat(filepath.Join(f.Home, "gopass-ran")); err == nil {
		t.Errorf("doctor ran gopass without --resolved-secrets")
	}
}

// TestDoctorResolvedSecrets: --resolved-secrets resolves every reference
// (cost X) and says only whether it resolved; the value never prints.
func TestDoctorResolvedSecrets(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	nativeClaude(t, f, "2.1.325")
	f.WriteConfig(`{
  "version": 1,
  "profiles": { "work": { "agent": "claude", "home": "~/.claude-work", "env_refs": { "API": "${MISSING_VAR_X}" } } },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKVAR}" } }
}`)
	f.Env["TOKVAR"] = refTokenValue
	env, r := runDoctor(t, f, "--resolved-secrets")
	if strings.Contains(r.Stdout+r.Stderr, refTokenValue) {
		t.Fatalf("resolved value printed")
	}
	if c, ok := find(env.Data, "secret-resolves", "login carol: resolves"); !ok || c.Status != "ok" {
		t.Errorf("carol not resolved: %+v", env.Data)
	}
	if c, ok := find(env.Data, "secret-resolves", "MISSING_VAR_X is not set"); !ok || c.Status != "fail" {
		t.Errorf("unset reference should fail: %+v", env.Data)
	}
	if r.Code != 1 {
		t.Errorf("code %d, want 1", r.Code)
	}
	env, _ = runDoctor(t, f)
	if _, ok := find(env.Data, "secret-resolves", ""); ok {
		t.Errorf("secrets resolved without --resolved-secrets")
	}
}

// TestDoctorTokensFileMode: a stored login's tokens.jsonc readable by
// others fails with the chmod fix; a missing stored value fails too.
func TestDoctorTokensFileMode(t *testing.T) {
	f := testkit.New(t)
	f.WriteConfig(`{
  "version": 1,
  "logins": { "derek": { "agents": ["claude"], "kind": "oauth-token", "stored": true }, "zed": { "agents": ["claude"], "kind": "oauth-token", "stored": true } }
}`)
	tokens := f.WriteFile(filepath.Join(f.ConfigDir, "tokens.jsonc"), `{ "tokens": { "derek": "`+plainTokenValue+`" } }`, 0o644)
	env, r := runDoctor(t, f)
	c, ok := find(env.Data, "tokens-mode", "must be 0600")
	if !ok || c.Status != "fail" || c.Fix != "chmod 600 "+tokens || r.Code != 1 {
		t.Fatalf("mode not checked: code %d %+v", r.Code, env.Data)
	}
	if err := os.Chmod(tokens, 0o600); err != nil {
		t.Fatal(err)
	}
	env, _ = runDoctor(t, f)
	if c, ok := find(env.Data, "tokens-mode", "is private"); !ok || c.Status != "ok" {
		t.Errorf("0600 should pass: %+v", env.Data)
	}
	if c, ok := find(env.Data, "stored-token", "login derek: stored value present"); !ok || c.Status != "ok" {
		t.Errorf("derek: %+v", env.Data)
	}
	if c, ok := find(env.Data, "stored-token", "login zed: no stored value"); !ok || c.Status != "fail" {
		t.Errorf("zed: %+v", env.Data)
	}
	if strings.Contains(r.Stdout+r.Stderr, plainTokenValue) {
		t.Errorf("stored value printed")
	}
}

// TestDoctorInvalidConfig: every config problem is a failing check at its
// position, and the built-in agents are still checked.
func TestDoctorInvalidConfig(t *testing.T) {
	f := testkit.New(t)
	f.WriteConfig(`{ "version": 1, "profles": {} }`)
	env, r := runDoctor(t, f)
	if r.Code != 1 {
		t.Fatalf("code %d", r.Code)
	}
	if c, ok := find(env.Data, "config-valid", "config.jsonc:1:"); !ok || c.Status != "fail" {
		t.Errorf("no positioned config failure: %+v", env.Data)
	}
	if _, ok := find(env.Data, "binary-found", "codex"); !ok {
		t.Errorf("built-in agents not checked with a broken config: %+v", env.Data)
	}
}

// TestDoctorShellLayerAndAgentFilter: AGX_SHELL marks the shell layer
// loaded; --agent limits the agent checks and refuses an unknown id.
func TestDoctorShellLayerAndAgentFilter(t *testing.T) {
	f := testkit.New(t)
	env, r := runDoctor(t, f)
	if c, ok := find(env.Data, "shell-layer", "not loaded"); !ok || c.Status != "info" || r.Code != 0 {
		t.Errorf("shell layer: code %d %+v", r.Code, env.Data)
	}
	f.Env["AGX_SHELL"] = "zsh"
	env, _ = runDoctor(t, f, "--agent", "codex")
	if c, ok := find(env.Data, "shell-layer", "loaded (zsh)"); !ok || c.Status != "ok" {
		t.Errorf("shell layer loaded: %+v", env.Data)
	}
	for _, c := range env.Data {
		if c.Agent != "" && c.Agent != "codex" {
			t.Errorf("--agent codex reported %s", c.Agent)
		}
	}
	r = f.Run("doctor", "--agent", "nope")
	if r.Code != 1 || !strings.Contains(r.Stderr, "unknown agent nope") {
		t.Errorf("unknown agent: code %d %s", r.Code, r.Stderr)
	}
}

// TestDoctorJSONGolden pins doctor's --json shape, including a failure:
// ok false, the full report kept, error checks-failed.
func TestDoctorJSONGolden(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	nativeClaude(t, f, newerClaude)
	f.WriteConfig(`{
  "version": 1,
  "profiles": { "work": { "agent": "claude", "home": "~/.claude-work", "env_refs": { "K": "${gopas:x}" } } }
}`)
	r := f.Run("doctor", "--agent", "claude", "--json")
	if r.Code != 1 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	f.Golden(t, "doctor", r.Stdout)
	h := f.Run("doctor", "--agent", "claude")
	// v0.3's report: a mark, the subject, the finding; the fix under it.
	if h.Code != 1 || !strings.Contains(h.Stdout, "✗ reference ") || !strings.Contains(h.Stdout, "✓ config ") ||
		!strings.Contains(h.Stdout, "fix: eval") || !strings.Contains(h.Stderr, "check") {
		t.Errorf("human report: code %d\n%s\n%s", h.Code, h.Stdout, h.Stderr)
	}
}
