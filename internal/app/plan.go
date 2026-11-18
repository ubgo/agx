package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/plan"
	"github.com/ubgo/agx/internal/run"
)

// Plan modes: which launching command wrote a plan (plan show "mode").
const (
	PlanModeRun    = "run"
	PlanModeNew    = "new"
	PlanModeResume = "resume"
)

// planModes maps a launch mode to its plan mode word.
var planModes = map[LaunchMode]string{ModeRun: PlanModeRun, ModeNew: PlanModeNew, ModeResume: PlanModeResume}

// PlanSummary is what a plan resolved, shown by agx plan show and copied
// into the run folder as plan.jsonc by agx launch. It never holds a secret:
// Command is the redacted view, built without resolving any reference.
type PlanSummary struct {
	// Mode is PlanModeRun, PlanModeNew or PlanModeResume.
	Mode    string     `json:"mode"`
	Agent   agentio.ID `json:"agent"`
	Profile string     `json:"profile"`
	Home    string     `json:"home"`
	// Login is the login name, "home" or "token-ref" (LaunchPlan.Login).
	Login  string `json:"login"`
	Binary string `json:"binary"`
	// Folder is the launch folder (the folder the wrapper cds into).
	Folder string `json:"folder"`
	// ResumeID is the conversation a resume continues ("" otherwise).
	ResumeID string `json:"resume_id,omitempty"`
	// Command is the planned command as --dry-run shows it (the token is
	// a placeholder: secrets resolve only in agx launch).
	Command CommandView `json:"command"`
}

// PlanView is agx plan show's data: the file header plus the summary.
// "dir" is what the shell wrapper reads (--field data.dir).
type PlanView struct {
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	Dir           string    `json:"dir"`
	PlanSummary
}

// planPayload is the plan file's "launch" object.
//
// Why the options travel and not the built command: the built command's
// environment holds the resolved token, and agx never writes a token.
// agx launch prepares again from Options, every choice pinned to what the
// plan resolved (profile, login, folder, conversation), so no picker,
// auto-pick or binding runs twice and the decisions cannot drift; it then
// checks the result against Summary before starting anything.
//
// Invariant: Options holds the flag values as the user typed them, so a
// reference stays a reference. A literal secret typed on the command line
// (--token-ref or --env with a plain value) is in this 0600 file until
// agx launch deletes it, as it already is in the shell's history.
type planPayload struct {
	Summary PlanSummary `json:"summary"`
	// Cwd is the folder the plan was made in: relative paths in Options
	// (--env-file, --before-run) resolve against it, not the folder the
	// wrapper cds into.
	Cwd string `json:"cwd"`
	// Config is the absolute --config file ("" = the default rules).
	Config  string        `json:"config,omitempty"`
	Options LaunchOptions `json:"options"`
}

// planRecord is the "launch" object of the run folder's plan.jsonc: the
// summary only.
type planRecord struct {
	Summary PlanSummary `json:"summary"`
}

// WritePlan is the plan phase of the shell layer (spec 12): it resolves
// every decision of o like --dry-run (no secret resolves, no run folder,
// no settings file), creates the session folder of agx new (the wrapper
// cds into it next), and writes the plan to o.PlanFile. The returned plan
// carries the notes and warnings to show now; agx launch shows none.
func (a *App) WritePlan(ctx context.Context, o LaunchOptions) (*LaunchPlan, PlanView, error) {
	dry := o
	dry.DryRun, dry.ResolvedSecrets, dry.PlanFile = true, false, ""
	p, err := a.Prepare(ctx, dry)
	if err != nil {
		return nil, PlanView{}, err
	}
	if o.Mode == ModeNew {
		if err := a.createSessionFolder(p.Folder); err != nil {
			return nil, PlanView{}, err
		}
	}
	cfgFile := ""
	if a.d.ConfigFlag != "" {
		cfgFile = a.paths.File
	}
	pay := planPayload{Summary: summaryOf(p), Cwd: a.d.Cwd, Config: cfgFile, Options: pinned(o, p)}
	body, err := json.Marshal(pay)
	if err != nil {
		return nil, PlanView{}, err
	}
	f := plan.File{SchemaVersion: plan.SchemaVersion, CreatedAt: a.Now(), Dir: p.Folder, Launch: body}
	if err := plan.Write(o.PlanFile, f); err != nil {
		return nil, PlanView{}, err
	}
	return p, PlanView{SchemaVersion: f.SchemaVersion, CreatedAt: f.CreatedAt, Dir: f.Dir, PlanSummary: pay.Summary}, nil
}

// summaryOf is the secret-free summary of a prepared plan.
func summaryOf(p *LaunchPlan) PlanSummary {
	return PlanSummary{
		Mode: planModes[p.Mode], Agent: p.Agent, Profile: p.Profile, Home: p.Home, Login: p.Login,
		Binary: p.BinaryPath, Folder: p.Folder, ResumeID: p.ResumeID, Command: p.View,
	}
}

