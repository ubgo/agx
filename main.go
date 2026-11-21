// Command agx is one CLI for every coding-agent CLI on the machine
// (Claude Code, Codex, Gemini CLI, pi and config-defined agents) across
// every account: usage, launch and resume, logins, project bindings,
// session folders and conversations. See README.md.
//
// main only wires the process: a signal-aware context, the real
// environment and the exit code. Everything else lives in packages.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	// Pool schedules name IANA time zones; embedding the database keeps
	// them working where the system has none (Windows, slim containers).
	_ "time/tzdata"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/cli"
)

func main() { os.Exit(run()) }

// run returns the exit code so deferred cleanup runs before os.Exit.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	env, err := cli.SystemEnv(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, cli.Name+": "+err.Error()) // stderr is the last place to report
		return apperr.ExitFailure
	}
	return cli.Main(ctx, env)
}
