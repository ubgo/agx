//go:build windows

package plan

import "io/fs"

// ownedBySelf is true on Windows: file ownership there is an ACL, not a
// uid, and the plan lives in the user's own %TEMP%, which other users
// cannot write. Staleness is still checked.
func ownedBySelf(fs.FileInfo) bool { return true }
