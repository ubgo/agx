package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
)

// Scope values (--scope, spec 5.2): the current folder by default, every
// folder only when asked (memory note picker-scope-current-folder: never
// widen on its own).
const (
	ScopeFolder = "folder"
	ScopeAll    = "all"
)

// ScopeValues is the canonical list of --scope values.
func ScopeValues() []string { return []string{ScopeFolder, ScopeAll} }

// Picker values for conversation move (--picker, spec 5.3): tick chats, or
// choose a folder first and then tick its chats.
const (
	PickerChat   = "chat"
	PickerFolder = "folder"
)

// PickerValues is the canonical list of --picker values.
func PickerValues() []string { return []string{PickerChat, PickerFolder} }

// DefaultConversationLimit is --limit's default (spec 13.1 resume, shared
// by conversation list). 0 given on the command line means no cap.
const DefaultConversationLimit = 50

// Warning codes of the conversation and session commands.
const (
	// WarnHomeMissing is a profile whose home folder is gone (skipped).
	WarnHomeMissing = "home-missing"
	// WarnHomeFailed is a home whose conversations could not be read.
	WarnHomeFailed = "home-failed"
	// WarnHistoryLeft is history that stays keyed to the old folder after
	// session promote --no-codex.
	WarnHistoryLeft = "history-left"
)

// homeRef is one home agx may read: its agent, the home, and the profiles
// configured on it (none for a home found on disk but not in config).
type homeRef struct {
	agent    agentio.Agent
	home     agentio.Home
	profiles []string
}

// homeFilter narrows configuredHomes: one agent and/or some profiles.
type homeFilter struct {
	agent    agentio.ID
	profiles []string
}

// configuredHomes lists the homes of the configured profiles, one entry per
// (agent, folder) with every profile on it, in registry then path order.
// A profile whose home is gone is skipped with a warning. Unknown names in
// the filter are refused (a flag always names a real entry, principle 13).
func (a *App) configuredHomes(f homeFilter) ([]homeRef, []report.Warning, error) {
	cfg, err := a.Config()
	if err != nil {
		return nil, nil, err
	}
	reg, err := a.Registry()
	if err != nil {
		return nil, nil, err
	}
	if f.agent != "" {
		if _, ok := reg.Get(f.agent); !ok {
			return nil, nil, apperr.Usage(fmt.Sprintf("unknown agent %q; agents: %s", f.agent, joinAgentIDs(reg.IDs())))
		}
	}
	for _, name := range f.profiles {
		p, ok := cfg.Config.Profiles[name]
		if !ok {
			return nil, nil, apperr.Failure(apperr.CodeNotFound, "no profile named "+name).WithHint("agx profile list")
		}
		if f.agent != "" && p.Agent != f.agent {
			return nil, nil, apperr.Conflict(fmt.Sprintf("--profile %s is a %s profile, not %s; pick one", name, p.Agent, f.agent))
		}
	}
	var out []homeRef
	var warns []report.Warning
	byKey := map[string]int{}
	for _, ag := range reg.All() {
		if f.agent != "" && ag.ID() != f.agent {
			continue
		}
		for _, name := range resolve.ProfilesOf(cfg.Config, ag.ID()) {
			if len(f.profiles) > 0 && !slices.Contains(f.profiles, name) {
				continue
			}
			dir := filepath.Clean(config.ExpandPath(cfg.Config.Profiles[name].Home, a.d.Lookup, a.d.Host.UserHome))
			if !statDir(dir) {
				warns = append(warns, report.Warning{Code: WarnHomeMissing, Message: fmt.Sprintf("profile %s: home %s does not exist; skipped", name, a.tilde(dir))})
				continue
			}
			key := string(ag.ID()) + "\x00" + dir
			if i, ok := byKey[key]; ok {
				out[i].profiles = append(out[i].profiles, name)
				continue
			}
			byKey[key] = len(out)
			out = append(out, homeRef{agent: ag, home: resolve.HomeOf(ag, dir, name), profiles: []string{name}})
		}
	}
	return out, warns, nil
}

