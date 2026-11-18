//go:build !windows

package plan

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedBySelf reports a file owned by this process's user (edge 53).
func ownedBySelf(st fs.FileInfo) bool {
	sys, ok := st.Sys().(*syscall.Stat_t)
	return ok && int(sys.Uid) == os.Getuid()
}
