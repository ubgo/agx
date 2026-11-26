package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
)

// LiveRun is one live-login run as agx live list / show / use print it.
// Login fields are names, never values. Every field is a stable part of
// the JSON (agx live list --json), for any tool to read.
type LiveRun struct {
	ID      string `json:"id"`
	Dir     string `json:"dir"`
	Agent   string `json:"agent"`
	Profile string `json:"profile"`
	Home    string `json:"home"`
	Folder  string `json:"folder"`
	// FolderShort is Folder for a table: "~/…" and at most its last
	// folderShortMax characters.
	FolderShort string `json:"folder_short"`
	// Label is the run's --label (or AGX_RUN_LABEL at launch); "" = none.
	Label string `json:"label"`
	// PID is the agent's process id (omitted when unknown); TTY its
	// terminal at launch; Parent the name of the process that started it
	// ("" = unknown).
	PID    int    `json:"pid,omitempty"`
	TTY    string `json:"tty"`
	Parent string `json:"parent"`
	// Env are the live.record_env values recorded at launch.
	Env map[string]string `json:"env,omitempty"`
	// ConversationID and ConversationTitle are the agent's conversation
	// running in this process, when the agent lists its live sessions
	// (agentio); "" = unknown.
	ConversationID    string `json:"conversation_id"`
	ConversationTitle string `json:"conversation_title"`
	// StartLogin, ChosenLogin, InUse: live.State.
	StartLogin  string    `json:"start_login"`
	ChosenLogin string    `json:"chosen_login"`
	InUse       string    `json:"in_use"`
	InUseAt     time.Time `json:"in_use_at,omitzero"`
	// InUseSince is when InUse last changed (a member switch).
	InUseSince time.Time `json:"in_use_since,omitzero"`
	// InUseText is InUse as people read it (InUseText): the login, or
	// "none (all over caps) since 10:52" after a refusal.
	InUseText string `json:"in_use_text"`
	// KeyAt is when a real key was last handed out (zero = never).
	KeyAt time.Time `json:"key_at,omitzero"`
	// Next is the login the agent gets at its next refresh.
	Next string `json:"next"`
	// Only is the member a pool run is pinned to with its caps enforced
	// (--pool-only); "" = none.
	Only string `json:"only,omitempty"`
	// NoKey says why the last key request got no key, NoKeyBrief the
	// same in a few words, NoKeySince since when; "" / zero once a key
	// was handed out again. WhenNoneUsable is how that refusal was
	// handled (stop or keep).
	NoKey          string    `json:"no_key,omitempty"`
	NoKeyBrief     string    `json:"no_key_brief,omitempty"`
	NoKeySince     time.Time `json:"no_key_since,omitzero"`
	WhenNoneUsable string    `json:"when_none_usable,omitempty"`
	// Liveness is alive, ended or unknown (agx clean's check); Reason
	// says how it was decided.
	Liveness string `json:"liveness"`
	Reason   string `json:"reason"`
}

// folderShortMax bounds LiveRun.FolderShort, so the table stays readable.
const folderShortMax = 32

// LiveUseOptions are agx live use's flags.
type LiveUseOptions struct {
	// ID is the run id (--id).
	ID string
	// Login is the login to switch to (--login).
	Login string
	// StartLogin goes back to the login the run started with.
	StartLogin bool
	// PoolOnly pins a pool run to one member with its caps enforced
	// (--pool-only, --pool-only-short, --pool-only-week).
	PoolOnly PoolOnly
}

// Credential is what agx credential hands the agent: the token, printed
// alone on stdout by the CLI and nowhere else. Token never encodes
// (agentio.Token redacts in JSON and every fmt verb).
type Credential struct {
	RunID string `json:"run_id"`
	Login string `json:"login"`
	// Pool is the pool login the member was picked from; "" otherwise.
	Pool  string        `json:"pool,omitempty"`
	Token agentio.Token `json:"-"`
	// Refused is why no key was handed out, in one line, when the run's
	// when_none_usable is "stop": the CLI prints NoKeyPlaceholder on
	// stdout instead of a key and the reason on stderr, and exits 0. ""
	// = a key was handed out.
	Refused string `json:"refused,omitempty"`
	// RefusedBrief is Refused in a few words ("all over caps").
	RefusedBrief string `json:"refused_brief,omitempty"`
	// Warnings go to stderr (never stdout, which the agent reads).
	Warnings []report.Warning `json:"-"`
}

