package human

import (
	"fmt"
	"path/filepath"
	"time"
	"unicode/utf8"
)

// The conversation row of agx v0.3's `resume --list` and its picker, on
// one line in fixed columns: when, agent, profile, title, folder and the
// id prefix a person can type back as --id.
//
//	just now     claude work       Review the API                   ~/projects/client             7b1d4e20

// Column widths (v0.3's).
const (
	agoWidth     = 12
	agentWidth   = 6 // "claude", "codex"
	profileWidth = 10
	titleWidth   = 32
	dirWidth     = 28
	// ShortIDLen is how much of an id the row shows: a prefix --id takes.
	ShortIDLen = 8
)

// Untitled stands in for a conversation without a title.
const Untitled = "(untitled)"

// ellipsis marks a cut value.
const ellipsis = "…"

// Conversation is what one row shows.
type Conversation struct {
	Updated time.Time
	Agent   string
	Profile string
	Title   string
	// Dir is the folder, already "~"-shortened.
	Dir string
	ID  string
}

// ConversationLine is c as one fixed-column row, measured from now.
func ConversationLine(c Conversation, now time.Time) string {
	title := c.Title
	if title == "" {
		title = Untitled
	}
	return fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s  %s",
		agoWidth, Ago(now, c.Updated),
		agentWidth, Truncate(c.Agent, agentWidth),
		profileWidth, Truncate(c.Profile, profileWidth),
		titleWidth, Truncate(title, titleWidth),
		dirWidth, shortDir(c.Dir),
		ShortID(c.ID))
}

// Truncate cuts s to n runes, the last one an ellipsis.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + ellipsis
}

// shortDir fits a folder into dirWidth: whole when it fits, else its last
// segment behind an ellipsis (the part that tells folders apart).
func shortDir(dir string) string {
	if utf8.RuneCountInString(dir) <= dirWidth {
		return dir
	}
	return Truncate(ellipsis+"/"+filepath.Base(dir), dirWidth)
}

// ShortID is the first ShortIDLen runes of id, unmarked: it is a prefix
// the user can type back, so it never carries an ellipsis.
func ShortID(id string) string {
	r := []rune(id)
	return string(r[:min(len(r), ShortIDLen)])
}

// The folder row of agx v0.3's `resume --browse` (conversation list
// --by-folder): when, how many chats, the profiles and the folder.
const (
	countWidth    = 9 // "123 chats"
	profilesWidth = 20
	folderWidth   = 56
)

// Folder is what one folder row shows.
type Folder struct {
	Updated  time.Time
	Count    int
	Profiles []string
	// Dir is the folder, already "~"-shortened.
	Dir string
}

// FolderLine is f as one fixed-column row, measured from now.
func FolderLine(f Folder, now time.Time) string {
	chats := fmt.Sprintf("%d chats", f.Count)
	if f.Count == 1 {
		chats = "1 chat"
	}
	return fmt.Sprintf("%-*s %-*s %-*s %s",
		agoWidth, Ago(now, f.Updated),
		countWidth, chats,
		profilesWidth, Truncate(joinComma(f.Profiles), profilesWidth),
		tailDir(f.Dir, folderWidth))
}

// tailDir keeps the end of a long folder (the part that tells folders
// apart) behind an ellipsis.
func tailDir(dir string, n int) string {
	r := []rune(dir)
	if len(r) <= n {
		return dir
	}
	return ellipsis + string(r[len(r)-(n-1):])
}

// joinComma joins with commas, no spaces (v0.3's profiles cell).
func joinComma(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
