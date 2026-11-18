package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
)

// Agent kinds (agent list "kind", spec 3.1): an agentio built-in module
// or an agents.<id>.spec entry in config.jsonc.
const (
	AgentKindBuiltin = "built-in"
	AgentKindCustom  = "custom"
)

// versionUnknown is shown when no install metadata names the version
// (edge 9); agx never runs the binary to find out unless asked
// (--vendor-checks, cost X).
const versionUnknown = "unknown"

// AgentInfo is one row of agx agent list and the head of agent show: what
// agx knows about an agent from agentio and the disk, never from running
// its binary.
type AgentInfo struct {
	// ID is the agent id (--agent, --id).
	ID agentio.ID `json:"id"`
	// Name is the display name ("Claude Code").
	Name string `json:"name"`
	// Kind is AgentKindBuiltin or AgentKindCustom.
	Kind string `json:"kind"`
	// Binary is the executable name looked up on PATH.
	Binary string `json:"binary"`
	// Path is the PATH hit; "" when the binary is not installed.
	Path string `json:"path"`
	// Version is the installed version from install metadata, "unknown"
	// when the install does not say, "" when not installed.
	Version string `json:"version"`
	// Method is how the binary was installed (npm, native, brew, …).
	Method string `json:"method,omitempty"`
	// Tested is the version range agentio was tested against
	// ("2.1.323..2.1.325"); "" for a custom agent, which has none.
	Tested string `json:"tested"`
	// Compat places Version against Tested: in-range, newer, older or
	// unknown (agentio CompatStatus).
	Compat string `json:"compat"`
	// Capabilities are the agentio capabilities the agent has, in
	// agentio's canonical order (generated, never a hand table, spec 3.2).
	Capabilities []string `json:"capabilities"`
}

// CapabilityInfo is one capability of agent show: whether the agent has
// it, its cost class, and the agent's own reason when it does not.
type CapabilityInfo struct {
	// Name is the agentio capability name ("usage", "token-login").
	Name string `json:"name"`
	// Supported is true when the agent has the capability.
	Supported bool `json:"supported"`
	// Reason is why it is missing (agentio.Unsupported); "" when supported.
	Reason string `json:"reason,omitempty"`
	// Cost is the capability's worst-case cost class (agentio Cost).
	Cost string `json:"cost"`
}

// AgentDetail is agx agent show: the list row plus every capability with
// its reason, the autonomy levels and token kinds the agent accepts.
type AgentDetail struct {
	AgentInfo
	// Shadowed lists other PATH entries with the same binary name, which
	// never run.
	Shadowed []string `json:"shadowed"`
	// Autonomy lists the autonomy levels a launch can ask for (the agent
	// refuses any other, principle 9).
	Autonomy []string `json:"autonomy"`
	// TokenKinds lists the token login kinds; empty when the agent has no
	// token login.
	TokenKinds []string `json:"token_kinds"`
	// CapabilityDetail is every agentio capability, supported or not.
	CapabilityDetail []CapabilityInfo `json:"capability_detail"`
	// VendorVersion is what `<binary> --version` printed, only with
	// --vendor-checks (cost X); "" otherwise.
	VendorVersion string `json:"vendor_version,omitempty"`
	// VendorError is why the --vendor-checks run failed; "" otherwise.
	VendorError string `json:"vendor_error,omitempty"`
}

// AgentShowOptions are agx agent show's flags.
type AgentShowOptions struct {
	// ID is --id, the agent to show; required.
	ID string
	// VendorChecks runs the binary's --version (cost X).
	VendorChecks bool
}

// noReason stands in when an agent lacks a capability without declaring
// why (agentio.Unsupported returned no reason).
const noReason = "not declared by the agent"

// AgentList lists every known agent, built-ins first then custom ones
// (agx agent list, spec 13.8). Disk · Read: install metadata only.
func (a *App) AgentList() ([]AgentInfo, error) {
	reg, err := a.Registry()
	if err != nil {
		return nil, err
	}
	out := make([]AgentInfo, 0, len(reg.All()))
	for _, ag := range reg.All() {
		info, _ := a.agentInfo(reg, ag)
		out = append(out, info)
	}
	return out, nil
}

// AgentShow describes one agent with every capability and its reason
// when missing (agx agent show, spec 13.8).
func (a *App) AgentShow(ctx context.Context, o AgentShowOptions) (AgentDetail, error) {
	if o.ID == "" {
		return AgentDetail{}, apperr.Usage("agent show needs --id (agx agent list shows the ids)")
	}
	reg, err := a.Registry()
	if err != nil {
		return AgentDetail{}, err
	}
	ag, ok := reg.Get(agentio.ID(o.ID))
	if !ok {
		return AgentDetail{}, unknownAgent(reg, o.ID)
	}
	info, inst := a.agentInfo(reg, ag)
	d := AgentDetail{AgentInfo: info, Shadowed: inst.Shadowed, Autonomy: autonomyLevels(ag, inst.Version), TokenKinds: tokenKinds(ag)}
	if d.Shadowed == nil {
		d.Shadowed = []string{}
	}
	supported := agentio.Supported(ag)
	for _, c := range agentio.CapabilityValues() {
		ci := CapabilityInfo{Name: c.String(), Supported: slices.Contains(supported, c), Cost: c.Cost().Class.String()}
		if !ci.Supported {
			ci.Reason = unsupportedReason(ag, c)
		}
		d.CapabilityDetail = append(d.CapabilityDetail, ci)
	}
	if o.VendorChecks {
		v, err := vendorVersion(ctx, ag)
		if err != nil {
			d.VendorError = apperr.Text(err)
		} else {
			d.VendorVersion = v.String()
		}
	}
	return d, nil
}

