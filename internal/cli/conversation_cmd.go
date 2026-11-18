package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/human"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
	"github.com/ubgo/agx/sessions"
)

// conversationCmd is agx conversation (spec 13.2).
func (c *cliCtx) conversationCmd() *cobra.Command {
	return group("conversation", "Conversations stored in agent homes",
		c.conversationListCmd(), c.conversationShowCmd(), c.conversationMoveCmd())
}

// Fields of conversation list's data (help --json); --by-folder rows
// carry folder, conversations, updated, agents and profiles.
var conversationListFields = []string{"data.*.agent", "data.*.profiles", "data.*.home", "data.*.id", "data.*.title", "data.*.folder", "data.*.created", "data.*.updated", "data.*.conversations", "data.*.agents"}

// conversationListCmd is agx conversation list: it only prints; choosing
// happens in resume (decision record).
func (c *cliCtx) conversationListCmd() *cobra.Command {
	var o app.ConversationListOptions
	cmd := c.newCmd("list", "Print conversations, newest first (this folder unless --scope all)", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			out := render.Output{Command: commandPath(cmd)}
			// A partial result (some homes failed, exit 3) keeps its rows.
			res, lerr := c.App().ConversationList(cmd.Context(), o)
			out.Warnings = res.Warnings
			if res.Rows == nil {
				return out, lerr
			}
			var err error
			now, home := c.App().Now(), c.env.Host.UserHome
			if o.ByFolder {
				out.Data, err = render.Encode(res.Folders)
				out.Text = func(w io.Writer) error { return folderLines(w, res.Folders, now, home) }
			} else {
				out.Data, err = render.Encode(res.Rows)
				out.Text = func(w io.Writer) error { return conversationLines(w, res.Rows, now, home) }
			}
			return out, errors.Join(lerr, err)
		})
	cmd.Example = "  agx conversation list\n  agx conversation list --scope all --query pglite\n  agx conversation list --scope all --by-folder --agent claude"
	f := cmd.Flags()
	f.StringVar(&o.Query, "query", "", "keep conversations whose title, folder or id holds this text")
	f.StringVar(&o.Scope, "scope", app.ScopeFolder, "folder (the current folder) or all")
	f.BoolVar(&o.ByFolder, "by-folder", false, "one row per folder")
	f.IntVar(&o.Limit, "limit", app.DefaultConversationLimit, "conversations considered, newest first (0 = all)")
	f.StringVar((*string)(&o.Agent), "agent", "", "only this agent's homes")
	f.StringArrayVar(&o.Profiles, "profile", nil, "only these profiles' homes (repeat)")
	return fields(cmd, conversationListFields...)
}

// conversationLines prints v0.3's `resume --list` rows: when, agent,
// profile, title, folder and the id prefix --id takes, no header; the
// full values stay in --json and --field.
func conversationLines(w io.Writer, rows []app.ConversationRow, now time.Time, userHome string) error {
	ew := textout.New(w)
	for _, r := range rows {
		ew.Line(human.ConversationLine(human.Conversation{Updated: r.Updated, Agent: string(r.Agent), Profile: strings.Join(r.Profiles, ","),
			Title: r.Title, Dir: config.TildePath(r.Folder, userHome), ID: r.ID}, now))
	}
	return ew.Err()
}

// folderLines prints v0.3's `resume --browse` folder rows.
func folderLines(w io.Writer, rows []app.FolderRow, now time.Time, userHome string) error {
	ew := textout.New(w)
	for _, r := range rows {
		ew.Line(human.FolderLine(human.Folder{Updated: r.Updated, Count: r.Conversations, Profiles: r.Profiles, Dir: config.TildePath(r.Folder, userHome)}, now))
	}
	return ew.Err()
}

// conversationShowCmd is agx conversation show.
func (c *cliCtx) conversationShowCmd() *cobra.Command {
	var id string
	cmd := c.newCmd("show", "One conversation: home, folder, title, turns, size, last activity, live", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			out := render.Output{Command: commandPath(cmd)}
			d, err := c.App().ConversationShow(cmd.Context(), id)
			if err != nil {
				return out, err
			}
			out.Data, err = render.Encode(d)
			out.Text = func(w io.Writer) error { return conversationText(w, d, c.dateFormat()) }
			return out, err
		})
	cmd.Example = "  agx conversation show --id 3f2a"
	cmd.Flags().StringVar(&id, "id", "", "the conversation, exact or a unique prefix")
	return fields(cmd, "data.agent", "data.profiles", "data.home", "data.id", "data.title", "data.folder", "data.created", "data.updated", "data.path", "data.size_bytes", "data.turns", "data.live")
}

