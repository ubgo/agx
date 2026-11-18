package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/testkit"
)

// The usage views are pinned against agx v0.3's look (the owner's
// reference, docsi/memory/v03-output-is-reference.md): the same scenario
// rendered by v0.3's own code is in the REPORT of the restore, and the
// goldens below must keep its columns, words, relative times and colours.

// lookLoc and lookNow are the reference screen's zone and clock: Wed 18
// Nov 2026 17:50 in Asia/Kolkata, so "5 d 11 hr" and "Tue 24 Nov 05:30"
// read exactly as on the owner's screen.
func lookClock(t *testing.T) (*time.Location, time.Time) {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	return loc, time.Date(2026, 11, 18, 17, 50, 0, 0, loc)
}

// at is now plus d, as a pointer for ResetsAt.
func at(now time.Time, d time.Duration) *time.Time {
	t := now.Add(d)
	return &t
}

// pct is a pointer for Left.
func pct(f float64) *float64 { return &f }

// Window lengths of the scenario, from now.
const (
	noraShort = 2*time.Hour + 50*time.Minute
	noraWeek  = 5*24*time.Hour + 11*time.Hour + 40*time.Minute
	ataShort  = 40 * time.Minute
	ataWeek   = 3*time.Hour + 40*time.Minute
)

// lookLogins is the owner's reference table as logins (fake accounts):
// nora fine, miriam at its cap, carol refused, key billed per token.
func lookLogins(now time.Time) []app.UsageRow {
	return []app.UsageRow{
		{Kind: app.RowLogin, Index: 2, Name: "nora", Agent: "claude", Login: "nora", Account: "nora@example.com", Source: app.SourceProbe, Status: app.StatusOK, Left: pct(51),
			Windows: []app.UsageWindow{
				{Class: "short", Label: "Current session", Percent: 13, ResetsAt: at(now, noraShort)},
				{Class: "week", Label: "All models", Percent: 49, ResetsAt: at(now, noraWeek)},
			}},
		{Kind: app.RowLogin, Index: 13, Name: "miriam", Agent: "claude", Login: "miriam", Account: "miriam@example.com", Source: app.SourceProbe, Status: app.StatusLimit, Left: pct(0),
			Windows: []app.UsageWindow{
				{Class: "short", Label: "Current session", Percent: 110, ResetsAt: at(now, ataShort)},
				{Class: "week", Label: "All models", Percent: 93, ResetsAt: at(now, ataWeek)},
			}},
		{Kind: app.RowLogin, Index: 5, Name: "carol", Agent: "claude", Login: "carol", Account: "carol@example.com", Source: app.SourceNone, Status: app.StatusRejected,
			Reason: "the server refused the credential (HTTP 401): token revoked or from another organization", Windows: []app.UsageWindow{}},
		{Kind: app.RowLogin, Index: 7, Name: "key", Agent: "claude", Login: "key", Source: app.SourceProbe, Status: app.StatusPerToken, Windows: []app.UsageWindow{}},
		// The reason as internal/app gives it: agentio's "agentio: " prefix
		// already stripped (TestClassifyUsageErrStripsLibraryPrefix).
		{Kind: app.RowLogin, Index: 9, Name: "oskar", Agent: "claude", Login: "oskar", Account: "oskar@example.com", Source: app.SourceNone, Status: app.StatusExpired,
			Reason: "login expired at 2026-11-17T09:00:00Z: run claude once in this home to refresh it", Windows: []app.UsageWindow{}},
	}
}

