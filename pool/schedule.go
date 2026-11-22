package pool

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Timed caps (docs/guides/pool-rules.md "Schedules and overrides"). Two
// things change a member's caps for a while without touching its normal
// caps:
//
//   - A Schedule is recurring: on its days and hours, in its time zone,
//     its caps replace the caps of the members it names (by name, or by
//     group: own, shared). It stops ReleaseBefore before its window
//     ends, so a teammate who starts work right after it finds a fresh
//     5-hour window.
//   - An Override (Account.Override) is one member's caps until a time;
//     after it, it is ignored and the report says so.
//
// For each cap (short, week, week_relax_max) the value comes from the
// first level that sets it: an active override, else the active
// schedules that name the member, else the active schedules that name
// its group, else the normal caps (member, group, defaults). When
// several active schedules at the same level set the same cap, the
// highest value wins (the most permissive): two schedules that both
// apply are both meant to allow that much.
//
// Config.At folds them into the member entries for one moment, so every
// rule (caps, relax, room, --pool-only) reads the timed caps without
// knowing about them; Select calls it first.
//
// One schedule cap depends on the member's reading, so At cannot fold
// it: week_before_reset (Schedule.WeekBeforeReset) is the weekly cap of
// a member whose weekly reset falls while the schedule applies, at or
// before its end (release_before counted). That member's room is lost at
// its reset, before the teammate the schedule makes room for is back, so
// it can only help this pool. A member that resets after the end keeps
// its normal weekly cap: its room still has to last its teammate's week.
// At records the values per member (beforeReset); the candidate applies
// the one that covers its reset as max(the cap it has now, relax
// included, value), right away and not ramped. Levels work as for the
// other caps: a schedule naming the member beats one naming its group,
// the highest value wins at one level, and an override that sets week
// wins outright (none is recorded then). It only ever raises a cap.

// Timed cap sources (CapSource.Kind).
const (
	// SourceSchedule is a cap from an active schedule.
	SourceSchedule = "schedule"
	// SourceOverride is a cap from a member's override that has not
	// expired.
	SourceOverride = "override"
)

// DefaultScheduleReleaseBefore is a schedule's release_before when it
// sets a 5-hour cap and does not say: one 5-hour window, so a teammate
// whose working day starts as the schedule ends finds their 5-hour
// window fresh instead of filled up to the raised cap.
const DefaultScheduleReleaseBefore = 5 * time.Hour

// Day and hour bounds of a schedule.
const (
	minutesPerDay  = 24 * 60
	minutesPerHour = 60
	// scheduleLookDays is how far around now a schedule's windows are
	// laid out (a week and a day each way covers any window: none is
	// longer than a day, and joined windows longer than this have no end).
	scheduleLookDays = 8
)

// FullDay is the hours of a schedule that leaves them out.
const FullDay = "00:00-24:00"

// dayNames are the day names a schedule may use, by time.Weekday.
var dayNames = [7]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// DayNames are the day names a schedule may use, Monday first.
func DayNames() []string { return append(slices.Clone(dayNames[1:]), dayNames[0]) }

// ScheduleCaps are the caps a schedule or an override sets; a nil field
// leaves that cap as it is.
type ScheduleCaps struct {
	Short        *float64
	Week         *float64
	WeekRelaxMax *float64
}

// given reports whether any cap is set.
func (s ScheduleCaps) given() bool { return s.Short != nil || s.Week != nil || s.WeekRelaxMax != nil }

// Schedule is a recurring window of different caps.
type Schedule struct {
	// Name labels it in the report.
	Name string
	// Days are the weekdays a window starts on, by time.Weekday.
	Days [7]bool
	// Start and End are minutes from local midnight (End 1440 = 24:00).
	// End at or before Start ends on the next day.
	Start, End int
	// Location is the time zone of Days and the hours; nil = the pool's
	// Config.Location.
	Location *time.Location
	// Members are member names and group names (own, shared).
	Members []string
	// Caps replace the members' caps while it applies.
	Caps ScheduleCaps
	// ReleaseBefore stops it this long before its window ends.
	ReleaseBefore time.Duration
	// WeekBeforeReset is the weekly cap of a member it names whose weekly
	// reset (or a per-model window's own reset) is at or before its end:
	// never lower than the cap the member has without it; nil = none
	// (the file comment).
	WeekBeforeReset *float64
}

// Override is one member's caps until a time.
type Override struct {
	Caps  ScheduleCaps
	Until time.Time
}

