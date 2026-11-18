package app

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/human"
	"github.com/ubgo/agx/internal/picker"
	"github.com/ubgo/agx/internal/resolve"
)

// Resume picker modes (--picker).
const (
	// ResumePickerChat picks one chat from the scope (the default).
	ResumePickerChat = "chat"
	// ResumePickerFolder picks a folder first, then one of its chats.
	ResumePickerFolder = "folder"
)

// ResumePickerValues is the canonical list of --picker values.
func ResumePickerValues() []string { return []string{ResumePickerChat, ResumePickerFolder} }

// Resume scopes (--scope).
const (
	// ResumeScopeFolder lists the chats started in the current folder
	// (the default).
	ResumeScopeFolder = "folder"
	// ResumeScopeAll lists the chats of every configured home.
	ResumeScopeAll = "all"
)

// ResumeScopeValues is the canonical list of --scope values.
func ResumeScopeValues() []string { return []string{ResumeScopeFolder, ResumeScopeAll} }

// DefaultResumeLimit is how many conversations resume considers (--limit).
// It is conversation list's default too: both commands read --limit the
// same way, newest first, and 0 means no cap in both.
const DefaultResumeLimit = DefaultConversationLimit

// resumeHome is one configured home searched for conversations; homes
// shared by several profiles are searched once (6.2).
type resumeHome struct {
	agent    agentio.Agent
	lister   agentio.ConversationLister
	home     agentio.Home
	dir      string
	profiles []string
}

// resumeMatch is a conversation and the home that stores it.
type resumeMatch struct {
	conv agentio.Conversation
	home *resumeHome
}

// resumeTarget finds the conversation to resume (spec 6.5) and sets
// o.Agent, o.Profile, o.Dir and o.ResumeID, plus who decided each
// (resolve.Inputs AgentFrom / ProfileFrom / HomeFrom). History comes from
// the chat's home; the login is resolved afterwards by Prepare as for any
// launch, with the binding of the chat's folder (decision 7).
//
// The conversation is chosen by --id (exact, else a unique prefix),
// --last (the newest in the scope) or the picker (a terminal is needed;
// edge 25). --profile and --agent narrow the homes searched.
func (a *App) resumeTarget(ctx context.Context, o *LaunchOptions) error {
	if err := checkResumeFlags(o); err != nil {
		return err
	}
	cfg, err := a.Config()
	if err != nil {
		return err
	}
	reg, err := a.Registry()
	if err != nil {
		return err
	}
	homes, skipped, err := a.resumeHomes(cfg.Config, o)
	if err != nil {
		return err
	}
	var m resumeMatch
	if o.ResumeID != "" {
		m, err = a.resumeByID(ctx, homes, skipped, o.ResumeID)
	} else {
		m, err = a.resumeByList(ctx, homes, o)
	}
	if err != nil {
		return err
	}
	if !statDir(m.conv.Dir) {
		return apperr.Failure(apperr.CodeFolderGone, fmt.Sprintf("conversation %s was in %s, which no longer exists", m.conv.ID, m.conv.Dir)).
			WithHint("mkdir -p " + m.conv.Dir + "   (recreate it to resume there)")
	}
	agentID := m.home.agent.ID()
	from := "conversation " + m.conv.ID
	if o.Profile == "" {
		rs := &resolve.Resolver{Cfg: cfg, Agents: reg, Lookup: a.d.Lookup, UserHome: a.d.Host.UserHome, FoldOS: a.foldOS}
		name, src, err := rs.ProfileForHome(agentID, m.home.dir, a.askSharedHome(ctx, cfg.Config, agentID, m.home.dir, o.DryRun))
		if err != nil {
			return err
		}
		o.Profile, o.ProfileFrom = name, src
	}
	o.Agent, o.Dir, o.ResumeID = agentID, m.conv.Dir, m.conv.ID
	o.AgentFrom, o.HomeFrom = from, from+" (stored there)"
	return nil
}

// checkResumeFlags validates the selection flags and refuses two choices
// of the conversation in one command.
func checkResumeFlags(o *LaunchOptions) error {
	r := &o.Resume
	if r.Picker == "" {
		r.Picker = ResumePickerChat
	}
	if r.Scope == "" {
		r.Scope = ResumeScopeFolder
	}
	switch {
	case !slices.Contains(ResumePickerValues(), r.Picker):
		return apperr.Usage(fmt.Sprintf("--picker %q: use one of %s", r.Picker, strings.Join(ResumePickerValues(), ", ")))
	case !slices.Contains(ResumeScopeValues(), r.Scope):
		return apperr.Usage(fmt.Sprintf("--scope %q: use one of %s", r.Scope, strings.Join(ResumeScopeValues(), ", ")))
	case r.Limit < 0:
		return apperr.Usage(fmt.Sprintf("--limit %d: must be 0 (no cap) or more", r.Limit))
	case o.ResumeID != "" && r.Last:
		return apperr.Conflict("--id and --last both choose the conversation; pick one")
	case o.ResumeID != "" && r.Query != "":
		return apperr.Conflict("--id and --query both choose the conversation; pick one")
	case o.ResumeID != "" && r.Picker == ResumePickerFolder:
		return apperr.Conflict("--id and --picker folder both choose the conversation; pick one")
	case r.Last && r.Picker == ResumePickerFolder:
		return apperr.Conflict("--last and --picker folder both choose the conversation; pick one")
	}
	return nil
}

