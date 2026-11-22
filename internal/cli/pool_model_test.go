package cli_test

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// agx pool pick --model through the real binary on a login-usage cache
// that holds per-model windows (login.UsageEntry.Models): the flag must
// reach the app layer (the report's probe_models and their source), the
// member whose Fable week is full is skipped for Fable only, and
// --cached keeps every pick off the network.

// modelNow is the fixture's clock; the cache was probed 20 s before.
const modelNow = "2026-11-18T10:00:00Z"

// modelPoolConfig is a pool of one own and one shared member, no rule
// keys. probeModelsKey is replaced to give the pool probe_models.
const (
	probeModelsKey  = `"own": ["alice"],`
	modelPoolConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude", "default": true } },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}", "account": "alice@example.com" },
    "derek": { "agents": ["claude"], "kind": "oauth-token", "token": "${DEREK_TOKEN}", "account": "derek@example.com" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": { ` + probeModelsKey + ` "shared": ["derek"] } }
  }
}`
)

// modelCache: derek's week resets in 30 h (inside shared_borrow_window)
// with 40% of room, so under urgency he goes before alice (38% over 4
// days) for any model but Fable, where he is skipped. The Fable probes recorded derek's Fable week (7d_oi) at 96% under the
// keys "fable" and "claude-fable-5-2"; the default and Opus probes
// recorded no per-model window. alice's Fable week is 40%.
const modelCache = `{
  "version": 1,
  "logins": {
    "alice": {
      "agent": "claude", "account": "alice@example.com", "probed_at": "2026-11-18T09:59:40Z", "status": "ok",
      "windows": [
        { "class": "short", "percent": 30, "resets_at": "2026-11-18T12:00:00Z", "id": "5h" },
        { "class": "week", "percent": 60, "resets_at": "2026-11-22T10:00:00Z", "id": "7d" }
      ],
      "models": {
        "": { "probed_at": "2026-11-18T09:59:40Z", "windows": [] },
        "fable": { "probed_at": "2026-11-18T09:58:10Z", "windows": [
          { "class": "week", "scope": "oi", "percent": 40, "resets_at": "2026-11-21T10:00:00Z", "id": "7d_oi", "status": "allowed" } ] },
        "opus": { "probed_at": "2026-11-18T09:58:10Z", "windows": [] }
      }
    },
    "derek": {
      "agent": "claude", "account": "derek@example.com", "probed_at": "2026-11-18T09:59:40Z", "status": "ok",
      "windows": [
        { "class": "short", "percent": 20, "resets_at": "2026-11-18T14:00:00Z", "id": "5h" },
        { "class": "week", "percent": 30, "resets_at": "2026-11-19T16:00:00Z", "id": "7d" }
      ],
      "models": {
        "": { "probed_at": "2026-11-18T09:59:40Z", "windows": [] },
        "fable": { "probed_at": "2026-11-18T09:58:10Z", "windows": [
          { "class": "week", "scope": "oi", "percent": 96, "resets_at": "2026-11-19T16:00:00Z", "id": "7d_oi", "status": "allowed" } ] },
        "claude-fable-5-2": { "probed_at": "2026-11-18T09:58:10Z", "windows": [
          { "class": "week", "scope": "oi", "percent": 96, "resets_at": "2026-11-19T16:00:00Z", "id": "7d_oi", "status": "allowed" } ] },
        "opus": { "probed_at": "2026-11-18T09:58:10Z", "windows": [] }
      }
    }
  }
}`

// Report words the tests look for.
const (
	fableSkipReason  = "fable week is 96% (cap 70%)"
	fableWindowName  = "fable week"
	srcModelFlag     = "--model"
	srcProbeModels   = "pool probe_models"
	srcDefaultModel  = "default probe model"
	probeLineDefault = "Probe model: the agent's default (all-models windows only)"
)

// modelFixture is the machine: the pool config (with extra spliced in
// before the own members) and the per-model cache, at modelNow.
func modelFixture(t *testing.T, extra string) (*testkit.Fixture, string) {
	t.Helper()
	f := testkit.New(t)
	f.WriteConfig(strings.Replace(modelPoolConfig, probeModelsKey, extra+probeModelsKey, 1))
	cachePath := f.WriteFile(f.ConfigDir+"/cache/login-usage.jsonc", modelCache, 0o600)
	f.Env[testkit.EnvTestNow] = modelNow
	return f, cachePath
}

// modelReport is the part of agx pool pick --json these tests read.
type modelReport struct {
	Data struct {
		ProbeModels       []string `json:"probe_models"`
		ProbeModelsSource string   `json:"probe_models_source"`
		Selected          *struct {
			Name string `json:"name"`
		} `json:"selected"`
		Candidates []struct {
			Name    string   `json:"name"`
			State   string   `json:"state"`
			Reasons []string `json:"reasons"`
			Scoped  []struct {
				Name    string  `json:"name"`
				Percent float64 `json:"percent"`
			} `json:"scoped"`
		} `json:"candidates"`
	} `json:"data"`
}

// pickJSON runs agx pool pick --id auto --cached --json with args and
// decodes the report; a non-zero exit fails the test.
func pickJSON(t *testing.T, f *testkit.Fixture, args ...string) modelReport {
	t.Helper()
	r := f.Run(append([]string{"pool", "pick", "--id", "auto", "--cached", "--json"}, args...)...)
	if r.Code != 0 {
		t.Fatalf("pick %v: exit %d: %s", args, r.Code, r.Stderr)
	}
	var rep modelReport
	if err := json.Unmarshal([]byte(r.Stdout), &rep); err != nil {
		t.Fatalf("pick %v: %v\n%s", args, err, r.Stdout)
	}
	return rep
}

// scopedOf is one candidate's per-model window names with their percent,
// and its reasons; it fails when the member is not in the report.
func scopedOf(t *testing.T, rep modelReport, name string) (map[string]float64, []string) {
	t.Helper()
	for _, c := range rep.Data.Candidates {
		if c.Name == name {
			out := map[string]float64{}
			for _, s := range c.Scoped {
				out[s.Name] = s.Percent
			}
			return out, c.Reasons
		}
	}
	t.Fatalf("no candidate %s", name)
	return nil, nil
}

// TestPoolPickModelFlag: --model reaches the app layer (probe_models
// and source "--model"), selects the cache's windows of that model only
// (key normalised: "Fable[1m]" reads "fable"), skips derek for Fable
// (alice is picked) and not for Opus or the default probe model (derek
// is picked), and the text report names the model and the skip.
func TestPoolPickModelFlag(t *testing.T) {
	f, cachePath := modelFixture(t, "")
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		args       []string
		wantModels []string
		wantSrc    string
		// derekFable is derek's Fable week in the report; 0 = none shown.
		derekFable float64
		wantPick   string
	}{
		{"no --model", nil, []string{}, srcDefaultModel, 0, "derek"},
		{"--model fable", []string{"--model", "fable"}, []string{"fable"}, srcModelFlag, 96, "alice"},
		{"--model with a context suffix", []string{"--model", "Fable[1m]"}, []string{"Fable[1m]"}, srcModelFlag, 96, "alice"},
		{"--model full id", []string{"--model", "claude-fable-5-2"}, []string{"claude-fable-5-2"}, srcModelFlag, 96, "alice"},
		{"--model opus", []string{"--model", "opus"}, []string{"opus"}, srcModelFlag, 0, "derek"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := pickJSON(t, f, c.args...)
			if !slices.Equal(rep.Data.ProbeModels, c.wantModels) || rep.Data.ProbeModelsSource != c.wantSrc {
				t.Errorf("probe models %v from %q, want %v from %q", rep.Data.ProbeModels, rep.Data.ProbeModelsSource, c.wantModels, c.wantSrc)
			}
			if rep.Data.Selected == nil || rep.Data.Selected.Name != c.wantPick {
				t.Errorf("selected %+v, want %s", rep.Data.Selected, c.wantPick)
			}
			scoped, reasons := scopedOf(t, rep, "derek")
			skipped := slices.Contains(reasons, fableSkipReason)
			switch {
			case c.derekFable == 0 && (len(scoped) != 0 || skipped):
				t.Errorf("derek: per-model windows %v, reasons %v; want none", scoped, reasons)
			case c.derekFable != 0 && (scoped[fableWindowName] != c.derekFable || !skipped):
				t.Errorf("derek: per-model windows %v, reasons %v; want %s %v and %q", scoped, reasons, fableWindowName, c.derekFable, fableSkipReason)
			}
		})
	}

	// alice's Fable week shows for Fable and blocks nothing.
	aliceScoped, aliceReasons := scopedOf(t, pickJSON(t, f, "--model", "fable"), "alice")
	if aliceScoped[fableWindowName] != 40 || len(aliceReasons) != 0 {
		t.Errorf("alice under fable: %v %v", aliceScoped, aliceReasons)
	}

	// The text report: the probe line names the flag, derek's line the
	// Fable week and the skip.
	r := f.Run("pool", "pick", "--id", "auto", "--cached", "--model", "fable")
	if r.Code != 0 || !strings.Contains(r.Stdout, "\nProbe model: fable ("+srcModelFlag+")\n") ||
		!strings.Contains(r.Stdout, "- derek [shared] skip: "+fableSkipReason+" (5h 20%, week 30%, fable week 96%, ") {
		t.Errorf("text with --model: exit %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("pool", "pick", "--id", "auto", "--cached")
	if r.Code != 0 || !strings.Contains(r.Stdout, "\n"+probeLineDefault+"\n") || strings.Contains(r.Stdout, fableWindowName) {
		t.Errorf("text without --model: exit %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}

	// --cached probed nothing, so nothing rewrote the cache.
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("--cached picks rewrote the login-usage cache")
	}
}

// TestPoolPickModelOverProbeModels: a pool's probe_models apply to a
// pick without --model (source "pool probe_models", derek skipped), and
// --model wins over them (Opus reads derek's Opus windows: none).
func TestPoolPickModelOverProbeModels(t *testing.T) {
	f, _ := modelFixture(t, `"probe_models": ["claude-fable-5-2"], `)
	rep := pickJSON(t, f)
	if !slices.Equal(rep.Data.ProbeModels, []string{"claude-fable-5-2"}) || rep.Data.ProbeModelsSource != srcProbeModels {
		t.Errorf("probe models %v from %q", rep.Data.ProbeModels, rep.Data.ProbeModelsSource)
	}
	if _, reasons := scopedOf(t, rep, "derek"); !slices.Contains(reasons, fableSkipReason) {
		t.Errorf("derek under probe_models: %v", reasons)
	}
	rep = pickJSON(t, f, "--model", "opus")
	if !slices.Equal(rep.Data.ProbeModels, []string{"opus"}) || rep.Data.ProbeModelsSource != srcModelFlag {
		t.Errorf("--model over probe_models: %v from %q", rep.Data.ProbeModels, rep.Data.ProbeModelsSource)
	}
	if scoped, reasons := scopedOf(t, rep, "derek"); len(scoped) != 0 || len(reasons) != 0 {
		t.Errorf("derek under --model opus: %v %v", scoped, reasons)
	}
	r := f.Run("pool", "pick", "--id", "auto", "--cached")
	if r.Code != 0 || !strings.Contains(r.Stdout, "\nProbe model: claude-fable-5-2 ("+srcProbeModels+")\n") {
		t.Errorf("text: exit %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}
}
