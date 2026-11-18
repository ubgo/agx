// Package config is agx's own configuration: the typed shape of
// config.jsonc (spec section 15), loading it with schema validation that
// reports every unknown key and wrong type at its line:col, the
// cross-checks the schema cannot express, and comment-preserving writes
// under a cross-process lock.
//
// Why typed JSONC: the file is edited both by people (comments, order and
// blank lines are theirs, principle 11) and by agx commands (profile add,
// login use, project use). textdata/jsonc keeps the layout on writes, and
// schema.StrictKeys turns a typo such as "profles" into an error with a
// position instead of a silently ignored key.
//
// Nothing here knows any agent: agent ids are opaque agentio.IDs, and the
// per-agent launch blocks of a profile are kept as raw JSON for the
// agent's own registry to validate (Profile.Blocks).
package config

import (
	"encoding/json"

	"github.com/ubgo/agentio"
)

// CurrentVersion is the config "version" this agx writes and reads. A
// file with a higher version is refused (edge 46); a lower one does not
// exist yet.
const CurrentVersion = 1

// SchemaURL is the "$schema" written by config init: the published schema
// gives editors completion; agx itself validates with the schema built in.
const SchemaURL = "https://raw.githubusercontent.com/ubgo/agx/main/schema/config.schema.json"

// Config is the whole of config.jsonc.
type Config struct {
	// Schema is the editor schema URL; ignored by agx.
	Schema string `json:"$schema,omitempty"`
	// Version is the file format version (CurrentVersion).
	Version int `json:"version"`
	// DefaultAgent is the agent used when nothing else names one
	// (resolution step 1). "" = ask at a terminal, refuse without one.
	DefaultAgent agentio.ID `json:"default_agent,omitempty"`
	// Sessions holds the session-folder roots (agx new, session promote).
	Sessions Sessions `json:"sessions,omitzero"`
	// Agents holds per-agent defaults and custom agent definitions, keyed
	// by agent id (built-in or custom).
	Agents map[agentio.ID]AgentConfig `json:"agents,omitempty"`
	// Profiles maps a profile name to one home plus launch defaults.
	Profiles map[string]Profile `json:"profiles,omitempty"`
	// Logins maps a login name to a token login.
	Logins map[string]Login `json:"logins,omitempty"`
	// Projects maps a normalized folder path to its per-agent bindings.
	Projects map[string]map[agentio.ID]Binding `json:"projects,omitempty"`
	// Aliases maps a shell alias name to named-flag agx arguments
	// ("run --profile work").
	Aliases map[string]string `json:"aliases,omitempty"`
	// DateFormat is how every human date and time reads (datefmt tokens
	// or a preset: "iso", "short", "long", "us"); "" = each screen's own
	// default. --date-format overrides it for one run, and
	// usage.date_format overrides it for agx usage.
	DateFormat string `json:"date_format,omitempty"`
	// Usage holds agx usage display defaults.
	Usage UsageConfig `json:"usage,omitzero"`
}

// UsageConfig is the "usage" block: how agx usage shows its data.
type UsageConfig struct {
	// TimeFormat is how reset times read: "relative" (the default, "2 hr
	// 50 min"), "day" ("Tue 05:30"), "absolute" ("Tue 24 Nov 05:30") or
	// "both" ("3 hr 10 min (22:20)"). --time-format overrides it for one
	// run.
	TimeFormat string `json:"time_format,omitempty" enum:"relative,day,absolute,both"`
	// DateFormat is how agx usage writes the clock and date parts of its
	// times (datefmt tokens or a preset); "" = the top-level date_format,
	// else "long". --date-format overrides it for one run.
	DateFormat string `json:"date_format,omitempty"`
}

