package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/testkit"
)

// Detected token kinds. Every token below is a made-up value that only
// starts with the public prefix Anthropic gives the kind; the prefixes
// are assembled here so no scanner reads a key shape in the source.
const (
	antPrefix   = "sk-" + "ant-"
	oatPrefix   = antPrefix + "oat"
	apiPrefix   = antPrefix + "api"
	kindPlan    = oatPrefix + "01-FAKEPLANVALUE"
	kindPerUse  = apiPrefix + "03-FAKEPERUSEVALUE"
	kindAllowed = apiPrefix + "03-FAKEALLOWEDVALUE"
	kindDeclKey = apiPrefix + "03-FAKEDECLAREDVALUE"
	kindCmdKey  = apiPrefix + "03-FAKECOMMANDVALUE"
	kindOther   = "fake-token-OTHERKIND"
)

// kindTokens are every planted value, for the leak checks.
var kindTokens = []string{kindPlan, kindPerUse, kindAllowed, kindDeclKey, kindCmdKey, kindOther}

// kindConfig has one login per case: a setup-token (plan), an API key
// declared oauth-token (perUse), the same allowed in config (allowed),
// an API key declared api-key (declkey), a value of neither shape
// (other), an API key behind a command reference (viacmd), and a pool
// whose first own member is the per-use one.
const kindConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": { "work": { "agent": "claude", "home": "~/.claude-work", "default": true } },
  "logins": {
    "plan": { "agents": ["claude"], "kind": "oauth-token", "stored": true },
    "peruse": { "agents": ["claude"], "kind": "oauth-token", "stored": true },
    "allowed": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALLOWED_TOKEN}", "allow_api_key": true },
    "declkey": { "agents": ["claude"], "kind": "api-key", "stored": true },
    "other": { "agents": ["claude"], "kind": "oauth-token", "token": "${OTHER_TOKEN}" },
    "viacmd": { "agents": ["claude"], "kind": "oauth-token", "token": "$(printf %s ${CMD_TOKEN})" },
    "pl": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["peruse", "plan"], "usage_max_age": "1h" } }
  }
}`

// kindNow is the fixture clock.
const kindNow = "2026-11-20T08:00:00Z"

// kindFixture is the machine with every detected-kind case.
func kindFixture(t *testing.T) *testkit.Fixture {
	t.Helper()
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.FakeAgent("claude")
	f.WriteConfig(kindConfig)
	tokens, _ := json.Marshal(map[string]map[string]string{"tokens": {"plan": kindPlan, "peruse": kindPerUse, "declkey": kindDeclKey}})
	f.WriteFile(f.ConfigDir+"/tokens.jsonc", string(tokens), 0o600)
	f.Env["ALLOWED_TOKEN"] = kindAllowed
	f.Env["OTHER_TOKEN"] = kindOther
	f.Env["CMD_TOKEN"] = kindCmdKey
	f.Env[testkit.EnvTestNow] = kindNow
	return f
}

// kindCache writes a login-usage cache for the pool's two members, probed
// a minute before kindNow: peruse has less usage, so the rules rank it
// first. detected is peruse's recorded kind ("" = an entry from before
// detection).
func kindCache(t *testing.T, f *testkit.Fixture, detected string) {
	t.Helper()
	now, _ := time.Parse(time.RFC3339, kindNow)
	at := now.Add(-time.Minute).Format(time.RFC3339)
	reset := now.Add(72 * time.Hour).Format(time.RFC3339)
	entry := func(short, week float64, det string) map[string]any {
		e := map[string]any{"agent": "claude", "probed_at": at, "status": "ok", "windows": []map[string]any{
			{"class": "short", "percent": short, "resets_at": reset}, {"class": "week", "percent": week, "resets_at": reset},
		}}
		if det != "" {
			e["detected"] = det
		}
		return e
	}
	b, _ := json.Marshal(map[string]any{"version": 1, "logins": map[string]any{
		"peruse": entry(5, 10, detected), "plan": entry(20, 30, "oauth-token"),
	}})
	f.WriteFile(f.ConfigDir+"/cache/login-usage.jsonc", string(b), 0o600)
}

// loginRow is the part of a login list row these tests read.
type loginRow struct {
	Name     string `json:"name"`
	Detected string `json:"detected"`
	Mismatch bool   `json:"detected_mismatch"`
	View     string `json:"detected_view"`
}

// listRows runs agx login list --json with extra flags.
func listRows(t *testing.T, f *testkit.Fixture, extra ...string) map[string]loginRow {
	t.Helper()
	r := f.Run(append([]string{"login", "list", "--json"}, extra...)...)
	if r.Code != 0 {
		t.Fatalf("login list: %d %s", r.Code, r.Stderr)
	}
	var env struct{ Data []loginRow }
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
		t.Fatal(err)
	}
	out := map[string]loginRow{}
	for _, row := range env.Data {
		out[row.Name] = row
	}
	return out
}

// TestDetectedKindPrefixes: login list reads the kind from the token's
// prefix for every cheap source: a setup-token is oauth-token, an API key
// is api-key, anything else unknown. A mismatch with the declared kind is
// marked; a command reference stays undetected until --verify.
func TestDetectedKindPrefixes(t *testing.T) {
	f := kindFixture(t)
	rows := listRows(t, f)
	for _, c := range []struct {
		name, detected, view string
		mismatch             bool
	}{
		{"plan", "oauth-token", "oauth-token", false},
		{"peruse", "api-key", "api-key (mismatch)", true},
		{"allowed", "api-key", "api-key (mismatch)", true},
		{"declkey", "api-key", "api-key", false},
		{"other", "unknown", "unknown", false},
		{"viacmd", "", "—", false},
		{"pl", "", "—", false},
	} {
		got := rows[c.name]
		if got.Detected != c.detected || got.Mismatch != c.mismatch || got.View != c.view {
			t.Errorf("%s: got %+v, want detected %q mismatch %v view %q", c.name, got, c.detected, c.mismatch, c.view)
		}
	}
	if got := listRows(t, f, "--verify")["viacmd"]; got.Detected != "api-key" || !got.Mismatch {
		t.Errorf("viacmd with --verify: %+v", got)
	}
	r := f.Run("login", "list", "--color", "never")
	if r.Code != 0 || !strings.Contains(r.Stdout, "DETECTED") || !strings.Contains(r.Stdout, "api-key (mismatch)") {
		t.Fatalf("login list table: %d %s", r.Code, r.Stdout)
	}
	r = f.Run("login", "check", "--no-probe", "--id", "plan", "--json")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"detected": "oauth-token"`) {
		t.Fatalf("login check plan: %d %s", r.Code, r.Stdout)
	}
	r = f.Run("login", "check", "--no-probe", "--id", "peruse", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout, `"detected": "api-key"`) || !strings.Contains(r.Stdout, "billed per use") {
		t.Fatalf("login check peruse must fail: %d %s", r.Code, r.Stdout)
	}
}

