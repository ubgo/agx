package cli

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
)

// Fields of agx usage's data (help --json): one object per row.
var usageFields = []string{
	"data.*.kind", "data.*.name", "data.*.agent", "data.*.home", "data.*.profile", "data.*.profiles",
	"data.*.login", "data.*.account", "data.*.source", "data.*.observed_at", "data.*.age",
	"data.*.status", "data.*.reason", "data.*.left", "data.*.windows", "data.*.plan",
	"data.*.index", "data.*.plan_label",
}

// usageCmd is agx usage (spec 13.1): live by default (cost N, one request
// per home), --cached reads disk only (R), the login flags probe (Q).
func (c *cliCtx) usageCmd() *cobra.Command {
	o := &app.UsageOptions{}
	// table is display only (the data is the same), so it stays here.
	table := false
	cmd := c.newCmd("usage", "Plan usage, one row per configured home", CostNetwork, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		o.DateFormat = c.g.DateFormat
		rep, err := c.App().Usage(cmd.Context(), *o)
		if rep.Rows == nil && err != nil {
			return render.Output{}, err
		}
		data, eerr := render.Encode(rep.Rows)
		if eerr != nil {
			return render.Output{}, eerr
		}
		out := render.Output{Data: data, Warnings: rep.Warnings}
		v := viewOf(rep, c.style())
		if table {
			out.Text = func(w io.Writer) error { return writeUsageTable(w, rep.Rows, v) }
		} else {
			out.Text = func(w io.Writer) error { return writeUsageBars(w, rep.Rows, v) }
		}
		return out, err
	})
	cmd.Long = "Plan usage for every configured home, each home once with all its profiles. Live by default (one request per home; vendors may answer 429), --cached reads what the agent left on disk. The login flags probe stored logins, which spends a little of each plan. Exit 3 when some rows failed, 1 when none answered."
	cmd.Example = "  agx usage\n  agx usage --cached --agent claude\n  agx usage --all-logins --table --sort -left\n  agx usage --login personal --login derek --plan"
	f := cmd.Flags()
	f.StringArrayVarP(&o.Profiles, "profile", "p", nil, "only these profiles' homes (repeat)")
	f.StringVar((*string)(&o.Agent), "agent", "", "only this agent")
	f.BoolVar(&o.Plan, "plan", false, "add plan, subscription status and start date")
	f.BoolVar(&o.Cached, "cached", false, "disk readings only, no network (cost R); each row shows its age")
	f.BoolVar(&o.LoginProbes, "login-probes", false, "add one probed row per stored login (cost Q)")
	f.StringArrayVar(&o.Logins, "login", nil, "show these logins instead of homes (repeat; cost Q)")
	f.BoolVar(&o.AllLogins, "all-logins", false, "show every stored login instead of homes (cost Q)")
	f.BoolVar(&o.LoginPicker, "login-picker", false, "tick logins in a checklist first (cost Q; terminal)")
	f.BoolVar(&table, "table", false, "one compact row per home or login")
	f.StringArrayVar(&o.Sort, "sort", nil, "order rows: "+strings.Join(app.SortKeyValues(), ", ")+"; - prefix for high to low; repeat for tie-breaks")
	timeFlags(cmd, &o.TimeOptions)
	return fields(cmd, usageFields...)
}

// timeFlags declares how reset times read, shared by agx usage and agx
// pool pick (the global --date-format completes the set).
func timeFlags(cmd *cobra.Command, o *app.TimeOptions) {
	f := cmd.Flags()
	f.StringVar(&o.TZ, "tz", "", "IANA zone for reset days and dates (default: this machine's)")
	f.StringVar(&o.TimeFormat, "time-format", "", "how reset times read: "+strings.Join(config.TimeFormatValues(), ", ")+" (default: config usage.time_format, else relative)")
	f.StringVar(&o.DurationFormat, "duration-format", "", "how a reset a day or more away reads: "+strings.Join(config.DurationFormatValues(), ", ")+" (default: config usage.duration_format, else truncate)")
}

// Text cells.
const (
	usagePercentFmt = "%.0f%%"
	usageReasonSep  = ": "
)
