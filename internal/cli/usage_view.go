package cli

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/human"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
)

// The human views of agx usage, ported from agx v0.3 (internal/render
// and internal/cli/usagetable.go there) because the owner reads that look
// at a glance: the claude.ai-style bars by default, and with --table one
// compact row per home or login with the v0.3 columns, relative resets,
// the weekly-reset summary and coloured LEFT and STATUS. Every function
// deciding a cell is pure (the report carries the clock), so the look is
// pinned by golden tests without a request.

// usageView is what the renderers need besides the rows: the clock the
// relative times are measured from, the --tz zone, the time format, the
// date format of its clock and date parts, and the colours.
type usageView struct {
	now      time.Time
	loc      *time.Location
	format   string
	date     datefmt.Format
	duration human.DurationMode
	st       render.Style
}

// viewOf is the view of a report painted with st.
func viewOf(rep app.UsageReport, st render.Style) usageView {
	loc := rep.Loc
	if loc == nil {
		loc = time.Local
	}
	date := rep.DateFormat
	if date.IsZero() {
		date = datefmt.Preset(datefmt.PresetLong)
	}
	return usageView{now: rep.Now, loc: loc, format: cmp.Or(rep.TimeFormat, config.TimeRelative), date: date, duration: durationMode(rep.DurationFormat), st: st}
}

// durationMode maps usage.duration_format / --duration-format to the
// formatter's mode; "" and unknown values (refused earlier) truncate.
func durationMode(s string) human.DurationMode {
	switch s {
	case config.DurationRound:
		return human.DurationRound
	case config.DurationMinutes:
		return human.DurationMinutes
	}
	return human.DurationTruncate
}

// until is a relative time in the view's duration mode.
func (v usageView) until(d time.Duration) string { return human.UntilAs(d, v.duration) }

// Bars view layout (v0.3's render.Usage).
const (
	// barWidth is the number of cells in a usage bar.
	barWidth = 30
	// barLabelWidth pads window labels so the bars line up.
	barLabelWidth = 18
	barIndent     = "  "
	// barFilled and barTrack are distinct shapes, so the bar reads with
	// colour off.
	barFilled = "█"
	barTrack  = "░"
	// barWarnAt is the used percent from which a bar turns amber: agentio
	// carries no vendor severity, so agx marks the last fifth before the
	// cap, the same line at which LEFT turns amber (leftWarnBelow).
	barWarnAt = 80.0
	// percentFull is the used percent at which a window is at its cap.
	percentFull = 100.0
	// perTokenText replaces the bars of a row billed per token.
	perTokenText = "billed per token — no usage windows"
	// probeMark ends a login row's header: its usage came from a probe.
	probeMark = "(probe)"
	// headerSep separates a header's profile and agent ("work · claude").
	headerSep = " · "
	// resetsInWord and resetsWord lead a window's reset in the bars.
	resetsInWord = "resets in "
	resetsWord   = "resets "
)

// classHeadings is the section title above a window class in the bars;
// the short (session) class and windows of unknown length (extra usage)
// have none, as in v0.3 where only the weekly group had a heading.
var classHeadings = map[string]string{
	agentio.WindowDay.String():   "Daily limits",
	agentio.WindowWeek.String():  "Weekly limits",
	agentio.WindowMonth.String(): "Monthly limits",
}

// writeUsageBars prints one block per row: a bold header naming the
// account, then each window as a bar with its percent and reset, then
// (with --plan) the v0.3 plan table under all blocks.
func writeUsageBars(w io.Writer, rows []app.UsageRow, v usageView) error {
	ew := textout.New(w)
	for i, r := range rows {
		if i > 0 {
			ew.Line("")
		}
		ew.Line(barsHeader(r, v))
		writeRowBody(ew, r, v)
	}
	if ew.Err() != nil {
		return ew.Err()
	}
	if hasPlan(rows) {
		ew.Line("")
		if err := writePlanTable(ew, rows, v.st); err != nil {
			return err
		}
	}
	return ew.Err()
}