// CapSource says where a timed cap comes from.
type CapSource struct {
	// Kind is SourceSchedule or SourceOverride.
	Kind string `json:"kind"`
	// Name is the schedule's name ("" for an override).
	Name string `json:"name,omitempty"`
	// Until is when it stops applying (a schedule's window end less its
	// release_before); nil when it never ends.
	Until *time.Time `json:"until"`
	// BeforeReset is true when the cap is the schedule's
	// week_before_reset: the member's weekly reset is at or before Until.
	BeforeReset bool `json:"before_reset,omitempty"`
}

// TimedCaps are the member's caps that a schedule or an override sets
// now; a nil field is a normal cap.
type TimedCaps struct {
	Short        *CapSource `json:"short,omitempty"`
	Week         *CapSource `json:"week,omitempty"`
	WeekRelaxMax *CapSource `json:"week_relax_max,omitempty"`
}

// any reports whether any cap is timed.
func (t TimedCaps) any() bool { return t.Short != nil || t.Week != nil || t.WeekRelaxMax != nil }

// ParseDays reads day names (mon … sun, any case); at least one.
func ParseDays(in []string) ([7]bool, error) {
	var out [7]bool
	if len(in) == 0 {
		return out, errors.New("days is empty; name at least one of " + strings.Join(DayNames(), ", "))
	}
	for _, d := range in {
		i := slices.Index(dayNames[:], strings.ToLower(strings.TrimSpace(d)))
		if i < 0 {
			return out, fmt.Errorf("day %q is not one of %s", d, strings.Join(DayNames(), ", "))
		}
		out[i] = true
	}
	return out, nil
}

// ParseHours reads "HH:MM-HH:MM" ("" = FullDay): start 00:00 to 23:59,
// end 00:00 to 24:00; an end at or before the start ends on the next
// day. A window must not be empty, and 00:00-24:00 is the only window of
// a whole day.
func ParseHours(s string) (start, end int, err error) {
	if s == "" {
		s = FullDay
	}
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("hours %q: write it like 09:00-18:00 or 22:00-06:00", s)
	}
	if start, err = clockMinutes(a, false); err != nil {
		return 0, 0, fmt.Errorf("hours %q: %w", s, err)
	}
	if end, err = clockMinutes(b, true); err != nil {
		return 0, 0, fmt.Errorf("hours %q: %w", s, err)
	}
	if start == end {
		return 0, 0, fmt.Errorf("hours %q: the start and end are the same; a whole day is %s", s, FullDay)
	}
	return start, end, nil
}

// clockMinutes reads "HH:MM" as minutes from midnight; 24:00 only as an
// end.
func clockMinutes(s string, isEnd bool) (int, error) {
	s = strings.TrimSpace(s)
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, fmt.Errorf("%q is not HH:MM", s)
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	switch {
	case err1 != nil || err2 != nil || hh < 0 || mm < 0 || mm >= minutesPerHour:
		return 0, fmt.Errorf("%q is not HH:MM", s)
	case hh == 24 && mm == 0 && isEnd:
		return minutesPerDay, nil
	case hh >= 24:
		return 0, fmt.Errorf("%q is past 23:59 (24:00 only ends a window)", s)
	}
	return hh*minutesPerHour + mm, nil
}

// span is one stretch of time [from, to).
type span struct{ from, to time.Time }

// window is the schedule's joined window around now: the stretch of
// back-to-back windows that holds now, and whether it has an end within
// the days laid out (a schedule of every day, all day, has none).
func (s Schedule) window(now time.Time, loc *time.Location) (span, bool, bool) {
	if s.Location != nil {
		loc = s.Location
	}
	if loc == nil {
		loc = time.Local
	}
	lt := now.In(loc)
	var spans []span
	for off := -scheduleLookDays; off <= scheduleLookDays; off++ {
		// time.Date normalizes the day, so off crosses months and years;
		// a local time that a DST change skips moves forward, one that it
		// repeats takes its first instance.
		day := time.Date(lt.Year(), lt.Month(), lt.Day()+off, 0, 0, 0, 0, loc)
		if !s.Days[day.Weekday()] {
			continue
		}
		from := time.Date(day.Year(), day.Month(), day.Day(), 0, s.Start, 0, 0, loc)
		endDay := day.Day()
		if s.End <= s.Start {
			endDay++
		}
		to := time.Date(day.Year(), day.Month(), endDay, 0, s.End, 0, 0, loc)
		spans = append(spans, span{from, to})
	}
	if len(spans) == 0 {
		return span{}, false, false
	}
	// spans are in order: join each that starts at or before the last end.
	joined := []span{spans[0]}
	for _, sp := range spans[1:] {
		last := &joined[len(joined)-1]
		if !sp.from.After(last.to) {
			if sp.to.After(last.to) {
				last.to = sp.to
			}
			continue
		}
		joined = append(joined, sp)
	}
	horizon := spans[len(spans)-1].to
	for _, sp := range joined {
		if !now.Before(sp.from) && now.Before(sp.to) {
			return sp, true, sp.to.Before(horizon)
		}
	}
	return span{}, false, false
}

