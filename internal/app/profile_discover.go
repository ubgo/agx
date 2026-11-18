package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
)

// Discovery sources (spec 6.2), shown in FOUND BY. Patterns, variables,
// processes and search roots carry their detail after a space.
const (
	foundDefault  = "default home"
	foundPattern  = "pattern"
	foundVariable = "variable"
	foundProcess  = "process"
	foundSearch   = "search"
	foundID       = "--id"
)

// DiscoverOptions are agx profile list --discovered's flags.
type DiscoverOptions struct {
	// Agent keeps only this agent's homes ("" = all).
	Agent agentio.ID
	// Search are extra roots: each root and its direct sub-folders are
	// checked (repeat --search).
	Search []string
}

// DiscoveredHome is one row of agx profile list --discovered.
type DiscoveredHome struct {
	Agent agentio.ID `json:"agent"`
	// Path is the home folder ("~/.claude-work").
	Path string `json:"path"`
	// Profiles are the configured profiles with this home (none = not in
	// config yet).
	Profiles []string `json:"profiles"`
	// FoundBy are the sources that found it, first source first.
	FoundBy []string `json:"found_by"`
	// LooksLikeHome is true when the folder holds one of the agent's home
	// markers; false = it matched by name only.
	LooksLikeHome bool `json:"looks_like_home"`

	dir string // absolute path, for import
}

// Discover is agx profile list --discovered: homes found on this machine
// from the six sources of spec 6.2, each with the profiles that already
// use it. It only reads (disk and the process table); agx never uses a
// discovered home until it is a profile (principle 5).
func (a *App) Discover(ctx context.Context, o DiscoverOptions) ([]DiscoveredHome, []report.Warning, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return nil, nil, err
	}
	all := reg.All()
	if o.Agent != "" {
		ag, ok := reg.Get(o.Agent)
		if !ok {
			return nil, nil, unknownAgent(reg, string(o.Agent))
		}
		all = []agentio.Agent{ag}
	}
	d := &discovery{app: a, cfg: cfg.Config, index: map[string]int{}}
	for _, ag := range all {
		if err := d.agentSources(ag); err != nil {
			return nil, nil, err
		}
	}
	if err := ctxErr(ctx); err != nil {
		return nil, nil, err
	}
	d.processes(ctx, all, reg.All())
	for _, root := range o.Search {
		if err := d.search(all, a.expand(root)); err != nil {
			return nil, nil, err
		}
	}
	return d.rows, d.warnings, nil
}

// discovery collects rows, one per (agent, folder).
type discovery struct {
	app      *App
	cfg      config.Config
	rows     []DiscoveredHome
	index    map[string]int
	warnings []report.Warning
}

// add records dir for ag found by source; a folder found again only gains
// the source.
func (d *discovery) add(ag agentio.Agent, dir, source string) error {
	dir = filepath.Clean(dir)
	key := string(ag.ID()) + "\x00" + dir
	if i, ok := d.index[key]; ok {
		if !slices.Contains(d.rows[i].FoundBy, source) {
			d.rows[i].FoundBy = append(d.rows[i].FoundBy, source)
		}
		return nil
	}
	isHome, err := ag.IsHome(dir)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, err.Error())
	}
	row := DiscoveredHome{Agent: ag.ID(), Path: d.app.tilde(dir), Profiles: d.profilesAt(ag.ID(), dir), FoundBy: []string{source}, LooksLikeHome: isHome, dir: dir}
	d.index[key] = len(d.rows)
	d.rows = append(d.rows, row)
	return nil
}

// profilesAt lists the profiles of agent id whose home is dir.
func (d *discovery) profilesAt(id agentio.ID, dir string) []string {
	out := []string{}
	for _, n := range resolve.ProfilesOf(d.cfg, id) {
		if filepath.Clean(d.app.expand(d.cfg.Profiles[n].Home)) == dir {
			out = append(out, n)
		}
	}
	return out
}

// agentSources are sources 1, 2, 3 and 5: the default home, the ExtraHomes
// pattern and the home variable of this shell, for built-in and custom
// agents alike (both come from the agent's spec).
func (d *discovery) agentSources(ag agentio.Agent) error {
	spec := ag.Spec()
	if def := ag.DefaultHome(); spec.DefaultHome != "" && statDir(def.Dir()) {
		if err := d.add(ag, def.Dir(), foundDefault); err != nil {
			return err
		}
	}
	if spec.ExtraHomes != "" {
		matches, err := filepath.Glob(filepath.Join(d.app.d.Host.UserHome, spec.ExtraHomes))
		if err != nil {
			return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("%s pattern %s: %s", ag.ID(), spec.ExtraHomes, err.Error()))
		}
		slices.Sort(matches)
		for _, m := range matches {
			if !statDir(m) {
				continue
			}
			if err := d.add(ag, m, foundPattern+" "+spec.ExtraHomes); err != nil {
				return err
			}
		}
	}
	if h, ok := ag.HomeFromEnv(); ok && statDir(h.Dir()) {
		return d.add(ag, h.Dir(), foundVariable+" "+spec.HomeEnv)
	}
	return nil
}

