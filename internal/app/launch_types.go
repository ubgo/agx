package app

import (
	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/internal/run"
)

// LaunchMode is which launching command built the plan.
type LaunchMode uint8

// Launch modes.
const (
	// ModeRun is agx run: a new conversation in --dir or the current folder.
	ModeRun LaunchMode = iota
	// ModeNew is agx new: a new conversation in a fresh session folder.
	ModeNew
	// ModeResume is agx resume: continue a stored conversation in its folder.
	ModeResume
)

// Consent values (--consent).
const (
	// ConsentFullAutonomy allows --autonomy full (principle 9).
	ConsentFullAutonomy = "full-autonomy"
)

// LaunchOptions carries every launch flag, already parsed (spec 13.1).
// Strings are raw flag values; validation happens in Prepare so the same
// messages appear for every launching command.
type LaunchOptions struct {
	Mode LaunchMode
	// Resolution inputs (profile, agent, login, folder).
	resolve.Inputs
	// Layer 1 (common options).
	Model    string
	Prompt   string
	AddDirs  []string
	Autonomy string
	Consent  string
	Headless bool
	// Layer 2: --option key=value (repeat).
	Options []string
	// Settings: --setting key=value (repeat) and --settings-delivery.
	Settings         []string
	SettingsDelivery string
	// Environment.
	Env      []string
	NoEnv    []string
	EnvFiles []string
	// BeforeRun is --before-run PATH (relative to the current folder).
	BeforeRun string
	// LiveLogin is --live-login.
	LiveLogin bool
	// DryRun prints the plan and starts nothing; ResolvedSecrets also
	// resolves references (cost X) and reports "resolved" or the error.
	DryRun          bool
	ResolvedSecrets bool
	// Raw are the args after "--", passed verbatim and last.
	Raw []string
	// ResumeID is the conversation to resume (ModeResume): --id.
	ResumeID string
	// Resume holds agx resume's selection flags (--last, --query,
	// --picker, --scope, --limit); ResumeID above is --id.
	Resume ResumeOptions
	// SessionName is agx new --name (slug words).
	SessionName string
	// PlanFile is the hidden --plan-file: write the plan for the shell
	// layer instead of starting the agent.
	PlanFile string
}

// LaunchPlan is everything resolved for one launch: what Start runs and
// what --dry-run / --json shows. Command.Env holds the token; print it only
// through View, built by secretRedactor (redaction by source and value).
type LaunchPlan struct {
	// Mode is the launching command.
	Mode LaunchMode `json:"-"`
	// RunID and RunDir name the run folder (created by Commit, not Prepare).
	RunID  string `json:"run_id"`
	RunDir string `json:"run_dir"`
	// Resolution is the agent, profile, home and login.
	Agent   agentio.ID `json:"agent"`
	Profile string     `json:"profile"`
	Home    string     `json:"home"`
	Login   string     `json:"login"`
	// LoginKind is named, home or token-ref: what Login holds.
	LoginKind string `json:"login_kind"`
	// Folder is the launch folder.
	Folder string `json:"folder"`
	// ResumeID is the conversation a resume continues ("" for run/new).
	ResumeID string `json:"resume_id,omitempty"`
	// BinaryPath is the resolved binary; Version from install metadata.
	BinaryPath string `json:"binary"`
	Version    string `json:"version,omitempty"`
	// Command is the built process; its Env holds the token, so it is
	// never encoded: View is what --dry-run and --json show.
	Command agentio.Command `json:"-"`
	// View is the printable command: argv, folder and only the env
	// changes (redacted), never the inherited environment, which may hold
	// the user's own secrets under names that do not look secret.
	View CommandView `json:"command"`
	// Decisions are every decision with its source (spec 6.6).
	Decisions []report.Decision `json:"decisions"`
	// Warnings are non-fatal notes for --dry-run and stderr.
	Warnings []report.Warning `json:"warnings"`
	// Notes are the one-line stderr notes (principle 2).
	Notes []string `json:"notes"`
	// BeforeRun is the script to run first; nil = none.
	BeforeRun *BeforeRunPlan `json:"before_run,omitempty"`
	// Options is the merged typed launch options object (profile block,
	// then --option) for agentio's LaunchConfigured; nil = none.
	Options []byte `json:"-"`
	// Settings is the merged settings object to deliver (nil = none) and
	// how; the file is written by Commit.
	Settings         []byte `json:"-"`
	SettingsDelivery string `json:"settings_delivery,omitempty"`
	// Live is the live-login state to write; nil = not a live run.
	Live *LiveStart `json:"live,omitempty"`
	// Record is run.jsonc, written by Commit.
	Record run.Record `json:"-"`
	// ParentStays is true on Windows (no exec; stated in --dry-run).
	ParentStays bool `json:"parent_stays,omitempty"`

	// finish completes the command once the run folder exists (settings
	// file path, live helper); set by the launch-config step.
	finish func(p *LaunchPlan) error
	// liveSrc is the source of the settings a pool's live plan adds
	// ("login auto (pool)"); "" = "--live-login".
	liveSrc string
	// rawSettings are the raw settings flags taken out of the raw args
	// (request), merged into Settings by the launch-config step.
	rawSettings []rawSettingsArg
}

// BeforeRunPlan is a resolved before_run script.
type BeforeRunPlan struct {
	// Path is the script (absolute).
	Path string `json:"path"`
	// Source says which input named it.
	Source string `json:"source"`
	// Dir is where it runs (the launch folder).
	Dir string `json:"dir"`
	// FullEnv is before_run_env "full" (credential variables kept).
	FullEnv bool `json:"full_env,omitempty"`
}

// LiveStart is the live-login state of a new run (live.jsonc).
type LiveStart struct {
	// StartLogin is the login the run started with.
	StartLogin string `json:"start_login"`
	// Helper is the command the agent runs to fetch its credential.
	Helper string `json:"helper"`
}

// CommandView is the printable form of a built command.
type CommandView struct {
	// Line is the argv shell-quoted for display (never run through a shell).
	Line string   `json:"line"`
	Path string   `json:"path"`
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
	// Env are the changes against agx's own environment: "set NAME=value"
	// (secret values redacted) and "removed NAME".
	Env []string `json:"env"`
}

// ResumeOptions are agx resume's conversation-selection flags (13.1).
type ResumeOptions struct {
	// Last picks the newest chat without a picker.
	Last bool
	// Query narrows the picker by title, folder or id.
	Query string
	// Picker is "chat" (default) or "folder".
	Picker string
	// Scope is "folder" (default) or "all".
	Scope string
	// Limit caps the conversations considered, newest first (the flag's
	// default is 50); 0 = no cap, as in conversation list.
	Limit int
}

// LoginKind values of LaunchPlan.LoginKind.
const (
	LoginKindNamed    = "named"
	LoginKindHome     = "home"
	LoginKindTokenRef = "token-ref"
)
