package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/picker"
	"github.com/ubgo/agx/sessions"
)

// ConversationMoveOptions are the flags of agx conversation move.
type ConversationMoveOptions struct {
	// Dir is the folder whose conversations move ("" = the current one).
	Dir string
	// DirSet is true when --dir was given (it conflicts with --scope all
	// and --picker folder, which choose the folder otherwise).
	DirSet bool
	// ToProfile names the target home; required.
	ToProfile string
	// FromProfile names the source home; "" = the one other home of the
	// agent that has history for the folder.
	FromProfile string
	// IDs limits the move to these conversations (exact or unique prefix).
	IDs []string
	// Picker is "" (none), chat or folder.
	Picker string
	// Scope is folder (default) or all.
	Scope string
	// NoBackup discards agentio's backup after a successful move.
	NoBackup bool
	// DryRun prints the plan and changes nothing.
	DryRun bool
}

// MoveSide is one end of a move: the home and its profiles.
type MoveSide struct {
	Profiles []string `json:"profiles"`
	Home     string   `json:"home"`
}

// MoveFolder is the plan (and outcome) for one folder.
type MoveFolder struct {
	Folder string `json:"folder"`
	// Conversations are the ids that move, newest first.
	Conversations []string `json:"conversations"`
	// Items are the files and folders renamed.
	Items []agentio.MoveItem `json:"items"`
	// Left are entries of the folder's history that do not move.
	Left []string `json:"left"`
	// Blockers stop the move before any file is touched.
	Blockers []string `json:"blockers"`
}

// ConversationMoveResult is the result of conversation move.
type ConversationMoveResult struct {
	Agent   agentio.ID   `json:"agent"`
	From    MoveSide     `json:"from"`
	To      MoveSide     `json:"to"`
	DryRun  bool         `json:"dry_run"`
	Folders []MoveFolder `json:"folders"`
	// Backups are the copies agentio saved and agx kept; empty on a dry
	// run or with --no-backup.
	Backups []string `json:"backups"`
}

// moveRequest is a resolved conversation move: the agent's mover and the
// two homes.
type moveRequest struct {
	ag       agentio.Agent
	mover    agentio.ConversationMover
	from, to homeRef
}

// ConversationMove moves a folder's conversations to another home of the
// same agent (spec 13.2), only through agentio's ConversationMover: an
// agent without one is refused with its own reason (exit 4). A running
// conversation, or any other plan blocker, stops the move before a file
// is touched. Check order, most useful message first: the request (flags,
// profiles, same agent, same home), then support, then the homes, then the
// folder. Disk · Write unless --dry-run.
func (a *App) ConversationMove(ctx context.Context, o ConversationMoveOptions) (ConversationMoveResult, error) {
	var res ConversationMoveResult
	if err := checkMoveFlags(&o); err != nil {
		return res, err
	}
	req, err := a.moveHomes(ctx, o)
	if err != nil {
		return res, err
	}
	res.Agent = req.ag.ID()
	res.From = MoveSide{Profiles: slices.Clone(req.from.profiles), Home: a.tilde(req.from.home.Dir())}
	res.To = MoveSide{Profiles: slices.Clone(req.to.profiles), Home: a.tilde(req.to.home.Dir())}
	res.DryRun = o.DryRun
	res.Folders, res.Backups = []MoveFolder{}, []string{}
	groups, err := a.moveSelection(ctx, o, req)
	if err != nil {
		return res, err
	}
	var plans []agentio.MovePlan
	var blockers []string
	for _, g := range groups {
		plan, err := req.mover.PlanMove(agentio.MoveRequest{From: req.from.home, To: req.to.home, Dir: g.dir, OnlyIDs: g.ids})
		if err != nil {
			return res, a.planMoveError(err, g.dir)
		}
		if len(plan.Conversations) == 0 {
			continue
		}
		plan.Blockers = append(plan.Blockers, a.liveBlockers(req.ag, plan, req.from.home, req.to.home)...)
		blockers = append(blockers, plan.Blockers...)
		plans = append(plans, plan)
		res.Folders = append(res.Folders, moveFolderOf(plan))
	}
	if len(plans) == 0 {
		return res, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("no %s conversations to move from %s", req.ag.ID(), a.tilde(req.from.home.Dir())))
	}
	if len(blockers) > 0 {
		return res, apperr.Failure(apperr.CodeMoveBlocked, "move blocked, nothing changed: "+strings.Join(blockers, "; "))
	}
	if o.DryRun {
		return res, nil
	}
	for _, plan := range plans {
		b, err := req.mover.ApplyMove(plan)
		if err != nil {
			return res, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("move %s: %s (that folder's files are back where they were)", plan.Dir, err.Error()))
		}
		if o.NoBackup {
			if err := sessions.DiscardBackup(b); err != nil {
				return res, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "moved, but the backup could not be discarded: "+err.Error())
			}
			continue
		}
		for _, e := range b.Entries {
			res.Backups = append(res.Backups, a.tilde(e.Copy))
		}
	}
	return res, nil
}

