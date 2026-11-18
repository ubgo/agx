package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// customAgentConfig defines one config agent beside the built-ins.
const customAgentConfig = `{
  "version": 1,
  "agents": { "mycli": { "spec": {
    "name": "My CLI", "binary": "mycli", "default_home": ".mycli",
    "new_args": ["{args}"], "resume_args": ["--resume", "{id}", "{args}"],
    "version_args": ["--version"],
    "autonomy_args": { "ask": [], "full": ["--yolo"] },
    "token_env": [{ "kind": "api-key", "env": "MYCLI_API_KEY" }]
  } } }
}`

// TestAgentListJSONGolden pins agent list: built-ins then custom, version
// from install metadata, tested range and capabilities from agentio.
func TestAgentListJSONGolden(t *testing.T) {
	f := testkit.New(t)
	nativeClaude(t, f, newerClaude)
	f.FakeAgent("mycli")
	f.WriteConfig(customAgentConfig)
	r := f.Run("agent", "list", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	f.Golden(t, "agent-list", r.Stdout)
	h := f.Run("agent", "list")
	for _, want := range []string{"AGENT", "claude", "built-in", newerClaude, "2.1.323..2.1.327", "newer", "mycli", "custom"} {
		if !strings.Contains(h.Stdout, want) {
			t.Errorf("table lacks %q:\n%s", want, h.Stdout)
		}
	}
}

// TestAgentShowJSONGolden pins agent show: every capability, with the
// agent's own reason when missing (codex has no token login).
func TestAgentShowJSONGolden(t *testing.T) {
	f := testkit.New(t)
	r := f.Run("agent", "show", "--id", "codex", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	f.Golden(t, "agent-show", r.Stdout)
	var env struct {
		Data struct {
			CapabilityDetail []struct {
				Name      string `json:"name"`
				Supported bool   `json:"supported"`
				Reason    string `json:"reason"`
			} `json:"capability_detail"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
		t.Fatal(err)
	}
	for _, c := range env.Data.CapabilityDetail {
		if c.Name == "token-login" && (c.Supported || c.Reason == "") {
			t.Errorf("codex token-login: %+v", c)
		}
		if c.Supported && c.Reason != "" {
			t.Errorf("%s supported but has a reason", c.Name)
		}
	}
	h := f.Run("agent", "show", "--id", "pi")
	if h.Code != 0 || !strings.Contains(h.Stdout, "CAPABILITY") || !strings.Contains(h.Stdout, "autonomy: default, read-only, full") {
		t.Errorf("pi text: code %d\n%s", h.Code, h.Stdout)
	}
}

// TestAgentShowVendorChecks: --vendor-checks runs the binary's --version
// (a fake script here); without it nothing runs.
func TestAgentShowVendorChecks(t *testing.T) {
	f := testkit.New(t)
	// Install metadata says 2.1.325 while the script answers newerClaude,
	// so the result proves the version came from running the binary.
	nativeClaude(t, f, "2.1.325")
	r := f.Run("agent", "show", "--id", "claude", "--json", "--field", "data.vendor_version")
	if r.Code == 0 {
		t.Fatalf("vendor_version present without --vendor-checks: %s", r.Stdout)
	}
	r = f.Run("agent", "show", "--id", "claude", "--vendor-checks", "--json", "--field", "data.vendor_version")
	if r.Code != 0 || strings.TrimSpace(r.Stdout) != newerClaude {
		t.Errorf("vendor version: code %d %q %s", r.Code, r.Stdout, r.Stderr)
	}
	env, _ := runDoctor(t, f, "--agent", "claude", "--vendor-checks")
	if c, ok := find(env.Data, "vendor-version", "claude --version: claude "+newerClaude+" is newer than tested"); !ok || c.Status != "warn" {
		t.Errorf("doctor --vendor-checks: %+v", env.Data)
	}
}

// TestAgentShowErrors: --id is required (usage, exit 2); an unknown id is
// not found (exit 1) and names the known ids; a bare word is refused.
func TestAgentShowErrors(t *testing.T) {
	f := testkit.New(t)
	if r := f.Run("agent", "show"); r.Code != 2 || !strings.Contains(r.Stderr, "--id") {
		t.Errorf("missing --id: code %d %s", r.Code, r.Stderr)
	}
	r := f.Run("agent", "show", "--id", "nope", "--json", "--field", "error.code")
	if r.Code != 1 || strings.TrimSpace(r.Stdout) != "not-found" {
		t.Errorf("unknown id: code %d %q", r.Code, r.Stdout)
	}
	if r := f.Run("agent", "show", "--id", "nope"); !strings.Contains(r.Stderr, "known: claude, codex, gemini, pi") {
		t.Errorf("known ids not named: %s", r.Stderr)
	}
	if r := f.Run("agent", "show", "claude"); r.Code != 2 {
		t.Errorf("bare word: code %d", r.Code)
	}
}

// TestCustomAgentUnsupportedNamesAgentAndAsk: a spec agent refuses a
// level its spec does not describe; the message names the agent and what
// was asked, not only agentio's bare reason.
func TestCustomAgentUnsupportedNamesAgentAndAsk(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".mycli")
	f.FakeAgent("mycli")
	f.WriteConfig(strings.Replace(customAgentConfig, `"version": 1,`,
		`"version": 1, "profiles": { "mine": { "agent": "mycli", "home": "~/.mycli", "default": true } },`, 1))
	r := f.Run("run", "--profile", "mine", "--autonomy", "accept-edits", "--dry-run")
	want := "agx: mycli: autonomy accept-edits: not described by the agent spec\n"
	if r.Code != 125 || r.Stderr != want {
		t.Fatalf("code %d, stderr %q, want %q", r.Code, r.Stderr, want)
	}
}

// fakeMycliKey is the stand-in value of the custom agent's API key; it
// must reach the agent's environment and never any agx output.
const fakeMycliKey = "fake-mycli-key-0123456789"

// TestCustomAgentTokenLogin: a config-defined agent whose spec has
// token_env takes token logins like a built-in: login add accepts the
// kinds token_env names and refuses others with the agent's reason, a dry
// run fills the spec's variable (redacted), and a launch hands the agent
// the resolved value in that variable.
func TestCustomAgentTokenLogin(t *testing.T) {
	f := testkit.New(t)
	f.HomeDir(".mycli")
	f.FakeAgent("mycli")
	f.Env["MYCLI_KEY_REF"] = fakeMycliKey
	f.WriteConfig(strings.Replace(customAgentConfig, `"version": 1,`,
		`"version": 1, "profiles": { "mine": { "agent": "mycli", "home": "~/.mycli", "default": true } },`, 1))

	r := f.Run("login", "add", "--name", "my-key", "--agent", "mycli", "--kind", "api-key", "--token-ref", "${MYCLI_KEY_REF}")
	if r.Code != 0 || !strings.Contains(r.Stdout, "added login my-key (api-key, env MYCLI_KEY_REF)") {
		t.Fatalf("login add: code %d\n%s%s", r.Code, r.Stdout, r.Stderr)
	}
	wantFail(t, f.Run("login", "add", "--name", "plan", "--agent", "mycli", "--kind", "oauth-token", "--token-ref", "x", "--json"),
		4, "unsupported", "mycli has no oauth-token login; it takes api-key")

	r = f.Run("run", "--profile", "mine", "--login", "my-key", "--dry-run", "--quiet", "--json", "--field", "data.command.env")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"set MYCLI_API_KEY=[redacted]"`) {
		t.Fatalf("dry run: code %d\n%s%s", r.Code, r.Stdout, r.Stderr)
	}
	noSecret(t, r, fakeMycliKey)

	r = f.Run("run", "--profile", "mine", "--login", "my-key")
	ag, ok := r.Agent()
	if r.Code != 0 || !ok || ag.Env["MYCLI_API_KEY"] != fakeMycliKey {
		t.Fatalf("launch: code %d, agent ran %v, MYCLI_API_KEY %q\n%s", r.Code, ok, ag.Env["MYCLI_API_KEY"], r.Stderr)
	}
}
