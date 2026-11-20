package pool_test

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// now is every test's clock.
var now = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

// in is a reset that far from now.
func in(d time.Duration) *time.Time {
	t := now.Add(d)
	return &t
}

// day is 24 hours.
const day = 24 * time.Hour

// read is an ok reading with a weekly window (and a short window when
// short >= 0).
func read(short, week float64, reset time.Duration) pool.Reading {
	r := pool.Reading{Status: pool.StatusOK, Week: &pool.Window{Percent: week, ResetsAt: in(reset)}}
	if short >= 0 {
		r.Short = &pool.Window{Percent: short, ResetsAt: in(3 * time.Hour)}
	}
	return r
}

// status is a reading with only a status (no windows).
func status(s string) pool.Reading { return pool.Reading{Status: s} }

// f returns a pointer to v.
func f(v float64) *float64 { return &v }

// no is a pointer to false.
func no() *bool { b := false; return &b }

// classicNew is a pool under the classic preset with own members at the
// shared defaults (50/70): the rules the ported selector tests were
// written for.
func classicNew(own, shared []string) pool.Config {
	p, _ := pool.PresetNamed(pool.PresetClassic)
	c := pool.New(own, shared).WithRules(p.Rules)
	c.OwnDefaults = c.Defaults
	return c
}

// team is the flow's pool with fake names: alice, bob and carol are
// yours (caps 95/98), the rest are teammates' (default caps 50/70).
func team() pool.Config {
	c := classicNew([]string{"alice", "bob", "carol"}, []string{"derek", "erin", "frank", "grace", "heidi", "ivan"})
	c.Accounts = map[string]pool.Account{
		"alice": {Short: f(95), Week: f(98)},
		"bob":   {Short: f(95), Week: f(98)},
		"carol": {Short: f(95), Week: f(98)},
	}
	return c
}

