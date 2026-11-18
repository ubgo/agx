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

// Until renders a duration the way claude.ai does: "4 hr 38 min",
// "5 d 3 hr", "12 min"; zero or negative reads "now". Truncated to the
// minute, so it never claims more time than is left.
func Until(d time.Duration) string {
	d = d.Truncate(time.Minute)
	switch {
	case d <= 0:
		return Now
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
