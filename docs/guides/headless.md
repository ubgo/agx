# Headless runs and a first prompt

`--prompt` gives an agent its first prompt. On its own it opens the agent's usual interactive UI with that prompt already sent. Add `--headless` and the agent instead runs the one task without a UI and writes a machine-readable stream of JSON events to stdout, which a script can read. agx maps both flags to each agent's own flags, so one command line works on Claude Code, Codex, Gemini CLI and pi.

You want this for a CI job, a git hook, a cron task or any script that runs an agent on a task and reads what it did, while still getting agx's profile, home, login and environment handling.

## Before you start

- A profile per agent you use (see [Getting started](../getting-started.md)).
- A login the agent can use without a browser: the home's own login, or a stored token login (see [Logins](../logins.md)). A headless run cannot answer a login prompt.
- Decide the [autonomy](autonomy.md) level: a headless agent cannot ask you for approval, so a level that asks will stop at the first action that needs approval.

The examples run against one profile per agent (from [examples/launch-headless](../../examples/launch-headless/README.md)):

<!-- snippet: examples/launch-headless/config.jsonc -->
```jsonc
{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "batch": { "agent": "claude", "home": "~/.claude-work", "claude": { "print": true } },
    "report": { "agent": "claude", "home": "~/.claude-work", "claude": { "print": true, "output_format": "json" } },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true },
    "gemini": { "agent": "gemini", "home": "~/.gemini", "default": true },
    "pi": { "agent": "pi", "home": "~/.pi/agent", "default": true }
  }
}
```

## Step 1: a first prompt in the interactive UI

Why: start the session already working on something, and stay in it.

<!-- snippet: examples/launch-headless/session.txt#prompt -->
```sh
$ agx run --agent claude --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
claude -- "Summarise the README"
```

The prompt is one argument however many spaces or quotes it holds: agx starts the agent without a shell, so nothing needs extra quoting (`--dry-run` quotes it only for display). Claude Code and pi get it after `--`, so a prompt that starts with `-` is never read as a flag. Gemini CLI gets it as `--prompt-interactive`:

<!-- snippet: examples/launch-headless/session.txt#gemini -->
```sh
$ agx run --agent gemini --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
gemini --prompt-interactive "Summarise the README"
$ agx run --agent gemini --headless --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
gemini --output-format stream-json --prompt "Summarise the README"
```

## Step 2: run one task headless

Why: no UI, one task, a JSON event per line on stdout.

<!-- snippet: examples/launch-headless/session.txt#claude -->
```sh
$ agx run --agent claude --headless --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
claude -p --output-format stream-json --verbose -- "Summarise the README"
```

<!-- snippet: examples/launch-headless/session.txt#codex -->
```sh
$ agx run --agent codex --headless --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
codex exec --json "Summarise the README"
```

<!-- snippet: examples/launch-headless/session.txt#pi -->
```sh
$ agx run --agent pi --headless --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
pi --mode json -- "Summarise the README"
```

| Agent | `--headless` becomes | the prompt goes |
|---|---|---|
| Claude Code | `-p --output-format stream-json --verbose` | after `--` |
| Codex | `codex exec --json` | as the last argument |
| Gemini CLI | `--output-format stream-json` | as `--prompt` (without `--headless`: `--prompt-interactive`) |
| pi | `--mode json` | after `--` |
| opencode | `opencode run --format json` | as the last argument |
| a custom agent | its `headless_args` | per its `prompt_args` |