// unknownAgent is the not-found error naming the known ids.
func unknownAgent(reg *agents.Registry, id string) error {
	ids := make([]string, 0, len(reg.IDs()))
	for _, k := range reg.IDs() {
		ids = append(ids, string(k))
	}
	return apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("unknown agent %s; known: %s", id, strings.Join(ids, ", ")))
}

// agentInfo builds the list row from agentio and install metadata. The
// Install is returned for show's extra fields; zero when not installed.
func (a *App) agentInfo(reg *agents.Registry, ag agentio.Agent) (AgentInfo, agentio.Install) {
	info := AgentInfo{ID: ag.ID(), Name: ag.Name(), Kind: AgentKindBuiltin, Binary: ag.Spec().Binary, Capabilities: []string{}}
	if reg.IsCustom(ag.ID()) {
		info.Kind = AgentKindCustom
	}
	compat, hasCompat := agents.Compat(ag.ID())
	if hasCompat {
		info.Tested = compatRange(compat)
	}
	for _, c := range agentio.Supported(ag) {
		info.Capabilities = append(info.Capabilities, c.String())
	}
	inst, installed := installOf(ag)
	if installed {
		info.Path = a.tilde(inst.Path)
		info.Method = string(inst.Method)
		info.Version = versionText(inst.Version)
	}
	info.Compat = agentio.CompatUnknown.String()
	if hasCompat && installed {
		info.Compat = compat.Check(inst.Version).String()
	}
	return info, inst
}

// installOf reads the install from disk: the agent's Installer when it
// has one, else the PATH hit alone. ok is false when the binary is not on
// PATH.
func installOf(ag agentio.Agent) (agentio.Install, bool) {
	if in, ok := agentio.InstallerOf(ag); ok {
		inst, err := in.Install()
		if err == nil {
			return inst, true
		}
		if errors.Is(err, agentio.ErrNotInstalled) {
			return agentio.Install{}, false
		}
	}
	p, err := ag.LookPath()
	if err != nil {
		return agentio.Install{}, false
	}
	return agentio.Install{Path: p, RealPath: p, Method: agentio.InstallUnknown}, true
}

// versionText is a version for display: "unknown" for the zero version.
func versionText(v agentio.Version) string {
	if v.IsZero() {
		return versionUnknown
	}
	return v.String()
}

// compatRange renders a tested range as "min..max".
func compatRange(c agentio.Compat) string { return c.Min.String() + ".." + c.Max.String() }

// unsupportedReason is the agent's own reason for lacking c.
func unsupportedReason(ag agentio.Agent, c agentio.Capability) string {
	if r, ok := agentio.Unsupported(ag, c); ok && r != "" {
		return r
	}
	return noReason
}

// autonomyLevels lists the levels a launch accepts, in agentio's canonical
// order, by asking the agent's own launch builder for each level.
//
// Why the builder and not Spec().AutonomyArgs: vendor agents (Claude,
// Codex) map autonomy in their typed builders and leave the spec map
// empty, so reading the map listed only "default" while agx run accepted
// more. Launch is Pure (no process, no file), so probing it per level is
// free and gives exactly the answer agx run --dry-run gets.
//
// The probe target is the agent's default home with the installed
// version (version gates refuse flags an older binary lacks, as in a real
// launch) and AllowDangerous set, so "full" is listed when the builder
// accepts it once consent is given; the launch itself still demands
// --consent full-autonomy. Any refusal (ErrUnsupported or another error)
// means the launch would refuse the level too, so it is not listed.
func autonomyLevels(ag agentio.Agent, installed agentio.Version) []string {
	t := ag.DefaultHome().Target()
	t.Version, t.AllowDangerous = installed, true
	out := []string{}
	for _, l := range agentio.AutonomyValues() {
		if _, err := ag.Launch(t, agentio.LaunchRequest{Autonomy: l}); err == nil {
			out = append(out, l.String())
		}
	}
	return out
}

// tokenKinds lists the token login kinds; empty without a token login.
func tokenKinds(ag agentio.Agent) []string {
	out := []string{}
	tl, ok := agentio.TokenLoginOf(ag)
	if !ok {
		return out
	}
	for _, k := range tl.TokenKinds() {
		out = append(out, k.String())
	}
	return out
}

// vendorVersion runs `<binary> --version` through agentio (cost X: a
// process that may touch the login). An agent without the capability
// gives its typed reason.
func vendorVersion(ctx context.Context, ag agentio.Agent) (agentio.Version, error) {
	bv, ok := agentio.BinaryVersionOf(ag)
	if !ok {
		return agentio.Version{}, errors.New("no --version run: " + unsupportedReason(ag, agentio.CapBinaryVersion))
	}
	return bv.BinaryVersion(ctx)
}
