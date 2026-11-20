package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/login"
)

// Warning codes a launch can add.
const (
	WarnVersion    = "version"
	WarnRawDup     = "raw-duplicate"
	WarnInline     = "inline-settings"
	WarnUnmodelled = "unmodelled-key"
	WarnNotice     = "notice"
)

// Login display values in decisions and run.jsonc.
const (
	// Machine values of LaunchPlan.Login, run.jsonc "login" and
	// AGX_RUN_LOGIN when no named login is used: one word, no space, so a
	// script can test them; the human wording ("home login") is only in
	// the decisions. LaunchPlan.LoginKind tells them from a login that
	// happens to be named "home".
	loginHomeText   = "home"
	loginOneOffText = "token-ref"
	notResolvedDry  = "not resolved: --dry-run"
	resolvedText    = "resolved"
)

// Prepare resolves every decision of a launch and builds the command
// (spec 6, 7, 11). With o.DryRun it reads only: no secret resolves (unless
// o.ResolvedSecrets), no before_run runs, no file is written, and the run
// folder shown is the one a real launch would create. Without it, the run
// folder is created and filled, ready for Start.
func (a *App) Prepare(ctx context.Context, o LaunchOptions) (*LaunchPlan, error) {
	cfg, err := a.Config()
	if err != nil {
		return nil, err
	}
	if !cfg.Exists || len(cfg.Config.Profiles) == 0 {
		return nil, apperr.Failure(apperr.CodeNoProfiles, "no profiles configured").
			WithHint("agx config init\nagx profile import --discovered --yes")
	}
	if o.Mode == ModeResume {
		if err := a.resumeTarget(ctx, &o); err != nil {
			return nil, err
		}
	}
	folderSrc := folderSource(o)
	if err := a.launchFolder(&o); err != nil {
		return nil, err
	}
	reg, _ := a.Registry()
	rs := &resolve.Resolver{Cfg: cfg, Agents: reg, Lookup: a.d.Lookup, UserHome: a.d.Host.UserHome, Choose: a.choosers(ctx), FoldOS: a.foldOS}
	res, err := rs.Resolve(o.Inputs)
	if err != nil {
		return nil, err
	}
	if res.RememberAgent {
		if err := a.SetConfigKey(ctx, "default_agent", string(res.Agent.ID())); err != nil {
			return nil, err
		}
		res.Notes = append(res.Notes, "default_agent set to "+string(res.Agent.ID())+" in "+a.paths.File)
	}
	p := &LaunchPlan{
		Mode: o.Mode, Agent: res.Agent.ID(), Profile: res.ProfileName,
		Home: res.Home.Dir(), Folder: o.Dir, ResumeID: o.ResumeID, Decisions: res.Decisions, Notes: res.Notes,
		ParentStays: run.ParentStays,
	}
	if err := a.binary(res.Agent, p); err != nil {
		return nil, err
	}
	p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecFolder, Value: a.tilde(o.Dir), Source: folderSrc})

	id, err := run.NewID(a.Now())
	if err != nil {
		return nil, err
	}
	p.RunID, p.RunDir = id, run.Dir(a.paths.Runs, id)

	req, t, err := a.request(ctx, o, res, p)
	if err != nil {
		return nil, err
	}
	if err := a.beforeRunPlan(o, res, p); err != nil {
		return nil, err
	}
	if err := a.launchConfig(ctx, o, res, &t, &req, p); err != nil {
		return nil, err
	}
	p.Record = run.Record{
		ID: p.RunID, Agent: string(p.Agent), Binary: p.BinaryPath, Profile: p.Profile,
		Home: p.Home, Login: p.Login, Folder: p.Folder, LiveLogin: p.Live != nil,
	}
	if o.DryRun {
		return p, a.build(res.Agent, t, req, p, true)
	}
	if o.Mode == ModeNew {
		if err := a.createSessionFolder(o.Dir); err != nil {
			return nil, err
		}
	}
	if _, err := run.Create(a.paths.Runs, p.RunID); err != nil {
		return nil, err
	}
	if err := a.build(res.Agent, t, req, p, false); err != nil {
		return nil, err
	}
	return p, nil
}