Without `--prompt`, a headless agent reads the task from stdin, so `echo "Fix the failing test" | agx run --headless` works too (Gemini CLI documents this; check your agent's own docs for the others).

## Step 3: a CI-ready command

Why: a script wants a fixed model and a known autonomy level, so the run never depends on a home's settings.

<!-- snippet: examples/launch-headless/session.txt#ci -->
```sh
$ agx run --agent claude --headless --prompt 'Fix the failing test' --autonomy accept-edits --model sonnet --dry-run --quiet --json --field data.command.line
claude -p --output-format stream-json --verbose --model sonnet --permission-mode acceptEdits -- "Fix the failing test"
```

In a real job, drop `--dry-run --quiet --json --field …` and pipe stdout to your reader. agx exec's into the agent, so stdout is the agent's own event stream and the exit code is the agent's own (125 to 127 mean agx refused before the agent started). Its own warnings go to stderr, as in this real launch on the example machine's stand-in `claude`:

<!-- snippet: examples/launch-headless/session.txt#launch -->
```sh
$ agx run --agent claude --headless --prompt 'Summarise the README'
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv [-p] [--output-format] [stream-json] [--verbose] [--] [Summarise the README]
pwd ~/projects/app
env AGX_CONFIG_DIR=~/.agx
env AGX_RUN_AGENT=claude
env AGX_RUN_HOME=~/.claude
env AGX_RUN_ID=20261118-061029-jl37
env AGX_RUN_LOGIN=home
env AGX_RUN_PROFILE=personal
env COLUMNS=400
env HOME=~
env LANG=C
env PATH=/usr/local/bin:/usr/bin:/bin
env PWD=~/projects/app
env TMPDIR=/tmp/agx-demo/tmp
env TZ=UTC
```

A run variable such as `AGX_RUN_PROFILE` in the agent's environment tells a hook which profile it runs on; see [run folders](run-folders.md).

## Step 4: continue a conversation headless

Why: a follow-up task in the same conversation, from a script. `agx resume` takes `--headless` and `--prompt` too, and finds the home that stores the conversation:

<!-- snippet: examples/launch-headless/session.txt#resume -->
```sh
$ agx resume --id 3f2a --headless --prompt 'Continue' --dry-run --quiet --json --field data.command.line
claude -p --output-format stream-json --verbose --resume 3f2a9c1e-0000-4000-8000-000000000001 -- Continue
```

Codex resumes headless with `codex exec resume`. See [`agx resume`](../commands/launch.md#agx-resume).

## Try it

From the repository:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-headless$'
```

On your own machine, without starting anything:

```sh
agx run --agent codex --headless --prompt 'Summarise the README' --dry-run
```

## Pitfalls

- `--headless` and a typed option for the same thing in one command are refused (exit 125). Use one:

<!-- snippet: examples/launch-headless/session.txt#conflict -->
```sh
$ agx run --agent claude --headless --option print=true --dry-run --quiet
agx: --headless and --option print both set the headless; pick one
[exit 125]
```

- A typed headless option in a profile (`"claude": { "print": true }` in a profile `batch`) is not an error: `--headless` on the command line wins and `--dry-run` notes which profile key was not used:

<!-- snippet: examples/launch-headless/session.txt#profile-override -->
```sh
$ agx run --profile batch --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
claude -p -- "Summarise the README"
$ agx run --profile batch --headless --prompt 'Summarise the README' --dry-run --quiet
agent    claude                                                                     from profile batch
profile  batch                                                                      from --profile
home     ~/.claude-work                                                             from profile batch
login    home login                                                                 from default (no login named)
binary   /usr/local/bin/claude version unknown                                      from no install metadata
folder   ~/projects/app                                                             from current folder
note     --headless wins the headless; profile batch claude.print is not used       from one input per concern; the command line wins
run      ~/.agx/runs/20261118-064744-5pz6                                           from AGX_RUN_ID=20261118-064744-5pz6
command  claude -p --output-format stream-json --verbose -- "Summarise the README"  from built by agentio
env      set AGX_RUN_ID=20261118-064744-5pz6                                        from launch environment
         set AGX_RUN_AGENT=claude                                                   from launch environment
         set AGX_RUN_PROFILE=batch                                                  from launch environment
         set AGX_RUN_LOGIN=home                                                     from launch environment
         set AGX_RUN_HOME=~/.claude-work                                            from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work                                       from launch environment
note     installed Claude Code version unknown; flags not checked
```

- `print` and `output_format` are one concern (headless output), so keys of it from one input are one choice: a profile with both (`"claude": { "print": true, "output_format": "json" }`), or `--option print=true --option output_format=json` on one command line, launch together. Two command-line inputs for it (`--headless` and `--option output_format`) are still refused:

<!-- snippet: examples/launch-headless/session.txt#print-format -->
```sh
$ agx run --profile report --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
claude -p --output-format json -- "Summarise the README"
$ agx run --agent claude --option print=true --option output_format=json --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
claude -p --output-format json -- "Summarise the README"
$ agx run --agent claude --headless --option output_format=json --dry-run --quiet
agx: --headless and --option output_format both set the headless; pick one
[exit 125]
```

- A level that asks (`ask`, or `default` when the home's settings ask) cannot be answered in a headless run. Pick `read-only`, `accept-edits` or `full` (with `--consent full-autonomy`) for unattended jobs.
- `--json` and `--field` are agx's own output options. On a real launch agx is gone before it could print anything, so they only apply to `--dry-run` and refusals; the event stream comes from the agent.
- A login that needs a browser cannot complete headless. Use a stored token login (`--login NAME`) or a home that is already logged in.
- The version check: agx refuses a flag the installed agent version provably lacks (Gemini CLI's `--output-format stream-json`, for example) with the version it needs; with the version unknown it passes the flags and warns.

## Related

- Reference: [`agx run`](../commands/launch.md#agx-run) (`--headless`, `--prompt`), [`agx resume`](../commands/launch.md#agx-resume)
- Example: [examples/launch-headless](../../examples/launch-headless/README.md)
- [Autonomy](autonomy.md): what an unattended agent may do
- [Passing anything to an agent](../launch-options.md): model, extra folders, settings, environment
- [Output and exit codes](../output.md#exit-codes)