// barsHeader is "email  Plan  profile · agent  ~/home" for a home and
// "account  login NAME · agent  (probe)" for a login: the account bold,
// the rest dim. A cached reading adds its age.
func barsHeader(r app.UsageRow, v usageView) string {
	st := v.st
	if r.Kind == app.RowLogin {
		who := cmp.Or(r.Account, loginWord+r.Name)
		return st.Bold(who) + st.Dim(barIndent+loginWord+r.Name+headerSep+string(r.Agent)+barIndent+probeMark)
	}
	who := cmp.Or(r.Account, r.Name)
	var b strings.Builder
	b.WriteString(st.Bold(who))
	if r.PlanLabel != "" {
		b.WriteString(st.Dim(barIndent + r.PlanLabel))
	}
	rest := barIndent + r.Name + headerSep + string(r.Agent) + barIndent + r.Home
	if r.Source == app.SourceCached && r.ObservedAt != nil {
		rest += barIndent + string(app.SourceCached) + " " + human.Ago(v.now, *r.ObservedAt)
	}
	b.WriteString(st.Dim(rest))
	return b.String()
}

// loginWord names a login in a header ("login carol").
const loginWord = "login "

// writeRowBody prints a row's windows, or why it has none.
func writeRowBody(ew *textout.Writer, r app.UsageRow, v usageView) {
	st := v.st
	switch {
	case r.Status == app.StatusPerToken:
		ew.Line(barIndent + st.Dim(perTokenText))
		return
	case len(r.Windows) == 0:
		ew.Line(barIndent + st.Paint(statusTone(r.Status), statusText(r)))
		return
	}
	prev := ""
	for i, win := range r.Windows {
		if i == 0 || win.Class != prev {
			if h := classHeadings[win.Class]; h != "" {
				ew.Line(barIndent + st.Bold(h))
			}
			prev = win.Class
		}
		ew.Line(barLine(win, v))
	}
	// A reading shown with a caveat (stale) says so under its bars.
	if r.Reason != "" {
		ew.Line(barIndent + st.Paint(statusTone(r.Status), statusText(r)))
	}
}

// barLine is "  label  ██████░░░░  42% used  resets in 2 hr 5 min".
func barLine(win app.UsageWindow, v usageView) string {
	st := v.st
	filled, empty := barCells(win.Percent, barWidth)
	bar := st.Paint(barTone(win.Percent), strings.Repeat(barFilled, filled)) + st.Dim(strings.Repeat(barTrack, empty))
	label := windowLabel(win)
	pad := strings.Repeat(" ", max(0, barLabelWidth-utf8.RuneCountInString(label)))
	line := fmt.Sprintf("%s%s%s %s %4.0f%% used", barIndent, label, pad, bar, win.Percent)
	if win.ResetsAt != nil {
		line += st.Dim(barIndent + resetPhrase(*win.ResetsAt, v))
	}
	return line
}

// windowLabel is the window's label, with its scope when the label does
// not already name it ("5h (gpt-5)", but "Sonnet", not "Sonnet (Sonnet)").
func windowLabel(win app.UsageWindow) string {
	if win.Scope == "" || win.Scope == win.Label {
		return win.Label
	}
	if win.Label == "" {
		return win.Scope
	}
	return win.Label + " (" + win.Scope + ")"
}

// resetPhrase is the bars' reset text in the chosen format: "resets in 2
// hr 5 min", "resets in 2 hr 5 min (17:35)", "resets Tue 05:30", "resets
// Tue 24 Nov 05:30".
func resetPhrase(at time.Time, v usageView) string {
	switch v.format {
	case config.TimeRelative, config.TimeBoth:
		// A reset already passed (an old cached reading) reads "resets
		// now", not "resets in now".
		if v.until(at.Sub(v.now)) == human.Now {
			return resetsWord + human.Now
		}
		return resetsInWord + resetText(at, v)
	}
	return resetsWord + resetText(at, v)
}

// bothWeekdayDays is how many calendar days ahead --time-format both
// names a reset by its weekday ("1 d 4 hr (Thu 23:30)"); further out a
// weekday is ambiguous (next Thursday or the one after), so the whole
// date is written.
const bothWeekdayDays = 6