// resumeWhat is the action named in a resume refusal ("mycli cannot
// resume conversations: <agentio's reason>").
const resumeWhat = "resume conversations"

// resumeHomes lists the configured homes to search: every profile's home
// whose agent can list conversations, once per (agent, folder), narrowed
// by --profile (its home only) and --agent. skipped names the agents with
// profiles but without the capability, with agentio's reason.
//
// An agent that cannot list conversations (a config-defined agent: a spec
// has no conversation reader) is refused up front, with agentio's typed
// reason, when the command chose it (--agent, or --profile of that agent)
// or when no searchable home is left at all. Searching nothing and then
// saying "not found in 0 configured home(s)" hid the real cause. Unchosen
// next to a searchable agent, it is only named in the not-found hint, the
// same rule as conversation list.
func (a *App) resumeHomes(c config.Config, o *LaunchOptions) ([]*resumeHome, []string, error) {
	reg, err := a.Registry()
	if err != nil {
		return nil, nil, err
	}
	if o.Profile != "" {
		p, ok := c.Profiles[o.Profile]
		if !ok {
			return nil, nil, apperr.Failure(apperr.CodeNotFound, "unknown profile "+o.Profile).WithHint("agx profile list")
		}
		if o.Agent != "" && o.Agent != p.Agent {
			return nil, nil, apperr.Conflict(fmt.Sprintf("--agent %s and --profile %s (a %s profile) disagree; pick one", o.Agent, o.Profile, p.Agent))
		}
	}
	var homes []*resumeHome
	var skipped []string
	var cannot []agentio.Agent
	for _, ag := range reg.All() {
		id := ag.ID()
		if o.Agent != "" && id != o.Agent {
			continue
		}
		names := resolve.ProfilesOf(c, id)
		if o.Profile != "" {
			names = slices.DeleteFunc(names, func(n string) bool { return n != o.Profile })
		}
		if len(names) == 0 {
			continue
		}
		lister, ok := agentio.ConversationsOf(ag)
		if !ok {
			if o.Agent != "" || o.Profile != "" {
				return nil, nil, convUnsupported(ag, agentio.CapConversations, resumeWhat)
			}
			reason, _ := agentio.Unsupported(ag, agentio.CapConversations)
			skipped = append(skipped, fmt.Sprintf("%s (%s)", id, reason))
			cannot = append(cannot, ag)
			continue
		}
		byDir := map[string]*resumeHome{}
		for _, n := range names {
			dir := filepath.Clean(config.ExpandPath(c.Profiles[n].Home, a.d.Lookup, a.d.Host.UserHome))
			if h, ok := byDir[dir]; ok {
				h.profiles = append(h.profiles, n)
				continue
			}
			h := &resumeHome{agent: ag, lister: lister, home: resolve.HomeOf(ag, dir, n), dir: dir, profiles: []string{n}}
			byDir[dir] = h
			homes = append(homes, h)
		}
	}
	if len(homes) == 0 && len(cannot) > 0 {
		parts := make([]string, len(cannot))
		for i, ag := range cannot {
			parts[i] = convUnsupported(ag, agentio.CapConversations, resumeWhat).Error()
		}
		return nil, nil, apperr.Unsupported(strings.Join(parts, "; "))
	}
	return homes, skipped, nil
}