// launchFolder fills o.Dir for run (the flag or the current folder, which
// must exist) and new (the session folder to create); resume set it from
// the conversation already.
func (a *App) launchFolder(o *LaunchOptions) error {
	switch o.Mode {
	case ModeNew:
		dir, err := a.newSessionDir(o.SessionName)
		if err != nil {
			return err
		}
		o.Dir = dir
		return nil
	case ModeResume:
		return nil
	}
	if o.Dir == "" {
		o.Dir = a.d.Cwd
	} else {
		o.Dir = config.ExpandPath(o.Dir, a.d.Lookup, a.d.Host.UserHome)
	}
	if !statDir(o.Dir) {
		return apperr.Failure(apperr.CodeFolderGone, "folder "+o.Dir+" does not exist")
	}
	return nil
}

// folderSource names where the launch folder comes from (6.1 step 6);
// called before launchFolder fills o.Dir, so an empty Dir means the
// current folder.
func folderSource(o LaunchOptions) string {
	switch o.Mode {
	case ModeNew:
		return "new session folder"
	case ModeResume:
		return "conversation " + o.ResumeID
	}
	if o.Dir != "" {
		return "--dir"
	}
	return "current folder"
}

// binary resolves the agent binary (6.1 step 5) and its version from
// install metadata, never by running it.
func (a *App) binary(ag agentio.Agent, p *LaunchPlan) error {
	path, err := ag.LookPath()
	if err != nil {
		msg := fmt.Sprintf("%s not found on PATH", ag.Spec().Binary)
		e := apperr.Wrap(err, apperr.CodeBinaryMissing, apperr.ClassNotFoundBinary, msg)
		if pk := ag.Spec().Packages; len(pk) > 0 {
			e.Message += "; install: npm i -g " + pk[0]
		}
		return e
	}
	p.BinaryPath = path
	version, src := "version unknown", "no install metadata"
	if in, ok := agentio.InstallerOf(ag); ok {
		if inst, err := in.Install(); err == nil && !inst.Version.IsZero() {
			p.Version = inst.Version.String()
			version, src = inst.Version.String(), "install metadata"
		}
	}
	p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecBinary, Value: path + " " + version, Source: src})
	return nil
}

// request builds the neutral launch request and target: layer 1 options,
// the credential, raw args, the environment.
func (a *App) request(ctx context.Context, o LaunchOptions, res resolve.Result, p *LaunchPlan) (agentio.LaunchRequest, agentio.Target, error) {
	prof := res.Profile
	req := agentio.LaunchRequest{Dir: o.Dir, Prompt: o.Prompt, ResumeID: o.ResumeID, AddDirs: o.AddDirs, Headless: o.Headless}
	t := res.Home.Target()
	// The installed version gates every typed flag (edge 10) and drives
	// the untested/unknown-version warnings (edges 8, 9).
	if v, ok := agentio.ParseVersion(p.Version); ok {
		t.Version = v
	}
	if prof.Billing == config.BillingAPI {
		t.Billing = agentio.BillingPerToken
	}

	req.Model = o.Model
	switch {
	case o.Model != "":
		p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecOptions, Value: "model=" + o.Model, Source: "--model"})
	case prof.Model != "":
		req.Model = prof.Model
		p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecOptions, Value: "model=" + prof.Model, Source: "profile " + res.ProfileName})
	}

	autonomy, autoSrc := o.Autonomy, "--autonomy"
	if autonomy == "" && prof.Autonomy != "" {
		autonomy, autoSrc = prof.Autonomy, "profile "+res.ProfileName
	}
	if autonomy != "" {
		lvl, ok := agentio.ParseAutonomy(autonomy)
		if !ok {
			return req, t, apperr.Usage(fmt.Sprintf("--autonomy %q: use one of default, read-only, ask, accept-edits, full", autonomy))
		}
		req.Autonomy = lvl
		p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecOptions, Value: "autonomy=" + autonomy, Source: autoSrc})
	}
	switch o.Consent {
	case "":
	case ConsentFullAutonomy:
		t.AllowDangerous = true
	default:
		return req, t, apperr.Usage(fmt.Sprintf("--consent %q: the only consent is %s", o.Consent, ConsentFullAutonomy))
	}
	if req.Autonomy == agentio.AutonomyFull && !t.AllowDangerous {
		return req, t, apperr.Usage("--autonomy full needs --consent full-autonomy")
	}
	if n, ok := fullByDefault(res.Agent); ok && req.Autonomy == agentio.AutonomyDefault {
		p.Warnings = append(p.Warnings, report.Warning{Code: WarnNotice, Message: n})
	}

	groups, raws, err := takeRawSettings(rawArgs(a.cfgOrEmpty(), res, o.Raw, launchOptionKeys(res.Agent), a.d.Host.UserHome), res.Agent)
	if err != nil {
		return req, t, err
	}
	p.rawSettings = raws
	args, argSrc := joinArgGroups(groups)
	req.Args = args
	if len(args) > 0 {
		p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecArgs, Value: strings.Join(args, " "), Source: argSrc})
	}

	cred, err := a.credential(ctx, o, res, p)
	if err != nil {
		return req, t, err
	}
	req.Credential = cred

	env, err := a.launchEnv(ctx, o, res, p)
	if err != nil {
		return req, t, err
	}
	t.ExtraEnv = append(env, runEnv(p)...)
	return req, t, nil
}