// discoveredHomes lists homes agentio finds on disk (default home and
// extra-home patterns, with markers) that no profile uses, for the footer
// of conversation list (spec 6.2) and for the safety checks of session
// clean and promote. They are reported or checked, never used to launch
// (no run-time discovery, decision 1).
//
// known are the homes the caller already has; a folder that any profile
// uses is skipped too, so a command narrowed by --profile never reports
// another profile's home as "not in config".
func (a *App) discoveredHomes(agentFilter agentio.ID, known []homeRef) []homeRef {
	reg, err := a.Registry()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, h := range known {
		seen[string(h.agent.ID())+"\x00"+h.home.Dir()] = true
	}
	inConfig := a.profileHomeDirs(a.cfgOrEmpty())
	var out []homeRef
	for _, ag := range reg.All() {
		if agentFilter != "" && ag.ID() != agentFilter {
			continue
		}
		homes, err := ag.Homes()
		if err != nil {
			continue // a home pattern that cannot be read finds nothing to report
		}
		for _, h := range homes {
			key := string(ag.ID()) + "\x00" + filepath.Clean(h.Dir())
			if seen[key] || inConfig[filepath.Clean(h.Dir())] {
				continue
			}
			seen[key] = true
			out = append(out, homeRef{agent: ag, home: h})
		}
	}
	return out
}

// profileHomeDirs is the set of home folders (expanded and cleaned) of
// every profile in c, whatever filter the command applied. "Not in config"
// means no profile uses the folder at all: the footer of usage and
// conversation list and the resume hint read this one set, so a --profile
// filter never makes another profile's home look unconfigured.
func (a *App) profileHomeDirs(c config.Config) map[string]bool {
	used := make(map[string]bool, len(c.Profiles))
	for _, p := range c.Profiles {
		used[filepath.Clean(config.ExpandPath(p.Home, a.d.Lookup, a.d.Host.UserHome))] = true
	}
	return used
}

// homesFooter is the footer naming homes not in config (spec 6.2): one
// warning per home, worded and coded as agx usage's footer.
func (a *App) homesFooter(homes []homeRef) []report.Warning {
	var out []report.Warning
	for _, h := range homes {
		out = append(out, report.Warning{Code: WarnUnconfiguredHome, Message: fmt.Sprintf(
			"%s home %s is not in config; agx profile import --discovered", h.agent.ID(), a.tilde(h.home.Dir()))})
	}
	return out
}

// ConversationListOptions are the flags of agx conversation list.
type ConversationListOptions struct {
	// Query narrows by a case-insensitive substring of title, folder or id.
	Query string
	// Scope is folder (the current folder, default) or all.
	Scope string
	// ByFolder groups the result by folder.
	ByFolder bool
	// Limit caps the conversations considered, newest first; 0 = no cap.
	Limit int
	// Agent keeps one agent's homes.
	Agent agentio.ID
	// Profiles keeps these profiles' homes (repeat).
	Profiles []string
}

// ConversationRow is one conversation of conversation list.
type ConversationRow struct {
	Agent agentio.ID `json:"agent"`
	// Profiles are every profile on the home that stores it.
	Profiles []string `json:"profiles"`
	// Home is the home folder (~ for the user home).
	Home    string    `json:"home"`
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Folder  string    `json:"folder"`
	Created time.Time `json:"created,omitzero"`
	// Updated is the transcript's last change (agentio: its mtime).
	Updated time.Time `json:"updated"`
}

// FolderRow is one folder of conversation list --by-folder.
type FolderRow struct {
	Folder        string       `json:"folder"`
	Conversations int          `json:"conversations"`
	Updated       time.Time    `json:"updated"`
	Agents        []agentio.ID `json:"agents"`
	Profiles      []string     `json:"profiles"`
}

// ConversationList is the result of conversation list: Rows always, and
// Folders with --by-folder.
type ConversationList struct {
	Rows     []ConversationRow
	Folders  []FolderRow
	Warnings []report.Warning
}

