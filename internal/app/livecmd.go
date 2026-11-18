package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
)

// LiveRun is one live-login run as agx live list / show / use print it.
// Login fields are names, never values.
type LiveRun struct {
	ID      string `json:"id"`
	Dir     string `json:"dir"`
	Agent   string `json:"agent"`
	Profile string `json:"profile"`
	Home    string `json:"home"`
	Folder  string `json:"folder"`
	// StartLogin, ChosenLogin, InUse: live.State.
	StartLogin  string    `json:"start_login"`
	ChosenLogin string    `json:"chosen_login"`
	InUse       string    `json:"in_use"`
	InUseAt     time.Time `json:"in_use_at,omitzero"`
	// Next is the login the agent gets at its next refresh.
	Next string `json:"next"`
	// Liveness is alive, ended or unknown (agx clean's check); Reason
	// says how it was decided.
	Liveness string `json:"liveness"`
	Reason   string `json:"reason"`
}

// LiveUseOptions are agx live use's flags.
type LiveUseOptions struct {
	// ID is the run id (--id).
	ID string
	// Login is the login to switch to (--login).
	Login string
	// StartLogin goes back to the login the run started with.
	StartLogin bool
}

// Credential is what agx credential hands the agent: the token, printed
// alone on stdout by the CLI and nowhere else. Token never encodes
// (agentio.Token redacts in JSON and every fmt verb).
type Credential struct {
	RunID string        `json:"run_id"`
	Login string        `json:"login"`
	Token agentio.Token `json:"-"`
}

// LiveList returns the running live-login runs, oldest first; agent ""
// = every agent. Runs whose agent has ended are left out (agx clean
// removes them); unknown liveness is listed with its reason.
func (a *App) LiveList(ctx context.Context, agent agentio.ID) ([]LiveRun, error) {
	folders, err := run.List(a.paths.Runs)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "read "+a.paths.Runs+": "+err.Error())
	}
	out := []LiveRun{}
	for _, f := range folders {
		if agent != "" && f.Record.Agent != string(agent) {
			continue
		}
		st, err := live.Read(f.Dir)
		if errors.Is(err, live.ErrNotLive) {
			continue
		}
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, f.ID+": "+err.Error())
		}
		r := a.liveRun(ctx, f, st)
		if r.Liveness == agentio.LivenessEnded.String() {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// LiveShow returns one run's live state, ended or not.
func (a *App) LiveShow(ctx context.Context, id string) (LiveRun, error) {
	f, st, err := a.liveFolder(id)
	if err != nil {
		return LiveRun{}, err
	}
	return a.liveRun(ctx, f, st), nil
}

// LiveUse chooses the login a live run gets at its next refresh (spec
// 10): --login NAME, or --start-login to go back. The login must exist,
// serve the run's agent and be a token login; it is not resolved here
// (agx credential resolves it when the agent asks). A run that has ended
// is refused: nothing would ever read the choice.
func (a *App) LiveUse(ctx context.Context, o LiveUseOptions) (LiveRun, error) {
	switch {
	case o.Login != "" && o.StartLogin:
		return LiveRun{}, apperr.Conflict("--login and --start-login both choose the login; pick one")
	case o.Login == "" && !o.StartLogin:
		return LiveRun{}, apperr.Usage("choose the login: --login NAME or --start-login")
	}
	f, st, err := a.liveFolder(o.ID)
	if err != nil {
		return LiveRun{}, err
	}
	if l, _ := a.runLiveness(ctx, f); l == agentio.LivenessEnded {
		return LiveRun{}, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("run %s has ended; its login can no longer change", f.ID)).WithHint("agx clean --yes")
	}
	if o.Login != "" {
		cfg, err := a.Config()
		if err != nil {
			return LiveRun{}, err
		}
		if _, err := liveLogin(cfg.Config, o.Login, st.Agent); err != nil {
			return LiveRun{}, err
		}
	}
	st, err = live.Update(ctx, f.Dir, func(s *live.State) error {
		s.ChosenLogin = o.Login
		s.ChosenAt = a.Now()
		return nil
	})
	if err != nil {
		return LiveRun{}, liveErr(err)
	}
	return a.liveRun(ctx, f, st), nil
}

// Credential is the hidden agx credential (spec 10): it finds its run by
// AGX_RUN_ID, resolves the chosen login (else the start login) and
// records it as the login in use. Only the CLI prints the token, alone.
//
// It never succeeds without one usable line: Claude Code treats an empty
// or failing helper as a hard failure (three tries, then exit 1, with no
// fallback to the home's own login; LIVEFIX V1), so the failure must say
// why in the one place Claude shows it. Every error is therefore one line
// (oneLine), and a login that does not resolve is reported briefly
// (login.Expander.Brief: the reference, never a store's own stderr) with
// agx login check, which shows the full reason, as the next step.
func (a *App) Credential(ctx context.Context) (Credential, error) {
	c, err := a.liveCredential(ctx)
	if err != nil {
		return Credential{}, oneLine(err)
	}
	return c, nil
}