// resetText is a reset time as a table cell, in the --tz zone: relative
// ("2 hr 50 min"), day ("Tue 05:30"), absolute ("Tue 24 Nov 05:30") or
// both ("3 hr 10 min (22:20)"); the clock and date parts follow the date
// format (--date-format). The clock is rounded once here (datefmt's
// Round, the nearest minute) so the weekday, the clock and the calendar
// day of both agree in every mode; the relative part is a duration and
// keeps its own truncating rule.
func resetText(at time.Time, v usageView) string {
	local := v.date.Round(at.In(v.loc))
	switch v.format {
	case config.TimeDay:
		return weekdayClock(local, v)
	case config.TimeAbsolute:
		return v.date.Date(local)
	case config.TimeBoth:
		rel := v.until(at.Sub(v.now))
		if rel == human.Now {
			return rel
		}
		return rel + " (" + bothWhen(local, v) + ")"
	}
	return v.until(at.Sub(v.now))
}

// weekdayClock is "Tue 05:30": the weekday, then the date format's clock;
// t is already rounded by the caller, so the weekday matches the clock.
func weekdayClock(t time.Time, v usageView) string {
	return t.Format(resetDayFmt) + " " + v.date.Clock(t)
}

// bothWhen is the part in brackets of --time-format both: the clock alone
// for a reset today, the weekday and clock within bothWeekdayDays
// calendar days, else the whole date format.
func bothWhen(t time.Time, v usageView) string {
	switch days := calendarDays(v.now.In(v.loc), t); {
	case days <= 0:
		return v.date.Clock(t)
	case days <= bothWeekdayDays:
		return weekdayClock(t, v)
	}
	return v.date.Date(t)
}

// calendarDays is how many midnights lie between from and to on the wall
// clock of their zone (both are in the --tz zone), so 23:00 to 01:00 the
// next morning is one day although only two hours pass.
func calendarDays(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a) / (24 * time.Hour))
}

// barCells splits width into filled and empty cells for pct (clamped to
// 0–100). Any non-zero usage shows at least one filled cell, so 1% is
// visibly not 0%.
func barCells(pct float64, width int) (filled, empty int) {
	p := math.Max(0, math.Min(percentFull, pct))
	filled = int(math.Round(p / percentFull * float64(width)))
	if filled == 0 && p > 0 {
		filled = 1
	}
	return filled, width - filled
}

// barTone is a bar's colour: blue, amber in the last fifth, red at the cap.
func barTone(pct float64) render.Tone {
	switch {
	case pct >= percentFull:
		return render.ToneBad
	case pct >= barWarnAt:
		return render.ToneWarn
	}
	return render.ToneGood
}

// statusTones colours each status: answers blue or dim, a caveat amber,
// every failure red (v0.3's usage table).
var statusTones = map[app.RowStatus]render.Tone{
	app.StatusOK:           render.ToneGood,
	app.StatusLimit:        render.ToneBad,
	app.StatusPerToken:     render.ToneDim,
	app.StatusUnsupported:  render.ToneDim,
	app.StatusNoCache:      render.ToneDim,
	app.StatusStale:        render.ToneWarn,
	app.StatusExpired:      render.ToneWarn,
	app.StatusRateLimited:  render.ToneBad,
	app.StatusNotLoggedIn:  render.ToneBad,
	app.StatusRejected:     render.ToneBad,
	app.StatusScopeLimited: render.ToneBad,
	app.StatusHomeMissing:  render.ToneBad,
	app.StatusSecret:       render.ToneBad,
	app.StatusNetwork:      render.ToneBad,
	app.StatusError:        render.ToneBad,
}

// statusTone is s's colour; an unknown status is red so it is never missed.
func statusTone(s app.RowStatus) render.Tone {
	if t, ok := statusTones[s]; ok {
		return t
	}
	return render.ToneBad
}

// statusWord is how a status reads to a person: the JSON value with
// spaces ("limit reached", "not logged in"), as v0.3 wrote them.
func statusWord(s app.RowStatus) string { return strings.ReplaceAll(string(s), "-", " ") }

