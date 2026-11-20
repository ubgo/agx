// Command poolsim runs the login-pool simulator and prints the results
// tables as markdown (task poolsim).
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agx/internal/poolsim"
)

// defaultSeeds is how many randomised weeks each scenario runs.
const defaultSeeds = 40

// options are the command's flags.
type options struct {
	seeds      int
	scenario   string
	strategies string
	step       time.Duration
	lag        int
	sweep      string
	accounts   bool
	ratio      float64
}

func main() {
	var o options
	flag.IntVar(&o.seeds, "seeds", defaultSeeds, "randomised runs per scenario")
	flag.StringVar(&o.scenario, "scenario", "", "run only these scenarios (comma-separated)")
	flag.StringVar(&o.strategies, "strategies", "", "run only these strategies (comma-separated)")
	flag.DurationVar(&o.step, "step", poolsim.DefaultStep, "simulation step = how often the pool re-picks")
	flag.IntVar(&o.lag, "lag", poolsim.DefaultLag, "how many steps old the readings a pick sees are")
	flag.StringVar(&o.sweep, "sweep", "", "sweep the knobs whose name starts with this (\"all\" = every knob) around the candidate policy")
	flag.BoolVar(&o.accounts, "accounts", false, "print each account's room left at each weekly reset (per strategy)")
	flag.Float64Var(&o.ratio, "ratio", poolsim.DefaultWeekRatio, "how many full 5-hour windows one week holds")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "poolsim:", err)
		os.Exit(1)
	}
}

// listed reports whether name is in the comma-separated filter ("" = all).
func listed(filter, name string) bool {
	return filter == "" || slices.Contains(strings.Split(filter, ","), name)
}

// run prints one table per scenario, or the sweeps.
func run(o options) error {
	if o.sweep != "" {
		return sweep(o)
	}
	for _, gen := range poolsim.Scenarios() {
		if !listed(o.scenario, gen.Name) {
			continue
		}
		gen = poolsim.WithWeekRatio(poolsim.WithTiming(gen, o.step, o.lag), o.ratio)
		sc := gen.Make(1)
		members := make([]poolsim.Member, len(sc.Accounts))
		for i, a := range sc.Accounts {
			members[i] = poolsim.Member{Name: a.Name, Group: a.Group}
		}
		var strategies []poolsim.Strategy
		for _, st := range append(poolsim.Strategies(), poolsim.RealToday(members), poolsim.RealRecommended(members)) {
			if listed(o.strategies, st.Name) {
				strategies = append(strategies, st)
			}
		}
		if o.accounts {
			for _, st := range strategies {
				fmt.Printf("\n#### %s, %s: room left / our use at each weekly reset\n\n", gen.Name, st.Name)
				for _, aw := range poolsim.AccountWastes(gen, st, o.seeds) {
					fmt.Printf("- %s:", aw.Account)
					for i := range aw.Left {
						fmt.Printf(" reset %d left %.0f%% (ours %.0f%%);", i+1, aw.Left[i], aw.Ours[i])
					}
					fmt.Println()
				}
			}
			continue
		}
		fmt.Printf("\n### %s: %s (%d seeds, step %s, lag %d, week ratio %g)\n\n", gen.Name, gen.About, o.seeds, o.step, o.lag, o.ratio)
		if err := poolsim.WriteTable(os.Stdout, poolsim.Evaluate(gen, strategies, o.seeds, poolsim.DefaultWeights)); err != nil {
			return err
		}
	}
	return nil
}

// sweep prints one table per knob.
func sweep(o options) error {
	var gens []poolsim.Generator
	for _, g := range poolsim.SweepScenarios() {
		if listed(o.scenario, g.Name) {
			gens = append(gens, poolsim.WithWeekRatio(poolsim.WithTiming(g, o.step, o.lag), o.ratio))
		}
	}
	for _, k := range poolsim.Knobs() {
		if o.sweep != "all" && !strings.HasPrefix(k.Name, o.sweep) {
			continue
		}
		fmt.Printf("\n#### %s (%d seeds, step %s)\n\n", k.Name, o.seeds, o.step)
		rows := poolsim.Sweep(gens, poolsim.Candidate(), k, o.seeds, poolsim.DefaultWeights)
		if err := poolsim.WriteSweep(os.Stdout, k, gens, rows); err != nil {
			return err
		}
	}
	return nil
}
