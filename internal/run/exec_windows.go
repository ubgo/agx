//go:build windows

package run

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
)

// ParentStays is true on Windows: there is no exec, so agx stays as the
// parent process (an OS limit, stated in --dry-run).
const ParentStays = true

// Start runs the agent as a child with agx's stdio, forwards Ctrl-C (the
// console delivers it to both; agx ignores it so it can wait and return
// the agent's code), and returns the agent's exit code.
func Start(path string, c agentio.Command) (int, error) {
	cmd := exec.Command(path, c.Args...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		for range sig {
			// The console sends Ctrl-C to the whole process group; the agent
			// handles it. agx only keeps waiting.
		}
	}()
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exitErr):
		return exitErr.ExitCode(), nil
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, os.ErrNotExist):
		return 0, apperr.Wrap(err, apperr.CodeBinaryMissing, apperr.ClassNotFoundBinary, path+" not found")
	}
	return 0, apperr.Wrap(err, apperr.CodeBinaryNotExecutable, apperr.ClassNotExecutable, path+" cannot run: "+err.Error())
}