// lookHomes is the default view's homes: one with a per-model window, one
// at its cap, one logged out.
func lookHomes(now time.Time) []app.UsageRow {
	return []app.UsageRow{
		{Kind: app.RowHome, Index: 1, Name: "personal", Agent: "claude", Home: "~/.claude", Account: "me@example.com", PlanLabel: "Max (20x)", Source: app.SourceLive, Status: app.StatusOK, Left: pct(51),
			Windows: []app.UsageWindow{
				{Class: "short", Label: "Current session", Percent: 13, ResetsAt: at(now, noraShort)},
				{Class: "week", Label: "All models", Percent: 49, ResetsAt: at(now, noraWeek)},
				{Class: "week", Label: "Sonnet", Scope: "Sonnet", Percent: 3, ResetsAt: at(now, noraWeek)},
			}},
		{Kind: app.RowHome, Index: 2, Name: "work", Agent: "claude", Home: "~/.claude-work", Account: "work@example.com", PlanLabel: "Pro", Source: app.SourceLive, Status: app.StatusLimit, Left: pct(0),
			Windows: []app.UsageWindow{
				{Class: "short", Label: "Current session", Percent: 110, ResetsAt: at(now, ataShort)},
				{Class: "week", Label: "All models", Percent: 93, ResetsAt: at(now, ataWeek)},
			}},
		{Kind: app.RowHome, Index: 3, Name: "codex", Agent: "codex", Home: "~/.codex", Source: app.SourceNone, Status: app.StatusNotLoggedIn,
			Reason: "not logged in; run the agent once in this home", Windows: []app.UsageWindow{}},
		{Kind: app.RowHome, Index: 4, Name: "side", Agent: "claude", Home: "~/.claude-side", Account: "side@example.com", Source: app.SourceNone, Status: app.StatusExpired,
			Reason: "login expired at 2026-11-17T09:00:00Z: run claude once in this home to refresh it", Windows: []app.UsageWindow{}},
	}
}

// view is the scenario's usageView in format f, coloured when color.
func view(t *testing.T, f string, color bool) usageView {
	loc, now := lookClock(t)
	return viewOf(app.UsageReport{Now: now, Loc: loc, TimeFormat: f}, render.NewStyle(color))
}

