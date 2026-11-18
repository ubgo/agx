package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/ubgo/textdata/atomicfile"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/internal/apperr"
)

// LockWait is how long a writer waits for another agx holding the config
// lock before failing (edge 44: "the second waits 5 s then fails").
const LockWait = 5 * time.Second

// Edit changes the file at path in place under the cross-process lock,
// keeping comments, order and formatting (principle 11). fn gets the
// document; nothing is written when fn returns an error or changes
// nothing. A new file is created with mode (0644 for config.jsonc, 0600
// for tokens.jsonc) and its folder with 0700.
//
// Every write of config.jsonc and tokens.jsonc goes through Edit: the lock
// is advisory, so one writer that skipped it would break the guarantee
// for all (spec G8).
func Edit(ctx context.Context, path string, mode os.FileMode, fn func(d *jsonc.Doc) error) error {
	_, statErr := os.Stat(path)
	existed := statErr == nil
	if !existed {
		if err := os.MkdirAll(dirOf(path), PrivateDirMode); err != nil {
			return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "create "+dirOf(path)+": "+err.Error())
		}
	}
	lctx, cancel := context.WithTimeout(ctx, LockWait)
	defer cancel()
	err := jsonc.Update(lctx, path, fn, jsonc.WithFileName(baseName(path)))
	switch {
	case errors.Is(err, atomicfile.ErrLockTimeout):
		return apperr.Wrap(err, apperr.CodeLocked, apperr.ClassFailure,
			fmt.Sprintf("%s is being written by another agx (waited %s)", baseName(path), LockWait))
	case err != nil:
		var ae *apperr.Error
		if errors.As(err, &ae) {
			return ae
		}
		return apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, err.Error())
	}
	if !existed {
		if err := os.Chmod(path, mode); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "chmod "+path+": "+err.Error())
		}
	}
	return nil
}

// EditConfig is Edit on config.jsonc. When the file is new, fn starts from
// an empty object and the version key is written first.
func EditConfig(ctx context.Context, paths Paths, fn func(d *jsonc.Doc) error) error {
	return Edit(ctx, paths.File, ConfigFileMode, func(d *jsonc.Doc) error {
		if !d.Has("version") {
			if err := d.Set("version", CurrentVersion); err != nil {
				return err
			}
		}
		return fn(d)
	})
}

// dirOf is filepath.Dir without importing it twice in messages.
func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if os.IsPathSeparator(p[i]) {
			if i == 0 {
				return p[:1]
			}
			return p[:i]
		}
	}
	return "."
}

// CreateHome makes a new agent home folder (profile add) with 0700, and
// its parents. An existing folder is left as it is (a shared or already
// logged-in home is fine). Homes are never deleted by agx.
func CreateHome(dir string) error {
	if err := os.MkdirAll(dir, PrivateDirMode); err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "create home "+dir+": "+err.Error())
	}
	return nil
}
