// Package sessions owns session folders: the timestamped scratch folders
// agx new creates under the sessions root (spec 13.7): naming, listing,
// planning a safe clean, and promoting one into a real project. It also
// runs the two file operations around them that agx itself performs:
// git init in a promoted folder, and discarding a conversation-move backup
// on --no-backup. It is one of the packages allowed to write files
// (internal/archtest).
//
// It touches only the filesystem: whether a folder has conversation
// history is asked through a callback, so this package never imports an
// agent.
package sessions

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/shell"
)

// TimeLayout is the timestamp suffix of a session folder
// ("pglite_golang_20261114_101500").
const TimeLayout = "20060102_150405"

// FolderMode is a new session folder's mode: an ordinary project folder.
const FolderMode os.FileMode = 0o755

// slugUnsafe matches every run of bytes a slug word may not hold.
var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// stampTail recognises the timestamp at the end of a folder name, with an
// optional numeric suffix a hand-made duplicate may carry ("_2").
var stampTail = regexp.MustCompile(`(?:^|_)([0-9]{8}_[0-9]{6})(?:_[0-9]+)?$`)

// ignoredFiles do not count as content: a folder holding only these is
// empty for clean (and they are removed with it). Finder drops .DS_Store
// into every folder it opens.
var ignoredFiles = map[string]bool{".DS_Store": true}

// Slug turns --name words into the folder prefix: lowercase, every run of
// other bytes becomes "_".
func Slug(name string) string {
	return strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(name), "_"), "_")
}

// FolderName is "<slug>_<timestamp>", or the timestamp alone.
func FolderName(name string, now time.Time) string {
	stamp := now.Format(TimeLayout)
	if s := Slug(name); s != "" {
		return s + "_" + stamp
	}
	return stamp
}

// SlugOf is the slug part of a session folder name: the name without its
// timestamp tail ("pglite_golang" of "pglite_golang_20261114_101500"), ""
// for a bare timestamp, or the whole name when it carries no timestamp.
// session promote uses it as the default project name.
func SlugOf(name string) string {
	loc := stampTail.FindStringIndex(name)
	if loc == nil {
		return name
	}
	return strings.Trim(name[:loc[0]], "_")
}

// Path is the session folder path under root.
func Path(root, name string, now time.Time) string {
	return filepath.Join(root, FolderName(name, now))
}

// Create makes a session folder (and the root), refusing nothing: a
// timestamped name never exists already unless two launches share a
// second, in which case they share the folder.
func Create(dir string) error { return os.MkdirAll(dir, FolderMode) }

// Folder is one session folder found under the root.
type Folder struct {
	// Name is the folder's base name.
	Name string `json:"name"`
	// Path is the absolute folder path.
	Path string `json:"path"`
	// Created is parsed from the name's timestamp (local time); zero when
	// the name carries none (a folder made by hand).
	Created time.Time `json:"created,omitzero"`
	// Modified is the newest mtime among the folder and its files.
	Modified time.Time `json:"modified"`
	// Files counts regular files, recursively, without ignoredFiles.
	Files int `json:"files"`
	// History names the agents holding conversations started here
	// ("claude", "codex"); empty when none does.
	History []string `json:"history"`
	// HistoryUnknown is true when an agent could not answer; clean then
	// keeps the folder, because "could not ask" must never read as "none".
	HistoryUnknown bool `json:"history_unknown,omitempty"`
}

// HistoryFunc reports which agents hold history for dir. unknown is true
// when at least one agent could not answer.
type HistoryFunc func(dir string) (agents []string, unknown bool)

// List returns the folders directly under root, newest first. A missing
// root is an empty list, not an error.
func List(root string, history HistoryFunc) ([]Folder, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	var out []Folder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		f, err := Inspect(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		f.History, f.HistoryUnknown = history(f.Path)
		if f.History == nil {
			f.History = []string{}
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return sortKey(out[i]).After(sortKey(out[j])) })
	return out, nil
}

// sortKey orders by the creation stamp, falling back to the mtime for
// hand-made folders.
func sortKey(f Folder) time.Time {
	if !f.Created.IsZero() {
		return f.Created
	}
	return f.Modified
}