// render runs fn into a string.
func renderString(t *testing.T, fn func(b *strings.Builder) error) string {
	t.Helper()
	var b strings.Builder
	if err := fn(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestUsageTableGolden pins the --table view, plain and coloured: v0.3's
// columns in v0.3's order, relative resets, RESET DAY and RESETS ON, LEFT
// and STATUS words, the weekly summary and the full reason under it.
func TestUsageTableGolden(t *testing.T) {
	_, now := lookClock(t)
	for _, c := range []struct {
		name  string
		color bool
	}{{"usage-table", false}, {"usage-table-color", true}} {
		got := renderString(t, func(b *strings.Builder) error {
			return writeUsageTable(b, lookLogins(now), view(t, config.TimeRelative, c.color))
		})
		testkit.GoldenText(t, c.name, got)
	}
}

// TestUsageTableMatchesV03Reference checks the plain table line by line
// against the owner's v0.3 screen (fake accounts; v0.3's code rendered
// the same rows to exactly these lines).
func TestUsageTableMatchesV03Reference(t *testing.T) {
	_, now := lookClock(t)
	got := renderString(t, func(b *strings.Builder) error {
		return writeUsageTable(b, lookLogins(now)[:2], view(t, config.TimeRelative, false))
	})
	want := "" +
		"#   LOGIN   PROVIDER  ACCOUNT             5H USED  5H RESETS    WEEKLY USED  WEEKLY RESETS  RESET DAY  RESETS ON         LEFT  STATUS\n" +
		"2   nora    claude    nora@example.com    13%      2 hr 50 min  49%          5 d 11 hr      Tue        Tue 24 Nov 05:30  51%   ok\n" +
		"13  miriam  claude    miriam@example.com  110%     40 min       93%          3 hr 40 min    Wed        Wed 18 Nov 21:30  0%    limit reached\n" +
		"Weekly resets: Wed 1 (+93%) · Tue 1 (+49%)\n"
	if got != want {
		t.Errorf("table differs from the v0.3 reference:\n--- want\n%s--- got\n%s", want, got)
	}
}

// TestUsageBarsGolden pins the default view, plain and coloured: the bold
// account header, bars with v0.3's glyphs and width, the weekly heading,
// relative resets, and a failed home's red reason.
func TestUsageBarsGolden(t *testing.T) {
	_, now := lookClock(t)
	for _, c := range []struct {
		name  string
		color bool
	}{{"usage-bars", false}, {"usage-bars-color", true}} {
		got := renderString(t, func(b *strings.Builder) error {
			return writeUsageBars(b, lookHomes(now), view(t, config.TimeRelative, c.color))
		})
		testkit.GoldenText(t, c.name, got)
	}
}

// TestUsageTimeFormats: --time-format day, absolute and both replace the
// relative resets in both views, read in the --tz zone.
func TestUsageTimeFormats(t *testing.T) {
	_, now := lookClock(t)
	rows := lookLogins(now)[:1]
	for _, c := range []struct{ format, cell, phrase string }{
		{config.TimeRelative, "2 hr 50 min", "resets in 2 hr 50 min"},
		{config.TimeDay, "Wed 20:40", "resets Wed 20:40"},
		{config.TimeAbsolute, "Wed 18 Nov 20:40", "resets Wed 18 Nov 20:40"},
		{config.TimeBoth, "2 hr 50 min (20:40)", "resets in 2 hr 50 min (20:40)"},
	} {
		v := view(t, c.format, false)
		table := renderString(t, func(b *strings.Builder) error { return writeUsageTable(b, rows, v) })
		bars := renderString(t, func(b *strings.Builder) error { return writeUsageBars(b, rows, v) })
		if !strings.Contains(table, "  "+c.cell+"  ") || !strings.Contains(bars, c.phrase) {
			t.Errorf("%s:\n%s\n%s", c.format, table, bars)
		}
	}
}

// viewDate is the scenario's plain usageView in time format f and date
// format spec.
func viewDate(t *testing.T, f, spec string) usageView {
	t.Helper()
	loc, now := lookClock(t)
	date, err := datefmt.Parse(spec)
	if err != nil {
		t.Fatal(err)
	}
	return viewOf(app.UsageReport{Now: now, Loc: loc, TimeFormat: f, DateFormat: date}, render.NewStyle(false))
}

// Resets of the --time-format both scenario, from Wed 18 Nov 17:50 IST.
const (
	// bothToday is 21:00 the same evening.
	bothToday = 3*time.Hour + 10*time.Minute
	// bothPastMidnight is 00:20 the next morning: fewer than 24 hours
	// away but on another calendar day, so the weekday is shown.
	bothPastMidnight = 6*time.Hour + 30*time.Minute
	// bothTomorrow is Thu 19 Nov 23:30.
	bothTomorrow = 29*time.Hour + 40*time.Minute
	// bothSixDays is Tue 24 Nov 05:30, six calendar days ahead: the last
	// day a weekday names alone.
	bothSixDays = noraWeek
	// bothSevenDays is Wed 25 Nov 18:00, seven calendar days ahead: the
	// whole date, since "Wed" would read as today.
	bothSevenDays = 7*24*time.Hour + 10*time.Minute
)

// bothRows covers every branch of --time-format both: a reset today, one
// after midnight, one tomorrow, six and seven calendar days ahead (a
// per-model weekly window), and a reset already passed.
func bothRows(now time.Time) []app.UsageRow {
	return []app.UsageRow{
		{Kind: app.RowHome, Index: 1, Name: "personal", Agent: "claude", Home: "~/.claude", Account: "me@example.com", Source: app.SourceLive, Status: app.StatusOK, Left: pct(51),
			Windows: []app.UsageWindow{
				{Class: "short", Label: "Current session", Percent: 13, ResetsAt: at(now, bothToday)},
				{Class: "week", Label: "All models", Percent: 49, ResetsAt: at(now, bothSixDays)},
				{Class: "week", Label: "Sonnet", Scope: "Sonnet", Percent: 3, ResetsAt: at(now, bothSevenDays)},
			}},
		{Kind: app.RowHome, Index: 2, Name: "work", Agent: "claude", Home: "~/.claude-work", Account: "work@example.com", Source: app.SourceLive, Status: app.StatusOK, Left: pct(40),
			Windows: []app.UsageWindow{
				{Class: "short", Label: "Current session", Percent: 60, ResetsAt: at(now, bothPastMidnight)},
				{Class: "week", Label: "All models", Percent: 20, ResetsAt: at(now, bothTomorrow)},
			}},
		{Kind: app.RowHome, Index: 3, Name: "old", Agent: "claude", Home: "~/.claude-old", Account: "old@example.com", Source: app.SourceCached, Status: app.StatusOK, Left: pct(70),
			Windows: []app.UsageWindow{
				{Class: "short", Label: "Current session", Percent: 30, ResetsAt: at(now, -time.Hour)},
			}},
	}
}

// TestUsageBothGolden pins --time-format both in the table and the bars,
// in the default date format (24-hour) and the us preset (12-hour).
func TestUsageBothGolden(t *testing.T) {
	_, now := lookClock(t)
	for _, c := range []struct{ name, date string }{
		{"usage-both", datefmt.PresetLong},
		{"usage-both-us", datefmt.PresetUS},
	} {
		v := viewDate(t, config.TimeBoth, c.date)
		got := renderString(t, func(b *strings.Builder) error {
			if err := writeUsageTable(b, bothRows(now), v); err != nil {
				return err
			}
			b.WriteString("\n")
			return writeUsageBars(b, bothRows(now), v)
		})
		testkit.GoldenText(t, c.name, got)
	}
}

// TestUsageBothCells: each branch of the bracket, cell by cell, so a
// golden refresh cannot hide a wrong rule.
func TestUsageBothCells(t *testing.T) {
	_, now := lookClock(t)
	for _, c := range []struct {
		d          time.Duration
		long, us   string
		phraseLong string
	}{
		{bothToday, "3 hr 10 min (21:00)", "3 hr 10 min (9:00 PM)", "resets in 3 hr 10 min (21:00)"},
		{bothPastMidnight, "6 hr 30 min (Thu 00:20)", "6 hr 30 min (Thu 12:20 AM)", "resets in 6 hr 30 min (Thu 00:20)"},
		{bothTomorrow, "1 d 5 hr (Thu 23:30)", "1 d 5 hr (Thu 11:30 PM)", "resets in 1 d 5 hr (Thu 23:30)"},
		{bothSixDays, "5 d 11 hr (Tue 05:30)", "5 d 11 hr (Tue 5:30 AM)", "resets in 5 d 11 hr (Tue 05:30)"},
		{bothSevenDays, "7 d 0 hr (Wed 25 Nov 18:00)", "7 d 0 hr (Nov 25 6:00 PM)", "resets in 7 d 0 hr (Wed 25 Nov 18:00)"},
		{-time.Hour, "now", "now", "resets now"},
	} {
		reset := *at(now, c.d)
		if got := resetText(reset, viewDate(t, config.TimeBoth, datefmt.PresetLong)); got != c.long {
			t.Errorf("%s long: %q, want %q", c.d, got, c.long)
		}
		if got := resetText(reset, viewDate(t, config.TimeBoth, datefmt.PresetUS)); got != c.us {
			t.Errorf("%s us: %q, want %q", c.d, got, c.us)
		}
		if got := resetPhrase(reset, viewDate(t, config.TimeBoth, datefmt.PresetLong)); got != c.phraseLong {
			t.Errorf("%s phrase: %q, want %q", c.d, got, c.phraseLong)
		}
	}
}

// TestUsageDateFormats: the date format writes day, absolute and RESETS
// ON (presets, 12- and 24-hour, a custom format); relative is unchanged.
func TestUsageDateFormats(t *testing.T) {
	_, now := lookClock(t)
	reset := *at(now, noraShort) // Wed 18 Nov 20:40 IST
	week := *at(now, noraWeek)   // Tue 24 Nov 05:30 IST
	for _, c := range []struct {
		spec, day, absolute, resetsOn string
	}{
		{datefmt.PresetLong, "Wed 20:40", "Wed 18 Nov 20:40", "Tue 24 Nov 05:30"},
		{datefmt.PresetISO, "Wed 20:40", "2026-11-18 20:40", "2026-11-24 05:30"},
		{datefmt.PresetShort, "Wed 20:40", "Wed 20:40", "Tue 05:30"},
		{datefmt.PresetUS, "Wed 8:40 PM", "Nov 18 8:40 PM", "Nov 24 5:30 AM"},
		{"DD/MM [at] H:mm", "Wed 20:40", "18/11 at 20:40", "24/11 at 5:30"},
		{"dddd hh:mm a", "Wed 08:40 pm", "Wednesday 08:40 pm", "Tuesday 05:30 am"},
	} {
		if got := resetText(reset, viewDate(t, config.TimeDay, c.spec)); got != c.day {
			t.Errorf("%s day: %q, want %q", c.spec, got, c.day)
		}
		if got := resetText(reset, viewDate(t, config.TimeAbsolute, c.spec)); got != c.absolute {
			t.Errorf("%s absolute: %q, want %q", c.spec, got, c.absolute)
		}
		if got := resetText(reset, viewDate(t, config.TimeRelative, c.spec)); got != "2 hr 50 min" {
			t.Errorf("%s relative: %q", c.spec, got)
		}
		w := &app.UsageWindow{Class: "week", Percent: 1, ResetsAt: &week}
		if got := weeklyDateCells(w, viewDate(t, config.TimeRelative, c.spec))[1].Text; got != c.resetsOn {
			t.Errorf("%s RESETS ON: %q, want %q", c.spec, got, c.resetsOn)
		}
	}
}

// TestUsageTableNameColumn: LOGIN for logins (v0.3), PROFILE for homes,
// NAME when --login-probes mixes them.
func TestUsageTableNameColumn(t *testing.T) {
	_, now := lookClock(t)
	for _, c := range []struct {
		rows []app.UsageRow
		want string
	}{
		{lookLogins(now), colLogin},
		{lookHomes(now), colProfile},
		{append(lookHomes(now), lookLogins(now)...), colName},
	} {
		if got := buildUsageTable(c.rows, view(t, config.TimeRelative, false)).Header[1]; got != c.want {
			t.Errorf("got %s, want %s", got, c.want)
		}
	}
}

// TestUsageStatusCellIsTheWordAlone: STATUS holds only the status words
// (v0.3's "expired", "limit reached"); every reason is a line under the
// table, and no library prefix reaches either.
func TestUsageStatusCellIsTheWordAlone(t *testing.T) {
	_, now := lookClock(t)
	rows := lookLogins(now)
	tbl := buildUsageTable(rows, view(t, config.TimeRelative, false))
	want := []string{"ok", "limit reached", "rejected", "per token", "expired"}
	for i, r := range tbl.Rows {
		if got := r[len(r)-1].Text; got != want[i] {
			t.Errorf("row %d STATUS %q, want %q", i, got, want[i])
		}
	}
	out := renderString(t, func(b *strings.Builder) error { return writeUsageTable(b, rows, view(t, config.TimeRelative, false)) })
	if !strings.Contains(out, "\n  oskar: login expired at 2026-11-17T09:00:00Z: run claude once in this home to refresh it\n") ||
		!strings.Contains(out, "\n  carol: the server refused") || strings.Contains(out, "agentio") {
		t.Errorf("reason lines:\n%s", out)
	}
}

// TestUsageCellTones: LEFT is blue, amber under 20% and red at 0; a bar is
// amber from 80% and red at the cap; an unknown status is red.
func TestUsageCellTones(t *testing.T) {
	for _, c := range []struct {
		left float64
		want render.Tone
	}{{51, render.ToneGood}, {19, render.ToneWarn}, {0, render.ToneBad}} {
		if got := leftCell(pct(c.left)).Tone; got != c.want {
			t.Errorf("left %v: tone %v, want %v", c.left, got, c.want)
		}
	}
	if leftCell(nil).Text != render.Dash || leftCell(nil).Tone != render.ToneDim {
		t.Error("no windows must be a dim dash, never 100%")
	}
	for _, c := range []struct {
		used float64
		want render.Tone
	}{{13, render.ToneGood}, {80, render.ToneWarn}, {100, render.ToneBad}, {110, render.ToneBad}} {
		if got := barTone(c.used); got != c.want {
			t.Errorf("bar %v: tone %v, want %v", c.used, got, c.want)
		}
	}
	if statusTone("something-new") != render.ToneBad {
		t.Error("an unknown status must be red")
	}
}

// TestBarCells: 0% is empty, any use shows a cell, the cap and beyond fill
// the bar.
func TestBarCells(t *testing.T) {
	for _, c := range []struct {
		pct           float64
		filled, empty int
	}{{0, 0, 30}, {1, 1, 29}, {13, 4, 26}, {50, 15, 15}, {100, 30, 0}, {110, 30, 0}, {-5, 0, 30}} {
		if f, e := barCells(c.pct, barWidth); f != c.filled || e != c.empty {
			t.Errorf("%v%%: %d/%d, want %d/%d", c.pct, f, e, c.filled, c.empty)
		}
	}
}

// TestWeeklyResetSummary: soonest weekday first, counted and summed; no
// weekly window means no line.
func TestWeeklyResetSummary(t *testing.T) {
	loc, now := lookClock(t)
	rows := lookLogins(now)
	extra := rows[1]
	extra.Name, extra.Index = "oskar", 3
	rows = append(rows, extra)
	if got := weeklyResetSummary(rows, loc, datefmt.Preset(datefmt.PresetLong)); got != "Weekly resets: Wed 2 (+186%) · Tue 1 (+49%)" {
		t.Errorf("summary %q", got)
	}
	if got := weeklyResetSummary(rows[2:3], loc, datefmt.Preset(datefmt.PresetLong)); got != "" {
		t.Errorf("no weekly window: %q", got)
	}
}

// Table cell positions of a row with one per-model column, for
// TestUsageClockRoundsToNearestMinute.
const (
	cell5hResets     = 5
	cellWeeklyResets = 7
	cellResetDay     = 8
	cellResetsOn     = 9
	cellModelResets  = 11
)

// TestUsageClockRoundsToNearestMinute: one reset prints one minute in
// every column, mode and date format. Resets at :29 seconds round down
// and at :31 round up (v0.3 printed 22:20 for 22:19:31; before the fix
// one path truncated to 22:19 while another showed 10:20 pm), while the
// relative part stays a truncated duration.
func TestUsageClockRoundsToNearestMinute(t *testing.T) {
	loc, _ := lookClock(t) // now: Wed 18 Nov 17:50:00 IST
	wall := func(day, hour, minute, sec int) time.Time {
		return time.Date(2026, 11, day, hour, minute, sec, 0, loc)
	}
	formats := []string{datefmt.PresetLong, datefmt.PresetISO, datefmt.PresetShort, datefmt.PresetUS, "DD/MM [at] H:mm", "dddd hh:mm a"}
	for _, c := range []struct {
		sec                        int
		short, week, model         time.Time // the expected rounded clocks
		relShort, bothLong, bothUS string
	}{
		{29, wall(18, 22, 19, 0), wall(19, 23, 29, 0), wall(21, 10, 19, 0), "4 hr 29 min", "4 hr 29 min (22:19)", "4 hr 29 min (10:19 PM)"},
		{31, wall(18, 22, 20, 0), wall(19, 23, 30, 0), wall(21, 10, 20, 0), "4 hr 29 min", "4 hr 29 min (22:20)", "4 hr 29 min (10:20 PM)"},
	} {
		short, week, model := wall(18, 22, 19, c.sec), wall(19, 23, 29, c.sec), wall(21, 10, 19, c.sec)
		row := app.UsageRow{Kind: app.RowHome, Index: 1, Name: "personal", Agent: "claude", Status: app.StatusOK, Left: pct(50),
			Windows: []app.UsageWindow{
				{Class: "short", Label: "Current session", Percent: 10, ResetsAt: &short},
				{Class: "week", Label: "All models", Percent: 20, ResetsAt: &week},
				{Class: "week", Label: "Sonnet", Scope: "Sonnet", Percent: 30, ResetsAt: &model},
			}}
		rows := []app.UsageRow{row}
		models := modelColumns(rows)
		for _, spec := range formats {
			for _, mode := range []string{config.TimeRelative, config.TimeDay, config.TimeAbsolute, config.TimeBoth} {
				v := viewDate(t, mode, spec)
				cells := usageTableRow(row, models, v)
				// RESETS ON and RESET DAY ignore the mode.
				if got, want := cells[cellResetsOn].Text, v.date.Date(c.week); got != want {
					t.Errorf("%ds %s %s RESETS ON: %q, want %q", c.sec, spec, mode, got, want)
				}
				if got, want := cells[cellResetDay].Text, c.week.Format(resetDayFmt); got != want {
					t.Errorf("%ds %s %s RESET DAY: %q, want %q", c.sec, spec, mode, got, want)
				}
				for _, col := range []struct {
					name string
					cell int
					want time.Time
				}{{"5H", cell5hResets, c.short}, {"WEEKLY", cellWeeklyResets, c.week}, {"SONNET WEEKLY", cellModelResets, c.model}} {
					got := cells[col.cell].Text
					if mode == config.TimeRelative {
						if strings.ContainsAny(got, ":") {
							t.Errorf("%ds %s relative %s: %q has a clock", c.sec, spec, col.name, got)
						}
						continue
					}
					if clock := v.date.Clock(col.want); !strings.Contains(got, clock) {
						t.Errorf("%ds %s %s %s: %q, want the clock %q", c.sec, spec, mode, col.name, got, clock)
					}
				}
			}
		}
		if got := resetText(short, viewDate(t, config.TimeRelative, datefmt.PresetLong)); got != c.relShort {
			t.Errorf("%ds relative: %q, want %q (durations truncate)", c.sec, got, c.relShort)
		}
		if got := resetText(short, viewDate(t, config.TimeBoth, datefmt.PresetLong)); got != c.bothLong {
			t.Errorf("%ds both long: %q, want %q", c.sec, got, c.bothLong)
		}
		if got := resetText(short, viewDate(t, config.TimeBoth, datefmt.PresetUS)); got != c.bothUS {
			t.Errorf("%ds both us: %q, want %q", c.sec, got, c.bothUS)
		}
	}

	// A reset at 23:59:40 is the next day's 00:00 in every column: the
	// weekday rounds with the clock.
	late := wall(19, 23, 59, 40)
	w := &app.UsageWindow{Class: "week", Percent: 1, ResetsAt: &late}
	cells := weeklyDateCells(w, viewDate(t, config.TimeRelative, datefmt.PresetLong))
	if cells[0].Text != "Fri" || cells[1].Text != "Fri 20 Nov 00:00" {
		t.Errorf("midnight RESET DAY/RESETS ON: %q %q", cells[0].Text, cells[1].Text)
	}
	if got := resetText(late, viewDate(t, config.TimeDay, datefmt.PresetLong)); got != "Fri 00:00" {
		t.Errorf("midnight day: %q", got)
	}
	if got := weeklyResetSummary([]app.UsageRow{{Windows: []app.UsageWindow{*w}}}, loc, datefmt.Preset(datefmt.PresetLong)); !strings.Contains(got, "Fri") {
		t.Errorf("midnight summary: %q", got)
	}
}