// checkMoveFlags validates values and refuses true conflicts (two choices
// for one concern, principle 3).
func checkMoveFlags(o *ConversationMoveOptions) error {
	scope, err := convCheckScope(o.Scope)
	if err != nil {
		return err
	}
	o.Scope = scope
	switch {
	case o.ToProfile == "":
		return apperr.Usage("--to-profile is required: the profile whose home receives the conversations")
	case o.Picker != "" && !slices.Contains(PickerValues(), o.Picker):
		return apperr.Usage(fmt.Sprintf("--picker %q: use %s", o.Picker, strings.Join(PickerValues(), " or ")))
	case len(o.IDs) > 0 && o.Picker != "":
		return apperr.Conflict("--id and --picker both choose the conversations; pick one")
	case o.DirSet && o.Picker == PickerFolder:
		return apperr.Conflict("--dir and --picker folder both choose the folder; pick one")
	case o.DirSet && o.Scope == ScopeAll:
		return apperr.Conflict("--dir and --scope all both choose the folders; pick one")
	}
	return nil
}

// moveHomes resolves the agent, its mover and both homes.
func (a *App) moveHomes(ctx context.Context, o ConversationMoveOptions) (moveRequest, error) {
	var req moveRequest
	cfg, err := a.Config()
	if err != nil {
		return req, err
	}
	reg, err := a.Registry()
	if err != nil {
		return req, err
	}
	to, ok := cfg.Config.Profiles[o.ToProfile]
	if !ok {
		return req, apperr.Failure(apperr.CodeNotFound, "no profile named "+o.ToProfile).WithHint("agx profile list")
	}
	if o.FromProfile != "" {
		from, ok := cfg.Config.Profiles[o.FromProfile]
		if !ok {
			return req, apperr.Failure(apperr.CodeNotFound, "no profile named "+o.FromProfile).WithHint("agx profile list")
		}
		if from.Agent != to.Agent {
			return req, apperr.Usage(fmt.Sprintf("--from-profile %s is %s and --to-profile %s is %s; conversations move only between homes of one agent", o.FromProfile, from.Agent, o.ToProfile, to.Agent))
		}
	}
	ag, ok := reg.Get(to.Agent)
	if !ok {
		return req, apperr.Failure(apperr.CodeConfig, fmt.Sprintf("profile %s: unknown agent %s", o.ToProfile, to.Agent))
	}
	req.ag = ag
	homes, _, err := a.configuredHomes(homeFilter{agent: ag.ID()})
	if err != nil {
		return req, err
	}
	toRef, ok := homeOfProfile(homes, o.ToProfile)
	if fromRef, fok := homeOfProfile(homes, o.FromProfile); ok && fok && fromRef.home.Dir() == toRef.home.Dir() {
		return req, apperr.Usage(fmt.Sprintf("--from-profile %s and --to-profile %s share the home %s; nothing to move", o.FromProfile, o.ToProfile, a.tilde(toRef.home.Dir())))
	}
	mover, mok := agentio.ConversationMoverOf(ag)
	if !mok {
		return req, convUnsupported(ag, agentio.CapConversationMover, "move conversations between homes")
	}
	req.mover = mover
	if !ok {
		return req, a.homeMissing(o.ToProfile)
	}
	req.to = toRef
	if o.FromProfile != "" {
		fromRef, ok := homeOfProfile(homes, o.FromProfile)
		if !ok {
			return req, a.homeMissing(o.FromProfile)
		}
		req.from = fromRef
		return req, nil
	}
	return a.moveSource(ctx, o, req, homes)
}

// homeOfProfile finds the home a profile is on.
func homeOfProfile(homes []homeRef, profile string) (homeRef, bool) {
	for _, h := range homes {
		if slices.Contains(h.profiles, profile) {
			return h, true
		}
	}
	return homeRef{}, false
}

// homeMissing is the refusal for a profile whose home folder is gone.
func (a *App) homeMissing(profile string) error {
	cfg, _ := a.Config() // loaded already by the caller
	dir := ""
	if cfg != nil {
		dir = a.tilde(cfg.Config.Profiles[profile].Home)
	}
	return apperr.Failure(apperr.CodeHomeMissing, fmt.Sprintf("profile %s: home %s does not exist", profile, dir))
}