// time_format values (usage.time_format, --time-format).
const (
	// TimeRelative is "in 2 hr 50 min", as agx v0.3 and claude.ai show it.
	TimeRelative = "relative"
	// TimeDay is the weekday and clock time in the --tz zone.
	TimeDay = "day"
	// TimeAbsolute is the weekday, date and clock time in the --tz zone.
	TimeAbsolute = "absolute"
	// TimeBoth is the relative time with the clock time after it: "3 hr
	// 10 min (22:20)", the weekday added when the reset is not today and
	// the whole date when it is more than six days away.
	TimeBoth = "both"
)

// TimeFormatValues is the canonical list, default first.
func TimeFormatValues() []string { return []string{TimeRelative, TimeDay, TimeAbsolute, TimeBoth} }

// Sessions are the folders agx new and session promote use.
type Sessions struct {
	// Root is where agx new creates timestamped session folders; "" = the
	// default (<config dir>/sessions).
	Root string `json:"root,omitempty"`
	// PromoteRoot is the default parent folder session promote moves into.
	PromoteRoot string `json:"promote_root,omitempty"`
}

// AgentConfig is one agents.<id> entry.
type AgentConfig struct {
	// Defaults apply to every profile of this agent (resolution steps 4, 7).
	Defaults AgentDefaults `json:"defaults,omitzero"`
	// Spec defines a custom agent; nil for built-ins. Its id comes from the
	// map key, so "id" may be omitted inside the block.
	Spec *agentio.Spec `json:"spec,omitempty"`
}

// AgentDefaults are an agent's launch defaults.
type AgentDefaults struct {
	// Args are raw args given to every launch of the agent, first, unless
	// a profile sets inherit_defaults false.
	Args []string `json:"args,omitempty"`
	// Login is the agent's default login name; "" = the home login.
	Login string `json:"login,omitempty"`
	// HomeLogin forces the home login (overrides nothing at this level, but
	// is accepted for symmetry with profiles and bindings).
	HomeLogin bool `json:"home_login,omitempty"`
}

// Billing values (profile "billing").
const (
	BillingPlan = "plan"
	BillingAPI  = "api"
)

// BillingValues is the canonical list.
func BillingValues() []string { return []string{BillingPlan, BillingAPI} }

// before_run_from values: where a profile's before_run script is found.
const (
	BeforeRunFromConfig = "config"
	BeforeRunFromFolder = "folder"
)

// BeforeRunFromValues is the canonical list.
func BeforeRunFromValues() []string { return []string{BeforeRunFromConfig, BeforeRunFromFolder} }

// before_run_env values: what environment the script sees.
const (
	BeforeRunEnvSafe = "safe"
	BeforeRunEnvFull = "full"
)

// BeforeRunEnvValues is the canonical list.
func BeforeRunEnvValues() []string { return []string{BeforeRunEnvSafe, BeforeRunEnvFull} }

// settings_delivery values.
const (
	DeliveryFile   = "file"
	DeliveryInline = "inline"
)

// DeliveryValues is the canonical list.
func DeliveryValues() []string { return []string{DeliveryFile, DeliveryInline} }

