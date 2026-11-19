package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/login"
)

// LoginUserType names what in config uses a login (login show "users",
// login remove --cascade).
type LoginUserType string

// Login user types: the three places config names a login (spec 15.3).
const (
	// UserProfile is profiles.<name>.login.
	UserProfile LoginUserType = "profile"
	// UserAgentDefault is agents.<id>.defaults.login.
	UserAgentDefault LoginUserType = "agent-default"
	// UserBinding is projects.<dir>.<agent>.login.
	UserBinding LoginUserType = "binding"
	// UserPool is a login pool listing the login as a member (own,
	// shared, or its force).
	UserPool LoginUserType = "pool"
)

// LoginUserTypeValues is the canonical list.
func LoginUserTypeValues() []LoginUserType {
	return []LoginUserType{UserProfile, UserAgentDefault, UserBinding, UserPool}
}

// LoginUser is one config entry that names a login (Name is the pool's
// login name for UserPool).
type LoginUser struct {
	// Type is where the entry lives.
	Type LoginUserType `json:"type"`
	// Name is the profile name, the agent id (agent default) or the bound
	// folder (binding, as written in config).
	Name string `json:"name"`
	// Agent is the agent the entry launches.
	Agent agentio.ID `json:"agent"`
}

// String renders "profile work", "agent-default claude",
// "binding ~/p (claude)".
func (u LoginUser) String() string {
	if u.Type == UserBinding {
		return fmt.Sprintf("%s %s (%s)", u.Type, u.Name, u.Agent)
	}
	return string(u.Type) + " " + u.Name
}

// LoginInfo is one login as list and show print it: never its value.
type LoginInfo struct {
	// Name is the logins.<name> key.
	Name string `json:"name"`
	// Agents are the agents it serves.
	Agents []agentio.ID `json:"agents"`
	// Kind is the token kind (oauth-token, api-key, api-key-helper).
	Kind string `json:"kind"`
	// Source is the readable source ("reference gopass", "plain",
	// "stored (tokens.jsonc)"); SourceType its machine form.
	Source     string           `json:"source"`
	SourceType login.SourceType `json:"source_type"`
	// Account is the display label (an email); "" when unset.
	Account string `json:"account"`
	// Provider is the model provider for provider-keyed agents (pi).
	Provider string `json:"provider,omitempty"`
	// Users are the profiles, agent defaults and bindings that name it.
	Users []LoginUser `json:"users"`
}

// LoginListOptions are agx login list's flags.
type LoginListOptions struct {
	// Agent keeps only logins serving this agent; "" = all.
	Agent agentio.ID
}

// LoginList returns every login, sorted by name (cost R: tokens.jsonc is
// read only to say whether a stored value exists).
func (a *App) LoginList(o LoginListOptions) ([]LoginInfo, error) {
	cfg, err := a.Config()
	if err != nil {
		return nil, err
	}
	if o.Agent != "" {
		if err := a.loginKnownAgent(o.Agent); err != nil {
			return nil, err
		}
	}
	out := []LoginInfo{}
	// The file's order, as v0.3's `login ls` was; the usage table's #
	// column counts in the same order.
	for _, name := range cfg.LoginOrder() {
		l := cfg.Config.Logins[name]
		if o.Agent != "" && !l.Serves(o.Agent) {
			continue
		}
		out = append(out, a.loginInfo(cfg.Config, name, l))
	}
	return out, nil
}

// LoginShowOptions are agx login show's flags.
type LoginShowOptions struct {
	// ID is the login name; required.
	ID string
	// Agent, when set, must be served by the login (edge 11), and the
	// users are narrowed to that agent.
	Agent agentio.ID
}

// LoginShow returns one login with its users.
func (a *App) LoginShow(o LoginShowOptions) (LoginInfo, error) {
	cfg, l, err := a.namedLogin(o.ID)
	if err != nil {
		return LoginInfo{}, err
	}
	info := a.loginInfo(cfg.Config, o.ID, l)
	if o.Agent != "" {
		if err := a.loginKnownAgent(o.Agent); err != nil {
			return LoginInfo{}, err
		}
		if !l.Serves(o.Agent) {
			return LoginInfo{}, loginWrongAgent(o.ID, l, "--agent is "+string(o.Agent))
		}
		info.Users = slices.DeleteFunc(info.Users, func(u LoginUser) bool { return u.Agent != o.Agent })
	}
	return info, nil
}

// LoginToken resolves login id's token from its source (a stored value,
// a reference, a plain value) for agx login token. It reads only what is
// stored or referenced: nothing is refreshed or written. An api-key-helper
// login has no token of its own (the agent runs its command), so it is
// refused rather than run: running it would make agx a second, silent
// caller of a command the user wrote for the agent. Errors come from the
// brief expander, the same wording agx credential uses, so a store's or a
// command's stderr is never repeated. No hint is added: the caller is a
// script or an agent, which reads one line (the CLI drops any hint).
func (a *App) LoginToken(ctx context.Context, id string) (agentio.Token, error) {
	tok, _, err := a.LoginTokenWarn(ctx, id)
	return tok, err
}