// NoKeyPlaceholder is what agx credential prints instead of a key when it
// has none to hand out and the run's when_none_usable is "stop". Why not
// exit 1: an agent that already holds a key keeps using it when its key
// command fails (Claude Code does, on every request), so a pool's caps
// would not hold once every member is over them. A value that is not a
// key makes the agent's next request fail authentication instead; the
// agent keeps asking with growing pauses and goes on by itself once a
// real key comes back. It is a fixed, public word with no vendor's token
// prefix, so nothing can mistake it for a token, and it is never stored.
const NoKeyPlaceholder = "agx-no-account-usable"

// Brief refusal reasons (live.jsonc no_key_brief; agx live list, agx
// statusline).
const (
	briefAllOverCaps   = "all over caps"
	briefNoneUsable    = "no member usable"
	briefOverCap       = "over cap"
	briefNotUsable     = "not usable"
	briefAPIKey        = "api key refused"
	briefNotResolved   = "key not resolved"
	briefRequestFailed = "key request failed"
)

// keyRefusal is a failed key request of a known run: where to record it,
// how to answer it (when_none_usable) and its words.
type keyRefusal struct {
	runID, dir, pool string
	// mode is the run's when_none_usable.
	mode string
	// why replaces the error's message in live.jsonc ("" = the message).
	why   string
	brief string
}

// LiveList returns the running live-login runs, oldest first; agent ""
// = every agent. Runs whose agent has ended are left out (agx clean
// removes them); unknown liveness is listed with its reason.
func (a *App) LiveList(ctx context.Context, agent agentio.ID) ([]LiveRun, error) {
	folders, err := run.List(a.paths.Runs)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "read "+a.paths.Runs+": "+err.Error())
	}
	type liveFolder struct {
		f  run.Folder
		st live.State
	}
	var lf []liveFolder
	var fs []run.Folder
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
		lf = append(lf, liveFolder{f, st})
		fs = append(fs, f)
	}
	lives := a.runsLiveness(ctx, fs)
	out := []LiveRun{}
	convs := a.newConvLookup()
	for _, x := range lf {
		l := lives[x.f.ID]
		if l.liveness == agentio.LivenessEnded {
			continue
		}
		out = append(out, a.liveRunOf(x.f, x.st, l, convs))
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
//
// --pool-only MEMBER pins a pool run to one member with its caps
// enforced: the pool is --login POOL when given, else the run's current
// pool (its chosen login, else its start login). Any other choice
// (--login, --start-login without --pool-only) clears the pin, so
// --login POOL hands the run back to the pool's rules.
func (a *App) LiveUse(ctx context.Context, o LiveUseOptions) (LiveRun, error) {
	switch {
	case o.Login != "" && o.StartLogin:
		return LiveRun{}, apperr.Conflict("--login and --start-login both choose the login; pick one")
	case o.Login == "" && !o.StartLogin && !o.PoolOnly.given():
		return LiveRun{}, apperr.Usage("choose the login: --login NAME, --start-login or " + flagPoolOnly + " MEMBER")
	}
	f, st, err := a.liveFolder(o.ID)
	if err != nil {
		return LiveRun{}, err
	}
	if l, _ := a.runLiveness(ctx, f); l == agentio.LivenessEnded {
		return LiveRun{}, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("run %s has ended; its login can no longer change", f.ID)).WithHint("agx clean --yes")
	}
	chosen := o.Login
	if o.Login != "" || o.PoolOnly.given() {
		cfg, err := a.Config()
		if err != nil {
			return LiveRun{}, err
		}
		if o.Login != "" {
			if _, err := liveLogin(cfg.Config, o.Login, st.Agent); err != nil {
				return LiveRun{}, err
			}
		}
		if o.PoolOnly.given() {
			if chosen, err = livePoolOnly(cfg.Config, st, o); err != nil {
				return LiveRun{}, err
			}
		}
	}
	st, err = live.Update(ctx, f.Dir, func(s *live.State) error {
		s.ChosenLogin = chosen
		s.ChosenAt = a.Now()
		// A choice of the user's ends the run's --pool-force and its pin:
		// pinning a member is the new choice, and --login POOL or
		// --start-login hands the run back to the pool's own rules.
		s.PoolForce = ""
		s.PoolOnly, s.PoolOnlyShort, s.PoolOnlyWeek, s.NoKey = o.PoolOnly.Member, o.PoolOnly.Caps.Short, o.PoolOnly.Caps.Week, ""
		return nil
	})
	if err != nil {
		return LiveRun{}, liveErr(err)
	}
	return a.liveRun(ctx, f, st), nil
}