// resumeByID finds conversation id: an exact id in any home first, else a
// unique id prefix (spec 13.1). Several matches are refused (edge 26);
// none is refused naming what was not searched and the homes that could
// not be read.
//
// A home whose exact lookup fails for another reason than "not found" is
// not fatal: agentio's codex lookup reports a plain codex error for a
// missing id (library gap, see the launch worker's requests), and one
// unreadable home must not block resuming a chat stored in another. The
// prefix listing runs for every home, and its failures are named only
// when nothing matched.
func (a *App) resumeByID(ctx context.Context, homes []*resumeHome, skipped []string, id string) (resumeMatch, error) {
	var found []resumeMatch
	for _, h := range homes {
		if err := ctxErr(ctx); err != nil {
			return resumeMatch{}, err
		}
		if c, err := h.lister.Conversation(h.home, id); err == nil {
			found = append(found, resumeMatch{conv: c, home: h})
		}
	}
	var unreadable []string
	if len(found) == 0 {
		for _, h := range homes {
			if err := ctxErr(ctx); err != nil {
				return resumeMatch{}, err
			}
			convs, err := h.lister.Conversations(ctx, h.home, agentio.ConversationQuery{})
			if err != nil {
				unreadable = append(unreadable, fmt.Sprintf("%s %s (%s)", h.agent.ID(), a.tilde(h.dir), apperr.Text(err)))
				continue
			}
			for _, c := range convs {
				if strings.HasPrefix(c.ID, id) {
					found = append(found, resumeMatch{conv: c, home: h})
				}
			}
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return resumeMatch{}, a.resumeNotFound(homes, skipped, unreadable, "conversation "+id+" not found")
	}
	var parts []string
	homesHit := map[string]bool{}
	for _, m := range found {
		homesHit[m.home.dir] = true
		parts = append(parts, fmt.Sprintf("%s in %s (--profile %s)", m.conv.ID, a.tilde(m.home.dir), m.home.profiles[0]))
	}
	advice := "add --profile"
	if len(homesHit) == 1 {
		advice = "give more of the id"
	}
	return resumeMatch{}, apperr.New(apperr.CodeAmbiguous, apperr.ClassUsage,
		fmt.Sprintf("%s matches %d conversations: %s; %s", id, len(found), strings.Join(parts, "; "), advice))
}

// resumeNotFound builds the "none" refusal (6.5): it names the homes
// that could not be read, the agents not searched and the homes on disk
// that are not in config.
func (a *App) resumeNotFound(homes []*resumeHome, skipped, unreadable []string, msg string) error {
	var hint []string
	if len(unreadable) > 0 {
		hint = append(hint, "could not read: "+strings.Join(unreadable, "; "))
	}
	if len(skipped) > 0 {
		hint = append(hint, "not searched (no conversations): "+strings.Join(skipped, ", "))
	}
	if extra := a.resumeUnconfiguredHomes(homes); len(extra) > 0 {
		hint = append(hint, "homes not in config: "+strings.Join(extra, ", ")+"   (agx profile import --discovered)")
	}
	return apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("%s in %d configured home(s)", msg, len(homes))).WithHint(strings.Join(hint, "\n"))
}

// resumeUnconfiguredHomes are homes on disk (agentio Homes: marked folders) of
// the searched agents that no profile uses. Disk read only; they are
// named, never searched (no run-time discovery, decision 1). Known is every
// profile's home, not only the searched ones: with --profile the other
// profiles' homes are in config, just not searched.
func (a *App) resumeUnconfiguredHomes(homes []*resumeHome) []string {
	known := a.profileHomeDirs(a.cfgOrEmpty())
	var agents []agentio.Agent
	for _, h := range homes {
		known[h.dir] = true
		if !slices.Contains(agents, h.agent) {
			agents = append(agents, h.agent)
		}
	}
	var out []string
	for _, ag := range agents {
		hs, err := ag.Homes()
		if err != nil {
			continue // naming extra homes is a hint; an unreadable folder only drops it
		}
		for _, h := range hs {
			if d := filepath.Clean(h.Dir()); !known[d] {
				known[d] = true
				out = append(out, a.tilde(d))
			}
		}
	}
	return out
}

// resumeByList chooses from the conversations in scope: the newest with
// --last, else through the picker (chat, or folder then chat).
func (a *App) resumeByList(ctx context.Context, homes []*resumeHome, o *LaunchOptions) (resumeMatch, error) {
	r := o.Resume
	dir := ""
	if r.Scope == ResumeScopeFolder && r.Picker == ResumePickerChat {
		dir = a.d.Cwd
	}
	if !r.Last && a.d.Picker == nil {
		return resumeMatch{}, picker.NoTTY("pass --id or --last")
	}
	all, unreadable, err := a.resumeList(ctx, homes, dir, r.Limit, r.Query)
	if err != nil {
		return resumeMatch{}, err
	}
	if len(all) == 0 {
		where := "anywhere"
		if dir != "" {
			where = "in " + a.tilde(dir)
		}
		if r.Query != "" {
			where += " matching " + quoteText(r.Query)
		}
		e := apperr.Failure(apperr.CodeNotFound, "no conversations "+where)
		var hint []string
		if dir != "" {
			hint = append(hint, "agx resume --scope all")
		}
		if len(unreadable) > 0 {
			hint = append(hint, "could not read: "+strings.Join(unreadable, "; "))
		}
		e = e.WithHint(strings.Join(hint, "\n"))
		return resumeMatch{}, e
	}
	if r.Last {
		return all[0], nil
	}
	if r.Picker == ResumePickerFolder {
		var dirs []string
		for _, m := range all {
			if !slices.Contains(dirs, m.conv.Dir) {
				dirs = append(dirs, m.conv.Dir)
			}
		}
		items := make([]picker.Item, len(dirs))
		for i, d := range dirs {
			items[i] = picker.Item{Label: a.tilde(d)}
		}
		i, err := a.d.Picker.Choose(ctx, "Resume a chat from which folder?", items)
		if err != nil {
			return resumeMatch{}, err
		}
		all = slices.DeleteFunc(all, func(m resumeMatch) bool { return m.conv.Dir != dirs[i] })
	}
	items := make([]picker.Item, len(all))
	for i, m := range all {
		items[i] = a.resumeItem(m)
	}
	i, err := a.d.Picker.Choose(ctx, "Resume which conversation?", items)
	if err != nil {
		return resumeMatch{}, err
	}
	return all[i], nil
}