// Inspect gathers one folder's stats (history is left to the caller).
func Inspect(path string) (Folder, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Folder{}, fmt.Errorf("stat %s: %w", path, err)
	}
	f := Folder{Name: filepath.Base(path), Path: path, Modified: info.ModTime(), History: []string{}}
	if m := stampTail.FindStringSubmatch(f.Name); m != nil {
		if t, err := time.ParseInLocation(TimeLayout, m[1], time.Local); err == nil {
			f.Created = t
		}
	}
	// An unreadable entry is skipped rather than failing the listing: the
	// walk counts what it can read, and clean then only ever sees a folder
	// as fuller than it is, never emptier (RemoveEmpty re-checks anyway).
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() && !ignoredFiles[d.Name()] {
			f.Files++
			if fi, err := d.Info(); err == nil && fi.ModTime().After(f.Modified) {
				f.Modified = fi.ModTime()
			}
		}
		return nil
	})
	if err != nil {
		return Folder{}, fmt.Errorf("walk %s: %w", path, err)
	}
	return f, nil
}

// Reason says why clean removes or keeps a folder. Closed set.
type Reason string

// Clean reasons, in the order PlanClean checks them.
const (
	// ReasonRemove: empty, no history, older than the minimum age.
	ReasonRemove Reason = "empty, no history"
	// ReasonHasFiles keeps a folder holding files.
	ReasonHasFiles Reason = "has files"
	// ReasonHistoryUnknown keeps a folder whose history could not be read.
	ReasonHistoryUnknown Reason = "history unknown"
	// ReasonHasHistory keeps a folder with conversations.
	ReasonHasHistory Reason = "has conversation history"
	// ReasonTooNew keeps a folder younger than --min-age.
	ReasonTooNew Reason = "newer than --min-age"
)

// ReasonValues lists every Reason in check order (canonical iteration).
func ReasonValues() []Reason {
	return []Reason{ReasonRemove, ReasonHasFiles, ReasonHistoryUnknown, ReasonHasHistory, ReasonTooNew}
}

// CleanItem is one folder's clean decision.
type CleanItem struct {
	Folder Folder `json:"folder"`
	Remove bool   `json:"remove"`
	Reason Reason `json:"reason"`
}

// PlanClean decides for each folder whether clean may remove it: no files,
// no history in any agent, and older than minAge (by its stamp, else its
// mtime). A folder whose history is unknown is kept. Pure.
func PlanClean(folders []Folder, now time.Time, minAge time.Duration) []CleanItem {
	out := make([]CleanItem, 0, len(folders))
	for _, f := range folders {
		item := CleanItem{Folder: f}
		switch {
		case f.Files > 0:
			item.Reason = ReasonHasFiles
		case f.HistoryUnknown:
			item.Reason = ReasonHistoryUnknown
		case len(f.History) > 0:
			item.Reason = ReasonHasHistory
		case now.Sub(sortKey(f)) < minAge:
			item.Reason = ReasonTooNew
		default:
			item.Remove, item.Reason = true, ReasonRemove
		}
		out = append(out, item)
	}
	return out
}

// RemoveEmpty deletes an empty folder tree: ignored files first, then the
// folders bottom-up with os.Remove, which refuses a folder that still has
// content. So a file created after the plan was made stops it instead of
// being deleted: clean can never remove real work.
func RemoveEmpty(path string) error {
	var dirs []string
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		if !ignoredFiles[d.Name()] {
			return fmt.Errorf("%s is not empty (%s)", path, p)
		}
		return os.Remove(p)
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Remove(dirs[i]); err != nil {
			return fmt.Errorf("remove %s: %w", dirs[i], err)
		}
	}
	return nil
}

// ErrExists means a promote destination already exists.
var ErrExists = errors.New("destination already exists")

// ErrCrossDevice means the destination is on another disk. Promote is one
// rename (atomic, never a half-copied project), which the OS cannot do
// across filesystems, so it refuses instead of copying.
var ErrCrossDevice = errors.New("destination is on another disk; promote only moves within one disk")