// livePoolOnly checks agx live use --pool-only and returns the chosen
// login to keep: the pool is --login when given, the start login with
// --start-login, else the run's current login (chosen, else start); it
// must be a pool of the run's agent with MEMBER in it.
func livePoolOnly(c config.Config, st live.State, o LiveUseOptions) (string, error) {
	name, chosen := st.Want(), st.ChosenLogin
	switch {
	case o.Login != "":
		name, chosen = o.Login, o.Login
	case o.StartLogin:
		name, chosen = st.StartLogin, ""
	}
	lg, err := liveLogin(c, name, st.Agent)
	if err != nil {
		return "", err
	}
	if lg.Kind != config.KindPool {
		return "", errNotPoolOnly("login " + name + " is " + lg.Kind)
	}
	rules, _ := lg.Pool.Rules() // the config loaded, so it parses
	if err := checkPoolOnly(name, lg.Pool, rules, o.PoolOnly, "", true); err != nil {
		return "", err
	}
	return chosen, nil
}

// Credential is the hidden agx credential (spec 10): it finds its run by
// AGX_RUN_ID, resolves the chosen login (else the start login) and
// records it as the login in use. Only the CLI prints the token, alone.
//
// When it cannot hand out a key for a known run (no pool member usable,
// a pinned member over its cap, a refused API key, a login that does not
// resolve), the run's when_none_usable decides (whenNoneUsable): "stop"
// (the default) returns no error and Refused set, so the CLI prints
// NoKeyPlaceholder and the agent stops spending; "keep" returns the
// error (exit 1) and the agent goes on with the key it has. Either way
// live.jsonc records the refusal, so agx live list, whoami and agx
// statusline say "none (why) since HH:MM". A failure before the run and
// its config are known (no AGX_RUN_ID, no such run) is always an error.
//
// Every error and reason is one line (oneLine): Claude Code shows the
// helper's stderr in /status (three tries, then exit 1, with no fallback
// to the home's own login; LIVEFIX V1). A login that does not resolve is
// reported briefly (login.Expander.Brief: the reference, never a store's
// own stderr) with agx login check, which shows the full reason, as the
// next step.
func (a *App) Credential(ctx context.Context) (Credential, error) {
	c, rf, err := a.liveCredential(ctx)
	if err == nil {
		return c, nil
	}
	why := messageBreaks.Replace(apperr.As(err).Message)
	err = oneLine(err)
	if rf == nil {
		return Credential{Warnings: c.Warnings}, err
	}
	if rf.why != "" {
		why = rf.why
	}
	a.recordNoKey(ctx, rf.dir, why, rf.brief, rf.mode)
	if rf.mode == config.WhenNoneUsableKeep {
		return Credential{Warnings: c.Warnings}, err
	}
	return Credential{RunID: rf.runID, Pool: rf.pool, Refused: apperr.As(err).Message, RefusedBrief: rf.brief, Warnings: c.Warnings}, nil
}