// TestPerUseRefusedAtLaunch: a login declared oauth-token whose token is
// an API key does not start, with one line naming the login; the run's
// --allow-api-key and the login's allow_api_key let it start, and a login
// declared api-key starts as before.
func TestPerUseRefusedAtLaunch(t *testing.T) {
	f := kindFixture(t)
	r := f.Run("run", "--login", "peruse")
	if _, launched := r.Agent(); launched || r.Code == 0 {
		t.Fatalf("peruse launched: %d %s", r.Code, r.Stderr)
	}
	first := strings.SplitN(strings.TrimSpace(r.Stderr), "\n", 2)[0]
	if !strings.Contains(first, "login peruse holds an API key (billed per use), but its kind is oauth-token") {
		t.Fatalf("refusal line: %q", r.Stderr)
	}
	r = f.Run("run", "--login", "peruse", "--json")
	if !strings.Contains(r.Stdout, `"code": "per-use-key"`) {
		t.Fatalf("refusal code: %s", r.Stdout)
	}
	for _, args := range [][]string{
		{"run", "--login", "peruse", "--allow-api-key"},
		{"run", "--login", "allowed"},
		{"run", "--login", "declkey"},
		{"run", "--login", "plan"},
	} {
		r := f.Run(args...)
		if _, launched := r.Agent(); !launched {
			t.Errorf("agx %s did not launch: %d %s", strings.Join(args, " "), r.Code, r.Stderr)
		}
	}
	r = f.Run("run", "--login", "peruse", "--dry-run", "--resolved-secrets")
	if r.Code == 0 {
		t.Fatalf("dry run with resolved secrets must refuse too: %s", r.Stdout)
	}
	r = f.Run("run", "--login", "plan", "--dry-run", "--resolved-secrets")
	if r.Code != 0 || !strings.Contains(r.Stdout, "(resolved, detected oauth-token)") {
		t.Fatalf("plan dry run: %d %s", r.Code, r.Stdout)
	}
}

