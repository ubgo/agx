package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/sessions"
)

// SessionsRoot is the session-folder root: sessions.root, else
// <config dir>/sessions.
func (a *App) SessionsRoot() string {
	c := a.cfgOrEmpty()
	if c.Sessions.Root != "" {
		return config.ExpandPath(c.Sessions.Root, a.d.Lookup, a.d.Host.UserHome)
	}
	return filepath.Join(a.paths.Dir, config.SessionsDirName)
}

// newSessionDir is the folder agx new would create.
func (a *App) newSessionDir(name string) (string, error) {
	return sessions.Path(a.SessionsRoot(), name, a.Now()), nil
}

// createSessionFolder creates a new session folder (agx new, cost W).
func (a *App) createSessionFolder(dir string) error {
	if err := sessions.Create(dir); err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "create "+dir+": "+err.Error())
	}
	return nil
}

// DefaultMinAge is session clean --min-age's default: a folder made today
// may be about to get its first file.
const DefaultMinAge = 24 * time.Hour

// historyHomes are the homes asked whether a folder has history: every
// configured home plus the homes agentio finds on disk. The safety checks
// of clean and promote must see history in a home that is not a profile
// too, or clean could delete a folder whose conversations live there.
func (a *App) historyHomes() ([]homeRef, []report.Warning, error) {
	homes, warns, err := a.configuredHomes(homeFilter{})
	if err != nil {
		return nil, nil, err
	}
	return append(homes, a.discoveredHomes("", homes)...), warns, nil
}

// folderHistory reports which agents hold history for dir in homes, and
// whether any home could not answer.
func (a *App) folderHistory(ctx context.Context, homes []homeRef, dir string) (agents []string, unknown bool) {
	agents = []string{}
	for _, h := range homes {
		id := string(h.agent.ID())
		if slices.Contains(agents, id) {
			continue
		}
		has, err := a.hasHistory(ctx, h, dir)
		switch {
		case err != nil:
			unknown = true
		case has:
			agents = append(agents, id)
		}
	}
	return agents, unknown
}

// SessionList lists the session folders under the root, newest first, with
// their file count and which agents hold history for them. Disk · Read.
func (a *App) SessionList(ctx context.Context) ([]sessions.Folder, []report.Warning, error) {
	homes, warns, err := a.historyHomes()
	if err != nil {
		return nil, nil, err
	}
	list, err := sessions.List(a.SessionsRoot(), func(dir string) ([]string, bool) { return a.folderHistory(ctx, homes, dir) })
	if err != nil {
		return nil, nil, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, err.Error())
	}
	if list == nil {
		list = []sessions.Folder{}
	}
	return list, warns, nil
}

// SessionCleanOptions are the flags of agx session clean.
type SessionCleanOptions struct {
	// MinAge keeps folders younger than this (default 24h).
	MinAge time.Duration
	// Yes removes; without it the command only prints the plan.
	Yes bool
}

// SessionCleanItem is one folder's decision and outcome.
type SessionCleanItem struct {
	sessions.CleanItem
	// Removed is true once the folder is gone (--yes).
	Removed bool `json:"removed"`
	// Error says why a planned removal failed.
	Error string `json:"error,omitempty"`
}

// SessionCleanResult is the result of session clean.
type SessionCleanResult struct {
	Root   string             `json:"root"`
	MinAge string             `json:"min_age"`
	Yes    bool               `json:"yes"`
	Items  []SessionCleanItem `json:"items"`
}

