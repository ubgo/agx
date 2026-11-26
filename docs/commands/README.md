# Command reference

Every agx command and flag. The docs gate builds agx's command tree and fails when a command or flag is missing here, so this reference is complete for the version it ships with.

## Rules every command follows

- **Every input is a named flag**, in any order, as `--name value` or `--name=value`. A bare word is a usage error that prints the named form.
- **`--id` is the object the verb acts on.** Other objects keep their noun: `--profile`, `--login`, `--agent`, `--dir`. Create verbs take `--name`.
- **Lists repeat the flag:** `--env A=1 --env B=2`.
- **`--` ends agx's flags**; everything after it goes to the agent untouched.
- **A bare `agx`, or a bare group such as `agx profile`, prints help.** agx never picks an action for you.
- **Two choices for one thing in one command are refused** ("pick one"), never guessed.
- **`--profile` and `--login` select** on `run`, `new` and `resume`, and **filter** (repeatable) on `usage` and lists.

Each command lists its cost class: **R** reads disk, **N** network, **Q** spends quota, **X** runs the agent binary or a secret command, **W** writes files. See [cost classes](../output.md#cost-classes).

## Global flags

These work on every command.

| Flag | Meaning |
|---|---|
| `--json` | print the [JSON envelope](../output.md#the-json-envelope) instead of text |
| `--field` | keep only this path of the envelope (repeat); on a table, `data.*.KEY` picks and orders columns. See [`--field`](../output.md#--field-keep-only-what-you-need) |
| `--quiet` | no stderr notes (what decided a value); warnings and errors still print |
| `--color` | colour, decided per stream: the human output on stdout (bold headers, dim indices, coloured statuses, usage bars) and agx's error (red) and warning (yellow) lines on stderr. `auto` (default: that stream is a terminal and `NO_COLOR` unset or empty), `always` (even with `NO_COLOR`) or `never`. `--json` is never coloured; another value is refused (exit 2) |
| `--config` | the config file to use; default `config.jsonc` in `$AGX_CONFIG_DIR`, else `$AGX_HOME`, else `~/.agx`. `tokens.jsonc` and `runs/` live next to it |
| `--date-format` | how dates and times read in human output: a preset (`iso` "2026-11-19 23:30", `short` "Thu 23:30", `long` "Thu 19 Nov 23:30", `us` "Nov 19 11:30 PM") or tokens (`YYYY`, `MM`, `MMM`, `DD`, `D`, `ddd`, `dddd`, `HH`, `H`, `hh`, `h`, `mm`, `ss`, `a`, `A`, words in `[brackets]`). Default config `date_format` (for `usage`, `usage.date_format` first), else `long` in `usage` and `iso` in table timestamps and `conversation show`. Relative times ("3 min ago") and `--json` never change; an unknown token or an empty value is refused (exit 2). See [change how times read](../guides/usage.md#step-4-change-how-times-read) |
| `--timeout` | per-request network timeout for live usage, login probes and auto-pick, as a duration (`5s`, `1m`); default `15s`, `0` is the default |
| `--help` | help for the command |

The line width of human tables is the terminal's, else `COLUMNS`, else 120 (never below 60). A wrong flag value is a usage error: exit 2, or 125 on a launching command.

<!-- snippet: examples/out-scripting/session.txt#timeout -->
```sh
$ agx usage --agent gemini --timeout 5s --json --field data.*.status
["unsupported"]

$ agx usage --timeout soon
agx: invalid argument "soon" for "--timeout" flag: must be a duration like "30s" or "5m"
[exit 2]
```

The [Scripting with agx](../guides/scripting.md) guide uses every global flag; [`examples/out-scripting`](../../examples/out-scripting/README.md) runs them.

## Commands

| Group | Commands |
|---|---|
| [Launching](launch.md) | `run`, `new`, `resume`, `whoami` |
| [Usage](usage.md) | `usage` |
| [Conversations and sessions](conversation.md) | `conversation list/show/move`, `session list/clean/promote` |
| [Live login](live.md) | `live list/show/use`, `statusline`, and the hidden `credential` the agent runs |
| [Profiles](profile.md) | `profile list/show/add/import/edit/remove/use` |
| [Logins](login.md) | `login list/show/add/edit/remove/use/check/token` |
| [Pools](pool.md) | `pool list/show/pick/preset/prune`; the pool flags of other commands: `--pool-force` and `--pool-only` (with `--pool-only-short`, `--pool-only-week`) on `run`, `new`, `resume`, `pool pick` and `login token`, `--pool-only` on `live use` |
| [Projects](project.md) | `project list/show/use/remove` |
| [Setup and tools](setup.md) | `agent list/show`, `doctor`, `clean`, `config init/show/path/check/set`, `alias list/add/remove`, `shell-init`, `completion`, `help`, `version` |