// statusText is "status: reason", or the reason alone when it already
// starts with the status words ("not logged in; run …").
func statusText(r app.UsageRow) string {
	word := statusWord(r.Status)
	switch {
	case r.Reason == "":
		return word
	case strings.HasPrefix(r.Reason, word):
		return r.Reason
	}
	return word + usageReasonSep + r.Reason
}

// The --table view: v0.3's columns, in v0.3's order and words.
const (
	colNum      = "#"
	colLogin    = "LOGIN"
	colProfile  = "PROFILE"
	colName     = "NAME"
	colProvider = "PROVIDER"
	colAccount  = "ACCOUNT"
	colLeft     = "LEFT"
	colStatus   = "STATUS"
	// Every window gets a USED and a RESETS column side by side, so each
	// window's own reset is visible (v0.3: one shared RESETS hid the
	// weekly reset whenever the 5-hour window decided LEFT).
	col5hName     = "5H"
	colWeeklyName = "WEEKLY"
	colUsedSuffix = " USED"
	colResetSufix = " RESETS"
	// colModelSuffix follows a per-model weekly window's name ("SONNET
	// WEEKLY").
	colModelSuffix = " WEEKLY"
	// colResetDay and colResetsOn follow WEEKLY RESETS: the weekday and
	// the date the weekly window resets (RESETS ON in the date format),
	// in the --tz zone.
	colResetDay = "RESET DAY"
	colResetsOn = "RESETS ON"
	resetDayFmt = "Mon"
	// leftWarnBelow colours LEFT amber under this many percent left (and
	// red at zero), mirroring the bars' amber line.
	leftWarnBelow = 20.0
	// fullReasonIndent starts each reason line under the table.
	fullReasonIndent = "  "
)

// The weekly-reset summary under the table ("Weekly resets: Tue 1 (+79%)
// · Wed 2 (+164%)"): per weekday, how many rows reset then and the room
// they get back.
const (
	summaryPrefix = "Weekly resets: "
	summarySep    = " · "
	summaryDayFmt = "%s %d (+%.0f%%)"
)

// usageTable is the table's header and rows, ready for StyledTable.
type usageTable struct {
	Header []string
	Rows   [][]render.StyledCell
}

// buildUsageTable lays the rows out in v0.3's columns: #, the name, the
// agent (PROVIDER, v0.3's word), ACCOUNT, each window's USED and RESETS,
// RESET DAY and RESETS ON after the weekly window, a column pair per
// model window any row carries, then LEFT and STATUS. Pure.
func buildUsageTable(rows []app.UsageRow, v usageView) usageTable {
	models := modelColumns(rows)
	header := []string{colNum, nameColumn(rows), colProvider, colAccount}
	for _, name := range append([]string{col5hName, colWeeklyName}, modelNames(models)...) {
		header = append(header, name+colUsedSuffix, name+colResetSufix)
		if name == colWeeklyName {
			header = append(header, colResetDay, colResetsOn)
		}
	}
	header = append(header, colLeft, colStatus)
	t := usageTable{Header: header}
	for _, r := range rows {
		t.Rows = append(t.Rows, usageTableRow(r, models, v))
	}
	return t
}

// nameColumn heads the name column: LOGIN for logins (v0.3's table),
// PROFILE for homes, NAME when both are shown (--login-probes).
func nameColumn(rows []app.UsageRow) string {
	logins, homes := false, false
	for _, r := range rows {
		logins = logins || r.Kind == app.RowLogin
		homes = homes || r.Kind == app.RowHome
	}
	switch {
	case logins && homes:
		return colName
	case homes:
		return colProfile
	}
	return colLogin
}

// modelColumn is one per-model window column pair: the scope and the
// window class it holds.
type modelColumn struct {
	scope, class string
}

// modelColumns lists the per-model windows any row carries, in first-seen
// order: a column pair appears only when a reading returned one.
func modelColumns(rows []app.UsageRow) []modelColumn {
	var out []modelColumn
	for _, r := range rows {
		for _, w := range r.Windows {
			c := modelColumn{w.Scope, w.Class}
			if w.Scope != "" && !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	return out
}

// modelClassSuffix follows a model's name in its column header by window
// class ("SONNET WEEKLY", as v0.3 named it; "GPT-5 5H").
var modelClassSuffix = map[string]string{
	agentio.WindowShort.String(): " " + col5hName,
	agentio.WindowWeek.String():  colModelSuffix,
}

// modelNames is each model column's name ("SONNET WEEKLY").
func modelNames(models []modelColumn) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = strings.ToUpper(m.scope) + modelClassSuffix[m.class]
	}
	return out
}