// conversationText renders conversation show, its times in date.
func conversationText(w io.Writer, d app.ConversationDetail, date datefmt.Format) error {
	turns, live := "unknown (the agent cannot read turns)", "unknown (the agent has no live sessions)"
	if d.Turns != nil {
		turns = strconv.Itoa(*d.Turns)
	}
	if d.Live != nil {
		live = "no"
		if *d.Live {
			live = "yes"
		}
	}
	title := d.Title
	if title == "" {
		title = "(untitled)"
	}
	return keyValues(w, [][2]string{
		{"id", d.ID}, {"title", title}, {"agent", string(d.Agent)}, {"profiles", strings.Join(d.Profiles, ", ")},
		{"home", d.Home}, {"folder", d.Folder}, {"turns", turns}, {"size", byteSize(d.SizeBytes)},
		{"created", timeText(d.Created, date)}, {"updated", timeText(d.Updated, date)}, {"live", live}, {"path", d.Path},
	})
}

// keyValues prints aligned "key  value" lines.
func keyValues(w io.Writer, rows [][2]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	ew := textout.New(tw)
	for _, r := range rows {
		ew.Line(r[0] + "\t" + r[1])
	}
	if ew.Err() != nil {
		return ew.Err()
	}
	return tw.Flush()
}

// timeText is a local time in date (JSON keeps RFC 3339), or a dash.
func timeText(t time.Time, date datefmt.Format) string {
	if t.IsZero() {
		return render.Dash
	}
	return date.Date(t.Local())
}

// Byte size units for byteSize.
const (
	kib = 1 << 10
	mib = 1 << 20
)

// byteSize renders a size for humans.
func byteSize(n int64) string {
	switch {
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/mib)
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/kib)
	}
	return fmt.Sprintf("%d B", n)
}

// conversationMoveCmd is agx conversation move.
func (c *cliCtx) conversationMoveCmd() *cobra.Command {
	var o app.ConversationMoveOptions
	cmd := c.newCmd("move", "Move a folder's conversations to another home of the same agent", CostWrite,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			out := render.Output{Command: commandPath(cmd)}
			o.DirSet = cmd.Flags().Changed("dir")
			res, err := c.App().ConversationMove(cmd.Context(), o)
			if err != nil {
				return out, err
			}
			out.Data, err = render.Encode(res)
			out.Text = func(w io.Writer) error { return moveText(w, res) }
			return out, err
		})
	cmd.Long = "Move a folder's conversations from one home to another home of the same agent, so they resume there. Only agents whose history can move (agentio ConversationMover; Claude today) are supported; others are refused with the agent's reason (exit 4). A running conversation, or an id already in the target, stops the move before any file changes. agentio backs every file up first; --no-backup discards that copy after a successful move."
	cmd.Example = "  agx conversation move --to-profile work --dry-run\n  agx conversation move --to-profile work --from-profile personal --id 3f2a\n  agx conversation move --to-profile work --picker chat"
	f := cmd.Flags()
	f.StringVar(&o.Dir, "dir", "", "the folder whose conversations move (default: the current folder)")
	f.StringVar(&o.ToProfile, "to-profile", "", "the profile whose home receives them (required)")
	f.StringVar(&o.FromProfile, "from-profile", "", "the profile whose home holds them (default: the one other home with history here)")
	f.StringArrayVar(&o.IDs, "id", nil, "only this conversation, exact or a unique prefix (repeat)")
	f.StringVar(&o.Picker, "picker", "", "chat (tick conversations) or folder (choose a folder, then tick); needs a terminal")
	f.StringVar(&o.Scope, "scope", app.ScopeFolder, "folder (the current folder) or all (every folder of the source home)")
	f.BoolVar(&o.NoBackup, "no-backup", false, "discard the backup after a successful move")
	f.BoolVar(&o.DryRun, "dry-run", false, "print the plan; change nothing")
	return fields(cmd, "data.agent", "data.from", "data.to", "data.dry_run", "data.folders.*.folder", "data.folders.*.conversations", "data.folders.*.items", "data.folders.*.left", "data.folders.*.blockers", "data.backups")
}

