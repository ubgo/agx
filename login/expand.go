// Package login turns agx logins into credentials: it expands token
// references strictly (textdata value.Expand with WithStrict), reads
// pasted values from tokens.jsonc, cleans the result, and maps every
// failure to a message that names the login and the reason, never the
// value (spec 7.7, 8, edges 16 to 20).
package login

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/plugins/gopass"
	"github.com/ubgo/textdata/shell"
	"github.com/ubgo/textdata/value"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// CommandTimeout bounds every $(…) and every plugin call (gopass) during
// expansion (spec 7.7): a locked or slow store stops the launch instead of
// hanging it (edge 16).
const CommandTimeout = 30 * time.Second

// Expander expands references strictly. The zero value is not usable;
// build it with NewExpander.
type Expander struct {
	lookup  config.Lookup
	timeout time.Duration
	// gopassBinary overrides the gopass program (tests point it at a fake).
	gopassBinary string
	// brief drops what a store or command printed on stderr from errors
	// (see Brief).
	brief bool
}

// NewExpander builds an Expander over lookup (the process env).
// gopassBinary "" = "gopass" on PATH.
func NewExpander(lookup config.Lookup, gopassBinary string) *Expander {
	return &Expander{lookup: lookup, timeout: CommandTimeout, gopassBinary: gopassBinary}
}

// WithTimeout returns a copy whose commands and plugin calls stop after d
// instead of CommandTimeout (tests prove edge 16 without a 30 s wait).
func (e *Expander) WithTimeout(d time.Duration) *Expander {
	c := *e
	c.timeout = d
	return &c
}

// Brief returns a copy whose errors name the login, the reference and
// what failed, but never repeat a store's or a command's own stderr. agx
// credential uses it: its one error line is shown by the agent (Claude's
// /status), and that stderr can name the store's files ("gpg: decryption
// failed: <store>/x.gpg"). agx login check keeps the full reason.
func (e *Expander) Brief() *Expander {
	c := *e
	c.brief = true
	return &c
}

// Expand resolves every reference in s. commands false turns $(…) into an
// error (env files without env_file_commands). label names the source in
// errors ("login personal", "--env API_KEY"). A string without "$" is
// returned as written (a plain value is allowed anywhere, principle 3).
func (e *Expander) Expand(ctx context.Context, label, s string, commands bool) (string, error) {
	if !strings.Contains(s, "$") {
		return s, nil
	}
	opts := []value.Option{
		value.WithStrict(),
		value.WithLookup(e.lookup),
		value.WithPlugins(e.plugins(ctx)...),
	}
	if commands {
		opts = append(opts, value.WithRunner(shell.Runner{Timeout: e.timeout}))
	}
	out, err := value.Expand(ctx, s, opts...)
	if err != nil {
		return "", expandError(label, err, e.timeout, e.brief)
	}
	return out, nil
}

// plugins are the reference plugins every expansion gets: gopass today
// (spec 15.4; vault, aws and file when added). Built per call because
// each carries the call's context and timeout.
func (e *Expander) plugins(ctx context.Context) []plugin.Plugin {
	gp := []gopassplugin.Option{gopassplugin.WithTimeout(e.timeout), gopassplugin.WithContext(ctx)}
	if e.gopassBinary != "" {
		gp = append(gp, gopassplugin.WithBinary(e.gopassBinary))
	}
	return []plugin.Plugin{gopassplugin.New(gp...)}
}

// Schemes lists the ${scheme:ref} schemes this expander resolves, sorted,
// so doctor can say "no plugin for scheme X" without resolving anything.
func (e *Expander) Schemes() []string {
	var out []string
	for _, p := range e.plugins(context.Background()) {
		if r, ok := p.(plugin.Resolver); ok {
			out = append(out, r.Schemes()...)
		}
	}
	slices.Sort(out)
	return out
}