// stateOf finds a candidate's state, rank and reasons.
func candidateOf(t *testing.T, r pool.Result, name string) pool.Candidate {
	t.Helper()
	for _, c := range r.Candidates {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no candidate %s", name)
	return pool.Candidate{}
}

// TestFlowExamples is TOKEN-SELECTION-FLOW section 6, A to E, with fake
// names: each case is the flow's accounts and the pick it documents.
func TestFlowExamples(t *testing.T) {
	cases := []struct {
		name     string
		cfg      func() pool.Config
		readings map[string]pool.Reading
		force    string
		want     string
		group    pool.Group
		reason   string
		check    func(t *testing.T, r pool.Result)
	}{
		{
			name: "A shared quota about to expire goes first",
			cfg:  team,
			readings: map[string]pool.Reading{
				"alice": read(-1, 29, 6*day),
				"carol": read(-1, 59, 60*time.Hour),
				"grace": read(0, 26, 38*time.Hour),
			},
			want: "grace", group: pool.GroupShared,
			reason: "shared account resets within 48h, before any usable own account; using its room before it is lost",
			check: func(t *testing.T, r pool.Result) {
				if g := candidateOf(t, r, "grace"); g.Rank != 1 || *g.Room != 44 {
					t.Errorf("grace rank %d room %v, want 1 and 44", g.Rank, *g.Room)
				}
				if c := candidateOf(t, r, "carol"); c.Rank != 2 || c.State != pool.StateReady {
					t.Errorf("carol %s rank %d, want ready 2", c.State, c.Rank)
				}
				if a := candidateOf(t, r, "alice"); a.Rank != 3 {
					t.Errorf("alice rank %d, want 3", a.Rank)
				}
			},
		},
		{
			name: "B shared account outside the window waits",
			cfg:  team,
			readings: map[string]pool.Reading{
				"alice": read(-1, 29, 6*day),
				"frank": read(0, 15, 4*day),
			},
			want: "alice", group: pool.GroupOwn, reason: "soonest weekly reset among usable accounts",
			check: func(t *testing.T, r pool.Result) {
				fr := candidateOf(t, r, "frank")
				if fr.State != pool.StateWaiting || fr.Rank != 0 || !slices.Contains(fr.Notes, "shared reset not within 48h while an own account is usable") {
					t.Errorf("frank %+v", fr)
				}
			},
		},
		{
			name: "C caps hold even on reset day",
			cfg:  team,
			readings: map[string]pool.Reading{
				"alice": read(-1, 29, 6*day),
				"ivan":  read(55, 43, 3*time.Hour),
			},
			want: "alice", group: pool.GroupOwn, reason: "soonest weekly reset among usable accounts",
			check: func(t *testing.T, r pool.Result) {
				iv := candidateOf(t, r, "ivan")
				if iv.State != pool.StateBlocked || !slices.Equal(iv.Reasons, []string{"5h is 55% (cap 50%)"}) {
					t.Errorf("ivan %s %v", iv.State, iv.Reasons)
				}
			},
		},
		{
			name: "D no own account usable",
			cfg:  team,
			readings: map[string]pool.Reading{
				"alice": status("limit-reached"),
				"bob":   status("limit-reached"),
				"carol": read(-1, 100, 3*day),
				"derek": read(0, 1, 7*day),
				"erin":  read(0, 3, 6*day),
			},
			want: "erin", group: pool.GroupShared, reason: "no own account is usable; soonest weekly reset among shared accounts",
			check: func(t *testing.T, r pool.Result) {
				if c := candidateOf(t, r, "carol"); !slices.Equal(c.Reasons, []string{"week is 100% (cap 98%)"}) {
					t.Errorf("carol reasons %v", c.Reasons)
				}
				if d := candidateOf(t, r, "derek"); d.Rank != 2 {
					t.Errorf("derek rank %d, want 2 (the window does not apply)", d.Rank)
				}
			},
		},
		{
			name: "E disabled: in A, carol is next",
			cfg: func() pool.Config {
				c := team()
				c.Accounts["grace"] = pool.Account{Enabled: no()}
				return c
			},
			readings: map[string]pool.Reading{
				"alice": read(-1, 29, 6*day),
				"carol": read(-1, 59, 60*time.Hour),
				"grace": read(0, 26, 38*time.Hour),
			},
			want: "carol", group: pool.GroupOwn, reason: "soonest weekly reset among usable accounts",
			check: func(t *testing.T, r pool.Result) {
				if g := candidateOf(t, r, "grace"); !slices.Equal(g.Reasons, []string{"disabled in config"}) {
					t.Errorf("grace reasons %v", g.Reasons)
				}
			},
		},
		{
			name:     "E forced by --force: every rule ignored, each a warning",
			cfg:      team,
			readings: map[string]pool.Reading{"alice": read(-1, 29, 6*day), "bob": status("limit-reached")},
			force:    "bob",
			want:     "bob", group: pool.GroupOwn, reason: "forced by --pool-force; all rules ignored",
			check: func(t *testing.T, r pool.Result) {
				if s := r.Selected; !s.Forced || s.ForcedBy != pool.ForcedByFlag || !slices.Equal(s.Warnings, []string{"status is limit-reached"}) {
					t.Errorf("selection %+v", s)
				}
			},
		},
		{
			name: "E --force wins over config force",
			cfg: func() pool.Config {
				c := team()
				c.Force = "alice"
				return c
			},
			readings: map[string]pool.Reading{"alice": read(-1, 29, 6*day), "carol": read(-1, 10, day)},
			force:    "carol",
			want:     "carol", group: pool.GroupOwn, reason: "forced by --pool-force; all rules ignored",
		},
		{
			name: "E config force",
			cfg: func() pool.Config {
				c := team()
				c.Force = "alice"
				return c
			},
			readings: map[string]pool.Reading{"alice": read(-1, 29, 6*day), "carol": read(-1, 10, day)},
			want:     "alice", group: pool.GroupOwn, reason: `forced by config "force"; all rules ignored`,
		},
		{
			name:     "E forcing an unlisted account works, with a warning",
			cfg:      team,
			readings: map[string]pool.Reading{"alice": read(-1, 29, 6*day), "zoe": read(60, 10, day)},
			force:    "zoe",
			want:     "zoe", group: pool.GroupUnlisted, reason: "forced by --pool-force; all rules ignored",
			check: func(t *testing.T, r pool.Result) {
				want := []string{"not listed in own or shared", "5h is 60% (cap 50%)"}
				if !slices.Equal(r.Selected.Warnings, want) {
					t.Errorf("warnings %v, want %v", r.Selected.Warnings, want)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg()
			if err := cfg.Validate(); err != nil {
				t.Fatal(err)
			}
			r, err := pool.Select(cfg, tc.readings, pool.Options{Now: now, Force: tc.force})
			if err != nil {
				t.Fatal(err)
			}
			if r.Selected == nil {
				t.Fatalf("nothing selected; candidates %+v", r.Candidates)
			}
			if r.Selected.Name != tc.want || r.Selected.Group != tc.group || r.Selected.Reason != tc.reason {
				t.Errorf("selected %s (%s) %q, want %s (%s) %q", r.Selected.Name, r.Selected.Group, r.Selected.Reason, tc.want, tc.group, tc.reason)
			}
			if tc.check != nil {
				tc.check(t, r)
			}
		})
	}
}

// TestUsableChecksListEveryReason: section 4: every reason is listed,
// not the first; caps are exclusive (< cap usable, >= cap not).
func TestUsableChecksListEveryReason(t *testing.T) {
	cfg := classicNew([]string{"alice"}, []string{"derek", "erin", "frank"})
	cfg.Accounts = map[string]pool.Account{"derek": {Enabled: no()}}
	readings := map[string]pool.Reading{
		"derek": {Status: "limit-reached", Short: &pool.Window{Percent: 99.5}, Week: &pool.Window{Percent: 70}},
		"erin":  read(49.9, 69.9, day),
		"frank": read(50, 70, day),
	}
	r, err := pool.Select(cfg, readings, pool.Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"disabled in config", "status is limit-reached", "5h is 99.5% (unavailable at 99%)", "5h is 99.5% (cap 50%)", "week is 70% (cap 70%)"}
	if d := candidateOf(t, r, "derek"); !slices.Equal(d.Reasons, want) {
		t.Errorf("derek reasons\n got %v\nwant %v", d.Reasons, want)
	}
	if a := candidateOf(t, r, "alice"); !slices.Equal(a.Reasons, []string{"no usage reading"}) || a.Status != "missing" {
		t.Errorf("alice %s %v", a.Status, a.Reasons)
	}
	if e := candidateOf(t, r, "erin"); !e.Eligible || e.Rank != 1 {
		t.Errorf("erin %+v, want ready 1 (49.9 < 50, 69.9 < 70)", e)
	}
	if fr := candidateOf(t, r, "frank"); !slices.Equal(fr.Reasons, []string{"5h is 50% (cap 50%)", "week is 70% (cap 70%)"}) {
		t.Errorf("frank reasons %v", fr.Reasons)
	}
	if r.Selected == nil || r.Selected.Name != "erin" {
		t.Errorf("selected %+v", r.Selected)
	}
}

// TestShortCutoffAppliesToOwnAccounts: the 99% short cutoff blocks even
// an own account whose short cap is 100.
func TestShortCutoffAppliesToOwnAccounts(t *testing.T) {
	cfg := classicNew([]string{"alice"}, nil)
	cfg.Accounts = map[string]pool.Account{"alice": {Short: f(100), Week: f(100)}}
	r, _ := pool.Select(cfg, map[string]pool.Reading{"alice": read(99, 10, day)}, pool.Options{Now: now})
	if r.Selected != nil || !slices.Equal(r.Candidates[0].Reasons, []string{"5h is 99% (unavailable at 99%)"}) {
		t.Errorf("selected %+v reasons %v", r.Selected, r.Candidates[0].Reasons)
	}
}

// TestNoneUsable: an empty pool selects nothing and is not an error.
func TestNoneUsable(t *testing.T) {
	cfg := classicNew([]string{"alice"}, []string{"derek"})
	r, err := pool.Select(cfg, map[string]pool.Reading{"alice": status("rejected"), "derek": read(0, 80, day)}, pool.Options{Now: now})
	if err != nil || r.Selected != nil {
		t.Fatalf("selected %+v err %v", r.Selected, err)
	}
	if len(r.Candidates) != 2 || r.Candidates[0].State != pool.StateBlocked {
		t.Errorf("candidates %+v", r.Candidates)
	}
}

// TestOrdering: soonest reset first; ties keep config order, own before
// shared; an unknown reset and a passed (stale) reset sort last, the
// passed one noted.
func TestOrdering(t *testing.T) {
	cfg := classicNew([]string{"alice", "bob", "carol"}, []string{"derek"})
	cfg.BorrowWindow = 7 * day
	unknown := pool.Reading{Status: pool.StatusOK, Week: &pool.Window{Percent: 1}}
	readings := map[string]pool.Reading{
		"alice": read(-1, 1, -2*time.Hour),
		"bob":   unknown,
		"carol": read(-1, 1, day),
		"derek": read(0, 1, day),
	}
	r, err := pool.Select(cfg, readings, pool.Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, c := range r.Candidates {
		order = append(order, c.Name)
	}
	if want := []string{"carol", "derek", "alice", "bob"}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
	if a := candidateOf(t, r, "alice"); !slices.Contains(a.Notes, "weekly reset time has passed; usage data is stale") {
		t.Errorf("alice notes %v", a.Notes)
	}
}

// TestBorrowWindowZeroNeverBorrows: window 0 holds every shared account
// back while an own account is usable, even one resetting now.
func TestBorrowWindowZeroNeverBorrows(t *testing.T) {
	cfg := classicNew([]string{"alice"}, []string{"derek"})
	cfg.BorrowWindow = 0
	r, _ := pool.Select(cfg, map[string]pool.Reading{"alice": read(0, 1, 6*day), "derek": read(0, 1, time.Minute)}, pool.Options{Now: now})
	if r.Selected.Name != "alice" || candidateOf(t, r, "derek").State != pool.StateWaiting {
		t.Errorf("selected %s derek %s", r.Selected.Name, candidateOf(t, r, "derek").State)
	}
	if n := candidateOf(t, r, "derek").Notes; !slices.Contains(n, "shared reset not within 0h while an own account is usable") {
		t.Errorf("notes %v", n)
	}
}

// TestForcedMissingIsAnError: forcing an account without a reading is
// ErrForcedMissing (exit 1 in agx), not a silent fallback.
func TestForcedMissingIsAnError(t *testing.T) {
	_, err := pool.Select(team(), map[string]pool.Reading{"alice": read(0, 1, day)}, pool.Options{Now: now, Force: "nobody"})
	if !errors.Is(err, pool.ErrForcedMissing) || !strings.Contains(err.Error(), "nobody") {
		t.Errorf("err %v", err)
	}
}

// TestValidate: names once across own/shared, no empty name, accounts
// only for listed names, percents in 0..100, no negative window.
func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		edit func(c *pool.Config)
		want string
	}{
		{"ok", func(*pool.Config) {}, ""},
		{"twice", func(c *pool.Config) { c.Shared = append(c.Shared, "alice") }, `account "alice" is listed more than once`},
		{"empty", func(c *pool.Config) { c.Own = append(c.Own, "") }, "an account name is empty"},
		{"unlisted account", func(c *pool.Config) { c.Accounts["zoe"] = pool.Account{} }, "accounts.zoe is not listed"},
		{"cap over 100", func(c *pool.Config) { c.Accounts["bob"] = pool.Account{Week: f(101)} }, "accounts.bob.week 101 is outside 0..100"},
		{"default under 0", func(c *pool.Config) { c.Defaults.Short = -1 }, "defaults.short -1 is outside 0..100"},
		{"cutoff", func(c *pool.Config) { c.ShortUnavailableAt = 120 }, "short_unavailable_at_percent 120"},
		{"negative window", func(c *pool.Config) { c.BorrowWindow = -time.Hour }, "borrow window is negative"},
		{"NaN cap", func(c *pool.Config) { c.Defaults.Week = math.NaN() }, "defaults.week NaN is outside 0..100"},
		{"NaN account cap", func(c *pool.Config) { c.Accounts["bob"] = pool.Account{Short: f(math.NaN())} }, "accounts.bob.short NaN is outside 0..100"},
		{"infinite cutoff", func(c *pool.Config) { c.ShortUnavailableAt = math.Inf(1) }, "short_unavailable_at_percent +Inf is outside 0..100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := team()
			tc.edit(&c)
			err := c.Validate()
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("err %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("err %v, want %q", err, tc.want)
			}
		})
	}
}

