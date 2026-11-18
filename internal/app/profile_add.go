package app

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/resolve"
)

// nameRe is the charset of a profile name: it is a config key, a flag
// value and part of messages, so it stays a plain word.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// homeNameSep joins an agent's default home and a profile name into the
// derived home folder (~/.claude-work), the same convention agentio's
// ExtraHomes patterns discover.
const homeNameSep = "-"

// ProfileAddOptions are agx profile add's flags.
type ProfileAddOptions struct {
	Name      string
	Agent     agentio.ID
	Home      string
	Login     string
	HomeLogin bool
	Billing   string
	// Bare skips linking the default home's shared files.
	Bare bool
	// LoginSetup runs the agent's token setup command in the new home
	// (cost X), then stores the pasted token as a login named like the
	// profile; the CLI runs the command between ProfileAdd and
	// StoreSetupToken (only internal/run starts processes).
	LoginSetup bool
	DryRun     bool
}

// HomeLinkRow is one shared file a new home reaches through a symlink.
type HomeLinkRow struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
	// Exists means the target is already there; it is never overwritten.
	Exists bool `json:"exists"`
}

// ProfileAddResult is agx profile add.
type ProfileAddResult struct {
	Name  string     `json:"name"`
	Agent agentio.ID `json:"agent"`
	Home  string     `json:"home"`
	// HomeSource says where the home came from (--home or derived).
	HomeSource string `json:"home_source"`
	// HomeCreated is true when the home folder is new (created 0700).
	HomeCreated bool `json:"home_created"`
	// Default is true when this is the agent's first profile.
	Default bool   `json:"default"`
	Login   string `json:"login"`
	Billing string `json:"billing"`
	// Links are the default home's shared files linked into the new home
	// (none with --bare or for the default home itself).
	Links []HomeLinkRow `json:"links"`
	// LoginSetup is the token setup command line with --login-setup
	// ("" without it).
	LoginSetup string `json:"login_setup,omitempty"`
	// Written is false for --dry-run.
	Written bool `json:"written"`
}

// newProfile is the entry profile add writes, in the order of spec 15.2.
type newProfile struct {
	Agent     agentio.ID `json:"agent"`
	Home      string     `json:"home"`
	Default   bool       `json:"default,omitempty"`
	Billing   string     `json:"billing,omitempty"`
	Login     string     `json:"login,omitempty"`
	HomeLogin bool       `json:"home_login,omitempty"`
}

// ProfileAdd is agx profile add: a profile entry for a home, plus the
// default home's shared files linked into it (agentio SharedHomeFiles)
// unless --bare. --dry-run returns the plan and writes nothing.
func (a *App) ProfileAdd(ctx context.Context, o ProfileAddOptions) (ProfileAddResult, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return ProfileAddResult{}, err
	}
	if err := checkNewName(cfg.Config, o.Name); err != nil {
		return ProfileAddResult{}, err
	}
	if o.Agent == "" {
		return ProfileAddResult{}, apperr.Usage("--agent is required")
	}
	ag, ok := reg.Get(o.Agent)
	if !ok {
		return ProfileAddResult{}, unknownAgent(reg, string(o.Agent))
	}
	if err := checkLoginChoice(cfg.Config, o.Agent, o.Login, o.HomeLogin); err != nil {
		return ProfileAddResult{}, err
	}
	if err := checkBilling(o.Billing); err != nil {
		return ProfileAddResult{}, err
	}
	dir, src, err := a.newHomeDir(ag, o.Home, o.Name)
	if err != nil {
		return ProfileAddResult{}, err
	}
	res := ProfileAddResult{
		Name: o.Name, Agent: o.Agent, Home: a.tilde(dir), HomeSource: src, HomeCreated: !statDir(dir),
		Default: len(resolve.ProfilesOf(cfg.Config, o.Agent)) == 0, Login: o.Login, Billing: o.Billing, Links: []HomeLinkRow{},
	}
	if res.Login == "" {
		res.Login = HomeLoginText
	}
	if res.Billing == "" {
		res.Billing = config.BillingPlan
	}
	home := resolve.HomeOf(ag, dir, o.Name)
	if o.LoginSetup {
		setup, err := a.checkLoginSetup(cfg.Config, ag, home, o)
		if err != nil {
			return ProfileAddResult{}, err
		}
		res.LoginSetup, res.Login = setup, o.Name
	}
	var links []agentio.HomeLink
	linker, canLink := agentio.HomeLinkerOf(ag)
	if !o.Bare && !home.IsDefault() && canLink {
		links, err = linker.PlanHomeLinks(ag.DefaultHome(), home)
		if err != nil {
			return ProfileAddResult{}, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "plan shared files: "+err.Error())
		}
		for _, l := range links {
			res.Links = append(res.Links, HomeLinkRow{Name: l.Name, From: a.tilde(l.From), To: a.tilde(l.To), Exists: l.Exists})
		}
	}
	if o.DryRun {
		return res, nil
	}
	if res.HomeCreated {
		if err := config.CreateHome(dir); err != nil {
			return res, err
		}
	}
	entry := newProfile{Agent: o.Agent, Home: a.tilde(dir), Default: res.Default, Billing: o.Billing, Login: o.Login, HomeLogin: o.HomeLogin}
	if err := a.editConfig(ctx, func(d *jsonc.Doc) error {
		return d.Set(config.Pointer("profiles", o.Name), entry)
	}); err != nil {
		return res, err
	}
	res.Written = true
	if len(links) > 0 {
		if err := linker.ApplyHomeLinks(links); err != nil {
			return res, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "profile "+o.Name+" added, but linking shared files failed: "+err.Error())
		}
	}
	return res, nil
}