// conversationDir is the folder a folder-scoped command works on: dir
// (or the current folder) made absolute and cleaned, with symlinks
// resolved when possible, because agents record the real path. It is not
// case-folded: Claude encodes the exact bytes (memory note
// claude-history-path-encoding).
func (a *App) conversationDir(dir string) string {
	if dir == "" {
		dir = a.d.Cwd
	}
	dir = config.ExpandPath(dir, a.d.Lookup, a.d.Host.UserHome)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(a.d.Cwd, dir)
	}
	dir = filepath.Clean(dir)
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

// conversationDirs are the spellings an agent may have recorded dir under:
// its real path (symlinks resolved, what getcwd gives a process started
// there) first, then the path as given when it differs (a folder opened
// through a symlinked projects folder, or macOS /var -> /private/var).
// Folder-scoped lookups try each, so a symlink never hides history.
func (a *App) conversationDirs(dir string) []string {
	real := a.conversationDir(dir)
	given := dir
	if given == "" {
		given = a.d.Cwd
	}
	given = config.ExpandPath(given, a.d.Lookup, a.d.Host.UserHome)
	if !filepath.IsAbs(given) {
		given = filepath.Join(a.d.Cwd, given)
	}
	given = filepath.Clean(given)
	if given == real {
		return []string{real}
	}
	return []string{real, given}
}

// convCheckScope validates --scope ("" = folder).
func convCheckScope(s string) (string, error) {
	if s == "" {
		return ScopeFolder, nil
	}
	if !slices.Contains(ScopeValues(), s) {
		return "", apperr.Usage(fmt.Sprintf("--scope %q: use %s", s, strings.Join(ScopeValues(), " or ")))
	}
	return s, nil
}

// convUnsupported is the exit-4 refusal carrying the agent's own reason.
func convUnsupported(ag agentio.Agent, c agentio.Capability, what string) error {
	reason, declared := agentio.Unsupported(ag, c)
	if !declared || reason == "" {
		reason = "it has no " + c.String() + " capability"
	}
	return apperr.Unsupported(fmt.Sprintf("%s cannot %s: %s", ag.ID(), what, reason))
}

// ConversationList lists conversations of the configured homes, newest
// first: the current folder's by default, every folder with --scope all.
// Disk · Read; transcripts are read at head and tail only (agentio).
func (a *App) ConversationList(ctx context.Context, o ConversationListOptions) (ConversationList, error) {
	var res ConversationList
	scope, err := convCheckScope(o.Scope)
	if err != nil {
		return res, err
	}
	if o.Limit < 0 {
		return res, apperr.Usage("--limit must be 0 (no cap) or more")
	}
	homes, warns, err := a.configuredHomes(homeFilter{agent: o.Agent, profiles: o.Profiles})
	if err != nil {
		return res, err
	}
	res.Warnings = warns
	q := agentio.ConversationQuery{}
	dirs := []string{""}
	if scope == ScopeFolder {
		dirs = a.conversationDirs("")
	}
	// The query filters after the read, so each home must give all its
	// conversations; without one, each home's newest Limit are enough.
	if o.Query == "" {
		q.Limit = o.Limit
	}
	needle := strings.ToLower(o.Query)
	var failed []string
	read := 0
	for _, h := range homes {
		cl, ok := agentio.ConversationsOf(h.agent)
		if !ok {
			if o.Agent != "" || len(o.Profiles) > 0 {
				return res, convUnsupported(h.agent, agentio.CapConversations, "list conversations")
			}
			continue
		}
		convs, err := listDirs(ctx, cl, h.home, q, dirs)
		if err != nil {
			if cerr := ctxErr(ctx); cerr != nil {
				return res, cerr
			}
			failed = append(failed, fmt.Sprintf("%s %s: %s", h.agent.ID(), a.tilde(h.home.Dir()), apperr.Text(err)))
			res.Warnings = append(res.Warnings, report.Warning{Code: WarnHomeFailed, Message: failed[len(failed)-1]})
			continue
		}
		read++
		for _, c := range convs {
			row := a.conversationRow(h, c)
			if needle != "" && !matchesQuery(row, needle) {
				continue
			}
			res.Rows = append(res.Rows, row)
		}
	}
	sort.SliceStable(res.Rows, func(i, j int) bool { return res.Rows[i].Updated.After(res.Rows[j].Updated) })
	if o.Limit > 0 && len(res.Rows) > o.Limit {
		res.Rows = res.Rows[:o.Limit]
	}
	if res.Rows == nil {
		res.Rows = []ConversationRow{}
	}
	if o.ByFolder {
		res.Folders = byFolder(res.Rows)
	}
	res.Warnings = append(res.Warnings, a.homesFooter(a.discoveredHomes(o.Agent, homes))...)
	return res, listOutcome(read, failed)
}

