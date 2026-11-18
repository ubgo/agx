package cli_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/testkit"
)

// The real binary uses the real network client, so the CLI tests stay on
// paths that send nothing: --cached, refusals, and auto-pick cases that
// fail before a request (expired or missing logins, per-token profiles,
// agents without usage). Live reads are covered in internal/app.

// usageCLIConfig has a Claude home with a cached reading, a second one,
// and a pi home.
const usageCLIConfig = `{
  "version": 1,
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work" },
    "side": { "agent": "claude", "home": "~/.claude-side" },
    "pi": { "agent": "pi", "home": "~/.pi-home" }
  }
}`

// cachedBody is a .claude.json holding a cached reading fetched ago
// before now, with session and weekly percents.
func cachedBody(ago time.Duration, session, weekly float64) string {
	return fmt.Sprintf(`{"cachedUsageUtilization":{"fetchedAtMs":%d,"utilization":{"limits":[`+
		`{"kind":"session","group":"session","percent":%g,"resets_at":"2026-11-13T14:00:00Z"},`+
		`{"kind":"weekly_all","group":"weekly","percent":%g,"resets_at":"2026-11-16T06:00:00Z"}]}}}`,
		time.Now().Add(-ago).UnixMilli(), session, weekly)
}

// usageMachine is usageCLIConfig with cached readings on both homes and
// an unconfigured Claude home on disk.
func usageMachine(t *testing.T) *testkit.Fixture {
	f := testkit.New(t)
	f.WriteConfig(usageCLIConfig)
	f.WriteFile(filepath.Join(f.HomeDir(".claude-work"), ".claude.json"), cachedBody(2*time.Hour, 30, 10), 0o600)
	f.WriteFile(filepath.Join(f.HomeDir(".claude-side"), ".claude.json"), cachedBody(2*time.Hour, 5, 70), 0o600)
	f.HomeDir(".pi-home")
	f.WriteFile(filepath.Join(f.HomeDir(".claude-extra"), ".claude.json"), "{}", 0o600)
	return f
}

// TestUsageCachedJSONGolden pins usage --json (cached): rows, ages,
// pi's unsupported row and the unconfigured-home footer.
func TestUsageCachedJSONGolden(t *testing.T) {
	f := usageMachine(t)
	r := f.Run("usage", "--cached", "--tz", "UTC", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s %s", r.Code, r.Stdout, r.Stderr)
	}
	f.Golden(t, "usage-cached", r.Stdout)
}

// TestUsageCachedTableSort: --table prints one compact row per home and
// --sort orders them.
func TestUsageCachedTableSort(t *testing.T) {
	f := usageMachine(t)
	r := f.Run("usage", "--cached", "--agent", "claude", "--table", "--sort", "-weekly", "--tz", "UTC")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	// v0.3's table: # is the home's place in the file, sorted rows keep
	// it; the weekly summary follows.
	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "#  PROFILE  PROVIDER") || !strings.HasPrefix(lines[1], "2  side") ||
		!strings.HasPrefix(lines[2], "1  work") || lines[3] != "Weekly resets: Mon 2 (+80%)" {
		t.Errorf("table:\n%s", r.Stdout)
	}
	if !strings.Contains(r.Stderr, ".claude-extra is not in config") {
		t.Errorf("footer missing: %s", r.Stderr)
	}
	r = f.Run("usage", "--cached", "--profile", "work", "--tz", "UTC")
	if r.Code != 0 || !strings.Contains(r.Stdout, "cached 2 hr ago") || !strings.Contains(r.Stdout, "30% used  resets now") || strings.Contains(r.Stdout, "side") {
		t.Errorf("blocks: %d\n%s", r.Code, r.Stdout)
	}
	r = f.Run("usage", "--cached", "--profile", "work", "--tz", "UTC", "--time-format", "absolute")
	if r.Code != 0 || !strings.Contains(r.Stdout, "resets Fri 13 Nov 14:00") {
		t.Errorf("--time-format absolute: %d\n%s", r.Code, r.Stdout)
	}
	if r := f.Run("usage", "--cached", "--time-format", "weekly"); r.Code != 2 || !strings.Contains(r.Stderr, "use one of relative, day, absolute") {
		t.Errorf("bad --time-format: %d %s", r.Code, r.Stderr)
	}
}