// fullByDefault reports an agent whose plain launch already runs at full
// autonomy (its spec maps "full" to no args, as pi's does): the notice of
// edge 31.
func fullByDefault(ag agentio.Agent) (string, bool) {
	args, ok := ag.Spec().AutonomyArgs[agentio.AutonomyFull]
	if !ok || len(args) > 0 {
		return "", false
	}
	return string(ag.ID()) + " runs with full autonomy and never asks", true
}

// rawArgs are the layer-3 args in precedence order (7.8), one group per
// source: agent default args (unless the profile sets inherit_defaults
// false), the profile's args, then everything after "--". Raw args are
// appended last, so they win over typed flags (rule "raw always wins");
// a raw settings flag is the exception, merged instead (takeRawSettings).
// In the args from the config file, "~" is expanded after a path
// option's flag (keys, home): no shell ever saw those; the args after
// "--" stay exactly as given.
func rawArgs(c config.Config, res resolve.Result, after []string, keys []agentio.OptionKey, home string) []argGroup {
	var groups []argGroup
	if res.Profile.InheritsDefaults() {
		if ac, ok := c.Agents[res.Agent.ID()]; ok && len(ac.Defaults.Args) > 0 {
			groups = append(groups, argGroup{src: "agents." + string(res.Agent.ID()) + ".defaults.args", args: expandPathArgs(ac.Defaults.Args, keys, home)})
		}
	}
	if len(res.Profile.Args) > 0 {
		groups = append(groups, argGroup{src: "profile " + res.ProfileName, args: expandPathArgs(res.Profile.Args, keys, home)})
	}
	if len(after) > 0 {
		groups = append(groups, argGroup{src: "after --", args: after})
	}
	return groups
}

// cfgOrEmpty returns the loaded config (Prepare checked it loads).
func (a *App) cfgOrEmpty() config.Config {
	if c, err := a.Config(); err == nil {
		return c.Config
	}
	return config.Config{}
}

// credential resolves the login to a token credential: nil for the home
// login. With --dry-run nothing resolves unless --resolved-secrets, and
// a placeholder token stands in so the command shows the variable.
func (a *App) credential(ctx context.Context, o LaunchOptions, res resolve.Result, p *LaunchPlan) (*agentio.TokenCredential, error) {
	l := res.Login
	switch l.Kind {
	case resolve.LoginHome:
		p.Login, p.LoginKind = loginHomeText, LoginKindHome
		return nil, nil
	case resolve.LoginTokenRef:
		p.Login, p.LoginKind = loginOneOffText, LoginKindTokenRef
	default:
		p.Login, p.LoginKind = l.Name, LoginKindNamed
	}
	if l.Kind == resolve.LoginNamed && l.Login.Kind == config.KindPool {
		// A pool has no token of its own: the launch is live (livePlan),
		// and agx credential picks a member each time the agent asks.
		return nil, nil
	}
	if _, ok := agentio.TokenLoginOf(res.Agent); !ok {
		reason, _ := agentio.Unsupported(res.Agent, agentio.CapTokenLogin)
		if reason == "" {
			reason = "no token login"
		}
		return nil, apperr.Unsupported(fmt.Sprintf("%s has no token login (%s); log in inside the home instead", res.Agent.ID(), reason))
	}
	resolveNow := !o.DryRun || o.ResolvedSecrets
	var (
		cred agentio.TokenCredential
		err  error
	)
	switch {
	case !resolveNow:
		kind := l.Login.Kind
		if l.Kind == resolve.LoginTokenRef {
			kind = l.RefKind
		}
		k, ok := login.KindOf(kind)
		if !ok {
			return nil, apperr.Usage(fmt.Sprintf("login kind %q: use one of %s", kind, strings.Join(config.KindValues(), ", ")))
		}
		cred = agentio.TokenCredential{Kind: k, Token: agentio.NewToken(notResolvedDry), Provider: l.Login.Provider}
		a.annotateLogin(p, notResolvedDry)
	case l.Kind == resolve.LoginTokenRef:
		cred, err = login.OneOff(ctx, a.exp, l.Ref, l.RefKind, "")
	default:
		cred, err = login.Credential(ctx, a.exp, a.paths, l.Name, l.Login)
	}
	if err != nil {
		if o.DryRun {
			a.annotateLogin(p, "error: "+apperr.As(err).Message)
			return nil, err
		}
		return nil, err
	}
	if resolveNow && l.Kind == resolve.LoginNamed {
		d := login.DetectToken(cred.Token)
		if err := login.CheckPerUse(l.Name, l.Login, d, o.AllowAPIKey, true); err != nil {
			if o.DryRun {
				a.annotateLogin(p, "error: "+apperr.As(err).Message)
			}
			return nil, err
		}
		if o.DryRun {
			a.annotateLogin(p, resolvedText+", detected "+string(d))
		}
		return &cred, nil
	}
	if resolveNow && o.DryRun {
		a.annotateLogin(p, resolvedText)
	}
	return &cred, nil
}

