package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Limits of a filter command's output: a picker prints one login name, so
// anything bigger is a mistake, never data to keep.
const (
	// maxFilterOutput caps stdout.
	maxFilterOutput = 64 << 10
	// maxFilterStderr caps the stderr kept for the error message.
	maxFilterStderr = 512
	// filterWaitDelay is how long a filter's leftover children may keep
	// its pipes open after it exits or is killed.
	filterWaitDelay = time.Second
)

// ErrFilterTimeout is a filter command that ran past its timeout.
var ErrFilterTimeout = errors.New("timed out")

// Filter runs cmdline through the system shell (sh -c; cmd /C on
// Windows) with stdin as its standard input and env as its environment
// (nil = agx's), and returns its stdout. A login pool's picker command
// runs this way: it reads the members' usage as JSON and prints a name.
// A non-zero exit is an error with the exit code and the first line of
// stderr; a run past timeout is ErrFilterTimeout.
func Filter(ctx context.Context, cmdline string, env []string, stdin []byte, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	shell, flag := "/bin/sh", "-c"
	if runtime.GOOS == goosWindows {
		shell, flag = "cmd", "/C"
	}
	c := exec.CommandContext(ctx, shell, flag, cmdline)
	c.Env = env
	c.Stdin = bytes.NewReader(stdin)
	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = maxFilterOutput, maxFilterStderr
	c.Stdout, c.Stderr = &stdout, &stderr
	c.WaitDelay = filterWaitDelay
	err := c.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", fmt.Errorf("%w after %s", ErrFilterTimeout, timeout)
	case err == nil, errors.Is(err, exec.ErrWaitDelay):
		if stdout.over {
			return "", fmt.Errorf("printed more than %d bytes", maxFilterOutput)
		}
		return stdout.String(), nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		msg := fmt.Sprintf("exited %d", ee.ExitCode())
		if line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n"); line != "" {
			msg += ": " + line
		}
		return "", errors.New(msg)
	}
	return "", err
}

// goosWindows is runtime.GOOS on Windows.
const goosWindows = "windows"

// limitedBuffer keeps the first max bytes written and notes the rest.
type limitedBuffer struct {
	bytes.Buffer
	max  int
	over bool
}

// Write keeps what fits and reports every byte written, so the command
// never blocks on a full pipe.
func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room < len(p) {
		b.over = true
		if room > 0 {
			b.Buffer.Write(p[:room])
		}
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

var _ io.Writer = (*limitedBuffer)(nil)