// SessionClean plans (and with --yes performs) the removal of session
// folders that are empty, have no history in any agent and are older
// than --min-age (spec 13.7). Removal can only delete empty folders
// (sessions.RemoveEmpty), so a file added after the plan stops it. Disk ·
// Write with --yes.
func (a *App) SessionClean(ctx context.Context, o SessionCleanOptions) (SessionCleanResult, []report.Warning, error) {
	res := SessionCleanResult{Root: a.tilde(a.SessionsRoot()), MinAge: o.MinAge.String(), Yes: o.Yes, Items: []SessionCleanItem{}}
	if o.MinAge < 0 {
		return res, nil, apperr.Usage("--min-age must not be negative")
	}
	list, warns, err := a.SessionList(ctx)
	if err != nil {
		return res, nil, err
	}
	failed := 0
	for _, it := range sessions.PlanClean(list, a.Now(), o.MinAge) {
		item := SessionCleanItem{CleanItem: it}
		if it.Remove && o.Yes {
			if err := ctxErr(ctx); err != nil {
				return res, warns, err
			}
			if err := sessions.RemoveEmpty(it.Folder.Path); err != nil {
				item.Error = apperr.Text(err)
				failed++
			} else {
				item.Removed = true
			}
		}
		res.Items = append(res.Items, item)
	}
	if failed > 0 {
		return res, warns, apperr.New(apperr.CodePartial, apperr.ClassPartial, fmt.Sprintf("%d folder(s) could not be removed", failed))
	}
	return res, warns, nil
}

// SessionPromoteOptions are the flags of agx session promote.
type SessionPromoteOptions struct {
	// ID is the session folder's name under the root.
	ID string
	// Name is the project folder's name; "" = the session's slug.
	Name string
	// To is the parent folder; "" = sessions.promote_root.
	To string
	// GitRepo runs git init in the promoted folder.
	GitRepo bool
	// NoCodex promotes even when an agent's history cannot move with the
	// folder (Codex), leaving it keyed to the old path.
	NoCodex bool
	// DryRun prints the plan and changes nothing.
	DryRun bool
}

// PromoteHistory is one home's history that moves with the folder.
type PromoteHistory struct {
	Agent agentio.ID `json:"agent"`
	Home  string     `json:"home"`
	// Moved is true once re-keyed (false on a dry run).
	Moved bool `json:"moved"`
	// Backup are the copies agentio saved first.
	Backup []string `json:"backup"`
}

// PromoteLeft is one home's history that cannot move with the folder.
type PromoteLeft struct {
	Agent         agentio.ID `json:"agent"`
	Home          string     `json:"home"`
	Conversations int        `json:"conversations"`
}

// SessionPromoteResult is the result of session promote.
type SessionPromoteResult struct {
	From      string           `json:"from"`
	To        string           `json:"to"`
	DryRun    bool             `json:"dry_run"`
	Histories []PromoteHistory `json:"histories"`
	Left      []PromoteLeft    `json:"left"`
	GitRepo   bool             `json:"git_repo"`
}

// promoteTimeout bounds git init (a local command that should take well
// under a second).
const promoteTimeout = 30 * time.Second

// SessionPromote moves a session folder into a project folder and re-keys
// its history in every home whose agent can move history (agentio
// HistoryMoverOf). History of an agent that cannot move (Codex: its index
// holds absolute paths) refuses the promote with exit 4 unless --no-codex
// (edge 55). A running agent in the folder, or history already keyed to
// the destination, refuses before anything moves; a failure after the
// folder moved puts everything back. Disk · Write unless --dry-run.
func (a *App) SessionPromote(ctx context.Context, o SessionPromoteOptions) (SessionPromoteResult, []report.Warning, error) {
	var res SessionPromoteResult
	src, dst, err := a.promotePaths(o)
	if err != nil {
		return res, nil, err
	}
	res = SessionPromoteResult{From: a.tilde(src), To: a.tilde(dst), DryRun: o.DryRun, Histories: []PromoteHistory{}, Left: []PromoteLeft{}, GitRepo: o.GitRepo}
	homes, warns, err := a.historyHomes()
	if err != nil {
		return res, nil, err
	}
	movers, left, err := a.promoteHistories(ctx, homes, src, dst)
	if err != nil {
		return res, warns, err
	}
	res.Left = left
	if n := leftCount(left); n > 0 {
		names := leftNames(homes, left)
		if !o.NoCodex {
			return res, warns, apperr.Unsupported(fmt.Sprintf("this folder has %d %s %s; add --no-codex", n, names, convPlural(n, "conversation"))).
				WithHint(names + " history cannot move with the folder; with --no-codex it stays keyed to " + a.tilde(src) + " and resumes only there")
		}
		warns = append(warns, report.Warning{Code: WarnHistoryLeft, Message: fmt.Sprintf("%d %s %s stay keyed to %s", n, names, convPlural(n, "conversation"), a.tilde(src))})
	}
	for _, h := range movers {
		res.Histories = append(res.Histories, PromoteHistory{Agent: h.agent.ID(), Home: a.tilde(h.home.Dir()), Backup: []string{}})
	}
	if o.DryRun {
		return res, warns, nil
	}
	if err := sessions.Promote(src, dst); err != nil {
		code := apperr.CodeIO
		if errors.Is(err, sessions.ErrExists) {
			code = apperr.CodeExists
		}
		return res, warns, apperr.Wrap(err, code, apperr.ClassFailure, "promote: "+err.Error())
	}
	if err := a.moveHistories(movers, src, dst, res.Histories); err != nil {
		return res, warns, err
	}
	if o.GitRepo {
		if err := sessions.GitInit(ctx, dst, promoteTimeout); err != nil {
			return res, warns, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "promoted to "+a.tilde(dst)+", but "+err.Error())
		}
	}
	return res, warns, nil
}