// annotateLogin appends a resolution note to the login decision.
func (a *App) annotateLogin(p *LaunchPlan, note string) {
	for i := range p.Decisions {
		if p.Decisions[i].Name == resolve.DecLogin {
			p.Decisions[i].Value += " (" + note + ")"
		}
	}
}

// runEnv are the AGX_RUN_* outputs every launch sets (spec 11).
func runEnv(p *LaunchPlan) []agentio.EnvChange {
	return []agentio.EnvChange{
		agentio.EnvSet(run.EnvRunID, p.RunID),
		agentio.EnvSet(run.EnvRunAgent, string(p.Agent)),
		agentio.EnvSet(run.EnvRunProfile, p.Profile),
		agentio.EnvSet(run.EnvRunLogin, p.Login),
		agentio.EnvSet(run.EnvRunHome, p.Home),
	}
}

// build runs the agent's launch builder. For a dry run the run folder is
// simulated in a temporary folder (deleted again) and its path replaced
// by the real one in the shown command, so --dry-run shows exactly what a
// launch would run without writing under the config dir.
func (a *App) build(ag agentio.Agent, t agentio.Target, req agentio.LaunchRequest, p *LaunchPlan, dry bool) error {
	runDir := p.RunDir
	if dry {
		dir, cleanup, err := run.DryRunDir(p.RunID)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "temp folder: "+err.Error())
		}
		defer cleanup()
		runDir = dir
	}
	cmd, err := a.launchBuild(ag, t, req, p, runDir)
	if err != nil {
		return launchError(ag, err)
	}
	if dry {
		for i, arg := range cmd.Args {
			cmd.Args[i] = strings.ReplaceAll(arg, runDir, p.RunDir)
		}
	}
	p.Command = cmd
	a.helperDecision(req, p, runDir)
	for _, w := range cmd.Warnings {
		p.Warnings = append(p.Warnings, report.Warning{Code: WarnVersion, Message: w.Detail})
	}
	if p.finish != nil {
		if err := p.finish(p); err != nil {
			return err
		}
	}
	a.commandDecisions(p)
	return nil
}

// launchError maps agentio's launch errors to agx's codes.
func launchError(ag agentio.Agent, err error) error {
	var ae *apperr.Error
	switch {
	case errors.As(err, &ae):
		return ae
	case errors.Is(err, agentio.ErrFeatureUnavailable):
		return apperr.Wrap(err, apperr.CodeFeatureUnavailable, apperr.ClassUsage, err.Error())
	case errors.Is(err, agentio.ErrDangerousNotAllowed):
		return apperr.Usage("--autonomy full needs --consent full-autonomy")
	case errors.Is(err, agentio.ErrUnsupported):
		return apperr.Wrap(err, apperr.CodeUnsupported, apperr.ClassUnsupported, unsupportedText(err))
	case errors.Is(err, agentio.ErrInvalidOption):
		// The agent refused the options it was given (a contradiction such
		// as --resume with --continue): the user's input, not a failure.
		return apperr.Wrap(err, apperr.CodeUsage, apperr.ClassUsage, agentPrefixed(ag.ID(), err.Error()))
	}
	return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, agentPrefixed(ag.ID(), err.Error()))
}