// listOutcome is nil when every home answered, partial (exit 3, rows
// kept) when some failed, and a failure when none answered.
func listOutcome(read int, failed []string) error {
	switch {
	case len(failed) == 0:
		return nil
	case read > 0:
		return apperr.New(apperr.CodePartial, apperr.ClassPartial, fmt.Sprintf("%d home(s) could not be read: %s", len(failed), strings.Join(failed, "; ")))
	}
	return apperr.Failure(apperr.CodeIO, "no home could be read: "+strings.Join(failed, "; "))
}

// conversationRow turns an agentio conversation into a row.
func (a *App) conversationRow(h homeRef, c agentio.Conversation) ConversationRow {
	return ConversationRow{
		Agent: h.agent.ID(), Profiles: slices.Clone(h.profiles), Home: a.tilde(h.home.Dir()),
		ID: c.ID, Title: c.Title, Folder: c.Dir, Created: c.Created, Updated: c.Updated,
	}
}

// matchesQuery reports a row whose title, folder or id holds needle
// (already lowercased).
func matchesQuery(r ConversationRow, needle string) bool {
	for _, s := range []string{r.Title, r.Folder, r.ID} {
		if strings.Contains(strings.ToLower(s), needle) {
			return true
		}
	}
	return false
}

// byFolder groups rows (newest first) by folder, newest folder first.
func byFolder(rows []ConversationRow) []FolderRow {
	idx := map[string]int{}
	out := []FolderRow{}
	for _, r := range rows {
		i, ok := idx[r.Folder]
		if !ok {
			i = len(out)
			idx[r.Folder] = i
			out = append(out, FolderRow{Folder: r.Folder, Updated: r.Updated, Agents: []agentio.ID{}, Profiles: []string{}})
		}
		f := &out[i]
		f.Conversations++
		if !slices.Contains(f.Agents, r.Agent) {
			f.Agents = append(f.Agents, r.Agent)
		}
		for _, p := range r.Profiles {
			if !slices.Contains(f.Profiles, p) {
				f.Profiles = append(f.Profiles, p)
			}
		}
	}
	return out
}

// ConversationDetail is the result of conversation show.
type ConversationDetail struct {
	ConversationRow
	// Path is the transcript file.
	Path string `json:"path"`
	// SizeBytes is the transcript's size; 0 when it cannot be read.
	SizeBytes int64 `json:"size_bytes"`
	// Turns counts user and assistant turns; null when the agent cannot
	// read turns.
	Turns *int `json:"turns"`
	// Live says whether an agent process runs the conversation now; null
	// when the agent cannot tell.
	Live *bool `json:"live"`
}