// TestHours: whole hours print as "48h", anything else as Go's form.
func TestHours(t *testing.T) {
	if got := pool.Hours(48 * time.Hour); got != "48h" {
		t.Errorf("%s", got)
	}
	if got := pool.Hours(90 * time.Minute); got != "1h30m0s" {
		t.Errorf("%s", got)
	}
}

// TestPerUseKeyNeverPicked: a member marked PerUseKey (an API key under a
// login declared oauth-token that does not allow it) is blocked with one
// reason however low its usage, and the next member is picked.
func TestPerUseKeyNeverPicked(t *testing.T) {
	cfg := classicNew([]string{"alice", "bob"}, nil)
	key := read(1, 1, day)
	key.PerUseKey = true
	r, err := pool.Select(cfg, map[string]pool.Reading{"alice": key, "bob": read(40, 60, day)}, pool.Options{Now: now})
	if err != nil || r.Selected == nil || r.Selected.Name != "bob" {
		t.Fatalf("selected %+v err %v", r.Selected, err)
	}
	for _, c := range r.Candidates {
		if c.Name == "alice" && (c.Eligible || len(c.Reasons) != 1 || c.Reasons[0] != "token is an API key (billed per use); allow_api_key is off") {
			t.Fatalf("alice = %+v", c)
		}
	}
}
