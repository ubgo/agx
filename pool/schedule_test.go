package pool

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// zone loads an IANA zone or fails the test.
func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// at is a local time in loc.
func at(loc *time.Location, y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, loc)
}

// fp is a pointer to v.
func fp(v float64) *float64 { return &v }

// days is a Days array of the named days.
func days(t *testing.T, names ...string) [7]bool {
	t.Helper()
	d, err := ParseDays(names)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// hours is a schedule's Start and End.
func hours(t *testing.T, s string) (int, int) {
	t.Helper()
	a, b, err := ParseHours(s)
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

// TestParseDays: day names in any case, at least one, unknown refused.
func TestParseDays(t *testing.T) {
	d, err := ParseDays([]string{"Sat", "sun"})
	if err != nil || !d[time.Saturday] || !d[time.Sunday] || d[time.Monday] {
		t.Fatalf("%v %v", d, err)
	}
	for _, bad := range [][]string{nil, {"saturday"}, {"mon", "xyz"}, {""}} {
		if _, err := ParseDays(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if got := strings.Join(DayNames(), ","); got != "mon,tue,wed,thu,fri,sat,sun" {
		t.Errorf("day names %s", got)
	}
}

// TestParseHours: HH:MM-HH:MM, the whole day by default, 24:00 only as
// an end, an empty window refused.
func TestParseHours(t *testing.T) {
	for _, tc := range []struct {
		in         string
		start, end int
	}{
		{"", 0, 1440}, {"00:00-24:00", 0, 1440}, {"09:00-18:30", 540, 1110}, {"22:00-06:00", 1320, 360}, {"23:59-00:00", 1439, 0},
	} {
		s, e, err := ParseHours(tc.in)
		if err != nil || s != tc.start || e != tc.end {
			t.Errorf("%q: %d %d %v", tc.in, s, e, err)
		}
	}
	for _, bad := range []string{"24:00-01:00", "09:00-09:00", "00:00-00:00", "9:00-10:00", "09:60-10:00", "0900", "09:00-24:01", "09:00-25:00", "aa:bb-cc:dd", "-1:00-02:00"} {
		if _, _, err := ParseHours(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// TestScheduleWeekend is the weekend schedule in Istanbul: active from
// Saturday 00:00 to Sunday 19:00 (Monday 00:00 less release_before 5h),
// the two days joined into one window.
func TestScheduleWeekend(t *testing.T) {
	ist := zone(t, "Europe/Istanbul")
	s := Schedule{Name: "weekend", Days: days(t, "sat", "sun"), Start: 0, End: 1440, Members: []string{"shared"}, ReleaseBefore: 5 * time.Hour}
	end := at(ist, 2026, 11, 22, 19, 0)
	for _, tc := range []struct {
		now    time.Time
		active bool
	}{
		{at(ist, 2026, 11, 20, 23, 59), false}, // Friday night
		{at(ist, 2026, 11, 21, 0, 0), true},    // Saturday starts it
		{at(ist, 2026, 11, 21, 13, 0), true},
		{at(ist, 2026, 11, 22, 18, 59), true},
		{at(ist, 2026, 11, 22, 19, 0), false}, // released 5h before Monday
		{at(ist, 2026, 11, 23, 0, 0), false},  // Monday
		{at(ist, 2026, 11, 23, 9, 0), false},
	} {
		ok, until := s.ActiveUntil(tc.now, ist)
		if ok != tc.active {
			t.Errorf("%s: active %v", tc.now, ok)
		}
		if ok && (until == nil || !until.Equal(end)) {
			t.Errorf("%s: until %v, want %s", tc.now, until, end)
		}
	}
	// The pool's zone applies to a schedule without its own; the same
	// UTC moment is Saturday in Istanbul and Friday in New York.
	s.ReleaseBefore = 0
	utc := time.Date(2026, 11, 21, 2, 0, 0, 0, time.UTC)
	if ok, _ := s.ActiveUntil(utc, ist); !ok {
		t.Error("Saturday 05:00 in Istanbul not active with the pool zone")
	}
	if ok, _ := s.ActiveUntil(utc, zone(t, "America/New_York")); ok {
		t.Error("Friday 21:00 in New York active")
	}
	s.Location = zone(t, "America/New_York")
	if ok, _ := s.ActiveUntil(utc, ist); ok {
		t.Error("the schedule's own zone does not beat the pool's")
	}
}

// TestScheduleMidnight: a window that crosses midnight belongs to the
// day it starts on.
func TestScheduleMidnight(t *testing.T) {
	loc := time.UTC
	st, en := hours(t, "22:00-06:00")
	s := Schedule{Days: days(t, "fri"), Start: st, End: en}
	for _, tc := range []struct {
		now    time.Time
		active bool
	}{
		{at(loc, 2026, 11, 19, 23, 0), false}, // Thursday night
		{at(loc, 2026, 11, 20, 21, 59), false},
		{at(loc, 2026, 11, 20, 22, 0), true},
		{at(loc, 2026, 11, 21, 5, 59), true}, // Saturday morning, Friday's window
		{at(loc, 2026, 11, 21, 6, 0), false},
		{at(loc, 2026, 11, 21, 23, 0), false}, // Saturday night is not listed
	} {
		ok, until := s.ActiveUntil(tc.now, loc)
		if ok != tc.active {
			t.Errorf("%s: active %v", tc.now, ok)
		}
		if ok && !until.Equal(at(loc, 2026, 11, 21, 6, 0)) {
			t.Errorf("%s: until %v", tc.now, until)
		}
	}
	// Back-to-back nights join only when they touch: 18:00-09:00 on
	// every weekday leaves 09:00-18:00 out, so each night ends at 09:00.
	st, en = hours(t, "18:00-09:00")
	s = Schedule{Days: days(t, "mon", "tue", "wed", "thu", "fri"), Start: st, End: en}
	if ok, until := s.ActiveUntil(at(loc, 2026, 11, 17, 2, 0), loc); !ok || !until.Equal(at(loc, 2026, 11, 17, 9, 0)) {
		t.Errorf("Tuesday 02:00: %v %v", ok, until)
	}
	// 12:00-12:00 the next day on consecutive days joins into one.
	s = Schedule{Days: days(t, "mon", "tue"), Start: 720, End: 720}
	if ok, until := s.ActiveUntil(at(loc, 2026, 11, 17, 2, 0), loc); !ok || !until.Equal(at(loc, 2026, 11, 18, 12, 0)) {
		t.Errorf("joined: %v %v", ok, until)
	}
}

// TestScheduleAlways: every day, all day has no end, so release_before
// never applies.
func TestScheduleAlways(t *testing.T) {
	s := Schedule{Days: days(t, DayNames()...), Start: 0, End: 1440, ReleaseBefore: 5 * time.Hour}
	ok, until := s.ActiveUntil(time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC), time.UTC)
	if !ok || until != nil {
		t.Errorf("%v %v", ok, until)
	}
}

// TestScheduleReleaseLonger: a release_before as long as the window
// leaves nothing.
func TestScheduleReleaseLonger(t *testing.T) {
	st, en := hours(t, "10:00-14:00")
	s := Schedule{Days: days(t, "sat"), Start: st, End: en, ReleaseBefore: 4 * time.Hour}
	if ok, _ := s.ActiveUntil(time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC), time.UTC); ok {
		t.Error("active though released before it starts")
	}
	s.ReleaseBefore = 3 * time.Hour
	if ok, _ := s.ActiveUntil(time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC), time.UTC); !ok {
		t.Error("not active in its first hour")
	}
}

// TestScheduleDST: a whole Sunday is 25 hours when the clocks go back
// and 23 when they go forward; the window follows the wall clock.
func TestScheduleDST(t *testing.T) {
	ny := zone(t, "America/New_York")
	s := Schedule{Days: days(t, "sun"), Start: 0, End: 1440, Location: ny}
	for _, tc := range []struct {
		day  time.Time
		long time.Duration
	}{
		{at(ny, 2026, 11, 1, 12, 0), 25 * time.Hour}, // clocks go back
		{at(ny, 2026, 3, 8, 12, 0), 23 * time.Hour},  // clocks go forward
		{at(ny, 2026, 11, 8, 12, 0), 24 * time.Hour},
	} {
		sp, in, ends := s.window(tc.day, nil)
		if !in || !ends {
			t.Fatalf("%s: in %v ends %v", tc.day, in, ends)
		}
		if got := sp.to.Sub(sp.from); got != tc.long {
			t.Errorf("%s: window %s, want %s", tc.day, got, tc.long)
		}
		if h, m, _ := sp.to.In(ny).Clock(); h != 0 || m != 0 {
			t.Errorf("%s: ends at %s, not midnight", tc.day, sp.to.In(ny))
		}
	}
	// 01:30-03:00 on the day the clocks go forward: 02:00-03:00 does not
	// exist, the window is half an hour of real time.
	st, en := hours(t, "01:30-03:00")
	s = Schedule{Days: days(t, "sun"), Start: st, End: en, Location: ny}
	sp, in, _ := s.window(at(ny, 2026, 3, 8, 1, 45), nil)
	if !in || sp.to.Sub(sp.from) != 30*time.Minute {
		t.Errorf("spring gap: in %v, %s", in, sp.to.Sub(sp.from))
	}
	if ok, _ := s.ActiveUntil(time.Date(2026, 3, 8, 7, 15, 0, 0, time.UTC), time.UTC); ok {
		t.Error("active after its real end (03:15 EDT)")
	}
}

// timedPool is own alice, shared carol and derek, with a weekend
// schedule for shared and one for derek by name.
func timedPool(t *testing.T) Config {
	t.Helper()
	c := New([]string{"alice"}, []string{"carol", "derek"})
	c.Location = time.UTC
	st, en := hours(t, "")
	c.Schedules = []Schedule{
		{Name: "weekend", Days: days(t, "sat", "sun"), Start: st, End: en, Members: []string{"shared"}, Caps: ScheduleCaps{Short: fp(90), Week: fp(75)}},
		{Name: "derek-sat", Days: days(t, "sat"), Start: st, End: en, Members: []string{"derek"}, Caps: ScheduleCaps{Short: fp(60)}},
	}
	return c
}

// saturday is Saturday 21 November 2026, 10:00 UTC.
var saturday = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

// TestTimedPrecedence: a schedule that names the member beats one that
// names its group, cap by cap; an override beats both; the highest of
// one level wins.
func TestTimedPrecedence(t *testing.T) {
	c := timedPool(t)
	got := c.At(saturday)
	if caps := got.CapsFor("carol"); caps != (Caps{Short: 90, Week: 75}) {
		t.Errorf("carol (group schedule) %+v", caps)
	}
	// derek-sat names derek: its short 60 beats the group's 90, and the
	// week it leaves out comes from the group schedule.
	if caps := got.CapsFor("derek"); caps != (Caps{Short: 60, Week: 75}) {
		t.Errorf("derek %+v", caps)
	}
	if tm := got.Timed("derek"); tm.Short.Name != "derek-sat" || tm.Week.Name != "weekend" || tm.WeekRelaxMax != nil {
		t.Errorf("derek sources %+v %+v", tm.Short, tm.Week)
	}
	if caps := got.CapsFor("alice"); caps != (Caps{Short: DefaultOwnShortCap, Week: DefaultOwnWeekCap}) {
		t.Errorf("own alice changed %+v", caps)
	}
	if got.Timed("alice").any() {
		t.Error("alice has timed caps")
	}
	// Weekday: nothing applies.
	if caps := c.At(saturday.Add(-48 * time.Hour)).CapsFor("carol"); caps != (Caps{Short: DefaultShortCap, Week: DefaultWeekCap}) {
		t.Errorf("Thursday carol %+v", caps)
	}

	// Two group schedules: the most permissive value wins, each cap on
	// its own.
	c.Schedules = append(c.Schedules, Schedule{Name: "late", Days: days(t, "sat"), Start: 0, End: 1440, Members: []string{"shared"}, Caps: ScheduleCaps{Short: fp(80), Week: fp(85)}})
	got = c.At(saturday)
	if caps := got.CapsFor("carol"); caps != (Caps{Short: 90, Week: 85}) {
		t.Errorf("most permissive %+v", caps)
	}
	if tm := got.Timed("carol"); tm.Short.Name != "weekend" || tm.Week.Name != "late" {
		t.Errorf("sources %s %s", tm.Short.Name, tm.Week.Name)
	}
	// A tie keeps config order.
	c.Schedules[2].Caps.Short = fp(90)
	if tm := c.At(saturday).Timed("carol"); tm.Short.Name != "weekend" {
		t.Errorf("tie went to %s", tm.Short.Name)
	}

	// An override beats every schedule, for the caps it sets.
	until := saturday.Add(6 * time.Hour)
	c.Accounts = map[string]Account{"derek": {Short: fp(40), Override: &Override{Caps: ScheduleCaps{Short: fp(95)}, Until: until}}}
	got = c.At(saturday)
	if caps := got.CapsFor("derek"); caps != (Caps{Short: 95, Week: 85}) {
		t.Errorf("override %+v", caps)
	}
	if tm := got.Timed("derek"); tm.Short.Kind != SourceOverride || !tm.Short.Until.Equal(until) {
		t.Errorf("override source %+v", tm.Short)
	}
	if _, ok := got.ExpiredOverride("derek"); ok {
		t.Error("active override reported expired")
	}
	// Expired: ignored (the schedules and the member's own cap apply),
	// and reported.
	got = c.At(until)
	if caps := got.CapsFor("derek"); caps.Short != 60 {
		t.Errorf("expired override still applies: %+v", caps)
	}
	if end, ok := got.ExpiredOverride("derek"); !ok || !end.Equal(until) {
		t.Errorf("expired %v %v", end, ok)
	}
	// The member's own cap is back once nothing timed applies.
	if caps := c.At(until.Add(48 * time.Hour)).CapsFor("derek"); caps.Short != 40 {
		t.Errorf("Monday derek %+v", caps)
	}
	// At leaves the caller's map alone.
	if *c.Accounts["derek"].Short != 40 {
		t.Error("At changed the config's accounts")
	}
}

// TestTimedSelect: Select applies the schedule (a teammate's account at
// 62% of its 5-hour window is usable on Saturday, blocked on Friday), and
// reports the timed caps and an ended override.
func TestTimedSelect(t *testing.T) {
	c := New(nil, []string{"carol", "erin"})
	c.Location = time.UTC
	c.Schedules = []Schedule{{Name: "weekend", Days: days(t, "sat", "sun"), Start: 0, End: 1440, Members: []string{"shared"}, Caps: ScheduleCaps{Short: fp(95)}, ReleaseBefore: 5 * time.Hour}}
	ended := saturday.Add(-time.Hour)
	c.Accounts = map[string]Account{"erin": {Override: &Override{Caps: ScheduleCaps{Week: fp(99)}, Until: ended}}}
	reset := saturday.Add(72 * time.Hour)
	rd := map[string]Reading{
		"carol": {Status: StatusOK, Short: &Window{Percent: 62}, Week: &Window{Percent: 40, ResetsAt: &reset}},
		"erin":  {Status: StatusOK, Short: &Window{Percent: 10}, Week: &Window{Percent: 72, ResetsAt: &reset}},
	}
	res, err := Select(c, rd, Options{Now: saturday})
	if err != nil {
		t.Fatal(err)
	}
	if res.Selected == nil || res.Selected.Name != "carol" {
		t.Fatalf("selected %+v", res.Selected)
	}
	carol, erin := res.Candidates[0], res.Candidates[1]
	if carol.Caps.Short != 95 || carol.Timed == nil || carol.Timed.Short.Name != "weekend" {
		t.Errorf("carol %+v %+v", carol.Caps, carol.Timed)
	}
	if want := time.Date(2026, 11, 22, 19, 0, 0, 0, time.UTC); !carol.Timed.Short.Until.Equal(want) {
		t.Errorf("until %s", carol.Timed.Short.Until)
	}
	if erin.Eligible || erin.ExpiredOverride == nil || !strings.Contains(strings.Join(erin.Notes, ";"), "override ended") {
		t.Errorf("erin %+v", erin)
	}
	friday := saturday.Add(-24 * time.Hour)
	res, err = Select(c, rd, Options{Now: friday})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Candidates {
		if c.Name == "carol" && (c.Eligible || c.Timed != nil || c.Caps.Short != DefaultShortCap) {
			t.Errorf("Friday carol %+v", c)
		}
	}
	// --pool-only reads the timed caps too: a stricter one-run cap is
	// measured against 95, not 50.
	res, err = Select(c, rd, Options{Now: saturday, Only: "carol", OnlyCaps: Tighter{Short: fp(70)}})
	if err != nil || res.Selected == nil {
		t.Fatalf("only: %v %+v", err, res.Selected)
	}
	if _, err := Select(c, rd, Options{Now: friday, Only: "carol", OnlyCaps: Tighter{Short: fp(70)}}); !errors.Is(err, ErrCapRaised) {
		t.Errorf("Friday 70 over cap 50: %v", err)
	}
}

// TestTimedValidate: every schedule and override rule.
func TestTimedValidate(t *testing.T) {
	good := func() Config { return timedPool(t) }
	if err := good().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		mod  func(*Config)
		want string
	}{
		{"no name", func(c *Config) { c.Schedules[0].Name = " " }, "has no name"},
		{"duplicate name", func(c *Config) { c.Schedules[1].Name = "weekend" }, "more than once"},
		{"no days", func(c *Config) { c.Schedules[0].Days = [7]bool{} }, "no days"},
		{"bad hours", func(c *Config) { c.Schedules[0].Start = 1440 }, "hours"},
		{"empty window", func(c *Config) { c.Schedules[0].Start, c.Schedules[0].End = 60, 60 }, "hours"},
		{"no members", func(c *Config) { c.Schedules[0].Members = nil }, "names no members"},
		{"unknown member", func(c *Config) { c.Schedules[0].Members = []string{"frank"} }, `"frank"`},
		{"no caps", func(c *Config) { c.Schedules[0].Caps = ScheduleCaps{} }, "sets no caps"},
		{"cap over 100", func(c *Config) { c.Schedules[0].Caps.Week = fp(101) }, "outside 0..100"},
		{"NaN cap", func(c *Config) { c.Schedules[0].Caps.Short = fp(math.NaN()) }, "outside 0..100"},
		{"negative cap", func(c *Config) { c.Schedules[1].Caps.WeekRelaxMax = fp(-1) }, "outside 0..100"},
		{"negative release", func(c *Config) { c.Schedules[0].ReleaseBefore = -time.Hour }, "release_before"},
		{"override without until", func(c *Config) {
			c.Accounts = map[string]Account{"carol": {Override: &Override{Caps: ScheduleCaps{Short: fp(90)}}}}
		}, "no until"},
		{"override without caps", func(c *Config) {
			c.Accounts = map[string]Account{"carol": {Override: &Override{Until: saturday}}}
		}, "sets no caps"},
		{"override cap NaN", func(c *Config) {
			c.Accounts = map[string]Account{"carol": {Override: &Override{Caps: ScheduleCaps{Week: fp(math.NaN())}, Until: saturday}}}
		}, "outside 0..100"},
	} {
		c := good()
		tc.mod(&c)
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	// A member and a group are both fine.
	c := good()
	c.Schedules[0].Members = []string{"alice", "own", "carol"}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}

// TestParseUntil: every form, in the pool's zone, always in the future.
func TestParseUntil(t *testing.T) {
	ist := zone(t, "Europe/Istanbul")
	now := at(ist, 2026, 11, 21, 13, 0) // Saturday
	for _, tc := range []struct {
		in   string
		want time.Time
	}{
		{"6h", now.Add(6 * time.Hour)},
		{"90m", now.Add(90 * time.Minute)},
		{"2026-11-23T08:00", at(ist, 2026, 11, 23, 8, 0)},
		{"2026-11-23 08:00", at(ist, 2026, 11, 23, 8, 0)},
		{"2026-11-23T08:00:00Z", time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)},
		{"2026-11-23", at(ist, 2026, 11, 24, 0, 0)},
		{"sun 23:59", at(ist, 2026, 11, 22, 23, 59)},
		{"Sun 23:59", at(ist, 2026, 11, 22, 23, 59)},
		{"sat 14:00", at(ist, 2026, 11, 21, 14, 0)}, // later today
		{"sat 10:00", at(ist, 2026, 11, 28, 10, 0)}, // passed today: next week
		{"18:00", at(ist, 2026, 11, 21, 18, 0)},
		{"12:00", at(ist, 2026, 11, 22, 12, 0)},
	} {
		got, err := ParseUntil(tc.in, now, ist)
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("%q: %s %v, want %s", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"", "-1h", "0s", "2026-11-20T08:00", "2026-11-20", "someday 10:00", "sun 25:00", "tomorrow", "1300"} {
		if _, err := ParseUntil(bad, now, ist); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// nil = local time, never an error.
	if _, err := ParseUntil("1h", now, nil); err != nil {
		t.Error(err)
	}
}