// moveSource picks the source home when --from-profile is absent: among
// the agent's other configured homes, the one holding history for the
// folder (scope folder), or the only other home (scope all, picker
// folder). None or several is a refusal naming --from-profile.
func (a *App) moveSource(ctx context.Context, o ConversationMoveOptions, req moveRequest, homes []homeRef) (moveRequest, error) {
	var others []homeRef
	for _, h := range homes {
		if h.home.Dir() != req.to.home.Dir() {
			others = append(others, h)
		}
	}
	perFolder := o.Scope == ScopeFolder && o.Picker != PickerFolder
	if perFolder {
		dirs := a.conversationDirs(o.Dir)
		dir := dirs[0]
		var with []homeRef
		for _, h := range others {
			for _, d := range dirs {
				has, err := a.hasHistory(ctx, h, d)
				if err != nil {
					return req, err
				}
				if has {
					with = append(with, h)
					break
				}
			}
		}
		if len(with) == 0 {
			return req, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("no %s conversations for %s in any other home", req.ag.ID(), dir)).
				WithHint("agx conversation list --scope all --agent " + string(req.ag.ID()))
		}
		others = with
	}
	switch len(others) {
	case 0:
		return req, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("%s has no other configured home to move from", req.ag.ID()))
	case 1:
		req.from = others[0]
		return req, nil
	}
	names := make([]string, len(others))
	for i, h := range others {
		names[i] = strings.Join(h.profiles, "/") + " (" + a.tilde(h.home.Dir()) + ")"
	}
	return req, apperr.New(apperr.CodeAmbiguous, apperr.ClassUsage,
		fmt.Sprintf("%d homes could be the source: %s; add --from-profile", len(others), strings.Join(names, ", ")))
}

// hasHistory asks the agent whether dir has conversations in a home: the
// cheap HistoryChecker, else a one-row listing. An agent with neither has
// no conversations agx can see.
func (a *App) hasHistory(ctx context.Context, h homeRef, dir string) (bool, error) {
	if hc, ok := agentio.HistoryCheckerOf(h.agent); ok {
		has, err := hc.HasHistory(h.home, dir)
		if err != nil {
			return false, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("%s %s: %s", h.agent.ID(), a.tilde(h.home.Dir()), err.Error()))
		}
		return has, nil
	}
	cl, ok := agentio.ConversationsOf(h.agent)
	if !ok {
		return false, nil
	}
	convs, err := cl.Conversations(ctx, h.home, agentio.ConversationQuery{Dir: dir, Limit: 1})
	if err != nil {
		return false, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("%s %s: %s", h.agent.ID(), a.tilde(h.home.Dir()), err.Error()))
	}
	return len(convs) > 0, nil
}

// moveGroup is one folder to plan, with the ids to move (nil = the whole
// folder, which also moves its memory and removes the emptied history).
type moveGroup struct {
	dir string
	ids []string
}

// moveSelection decides which folders and conversations move: the folder
// as a whole by default; --id narrows; --picker asks; --scope all takes
// every folder of the source home.
func (a *App) moveSelection(ctx context.Context, o ConversationMoveOptions, req moveRequest) ([]moveGroup, error) {
	if o.Scope == ScopeFolder && o.Picker == "" {
		// The folder spelling the source home recorded (real or as given).
		dirs := a.conversationDirs(o.Dir)
		for _, d := range dirs {
			if has, err := a.hasHistory(ctx, req.from, d); err == nil && has {
				return []moveGroup{{dir: d, ids: o.IDs}}, nil
			}
		}
		return []moveGroup{{dir: dirs[0], ids: o.IDs}}, nil
	}
	cl, ok := agentio.ConversationsOf(req.ag)
	if !ok {
		return nil, convUnsupported(req.ag, agentio.CapConversations, "list conversations")
	}
	q := agentio.ConversationQuery{}
	dirs := []string{""}
	if o.Scope == ScopeFolder && o.Picker != PickerFolder {
		dirs = a.conversationDirs(o.Dir)
		q.Dir = dirs[0]
	}
	convs, err := listDirs(ctx, cl, req.from.home, q, dirs)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("%s %s: %s", req.ag.ID(), a.tilde(req.from.home.Dir()), err.Error()))
	}
	if len(convs) == 0 {
		where := "any folder"
		if q.Dir != "" {
			where = q.Dir
		}
		return nil, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("no %s conversations for %s in %s", req.ag.ID(), where, a.tilde(req.from.home.Dir())))
	}
	whole := len(o.IDs) == 0 && o.Picker == ""
	switch {
	case o.Picker != "":
		convs, err = a.pickConversations(ctx, o.Picker, convs)
	case len(o.IDs) > 0:
		convs, err = selectByIDs(convs, o.IDs)
	}
	if err != nil {
		return nil, err
	}
	var groups []moveGroup
	idx := map[string]int{}
	for _, c := range convs {
		i, ok := idx[c.Dir]
		if !ok {
			i = len(groups)
			idx[c.Dir] = i
			groups = append(groups, moveGroup{dir: filepath.Clean(c.Dir)})
		}
		if !whole {
			groups[i].ids = append(groups[i].ids, c.ID)
		}
	}
	return groups, nil
}

