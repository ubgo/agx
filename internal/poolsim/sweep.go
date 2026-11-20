package poolsim

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Knob is one policy setting a sweep varies: its values as the report
// prints them and how each is applied.
type Knob struct {
	Name   string
	Values []string
	Apply  func(p *Policy, i int)
}

// SweepRow is one knob value's result over every scenario, its cost
// relative to the classic preset (A, the rules before v0.4.16) on the same scenario.
type SweepRow struct {
	Value string
	// MeanRatio and WorstRatio are over every scenario; WorstFile is
	// the scenario of the worst.
	MeanRatio, WorstRatio float64
	WorstFile             string
	// Groups are the mean ratio per group (GroupOrder).
	Groups map[string]float64
	// Mean is the average of the per-scenario metrics.
	Mean Metrics
}

// Sweep runs base with each value of k, and today's rules, on every
// generator and returns one row per value.
func Sweep(gens []Generator, base Policy, k Knob, w Weights) []SweepRow {
	strategies := []Strategy{policy("A", "", Today())}
	for i, v := range k.Values {
		p := base
		k.Apply(&p, i)
		strategies = append(strategies, policy(v, "", p))
	}
	res := EvaluateAll(gens, strategies, w)
	byGroup := map[string]GroupRow{}
	for _, g := range res.Groups("A") {
		byGroup[g.Group+"\x00"+g.Strategy] = g
	}
	var rows []SweepRow
	for _, v := range k.Values {
		all := byGroup[GroupAll+"\x00"+v]
		row := SweepRow{Value: v, MeanRatio: all.MeanRatio, WorstRatio: all.WorstRatio, WorstFile: all.WorstFile, Mean: all.Mean, Groups: map[string]float64{}}
		for _, grp := range GroupOrder {
			if g, ok := byGroup[grp+"\x00"+v]; ok {
				row.Groups[grp] = g.MeanRatio
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// addScaled adds f times every summarised metric of m to dst.
func addScaled(dst *Metrics, m Metrics, f float64) {
	for _, fl := range fields {
		*fl.get(dst) += *fl.get(&m) * f
	}
}

// WriteSweep writes a sweep as a markdown table: the cost relative to
// today's rules (mean, worst, per group) and the mean metrics.
func WriteSweep(w io.Writer, k Knob, rows []SweepRow) error {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | cost vs A: mean | worst (at) |", k.Name)
	for _, g := range GroupOrder {
		fmt.Fprintf(&b, " %s |", g)
	}
	b.WriteString(" waste % | served | lockout h | squeeze h | switches | own drained h |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | **%.3f** | %.3f (%s) |", r.Value, r.MeanRatio, r.WorstRatio, r.WorstFile)
		for _, g := range GroupOrder {
			fmt.Fprintf(&b, " %.3f |", r.Groups[g])
		}
		m := r.Mean
		fmt.Fprintf(&b, " %.1f | %.1f%% | %.2f | %.1f | %.2f | %.1f |\n", m.WasteTotal, m.Served*100, m.LockoutHours, m.SqueezeHours, m.Switches, m.OwnDrainedHours)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// hours names a duration in whole hours ("24h"), 0 as "off".
func hours(d time.Duration) string {
	if d == 0 {
		return "off"
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// durations is a knob over durations.
func durations(name string, vals []time.Duration, set func(*Policy, time.Duration)) Knob {
	k := Knob{Name: name, Apply: func(p *Policy, i int) { set(p, vals[i]) }}
	for _, v := range vals {
		k.Values = append(k.Values, hours(v))
	}
	return k
}

// numbers is a knob over numbers.
func numbers(name string, vals []float64, set func(*Policy, float64)) Knob {
	k := Knob{Name: name, Apply: func(p *Policy, i int) { set(p, vals[i]) }}
	for _, v := range vals {
		k.Values = append(k.Values, fmt.Sprintf("%g", v))
	}
	return k
}

// Knobs are the sweeps of the recommended policy's settings, plus the
// strategies it leaves out (to show what turning them on would do).
func Knobs() []Knob {
	return []Knob{
		durations("relax.start (C)", []time.Duration{0, 12 * time.Hour, 24 * time.Hour, 48 * time.Hour, 72 * time.Hour, WeekPeriod},
			func(p *Policy, d time.Duration) { p.Relax.Start = d }),
		numbers("relax.max (C)", []float64{80, 85, 90, 95, 98}, func(p *Policy, v float64) { p.Relax.Max = v }),
		numbers("relax.short (C, 5h at reset)", []float64{0, 60, 70, 80, 90}, func(p *Policy, v float64) { p.Relax.Short = v }),
		numbers("shared week cap", []float64{50, 60, 70, 80}, func(p *Policy, v float64) { p.Override.SharedWeek = v }),
		numbers("shared 5h cap", []float64{30, 40, 50, 60, 70}, func(p *Policy, v float64) { p.Override.SharedShort = v }),
		numbers("own week cap", []float64{90, 95, 98, 99}, func(p *Policy, v float64) { p.Override.OwnWeek = v }),
		numbers("spread per account (G)", []float64{0, 1, 2, 3, 4, 6}, func(p *Policy, v float64) { p.Spread = int(v) }),
		numbers("stick margin (H, -1 = off)", []float64{-1, 0, 2, 5, 10}, func(p *Policy, v float64) {
			p.Stick = Hysteresis{On: v >= 0, Margin: max(0, v)}
		}),
		numbers("sticky leave ratio (0 = never)", []float64{0, 1.5, 2, 3, 5}, func(p *Policy, v float64) { p.Stick.Ratio = v }),
		durations("borrow window", []time.Duration{0, 24 * time.Hour, 48 * time.Hour, 72 * time.Hour, 96 * time.Hour},
			func(p *Policy, d time.Duration) { p.Borrow = d }),
		numbers("order (0 reset, 1 urgency)", []float64{0, 1}, func(p *Policy, v float64) { p.Order = Order(v) }),
		numbers("pace slack (F, -1 = off)", []float64{-1, 0, 5, 10, 20}, func(p *Policy, v float64) {
			p.Pace = OwnPace{On: v >= 0, Slack: max(0, v)}
		}),
		durations("urgent window (B)", []time.Duration{0, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour},
			func(p *Policy, d time.Duration) {
				p.Urgent = UrgentTier{Window: d, Short: urgentShort, Week: urgentWeek}
			}),
		numbers("weighted by plan size (W, 0 off, 1 on)", []float64{0, 1}, func(p *Policy, v float64) { p.Weighted = v > 0 }),
		numbers("idle 5h cap (D, 0 = off)", []float64{0, 60, 70, 80}, func(p *Policy, v float64) {
			p.Idle = IdleShort{Short: v, ActiveFor: idleActiveFor}
		}),
	}
}