// liveCredential is Credential before its error is made one line.
func (a *App) liveCredential(ctx context.Context) (Credential, error) {
	runID, ok := a.d.Lookup(run.EnvRunID)
	if !ok || runID == "" {
		return Credential{}, apperr.Usage("agx credential runs inside a live-login session: " + run.EnvRunID + " is not set")
	}
	f, st, err := a.liveFolder(runID)
	if err != nil {
		return Credential{}, err
	}
	cfg, err := a.Config()
	if err != nil {
		return Credential{}, err
	}
	name := st.Want()
	lg, err := liveLogin(cfg.Config, name, st.Agent)
	if err != nil {
		return Credential{}, err
	}
	cred, err := login.Credential(ctx, a.exp.Brief(), a.paths, name, lg)
	if err != nil {
		return Credential{}, withCheckHint(err, name)
	}
	if _, err := live.Update(ctx, f.Dir, func(s *live.State) error {
		s.InUse = name
		s.InUseAt = a.Now()
		return nil
	}); err != nil {
		return Credential{}, liveErr(err)
	}
	return Credential{RunID: f.ID, Login: name, Token: cred.Token}, nil
}

// hintPrefix introduces the next step inside a one-line error.
const hintPrefix = " (try: "

// hintSuffix closes it.
const hintSuffix = ")"

// messageBreaks and hintBreaks are what oneLine replaces: a message line break becomes a
// space, a hint line break a "; " between two next steps.
var (
	messageBreaks = strings.NewReplacer("\r\n", " ", "\n", " ")
	hintBreaks    = strings.NewReplacer("\r\n", "; ", "\n", "; ")
)

// oneLine folds an error's hint into its message and joins any line
// breaks, so agx credential's failure is one stderr line: Claude Code
// shows the helper's stderr in /status, where a second line would be
// read as a separate message. Code, class and cause are kept.
func oneLine(err error) error {
	e := apperr.As(err)
	msg := messageBreaks.Replace(e.Message)
	if hint := strings.TrimSpace(e.Hint); hint != "" {
		msg += hintPrefix + hintBreaks.Replace(hint) + hintSuffix
	}
	return apperr.Wrap(e, e.Code, e.Class, msg)
}

// withCheckHint adds agx login check as the next step of a login that did
// not resolve, unless the error already names its own (tokens.jsonc's
// "agx login edit"): the brief message names the reference, and the check
// shows the full reason.
func withCheckHint(err error, name string) error {
	e := apperr.As(err)
	if e.Hint != "" {
		return e
	}
	return e.WithHint("agx login check --id " + name + " --no-probe")
}

// liveLogin checks that login name can be handed to a live run of agent:
// it exists, serves the agent and is a token login.
func liveLogin(c config.Config, name, agent string) (config.Login, error) {
	lg, ok := c.Logins[name]
	if !ok {
		return config.Login{}, apperr.Failure(apperr.CodeNotFound, "unknown login "+name).WithHint("agx login list")
	}
	if !lg.Serves(agentio.ID(agent)) {
		return config.Login{}, apperr.New(apperr.CodeWrongAgent, apperr.ClassUsage, fmt.Sprintf("login %s serves %s; this run is %s", name, joinAgentIDs(lg.Agents), agent))
	}
	if lg.Kind == config.KindAPIKeyHelper {
		return config.Login{}, apperr.Unsupported(fmt.Sprintf("login %s is an api-key-helper; a live run needs a token login", name))
	}
	return lg, nil
}

// liveFolder finds a live run by id: a well-formed id, an existing run
// folder, and a live.jsonc in it.
func (a *App) liveFolder(runID string) (run.Folder, live.State, error) {
	if runID == "" {
		return run.Folder{}, live.State{}, apperr.Usage("--id RUN is required (agx live list shows the runs)")
	}
	if !run.ValidID(runID) {
		return run.Folder{}, live.State{}, apperr.Usage(fmt.Sprintf("%q is not a run id (YYYYMMDD-HHMMSS-xxxx)", runID))
	}
	dir := run.Dir(a.paths.Runs, runID)
	if !statDir(dir) {
		return run.Folder{}, live.State{}, apperr.Failure(apperr.CodeNotFound, "no run "+runID).WithHint("agx live list")
	}
	f := run.Folder{ID: runID, Dir: dir}
	f.Record, f.RecordErr = run.ReadRecord(dir)
	st, err := live.Read(dir)
	if errors.Is(err, live.ErrNotLive) {
		return f, st, apperr.Failure(apperr.CodeNotFound, "run "+runID+" was not started with --live-login")
	}
	if err != nil {
		return f, st, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, runID+": "+err.Error())
	}
	return f, st, nil
}

// liveRun assembles the printable row of a live run.
func (a *App) liveRun(ctx context.Context, f run.Folder, st live.State) LiveRun {
	l, reason := a.runLiveness(ctx, f)
	return LiveRun{
		ID: f.ID, Dir: f.Dir, Agent: st.Agent, Profile: f.Record.Profile, Home: f.Record.Home, Folder: f.Record.Folder,
		StartLogin: st.StartLogin, ChosenLogin: st.ChosenLogin, InUse: st.InUse, InUseAt: st.InUseAt, Next: st.Want(),
		Liveness: l.String(), Reason: reason,
	}
}

// liveErr keeps an apperr from the live package (a lock timeout) and
// wraps anything else as an I/O failure.
func liveErr(err error) error {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae
	}
	return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "live.jsonc: "+err.Error())
}