// pluginResolveOp is the start of a plugin error's Op when it resolved a
// ${scheme:ref} (textdata value: "resolve gopass:a/b"); the rest is the
// reference as written.
const pluginResolveOp = "resolve "

// ExpandError maps a strict expansion failure to an apperr naming label
// and the reason, never a value.
func ExpandError(label string, err error, timeout time.Duration) error {
	return expandError(label, err, timeout, false)
}

// expandError is ExpandError; brief leaves out what a store or a command
// printed on stderr (Expander.Brief).
func expandError(label string, err error, timeout time.Duration, brief bool) error {
	var (
		plug    *plugin.Error
		unset   *expand.UnsetError
		scheme  *expand.UnknownSchemeError
		off     *expand.CommandOffError
		cmdErr  *expand.CommandError
		exitErr *shell.ExitError
		req     *expand.RequiredError
	)
	msg := ""
	switch {
	case errors.Is(err, context.Canceled):
		return apperr.Wrap(err, apperr.CodeInterrupted, apperr.ClassInterrupted, label+": interrupted")
	case errors.As(err, &unset):
		msg = fmt.Sprintf("%s: %s is not set", label, unset.Name)
	case errors.As(err, &scheme):
		msg = fmt.Sprintf("%s: no plugin for scheme %s", label, scheme.Scheme)
	case errors.As(err, &off):
		msg = fmt.Sprintf("%s: $(%s) is a command, and commands are off here", label, off.Cmd)
	case isTimeout(err):
		msg = fmt.Sprintf("%s: %s timed out after %s", label, timedOutWhat(err), timeout)
	case errors.As(err, &plug) && errors.Is(err, plugin.ErrNotFound):
		msg = fmt.Sprintf("%s: %s is not in the %s store", label, strings.TrimPrefix(plug.Op, pluginResolveOp), plug.Plugin)
	case brief && errors.As(err, &plug):
		msg = fmt.Sprintf("%s: %s could not be read by %s", label, strings.TrimPrefix(plug.Op, pluginResolveOp), plug.Plugin)
	case errors.As(err, &exitErr):
		msg = fmt.Sprintf("%s: command %q failed: exit %d", label, exitErr.Cmd, exitErr.Code)
		if exitErr.Stderr != "" && !brief {
			msg += ": " + firstLine(exitErr.Stderr)
		}
	case errors.As(err, &cmdErr):
		msg = fmt.Sprintf("%s: command %q failed", label, cmdErr.Cmd)
	case errors.As(err, &req):
		msg = fmt.Sprintf("%s: %s", label, req.Error())
	default:
		msg = fmt.Sprintf("%s: %s", label, err.Error())
	}
	return apperr.Wrap(err, apperr.CodeSecret, apperr.ClassFailure, msg)
}

// timedOutWhat names what timed out: gopass for a gopass reference, else
// the command.
func timedOutWhat(err error) string {
	if strings.Contains(err.Error(), "gopass") {
		return "gopass"
	}
	return "the command"
}

// firstLine is the first line of s.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// Clean trims a resolved token and refuses an empty or multi-line value
// (edge 17). label names the login.
func Clean(label, tok string) (string, error) {
	t := strings.TrimSpace(tok)
	if t == "" {
		return "", apperr.Failure(apperr.CodeSecret, label+": token is empty")
	}
	if n := strings.Count(t, "\n") + 1; n > 1 {
		return "", apperr.Failure(apperr.CodeSecret, fmt.Sprintf("%s: token has %d lines", label, n))
	}
	return t, nil
}

// Timeout texts in errors that lost their chain: textdata's gopass plugin
// reports a deadline as text inside its "plugin unavailable" error, so
// errors.Is cannot see it.
var timeoutTexts = []string{"timed out", context.DeadlineExceeded.Error()}

// isTimeout reports a command or plugin that hit the runner timeout.
func isTimeout(err error) bool {
	if errors.Is(err, shell.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	for _, t := range timeoutTexts {
		if strings.Contains(err.Error(), t) {
			return true
		}
	}
	return false
}