// usageTableRow is one row. A window the row lacks is a dim "—" in both
// its cells, never 0%: an account without a 5-hour window is not idle.
func usageTableRow(r app.UsageRow, models []modelColumn, v usageView) []render.StyledCell {
	dim := func(s string) render.StyledCell { return render.StyledCell{Text: s, Tone: render.ToneDim} }
	num := dim(render.Dash)
	if r.Index > 0 {
		num = dim(strconv.Itoa(r.Index))
	}
	row := []render.StyledCell{num, {Text: r.Name}, plainOrDash(string(r.Agent)), plainOrDash(r.Account)}
	row = append(row, windowCells(r.ShortWindow(), v)...)
	week := r.WeeklyWindow()
	row = append(row, windowCells(week, v)...)
	row = append(row, weeklyDateCells(week, v)...)
	for _, m := range models {
		row = append(row, windowCells(scopedWindow(r, m), v)...)
	}
	return append(row, leftCell(r.Left), statusCell(r))
}

// plainOrDash is s, or a dim dash when empty.
func plainOrDash(s string) render.StyledCell {
	if s == "" {
		return render.StyledCell{Text: render.Dash, Tone: render.ToneDim}
	}
	return render.StyledCell{Text: s}
}

// scopedWindow is r's first window of m's scope and class, or nil.
func scopedWindow(r app.UsageRow, m modelColumn) *app.UsageWindow {
	for i := range r.Windows {
		if r.Windows[i].Scope == m.scope && r.Windows[i].Class == m.class {
			return &r.Windows[i]
		}
	}
	return nil
}

// windowCells is the USED and RESETS pair of w, or two dim dashes.
func windowCells(w *app.UsageWindow, v usageView) []render.StyledCell {
	missing := render.StyledCell{Text: render.Dash, Tone: render.ToneDim}
	if w == nil {
		return []render.StyledCell{missing, missing}
	}
	used := render.StyledCell{Text: fmt.Sprintf(usagePercentFmt, w.Percent)}
	if w.ResetsAt == nil {
		return []render.StyledCell{used, missing}
	}
	return []render.StyledCell{used, {Text: resetText(*w.ResetsAt, v)}}
}

// weeklyDateCells is the RESET DAY and RESETS ON pair in the --tz zone,
// rounded like every other column (a reset at 23:59:40 is the next day's
// 00:00 in both cells), or two dashes.
func weeklyDateCells(w *app.UsageWindow, v usageView) []render.StyledCell {
	if w == nil || w.ResetsAt == nil {
		missing := render.StyledCell{Text: render.Dash, Tone: render.ToneDim}
		return []render.StyledCell{missing, missing}
	}
	t := v.date.Round(w.ResetsAt.In(v.loc))
	return []render.StyledCell{{Text: t.Format(resetDayFmt)}, {Text: v.date.Date(t)}}
}

// leftCell colours what is left: red at zero, amber when low, else blue.
func leftCell(left *float64) render.StyledCell {
	if left == nil {
		return render.StyledCell{Text: render.Dash, Tone: render.ToneDim}
	}
	tone := render.ToneGood
	switch {
	case *left <= 0:
		tone = render.ToneBad
	case *left < leftWarnBelow:
		tone = render.ToneWarn
	}
	return render.StyledCell{Text: fmt.Sprintf(usagePercentFmt, *left), Tone: tone}
}

// statusCell is the status words alone, coloured ("expired", "limit
// reached"), as v0.3's cell read; the reason goes on a line under the
// table, so a long library message never stretches or truncates the cell.
func statusCell(r app.UsageRow) render.StyledCell {
	return render.StyledCell{Text: statusWord(r.Status), Tone: statusTone(r.Status)}
}

