package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/login"
)

// EnvShell is set by the shell layer's wrapper (agx shell-init) to the
// shell it was generated for; doctor reads it to say whether the layer
// is loaded in this terminal (spec 9, 12).
const EnvShell = "AGX_SHELL"

// shellInitHint is how to load the shell layer.
const shellInitHint = `eval "$(agx shell-init --shell zsh)"`

// checkConfig reports the config file: missing (info), valid (ok), or
// every problem the loader found, each at its line:col (fail).
func (a *App) checkConfig(r *DoctorReport, cfg *config.Loaded, err error) {
	if err != nil {
		for _, p := range config.ProblemsOf(err) {
			r.add(DoctorCheck{Area: AreaConfig, ID: CheckConfigValid, Status: agentio.CheckFail, Message: p.String()})
		}
		return
	}
	if !cfg.Exists {
		r.add(DoctorCheck{Area: AreaConfig, ID: CheckConfigValid, Status: agentio.CheckInfo,
			Message: "no config file at " + a.tilde(a.paths.File), Fix: "agx config init"})
		return
	}
	r.add(DoctorCheck{Area: AreaConfig, ID: CheckConfigValid, Status: agentio.CheckOK, Message: a.tilde(a.paths.File) + " is valid"})
}

// checkTokensFile reports tokens.jsonc: its mode (0600, checked on
// POSIX) and, for every stored login, that its value is there. Values are
// read only to test presence and never leave this function.
func (a *App) checkTokensFile(r *DoctorReport, c config.Config) {
	stored := storedLogins(c)
	st, err := os.Stat(a.paths.Tokens)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if len(stored) == 0 {
			return
		}
	case err != nil:
		r.add(DoctorCheck{Area: AreaFiles, ID: CheckTokensMode, Status: agentio.CheckFail, Message: "read " + a.tilde(a.paths.Tokens) + ": " + apperr.Text(err)})
		return
	default:
		if merr := login.CheckMode(a.paths.Tokens, st.Mode()); merr != nil {
			r.add(DoctorCheck{Area: AreaFiles, ID: CheckTokensMode, Status: agentio.CheckFail,
				Message: fmt.Sprintf("%s has mode %04o; it holds tokens and must be 0600", a.tilde(a.paths.Tokens), st.Mode().Perm()),
				Fix:     "chmod 600 " + a.paths.Tokens})
			return
		}
		r.add(DoctorCheck{Area: AreaFiles, ID: CheckTokensMode, Status: agentio.CheckOK, Message: a.tilde(a.paths.Tokens) + " is private"})
	}
	for _, name := range stored {
		_, found, err := login.ReadStored(a.paths, name)
		switch {
		case err != nil:
			r.add(DoctorCheck{Area: AreaFiles, ID: CheckStoredToken, Status: agentio.CheckFail, Message: "login " + name + ": " + apperr.Text(err)})
		case !found:
			r.add(DoctorCheck{Area: AreaFiles, ID: CheckStoredToken, Status: agentio.CheckFail,
				Message: "login " + name + ": no stored value in tokens.jsonc", Fix: "agx login edit --id " + name + " --stdin"})
		default:
			r.add(DoctorCheck{Area: AreaFiles, ID: CheckStoredToken, Status: agentio.CheckOK, Message: "login " + name + ": stored value present"})
		}
	}
}

// storedLogins lists the logins whose value lives in tokens.jsonc, sorted.
func storedLogins(c config.Config) []string {
	var out []string
	for _, name := range sortedNames(c.Logins) {
		if c.Logins[name].Stored {
			out = append(out, name)
		}
	}
	return out
}

// checkRuns reports run folders whose agent has ended (edge 47): they are
// kept until agx clean, never deleted here. A run agx cannot verify is
// listed with the reason (edge 49); a live run is not reported.
func (a *App) checkRuns(ctx context.Context, r *DoctorReport) {
	folders, err := run.List(a.paths.Runs)
	if err != nil {
		r.add(DoctorCheck{Area: AreaRuns, ID: CheckRunFolder, Status: agentio.CheckWarn, Message: "read " + a.tilde(a.paths.Runs) + ": " + apperr.Text(err)})
		return
	}
	before := len(r.Checks)
	for _, f := range folders {
		if f.RecordErr != nil {
			r.add(DoctorCheck{Area: AreaRuns, ID: CheckRunFolder, Status: agentio.CheckInfo,
				Message: "run " + f.ID + ": cannot verify: " + apperr.Text(f.RecordErr) + "; kept"})
			continue
		}
		live, lerr := a.d.Host.ProcessAlive(ctx, f.Record.PID, f.Record.StartedAt, run.EnvRunID, f.ID)
		switch {
		case lerr != nil || live == agentio.LivenessUnknown:
			r.add(DoctorCheck{Area: AreaRuns, Agent: agentio.ID(f.Record.Agent), ID: CheckRunFolder, Status: agentio.CheckInfo,
				Message: "run " + f.ID + ": cannot verify: " + unknownReason(lerr) + "; kept"})
		case live == agentio.LivenessEnded:
			r.add(DoctorCheck{Area: AreaRuns, Agent: agentio.ID(f.Record.Agent), ID: CheckRunFolder, Status: agentio.CheckInfo,
				Message: "run " + f.ID + " ended (profile " + f.Record.Profile + "); kept until agx clean", Fix: "agx clean"})
		}
	}
	switch {
	case len(folders) == 0:
		r.add(DoctorCheck{Area: AreaRuns, ID: CheckRunFolder, Status: agentio.CheckOK, Message: "no run folders"})
	case len(r.Checks) == before:
		r.add(DoctorCheck{Area: AreaRuns, ID: CheckRunFolder, Status: agentio.CheckOK, Message: fmt.Sprintf("%d run folders, none ended", len(folders))})
	}
}

// unknownReason explains an unknown liveness.
func unknownReason(err error) string {
	if err != nil {
		return apperr.Text(err)
	}
	return "process info unreadable"
}

// checkShell reports whether the shell layer is loaded in this terminal.
// Not loaded is information, never a failure: every command works
// without it, only cd after new/resume and login use --terminal need it.
func (a *App) checkShell(r *DoctorReport) {
	if sh, ok := a.d.Lookup(EnvShell); ok && sh != "" {
		r.add(DoctorCheck{Area: AreaShell, ID: CheckShellLayer, Status: agentio.CheckOK, Message: "shell layer loaded (" + sh + ")"})
		return
	}
	r.add(DoctorCheck{Area: AreaShell, ID: CheckShellLayer, Status: agentio.CheckInfo,
		Message: "shell layer not loaded: new and resume leave this shell where it is", Fix: shellInitHint})
}
