# Setup and tools

The commands that describe agents, check and edit the config, clean up, and set up the shell. Guides: [Agents and what they support](../guides/agents.md), [A custom agent](../guides/custom-agent.md), [Check your setup](../guides/doctor.md), [Scripting with agx](../guides/scripting.md).

## agx agent list

Known agents: built in or custom, binary, version from install metadata, tested range, compat and capabilities. Cost R: no binary runs. No flags of its own.

```sh
agx agent list
```

<!-- runnable -->
```sh
$ agx agent list --json --field data.*.id
["claude","codex","gemini","pi"]
```

`COMPAT` is `in-range`, `newer`, `older` or `unknown` (no version in the install metadata, or a custom agent, which has no tested range). A version outside the range is a warning, never a refusal.

| Agent | Usage | Conversations | Resume | Token login | Autonomy | Settings | Live login |
|---|---|---|---|---|---|---|---|
| claude | yes | yes | yes | yes | every level | yes | yes |
| codex | yes | listed, never moved | yes | no | not `accept-edits` | yes (`-c`) | no |
| gemini | no | yes | yes | yes | every level | no | no |
| pi | no | yes | yes | yes | `default`, `read-only` and `full` | no | no |
| custom | no | no | no | no (use `env_refs`) | `default` plus `autonomy_args` | no | no |

Try it: [`examples/agent-capabilities`](../../examples/agent-capabilities/README.md) (`list` region).

<!-- snippet: examples/agent-capabilities/session.txt#ids -->
```sh
$ agx agent list --json --field data.*.id
["claude","codex","gemini","pi"]
```