// weeklyResetSummary is the line under the table: for each weekday (in
// loc) on which some row's weekly window resets, how many rows reset then
// and the sum of their weekly USED (the room they get back), soonest
// first. Empty when no row has a weekly reset. Each reset is rounded with
// date's Round, so its weekday is the one RESET DAY shows. Pure.
func weeklyResetSummary(rows []app.UsageRow, loc *time.Location, date datefmt.Format) string {
	type bucket struct {
		name     string
		soonest  time.Time
		rows     int
		returned float64
	}
	byDay := map[time.Weekday]*bucket{}
	var days []*bucket
	for _, r := range rows {
		w := r.WeeklyWindow()
		if w == nil || w.ResetsAt == nil {
			continue
		}
		t := date.Round(w.ResetsAt.In(loc))
		d, ok := byDay[t.Weekday()]
		if !ok {
			d = &bucket{name: t.Format(resetDayFmt), soonest: t}
			byDay[t.Weekday()] = d
			days = append(days, d)
		}
		if t.Before(d.soonest) {
			d.soonest = t
		}
		d.rows++
		d.returned += w.Percent
	}
	if len(days) == 0 {
		return ""
	}
	slices.SortStableFunc(days, func(a, b *bucket) int { return a.soonest.Compare(b.soonest) })
	parts := make([]string, len(days))
	for i, d := range days {
		parts[i] = fmt.Sprintf(summaryDayFmt, d.name, d.rows, d.returned)
	}
	return summaryPrefix + strings.Join(parts, summarySep)
}

// writeUsageTable prints the table, the weekly-reset summary, then
// "  name: reason" for every row with a reason, as v0.3 printed the
// details under its table.
func writeUsageTable(w io.Writer, rows []app.UsageRow, v usageView) error {
	t := buildUsageTable(rows, v)
	if err := render.StyledTable(w, t.Header, t.Rows, v.st); err != nil {
		return err
	}
	ew := textout.New(w)
	if s := weeklyResetSummary(rows, v.loc, v.date); s != "" {
		ew.Line(s)
	}
	for _, r := range rows {
		if r.Reason != "" {
			ew.Line(fullReasonIndent + r.Name + usageReasonSep + r.Reason)
		}
	}
	if ew.Err() != nil {
		return ew.Err()
	}
	if hasPlan(rows) {
		ew.Line("")
		if err := writePlanTable(ew, rows, v.st); err != nil {
			return err
		}
	}
	return ew.Err()
}

// The --plan table (v0.3's `agx plan`): one row per home, the errors under it.
const (
	planErrorCell = "error (see below)"
	planSinceFmt  = "2 Jan 2006"
)

// hasPlan reports whether --plan filled any row.
func hasPlan(rows []app.UsageRow) bool {
	return slices.ContainsFunc(rows, func(r app.UsageRow) bool { return r.Plan != nil })
}

// writePlanTable prints PROFILE PROVIDER ACCOUNT PLAN STATUS SINCE for
// every row with a plan, then "name: error" for each plan that failed.
func writePlanTable(w io.Writer, rows []app.UsageRow, st render.Style) error {
	var cells [][]render.StyledCell
	var errs []string
	for _, r := range rows {
		p := r.Plan
		if p == nil {
			continue
		}
		plan, status, since := plainOrDash(p.Plan), plainOrDash(p.Status), plainOrDash("")
		if p.StartedAt != nil {
			since = render.StyledCell{Text: p.StartedAt.In(time.UTC).Format(planSinceFmt)}
		}
		if p.Error != "" {
			plan = render.StyledCell{Text: planErrorCell, Tone: render.ToneBad}
			errs = append(errs, r.Name+usageReasonSep+p.Error)
		}
		cells = append(cells, []render.StyledCell{{Text: r.Name}, plainOrDash(string(r.Agent)), plainOrDash(r.Account), plan, status, since})
	}
	if err := render.StyledTable(w, []string{colProfile, colProvider, colAccount, "PLAN", colStatus, "SINCE"}, cells, st); err != nil {
		return err
	}
	ew := textout.New(w)
	if len(errs) > 0 {
		ew.Line("")
	}
	for _, e := range errs {
		ew.Line(e)
	}
	return ew.Err()
}
