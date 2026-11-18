package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
)

// --sort and --tz of agx usage: the order of the rows and the zone reset
// days are read in. Pure, so the rules are table-tested without a read.

// Sort keys (--sort).
const (
	// SortLogin orders by the row's name (login or profile).
	SortLogin = "login"
	// SortLeft orders by what is left before the first cap.
	SortLeft = "left"
	// Sort5h orders by the short (5-hour) window's percent.
	Sort5h = "5h"
	// Sort5hReset orders by when the short window resets.
	Sort5hReset = "5h-reset"
	// SortWeekly orders by the weekly window's percent.
	SortWeekly = "weekly"
	// SortWeeklyReset orders by when the weekly window resets.
	SortWeeklyReset = "weekly-reset"
	// SortDay orders by the weekday (Monday first, in --tz) the weekly
	// window resets on, so accounts group by their reset day.
	SortDay = "day"
	// SortStatus orders by status rank (RowStatusValues order).
	SortStatus = "status"
)

// SortKeyValues lists the --sort keys in the order help shows them.
func SortKeyValues() []string {
	return []string{SortLogin, SortLeft, Sort5h, Sort5hReset, SortWeekly, SortWeeklyReset, SortDay, SortStatus}
}

// sortDescPrefix marks a key sorted high to low ("-left").
const sortDescPrefix = "-"

// SortKey is one parsed --sort value.
type SortKey struct {
	Name string
	Desc bool
}

// ParseSortKeys reads the repeated --sort values; an unknown key is a
// usage error listing the valid ones.
func ParseSortKeys(vals []string) ([]SortKey, error) {
	keys := make([]SortKey, 0, len(vals))
	for _, v := range vals {
		k := SortKey{Name: strings.TrimPrefix(strings.TrimSpace(v), sortDescPrefix)}
		k.Desc = k.Name != strings.TrimSpace(v)
		if !slices.Contains(SortKeyValues(), k.Name) {
			return nil, apperr.Usage(fmt.Sprintf("--sort %q: use one of %s (prefix %s for high to low; repeat for tie-breaks)",
				v, strings.Join(SortKeyValues(), ", "), sortDescPrefix))
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// ParseTZ resolves --tz: an IANA zone, or the machine's zone when empty.
func ParseTZ(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, apperr.Usage(fmt.Sprintf("--tz %q: unknown time zone (use an IANA name such as Asia/Kolkata)", name))
	}
	return loc, nil
}

// SortRows orders rows by keys, stably; no keys keeps config order. A row
// without a value for a key goes last in either direction.
func SortRows(rows []UsageRow, keys []SortKey, loc *time.Location) {
	if len(keys) == 0 {
		return
	}
	slices.SortStableFunc(rows, func(x, y UsageRow) int {
		for _, k := range keys {
			if c := compareSort(sortValueOf(x, k.Name, loc), sortValueOf(y, k.Name, loc), k.Desc); c != 0 {
				return c
			}
		}
		return 0
	})
}

// sortValue is a row's value for one key: a number or a string; ok is
// false when the row has none.
type sortValue struct {
	num float64
	str string
	ok  bool
}

// sortValueOf is r's value for key, in loc for the day key.
func sortValueOf(r UsageRow, key string, loc *time.Location) sortValue {
	num := func(f float64) sortValue { return sortValue{num: f, ok: true} }
	at := func(w *UsageWindow) sortValue {
		if w == nil || w.ResetsAt == nil {
			return sortValue{}
		}
		return num(float64(w.ResetsAt.Unix()))
	}
	pct := func(w *UsageWindow) sortValue {
		if w == nil {
			return sortValue{}
		}
		return num(w.Percent)
	}
	short, week := r.ShortWindow(), r.WeeklyWindow()
	switch key {
	case SortLogin:
		return sortValue{str: r.Name, ok: true}
	case SortLeft:
		if r.Left != nil {
			return num(*r.Left)
		}
	case Sort5h:
		return pct(short)
	case Sort5hReset:
		return at(short)
	case SortWeekly:
		return pct(week)
	case SortWeeklyReset:
		return at(week)
	case SortDay:
		if week != nil && week.ResetsAt != nil {
			return num(float64(MondayFirst(week.ResetsAt.In(loc).Weekday())))
		}
	case SortStatus:
		return num(float64(slices.Index(RowStatusValues(), r.Status)))
	}
	return sortValue{}
}

// compareSort orders two values; a missing value goes last always.
func compareSort(a, b sortValue, desc bool) int {
	switch {
	case !a.ok && !b.ok:
		return 0
	case !a.ok:
		return 1
	case !b.ok:
		return -1
	}
	c := cmp.Or(cmp.Compare(a.num, b.num), strings.Compare(a.str, b.str))
	if desc {
		return -c
	}
	return c
}

// MondayFirst numbers weekdays Mon=0 … Sun=6 (calendar order for day).
func MondayFirst(d time.Weekday) int { return (int(d) + 6) % 7 }

// ShortWindow is the all-usage window shorter than a day (the 5-hour
// session), or nil.
func (r UsageRow) ShortWindow() *UsageWindow { return r.window(agentio.WindowShort) }

// WeeklyWindow is the all-usage weekly window, or nil.
func (r UsageRow) WeeklyWindow() *UsageWindow { return r.window(agentio.WindowWeek) }

// window is the first unscoped window of class c.
func (r UsageRow) window(c agentio.WindowClass) *UsageWindow {
	for i := range r.Windows {
		if r.Windows[i].Class == c.String() && r.Windows[i].Scope == "" {
			return &r.Windows[i]
		}
	}
	return nil
}