// liveCredential is Credential before a failure is answered: the
// refusal is nil until the run is known.
func (a *App) liveCredential(ctx context.Context) (Credential, *keyRefusal, error) {
	runID, ok := a.d.Lookup(run.EnvRunID)
	if !ok || runID == "" {
		return Credential{}, nil, apperr.Usage("agx credential runs inside a live-login session: " + run.EnvRunID + " is not set")
	}
	f, st, err := a.liveFolder(runID)
	if err != nil {
		return Credential{}, nil, err
	}
	name := st.Want()
	// Until the config is read its setting is unknown: the default holds.
	rf := &keyRefusal{runID: f.ID, dir: f.Dir, mode: config.WhenNoneUsableStop, brief: briefRequestFailed}
	cfg, err := a.Config()
	if err != nil {
		return Credential{}, rf, err
	}
	rf.mode, _ = whenNoneUsable(cfg.Config, name)
	lg, err := liveLogin(cfg.Config, name, st.Agent)
	if err != nil {
		return Credential{}, rf, err
	}
	inUse, pool := name, ""
	var (
		tok      agentio.Token
		warnings []report.Warning
		detected login.Detected
	)
	if lg.Kind == config.KindPool {
		// A pool run: pick a member now, by usage, each time the agent
		// asks (spec: login pools). The pick is what is in use.
		pool, rf.pool = name, name
		var rep PoolReport
		only := stateOnly(st)
		inUse, tok, rep, err = a.poolMemberToken(ctx, name, lg, poolModeCredential, st.PoolForce, only, f.ID)
		warnings = rep.StderrWarnings()
		if err != nil {
			rf.brief = poolBrief(err, rep)
			if only.Member != "" && rep.Selected == nil && len(rep.Candidates) == 1 {
				// The pin is blocked: say which cap, in the pin's words.
				reasons := rep.Candidates[0].Reasons
				rf.why, rf.brief = onlyNoKey(only.Member, reasons), only.Member+" "+onlyWhat(reasons)
			}
			if inUse != "" {
				return Credential{Warnings: warnings}, rf, withCheckHint(err, inUse)
			}
			return Credential{Warnings: warnings}, rf, err
		}
		detected = login.Detected(rep.Detected)
	} else {
		cred, err := login.Credential(ctx, a.exp.Brief(), a.paths, name, lg)
		if err != nil {
			rf.brief = codeBrief(err)
			return Credential{}, rf, withCheckHint(err, name)
		}
		detected = login.DetectToken(cred.Token)
		if err := login.CheckPerUse(name, lg, detected, st.AllowAPIKey, true); err != nil {
			rf.brief = codeBrief(err)
			return Credential{}, rf, err
		}
		tok = cred.Token
	}
	if _, err := live.Update(ctx, f.Dir, func(s *live.State) error {
		s.SetInUse(inUse, a.Now())
		s.InUseDetected = string(detected)
		return nil
	}); err != nil {
		return Credential{Warnings: warnings}, rf, liveErr(err)
	}
	return Credential{RunID: f.ID, Login: inUse, Pool: pool, Token: tok, Warnings: warnings}, nil, nil
}

// liveKeyWhenNoneUsable is the source of live.when_none_usable.
const liveKeyWhenNoneUsable = "config live.when_none_usable"

// whenNoneUsable is what agx credential does for a run on login name
// when it has no key to hand out, and the setting it comes from: the
// pool's when_none_usable, else live.when_none_usable, else "stop"
// (SourceDefault).
func whenNoneUsable(c config.Config, name string) (string, string) {
	if lg, ok := c.Logins[name]; ok && lg.Pool != nil && lg.Pool.WhenNoneUsable != "" {
		return lg.Pool.WhenNoneUsable, "config logins." + name + ".pool.when_none_usable"
	}
	if c.Live.WhenNoneUsable != "" {
		return c.Live.WhenNoneUsable, liveKeyWhenNoneUsable
	}
	return config.WhenNoneUsableStop, SourceDefault
}

// whenNoneUsableText says what a mode does, for a launch's dry run.
func whenNoneUsableText(mode string) string {
	if mode == config.WhenNoneUsableKeep {
		return "keep (the agent goes on with its old key)"
	}
	return "stop (the agent waits for a usable key)"
}

// poolBrief names a pool's failed key request in a few words: every
// member over a cap, no member usable for another reason (status, no
// reading), or the failure's own kind (codeBrief).
func poolBrief(err error, rep PoolReport) string {
	if apperr.As(err).Code != apperr.CodeAutoPick || rep.Selected != nil || len(rep.Candidates) == 0 {
		return codeBrief(err)
	}
	for _, c := range rep.Candidates {
		if !slices.ContainsFunc(c.Reasons, isCapReason) {
			return briefNoneUsable
		}
	}
	return briefAllOverCaps
}

