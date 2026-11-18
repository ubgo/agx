package cli

import (
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
)

// Fields of doctor's data (help --json): one object per check.
var doctorFields = []string{"data.*.area", "data.*.agent", "data.*.home", "data.*.id", "data.*.status", "data.*.message", "data.*.fix"}

// doctorCmd is agx doctor (spec 9): local checks, exit 1 when one fails.
// The report is printed in full either way (render KeepData).
func (c *cliCtx) doctorCmd() *cobra.Command {
	var o app.DoctorOptions
	cmd := c.newCmd("doctor", "Local checks: config, references, binaries, homes, run folders", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			rep, derr := c.App().Doctor(cmd.Context(), o)
			checks := rep.Checks
			if checks == nil {
				checks = []app.DoctorCheck{}
			}
			data, err := render.Encode(checks)
			if err != nil {
				return render.Output{}, err
			}
			st, width := c.style(), c.env.Width
			return render.Output{Data: data, KeepData: true, Text: func(w io.Writer) error { return doctorText(w, checks, width, st) }}, derr
		})
	cmd.Long = "Checks this machine without the network, without resolving a secret and without running a vendor binary: config, references (parse, scheme plugin, program on PATH), tokens.jsonc mode, ended run folders (reported, never deleted), the shell layer, each agent's binary and version, and agentio's checks of every configured home. Plain token values in config are listed by key. Exits 1 when a check fails."
	cmd.Example = "  agx doctor\n  agx doctor --agent codex\n  agx doctor --resolved-secrets --vendor-checks"
	f := cmd.Flags()
	f.StringVar(&o.Agent, "agent", "", "check only this agent's binary and homes")
	f.BoolVar(&o.ResolvedSecrets, "resolved-secrets", false, "also resolve every reference (cost X); values are never shown")
	f.BoolVar(&o.VendorChecks, "vendor-checks", false, "also run each binary's --version (cost X)")
	return fields(cmd, doctorFields...)
}

// doctor's report, ported from agx v0.3: one line per check, a mark, the
// thing checked and the finding, so a glance down the marks finds what
// needs a look. The check id stays in --json.

// doctorMarks are the line marks per status (v0.3's).
var doctorMarks = map[agentio.CheckStatus]string{
	agentio.CheckOK:   "✓",
	agentio.CheckInfo: "·",
	agentio.CheckWarn: "!",
	agentio.CheckFail: "✗",
	agentio.CheckSkip: "-",
}

// doctorTones colours each mark: a pass blue, a warning amber, a failure
// red, notes and skips dim.
var doctorTones = map[agentio.CheckStatus]render.Tone{
	agentio.CheckOK:   render.ToneGood,
	agentio.CheckInfo: render.ToneDim,
	agentio.CheckWarn: render.ToneWarn,
	agentio.CheckFail: render.ToneBad,
	agentio.CheckSkip: render.ToneDim,
}

// doctorFixWord leads a check's fix line.
const doctorFixWord = "fix: "

// doctorSubject is what a check is about: its area, then the agent and
// home when it is about one ("home claude ~/.claude").
func doctorSubject(ch app.DoctorCheck) string {
	parts := []string{ch.Area}
	if ch.Agent != "" {
		parts = append(parts, string(ch.Agent))
	}
	if ch.Home != "" {
		parts = append(parts, ch.Home)
	}
	return strings.Join(parts, " ")
}

// doctorText prints "✓ subject  message" per check, the subjects padded
// to one width, a long message wrapped under itself and the fix on its
// own line below.
func doctorText(w io.Writer, checks []app.DoctorCheck, width int, st render.Style) error {
	pad := 0
	for _, ch := range checks {
		pad = max(pad, len([]rune(doctorSubject(ch))))
	}
	// The mark, a space, the subject column and two spaces.
	indent := strings.Repeat(" ", 2+pad+2)
	msgWidth := max(render.MinWidth, lineWidth(width)) - len(indent)
	ew := textout.New(w)
	for _, ch := range checks {
		subj := doctorSubject(ch)
		head := st.Paint(doctorTones[ch.Status], doctorMarks[ch.Status]) + " " + subj + strings.Repeat(" ", pad-len([]rune(subj))) + "  "
		for i, line := range render.Wrap(ch.Message, msgWidth) {
			if i > 0 {
				head = indent
			}
			ew.Line(strings.TrimRight(head+line, " "))
		}
		if ch.Fix != "" {
			ew.Line(indent + st.Dim(doctorFixWord) + ch.Fix)
		}
	}
	return ew.Err()
}

// lineWidth is the line width, or render.DefaultWidth when unknown.
func lineWidth(width int) int {
	if width <= 0 {
		return render.DefaultWidth
	}
	return width
}