// ConversationShow finds one conversation by exact id or unique prefix in
// every configured home and describes it. Disk · Read; counting turns
// streams the transcript once, line by line (agentio TurnReader), and is
// the only full read of a transcript agx makes, on this explicit request.
func (a *App) ConversationShow(ctx context.Context, id string) (ConversationDetail, error) {
	var d ConversationDetail
	if id == "" {
		return d, apperr.Usage("--id is required")
	}
	homes, _, err := a.configuredHomes(homeFilter{})
	if err != nil {
		return d, err
	}
	h, c, err := a.findConversation(ctx, homes, id)
	if err != nil {
		return d, err
	}
	d.ConversationRow = a.conversationRow(h, c)
	d.Path = c.Path
	if st, err := os.Stat(c.Path); err == nil {
		d.SizeBytes = st.Size()
	}
	if tr, ok := agentio.TurnsOf(h.agent); ok {
		n := 0
		if _, err := tr.Turns(h.home, c.ID, 0, func(agentio.Turn) error {
			n++
			return ctxErr(ctx)
		}); err != nil {
			return d, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("read turns of %s: %s", c.ID, err.Error()))
		}
		d.Turns = &n
	}
	if ls, ok := agentio.LiveSessionsOf(h.agent); ok {
		live, err := ls.IsLive(h.home, c.ID)
		if err != nil {
			return d, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("live sessions of %s: %s", a.tilde(h.home.Dir()), apperr.Text(err)))
		}
		d.Live = &live
	}
	return d, nil
}

// conversationMatch is one conversation found for an id.
type conversationMatch struct {
	home homeRef
	conv agentio.Conversation
}

// findConversation finds id in homes: an exact id first (cheap lookup per
// home), else a unique prefix over every conversation. Two matches are
// ambiguous (exit 2, edge 26 for show); none is not-found.
func (a *App) findConversation(ctx context.Context, homes []homeRef, id string) (homeRef, agentio.Conversation, error) {
	var exact, prefix []conversationMatch
	for _, h := range homes {
		cl, ok := agentio.ConversationsOf(h.agent)
		if !ok {
			continue
		}
		// Any lookup error falls through to the prefix scan: Codex's
		// not-found does not wrap agentio.ErrNotFound yet (requested), and
		// a real read problem shows up there as a home with no match.
		if c, err := cl.Conversation(h.home, id); err == nil {
			exact = append(exact, conversationMatch{h, c})
			continue
		}
		all, err := cl.Conversations(ctx, h.home, agentio.ConversationQuery{})
		if err != nil {
			if cerr := ctxErr(ctx); cerr != nil {
				return homeRef{}, agentio.Conversation{}, cerr
			}
			continue // an unreadable home holds no match we could resume anyway
		}
		for _, c := range all {
			if strings.HasPrefix(c.ID, id) {
				prefix = append(prefix, conversationMatch{h, c})
			}
		}
	}
	found := exact
	if len(found) == 0 {
		found = prefix
	}
	switch len(found) {
	case 0:
		return homeRef{}, agentio.Conversation{}, apperr.Failure(apperr.CodeNotFound, "no conversation "+id+" in any configured home").
			WithHint("agx conversation list --scope all")
	case 1:
		return found[0].home, found[0].conv, nil
	}
	lines := make([]string, len(found))
	for i, m := range found {
		lines[i] = fmt.Sprintf("%s %s %s %s", m.home.agent.ID(), a.tilde(m.home.home.Dir()), m.conv.ID, m.conv.Dir)
	}
	return homeRef{}, agentio.Conversation{}, apperr.New(apperr.CodeAmbiguous, apperr.ClassUsage,
		fmt.Sprintf("%s matches %d conversations; give more of the id", id, len(found))).WithHint(strings.Join(lines, "\n"))
}

// listDirs lists conversations of home for each folder spelling in dirs
// ("" = every folder), newest first, each conversation once.
func listDirs(ctx context.Context, cl agentio.ConversationLister, h agentio.Home, q agentio.ConversationQuery, dirs []string) ([]agentio.Conversation, error) {
	var out []agentio.Conversation
	seen := map[string]bool{}
	for _, d := range dirs {
		q.Dir = d
		convs, err := cl.Conversations(ctx, h, q)
		if err != nil {
			return nil, err
		}
		for _, c := range convs {
			if !seen[c.ID] {
				seen[c.ID] = true
				out = append(out, c)
			}
		}
	}
	if len(dirs) > 1 {
		slices.SortStableFunc(out, func(x, y agentio.Conversation) int { return y.Updated.Compare(x.Updated) })
		if q.Limit > 0 && len(out) > q.Limit {
			out = out[:q.Limit]
		}
	}
	return out, nil
}