// Promote moves src to dst with one rename, refusing to overwrite; dst's
// parent folders are created.
//
// Invariant: a refused or failed promote changes nothing. The rename is
// the only step that can reveal a cross-disk destination, and it needs the
// parents first, so the parents Promote created are removed again when it
// fails (innermost first, with os.Remove, which cannot delete a folder
// something else filled meanwhile).
func Promote(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, dst)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", dst, err)
	}
	created, err := mkdirParents(filepath.Dir(dst))
	if err != nil {
		return fmt.Errorf("create the parent of %s: %w", dst, err)
	}
	if err := os.Rename(src, dst); err != nil {
		removeCreated(created)
		if isCrossDevice(err) {
			return fmt.Errorf("%w: %s to %s", ErrCrossDevice, src, dst)
		}
		return fmt.Errorf("move %s to %s: %w", src, dst, err)
	}
	return nil
}

// Unpromote moves a promoted folder back (the undo when a later step of
// promote fails). It refuses to overwrite src.
func Unpromote(src, dst string) error {
	if _, err := os.Lstat(src); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, src)
	}
	return os.Rename(dst, src)
}

// mkdirParents is os.MkdirAll that also returns the folders it made,
// innermost first, so a caller can undo exactly those.
func mkdirParents(dir string) ([]string, error) {
	var missing []string
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		missing = append(missing, d)
		if parent := filepath.Dir(d); parent == d {
			break // reached the filesystem root
		}
	}
	if err := os.MkdirAll(dir, FolderMode); err != nil {
		removeCreated(missing)
		return nil, err
	}
	return missing, nil
}

// removeCreated undoes mkdirParents, innermost first. It runs only on a
// path that is already failing, and os.Remove refusing a non-empty folder
// is exactly the safety wanted, so its errors are dropped on purpose.
func removeCreated(dirs []string) {
	for _, d := range dirs {
		_ = os.Remove(d) // see the doc comment: a refusal here is the safe outcome
	}
}

// gitInitCommand is the command session promote --git-repo runs in the
// promoted folder; "-q" keeps git's own banner out of agx's output.
const gitInitCommand = "git init -q"

// GitInit runs git init in dir through textdata's shell runner (the one
// runner besides internal/run allowed to start processes, spec 16.1). The
// runner's error never holds stdout; its stderr excerpt is enough to say
// why git failed.
func GitInit(ctx context.Context, dir string, timeout time.Duration) error {
	r := shell.Runner{Dir: dir, Timeout: timeout}
	if _, err := r.Run(ctx, gitInitCommand); err != nil {
		return fmt.Errorf("git init in %s: %w", dir, err)
	}
	return nil
}

// backupMarker is the infix agentio puts in a backup folder's name
// ("<home>.agentio-bak-<stamp>"), see agentio.BackupPath.
const backupMarker = ".agentio-bak-"

// DiscardBackup removes the copies a conversation move saved, for
// conversation move --no-backup: agentio always backs up before a write
// (its invariant 11), so not keeping a backup means deleting it after the
// move succeeded. Only paths inside an agentio backup folder are touched,
// and the backup folders left empty are removed with os.Remove (never a
// folder that holds anything else).
func DiscardBackup(b agentio.Backup) error {
	var errs []error
	for _, e := range b.Entries {
		root, ok := backupRoot(e.Copy)
		if !ok {
			errs = append(errs, fmt.Errorf("%s is not inside an agentio backup folder; kept", e.Copy))
			continue
		}
		if err := os.RemoveAll(e.Copy); err != nil {
			errs = append(errs, err)
			continue
		}
		removeEmptyUpTo(filepath.Dir(e.Copy), root)
	}
	return errors.Join(errs...)
}

// backupRoot finds the agentio backup folder that holds p: the nearest
// ancestor (or p itself) whose name contains backupMarker.
func backupRoot(p string) (string, bool) {
	for d := filepath.Clean(p); ; d = filepath.Dir(d) {
		if strings.Contains(filepath.Base(d), backupMarker) {
			return d, true
		}
		if parent := filepath.Dir(d); parent == d {
			return "", false
		}
	}
}

// removeEmptyUpTo removes dir and its parents up to and including root
// while they are empty; os.Remove refuses a non-empty folder, which ends
// the walk, so a folder still holding another backup copy stays.
func removeEmptyUpTo(dir, root string) {
	for d := filepath.Clean(dir); strings.HasPrefix(d, root); d = filepath.Dir(d) {
		if os.Remove(d) != nil || d == root {
			return
		}
	}
}