// TestPoolSkipsPerUseMember: the rules never pick a member whose cached
// kind is a per-use API key, and pool pick says why. With a cache entry
// from before detection, the key request learns the kind as it resolves
// the token, records it and picks again, so the API key is never handed
// out; a forced per-use member is refused.
func TestPoolSkipsPerUseMember(t *testing.T) {
	f := kindFixture(t)
	kindCache(t, f, "api-key")
	r := f.Run("pool", "pick", "--id", "pl", "--cached", "--json")
	if r.Code != 0 {
		t.Fatalf("pool pick: %d %s", r.Code, r.Stderr)
	}
	var env struct {
		Data struct {
			Selected   struct{ Name string }
			Candidates []struct {
				Name    string
				Reasons []string
			}
		}
	}
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Selected.Name != "plan" {
		t.Fatalf("selected %q, want plan", env.Data.Selected.Name)
	}
	skipped := false
	for _, c := range env.Data.Candidates {
		if c.Name == "peruse" && strings.Join(c.Reasons, ";") == "token is an API key (billed per use); allow_api_key is off" {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("peruse not skipped with its reason: %s", r.Stdout)
	}
	r = f.Run("pool", "pick", "--id", "pl", "--cached", "--color", "never")
	if !strings.Contains(r.Stdout, "token is an API key (billed per use)") {
		t.Fatalf("pool pick text lacks the reason: %s", r.Stdout)
	}

	kindCache(t, f, "")
	r = f.Run("login", "token", "--id", "pl", "--reveal")
	if r.Code != 0 || strings.TrimSpace(r.Stdout) != kindPlan {
		t.Fatalf("pool key request must hand out plan: %d %q", r.Code, r.Stderr)
	}
	cache := f.ReadFile(filepath.Join(f.ConfigDir, "cache", "login-usage.jsonc"))
	if !strings.Contains(cache, `"detected": "api-key"`) {
		t.Fatalf("detected kind not recorded: %s", cache)
	}
	r = f.Run("login", "token", "--id", "pl", "--reveal", "--pool-force", "peruse")
	if r.Code == 0 || strings.Contains(r.Stdout, kindPerUse) || !strings.Contains(r.Stderr, "billed per use") {
		t.Fatalf("forced per-use member must be refused: %d %q %q", r.Code, r.Stdout, r.Stderr)
	}
}

// TestCredentialPerUseAndWhoami: agx credential refuses a per-use key on
// a run without --allow-api-key, hands it out on one that has it, and
// whoami's in-use line names the detected kind.
func TestCredentialPerUseAndWhoami(t *testing.T) {
	f := kindFixture(t)
	const id = "20261120-080000-kind"
	dir := writeRun(t, f, id, 0, `{"run_id":"`+id+`","agent":"claude","start_login":"peruse","helper":"agx credential"}`)
	f.Env["AGX_RUN_ID"] = id
	r := f.Run("credential")
	if r.Code != 0 || r.Stdout != app.NoKeyPlaceholder+"\n" || strings.Count(strings.TrimSpace(r.Stderr), "\n") != 0 || !strings.Contains(r.Stderr, "billed per use") {
		t.Fatalf("credential must refuse in one line, with the placeholder: %d %q %q", r.Code, r.Stdout, r.Stderr)
	}
	if st := f.ReadFile(filepath.Join(dir, "live.jsonc")); !strings.Contains(st, `"no_key_brief": "api key refused"`) {
		t.Fatalf("live.jsonc: %s", st)
	}
	f.WriteFile(filepath.Join(dir, "live.jsonc"), `{"run_id":"`+id+`","agent":"claude","start_login":"peruse","helper":"agx credential","allow_api_key":true}`, 0o600)
	r = f.Run("credential")
	if r.Code != 0 || strings.TrimSpace(r.Stdout) != kindPerUse {
		t.Fatalf("credential with allow_api_key: %d %q", r.Code, r.Stderr)
	}
	st := f.ReadFile(filepath.Join(dir, "live.jsonc"))
	if !strings.Contains(st, `"in_use_detected": "api-key"`) || strings.Contains(st, antPrefix) {
		t.Fatalf("live.jsonc: %s", st)
	}
	f.Env["AGX_RUN_LOGIN"] = "peruse"
	r = f.Run("whoami", "--color", "never")
	if r.Code != 0 || !strings.Contains(r.Stdout, "peruse (detected api-key)") {
		t.Fatalf("whoami: %d %s", r.Code, r.Stdout)
	}
}

// kindNoLeakExtra reach every path that reads a token's kind.
var kindNoLeakExtra = [][]string{
	{"login", "list"}, {"login", "list", "--verify"}, {"login", "show", "--id", "peruse"}, {"login", "show", "--id", "viacmd"},
	{"login", "check", "--no-probe"}, {"login", "check", "--no-probe", "--id", "peruse"},
	{"run", "--login", "peruse"}, {"run", "--login", "peruse", "--dry-run", "--resolved-secrets"},
	{"run", "--login", "plan", "--dry-run", "--resolved-secrets"}, {"run", "--login", "pl", "--dry-run"},
	{"pool", "pick", "--id", "pl", "--cached"}, {"pool", "show", "--id", "pl"},
	{"login", "token", "--id", "pl", "--pool-force", "peruse"}, {"whoami"}, {"doctor"},
}

// TestNoCommandPrintsATokenOrPrefix sweeps every command plus the paths
// above on the detected-kind machine: no planted token appears, and not
// even the public prefix the kind is read from. Only the kind word is
// ever shown. The two commands that print a token by design are skipped
// as in TestNoCommandPrintsASecret.
func TestNoCommandPrintsATokenOrPrefix(t *testing.T) {
	f := kindFixture(t)
	kindCache(t, f, "api-key")
	for _, inv := range append(leafCommands(t), kindNoLeakExtra...) {
		if noLeakExempt[exemptKey(inv)] {
			continue
		}
		for _, mode := range [][]string{nil, {"--json"}} {
			args := append(append([]string{}, inv...), mode...)
			r := f.Run(args...)
			out := r.Stdout + r.Stderr
			if _, launched := r.Agent(); launched {
				out = r.Stderr
			}
			for _, s := range append([]string{antPrefix}, kindTokens...) {
				if strings.Contains(out, s) {
					t.Errorf("agx %s printed token bytes (%d chars of a planted value): exit %d", strings.Join(args, " "), len(s), r.Code)
				}
			}
		}
	}
	// The refused login token path is not exempt by name: its error must
	// carry no token either.
	r := f.Run("login", "token", "--id", "pl", "--reveal", "--pool-force", "peruse")
	for _, s := range append([]string{antPrefix}, kindTokens...) {
		if strings.Contains(r.Stdout+r.Stderr, s) {
			t.Errorf("refused login token printed token bytes: %q", r.Stderr)
		}
	}
}