// agentPrefixed names the agent in front of msg once: an agent package
// often words its own errors "<agent>: …" already (Claude's "claude:
// invalid launch options"), and prefixing again printed "claude:
// claude: …".
func agentPrefixed(id agentio.ID, msg string) string {
	prefix := id.String() + ": "
	if strings.HasPrefix(msg, prefix) {
		return msg
	}
	return prefix + msg
}

// unsupportedText strips agentio's "agentio: unsupported: " prefix (and
// anything wrapped before it) and keeps the rest: agentio words these
// errors "<agent>: <what was asked>: <reason>", and all three are needed.
// Keeping only the reason (the text after the last ": ") turned a spec
// agent's refusal into "not described by the agent spec", naming neither
// the agent nor the level, and cut any reason that itself holds ": ".
func unsupportedText(err error) string {
	s := err.Error()
	prefix := agentio.ErrUnsupported.Error() + ": "
	if i := strings.Index(s, prefix); i >= 0 {
		return apperr.UserText(s[i+len(prefix):])
	}
	return apperr.UserText(s)
}

// commandDecisions adds the run, command and env lines (6.6), every
// printed value passed through the launch's secretRedactor first.
func (a *App) commandDecisions(p *LaunchPlan) {
	r := newSecretRedactor(p.Command, a.agentCredentialVars())
	r.decisions(p.Decisions)
	for i := range p.Warnings {
		p.Warnings[i].Message = r.text(p.Warnings[i].Message)
	}
	red := r.command(p.Command)
	p.View = CommandView{Line: red.String(), Path: red.Path, Args: red.Args, Dir: red.Dir, Env: envDiff(a.d.Host.Env, p.Command.Env, red.Env)}
	if p.View.Args == nil {
		p.View.Args = []string{}
	}
	p.Decisions = append(p.Decisions,
		report.Decision{Name: resolve.DecRun, Value: a.tilde(p.RunDir), Source: "AGX_RUN_ID=" + p.RunID},
		report.Decision{Name: resolve.DecCommand, Value: red.String(), Source: "built by agentio"},
	)
	for _, ch := range p.View.Env {
		p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecEnv, Value: ch, Source: "launch environment"})
	}
}

// envDiff lists what the launch changed against base: "set NAME=value"
// and "removed NAME". It compares the real values (raw) and prints the
// redacted ones (red), so an inherited secret that did not change is not
// listed, and a changed one shows as [redacted].
func envDiff(base, raw, red []string) []string {
	have := map[string]string{}
	for _, kv := range base {
		k, v, _ := strings.Cut(kv, "=")
		have[k] = v
	}
	shown := map[string]string{}
	for _, kv := range red {
		k, v, _ := strings.Cut(kv, "=")
		shown[k] = v
	}
	seen := map[string]bool{}
	var res []string
	for _, kv := range raw {
		k, v, _ := strings.Cut(kv, "=")
		seen[k] = true
		if old, ok := have[k]; !ok || old != v {
			res = append(res, "set "+k+"="+shown[k])
		}
	}
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !seen[k] {
			res = append(res, "removed "+k)
		}
	}
	return res
}

// tilde renders a path under the user home as ~/….
func (a *App) tilde(p string) string { return config.TildePath(p, a.d.Host.UserHome) }

// Commit writes run.jsonc (with this process's pid and start time) and
// runs before_run; the agent is started next by the CLI layer
// (internal/run.Start). Separate from Prepare so --plan-file can resolve
// now and launch later.
func (a *App) Commit(ctx context.Context, p *LaunchPlan) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	p.Record.PID = os.Getpid()
	p.Record.StartedAt = a.ownStart(ctx)
	if err := run.WriteRecord(p.RunDir, p.Record); err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "write run.jsonc: "+err.Error())
	}
	if p.Live != nil {
		if err := a.writeLiveStart(ctx, p); err != nil {
			return err
		}
	}
	if p.BeforeRun != nil {
		if err := a.runBeforeRun(ctx, p); err != nil {
			return err
		}
	}
	return ctxErr(ctx)
}

// ownStart is this process's start time from the process table (the clock
// agx clean compares, agentio.Host.ProcessAlive), else the host clock.
func (a *App) ownStart(ctx context.Context) time.Time {
	if a.d.Host.Processes != nil {
		if procs, err := a.d.Host.Processes.Processes(ctx); err == nil {
			pid := os.Getpid()
			for _, pr := range procs {
				if pr.PID == pid && !pr.Start.IsZero() {
					return pr.Start
				}
			}
		}
	}
	return a.Now()
}