// ActiveUntil reports whether the schedule applies at now (loc is the
// pool's zone for a schedule without its own) and until when: its joined
// window's end less ReleaseBefore; nil when the window never ends.
func (s Schedule) ActiveUntil(now time.Time, loc *time.Location) (bool, *time.Time) {
	sp, in, ends := s.window(now, loc)
	if !in {
		return false, nil
	}
	if !ends {
		return true, nil
	}
	until := sp.to.Add(-s.ReleaseBefore)
	if !now.Before(until) {
		return false, nil
	}
	return true, &until
}

// names reports whether the schedule names member name (byName) or its
// group (byGroup).
func (s Schedule) names(name string, g Group) (byName, byGroup bool) {
	for _, m := range s.Members {
		switch {
		case m == string(GroupOwn) || m == string(GroupShared):
			byGroup = byGroup || Group(m) == g
		case m == name:
			byName = true
		}
	}
	return byName, byGroup
}

// Active reports whether the override applies at now.
func (o Override) Active(now time.Time) bool { return now.Before(o.Until) }

// validateTimed checks the schedules and the overrides.
func (c Config) validateTimed(members map[string]bool) error {
	seen := map[string]bool{}
	for i, s := range c.Schedules {
		label := fmt.Sprintf("schedules[%d]", i)
		if s.Name != "" {
			label = "schedule " + s.Name
		}
		switch {
		case strings.TrimSpace(s.Name) == "":
			return fmt.Errorf("pool: %s has no name", label)
		case seen[s.Name]:
			return fmt.Errorf("pool: schedule %q is listed more than once", s.Name)
		case s.Days == [7]bool{}:
			return fmt.Errorf("pool: %s has no days", label)
		case s.Start < 0 || s.Start >= minutesPerDay || s.End < 0 || s.End > minutesPerDay || s.Start == s.End:
			return fmt.Errorf("pool: %s hours are not a window of one day or less", label)
		case len(s.Members) == 0:
			return fmt.Errorf("pool: %s names no members (give member names, own or shared)", label)
		case !s.Caps.given() && s.WeekBeforeReset == nil:
			return fmt.Errorf("pool: %s sets no caps (short, week, week_relax_max or week_before_reset)", label)
		case s.WeekBeforeReset != nil && !validPercent(*s.WeekBeforeReset):
			return fmt.Errorf("pool: %s caps.week_before_reset %v is outside 0..100", label, *s.WeekBeforeReset)
		case s.ReleaseBefore < 0:
			return fmt.Errorf("pool: %s release_before is negative", label)
		}
		seen[s.Name] = true
		for _, m := range s.Members {
			if m != string(GroupOwn) && m != string(GroupShared) && !members[m] {
				return fmt.Errorf("pool: %s names %q, which is neither a member nor a group (own, shared)", label, m)
			}
		}
		if err := validateCaps(label+" caps", s.Caps); err != nil {
			return err
		}
	}
	for _, name := range sortedKeys(c.Accounts) {
		o := c.Accounts[name].Override
		if o == nil {
			continue
		}
		label := "accounts." + name + ".override"
		if o.Until.IsZero() {
			return fmt.Errorf("pool: %s has no until", label)
		}
		if !o.Caps.given() {
			return fmt.Errorf("pool: %s sets no caps (short, week or week_relax_max)", label)
		}
		if err := validateCaps(label, o.Caps); err != nil {
			return err
		}
	}
	return nil
}

// validateCaps checks that each set cap is a percent.
func validateCaps(label string, s ScheduleCaps) error {
	for _, p := range []struct {
		key string
		v   *float64
	}{{"short", s.Short}, {"week", s.Week}, {"week_relax_max", s.WeekRelaxMax}} {
		if p.v != nil && !validPercent(*p.v) {
			return fmt.Errorf("pool: %s.%s %v is outside 0..100", label, p.key, *p.v)
		}
	}
	return nil
}