// checkNewName refuses an empty, malformed or taken profile name.
func checkNewName(c config.Config, name string) error {
	if name == "" {
		return apperr.Usage("--name is required")
	}
	if !nameRe.MatchString(name) {
		return apperr.Usage(fmt.Sprintf("--name %q: use letters, digits, '.', '_' and '-'", name))
	}
	if _, ok := c.Profiles[name]; ok {
		return apperr.Conflict("profile " + name + " already exists").WithHint("agx profile edit --id " + name)
	}
	return nil
}

// checkLoginChoice refuses --login with --home-login, an unknown login,
// and a login that does not serve agent id.
func checkLoginChoice(c config.Config, id agentio.ID, login string, homeLogin bool) error {
	if login != "" && homeLogin {
		return apperr.Conflict("--login and --home-login both choose the login; pick one")
	}
	if login == "" {
		return nil
	}
	lg, ok := c.Logins[login]
	if !ok {
		return apperr.Failure(apperr.CodeNotFound, "unknown login "+login).WithHint("agx login list")
	}
	if !lg.Serves(id) {
		return apperr.New(apperr.CodeWrongAgent, apperr.ClassUsage, fmt.Sprintf("login %s serves %s, not %s", login, joinAgentIDs(lg.Agents), id))
	}
	return nil
}

// checkBilling refuses a billing value outside plan and api.
func checkBilling(b string) error {
	if b != "" && !slices.Contains(config.BillingValues(), b) {
		return apperr.Usage(fmt.Sprintf("--billing %q: use %s", b, strings.Join(config.BillingValues(), " or ")))
	}
	return nil
}

// Home sources of profile add.
const (
	homeFromFlag    = "--home"
	homeFromDerived = "derived from the agent's default home and --name"
)

// newHomeDir decides the home of a new profile: --home, else the agent's
// default home with "-<name>" (~/.claude-work). The agent's spec decides
// what a home can be: an agent with no home variable has one home only,
// and an agent whose variable names the parent folder (Gemini CLI) needs
// homes named like its default home, so --home is required there.
func (a *App) newHomeDir(ag agentio.Agent, flag, name string) (string, string, error) {
	spec := ag.Spec()
	def := ag.DefaultHome().Dir()
	defBase := filepath.Base(spec.DefaultHome)
	dir, src := a.expand(flag), homeFromFlag
	if flag == "" {
		if spec.HomeEnvNamesParent {
			return "", "", apperr.Usage(fmt.Sprintf("--home is required for %s: its homes are folders named %s (PARENT/%s)", ag.ID(), defBase, defBase))
		}
		dir, src = filepath.Join(filepath.Dir(def), defBase+homeNameSep+name), homeFromDerived
	}
	if filepath.Clean(dir) == filepath.Clean(def) {
		return dir, src, nil
	}
	if spec.HomeEnv == "" {
		return "", "", apperr.Unsupported(fmt.Sprintf("%s has one home only (%s); it has no variable to select another", ag.ID(), a.tilde(def)))
	}
	if spec.HomeEnvNamesParent && filepath.Base(dir) != defBase {
		return "", "", apperr.Unsupported(fmt.Sprintf("%s homes must be named %s (PARENT/%s); %s is not", ag.ID(), defBase, defBase, a.tilde(dir)))
	}
	return dir, src, nil
}