// selectByIDs keeps the conversations named by ids (exact or unique
// prefix); a missing or ambiguous id is refused.
func selectByIDs(convs []agentio.Conversation, ids []string) ([]agentio.Conversation, error) {
	var out []agentio.Conversation
	for _, want := range ids {
		var hits []agentio.Conversation
		for _, c := range convs {
			if c.ID == want {
				hits = []agentio.Conversation{c}
				break
			}
			if strings.HasPrefix(c.ID, want) {
				hits = append(hits, c)
			}
		}
		switch len(hits) {
		case 0:
			return nil, apperr.Failure(apperr.CodeNotFound, "no conversation "+want+" in the source home")
		case 1:
			out = append(out, hits[0])
		default:
			return nil, apperr.New(apperr.CodeAmbiguous, apperr.ClassUsage, fmt.Sprintf("%s matches %d conversations; give more of the id", want, len(hits)))
		}
	}
	return out, nil
}

// pickConversations asks at a terminal: with picker folder, a folder
// first; then a checklist of chats. Without a terminal it refuses with
// the named form (exit 2).
func (a *App) pickConversations(ctx context.Context, mode string, convs []agentio.Conversation) ([]agentio.Conversation, error) {
	if a.d.Picker == nil {
		return nil, picker.NoTTY("pass --id (repeat) to name the conversations")
	}
	if mode == PickerFolder {
		var folders []string
		for _, c := range convs {
			if !slices.Contains(folders, c.Dir) {
				folders = append(folders, c.Dir)
			}
		}
		items := make([]picker.Item, len(folders))
		for i, f := range folders {
			items[i] = picker.Item{Label: a.tilde(f)}
		}
		i, err := a.d.Picker.Choose(ctx, "Move conversations of which folder?", items)
		if err != nil {
			return nil, err
		}
		convs = slices.DeleteFunc(slices.Clone(convs), func(c agentio.Conversation) bool { return c.Dir != folders[i] })
	}
	items := make([]picker.Item, len(convs))
	for i, c := range convs {
		title := c.Title
		if title == "" {
			title = "(untitled)"
		}
		items[i] = picker.Item{Label: title, Detail: c.ID + "  " + datefmt.RoundMinute(c.Updated).Format("2006-01-02 15:04")}
	}
	picked, err := a.d.Picker.Checklist(ctx, "Move which conversations?", items)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Conversation, 0, len(picked))
	for _, i := range picked {
		out = append(out, convs[i])
	}
	return out, nil
}

// liveBlockers names running sessions of the conversations that move, or
// in their folder, in either home (agentio LiveSessionsOf), so a live
// conversation is never moved from under its agent. An agent without live
// sessions adds none; its own plan blockers still apply.
func (a *App) liveBlockers(ag agentio.Agent, plan agentio.MovePlan, homes ...agentio.Home) []string {
	ls, ok := agentio.LiveSessionsOf(ag)
	if !ok {
		return nil
	}
	var out []string
	for _, h := range homes {
		live, err := ls.LiveSessions(h)
		if err != nil {
			out = append(out, fmt.Sprintf("live sessions of %s cannot be read: %s", a.tilde(h.Dir()), apperr.Text(err)))
			continue
		}
		for _, s := range live {
			if !s.Alive {
				continue
			}
			if filepath.Clean(s.Dir) == plan.Dir || slices.Contains(plan.Conversations, s.ConversationID) {
				// The agent's own plan may already name this pid; one line
				// per running process is enough.
				pid := fmt.Sprintf("pid %d", s.PID)
				named := func(b string) bool { return strings.Contains(b, pid) }
				if !slices.ContainsFunc(plan.Blockers, named) && !slices.ContainsFunc(out, named) {
					out = append(out, fmt.Sprintf("conversation %s is running (%s); exit it first", s.ConversationID, pid))
				}
			}
		}
	}
	return out
}

// planMoveError maps a PlanMove failure: an id the folder does not hold is
// not-found, anything else an I/O failure.
func (a *App) planMoveError(err error, dir string) error {
	if errors.Is(err, agentio.ErrNotFound) {
		return apperr.Wrap(err, apperr.CodeNotFound, apperr.ClassFailure, err.Error())
	}
	return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("plan the move of %s: %s", dir, err.Error()))
}

// moveFolderOf is the data view of a plan (nil lists as empty).
func moveFolderOf(p agentio.MovePlan) MoveFolder {
	nz := func(s []string) []string {
		if s == nil {
			return []string{}
		}
		return s
	}
	items := p.Items
	if items == nil {
		items = []agentio.MoveItem{}
	}
	return MoveFolder{Folder: p.Dir, Conversations: nz(p.Conversations), Items: items, Left: nz(p.Left), Blockers: nz(p.Blockers)}
}