// sortedKeys are a map's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// timedCap is one cap's value and source at one level.
type timedCap struct {
	v   *float64
	src *CapSource
}

// pick keeps the higher of two candidates of the same level (the most
// permissive); the first wins a tie, so config order decides the name.
func (t *timedCap) pick(v *float64, src CapSource) {
	if v == nil || (t.v != nil && *v <= *t.v) {
		return
	}
	t.v, t.src = v, &src
}

// At is c with the schedules and overrides that apply at now folded into
// the member entries, so every rule reads the timed caps; Timed reports
// them per member, and an expired override is left out (ExpiredOverride
// reports it). The normal caps are kept (normal), so a config folded at
// one time folds again at another from them: At(t1).At(t2) is At(t2),
// which Select needs to read the caps at a member's reset.
func (c Config) At(now time.Time) Config {
	if c.normal != nil {
		c = *c.normal
	}
	if len(c.Schedules) == 0 && !c.hasOverrides() {
		return c
	}
	normal := c
	out := c
	out.normal = &normal
	out.Accounts = make(map[string]Account, len(c.Accounts))
	for k, v := range c.Accounts {
		out.Accounts[k] = v
	}
	out.timed = map[string]TimedCaps{}
	out.expired = map[string]time.Time{}
	out.beforeReset = map[string]resetCaps{}
	type active struct {
		s     Schedule
		until *time.Time
	}
	var on []active
	for _, s := range c.Schedules {
		if ok, until := s.ActiveUntil(now, c.Location); ok {
			on = append(on, active{s, until})
		}
	}
	add := func(names []string, g Group) {
		for _, name := range names {
			var byName, byGroup [3]timedCap
			var floors resetCaps
			for _, a := range on {
				n, gr := a.s.names(name, g)
				if !n && !gr {
					continue
				}
				src := CapSource{Kind: SourceSchedule, Name: a.s.Name, Until: a.until}
				level, floorLevel := &byGroup, &floors.byGroup
				if n {
					level, floorLevel = &byName, &floors.byName
				}
				level[0].pick(a.s.Caps.Short, src)
				level[1].pick(a.s.Caps.Week, src)
				level[2].pick(a.s.Caps.WeekRelaxMax, src)
				if a.s.WeekBeforeReset != nil {
					src.BeforeReset = true
					*floorLevel = append(*floorLevel, resetCap{v: *a.s.WeekBeforeReset, src: src})
				}
			}
			var ovr [3]timedCap
			if o := c.Accounts[name].Override; o != nil {
				if o.Active(now) {
					src := CapSource{Kind: SourceOverride, Until: &o.Until}
					ovr[0].pick(o.Caps.Short, src)
					ovr[1].pick(o.Caps.Week, src)
					ovr[2].pick(o.Caps.WeekRelaxMax, src)
				} else {
					out.expired[name] = o.Until
				}
			}
			if ovr[1].v == nil && floors.given() {
				// An override that sets the weekly cap wins outright.
				out.beforeReset[name] = floors
			}
			var final [3]timedCap
			for i := range final {
				for _, lv := range [][3]timedCap{ovr, byName, byGroup} {
					if lv[i].v != nil {
						final[i] = lv[i]
						break
					}
				}
			}
			t := TimedCaps{Short: final[0].src, Week: final[1].src, WeekRelaxMax: final[2].src}
			if !t.any() {
				continue
			}
			out.timed[name] = t
			a := out.Accounts[name]
			if final[0].v != nil {
				a.Short = final[0].v
			}
			if final[1].v != nil {
				a.Week = final[1].v
			}
			if final[2].v != nil {
				a.WeekRelaxMax = final[2].v
			}
			out.Accounts[name] = a
		}
	}
	add(c.Own, GroupOwn)
	add(c.Shared, GroupShared)
	return out
}

// resetCap is one active schedule's week_before_reset for a member.
type resetCap struct {
	v   float64
	src CapSource
}

// resetCaps are a member's week_before_reset values after At: from the
// active schedules that name it (byName) and those that name its group
// (byGroup), each with the schedule's end.
type resetCaps struct {
	byName, byGroup []resetCap
}

// given reports whether any schedule gives a value.
func (r resetCaps) given() bool { return len(r.byName)+len(r.byGroup) > 0 }