// processes is source 4: the start environment of running agent
// processes. A process whose environment cannot be read (another user's,
// or Windows) is skipped and counted in one warning.
func (d *discovery) processes(ctx context.Context, wanted, classify []agentio.Agent) {
	lister := d.app.d.Host.Processes
	if lister == nil {
		return
	}
	envs, ok := lister.(agentio.ProcessEnvReader)
	if !ok {
		return
	}
	procs, err := lister.Processes(ctx)
	if err != nil {
		d.warnings = append(d.warnings, report.Warning{Code: WarnDiscovery, Message: "process table: " + apperr.Text(err)})
		return
	}
	unreadable := 0
	for _, p := range procs {
		info, ok := agentio.ClassifyProcess(classify, p)
		if !ok || !slices.ContainsFunc(wanted, func(ag agentio.Agent) bool { return ag.ID() == info.Agent }) {
			continue
		}
		env, err := envs.Env(ctx, p.PID)
		if err != nil {
			unreadable++
			continue
		}
		if !sameUserHome(env, d.app.d.Host.UserHome) {
			continue
		}
		ag, ok := d.agentOnEnv(info.Agent, env)
		if !ok {
			continue
		}
		if h, ok := ag.HomeFromEnv(); ok && statDir(h.Dir()) {
			if err := d.add(ag, h.Dir(), foundProcess+" "+strconv.Itoa(p.PID)); err != nil {
				unreadable++
			}
		}
	}
	if unreadable > 0 {
		d.warnings = append(d.warnings, report.Warning{Code: WarnDiscovery, Message: fmt.Sprintf("%d agent processes could not be read", unreadable)})
	}
}

// envHome is the HOME variable name in a process environment.
const envHome = "HOME"

// sameUserHome reports a process started with agx's own user home: its
// HOME, when the start environment names one, is the same folder.
// Processes of another home context (a sandbox, a container, a test
// fixture's machine, a sudo shell) are not this user's agents, so their
// homes are not offered. A process whose environment has no HOME (Windows
// keeps USERPROFILE; darwin `ps -E` may omit it) is kept. Pure.
func sameUserHome(env []string, userHome string) bool {
	v, ok := agentio.EnvLookup(env, envHome)
	if !ok || v == "" || userHome == "" {
		return true
	}
	return filepath.Clean(v) == filepath.Clean(userHome) || evalOrSelf(v) == evalOrSelf(userHome)
}

