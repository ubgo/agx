# Raw args: flags agx passes untouched

Raw args are arguments agx hands to the agent as written, without knowing what they mean. They come from three places, always in this order, and always after everything agx builds itself (a Claude profile's `claude.extra_args` come just before them):

1. `agents.<agent>.defaults.args`: every launch of that agent gets them first, unless the profile sets `"inherit_defaults": false`.
2. `profiles.<profile>.args`: every launch of that profile.
3. Everything after `--` on the command line: this run only.

Raw always wins: when a raw arg repeats something agx also set, both are passed and the agent uses the later one, which is the raw one. The exception is a raw `--settings`, which agx reads and merges into its own settings (raw keys winning), and which is refused only when it cannot be read or sets `apiKeyHelper` next to `--live-login` ([recipe 8](../launch-options.md#a-raw---settings-of-your-own)). The one change agx makes: in args from the config file (1 and 2), `~` is expanded in the value of a path option's flag (see the pitfalls); args after `--` are passed exactly as given.

You want this for a flag agx has no option for (a brand-new vendor flag, a debug switch), for a flag every launch of an agent should get, or for one profile (an API provider on a shared home, say) that must not get the agent's usual flags.

## Before you start

- A profile (see [Getting started](../getting-started.md)).
- Check first whether agx already models the flag: a typed option (`--option`, the profile's agent block) is checked against the installed version and shows in `--dry-run` as an option; see [Passing anything to an agent](../launch-options.md#recipe-7-typed-vendor-launch-flags). Raw args are the escape hatch for everything else.

The examples run against this config (from [examples/launch-raw-args](../../examples/launch-raw-args/README.md)):

<!-- snippet: examples/launch-raw-args/config.jsonc#agents -->
```jsonc
  "agents": {
    "claude": { "defaults": { "args": ["--chrome"] } }
  },
```

<!-- snippet: examples/launch-raw-args/config.jsonc#profiles -->
```jsonc
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work", "args": ["--verbose"] },
    "kimi": { "agent": "claude", "home": "~/.claude-work", "inherit_defaults": false, "args": ["--strict-mcp-config"] }
```

## Step 1: args for every launch of an agent

Why: a flag you always want for Claude Code, whatever the profile.

```jsonc
"agents": { "claude": { "defaults": { "args": ["--chrome"] } } }
```

The dry run shows them on the `args` line with their source:

<!-- snippet: examples/launch-raw-args/session.txt#defaults -->
```sh
$ agx run --profile personal --dry-run --quiet
agent    claude                                 from profile personal
profile  personal                               from --profile
home     ~/.claude                              from profile personal
login    home login                             from default (no login named)
binary   /usr/local/bin/claude version unknown  from no install metadata
folder   ~/projects/app                         from current folder
args     --chrome                               from agents.claude.defaults.args
run      ~/.agx/runs/20261118-061043-j98j       from AGX_RUN_ID=20261118-061043-j98j
command  claude --chrome                        from built by agentio
env      set AGX_RUN_ID=20261118-061043-j98j    from launch environment
         set AGX_RUN_AGENT=claude               from launch environment
         set AGX_RUN_PROFILE=personal           from launch environment
         set AGX_RUN_LOGIN=home                 from launch environment
         set AGX_RUN_HOME=~/.claude             from launch environment
note     installed Claude Code version unknown; flags not checked
```

## Step 2: args for one profile

Why: one account needs a flag the others do not. A profile's `args` come after the agent defaults:

<!-- snippet: examples/launch-raw-args/session.txt#profile -->
```sh
$ agx run --profile work --dry-run --quiet --json --field data.command.line
claude --chrome --verbose
```

## Step 3: args for one run, after `--`

Why: try a flag once. Everything after `--` goes to the agent untouched, after the defaults and the profile's args:

<!-- snippet: examples/launch-raw-args/session.txt#after -->
```sh
$ agx run --profile work --dry-run --quiet --json --field data.command.line -- --add-dir ../shared
claude --chrome --verbose --add-dir ../shared
```

Even a flag agx would otherwise build itself passes this way, unchecked:

<!-- snippet: examples/launch-raw-args/session.txt#dashes -->
```sh
$ agx run --profile personal --dry-run --quiet --json --field data.command.line -- --resume
claude --chrome --resume
```

## Step 4: a profile without the agent defaults

Why: several profiles share one agent, and one of them (an API provider such as `kimi` on a shared Claude home) must not get the flags your plan accounts use. `"inherit_defaults": false` drops `agents.<agent>.defaults.args` for that profile only; its own `args` still apply:

```jsonc
"kimi": { "agent": "claude", "home": "~/.claude-work", "inherit_defaults": false, "args": ["--strict-mcp-config"] }
```

<!-- snippet: examples/launch-raw-args/session.txt#no-inherit -->
```sh
$ agx run --profile kimi --dry-run --quiet --json --field data.command.line
claude --strict-mcp-config
```

## Step 5: change a profile's args from the command line

Why: no hand-editing of the config. `agx profile edit --args` replaces the profile's args (repeat it, one arg each; `--args ''` removes them), and `--no-inherit-defaults` / `--inherit-defaults` set `inherit_defaults`. The edit prints what changed:

<!-- snippet: examples/launch-raw-args/session.txt#edit -->
```sh
$ agx profile edit --id work --no-inherit-defaults --args --verbose --args --debug
profile work: changed
  args              --verbose  ->  --verbose --debug
  inherit_defaults  true       ->  false
$ agx run --profile work --dry-run --quiet --json --field data.command.line
claude --verbose --debug
$ agx profile edit --id work --args ''
profile work: changed
  args                 --verbose --debug  ->  —
$ agx profile edit --id work --inherit-defaults --no-inherit-defaults
agx: --inherit-defaults and --no-inherit-defaults both set inherit_defaults; pick one
[exit 2]
```

`profile edit` takes `--dry-run` to show the change without writing it. See [`agx profile edit`](../commands/profile.md#agx-profile-edit).

## Try it

From the repository:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-raw-args$'
```

On your own machine, without starting anything:

```sh
agx run --dry-run -- --verbose
```

## Pitfalls

- A raw arg that repeats a typed choice is not refused. Both are passed, the raw one last, and `--dry-run` notes it:

<!-- snippet: examples/launch-raw-args/session.txt#duplicate -->
```sh
$ agx run --profile personal --model opus --dry-run --quiet -- --model sonnet
agent    claude                                       from profile personal
profile  personal                                     from --profile
home     ~/.claude                                    from profile personal
login    home login                                   from default (no login named)
binary   /usr/local/bin/claude version unknown        from no install metadata
folder   ~/projects/app                               from current folder
options  model=opus                                   from --model
args     --chrome --model sonnet                      from agents.claude.defaults.args, after --
run      ~/.agx/runs/20261118-061043-67cw             from AGX_RUN_ID=20261118-061043-67cw
command  claude --model opus --chrome --model sonnet  from built by agentio
env      set AGX_RUN_ID=20261118-061043-67cw          from launch environment
         set AGX_RUN_AGENT=claude                     from launch environment
         set AGX_RUN_PROFILE=personal                 from launch environment
         set AGX_RUN_LOGIN=home                       from launch environment
         set AGX_RUN_HOME=~/.claude                   from launch environment
note     raw --model repeats a typed model; both are passed and the agent uses the raw one (last)
         installed Claude Code version unknown; flags not checked
```

- A raw `--settings` is the one raw flag agx does not pass as written. Claude Code keeps only the last `--settings` it gets, wholesale, so agx reads yours (inline JSON, or a JSONC file relative to the launch folder), merges it over its own settings key by key (objects merge, anything else replaces), takes it out of the args and passes exactly one `--settings`. Several raw values merge in argv order. The dry run names your keys `from raw --settings` and the `args` line no longer shows the value (details in [launch options, recipe 8](../launch-options.md#a-raw---settings-of-your-own)):

<!-- snippet: examples/launch-raw-args/session.txt#raw-settings -->
```sh
$ agx run --profile personal --setting cleanupPeriodDays=30 --dry-run --quiet -- --settings '{"theme":"dark"}'
agent     claude                                                                                          from profile personal
profile   personal                                                                                        from --profile
home      ~/.claude                                                                                       from profile personal
login     home login                                                                                      from default (no login named)
binary    /usr/local/bin/claude version unknown                                                           from no install metadata
folder    ~/projects/app                                                                                  from current folder
args      --chrome                                                                                        from agents.claude.defaults.args
settings  cleanupPeriodDays                                                                               from --setting
          theme                                                                                           from raw --settings
          delivered as file ~/.agx/runs/20261118-091339-fb1a/settings-20261118-091339-fb1a.json (0600)    from settings_delivery file
run       ~/.agx/runs/20261118-091339-fb1a                                                                from AGX_RUN_ID=20261118-091339-fb1a
command   claude --settings ~/.agx/runs/20261118-091339-fb1a/settings-20261118-091339-fb1a.json --chrome  from built by agentio
env       set AGX_RUN_ID=20261118-091339-fb1a                                                             from launch environment
          set AGX_RUN_AGENT=claude                                                                        from launch environment
          set AGX_RUN_PROFILE=personal                                                                    from launch environment
          set AGX_RUN_LOGIN=home                                                                          from launch environment
          set AGX_RUN_HOME=~/.claude                                                                      from launch environment
note      installed Claude Code version unknown; flags not checked
```

- `~` in config args is expanded only after a path option's flag. No shell runs between agx and the agent, so in `profiles.<profile>.args` and `agents.<agent>.defaults.args` agx expands a leading `~` in the argument after a flag of an option agentio marks as a path (Claude Code: `--add-dir`, `--mcp-config`, `--system-prompt-file`, `--append-system-prompt-file`, `--plugin-dir`, `--debug-file`; Codex: `-C`, `--add-dir`, `-i`, written exactly as those flags), or after its `=` (`--debug-file=~/d.log`). Any other argument, `~user`, `$VAR`, and everything after `--` stay as written (a raw `--settings` value is read by agx itself, so its leading `~` is expanded wherever it comes from). Gemini CLI, pi and config-defined agents have no typed options, so nothing of theirs is expanded: write absolute paths there.
- A Claude profile's `claude.extra_args` and other raw args combine: `extra_args` first, then agent defaults, profile `args` and args after `--`, so the later one wins when they repeat a flag.
- Raw args are never checked against the installed version and never type-checked; a misspelled flag reaches the agent, which reports it. Prefer `--option` for flags agx models.
- Raw args are visible in `ps` like any argument. Put a secret in an environment variable (`--env`, profile `env_refs`) or a login, never in `args`.
- `--inherit-defaults` and `--no-inherit-defaults` together are refused: `agx: --inherit-defaults and --no-inherit-defaults both set inherit_defaults; pick one` (exit 2).
- `agx` flags go before `--`. After it, even `--dry-run` is passed to the agent instead of being read by agx.
- A raw arg does not change agx's own decisions: `-- --model x` does not change the `options` line or the profile, and a raw permission flag does not change the [autonomy](autonomy.md) agx reports; the agent just sees both.

## Related

- Reference: [`agx run`](../commands/launch.md#agx-run) (`--`), [`agx profile edit`](../commands/profile.md#agx-profile-edit), [config: agents](../config.md#agents) (`defaults.args`), [config: profiles](../config.md#profiles) (`args`, `inherit_defaults`)
- Example: [examples/launch-raw-args](../../examples/launch-raw-args/README.md)
- [Passing anything to an agent](../launch-options.md): typed options, settings and the one-winner rule
- [How agx decides](../how-agx-decides.md): where each decision comes from
