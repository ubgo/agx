//go:build !windows

package run

import (
	"errors"
	"os"
	"syscall"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
)

// ParentStays is false on Unix: agx is replaced by the agent (exec).
const ParentStays = false

// Start replaces agx with the agent: chdir into c.Dir, then execve(path,
// argv, c.Env). On success it never returns. path is the resolved binary
// (Agent.LookPath); argv[0] is c.Path so the agent sees its own name.
// The returned code is only meaningful on Windows.
func Start(path string, c agentio.Command) (int, error) {
	if c.Dir != "" {
		if err := os.Chdir(c.Dir); err != nil {
			return 0, apperr.Wrap(err, apperr.CodeFolderGone, apperr.ClassFailure, "cannot enter "+c.Dir+": "+err.Error())
		}
	}
	argv := append([]string{c.Path}, c.Args...)
	err := syscall.Exec(path, argv, c.Env)
	switch {
	case errors.Is(err, syscall.ENOENT):
		return 0, apperr.Wrap(err, apperr.CodeBinaryMissing, apperr.ClassNotFoundBinary, path+" not found")
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.ENOEXEC):
		return 0, apperr.Wrap(err, apperr.CodeBinaryNotExecutable, apperr.ClassNotExecutable, path+" cannot run: "+err.Error())
	}
	return 0, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "exec "+path+": "+err.Error())
}
