// Package app holds agx's use cases: one method per command, typed
// options in, typed results out. It never prints, never imports cobra or
// the renderer, and never starts a process (internal/archtest): the CLI
// layer parses flags into options and renders results; internal/run
// starts the agent.
//
// Why the split: every command must be testable without a terminal and
// must give the same typed data to the human table, --json and --field.
package app

import (
	"context"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/picker"
	"github.com/ubgo/agx/login"
)

// Deps is everything the app reads from the machine, injected so tests
// run against fixture homes (spec: tests never touch real accounts).
type Deps struct {
	// Host is the agentio host (user home, env, clock, HTTP, processes).
	Host agentio.Host
	// Lookup reads agx's own process environment (AGX_*, references).
	Lookup config.Lookup
	// Cwd is the current folder (absolute).
	Cwd string
	// ConfigFlag is --config ("" = the default dir rules).
	ConfigFlag string
	// Picker asks questions at a terminal; nil = no terminal.
	Picker picker.Picker
	// Stdin is agx's stdin (login add --stdin).
	Stdin io.Reader
	// Self is the path of the running agx binary (live-login helper).
	Self string
	// GopassBinary overrides the gopass program ("" = gopass on PATH).
	GopassBinary string
	// Timeout is the per-request network timeout (--timeout).
	Timeout time.Duration
	// AliasCheck validates one alias value against the command tree (the
	// CLI supplies it: only it knows the commands); nil = unchecked.
	AliasCheck func(args string) error
}

// App is one agx invocation's use cases.
type App struct {
	d     Deps
	paths config.Paths

	once   sync.Once
	cfg    *config.Loaded
	reg    *agents.Registry
	cfgErr error

	exp *login.Expander

	// foldOS is the OS whose folder-case rule dirKey applies
	// (runtime.GOOS; a test sets "darwin" to check the macOS rule on any
	// machine).
	foldOS string
}

// New builds the App; the config is loaded on first use, so commands that
// work without a valid config (config path, config check, help, version)
// never fail on it.
func New(d Deps) *App {
	if d.Timeout == 0 {
		d.Timeout = DefaultTimeout
	}
	return &App{
		d:     d,
		paths: config.ResolvePaths(d.ConfigFlag, d.Lookup, d.Host.UserHome),
		exp:   login.NewExpander(d.Lookup, d.GopassBinary),

		foldOS: runtime.GOOS,
	}
}

// DefaultTimeout is the per-request network timeout (spec 13 globals).
const DefaultTimeout = 15 * time.Second

// Paths are the config files this invocation uses.
func (a *App) Paths() config.Paths { return a.paths }

// Deps returns the injected dependencies (for the CLI layer's pickers).
func (a *App) Deps() Deps { return a.d }

// Now is the host clock.
func (a *App) Now() time.Time {
	if a.d.Host.Now != nil {
		return a.d.Host.Now()
	}
	return time.Now()
}

// load reads the config once and builds the agent registry from it.
func (a *App) load() {
	a.once.Do(func() {
		known := config.Known{Agents: agents.BuiltinIDs(), Blocks: agents.BlockSchemas(), Alias: a.d.AliasCheck, PoolAgent: agents.PoolReason}
		a.cfg, a.cfgErr = config.Load(a.paths, known)
		if a.cfgErr != nil {
			return
		}
		custom := map[agentio.ID]agentio.Spec{}
		for id, ac := range a.cfg.Config.Agents {
			if ac.Spec != nil {
				custom[id] = *ac.Spec
			}
		}
		a.reg, a.cfgErr = agents.New(a.d.Host, custom)
		if a.cfgErr != nil {
			a.cfgErr = apperr.Wrap(a.cfgErr, apperr.CodeConfig, apperr.ClassFailure, a.cfgErr.Error())
		}
	})
}

// Config returns the loaded config (an error for an invalid file).
func (a *App) Config() (*config.Loaded, error) {
	a.load()
	return a.cfg, a.cfgErr
}

// Registry returns the agent registry (needs a valid config).
func (a *App) Registry() (*agents.Registry, error) {
	a.load()
	return a.reg, a.cfgErr
}

// Reload drops the cached config after a write, so a later step in the
// same invocation sees the change.
func (a *App) Reload() {
	a.once = sync.Once{}
	a.cfg, a.reg, a.cfgErr = nil, nil, nil
}

// Expander is the strict reference expander.
func (a *App) Expander() *login.Expander { return a.exp }

// ctxErr maps a cancelled context (Ctrl-C) to the interrupted error.
func ctxErr(ctx context.Context) error {
	if ctx.Err() != nil {
		return apperr.Wrap(ctx.Err(), apperr.CodeInterrupted, apperr.ClassInterrupted, "interrupted")
	}
	return nil
}

// statDir reports an existing directory.
func statDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
