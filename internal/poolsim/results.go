package poolsim

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
)

// Results are every strategy's summary on every scenario.
type Results struct {
	Gens       []Generator
	Strategies []string
	// Sums[g][s] is strategy s on scenario g.
	Sums [][]Summary
}

// EvaluateAll runs every strategy on every scenario.
func EvaluateAll(gens []Generator, strategies []Strategy, w Weights) Results {
	r := Results{Gens: gens}
	for _, st := range strategies {
		r.Strategies = append(r.Strategies, st.Name)
	}
	for _, g := range gens {
		r.Sums = append(r.Sums, Evaluate(g, strategies, w))
	}
	return r
}

// ratio is strategy s's cost on scenario g relative to strategy base's
// (1 = the same, below 1 = better): the score scales with the pool's
// size, the ratio does not, so scenarios of any size average fairly.
func (r Results) ratio(g, s, base int) float64 {
	b := r.Sums[g][base].Score
	if b <= 0 {
		return 1
	}
	return r.Sums[g][s].Score / b
}

// GroupRow is one strategy over one group of scenarios.
type GroupRow struct {
	Strategy string
	Group    string
	Files    int
	// Mean is the per-scenario means averaged over the group's files.
	Mean Metrics
	// MeanRatio and WorstRatio are the cost relative to the base
	// strategy, averaged and at its worst scenario (WorstFile).
	MeanRatio, WorstRatio float64
	WorstFile             string
}

// GroupAll is the group name of the rows over every scenario.
const GroupAll = "all"

// Groups summarises each strategy per group (GroupOrder, then
// GroupAll), costs relative to strategy base.
func (r Results) Groups(base string) []GroupRow {
	bi := slices.Index(r.Strategies, base)
	var out []GroupRow
	for _, group := range append(slices.Clone(GroupOrder), GroupAll) {
		for si, name := range r.Strategies {
			row := GroupRow{Strategy: name, Group: group, WorstRatio: math.Inf(-1)}
			for gi, g := range r.Gens {
				if group != GroupAll && g.Group != group {
					continue
				}
				row.Files++
				addScaled(&row.Mean, r.Sums[gi][si].Mean, 1)
				rt := r.ratio(gi, si, bi)
				row.MeanRatio += rt
				if rt > row.WorstRatio {
					row.WorstRatio, row.WorstFile = rt, g.Name
				}
			}
			if row.Files == 0 {
				continue
			}
			scale(&row.Mean, 1/float64(row.Files))
			row.MeanRatio /= float64(row.Files)
			out = append(out, row)
		}
	}
	return out
}

// scale multiplies every summarised metric of m by f.
func scale(m *Metrics, f float64) {
	for _, fl := range fields {
		*fl.get(m) *= f
	}
}

// groupHead is the group table's header.
const groupHead = "| strategy | scenarios | cost vs A (mean / worst, at) | waste %/acct-wk | 429s/wk | stall h/wk | squeeze h/wk | lockout h/wk | own room % | own drained h | switches /s/day | served |\n" +
	"|---|---|---|---|---|---|---|---|---|---|---|---|\n"

// WriteGroups writes one table per group.
func WriteGroups(w io.Writer, rows []GroupRow) error {
	var b strings.Builder
	cur := ""
	for _, r := range rows {
		if r.Group != cur {
			cur = r.Group
			fmt.Fprintf(&b, "\n#### %s\n\n%s", cur, groupHead)
		}
		m := r.Mean
		fmt.Fprintf(&b, "| %s | %d | %.3f / %.3f (%s) | %.1f | %.1f | %.1f | %.1f | %.2f | %.0f | %.1f | %.2f | %.1f%% |\n",
			r.Strategy, r.Files, r.MeanRatio, r.WorstRatio, r.WorstFile, m.WasteTotal, m.Hits429, m.StallHours, m.SqueezeHours,
			m.LockoutHours, m.OwnRoom, m.OwnDrainedHours, m.Switches, m.Served*100)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