// TestUsageBothAndDateFormatCLI: --time-format both and the date format
// through the real binary on a fixed clock (Fri 13 Nov 10:50 UTC; the
// readings reset at 14:00 the same day and Mon 16 Nov 06:00): config
// usage.time_format and date_format apply, usage.date_format beats the
// top-level key, and each flag beats both.
func TestUsageBothAndDateFormatCLI(t *testing.T) {
	f := usageMachine(t)
	f.Env[testkit.EnvTestNow] = "2026-11-13T10:50:00Z"
	run := func(args ...string) string {
		t.Helper()
		r := f.Run(append([]string{"usage", "--cached", "--profile", "work", "--tz", "UTC"}, args...)...)
		if r.Code != 0 {
			t.Fatalf("%v: %d %s", args, r.Code, r.Stderr)
		}
		return r.Stdout
	}
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"--time-format", "both"}, []string{"resets in 3 hr 10 min (14:00)", "resets in 2 d 19 hr (Mon 06:00)"}},
		{[]string{"--time-format", "both", "--date-format", "us"}, []string{"(2:00 PM)", "(Mon 6:00 AM)"}},
		{[]string{"--time-format", "both", "--table"}, []string{"3 hr 10 min (14:00)", "2 d 19 hr (Mon 06:00)", "Mon 16 Nov 06:00"}},
		{[]string{"--table", "--date-format", "iso"}, []string{"3 hr 10 min ", "2026-11-16 06:00"}},
	} {
		out := run(c.args...)
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v: want %q in\n%s", c.args, w, out)
			}
		}
	}
	// Config: both and the top-level date format, then usage.date_format
	// over it, then the flags over both keys.
	f.WriteConfig(strings.Replace(usageCLIConfig, `"version": 1,`, `"version": 1, "date_format": "iso", "usage": { "time_format": "both" },`, 1))
	if out := run("--table"); !strings.Contains(out, "3 hr 10 min (14:00)") || !strings.Contains(out, "2026-11-16 06:00") {
		t.Errorf("config date_format:\n%s", out)
	}
	f.WriteConfig(strings.Replace(usageCLIConfig, `"version": 1,`, `"version": 1, "date_format": "iso", "usage": { "time_format": "both", "date_format": "dddd h:mm a" },`, 1))
	if out := run("--table"); !strings.Contains(out, "3 hr 10 min (2:00 pm)") || !strings.Contains(out, "Monday 6:00 am") {
		t.Errorf("config usage.date_format:\n%s", out)
	}
	if out := run("--table", "--time-format", "relative", "--date-format", "short"); strings.Contains(out, "min (") || !strings.Contains(out, "Mon 06:00") {
		t.Errorf("flags over config:\n%s", out)
	}
}