// promotePaths resolves the session folder and the destination.
func (a *App) promotePaths(o SessionPromoteOptions) (src, dst string, err error) {
	if o.ID == "" {
		return "", "", apperr.Usage("--id is required: the session folder's name (agx session list)")
	}
	if !plainName(o.ID) {
		return "", "", apperr.Usage(fmt.Sprintf("--id %q: give a session folder's name, not a path", o.ID))
	}
	root := a.SessionsRoot()
	src = filepath.Join(root, o.ID)
	if !statDir(src) {
		return "", "", apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("no session folder %s in %s", o.ID, a.tilde(root))).WithHint("agx session list")
	}
	name := o.Name
	if name == "" {
		name = sessions.SlugOf(o.ID)
	}
	if name == "" {
		return "", "", apperr.Usage(fmt.Sprintf("%s has no name before its timestamp; add --name", o.ID))
	}
	if !plainName(name) {
		return "", "", apperr.Usage(fmt.Sprintf("--name %q: give a folder name, not a path", name))
	}
	to := o.To
	if to == "" {
		to = a.cfgOrEmpty().Sessions.PromoteRoot
	}
	if to == "" {
		return "", "", apperr.Usage("--to is required: the folder to move the session into (or set sessions.promote_root)").
			WithHint("agx config set --key sessions.promote_root --value ~/projects")
	}
	to = config.ExpandPath(to, a.d.Lookup, a.d.Host.UserHome)
	if !filepath.IsAbs(to) {
		to = filepath.Join(a.d.Cwd, to)
	}
	return src, filepath.Join(filepath.Clean(to), name), nil
}

