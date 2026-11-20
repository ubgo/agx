package poolsim

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
)

// Evaluate runs every strategy on the generator's seeds (Seed,
// Seed+1, … Seeds of them) and summarises each strategy; seeds run in
// parallel, results are the same in any order.
func Evaluate(gen Generator, strategies []Strategy, w Weights) []Summary {
	seeds := max(gen.Seeds, 1)
	runs := make([][]Metrics, len(strategies))
	for i := range runs {
		runs[i] = make([]Metrics, seeds)
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for s := range seeds {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			sc := gen.Make(gen.Seed + uint64(s))
			for i, st := range strategies {
				runs[i][s] = Run(sc, st)
			}
		}()
	}
	wg.Wait()
	out := make([]Summary, len(strategies))
	for i, st := range strategies {
		out[i] = Summarise(st.Name, gen.Name, runs[i], w)
	}
	return out
}

// tableHead is the results table's header: each metric as mean / worst.
const tableHead = "| strategy | waste %/acct-wk | 429s/wk | stall h/wk | squeeze h/wk | lockout h/wk (week part) | own room % | own drained h | switches /s/day | served | score |\n" +
	"|---|---|---|---|---|---|---|---|---|---|---|\n"

// WriteTable writes summaries as one markdown table: each cell is the
// mean, then the worst run after a slash.
func WriteTable(w io.Writer, sums []Summary) error {
	var b strings.Builder
	b.WriteString(tableHead)
	for _, s := range sums {
		m, x := s.Mean, s.Worst
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %.0f |\n", s.Strategy,
			pair(m.WasteTotal, x.WasteTotal, 1), pair(m.Hits429, x.Hits429, 1), pair(m.StallHours, x.StallHours, 1),
			pair(m.SqueezeHours, x.SqueezeHours, 1), pair(m.LockoutHours, x.LockoutHours, 2)+fmt.Sprintf(" (%.2f)", m.LockoutWeekHours), pair(m.OwnRoom, x.OwnRoom, 0),
			pair(m.OwnDrainedHours, x.OwnDrainedHours, 1), pair(m.Switches, x.Switches, 2),
			fmt.Sprintf("%.1f%% / %.1f%%", m.Served*100, x.Served*100), s.Score)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// pair prints "mean / worst" with prec decimals.
func pair(mean, worst float64, prec int) string {
	return fmt.Sprintf("%.*f / %.*f", prec, mean, prec, worst)
}

// AccountWaste is one account's average room left at its first,
// second and later weekly resets of a run, over the seeds.
type AccountWaste struct {
	Account string
	// Left[i] is the average room left at the account's reset i.
	Left []float64
	// Ours[i] is the average part of week i we used.
	Ours []float64
}

// AccountWastes runs one strategy on seeds of gen and averages each
// account's leftover per reset, accounts in config order.
func AccountWastes(gen Generator, st Strategy) []AccountWaste {
	var out []AccountWaste
	idx := map[string]int{}
	counts := map[string][]float64{}
	for s := range max(gen.Seeds, 1) {
		sc := gen.Make(gen.Seed + uint64(s))
		if out == nil {
			for i, a := range sc.Accounts {
				idx[a.Name] = i
				out = append(out, AccountWaste{Account: a.Name})
			}
		}
		seen := map[string]int{}
		for _, r := range Run(sc, st).Resets {
			i, n := idx[r.Account], seen[r.Account]
			seen[r.Account]++
			aw := &out[i]
			for len(aw.Left) <= n {
				aw.Left, aw.Ours = append(aw.Left, 0), append(aw.Ours, 0)
				counts[r.Account] = append(counts[r.Account], 0)
			}
			aw.Left[n] += r.Left
			aw.Ours[n] += r.Ours
			counts[r.Account][n]++
		}
	}
	for i := range out {
		for n := range out[i].Left {
			c := counts[out[i].Account][n]
			out[i].Left[n] /= c
			out[i].Ours[n] /= c
		}
	}
	return out
}
