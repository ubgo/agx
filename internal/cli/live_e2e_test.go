package cli_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestLiveLoginEndToEnd launches with --live-login and checks what the
// agent really receives (spec 10): the settings file carries apiKeyHelper
// pointing at agx credential, the refresh interval is in the env, no token
// variable is set, live.jsonc exists; then agx credential (run inside the
// "agent", by AGX_RUN_ID) prints the start login's token, and agx live
// use refuses the run once the agent has ended.
func TestLiveLoginEndToEnd(t *testing.T) {
	f, _ := fixtureResume(t)
	r := f.Run("run", "--profile", "work", "--login", "carol", "--live-login")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	a, ok := r.Agent()
	if !ok {
		t.Fatalf("no agent ran: %s", r.Stdout)
	}
	for _, v := range []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		if _, set := a.Env[v]; set {
			t.Errorf("%s is set in a live run", v)
		}
	}
	if a.Env["CLAUDE_CODE_API_KEY_HELPER_TTL_MS"] == "" {
		t.Errorf("no helper refresh interval in env")
	}
	runID := a.Env["AGX_RUN_ID"]
	settingsPath := ""
	for i, arg := range a.Argv {
		if arg == "--settings" && i+1 < len(a.Argv) {
			settingsPath = a.Argv[i+1]
		}
	}
	if settingsPath == "" {
		t.Fatalf("no --settings in argv %q", a.Argv)
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		APIKeyHelper string `json:"apiKeyHelper"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil || !strings.HasSuffix(settings.APIKeyHelper, "credential") {
		t.Fatalf("settings %s: %v", raw, err)
	}
	if st, err := os.Stat(settingsPath); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("settings file mode: %v %v", st, err)
	}
	if strings.Contains(string(raw), "fake-token") || strings.Contains(r.Stdout+r.Stderr, "fake-token-CAROL") && !strings.Contains(r.Stdout, "env TOKVAR=") {
		t.Errorf("token leaked into settings or output")
	}

	f.Env["AGX_RUN_ID"] = runID
	c := f.Run("credential")
	if c.Code != 0 || strings.TrimSpace(c.Stdout) != "fake-token-CAROL" {
		t.Fatalf("credential: code %d out %q err %s", c.Code, c.Stdout, c.Stderr)
	}
	// The fake agent has exited, so the run has ended: its login can no
	// longer change (the switch on a running run is covered in live_test).
	delete(f.Env, "AGX_RUN_ID")
	if u := f.Run("live", "use", "--id", runID, "--login", "alice"); u.Code != 1 || !strings.Contains(u.Stderr, "has ended") {
		t.Fatalf("live use on an ended run: %d %s", u.Code, u.Stderr)
	}
}
