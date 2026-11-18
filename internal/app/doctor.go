package app

import (
	"context"
	"strconv"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
)

// Doctor areas: which part of the machine a check looked at (spec 9).
// They group the human report and let --field data.*.area filter it.
const (
	// AreaConfig is config.jsonc: schema, version and cross-checks.
	AreaConfig = "config"
	// AreaReference is a secret reference in config: parse, scheme, program.
	AreaReference = "reference"
	// AreaPlainToken is a plain token value in config (info only, edge 23).
	AreaPlainToken = "plain-token"
	// AreaFiles is agx's own files (tokens.jsonc mode, stored values).
	AreaFiles = "files"
	// AreaRuns is the run folders (stale ones are reported, edge 47).
	AreaRuns = "runs"
	// AreaShell is the shell layer (spec 12).
	AreaShell = "shell"
	// AreaBinary is an agent binary and its version (edges 7, 8, 9).
	AreaBinary = "binary"
	// AreaHome is agentio's per-home disk checks (DoctorOf).
	AreaHome = "home"
	// AreaSecret is a reference resolved with --resolved-secrets (cost X).
	AreaSecret = "secret"
	// AreaVendor is a binary run with --vendor-checks (cost X).
	AreaVendor = "vendor"
)

// agx's own check ids. agentio's CheckID is an open set ("vendors add
// their own"); agx adds these beside agentio's per-home ids.
const (
	CheckConfigValid      agentio.CheckID = "config-valid"
	CheckReferenceParse   agentio.CheckID = "reference-parse"
	CheckReferenceScheme  agentio.CheckID = "reference-scheme"
	CheckReferenceProgram agentio.CheckID = "reference-program"
	CheckReferenceVar     agentio.CheckID = "reference-variable"
	CheckEnvFile          agentio.CheckID = "env-file"
	CheckPlainToken       agentio.CheckID = "plain-token"
	CheckTokensMode       agentio.CheckID = "tokens-mode"
	CheckStoredToken      agentio.CheckID = "stored-token"
	CheckRunFolder        agentio.CheckID = "run-folder"
	CheckShellLayer       agentio.CheckID = "shell-layer"
	CheckBinaryShadowed   agentio.CheckID = "binary-shadowed"
	CheckSecretResolves   agentio.CheckID = "secret-resolves"
	CheckVendorVersion    agentio.CheckID = "vendor-version"
)

// DoctorOptions are agx doctor's flags (spec 13.8).
type DoctorOptions struct {
	// Agent limits the agent checks (binary, homes, vendor) to one agent;
	// "" = every known agent.
	Agent string
	// ResolvedSecrets resolves every reference (cost X).
	ResolvedSecrets bool
	// VendorChecks runs each binary's --version (cost X).
	VendorChecks bool
}

// DoctorCheck is one line of the doctor report. It never holds a secret
// value: references are named by their config key, plain tokens by key.
type DoctorCheck struct {
	// Area is one of the Area constants.
	Area string `json:"area"`
	// Agent is the agent the check is about; "" for agx-wide checks.
	Agent agentio.ID `json:"agent,omitempty"`
	// Home is the home folder (~/…) for per-home checks.
	Home string `json:"home,omitempty"`
	// ID names the check (agentio's or agx's CheckID).
	ID agentio.CheckID `json:"id"`
	// Status is skip, ok, info, warn or fail; only fail makes doctor exit 1.
	Status agentio.CheckStatus `json:"status"`
	// Message is the finding, without any secret value.
	Message string `json:"message"`
	// Fix is a command that would fix it, for the user to run; "" when
	// none. Doctor never runs it.
	Fix string `json:"fix,omitempty"`
}

// DoctorReport is the whole report: every check in a stable order
// (config, references, files, runs, shell, then per agent).
type DoctorReport struct {
	Checks []DoctorCheck
}

// Failed counts the checks that failed (doctor exits 1 when > 0).
func (r DoctorReport) Failed() int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == agentio.CheckFail {
			n++
		}
	}
	return n
}

// add appends one check.
func (r *DoctorReport) add(c DoctorCheck) { r.Checks = append(r.Checks, c) }

// Doctor runs the local checks of spec 9: no network, no secret
// resolution and no vendor binary run unless asked (--resolved-secrets,
// --vendor-checks, cost X). It always returns the full report; the error
// is non-nil only for a usage problem (an unknown --agent) or when a
// check failed (apperr code checks-failed, exit 1), and the report is
// still complete then.
func (a *App) Doctor(ctx context.Context, o DoctorOptions) (DoctorReport, error) {
	var r DoctorReport
	cfg, cfgErr := a.Config()
	a.checkConfig(&r, cfg, cfgErr)
	reg, err := a.doctorRegistry(cfgErr)
	if err != nil {
		return r, err
	}
	scope := reg.All()
	if o.Agent != "" {
		ag, ok := reg.Get(agentio.ID(o.Agent))
		if !ok {
			return r, unknownAgent(reg, o.Agent)
		}
		scope = []agentio.Agent{ag}
	}
	var c config.Config
	if cfgErr == nil {
		c = cfg.Config
		a.checkReferences(&r, c)
		a.checkPlainTokens(&r, c, reg)
		a.checkTokensFile(&r, c)
	}
	a.checkRuns(ctx, &r)
	a.checkShell(&r)
	for _, ag := range scope {
		a.checkAgent(ctx, &r, c, ag, o.Agent != "", o.VendorChecks)
	}
	if o.ResolvedSecrets && cfgErr == nil {
		if err := a.checkResolved(ctx, &r, c); err != nil {
			return r, err
		}
	}
	if err := ctxErr(ctx); err != nil {
		return r, err
	}
	if n := r.Failed(); n > 0 {
		return r, apperr.Failure(apperr.CodeChecksFailed, failedText(n))
	}
	return r, nil
}

// failedText is the doctor failure sentence.
func failedText(n int) string {
	if n == 1 {
		return "1 check failed"
	}
	return strconv.Itoa(n) + " checks failed"
}

// doctorRegistry is the agent registry; with an invalid config (already
// reported) it falls back to the built-ins so binary and home checks
// still run.
func (a *App) doctorRegistry(cfgErr error) (*agents.Registry, error) {
	if cfgErr == nil {
		return a.Registry()
	}
	reg, err := agents.New(a.d.Host, nil)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "agents: "+err.Error())
	}
	return reg, nil
}