// resumeList merges the conversations of every home, newest first,
// capped at limit after the query filter (0 = no cap, as in conversation
// list and agentio's ConversationQuery). A home that cannot be listed is
// skipped and named in unreadable (shown when nothing is found).
func (a *App) resumeList(ctx context.Context, homes []*resumeHome, dir string, limit int, query string) ([]resumeMatch, []string, error) {
	var all []resumeMatch
	var unreadable []string
	q := agentio.ConversationQuery{Dir: dir}
	if query == "" {
		q.Limit = limit
	}
	for _, h := range homes {
		if err := ctxErr(ctx); err != nil {
			return nil, nil, err
		}
		convs, err := h.lister.Conversations(ctx, h.home, q)
		if err != nil {
			unreadable = append(unreadable, fmt.Sprintf("%s %s (%s)", h.agent.ID(), a.tilde(h.dir), apperr.Text(err)))
			continue
		}
		for _, c := range convs {
			if query == "" || resumeMatchesQuery(c, query) {
				all = append(all, resumeMatch{conv: c, home: h})
			}
		}
	}
	slices.SortStableFunc(all, func(x, y resumeMatch) int { return y.conv.Updated.Compare(x.conv.Updated) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, unreadable, nil
}

// resumeMatchesQuery is --query: a case-insensitive substring of the title,
// the folder or the id.
func resumeMatchesQuery(c agentio.Conversation, query string) bool {
	q := strings.ToLower(query)
	for _, s := range []string{c.Title, c.Dir, c.ID} {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return false
}

// resumeItem is one picker row, v0.3's conversation line: when, agent,
// profile, title, folder and the id prefix (internal/human).
func (a *App) resumeItem(m resumeMatch) picker.Item {
	return picker.Item{Label: human.ConversationLine(human.Conversation{
		Updated: m.conv.Updated, Agent: string(m.home.agent.ID()), Profile: strings.Join(m.home.profiles, ","),
		Title: m.conv.Title, Dir: a.tilde(m.conv.Dir), ID: m.conv.ID,
	}, a.Now())}
}

// quoteText wraps s in double quotes for messages.
func quoteText(s string) string { return `"` + s + `"` }

// askSharedHome is the terminal question of the shared-home rule (6.2
// step 3, edge 27): which profile of the home, and whether to remember
// it as "default". nil without a terminal (edge 28 refuses then).
// Remembering is offered only when the agent has no default profile yet:
// "default" also marks the agent's default profile, and two defaults for
// one agent is a config error (edge 5). A dry run asks but never writes.
func (a *App) askSharedHome(ctx context.Context, c config.Config, id agentio.ID, dir string, dry bool) func(names []string) (string, error) {
	if a.d.Picker == nil {
		return nil
	}
	return func(names []string) (string, error) {
		items := make([]picker.Item, len(names))
		for i, n := range names {
			items[i] = picker.Item{Label: n, Detail: c.Profiles[n].Billing}
		}
		i, err := a.d.Picker.Choose(ctx, fmt.Sprintf("%s has %d profiles and no default. Resume on which?", a.tilde(dir), len(names)), items)
		if err != nil {
			return "", err
		}
		name := names[i]
		if dry || resolve.DefaultProfile(c, id) != "" {
			return name, nil
		}
		ok, err := a.d.Picker.Confirm(ctx, fmt.Sprintf("Remember %s as the default profile of %s?", name, id), false)
		if err != nil || !ok {
			return name, err
		}
		if err := config.EditConfig(ctx, a.paths, func(d *jsonc.Doc) error {
			return d.Set(config.Pointer("profiles", name, "default"), true)
		}); err != nil {
			return "", err
		}
		a.Reload()
		return name, nil
	}
}
