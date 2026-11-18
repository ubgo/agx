package app

import (
	"context"
	"fmt"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
)

// BindingRow is one row of agx project list: one folder, one agent.
type BindingRow struct {
	// Dir is the bound folder ("~/projects/client").
	Dir   string     `json:"dir"`
	Agent agentio.ID `json:"agent"`
	// Profile is the bound profile ("" = not bound).
	Profile string `json:"profile"`
	// Login is the bound login, "home login", or "" when not bound.
	Login string `json:"login"`
}

// ProjectList is agx project list: every binding, per agent, by folder.
func (a *App) ProjectList() ([]BindingRow, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return nil, err
	}
	rows := []BindingRow{}
	for _, dir := range sortedNames(cfg.Config.Projects) {
		binds := cfg.Config.Projects[dir]
		for _, id := range bindingOrder(reg.IDs(), binds) {
			rows = append(rows, BindingRow{Dir: a.tilde(dir), Agent: id, Profile: binds[id].Profile, Login: bindingLogin(binds[id])})
		}
	}
	return rows, nil
}

// bindingOrder lists the agents of a folder's bindings in registry order,
// then any id the registry does not know (config check reports those).
func bindingOrder(ids []agentio.ID, binds map[agentio.ID]config.Binding) []agentio.ID {
	var out []agentio.ID
	seen := map[agentio.ID]bool{}
	for _, id := range ids {
		if _, ok := binds[id]; ok {
			out = append(out, id)
			seen[id] = true
		}
	}
	for _, id := range sortedNames(binds) {
		if !seen[id] {
			out = append(out, id)
		}
	}
	return out
}

// bindingLogin renders a binding's login choice.
func bindingLogin(b config.Binding) string {
	if b.HomeLogin {
		return HomeLoginText
	}
	return b.Login
}

// ProjectShowOptions are agx project show's flags.
type ProjectShowOptions struct {
	// Dir is the folder ("" = the current folder).
	Dir     string
	Agent   agentio.ID
	Profile string
	Login   string
}

// ProjectShowResult is what a run in the folder would use (the decisions
// carry each value's source).
type ProjectShowResult struct {
	Dir     string     `json:"dir"`
	Agent   agentio.ID `json:"agent"`
	Profile string     `json:"profile"`
	Home    string     `json:"home"`
	Login   string     `json:"login"`
	// Binding is the nearest bound folder that decided something ("" =
	// none).
	Binding string `json:"binding"`
	// Notes are the stderr notes a run would print (principle 2).
	Notes []string `json:"notes"`
}

// ProjectShow is agx project show: the decisions a run in --dir would
// make (agent, profile, home, login), each with its source, through the
// same resolver as run --dry-run, without building a command. It never
// asks and never probes: an --auto choice is not offered here.
func (a *App) ProjectShow(o ProjectShowOptions) (ProjectShowResult, []report.Decision, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return ProjectShowResult{}, nil, err
	}
	dir := a.d.Cwd
	if o.Dir != "" {
		dir = a.expand(o.Dir)
	}
	if !statDir(dir) {
		return ProjectShowResult{}, nil, apperr.Failure(apperr.CodeFolderGone, "folder "+dir+" does not exist")
	}
	rs := &resolve.Resolver{Cfg: cfg, Agents: reg, Lookup: a.d.Lookup, UserHome: a.d.Host.UserHome, FoldOS: a.foldOS}
	res, err := rs.Resolve(resolve.Inputs{Profile: o.Profile, Agent: o.Agent, Login: o.Login, Dir: dir})
	if err != nil {
		return ProjectShowResult{}, nil, err
	}
	out := ProjectShowResult{
		Dir: a.tilde(dir), Agent: res.Agent.ID(), Profile: res.ProfileName, Home: a.tilde(res.Home.Dir()),
		Login: loginText(res.Login), Notes: res.Notes,
	}
	if out.Notes == nil {
		out.Notes = []string{}
	}
	if res.BindingDir != "" {
		out.Binding = a.tilde(res.BindingDir)
	}
	res.Decisions = append(res.Decisions, report.Decision{Name: resolve.DecFolder, Value: a.tilde(dir), Source: folderShowSource(o.Dir)})
	return out, res.Decisions, nil
}

// oneOffLoginText is project show's human wording for a --token-ref.
const oneOffLoginText = "one-off token"

// loginText renders a resolved login choice.
func loginText(l resolve.Login) string {
	switch l.Kind {
	case resolve.LoginNamed:
		return l.Name
	case resolve.LoginTokenRef:
		return oneOffLoginText
	}
	return HomeLoginText
}

// folderShowSource names where project show's folder came from.
func folderShowSource(flag string) string {
	if flag != "" {
		return "--dir"
	}
	return "current folder"
}

// ProjectUseOptions are agx project use's flags.
type ProjectUseOptions struct {
	Dir       string
	Profile   string
	Agent     agentio.ID
	Login     string
	HomeLogin bool
}

