package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/resolve"
)

// checkAgent reports one agent: its binary and version from install
// metadata (edges 7, 8, 9), agentio's disk checks for every configured
// home, and with vendorChecks its --version run (cost X). A missing
// binary fails only for an agent in use (named by --agent, a profile or
// default_agent); for any other it is information.
func (a *App) checkAgent(ctx context.Context, r *DoctorReport, c config.Config, ag agentio.Agent, named, vendorChecks bool) {
	id := ag.ID()
	homes := profileHomes(c, id, a)
	used := named || len(homes) > 0 || c.DefaultAgent == id
	inst, installed := installOf(ag)
	compat, hasCompat := agents.Compat(id)
	if !installed {
		st := agentio.CheckInfo
		if used {
			st = agentio.CheckFail
		}
		r.add(DoctorCheck{Area: AreaBinary, Agent: id, ID: agentio.CheckBinary, Status: st, Message: missingBinaryText(ag)})
	} else {
		r.add(DoctorCheck{Area: AreaBinary, Agent: id, ID: agentio.CheckBinary, Status: agentio.CheckOK, Message: a.tilde(inst.Path) + " (" + string(inst.Method) + ")"})
		r.add(versionCheck(id, inst.Version, compat, hasCompat))
		if len(inst.Shadowed) > 0 {
			sh := make([]string, len(inst.Shadowed))
			for i, p := range inst.Shadowed {
				sh[i] = a.tilde(p)
			}
			r.add(DoctorCheck{Area: AreaBinary, Agent: id, ID: CheckBinaryShadowed, Status: agentio.CheckInfo,
				Message: "also on PATH, never run: " + strings.Join(sh, ", ")})
		}
	}
	if d, ok := agentio.DoctorOf(ag); ok {
		for _, h := range homes {
			home := resolve.HomeOf(ag, h.dir, h.profile)
			for _, ck := range d.Checks(ctx, home) {
				// Binary and version are agent facts, reported once above.
				if ck.ID == agentio.CheckBinary || ck.ID == agentio.CheckVersionTested {
					continue
				}
				dc := DoctorCheck{Area: AreaHome, Agent: id, Home: a.tilde(h.dir), ID: ck.ID, Status: ck.Status, Message: ck.Message}
				if ck.Fix != nil {
					dc.Fix = ck.Fix.Redacted().String()
				}
				r.add(dc)
			}
		}
	}
	if vendorChecks && installed {
		r.add(vendorCheck(ctx, ag, compat, hasCompat))
	}
}

// missingBinaryText is edge 7's message: the binary and, when agentio
// knows the npm package, the install command.
func missingBinaryText(ag agentio.Agent) string {
	msg := ag.Spec().Binary + " not found on PATH"
	if h := installHint(ag); h != "" {
		msg += "; install: " + h
	}
	return msg
}

// installHint is the npm install command for the agent's first package;
// "" when agentio names none.
func installHint(ag agentio.Agent) string {
	if pk := ag.Spec().Packages; len(pk) > 0 {
		return "npm i -g " + pk[0]
	}
	return ""
}

// versionCheck places the installed version against the tested range:
// ok inside, warn newer or older (edge 8), warn unknown (edge 9). A
// custom agent has no range: ok with a version, info without.
func versionCheck(id agentio.ID, v agentio.Version, compat agentio.Compat, hasCompat bool) DoctorCheck {
	dc := DoctorCheck{Area: AreaBinary, Agent: id, ID: agentio.CheckVersionTested}
	if !hasCompat {
		if v.IsZero() {
			dc.Status, dc.Message = agentio.CheckInfo, "version unknown (no install metadata)"
			return dc
		}
		dc.Status, dc.Message = agentio.CheckOK, "version "+v.String()+" (no tested range for a custom agent)"
		return dc
	}
	dc.Status, dc.Message = compatText(id, v, compat)
	return dc
}

// compatText is the status and message for v against compat, worded as
// spec edge 8 ("claude 2.1.330 is newer than tested (2.1.325)").
func compatText(id agentio.ID, v agentio.Version, compat agentio.Compat) (agentio.CheckStatus, string) {
	switch compat.Check(v) {
	case agentio.CompatInRange:
		return agentio.CheckOK, fmt.Sprintf("%s %s is tested (%s)", id, v, compatRange(compat))
	case agentio.CompatNewer:
		return agentio.CheckWarn, fmt.Sprintf("%s %s is newer than tested (%s)", id, v, compat.Max)
	case agentio.CompatOlder:
		return agentio.CheckWarn, fmt.Sprintf("%s %s is older than tested (%s)", id, v, compat.Min)
	}
	return agentio.CheckWarn, fmt.Sprintf("%s version unknown (no install metadata); tested %s", id, compatRange(compat))
}

// vendorCheck runs the binary's --version (cost X) and places the answer
// against the tested range. A failed run fails the check; an agent that
// cannot report its version is skipped with its reason.
func vendorCheck(ctx context.Context, ag agentio.Agent, compat agentio.Compat, hasCompat bool) DoctorCheck {
	dc := DoctorCheck{Area: AreaVendor, Agent: ag.ID(), ID: CheckVendorVersion}
	if _, ok := agentio.BinaryVersionOf(ag); !ok {
		dc.Status, dc.Message = agentio.CheckSkip, "no --version run: "+unsupportedReason(ag, agentio.CapBinaryVersion)
		return dc
	}
	v, err := vendorVersion(ctx, ag)
	if err != nil {
		dc.Status, dc.Message = agentio.CheckFail, apperr.Text(err)
		return dc
	}
	if !hasCompat {
		dc.Status, dc.Message = agentio.CheckOK, ag.Spec().Binary+" --version: "+v.String()
		return dc
	}
	st, msg := compatText(ag.ID(), v, compat)
	dc.Status, dc.Message = st, ag.Spec().Binary+" --version: "+msg
	return dc
}

// profileHome is one configured home of an agent and the first profile
// (by name) that uses it.
type profileHome struct {
	dir, profile string
}

// profileHomes lists the distinct homes of id's profiles, in profile-name
// order (shared homes appear once).
func profileHomes(c config.Config, id agentio.ID, a *App) []profileHome {
	var out []profileHome
	seen := map[string]bool{}
	for _, name := range sortedNames(c.Profiles) {
		p := c.Profiles[name]
		if p.Agent != id {
			continue
		}
		dir := filepath.Clean(config.ExpandPath(p.Home, a.d.Lookup, a.d.Host.UserHome))
		if seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, profileHome{dir: dir, profile: name})
	}
	return out
}