// codeBrief names a failed key request by its error code.
func codeBrief(err error) string {
	switch apperr.As(err).Code {
	case apperr.CodePerUseKey:
		return briefAPIKey
	case apperr.CodeSecret:
		return briefNotResolved
	case apperr.CodeAutoPick:
		return briefNoneUsable
	}
	return briefRequestFailed
}

// recordNoKey keeps why a run's key request got no key (live.State
// SetNoKey). A write failure is dropped: the request is answered either
// way, and a second message would make the helper's stderr two lines.
func (a *App) recordNoKey(ctx context.Context, dir, why, brief, mode string) {
	_, _ = live.Update(ctx, dir, func(s *live.State) error {
		s.SetNoKey(why, brief, mode, a.Now())
		return nil
	})
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
	return a.liveRunOf(f, st, runLive{l, reason}, a.newConvLookup())
}

// liveRunOf is a run's live view with its liveness already decided.
func (a *App) liveRunOf(f run.Folder, st live.State, rl runLive, convs *convLookup) LiveRun {
	l, reason := rl.liveness, rl.reason
	r := LiveRun{
		ID: f.ID, Dir: f.Dir, Agent: st.Agent, Profile: f.Record.Profile, Home: f.Record.Home, Folder: f.Record.Folder,
		FolderShort: shortPath(a.tilde(f.Record.Folder), folderShortMax),
		Label:       f.Record.Label, PID: f.Record.PID, TTY: f.Record.TTY, Parent: f.Record.Parent, Env: f.Record.Env,
		StartLogin: st.StartLogin, ChosenLogin: st.ChosenLogin, InUse: st.InUse, InUseAt: st.InUseAt, InUseSince: st.InUseSince,
		InUseText: InUseText(st, a.Now()), KeyAt: st.KeyAt, Next: st.Want(), Only: st.PoolOnly,
		NoKey: st.NoKey, NoKeyBrief: st.NoKeyBrief, NoKeySince: st.NoKeySince, WhenNoneUsable: st.WhenNoneUsable,
		Liveness: l.String(), Reason: reason,
	}
	if l != agentio.LivenessEnded {
		r.ConversationID, r.ConversationTitle = convs.find(agentio.ID(st.Agent), f.Record.Home, f.Record.PID)
	}
	return r
}

// shortPath keeps the end of p within max runes: "…/projects/acme/api".
func shortPath(p string, maxRunes int) string {
	r := []rune(p)
	if len(r) <= maxRunes {
		return p
	}
	return "…" + string(r[len(r)-maxRunes+1:])
}

// convLookup finds the conversation a run's process is in, reading each
// home's live sessions once (agentio LiveSessionLister). Any failure is
// an unknown conversation: the list must not fail for a detail.
type convLookup struct {
	a    *App
	byID map[string][]agentio.LiveSession
}

// newConvLookup starts an empty lookup.
func (a *App) newConvLookup() *convLookup {
	return &convLookup{a: a, byID: map[string][]agentio.LiveSession{}}
}

// find returns the id and title of the conversation process pid of
// agent runs in home; "", "" when unknown.
func (c *convLookup) find(id agentio.ID, home string, pid int) (string, string) {
	if home == "" || pid == 0 {
		return "", ""
	}
	reg, err := c.a.Registry()
	if err != nil || reg == nil {
		return "", ""
	}
	ag, ok := reg.Get(id)
	if !ok {
		return "", ""
	}
	ls, ok := agentio.LiveSessionsOf(ag)
	if !ok {
		return "", ""
	}
	h := resolve.HomeOf(ag, home, "")
	key := string(id) + "\x00" + home
	sessions, seen := c.byID[key]
	if !seen {
		sessions, _ = ls.LiveSessions(h)
		c.byID[key] = sessions
	}
	for _, s := range sessions {
		if s.PID != pid || s.ConversationID == "" {
			continue
		}
		title := ""
		if cl, ok := agentio.ConversationsOf(ag); ok {
			if conv, err := cl.Conversation(h, s.ConversationID); err == nil {
				title = conv.Title
			}
		}
		return s.ConversationID, title
	}
	return "", ""
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