// covers reports whether a reset at reset falls while the schedule of
// src applies: at or before its end (a reset at the very end loses its
// room as the teammate comes back), or any reset when it never ends. A
// reset before now has passed and covers nothing (its reading is old).
func covers(src CapSource, reset, now time.Time) bool {
	return !reset.Before(now) && (src.Until == nil || !reset.After(*src.Until))
}

// WeekBeforeReset is the week_before_reset member name gets after At
// for a window that resets at reset: the highest value of the first
// level (schedules naming the member, then its group) with a schedule
// that covers the reset, and that schedule as its source; ok false when
// none does or the reset has passed (an unreadable reset, the zero time,
// has passed).
func (c Config) WeekBeforeReset(name string, reset, now time.Time) (float64, CapSource, bool) {
	r := c.beforeReset[name]
	for _, level := range [][]resetCap{r.byName, r.byGroup} {
		best := -1
		for i, f := range level {
			if covers(f.src, reset, now) && (best < 0 || f.v > level[best].v) {
				best = i
			}
		}
		if best >= 0 {
			return level[best].v, level[best].src, true
		}
	}
	return 0, CapSource{}, false
}

// limitBeforeReset lowers member name's week_before_reset values to
// limit: a pinned run's weekly cap is a hard limit. Only Select calls it,
// on the config it has just folded with At, whose map and lists are its
// own, so it changes them in place.
func (c Config) limitBeforeReset(name string, limit float64) {
	r := c.beforeReset[name]
	for _, level := range [][]resetCap{r.byName, r.byGroup} {
		for i := range level {
			level[i].v = math.Min(level[i].v, limit)
		}
	}
}

// hasOverrides reports whether any member has an override.
func (c Config) hasOverrides() bool {
	for _, a := range c.Accounts {
		if a.Override != nil {
			return true
		}
	}
	return false
}

// Timed is member name's timed caps after At; zero when none.
func (c Config) Timed(name string) TimedCaps { return c.timed[name] }

// ExpiredOverride is when member name's override expired, after At; ok
// false when it has none or it still applies.
func (c Config) ExpiredOverride(name string) (time.Time, bool) {
	t, ok := c.expired[name]
	return t, ok
}

// ParseUntil reads an override's end, in loc (nil = time.Local) and
// after now: a duration ("6h", "90m"), a date and time
// ("2026-11-23T08:00", "2026-11-23 08:00", RFC 3339 with a zone), a date
// alone (the end of that day), "DAY HH:MM" (the next such moment: "sun
// 23:59") or "HH:MM" (the next such moment, today or tomorrow).
func ParseUntil(s string, now time.Time, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	s = strings.TrimSpace(s)
	t, err := parseUntil(s, now, loc)
	if err != nil {
		return time.Time{}, err
	}
	if !t.After(now) {
		return time.Time{}, fmt.Errorf("%q is not in the future", s)
	}
	return t, nil
}

// untilHelp lists the forms ParseUntil reads.
const untilHelp = `use a duration ("6h"), a date and time ("2026-11-23T08:00"), "DAY HH:MM" ("sun 23:59") or "HH:MM"`

// parseUntil is ParseUntil without the future check.
func parseUntil(s string, now time.Time, loc *time.Location) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		if d <= 0 {
			return time.Time{}, fmt.Errorf("%q is not a positive duration", s)
		}
		return now.Add(d), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc), nil
	}
	lt := now.In(loc)
	day, clock, hasDay := strings.Cut(s, " ")
	if !hasDay {
		clock, day = s, ""
	}
	mins, err := clockMinutes(clock, false)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q: %s", s, untilHelp)
	}
	want := -1
	if day != "" {
		want = slices.Index(dayNames[:], strings.ToLower(strings.TrimSpace(day)))
		if want < 0 {
			return time.Time{}, fmt.Errorf("%q: %q is not a day (%s); %s", s, day, strings.Join(DayNames(), ", "), untilHelp)
		}
	}
	for off := 0; off <= 7; off++ {
		t := time.Date(lt.Year(), lt.Month(), lt.Day()+off, 0, mins, 0, 0, loc)
		if (want < 0 || int(t.Weekday()) == want) && t.After(now) {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q: %s", s, untilHelp)
}

// untilLayout writes an end in notes: "Sun 22 Nov 23:59 +03".
const untilLayout = "Mon 2 Jan 15:04 MST"
