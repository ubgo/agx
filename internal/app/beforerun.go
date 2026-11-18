package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/textdata/shell"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
)

// BeforeRunTimeout bounds a before_run script (spec 7.6): a script that
// hangs stops the launch instead of the agent never starting.
const BeforeRunTimeout = 60 * time.Second

// goosWindows selects PowerShell quoting for the script path (textdata's
// runner uses pwsh/powershell there, sh -c elsewhere).
const goosWindows = "windows"

// beforeRunPlan resolves before_run (spec 7.6) without running it:
// --before-run PATH (relative to the current folder) beats the profile's
// before_run, which is found relative to the config dir
// (before_run_from config, the default) or the launch folder
// (before_run_from folder). A missing script refuses the launch (edge
// 42), in --dry-run too, which shows the path and its source.
func (a *App) beforeRunPlan(o LaunchOptions, res resolve.Result, p *LaunchPlan) error {
	prof := res.Profile
	var raw, base, src string
	switch {
	case o.BeforeRun != "":
		raw, base, src = o.BeforeRun, a.d.Cwd, "--before-run"
		if prof.BeforeRun != "" {
			src += " (overrides profile " + res.ProfileName + ": " + prof.BeforeRun + ")"
		}
	case prof.BeforeRun != "":
		raw, base = prof.BeforeRun, a.paths.Dir
		from := prof.BeforeRunFrom
		if from == "" {
			from = config.BeforeRunFromConfig
		}
		if from == config.BeforeRunFromFolder {
			base = o.Dir
		}
		src = fmt.Sprintf("profile %s (before_run_from %s)", res.ProfileName, from)
	default:
		return nil
	}
	path := resolvePath(raw, base, a)
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return apperr.Failure(apperr.CodeBeforeRunFailed, fmt.Sprintf("before_run %s not found in %s", raw, a.tilde(base)))
	}
	full := prof.BeforeRunEnv == config.BeforeRunEnvFull
	p.BeforeRun = &BeforeRunPlan{Path: path, Source: src, Dir: o.Dir, FullEnv: full}
	val := a.tilde(path)
	if full {
		val += " (before_run_env full: credential variables kept)"
	}
	p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecBefore, Value: val, Source: src})
	return nil
}

// runBeforeRun runs the planned script in the launch folder through
// textdata's shell runner: BeforeRunTimeout, stdin closed, stdout
// discarded and never put in an error. Its environment is the agent's
// launch environment without credential variables (the token, the
// agent's credential and token variables) unless before_run_env full.
// Any failure stops the launch (edge 43, before-run-failed).
func (a *App) runBeforeRun(ctx context.Context, p *LaunchPlan) error {
	br := p.BeforeRun
	r := shell.Runner{Timeout: BeforeRunTimeout, Dir: br.Dir, Env: a.beforeRunEnv(p)}
	if _, err := r.Run(ctx, scriptCommand(br.Path)); err != nil {
		return beforeRunError(ctx, err)
	}
	return nil
}

// beforeRunEnv is the script's environment: the built command's env,
// minus every credential variable unless FullEnv.
func (a *App) beforeRunEnv(p *LaunchPlan) []string {
	env := slices.Clone(p.Command.Env)
	if p.BeforeRun.FullEnv {
		return env
	}
	drop := p.Command.SecretEnvNames()
	if reg, err := a.Registry(); err == nil {
		if ag, ok := reg.Get(p.Agent); ok {
			spec := ag.Spec()
			drop = append(drop, spec.CredentialEnv...)
			for _, te := range spec.TokenEnv {
				drop = append(drop, te.Env)
			}
		}
	}
	return slices.DeleteFunc(env, func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return slices.Contains(drop, k)
	})
}

// scriptCommand quotes path as one word for the runner's shell, so a path
// with spaces or quotes runs as that one file.
func scriptCommand(path string) string {
	if runtime.GOOS == goosWindows {
		return "& '" + strings.ReplaceAll(path, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// beforeRunError maps a script failure: exit code and the first stderr
// line, a timeout, or Ctrl-C; never stdout.
func beforeRunError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return apperr.Wrap(err, apperr.CodeInterrupted, apperr.ClassInterrupted, "before_run: interrupted")
	}
	var ee *shell.ExitError
	switch {
	case errors.As(err, &ee):
		msg := fmt.Sprintf("before_run failed (exit %d)", ee.Code)
		if line, _, _ := strings.Cut(ee.Stderr, "\n"); line != "" {
			msg += ": " + line
		}
		return apperr.Wrap(err, apperr.CodeBeforeRunFailed, apperr.ClassFailure, msg)
	case errors.Is(err, shell.ErrTimeout):
		return apperr.Wrap(err, apperr.CodeBeforeRunFailed, apperr.ClassFailure, fmt.Sprintf("before_run timed out after %s", BeforeRunTimeout))
	}
	return apperr.Wrap(err, apperr.CodeBeforeRunFailed, apperr.ClassFailure, "before_run failed: "+err.Error())
}