Pitfalls: `VERSION` `unknown` only means the install has no metadata agx reads; `agx agent show --vendor-checks` asks the binary. Guide: [see every agent](../guides/agents.md#step-1-see-every-agent).

## agx agent show

One agent, with each capability and the reason when one is missing, the autonomy levels a launch accepts, the token login kinds, and shadowed copies of the binary on `PATH`. Cost R.

| Flag | Meaning |
|---|---|
| `--id` | the agent |
| `--vendor-checks` | also run the binary's `--version` (cost X); the answer is `vendor_version`, or `vendor_error` |

```sh
agx agent show --id claude --vendor-checks
```

Every missing capability carries its reason:

<!-- runnable -->
```sh
$ agx agent show --id pi
agent: pi (pi, built-in)
...
autonomy: default, read-only, full
token kinds: api-key
...
usage               no         network  pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
...
```

Try it, the levels and kinds as data:

<!-- snippet: examples/agent-capabilities/session.txt#levels -->
```sh
$ agx agent show --id pi --json --field data.autonomy --field data.token_kinds
{
  "data.autonomy": [
    "default",
    "read-only",
    "full"
  ],
  "data.token_kinds": [
    "api-key"
  ]
}
```

Pitfalls:

- An unknown id exits 1 and lists the known ones:

  <!-- snippet: examples/agent-capabilities/session.txt#unknown -->
  ```sh
  $ agx agent show --id cursor
  agx: unknown agent cursor; known: claude, codex, gemini, pi
  [exit 1]
  ```

- `--vendor-checks` starts the agent binary; keep it out of loops and status bars.

Guide: [look at one agent](../guides/agents.md#step-2-look-at-one-agent). Example: [`examples/agent-capabilities`](../../examples/agent-capabilities/README.md).

## agx doctor

Local checks, with no network, no secret resolution and no agent binary run unless asked. Cost R. It checks:

- the config file: schema and cross-checks, or a missing file (information, with the fix `agx config init`);
- each reference parses, its scheme has a plugin, each command's program is on `PATH`, each `${VAR}` is set and each `env_file` exists;
- plain token values in `config.jsonc`, listed by key as information;
- the mode of `tokens.jsonc`, and a value for each `stored` login;
- run folders whose agent has ended (reported, never deleted; `agx clean` deletes);
- whether the shell layer is loaded (`AGX_SHELL`);
- each agent binary, its version from install metadata against the tested range, and shadowed copies;
- each configured home: it exists and looks like a home, its login, and the agent's own notes.

| Flag | Meaning |
|---|---|
| `--agent` | only this agent's binary and home rows (the config, reference, files, runs and shell rows always show) |
| `--resolved-secrets` | also resolve every reference (cost X); values are never printed |
| `--vendor-checks` | also run each binary's `--version` (cost X) |

```sh
agx doctor --agent claude --resolved-secrets
```

<!-- runnable -->
```sh
$ agx doctor --agent pi
✓ config     ~/.agx/config.jsonc is valid
✓ reference  logins.carol.token: parses
...
· shell      shell layer not loaded: new and resume leave this shell where it is
             fix: eval "$(agx shell-init --shell zsh)"
✓ binary pi  /usr/local/bin/pi (unknown)
...
```

One line per check: a mark (`✓` ok, `·` info, `!` warn, `✗` fail, `-` skip; blue, dim, amber and red on a terminal), what was checked (the area, then the agent and home), and the finding. A long finding wraps under itself to the terminal width; a check with a fix shows it on an indented `fix:` line. `--json` keeps every field (`area`, `agent`, `home`, `id`, `status`, `message`, `fix`). Exit 0 when nothing failed, 1 when a check failed (`error.code` `checks-failed`).

Try it, on a config with problems:

<!-- snippet: examples/cfg-doctor/session.txt#broken -->
```sh
$ agx doctor --config ~/broken.jsonc --agent claude
agx: 3 checks failed
✓ config                      ~/broken.jsonc is valid
✗ reference                   logins.carol.token: program op not on PATH
✓ reference                   logins.carol.token: parses
✗ reference                   profiles.work.env_refs.API_KEY: no plugin for scheme gopas (known: gopass)
✗ files                       ~/tokens.jsonc has mode 0644; it holds tokens and must be 0600
                              fix: chmod 600 ~/tokens.jsonc
✓ runs                        no run folders
· shell                       shell layer not loaded: new and resume leave this shell where it is
                              fix: eval "$(agx shell-init --shell zsh)"
✓ binary claude               /usr/local/bin/claude (unknown)
! binary claude               claude version unknown (no install metadata); tested 2.1.323..2.1.327
✓ home claude ~/.claude       ~/.claude
! home claude ~/.claude       no login found in the credentials file or keychain
· home claude ~/.claude       transcripts kept 30 days
✓ home claude ~/.claude-work  ~/.claude-work
! home claude ~/.claude-work  no login found in the credentials file or keychain
· home claude ~/.claude-work  transcripts kept 30 days
[exit 1]
```

Pitfalls: warnings never change the exit code; `--resolved-secrets` waits up to 30 seconds per reference on a locked secret store. Guide: [Check your setup](../guides/doctor.md). Example: [`examples/cfg-doctor`](../../examples/cfg-doctor/README.md).

## agx clean

Delete run folders whose agent has ended. Cost W.

A run has ended when its process is gone, or its process id was reused by another program (the start time differs, or the process lacks the run's `AGX_RUN_ID`). A run agx cannot verify is kept and listed.

| Flag | Meaning |
|---|---|
| `--yes` | delete the ended runs; without it agx only shows the plan |
| `--dry-run` | show the plan; delete nothing |

```sh
agx clean          # the plan: which runs ended, which are kept and why
agx clean --yes    # delete the ended runs
```

<!-- runnable -->
```sh
$ agx clean
no run folders
```

Pitfalls: agx execs into the agent, so it cannot remove a run folder when the agent ends; folders pile up until you run `agx clean --yes` (`agx doctor` reports them). Guide: [clean up ended runs](../guides/run-folders.md#step-5-clean-up-ended-runs). Example: [`examples/launch-run-folder/clean.session.txt`](../../examples/launch-run-folder/clean.session.txt).

## agx config init

Write a commented starter `config.jsonc`. At a terminal it asks for `default_agent` and offers the discovered homes as profiles. Cost W.

| Flag | Meaning |
|---|---|
| `--default-agent` | write `default_agent` (at a terminal without it, agx asks) |
| `--discovered` | add every discovered home as a profile (at a terminal without it, agx offers them) |
| `--dry-run` | print the file; write nothing |

```sh
agx config init --default-agent claude --discovered
```

Try it, into a fresh folder:

<!-- snippet: examples/cfg-config-set/session.txt#init -->
```sh
$ AGX_CONFIG_DIR= AGX_HOME=~/fresh agx config init --default-agent claude --discovered --dry-run
{
  "$schema": "https://raw.githubusercontent.com/ubgo/agx/main/schema/config.schema.json",
  // agx config: default_agent, agents, profiles, logins, projects, aliases.
  // Edit it by hand or with agx profile / login / project / alias / config set; comments and layout
  // are kept on every write. agx config check validates it; agx config show prints each value's source.
  "version": 1,

  // The agent used when no --profile, --agent or folder binding names one.
  "default_agent": "claude",

  // A profile is a name for one agent home plus launch defaults (agx profile add / import).
  "profiles": {
    "personal": {
      "agent": "claude",
      "home": "~/.claude",
      "default": true
    },
    "work": {
      "agent": "claude",
      "home": "~/.claude-work"
    }
  }
}
(--dry-run: ~/fresh/config.jsonc not written)
```

Pitfalls: `config init` never overwrites an existing file: it stops with `already exists` (exit 1) and names `agx config check` and `agx config set`; edit the file, or use those and the `profile`, `login`, `project` and `alias` commands. Example: [`examples/quickstart`](../../examples/quickstart/README.md).

## agx config show

The resolved config, with the source of each value (`config.jsonc:LINE:COL`, or `default`). Login tokens show only whether they are a reference or a plain value. Cost R. No flags of its own.

```sh
agx config show
agx config show --json --field data.*.key
```

Try it: the `show` region of [`examples/cfg-doctor`](../../examples/cfg-doctor/session.txt) and the `show-file` region of [`examples/cfg-config-set`](../../examples/cfg-config-set/session.txt).

<!-- snippet: examples/cfg-config-set/session.txt#show-file -->
```sh
$ agx config show --field data.*.key --field data.*.value --field data.*.source
KEY                                 VALUE                                                                      SOURCE
$schema                             https://raw.githubusercontent.com/ubgo/agx/main/schema/config.schema.json  config.jsonc:2:3
version                             1                                                                          config.jsonc:3:3
default_agent                       codex                                                                      config.jsonc:5:3
sessions.root                       ~/sessions                                                                 config.jsonc:12:5
profiles.personal.agent             claude                                                                     config.jsonc:8:19
profiles.personal.home              ~/.claude                                                                  config.jsonc:8:38
profiles.personal.default           true                                                                       config.jsonc:8:59
profiles.work.agent                 claude                                                                     config.jsonc:9:15
profiles.work.home                  ~/.claude-work                                                             config.jsonc:9:34
profiles.work.model                 opus                                                                       config.jsonc:9:60
profiles.work.live_login            true                                                                       config.jsonc:9:77
profiles.personal.billing           plan                                                                       default
profiles.personal.inherit_defaults  true                                                                       default
profiles.work.billing               plan                                                                       default
profiles.work.inherit_defaults      true                                                                       default
```

Guide: [see the config agx uses](../guides/doctor.md#step-6-see-the-config-agx-uses).

## agx config path

The config file in use. With `--json`, `data` also has `dir`, `exists` and `source` (`--config`, `AGX_CONFIG_DIR`, `AGX_HOME` or `default (~/.agx)`). It works without a valid config. Cost R. No flags of its own.

<!-- runnable -->
```sh
$ agx config path
~/.agx/config.jsonc
```

Try it, under each rule:

<!-- snippet: examples/cfg-config-set/session.txt#where -->
```sh
$ agx config path --json --field data.file --field data.source
{
  "data.file": "~/.agx/config.jsonc",
  "data.source": "AGX_CONFIG_DIR"
}

$ AGX_CONFIG_DIR= AGX_HOME=~/agx-alt agx config path --json --field data.file --field data.source
{
  "data.file": "~/agx-alt/config.jsonc",
  "data.source": "AGX_HOME"
}

$ AGX_CONFIG_DIR= agx config path --json --field data.file --field data.source
{
  "data.file": "~/.agx/config.jsonc",
  "data.source": "default (~/.agx)"
}

$ agx config path --config ~/other/agx.jsonc --json --field data.file --field data.exists
{
  "data.file": "~/other/agx.jsonc",
  "data.exists": false
}
```

Pitfalls: `--config` names a file, `AGX_CONFIG_DIR` and `AGX_HOME` name a folder; an empty variable counts as unset.

## agx config check

Load, validate and cross-check the config, listing every problem with its line and column. Cost R. No flags of its own.

<!-- runnable -->
```sh
$ agx config check
~/.agx/config.jsonc: ok (1 agent, 4 profiles, 2 logins, 1 project, 2 aliases)
```

Try it, on a file with two problems:

<!-- snippet: examples/cfg-doctor/session.txt#check-typo -->
```sh
$ agx config check --config ~/typo.jsonc
agx: typo.jsonc has 2 problems
  typo.jsonc:4:76: profiles.personal.modle: unknown key "modle"
  typo.jsonc:5:71: profiles.work.default: must be boolean, got string
[exit 1]
```

Pitfalls: a syntax error, unknown key or wrong type exits 1; a cross-check problem (a missing login, two default profiles, an alias to an internal command) exits 2. Both have `error.code` `config`, with every problem in `error.hint`. Guide: [fix what fails](../guides/doctor.md#step-4-fix-what-fails).

## agx config set

Set one scalar key, keeping comments and layout. The whole file is validated (the schema, then the cross-checks and the pool rules) before anything is written. Cost W.

| Flag | Meaning |
|---|---|
| `--key` | a dotted scalar key, such as `default_agent`, `sessions.root`, `profiles.NAME.model` or `logins.POOL.pool.accounts.MEMBER.short` |
| `--value` | the value, typed by the config schema at that key: a string, a number, an integer, `true`/`false`, or `null` where the key takes it |

The key's type comes from the config schema (the same one `agx config check` and your editor use), never from the value already in the file, so a key that is not in the file yet works as well: `--key logins.auto.pool.accounts.bob.short --value 95` writes the number 95 on a member entry that did not exist. A number key checks the schema's range (a pool cap takes a number from 0 to 100; `NaN` and `Inf` are refused), digits on a string key stay a string (`--value 4` on `profiles.NAME.model` writes `"4"`), a key with a fixed set of values takes only those, and `null` is written only where the key takes it (`logins.POOL.pool.force` does; a cap does not). On a key that takes both a string and `null`, quote the word to write it as a string: `--value '"null"'`. Objects and lists have their own commands (`profile edit`, `project use`, `alias add`, `login edit`).

```sh
agx config set --key logins.auto.pool.accounts.alice.short --value 95
agx config set --key logins.auto.pool.defaults.week --value 80
agx config set --key profiles.work.inherit_defaults --value false
```

```sh
agx config set --key sessions.root --value ~/sessions
```

<!-- runnable -->
```sh
$ agx config set --key default_agent --value codex
default_agent = codex
```

Pitfalls, each exit 2:

<!-- snippet: examples/cfg-config-set/session.txt#set-refused -->
```sh
$ agx config set --key profiles.work.live_login --value yes
agx: --key profiles.work.live_login takes true or false, not "yes"
[exit 2]

$ agx config set --key profiles --value work
agx: --key profiles is an object, not a scalar; use its own command (profile edit, project use, alias add, login edit) or edit config.jsonc
[exit 2]

$ agx config set --key profiles.work.modle --value opus
agx: --key profiles.work.modle: unknown key profiles.work.modle
  keys: agent, home, default, billing, login, home_login, args, inherit_defaults, model, autonomy, env, env_refs, env_file, env_file_commands, before_run, before_run_from, before_run_env, settings_delivery, live_login, claude, codex
[exit 2]

$ agx config set --key profiles.work.home --value ''
agx: refused: the change would make config.jsonc invalid: home is empty
  home is empty
[exit 2]
```

Typed by the schema:

<!-- snippet: examples/cfg-config-set/session.txt#set-typed -->
```sh
$ agx config set --key profiles.work.inherit_defaults --value false
profiles.work.inherit_defaults = false

$ agx config set --key profiles.work.inherit_defaults --value null
profiles.work.inherit_defaults = null

$ agx config set --key profiles.work.model --value 4
profiles.work.model = 4

$ agx config set --key profiles.work.billing --value api
profiles.work.billing = api

$ agx config set --key profiles.work.claude.effort --value high
profiles.work.claude.effort = high

$ agx config set --key profiles.work.billing --value free
agx: --key profiles.work.billing takes one of: plan, api, not "free"
[exit 2]

$ agx config set --key version --value 1.5
agx: --key version takes an integer, not "1.5"
[exit 2]
```

Guide: [change one key](../guides/doctor.md#step-5-change-one-key). Example: [`examples/cfg-config-set`](../../examples/cfg-config-set/README.md).

## agx alias list

The shell aliases from config. Cost R. No flags of its own.

<!-- runnable -->
```sh
$ agx alias list
NAME  ARGS
cl    run --profile personal
clw   run --profile work
```

## agx alias add

Add or replace an alias. Cost W.

| Flag | Meaning |
|---|---|
| `--name` | the alias name |
| `--args` | the agx arguments it runs, named flags only, such as `"run --profile work"` |

```sh
agx alias add --name clw --args "run --profile work"
```

Pitfalls: the arguments are checked against the command tree, so a positional word, shell characters, a group or internal command, or an unknown flag are refused. A name the shell layer cannot define (`agx`, `command`, `builtin`, `cd`, `set`, `function`) is refused too (exit 2). Guide: [aliases](../shell.md#aliases). Example: [`examples/shell-aliases`](../../examples/shell-aliases/README.md).

## agx alias remove

Remove an alias. Cost W.

| Flag | Meaning |
|---|---|
| `--id` | the alias name |

```sh
agx alias remove --id clw
```

## agx shell-init

Print the [shell layer](../shell.md) for your shell. Cost R.

| Flag | Meaning |
|---|---|
| `--shell` | `zsh`, `bash`, `fish` or `pwsh` |
| `--osc7` | also report the current folder to the terminal (OSC 7) after the wrapper's `cd` |

```sh
eval "$(agx shell-init --shell zsh)"
```

A layer made with the global `--config X` runs every command and alias against X (it adds `--config X` to each call, quoted for the shell); a `--config` you type before a bare `--` wins instead, and one after `--` belongs to the agent. An alias name or word the layer cannot define (`agx`, a shell builtin, a word starting with `=` or `@`) is refused when the config loads, with its line and column; shell-init then warns and prints the wrapper without aliases.

Guide: [load it](../shell.md#load-it). Example: [`examples/shell-init`](../../examples/shell-init/README.md).

## agx completion

Print a completion script. Cost R.

| Flag | Meaning |
|---|---|
| `--shell` | `zsh`, `bash`, `fish` or `pwsh` |

```sh
eval "$(agx completion --shell zsh)"
```

Guide: [completion](../shell.md#completion).

## agx help

Help for agx. For one command, use `agx <command> --help` (`agx usage --help`); `agx help usage` is refused like any bare word (exit 2). With `--json`, the whole command tree with every flag, its cost class and the `--field` paths of each command's data, generated from the parser's own declarations. Cost R. No flags of its own beyond the global ones.

<!-- runnable -->
```sh
$ agx help --json --field data.commands.*.name
["agent","alias","clean","completion","config","conversation","credential","doctor","help","launch","live","login","new","plan","pool","profile","project","resume","run","session","shell-init","skill","usage","version","whoami"]
```

Hidden commands (`credential`, `launch`, `plan`) appear with `"hidden": true`; the shell layer and live login use them, and you never type them. Guide: [know the cost before you call](../guides/scripting.md#step-6-know-the-cost-before-you-call).

## agx skill

Print a Claude Code skill (`SKILL.md`) that teaches an agent session how to drive agx: frontmatter (`name: agx` and when to use it), a short "how agx works" (profiles, logins, login pools, the resolution chain, `--dry-run`, `--json` and `--field`, exit codes), then every visible command with its short and long description, examples and every flag with its help text. The command list is generated from the command tree this binary is built from (the one `agx help --json` prints), so it never drifts from the commands you have; it names the version it came from. Cost R; W with `--install`.

| Flag | Meaning |
|---|---|
| `--install` | write it to `<dir>/agx/SKILL.md` instead of printing it: the folder 0755, the file 0644; an existing file is renamed to `SKILL.md.bak-<timestamp>` first, and the path is printed |
| `--dir` | the skills folder `--install` writes into; default `~/.claude/skills` (for example `.claude/skills` for one project) |

```sh
agx skill | less
agx skill --install                          # ~/.claude/skills/agx/SKILL.md
agx skill --install --dir .claude/skills     # this project only
```

`--json` and `--field` are refused (exit 2): the output is markdown. `--dir` without `--install` is refused too. Run `agx skill --install` again after upgrading agx, so the skill lists the new commands.

## agx version

The version and build provenance: binary, version, source, commit (with `(dirty)` for a modified tree), build time, Go version and platform. Cost R. No flags of its own.

```sh
agx version
agx version --json
```

A development build from a modified tree prints:

```text
agx v0.0.0-20261117174012-a5f98d992e41+dirty
  source:      module
  commit:      a5f98d992e41630a6ae5555a17ecee3a759bd871 (dirty)
  built:       2026-11-17T17:40:12Z
  go:          go1.27.1
  platform:    linux/amd64
```

<!-- snippet: examples/out-scripting/session.txt#version -->
```sh
$ agx version --json --field data.binary
agx
```
