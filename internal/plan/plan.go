// Package plan owns the shell layer's plan file (spec 11 and 12).
//
// The shell wrapper cannot cd its own shell from a child process, so a
// launch through the wrapper runs in two steps: `agx new|resume
// --plan-file F` resolves every decision and writes F, the wrapper cds to
// F's folder, then `agx launch --plan-file F` starts the agent. This
// package writes F (0600, atomic, never through a symlink) and reads it
// back with the checks of edge 53: a plan older than MaxAge, from the
// future, owned by another user or writable by others is refused, so a
// plan file planted or left behind cannot steer a later launch.
//
// Invariant: a plan never holds a resolved secret. The payload (Launch)
// is opaque here; package app fills it with the pinned launch options and
// prepares for real (secrets included) only in agx launch.
package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/ubgo/textdata/atomicfile"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// SchemaVersion is the plan file's shape version; Read refuses any other,
// so a shell layer and binary from different agx builds never mix.
const SchemaVersion = 1

// MaxAge is how old a plan may be when read (spec 12): the wrapper reads
// it back within the same command, so anything older is a leftover.
const MaxAge = 60 * time.Second

// clockSkew tolerates a created_at slightly ahead of the reader's clock
// (two processes, one host clock: only rounding, never real skew).
const clockSkew = 2 * time.Second

// FileMode is the plan file's permission: owner only (it names folders,
// profiles and the flags the user typed).
const FileMode = config.TokensFileMode

// othersWrite are the permission bits that let another user edit a file.
const othersWrite fs.FileMode = 0o022

// MsgStaleOrForeign is the refusal of edge 53, word for word.
const MsgStaleOrForeign = "plan file is older than 60s or not yours"

// File is the plan file's content.
type File struct {
	// SchemaVersion is SchemaVersion at write time.
	SchemaVersion int `json:"schema_version"`
	// CreatedAt is when the plan was written (staleness check).
	CreatedAt time.Time `json:"created_at"`
	// Dir is the folder the wrapper cds into before agx launch.
	Dir string `json:"dir"`
	// Launch is package app's payload: the summary shown by plan show and
	// the pinned launch options. Opaque here so this package has no
	// dependency on app (app imports plan).
	Launch json.RawMessage `json:"launch"`
}

// Write writes f to path atomically with FileMode. A symlink at path is
// replaced, never followed, so a planted link cannot redirect the write.
func Write(path string, f File) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicfile.Write(path, append(b, '\n'), atomicfile.Options{Mode: FileMode, NoFollow: true}); err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "write plan file "+path+": "+err.Error())
	}
	return nil
}

// Read reads and checks the plan at path as of now: a regular file (not a
// symlink) owned by this user, not writable by others, of this schema
// version, written at most MaxAge ago. Every refusal is CodePlanFile.
func Read(path string, now time.Time) (File, error) {
	st, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return File{}, refuse("plan file " + path + " does not exist")
		}
		return File{}, refuse("plan file " + path + ": " + err.Error())
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&othersWrite != 0 || !ownedBySelf(st) {
		return File{}, refuse(MsgStaleOrForeign)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, refuse("plan file " + path + ": " + err.Error())
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, refuse("plan file " + path + " is not a plan: " + err.Error())
	}
	if f.SchemaVersion != SchemaVersion {
		return File{}, refuse(fmt.Sprintf("plan file schema_version %d, this agx reads %d; reload the shell layer (agx shell-init)", f.SchemaVersion, SchemaVersion))
	}
	age := now.Sub(f.CreatedAt)
	if f.CreatedAt.IsZero() || age > MaxAge || age < -clockSkew {
		return File{}, refuse(MsgStaleOrForeign)
	}
	return f, nil
}

// Remove deletes a plan after use (single use: agx launch removes it
// before exec, so a second launch from the same file is impossible).
func Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "remove plan file "+path+": "+err.Error())
	}
	return nil
}

// refuse is a plan-file refusal (exit 125 in agx launch, 1 in plan show).
func refuse(msg string) *apperr.Error { return apperr.Failure(apperr.CodePlanFile, msg) }