// plainName reports a single path element (no separator, not . or ..).
func plainName(s string) bool {
	return s != "." && s != ".." && !strings.ContainsAny(s, `/\`) && filepath.Base(s) == s
}

// promoteHistories splits the homes holding history for src into those
// whose agent can move it and those that cannot (counted), and refuses a
// running agent in src or history already keyed to dst.
func (a *App) promoteHistories(ctx context.Context, homes []homeRef, src, dst string) ([]homeRef, []PromoteLeft, error) {
	var movers []homeRef
	left := []PromoteLeft{}
	for _, h := range homes {
		has, err := a.hasHistory(ctx, h, src)
		if err != nil {
			return nil, nil, err
		}
		if !has {
			continue
		}
		if err := a.refuseLive(h, src); err != nil {
			return nil, nil, err
		}
		if _, ok := agentio.HistoryMoverOf(h.agent); !ok {
			n, err := a.countConversations(ctx, h, src)
			if err != nil {
				return nil, nil, err
			}
			left = append(left, PromoteLeft{Agent: h.agent.ID(), Home: a.tilde(h.home.Dir()), Conversations: n})
			continue
		}
		taken, err := a.hasHistory(ctx, h, dst)
		if err != nil {
			return nil, nil, err
		}
		if taken {
			return nil, nil, apperr.Failure(apperr.CodeMoveBlocked, fmt.Sprintf("%s %s already has history for %s; nothing changed", h.agent.ID(), a.tilde(h.home.Dir()), a.tilde(dst)))
		}
		movers = append(movers, h)
	}
	return movers, left, nil
}

// refuseLive refuses a promote while an agent runs in dir (agentio
// LiveSessionsOf): moving the folder from under it would orphan its
// session.
func (a *App) refuseLive(h homeRef, dir string) error {
	ls, ok := agentio.LiveSessionsOf(h.agent)
	if !ok {
		return nil
	}
	live, err := ls.LiveSessions(h.home)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("live sessions of %s: %s", a.tilde(h.home.Dir()), err.Error()))
	}
	for _, s := range live {
		if s.Alive && filepath.Clean(s.Dir) == dir {
			return apperr.Failure(apperr.CodeMoveBlocked, fmt.Sprintf("%s is running in %s (pid %d); exit it first, nothing changed", h.agent.ID(), a.tilde(dir), s.PID))
		}
	}
	return nil
}

// countConversations counts a folder's conversations in one home.
func (a *App) countConversations(ctx context.Context, h homeRef, dir string) (int, error) {
	cl, ok := agentio.ConversationsOf(h.agent)
	if !ok {
		return 0, nil
	}
	convs, err := cl.Conversations(ctx, h.home, agentio.ConversationQuery{Dir: dir})
	if err != nil {
		return 0, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("%s %s: %s", h.agent.ID(), a.tilde(h.home.Dir()), err.Error()))
	}
	return len(convs), nil
}

// leftCount sums the conversations that cannot move.
func leftCount(left []PromoteLeft) int {
	n := 0
	for _, l := range left {
		n += l.Conversations
	}
	return n
}

// leftNames is the display names of the agents in left ("Codex").
func leftNames(homes []homeRef, left []PromoteLeft) string {
	var names []string
	for _, l := range left {
		for _, h := range homes {
			if h.agent.ID() == l.Agent && !slices.Contains(names, h.agent.Name()) {
				names = append(names, h.agent.Name())
			}
		}
	}
	return strings.Join(names, "/")
}

// moveHistories re-keys each mover's history from src to dst. On a
// failure it moves back what it moved and the folder itself, so a failed
// promote leaves everything where it was.
func (a *App) moveHistories(movers []homeRef, src, dst string, out []PromoteHistory) error {
	for i, h := range movers {
		hm, _ := agentio.HistoryMoverOf(h.agent) // promoteHistories kept only agents that have one
		mv, err := hm.MoveHistory(h.home, src, dst)
		if err != nil {
			undo := a.undoPromote(movers[:i], src, dst)
			return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("move %s history in %s: %s%s", h.agent.ID(), a.tilde(h.home.Dir()), err.Error(), undo))
		}
		out[i].Moved = mv.Moved
		for _, e := range mv.Backup.Entries {
			out[i].Backup = append(out[i].Backup, a.tilde(e.Copy))
		}
	}
	return nil
}

// undoPromote moves the histories already re-keyed and the folder back; it
// returns "" when everything went back, else a note naming what did not.
func (a *App) undoPromote(done []homeRef, src, dst string) string {
	var failed []string
	for _, h := range done {
		hm, _ := agentio.HistoryMoverOf(h.agent) // only movers are in done
		if _, err := hm.MoveHistory(h.home, dst, src); err != nil {
			failed = append(failed, fmt.Sprintf("%s history in %s", h.agent.ID(), a.tilde(h.home.Dir())))
		}
	}
	if err := sessions.Unpromote(src, dst); err != nil {
		failed = append(failed, "the folder "+a.tilde(dst))
	}
	if len(failed) == 0 {
		return "; everything was moved back"
	}
	return "; could not move back: " + strings.Join(failed, ", ")
}

// convPlural adds "s" to word unless n is 1.
func convPlural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