// ProjectUseResult is agx project use: the binding as written.
type ProjectUseResult struct {
	// Dir is the stored (normalized) folder.
	Dir     string     `json:"dir"`
	Agent   agentio.ID `json:"agent"`
	Profile string     `json:"profile"`
	Login   string     `json:"login"`
}

// ProjectUse is agx project use: bind a folder for one agent. The agent
// comes from --profile (or --agent, which must agree); only the given
// fields change, so --agent Y --login Z keeps a bound profile. The folder
// is stored normalized (absolute, symlinks resolved), so a folder reached
// through a symlink binds its real path (edge 30).
func (a *App) ProjectUse(ctx context.Context, o ProjectUseOptions) (ProjectUseResult, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return ProjectUseResult{}, err
	}
	if o.Profile == "" && o.Login == "" && !o.HomeLogin {
		return ProjectUseResult{}, apperr.Usage("give --profile, --login or --home-login")
	}
	id := o.Agent
	if o.Profile != "" {
		p, ok := cfg.Config.Profiles[o.Profile]
		if !ok {
			return ProjectUseResult{}, unknownProfile(o.Profile)
		}
		if id != "" && id != p.Agent {
			return ProjectUseResult{}, apperr.Conflict(fmt.Sprintf("--agent %s and --profile %s (a %s profile) disagree; pick one", id, o.Profile, p.Agent))
		}
		id = p.Agent
	}
	if id == "" {
		return ProjectUseResult{}, apperr.Usage("--agent is required with --login or --home-login alone (a binding is per agent)")
	}
	if _, ok := reg.Get(id); !ok {
		return ProjectUseResult{}, unknownAgent(reg, string(id))
	}
	if err := checkLoginChoice(cfg.Config, id, o.Login, o.HomeLogin); err != nil {
		return ProjectUseResult{}, err
	}
	dir := a.d.Cwd
	if o.Dir != "" {
		dir = a.expand(o.Dir)
	}
	if !statDir(dir) {
		return ProjectUseResult{}, apperr.Failure(apperr.CodeFolderGone, "folder "+dir+" does not exist")
	}
	key := a.normDir(dir)
	// An existing entry spelled differently (written by hand, or another
	// case on macOS/Windows) is the same folder: edit it in place rather
	// than adding a twin.
	want := a.dirKey(dir)
	for k := range cfg.Config.Projects {
		if a.dirKey(k) == want {
			key = k
			break
		}
	}
	old := cfg.Config.Projects[key][id]
	at := func(k ...string) []string { return append([]string{"projects", key, string(id)}, k...) }
	var edits []edit
	if o.Profile != "" {
		edits = append(edits, setTo(at(pkProfile), o.Profile))
	}
	switch {
	case o.Login != "":
		edits = append(edits, setTo(at(pkLogin), o.Login), deleteIf(at(pkHomeLogin), old.HomeLogin))
	case o.HomeLogin:
		edits = append(edits, setTo(at(pkHomeLogin), true), deleteIf(at(pkLogin), old.Login != ""))
	}
	if err := a.editConfig(ctx, func(d *jsonc.Doc) error { return applyEdits(d, edits) }); err != nil {
		return ProjectUseResult{}, err
	}
	b := old
	if o.Profile != "" {
		b.Profile = o.Profile
	}
	switch {
	case o.Login != "":
		b.Login, b.HomeLogin = o.Login, false
	case o.HomeLogin:
		b.Login, b.HomeLogin = "", true
	}
	return ProjectUseResult{Dir: a.tilde(key), Agent: id, Profile: b.Profile, Login: bindingLogin(b)}, nil
}

// ProjectRemoveResult is agx project remove.
type ProjectRemoveResult struct {
	Dir string `json:"dir"`
	// Agents are the agents whose binding was removed.
	Agents []agentio.ID `json:"agents"`
}

// ProjectRemove is agx project remove: drop a folder's binding for one
// agent (--agent) or for all.
func (a *App) ProjectRemove(ctx context.Context, dirFlag string, id agentio.ID) (ProjectRemoveResult, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return ProjectRemoveResult{}, err
	}
	dir := a.d.Cwd
	if dirFlag != "" {
		dir = a.expand(dirFlag)
	}
	norm := a.dirKey(dir)
	key, found := "", false
	for k := range cfg.Config.Projects {
		if a.dirKey(k) == norm {
			key, found = k, true
			break
		}
	}
	if !found {
		return ProjectRemoveResult{}, apperr.Failure(apperr.CodeNotFound, "no binding for "+a.tilde(dir)).WithHint("agx project list")
	}
	binds := cfg.Config.Projects[key]
	res := ProjectRemoveResult{Dir: a.tilde(key)}
	if id != "" {
		if _, ok := binds[id]; !ok {
			return ProjectRemoveResult{}, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("%s has no %s binding", a.tilde(key), id)).WithHint("agx project list")
		}
		res.Agents = []agentio.ID{id}
	} else {
		res.Agents = bindingOrder(reg.IDs(), binds)
	}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error {
		if id == "" || len(binds) == 1 {
			return d.Delete(config.Pointer("projects", key))
		}
		return d.Delete(config.Pointer("projects", key, string(id)))
	})
	return res, err
}
