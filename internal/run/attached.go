package run

import (
	"context"
	"errors"
	"io"
	"os/exec"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
)

// RunAttached runs c as a child of agx with stdin, stdout and stderr
// attached (an interactive vendor command such as claude setup-token), in
// c.Dir with c.Env (nil Env = agx's environment), and waits. path is the
// resolved binary (Agent.LookPath); argv[0] is c.Path. A non-zero exit is
// returned as code with a nil error; err is only for a start failure
// (missing: 127-class, not executable: 126-class, as Start) or ctx
// cancellation (CodeInterrupted). Ctrl-C reaches the child through the
// shared process group; agx keeps waiting for it.
func RunAttached(ctx context.Context, path string, c agentio.Command, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, path, c.Args...)
	cmd.Args[0] = c.Path
	cmd.Env, cmd.Dir = c.Env, c.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case ctx.Err() != nil:
		return 0, apperr.Wrap(ctx.Err(), apperr.CodeInterrupted, apperr.ClassInterrupted, "interrupted")
	case errors.As(err, &exitErr):
		return exitErr.ExitCode(), nil
	case errors.Is(err, exec.ErrNotFound):
		return 0, apperr.Wrap(err, apperr.CodeBinaryMissing, apperr.ClassNotFoundBinary, path+" not found")
	}
	return 0, apperr.Wrap(err, apperr.CodeBinaryNotExecutable, apperr.ClassNotExecutable, path+" cannot run: "+err.Error())
}
