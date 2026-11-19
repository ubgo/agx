// Package skill installs the Claude Code skill agx prints (agx skill
// --install): SKILL.md in <skills dir>/agx/. It owns that one file write
// (internal/archtest lists the packages that write files), and backs up a
// file it would replace, since the user may have edited it.
package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/textdata/atomicfile"
)

// Names and modes of the installed skill.
const (
	// Folder is the skill's folder inside the skills dir.
	Folder = "agx"
	// File is the skill file Claude Code reads.
	File = "SKILL.md"
	// DirMode is the skill folder's mode: skills are read by the agent,
	// never secret.
	DirMode fs.FileMode = 0o755
	// FileMode is SKILL.md's mode.
	FileMode fs.FileMode = 0o644
	// backupStamp names a backup: SKILL.md.bak-20261119T120000Z.
	backupStamp = "20060102T150405Z"
)

// Installed is where the skill went and the backup of a replaced file.
type Installed struct {
	// Path is the SKILL.md written.
	Path string `json:"path"`
	// Backup is the previous SKILL.md, renamed; "" when there was none.
	Backup string `json:"backup,omitempty"`
}

// Install writes content to <dir>/agx/SKILL.md (folder 0755, file 0644,
// atomically). An existing file is first renamed to
// SKILL.md.bak-<UTC stamp of now>.
func Install(dir, content string, now time.Time) (Installed, error) {
	folder := filepath.Join(dir, Folder)
	if err := os.MkdirAll(folder, DirMode); err != nil {
		return Installed{}, err
	}
	out := Installed{Path: filepath.Join(folder, File)}
	switch _, err := os.Stat(out.Path); {
	case err == nil:
		out.Backup = out.Path + ".bak-" + now.UTC().Format(backupStamp)
		if err := os.Rename(out.Path, out.Backup); err != nil {
			return Installed{}, fmt.Errorf("back up %s: %w", out.Path, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return Installed{}, err
	}
	if err := atomicfile.Write(out.Path, []byte(content), atomicfile.Options{Mode: FileMode}); err != nil {
		return Installed{}, err
	}
	return out, nil
}