// Profile is one profiles.<name> entry: a home plus launch defaults.
type Profile struct {
	// Agent is the agent id; required.
	Agent agentio.ID `json:"agent"`
	// Home is the agent config folder ("~/.claude-work"); required; "~"
	// and ${VAR} are expanded.
	Home string `json:"home"`
	// Default marks the agent's default profile, and the profile that a
	// shared home resolves to (6.2).
	Default bool `json:"default,omitempty"`
	// Billing is "plan" (the default) or "api" (pays per token).
	Billing string `json:"billing,omitempty" enum:"plan,api"`
	// Login is the profile's default login name; "" = inherit.
	Login string `json:"login,omitempty"`
	// HomeLogin forces the home login over an inherited one.
	HomeLogin bool `json:"home_login,omitempty"`
	// Args are raw args after the agent defaults.
	Args []string `json:"args,omitempty"`
	// InheritDefaults false drops agents.<id>.defaults.args; nil = true.
	InheritDefaults *bool `json:"inherit_defaults,omitempty"`
	// Model is the default model (layer 1).
	Model string `json:"model,omitempty"`
	// Autonomy is the default autonomy level name (agentio Autonomy).
	Autonomy string `json:"autonomy,omitempty" enum:"default,read-only,ask,accept-edits,full"`
	// Env are literal variables, never expanded.
	Env map[string]string `json:"env,omitempty"`
	// EnvRefs are references, expanded strictly at launch.
	EnvRefs map[string]string `json:"env_refs,omitempty"`
	// EnvFile is a dotenv file relative to the config dir.
	EnvFile string `json:"env_file,omitempty"`
	// EnvFileCommands allows $(…) in EnvFile.
	EnvFileCommands bool `json:"env_file_commands,omitempty"`
	// BeforeRun is a script run before every launch of this profile.
	BeforeRun string `json:"before_run,omitempty"`
	// BeforeRunFrom says where BeforeRun is found: config (default) or folder.
	BeforeRunFrom string `json:"before_run_from,omitempty" enum:"config,folder"`
	// BeforeRunEnv is safe (no credential variables, default) or full.
	BeforeRunEnv string `json:"before_run_env,omitempty" enum:"safe,full"`
	// SettingsDelivery is file (default) or inline.
	SettingsDelivery string `json:"settings_delivery,omitempty" enum:"file,inline"`
	// LiveLogin turns on live login for every launch of the profile.
	LiveLogin bool `json:"live_login,omitempty"`

	// Blocks are the per-agent typed launch blocks, keyed by agent id: the
	// "claude" or "codex" object of a profile (spec 7.3), holding
	// "settings" (a path or an object) and the agent's launch option keys.
	// They are not decoded by encoding/json (the key is any agent id that
	// has typed launch config); Load fills them from the document and the
	// agent registry validates them. A block named after another agent
	// than the profile's is a config error.
	Blocks map[agentio.ID]json.RawMessage `json:"-"`
}

// InheritsDefaults reports whether agent default args apply (default true).
func (p Profile) InheritsDefaults() bool { return p.InheritDefaults == nil || *p.InheritDefaults }

// Login kinds (logins.<name>.kind), the agentio TokenKind wire names that
// a stored login can be.
const (
	KindOAuthToken   = "oauth-token"
	KindAPIKey       = "api-key"
	KindAPIKeyHelper = "api-key-helper"
)

// KindValues is the canonical list.
func KindValues() []string { return []string{KindOAuthToken, KindAPIKey, KindAPIKeyHelper} }

// Login is one logins.<name> entry.
type Login struct {
	// Agents are the agent ids this login serves; at least one.
	Agents []agentio.ID `json:"agents"`
	// Kind is the token kind.
	Kind string `json:"kind" enum:"oauth-token,api-key,api-key-helper"`
	// Token is a reference (${gopass:…}, $(…), ${VAR}) or a plain value.
	// Exactly one of Token, Stored or Command is set (Command only for
	// api-key-helper).
	Token string `json:"token,omitempty"`
	// Stored means the value lives in tokens.jsonc under the login name.
	Stored bool `json:"stored,omitempty"`
	// Account is a display label (an email); never verified.
	Account string `json:"account,omitempty"`
	// Command is the api-key-helper command the agent runs itself.
	Command string `json:"command,omitempty"`
	// Provider names the model provider for agents whose token variable
	// depends on it (pi: "anthropic", "openai"); "" otherwise.
	Provider string `json:"provider,omitempty"`
}

// Serves reports whether the login serves agent id.
func (l Login) Serves(id agentio.ID) bool {
	for _, a := range l.Agents {
		if a == id {
			return true
		}
	}
	return false
}

// Binding is one projects.<dir>.<agent> entry.
type Binding struct {
	// Profile is the profile for this agent in the folder.
	Profile string `json:"profile,omitempty"`
	// Login is the login for this agent in the folder.
	Login string `json:"login,omitempty"`
	// HomeLogin forces the home login in the folder.
	HomeLogin bool `json:"home_login,omitempty"`
}