// evalOrSelf is p with symlinks resolved, or p when that fails.
func evalOrSelf(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// agentOnEnv builds agent id on a host whose environment is env, so the
// agent's own HomeFromEnv reads a process's variables (each agent knows
// whether its variable names the home or its parent).
func (d *discovery) agentOnEnv(id agentio.ID, env []string) (agentio.Agent, bool) {
	h := d.app.d.Host
	h.Env = env
	custom := map[agentio.ID]agentio.Spec{}
	for cid, ac := range d.cfg.Agents {
		if ac.Spec != nil {
			custom[cid] = *ac.Spec
		}
	}
	reg, err := agents.New(h, custom)
	if err != nil {
		return nil, false
	}
	return reg.Get(id)
}

// search is source 6: root and each direct sub-folder that is a home of
// one of the agents by name (the default home's name or the ExtraHomes
// pattern) or, for agents with home markers, by its contents.
func (d *discovery) search(all []agentio.Agent, root string) error {
	if !statDir(root) {
		return apperr.Failure(apperr.CodeNotFound, "--search "+d.app.tilde(root)+": no such folder")
	}
	dirs := []string{root}
	entries, err := os.ReadDir(root)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "--search "+root+": "+err.Error())
	}
	for _, e := range entries {
		if p := filepath.Join(root, e.Name()); statDir(p) {
			dirs = append(dirs, p)
		}
	}
	for _, dir := range dirs {
		for _, ag := range all {
			ok, err := looksLike(ag, dir)
			if err != nil {
				return err
			}
			if ok {
				if err := d.add(ag, dir, foundSearch+" "+d.app.tilde(root)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// looksLike reports whether dir could be a home of ag: its name matches
// the default home or the ExtraHomes pattern, or the agent has home
// markers and dir holds one (an agent without markers matches only by
// name, else every folder would be its home).
func looksLike(ag agentio.Agent, dir string) (bool, error) {
	spec := ag.Spec()
	base := filepath.Base(dir)
	if spec.DefaultHome != "" && base == filepath.Base(spec.DefaultHome) {
		return true, nil
	}
	if spec.ExtraHomes != "" {
		if ok, err := filepath.Match(filepath.Base(spec.ExtraHomes), base); err == nil && ok {
			return true, nil
		}
	}
	if len(spec.HomeMarkers) == 0 {
		return false, nil
	}
	ok, err := ag.IsHome(dir)
	if err != nil {
		return false, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, err.Error())
	}
	return ok, nil
}

// ProfileImportOptions are agx profile import's flags.
type ProfileImportOptions struct {
	// Discovered imports every discovered home that looks like a home and
	// has no profile yet.
	Discovered bool
	// IDs are home paths to import (repeat --id).
	IDs []string
	// Name names the profile; only with exactly one home.
	Name string
	// Agent limits discovery and --id matching to one agent.
	Agent  agentio.ID
	Search []string
	// Yes writes; without it the result is the plan.
	Yes bool
}

// ImportRow is one profile profile import writes (or would write).
type ImportRow struct {
	Name    string     `json:"name"`
	Agent   agentio.ID `json:"agent"`
	Home    string     `json:"home"`
	Default bool       `json:"default"`
	FoundBy []string   `json:"found_by"`
}

// ProfileImportResult is agx profile import.
type ProfileImportResult struct {
	Profiles []ImportRow `json:"profiles"`
	// Skipped are discovered homes not imported, with the reason.
	Skipped []SkippedHome `json:"skipped"`
	// Written is false for the plan (no --yes).
	Written bool `json:"written"`
}

// SkippedHome is a discovered home profile import leaves out.
type SkippedHome struct {
	Agent  agentio.ID `json:"agent"`
	Home   string     `json:"home"`
	Reason string     `json:"reason"`
}

// Skip reasons.
const (
	skipConfigured = "already a profile"
	skipNameOnly   = "name only, no home markers (import it with --id)"
)

// ProfileImport is agx profile import: profiles for discovered homes
// (--discovered) or named homes (--id), a plan unless --yes. Names come
// from the agent's suggestion for the home (agentio Home.Name: "personal"
// for a default home, "work" for ~/.claude-work), made unique; the first
// profile of an agent without a default becomes its default.
func (a *App) ProfileImport(ctx context.Context, o ProfileImportOptions) (ProfileImportResult, error) {
	res := ProfileImportResult{Profiles: []ImportRow{}, Skipped: []SkippedHome{}}
	if !o.Discovered && len(o.IDs) == 0 {
		return res, apperr.Usage("give --discovered or --id HOME (repeat)")
	}
	if o.Name != "" && (o.Discovered || len(o.IDs) != 1) {
		return res, apperr.Usage("--name needs exactly one --id")
	}
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return res, err
	}
	found, _, err := a.Discover(ctx, DiscoverOptions{Agent: o.Agent, Search: o.Search})
	if err != nil {
		return res, err
	}
	var picked []DiscoveredHome
	if o.Discovered {
		for _, h := range found {
			switch {
			case len(h.Profiles) > 0:
				res.Skipped = append(res.Skipped, SkippedHome{Agent: h.Agent, Home: h.Path, Reason: skipConfigured})
			case !h.LooksLikeHome:
				res.Skipped = append(res.Skipped, SkippedHome{Agent: h.Agent, Home: h.Path, Reason: skipNameOnly})
			default:
				picked = append(picked, h)
			}
		}
	}
	for _, id := range o.IDs {
		h, err := a.homeByPath(cfg.Config, reg.All(), found, o.Agent, id)
		if err != nil {
			return res, err
		}
		if len(h.Profiles) > 0 {
			return res, apperr.Conflict(fmt.Sprintf("%s is already the home of %s profile %s", h.Path, h.Agent, strings.Join(h.Profiles, ", "))).
				WithHint("agx profile add --name NAME --agent " + string(h.Agent) + " --home " + h.Path + "   (a second profile on the same home)")
		}
		if !slices.ContainsFunc(picked, func(p DiscoveredHome) bool { return p.Agent == h.Agent && p.dir == h.dir }) {
			picked = append(picked, h)
		}
	}
	taken := map[string]bool{}
	for n := range cfg.Config.Profiles {
		taken[n] = true
	}
	hasDefault := map[agentio.ID]bool{}
	for _, p := range cfg.Config.Profiles {
		hasDefault[p.Agent] = hasDefault[p.Agent] || p.Default
	}
	for _, h := range picked {
		ag, _ := reg.Get(h.Agent) // every discovered row comes from a registry agent
		name := o.Name
		if name != "" {
			if err := checkNewName(cfg.Config, name); err != nil {
				return res, err
			}
		} else {
			name = uniqueName(suggestName(ag, h.dir), ag, h.dir, taken)
		}
		taken[name] = true
		row := ImportRow{Name: name, Agent: h.Agent, Home: h.Path, Default: !hasDefault[h.Agent], FoundBy: h.FoundBy}
		hasDefault[h.Agent] = true
		res.Profiles = append(res.Profiles, row)
	}
	if !o.Yes || len(res.Profiles) == 0 {
		return res, nil
	}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error {
		for _, r := range res.Profiles {
			if err := d.Set(config.Pointer("profiles", r.Name), newProfile{Agent: r.Agent, Home: r.Home, Default: r.Default}); err != nil {
				return err
			}
		}
		return nil
	})
	res.Written = err == nil
	return res, err
}

// homeByPath finds the home --id names: the discovered row at that path,
// else the one agent the folder looks like a home of (by name or
// markers). --agent decides when given, even for a folder that does not
// look like a home: the user named both (principle 3).
func (a *App) homeByPath(cfg config.Config, all []agentio.Agent, found []DiscoveredHome, agent agentio.ID, id string) (DiscoveredHome, error) {
	dir := filepath.Clean(a.expand(id))
	if !statDir(dir) {
		return DiscoveredHome{}, apperr.Failure(apperr.CodeNotFound, "--id "+a.tilde(dir)+": no such folder")
	}
	var hits []DiscoveredHome
	for _, h := range found {
		if h.dir == dir {
			hits = append(hits, h)
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	var cands []agentio.Agent
	for _, ag := range all {
		if agent != "" && ag.ID() != agent {
			continue
		}
		ok, err := looksLike(ag, dir)
		if err != nil {
			return DiscoveredHome{}, err
		}
		if ok || agent != "" {
			cands = append(cands, ag)
		}
	}
	switch len(cands) {
	case 0:
		return DiscoveredHome{}, apperr.Failure(apperr.CodeNotFound, a.tilde(dir)+" does not look like a home of any known agent").
			WithHint("add --agent ID to import it anyway")
	case 1:
		d := &discovery{app: a, cfg: cfg, index: map[string]int{}}
		if err := d.add(cands[0], dir, foundID); err != nil {
			return DiscoveredHome{}, err
		}
		return d.rows[0], nil
	}
	ids := make([]string, len(cands))
	for i, ag := range cands {
		ids[i] = string(ag.ID())
	}
	return DiscoveredHome{}, apperr.New(apperr.CodeAmbiguous, apperr.ClassUsage,
		fmt.Sprintf("%s looks like a home of %s; add --agent", a.tilde(dir), strings.Join(ids, ", ")))
}

// suggestName is the agent's suggested name for a home: agentio's name for
// the default home ("personal"), else the folder name without the default
// home's name and separator (~/.claude-work → work), else without its
// leading dot. A home named like the default home inside another folder
// (Gemini CLI's PARENT/.gemini) takes the parent's name.
func suggestName(ag agentio.Agent, dir string) string {
	def := ag.DefaultHome()
	if filepath.Clean(def.Dir()) == dir && def.Name() != "" {
		return def.Name()
	}
	defBase := filepath.Base(ag.Spec().DefaultHome)
	base := filepath.Base(dir)
	if base == defBase {
		base = filepath.Base(filepath.Dir(dir))
	}
	if rest, ok := strings.CutPrefix(base, defBase+homeNameSep); ok && rest != "" {
		base = rest
	}
	base = strings.TrimLeft(base, ".")
	var b strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 || !nameRe.MatchString(b.String()) {
		return string(ag.ID())
	}
	return b.String()
}

// uniqueName returns the first free name of: name; for a default home,
// the agent id (a second agent's default home is "codex", not
// "personal-codex"); name-<agent>; name-<agent>-N.
func uniqueName(name string, ag agentio.Agent, dir string, taken map[string]bool) string {
	id := string(ag.ID())
	cands := []string{name}
	if filepath.Clean(ag.DefaultHome().Dir()) == dir {
		cands = append(cands, id)
	}
	base := name + homeNameSep + id
	if name == id {
		base = name
	}
	cands = append(cands, base)
	for _, n := range cands {
		if !taken[n] {
			return n
		}
	}
	for i := 2; ; i++ {
		if n := base + homeNameSep + strconv.Itoa(i); !taken[n] {
			return n
		}
	}
}