// moveText renders conversation move.
func moveText(w io.Writer, r app.ConversationMoveResult) error {
	ew := textout.New(w)
	verb := "moved"
	if r.DryRun {
		verb = "would move"
	}
	for _, f := range r.Folders {
		ew.Line(fmt.Sprintf("%s %d %s conversation(s) of %s from %s to %s", verb, len(f.Conversations), r.Agent, f.Folder, r.From.Home, r.To.Home))
		for _, id := range f.Conversations {
			ew.Line("  " + id)
		}
		for _, l := range f.Left {
			ew.Line("  stays: " + l)
		}
	}
	for _, b := range r.Backups {
		ew.Line("backup: " + b)
	}
	if r.DryRun {
		ew.Line("dry run: nothing changed")
	}
	return ew.Err()
}

// sessionCmd is agx session (spec 13.7).
func (c *cliCtx) sessionCmd() *cobra.Command {
	return group("session", "Session folders made by agx new",
		c.sessionListCmd(), c.sessionCleanCmd(), c.sessionPromoteCmd())
}

// sessionListCmd is agx session list.
func (c *cliCtx) sessionListCmd() *cobra.Command {
	cmd := c.newCmd("list", "Session folders with files and history", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			out := render.Output{Command: commandPath(cmd)}
			list, warns, err := c.App().SessionList(cmd.Context())
			out.Warnings = warns
			if err != nil {
				return out, err
			}
			out.Data, err = render.Encode(list)
			a, st := c.App(), c.style()
			root, now := config.TildePath(a.SessionsRoot(), c.env.Host.UserHome), a.Now()
			out.Text = func(w io.Writer) error {
				// The root and count lead on stderr, as v0.3's did, so stdout
				// stays the table alone.
				c.note(fmt.Sprintf("%s (%d folders)", root, len(list)))
				return sessionListText(w, list, now, st)
			}
			return out, err
		})
	return fields(cmd, "data.*.name", "data.*.path", "data.*.created", "data.*.modified", "data.*.files", "data.*.history", "data.*.history_unknown")
}

// sessionListText is v0.3's `sessions ls`: FOLDER, LAST ACTIVITY (how
// long ago, the way a person thinks of it), FILES and HISTORY, newest
// first; the full paths stay in --json.
func sessionListText(w io.Writer, list []sessions.Folder, now time.Time, st render.Style) error {
	rows := make([][]render.StyledCell, 0, len(list))
	for _, f := range list {
		history := render.StyledCell{Text: strings.Join(f.History, ",")}
		switch {
		case f.HistoryUnknown:
			history = render.StyledCell{Text: sessionHistoryUnknown, Tone: render.ToneWarn}
		case len(f.History) == 0:
			history = render.StyledCell{Text: render.Dash, Tone: render.ToneDim}
		}
		rows = append(rows, []render.StyledCell{{Text: f.Name}, {Text: human.Ago(now, f.Modified)}, {Text: strconv.Itoa(f.Files)}, history})
	}
	return render.StyledTable(w, []string{"FOLDER", "LAST ACTIVITY", "FILES", "HISTORY"}, rows, st)
}

// sessionHistoryUnknown is HISTORY when an agent could not answer.
const sessionHistoryUnknown = "unknown"

// sessionCleanCmd is agx session clean: a plan unless --yes.
func (c *cliCtx) sessionCleanCmd() *cobra.Command {
	var o app.SessionCleanOptions
	cmd := c.newCmd("clean", "Remove empty session folders without history; a plan unless --yes", CostWrite,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			out := render.Output{Command: commandPath(cmd)}
			// A partial clean (some removals failed, exit 3) keeps its items.
			res, warns, cerr := c.App().SessionClean(cmd.Context(), o)
			out.Warnings = warns
			if res.Items == nil {
				return out, cerr
			}
			data, err := render.Encode(res)
			out.Data = data
			out.Text = func(w io.Writer) error { return cleanText(w, res) }
			return out, errors.Join(cerr, err)
		})
	cmd.Example = "  agx session clean\n  agx session clean --min-age 72h --yes"
	f := cmd.Flags()
	f.DurationVar(&o.MinAge, "min-age", app.DefaultMinAge, "keep folders younger than this")
	f.BoolVar(&o.Yes, "yes", false, "remove the planned folders (without it: only the plan)")
	return fields(cmd, "data.root", "data.min_age", "data.yes", "data.items.*.folder", "data.items.*.remove", "data.items.*.reason", "data.items.*.removed", "data.items.*.error")
}