// pinned are the options agx launch prepares with: o with every choice
// the plan phase made fixed, so a replay needs no terminal, no network
// and no second auto-pick. agx new replays as a run in the folder it
// created (a new session folder would get a new timestamp); a resume
// replays the conversation id it found, never --last or the picker.
func pinned(o LaunchOptions, p *LaunchPlan) LaunchOptions {
	o.DryRun, o.ResolvedSecrets, o.PlanFile = false, false, ""
	o.Profile, o.Agent, o.AutoProfile, o.AutoLogin = p.Profile, p.Agent, false, false
	switch p.LoginKind {
	case LoginKindHome:
		o.Login, o.HomeLogin, o.TokenRef = "", true, ""
	case LoginKindTokenRef:
		o.Login, o.HomeLogin = "", false
	default:
		o.Login, o.HomeLogin, o.TokenRef = p.Login, false, ""
	}
	switch p.Mode {
	case ModeNew:
		o.Mode, o.Dir, o.SessionName = ModeRun, p.Folder, ""
	case ModeResume:
		o.ResumeID, o.Resume = p.ResumeID, ResumeOptions{}
	default:
		o.Dir = p.Folder
	}
	return o
}

// ShowPlan reads and checks the plan at path (edge 53) for agx plan show.
func (a *App) ShowPlan(path string) (PlanView, error) {
	f, pay, err := a.readPlanFile(path)
	if err != nil {
		return PlanView{}, err
	}
	return PlanView{SchemaVersion: f.SchemaVersion, CreatedAt: f.CreatedAt, Dir: f.Dir, PlanSummary: pay.Summary}, nil
}

// readPlanFile reads, checks and decodes the plan at path.
func (a *App) readPlanFile(path string) (plan.File, planPayload, error) {
	if path == "" {
		return plan.File{}, planPayload{}, apperr.Usage("--plan-file is required")
	}
	f, err := plan.Read(path, a.Now())
	if err != nil {
		return plan.File{}, planPayload{}, err
	}
	var pay planPayload
	if err := json.Unmarshal(f.Launch, &pay); err != nil {
		return plan.File{}, planPayload{}, apperr.Failure(apperr.CodePlanFile, "plan file "+path+" is not a plan: "+err.Error())
	}
	return f, pay, nil
}

// LaunchPlanFile is agx launch --plan-file (spec 12): it re-checks the
// plan (edge 53), deletes it (single use), prepares the launch for real
// from the pinned options (secrets resolve now, the run folder is
// created), refuses when the result differs from what the plan showed,
// records plan.jsonc in the run folder and commits (run.jsonc,
// before_run). The caller then starts the returned plan (internal/run).
func (a *App) LaunchPlanFile(ctx context.Context, path string) (*LaunchPlan, error) {
	f, pay, err := a.readPlanFile(path)
	if err != nil {
		return nil, err
	}
	if err := plan.Remove(path); err != nil {
		return nil, err
	}
	// The plan's own folder and config, not this process's: the wrapper
	// has cd'd since, and relative paths in the options mean the old cwd.
	d := a.d
	d.Cwd, d.ConfigFlag = pay.Cwd, pay.Config
	b := New(d)
	p, err := b.Prepare(ctx, pay.Options)
	if err != nil {
		return nil, err
	}
	if d := drift(pay.Summary, summaryOf(p)); d != "" {
		return nil, apperr.Failure(apperr.CodePlanFile, "the launch no longer matches its plan ("+d+"); run the command again")
	}
	// The run folder's plan.jsonc keeps the header and the summary only,
	// never the options, which may hold a typed literal secret and would
	// outlive the run.
	body, err := json.Marshal(planRecord{Summary: pay.Summary})
	if err != nil {
		return nil, err
	}
	f.Launch = body
	if err := plan.Write(filepath.Join(p.RunDir, run.PlanFile), f); err != nil {
		return nil, err
	}
	if err := b.Commit(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// drift names the first decision that changed between the plan and the
// launch ("" = none). The mode differs by design for agx new (replayed as
// a run in the folder it created) and the command view by the run id and
// the token placeholder, so neither is compared.
func drift(want, got PlanSummary) string {
	pairs := []struct{ name, want, got string }{
		{"agent", string(want.Agent), string(got.Agent)},
		{"profile", want.Profile, got.Profile},
		{"home", want.Home, got.Home},
		{"login", want.Login, got.Login},
		{"binary", want.Binary, got.Binary},
		{"folder", want.Folder, got.Folder},
		{"conversation", want.ResumeID, got.ResumeID},
	}
	for _, p := range pairs {
		if p.want != p.got {
			return p.name + " " + p.want + " is now " + p.got
		}
	}
	return ""
}