// TestUsageCachedPartialExit3: one home's reading is unreadable, the
// other answers: exit 3, error.code partial (edge 57's partial case).
func TestUsageCachedPartialExit3(t *testing.T) {
	f := usageMachine(t)
	f.WriteFile(filepath.Join(f.Home, ".claude-side", ".claude.json"), `{"cachedUsageUtilization":{"fetchedAtMs":1,"utilization":{"limits":"broken"}}}`, 0o600)
	r := f.Run("usage", "--cached", "--agent", "claude", "--json")
	var env struct {
		OK    bool                  `json:"ok"`
		Data  []json.RawMessage     `json:"data"`
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
		t.Fatal(err, r.Stdout)
	}
	if r.Code != 3 || env.Error.Code != "partial" || env.OK || len(env.Data) != 2 {
		t.Fatalf("code %d envelope %s", r.Code, r.Stdout)
	}
	r = f.Run("usage", "--cached", "--agent", "claude")
	if r.Code != 3 || !strings.Contains(r.Stdout, "work") || !strings.Contains(r.Stderr, "1 of 2 rows failed: side (error)") {
		t.Errorf("human partial: code %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}
}

// TestUsageNoConfigShowsNothing is edge 2 for usage: exit 0, no rows.
func TestUsageNoConfigShowsNothing(t *testing.T) {
	f := testkit.New(t)
	r := f.Run("usage", "--cached", "--json", "--field", "data")
	if r.Code != 0 || strings.TrimSpace(r.Stdout) != "[]" {
		t.Fatalf("code %d stdout %q", r.Code, r.Stdout)
	}
}

// TestUsageRefusals: flag conflicts and the picker without a terminal
// are exit 2 with their codes.
func TestUsageRefusals(t *testing.T) {
	f := usageMachine(t)
	for _, c := range []struct {
		args []string
		code string
	}{
		{[]string{"--login-picker"}, "no-tty"},
		{[]string{"--cached", "--all-logins"}, "conflict"},
		{[]string{"--sort", "fast"}, "usage"},
		{[]string{"--tz", "Mars/Base"}, "usage"},
	} {
		r := f.Run(append([]string{"usage", "--json", "--field", "error.code"}, c.args...)...)
		if r.Code != 2 || strings.TrimSpace(r.Stdout) != c.code {
			t.Errorf("%v: code %d stdout %q", c.args, r.Code, r.Stdout)
		}
	}
}

// TestEdge13AutoProfileCLI: every candidate failed (an expired login, no
// login) is refused with each reason; nothing is requested.
func TestEdge13AutoProfileCLI(t *testing.T) {
	f := testkit.New(t)
	f.FakeAgent("claude")
	f.WriteConfig(`{"version":1,"profiles":{"a":{"agent":"claude","home":"~/.claude-a"},"b":{"agent":"claude","home":"~/.claude-b"}}}`)
	f.WriteFile(filepath.Join(f.HomeDir(".claude-a"), ".credentials.json"), `{"claudeAiOauth":{"accessToken":"fake-token-ALPHA","expiresAt":1000}}`, 0o600)
	f.HomeDir(".claude-b")
	r := f.Run("run", "--agent", "claude", "--auto-profile", "--dry-run")
	if r.Code != 125 || !strings.Contains(r.Stderr, "auto profile: none eligible: a (expired); b (not-logged-in)") {
		t.Fatalf("code %d stderr %s", r.Code, r.Stderr)
	}
}

// TestEdge14AutoProfileCLI: only per-token profiles is refused.
func TestEdge14AutoProfileCLI(t *testing.T) {
	f := testkit.New(t)
	f.FakeAgent("claude")
	f.WriteConfig(`{"version":1,"profiles":{"api":{"agent":"claude","home":"~/.claude-api","billing":"api"}}}`)
	f.HomeDir(".claude-api")
	r := f.Run("run", "--agent", "claude", "--auto-profile", "--dry-run")
	if r.Code != 125 || !strings.Contains(r.Stderr, "auto profile: every candidate bills per token") {
		t.Fatalf("code %d stderr %s", r.Code, r.Stderr)
	}
}

// TestEdge15AutoProfileCLI: an agent without usage is refused.
func TestEdge15AutoProfileCLI(t *testing.T) {
	f := testkit.New(t)
	f.FakeAgent("pi")
	f.WriteConfig(`{"version":1,"profiles":{"pi":{"agent":"pi","home":"~/.pi-home"}}}`)
	f.HomeDir(".pi-home")
	r := f.Run("run", "--agent", "pi", "--auto-profile", "--dry-run", "--json", "--field", "error.code")
	if r.Code != 125 || strings.TrimSpace(r.Stdout) != "unsupported" {
		t.Fatalf("code %d stdout %s", r.Code, r.Stdout)
	}
	r = f.Run("run", "--agent", "pi", "--auto-profile")
	if !strings.Contains(r.Stderr, "pi has no usage; choose a profile") {
		t.Errorf("stderr %s", r.Stderr)
	}
}