// cleanText renders session clean.
func cleanText(w io.Writer, r app.SessionCleanResult) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	ew := textout.New(tw)
	n := 0
	for _, it := range r.Items {
		action := "keep"
		switch {
		case it.Removed:
			action = "removed"
		case it.Error != "":
			action = "failed"
		case it.Remove:
			action = "remove"
			n++
		}
		line := action + "\t" + it.Folder.Name + "\t" + string(it.Reason)
		if it.Error != "" {
			line += ": " + it.Error
		}
		ew.Line(line)
	}
	if !r.Yes {
		ew.Line(fmt.Sprintf("plan only: %d folder(s) to remove in %s; add --yes to remove them", n, r.Root))
	}
	if ew.Err() != nil {
		return ew.Err()
	}
	return tw.Flush()
}

// sessionPromoteCmd is agx session promote.
func (c *cliCtx) sessionPromoteCmd() *cobra.Command {
	var o app.SessionPromoteOptions
	cmd := c.newCmd("promote", "Move a session folder into a project with its history", CostWrite,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			out := render.Output{Command: commandPath(cmd)}
			res, warns, err := c.App().SessionPromote(cmd.Context(), o)
			out.Warnings = warns
			if err != nil {
				return out, err
			}
			out.Data, err = render.Encode(res)
			out.Text = func(w io.Writer) error { return promoteText(w, res) }
			return out, err
		})
	cmd.Long = "Move a session folder into <--to or sessions.promote_root>/<--name or its slug> with one rename, and re-key its history in every home whose agent can move history. An agent whose history cannot move (Codex) refuses the promote (exit 4) unless --no-codex, which leaves those conversations keyed to the old folder."
	cmd.Example = "  agx session promote --id pglite_golang_20261114_101500 --dry-run\n  agx session promote --id pglite_golang_20261114_101500 --name pglite --to ~/projects --git-repo"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the session folder's name (agx session list)")
	f.StringVar(&o.Name, "name", "", "the project folder's name (default: the session's slug)")
	f.StringVar(&o.To, "to", "", "the parent folder (default: sessions.promote_root)")
	f.BoolVar(&o.GitRepo, "git-repo", false, "run git init in the promoted folder")
	f.BoolVar(&o.NoCodex, "no-codex", false, "promote even though some history (Codex) cannot move with the folder")
	f.BoolVar(&o.DryRun, "dry-run", false, "print the plan; change nothing")
	return fields(cmd, "data.from", "data.to", "data.dry_run", "data.histories.*.agent", "data.histories.*.home", "data.histories.*.moved", "data.histories.*.backup", "data.left.*.agent", "data.left.*.home", "data.left.*.conversations", "data.git_repo")
}

// promoteText renders session promote.
func promoteText(w io.Writer, r app.SessionPromoteResult) error {
	ew := textout.New(w)
	verb := "promoted"
	if r.DryRun {
		verb = "would promote"
	}
	ew.Line(fmt.Sprintf("%s %s to %s", verb, r.From, r.To))
	for _, h := range r.Histories {
		ew.Line(fmt.Sprintf("  %s history in %s moves with it", h.Agent, h.Home))
		for _, b := range h.Backup {
			ew.Line("    backup: " + b)
		}
	}
	for _, l := range r.Left {
		ew.Line(fmt.Sprintf("  %d %s conversation(s) in %s stay keyed to %s", l.Conversations, l.Agent, l.Home, r.From))
	}
	if r.GitRepo {
		ew.Line("  git init in " + r.To)
	}
	if r.DryRun {
		ew.Line("dry run: nothing changed")
	}
	return ew.Err()
}
