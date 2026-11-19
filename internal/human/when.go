// Package human formats values for people the way agx v0.3 did (the
// owner's reference look, docsi/memory/v03-output-is-reference.md):
// relative times ("2 hr 50 min", "3 min ago") and the one-line
// conversation row of lists and pickers. It is a leaf package so both
// the renderer and internal/app (picker rows) share one spelling.
package human

import (
	"fmt"
	"time"
)

// Relative times the way agx v0.3 printed them (and claude.ai prints
// usage resets): the owner reads "2 hr 50 min" faster than a timestamp.

// day is one calendar day of clock time.
const day = 24 * time.Hour

// Now is what Until prints for a time that has already passed: the
// window has reset (or is resetting) as you read it.
const Now = "now"

// JustNow is what Ago prints for under a minute ago.
const JustNow = "just now"

// DurationMode is how Until writes a duration of a day or more; under a
// day every mode reads "4 hr 38 min" / "12 min".
type DurationMode uint8

// Duration modes (usage.duration_format, --duration-format).
const (
	// DurationTruncate drops the minutes: "1 d 2 hr" for 1 d 2 hr 52 min, as agx
	// v0.3 and claude.ai print it (the default).
	DurationTruncate DurationMode = iota
	// DurationRound rounds to the nearest hour: "1 d 3 hr" for 1 d 2 hr 52 min,
	// so a reset at 13:30 seen at 10:37 the day before does not read as
	// 12:30.
	DurationRound
	// DurationMinutes keeps the minutes: "1 d 2 hr 52 min".
	DurationMinutes
)

// Until renders a duration the way claude.ai does: "4 hr 38 min",
// "5 d 3 hr", "12 min"; zero or negative reads "now". Truncated to the
// minute, so it never claims more time than is left.
func Until(d time.Duration) string { return UntilAs(d, DurationTruncate) }

// UntilAs is Until with the mode for a day or more. Every mode first
// truncates to the minute, so under a day all three read the same.
func UntilAs(d time.Duration, mode DurationMode) string {
	d = d.Truncate(time.Minute)
	switch {
	case d <= 0:
		return Now
	case d >= day && mode == DurationMinutes:
		return fmt.Sprintf("%d d %d hr %d min", d/day, (d%day)/time.Hour, (d%time.Hour)/time.Minute)
	case d >= day && mode == DurationRound:
		d = d.Round(time.Hour)
		return fmt.Sprintf("%d d %d hr", d/day, (d%day)/time.Hour)
	case d >= day:
		return fmt.Sprintf("%d d %d hr", d/day, (d%day)/time.Hour)
	case d >= time.Hour:
		return fmt.Sprintf("%d hr %d min", d/time.Hour, (d%time.Hour)/time.Minute)
	default:
		return fmt.Sprintf("%d min", d/time.Minute)
	}
}

// Ago renders how long ago t was, coarsely: "3 min ago", "2 hr
// ago", "5 d ago"; under a minute (or in the future) is "just now".
func Ago(now, t time.Time) string {
	d := now.Sub(t).Truncate(time.Minute)
	switch {
	case d < time.Minute:
		return JustNow
	case d >= day:
		return fmt.Sprintf("%d d ago", d/day)
	case d >= time.Hour:
		return fmt.Sprintf("%d hr ago", d/time.Hour)
	default:
		return fmt.Sprintf("%d min ago", d/time.Minute)
	}
}