// LoginTokenWarn is LoginToken plus its warnings for stderr: a pool login
// picks a member by usage first (cost Q when the cache is old) and prints
// that member's token; a forced pick or a cache fallback is a warning.
func (a *App) LoginTokenWarn(ctx context.Context, id string) (agentio.Token, []report.Warning, error) {
	_, l, err := a.namedLogin(id)
	if err != nil {
		return agentio.Token{}, nil, err
	}
	if l.Kind == config.KindPool {
		_, tok, warnings, err := a.poolMemberToken(ctx, id, l, poolModeToken)
		return tok, warnings, err
	}
	tok, err := a.loginToken(ctx, id, l)
	return tok, nil, err
}

// loginToken resolves a token login's value (LoginToken's rules).
func (a *App) loginToken(ctx context.Context, id string, l config.Login) (agentio.Token, error) {
	if l.Kind == config.KindAPIKeyHelper {
		return agentio.Token{}, apperr.Unsupported(fmt.Sprintf("login %s is an api-key-helper: it has no token to print; the agent runs its helper command itself", id))
	}
	cred, err := login.Credential(ctx, a.exp.Brief(), a.paths, id, l)
	if err != nil {
		return agentio.Token{}, err
	}
	return cred.Token, nil
}

// namedLogin loads the config and finds login id (a usage error when id
// is empty, not-found when missing).
func (a *App) namedLogin(id string) (*config.Loaded, config.Login, error) {
	if id == "" {
		return nil, config.Login{}, apperr.Usage("--id is required: the login name (agx login list)")
	}
	cfg, err := a.Config()
	if err != nil {
		return nil, config.Login{}, err
	}
	l, ok := cfg.Config.Logins[id]
	if !ok {
		return nil, config.Login{}, apperr.Failure(apperr.CodeNotFound, "unknown login "+id).WithHint("agx login list")
	}
	return cfg, l, nil
}

// loginKnownAgent refuses an agent id the registry does not know.
func (a *App) loginKnownAgent(id agentio.ID) error {
	_, err := a.loginAgent(id)
	return err
}

// loginAgent finds an agent in the registry.
func (a *App) loginAgent(id agentio.ID) (agentio.Agent, error) {
	reg, err := a.Registry()
	if err != nil {
		return nil, err
	}
	ag, ok := reg.Get(id)
	if !ok {
		return nil, apperr.Usage(fmt.Sprintf("unknown agent %s; known: %s", id, loginJoinIDs(reg.IDs())))
	}
	return ag, nil
}

// loginWrongAgent is edge 11 outside a launch: a usage error (exit 2).
func loginWrongAgent(name string, l config.Login, this string) error {
	return apperr.New(apperr.CodeWrongAgent, apperr.ClassUsage,
		fmt.Sprintf("login %s serves %s; %s", name, loginJoinIDs(l.Agents), this))
}

// loginInfo builds the printable form of one login.
func (a *App) loginInfo(c config.Config, name string, l config.Login) LoginInfo {
	stored := login.StoredMissing
	if l.Stored {
		// A read error (bad mode, bad JSONC) shows as unreadable, never as
		// missing; login check reports the real reason when it resolves.
		_, found, err := login.ReadStored(a.paths, name)
		stored = login.StoredStateOf(found, err)
	}
	src := login.Describe(l, stored)
	agents := l.Agents
	if agents == nil {
		agents = []agentio.ID{}
	}
	return LoginInfo{
		Name: name, Agents: agents, Kind: l.Kind, Source: src.String(), SourceType: src.Type,
		Account: l.Account, Provider: l.Provider, Users: loginUsers(c, name),
	}
}

// loginUsers lists every config entry naming login name, in a stable
// order: profiles, agent defaults, bindings.
func loginUsers(c config.Config, name string) []LoginUser {
	out := []LoginUser{}
	for _, p := range loginSortedKeys(c.Profiles) {
		if c.Profiles[p].Login == name {
			out = append(out, LoginUser{Type: UserProfile, Name: p, Agent: c.Profiles[p].Agent})
		}
	}
	for _, id := range loginSortedKeys(c.Agents) {
		if c.Agents[id].Defaults.Login == name {
			out = append(out, LoginUser{Type: UserAgentDefault, Name: string(id), Agent: id})
		}
	}
	for _, dir := range loginSortedKeys(c.Projects) {
		for _, id := range loginSortedKeys(c.Projects[dir]) {
			if c.Projects[dir][id].Login == name {
				out = append(out, LoginUser{Type: UserBinding, Name: dir, Agent: id})
			}
		}
	}
	for _, p := range loginSortedKeys(c.Logins) {
		pl := c.Logins[p]
		if pl.Pool == nil || len(pl.Agents) == 0 {
			continue
		}
		if slices.Contains(pl.Pool.Members(), name) || (pl.Pool.Force != nil && *pl.Pool.Force == name) {
			out = append(out, LoginUser{Type: UserPool, Name: p, Agent: pl.Agents[0]})
		}
	}
	return out
}

// loginSortedKeys returns a map's keys sorted.
func loginSortedKeys[K ~string, V config.Login | config.Profile | config.AgentConfig | config.Binding | map[agentio.ID]config.Binding](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// loginJoinIDs renders ids comma-separated.
func loginJoinIDs(ids []agentio.ID) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = string(id)
	}
	return strings.Join(s, ", ")
}
