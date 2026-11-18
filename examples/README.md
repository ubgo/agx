# Examples

Each folder here is a scripted agx session that runs in the test suite with the real binary, on a fake machine in a temporary folder: fake `claude`, `codex`, `gemini` and `pi` binaries that print what they received, a fake `gopass`, the homes `~/.claude`, `~/.claude-work`, `~/.codex`, `~/.gemini` and `~/.pi/agent`, the folders `~/projects/app` and `~/projects/client`, and two fake Claude conversations in `~/projects/client`. Nothing touches a real account, binary, network or quota. The guides under [docs](../docs/README.md) show parts of these sessions, and a test keeps every shown part equal to the file, so what you read is what agx prints today.

## Index

Every example, grouped by topic. The guide column names the page that explains it.

### First steps

| Example | What it shows | Guide |
|---|---|---|
| [quickstart](quickstart/README.md) | Discover homes, write a starter config with them, preview a launch, store a token from stdin. | [getting-started.md](../docs/getting-started.md) |
| [getting-started](getting-started/README.md) | The getting-started path: `config init`, `profile import --discovered` and `--id`, `usage --cached`, `run --dry-run`, a real `run`. | [getting-started.md](../docs/getting-started.md) |
| [readme-tour](readme-tour/README.md) | The README quick start: config from discovered homes, a gopass login, a folder binding, Codex at `ask`, a resume. | [README](../README.md) |

### Accounts and logins

| Example | What it shows | Guide |
|---|---|---|
| [acct-login-kinds](acct-login-kinds/README.md) | Add one login of every kind and source, list and check them, and see how each reaches its agent. | [logins.md](../docs/logins.md#kinds) |
| [acct-login-manage](acct-login-manage/README.md) | Check, default, rotate and remove logins; `--cascade`; the refusals. | [logins.md](../docs/logins.md#rotating-and-removing) |
| [acct-use-login](acct-use-login/README.md) | Every place a login can come from and what wins: agent default, profile, flags, `--token-ref`, `AGX_LOGIN`. | [logins.md](../docs/logins.md#using-a-login) |
| [acct-second-account](acct-second-account/README.md) | A second Claude account in its own home: discover, `profile add` (with and without `--bare`), import, edit, default, remove. | [guides/second-account.md](../docs/guides/second-account.md) |
| [acct-auto-pick](acct-auto-pick/README.md) | What `--auto-profile` and `--auto-login` decide before any request, and their refusals. | [guides/auto-pick.md](../docs/guides/auto-pick.md) |
| [acct-usage-cached](acct-usage-cached/README.md) | `agx usage --cached` on Claude (stale, none), Codex and Gemini homes, `--plan`, `--table`, `--sort`. | [guides/usage.md](../docs/guides/usage.md) |
| [acct-usage-times](acct-usage-times/README.md) | How reset times read: `--time-format relative`, `both`, `day`, `absolute`; `--date-format` presets and tokens; the config keys and the refusals. | [guides/usage.md](../docs/guides/usage.md#step-4-change-how-times-read) |
| [acct-live-login](acct-live-login/README.md) | Start a Claude session switchable, list and show the live run, switch its login and go back. | [guides/live-login.md](../docs/guides/live-login.md) |

### Deciding: profiles, bindings and homes

| Example | What it shows | Guide |
|---|---|---|
| [decide-agent-profile](decide-agent-profile/README.md) | Where the agent and the profile come from: `--profile`, `--agent`, a binding, `default_agent`, nothing at all. | [how-agx-decides.md](../docs/how-agx-decides.md#step-1-the-agent) |
| [decide-login-chain](decide-login-chain/README.md) | Every source of a run's login, from the home login up to the flags. | [how-agx-decides.md](../docs/how-agx-decides.md#step-4-the-login) |
| [decide-launch-sources](decide-launch-sources/README.md) | One full dry run and the source of each line: folder, binary, args, options, settings, environment. | [how-agx-decides.md](../docs/how-agx-decides.md#see-every-decision---dry-run) |
| [decide-inspect](decide-inspect/README.md) | `agx profile show`, `agx config show` and the JSON form of a dry run's decisions. | [how-agx-decides.md](../docs/how-agx-decides.md#see-every-decision---dry-run) |
| [decide-folder-binding](decide-folder-binding/README.md) | Bind folders per agent with `agx project use`, list, show and remove bindings. | [guides/folder-bindings.md](../docs/guides/folder-bindings.md) |
| [decide-shared-home](decide-shared-home/README.md) | Two profiles on one Claude home, and how `agx resume` and `agx usage` pick between them. | [guides/shared-homes.md](../docs/guides/shared-homes.md) |

### Launching

| Example | What it shows | Guide |
|---|---|---|
| [launch-commands](launch-commands/README.md) | One dry run per launching command: `run`, `new`, `resume`, `whoami`. | [commands/launch.md](../docs/commands/launch.md) |
| [launch-run-folder](launch-run-folder/README.md) | Real launches, the run variables and home variable each agent gets, `agx clean`, `agx whoami`. | [guides/run-folders.md](../docs/guides/run-folders.md) |
| [launch-options](launch-options/README.md) | Typed Claude options: profile `model` and `claude` block, `--option`, `--model`, `--add-dir`, `--prompt`. | [launch-options.md](../docs/launch-options.md#recipe-7-typed-vendor-launch-flags) |
| [launch-settings](launch-settings/README.md) | Claude settings from a file, inline in the profile, one key with `--setting`, file or inline delivery. | [launch-options.md](../docs/launch-options.md#recipe-1-a-claude-settings-file) |
| [launch-env](launch-env/README.md) | Environment for one launch: profile `env`, `env_refs`, `env_file`, `--env`, `--env-file`, `--no-env`, strict expansion. | [launch-options.md](../docs/launch-options.md#recipe-5-any-environment-variable) |
| [launch-before-run](launch-before-run/README.md) | A script before the agent: `before_run`, `before_run_from`, `before_run_env`, `--before-run`. | [launch-options.md](../docs/launch-options.md#a-script-before-the-agent-before_run) |
| [launch-codex](launch-codex/README.md) | What passes to Codex: `-c` settings, the `codex` block, `--option`, autonomy, no token login. | [launch-options.md](../docs/launch-options.md#recipe-9-codex) |
| [launch-gemini-pi](launch-gemini-pi/README.md) | What passes to Gemini CLI and pi: model, folders, prompt, autonomy, API-key logins, refusals. | [launch-options.md](../docs/launch-options.md#recipe-10-gemini-pi-and-custom-agents) |
| [launch-autonomy](launch-autonomy/README.md) | Every `--autonomy` level per agent as the command agx would run, and the refusals. | [guides/autonomy.md](../docs/guides/autonomy.md) |
| [launch-raw-args](launch-raw-args/README.md) | Agent default args, profile `args`, `inherit_defaults`, and args after `--`. | [guides/raw-args.md](../docs/guides/raw-args.md) |
| [launch-raw-settings](launch-raw-settings/README.md) | A raw `--settings` (inline or a JSONC file) merged into agx's settings: sources, two values, live login, errors. | [launch-options.md](../docs/launch-options.md#a-raw---settings-of-your-own) |
| [launch-headless](launch-headless/README.md) | `--prompt` and `--headless` per agent, for scripts and CI. | [guides/headless.md](../docs/guides/headless.md) |

### Conversations, sessions and the shell

| Example | What it shows | Guide |
|---|---|---|
| [convo-list](convo-list/README.md) | List conversations: this folder, every folder, by folder, narrowed and capped. | [guides/conversations.md](../docs/guides/conversations.md) |
| [convo-resume](convo-resume/README.md) | Resume the newest or one conversation on the home that stores it; resume on another login; refusals. | [guides/conversations.md](../docs/guides/conversations.md#step-4-resume-a-conversation) |
| [convo-move-to-work](convo-move-to-work/README.md) | Move a folder's Claude conversations to another home, with a backup. | [guides/conversations.md](../docs/guides/conversations.md#step-6-move-a-folders-conversations-to-another-account) |
| [session-new-promote](session-new-promote/README.md) | `agx new`, `agx session list` and `agx session promote`. | [guides/session-folders.md](../docs/guides/session-folders.md) |
| [session-clean](session-clean/README.md) | `agx session clean` and `--min-age`. | [guides/session-folders.md](../docs/guides/session-folders.md#step-4-remove-the-folders-you-never-used) |
| [shell-init](shell-init/README.md) | The shell layer script for zsh, bash, fish and PowerShell, `--osc7`, and completion. | [shell.md](../docs/shell.md#load-it) |
| [shell-plan-file](shell-plan-file/README.md) | The three commands the shell layer runs for `agx new` and `agx resume`. | [shell.md](../docs/shell.md#how-the-cd-works-the-plan-file) |
| [shell-aliases](shell-aliases/README.md) | Add, list and remove aliases, and the functions the layer makes of them. | [shell.md](../docs/shell.md#aliases) |

### Setup, agents and scripting

| Example | What it shows | Guide |
|---|---|---|
| [agent-capabilities](agent-capabilities/README.md) | `agx agent list` and `agx agent show`: each agent's capabilities and its reason for each it lacks. | [guides/agents.md](../docs/guides/agents.md) |
| [cfg-custom-agent](cfg-custom-agent/README.md) | A config-defined agent with a stand-in binary: spec, profile, checks, launches. | [guides/custom-agent.md](../docs/guides/custom-agent.md) |
| [cfg-agent-defaults](cfg-agent-defaults/README.md) | Agent defaults (args, login) and how a profile inherits or replaces them. | [config.md](../docs/config.md#agents) |
| [cfg-doctor](cfg-doctor/README.md) | `agx doctor` healthy and broken, `--agent`, `--resolved-secrets`, `--vendor-checks`, `config check`. | [guides/doctor.md](../docs/guides/doctor.md) |
| [cfg-config-set](cfg-config-set/README.md) | `agx config set`, `config show`, `config init --dry-run`, and their refusals. | [guides/doctor.md](../docs/guides/doctor.md#step-5-change-one-key) |
| [out-scripting](out-scripting/README.md) | The JSON envelope, `--field`, `--quiet`, width, colour, timeout, `help --json`. | [guides/scripting.md](../docs/guides/scripting.md) |
| [out-exit-codes](out-exit-codes/README.md) | Every exit code a script can see, made to happen, with its `error.code`. | [guides/scripting.md](../docs/guides/scripting.md#step-4-branch-on-the-exit-code) |
| [exit-codes](exit-codes/README.md) | Refusals and their exit codes: 125 for a launching command, 2 for any other. | [output.md](../docs/output.md#exit-codes) |

### Choosing

| Example | What it shows | Guide |
|---|---|---|
| [choose-login-kind](choose-login-kind/README.md) | Every login kind side by side: what each agent takes, how each reaches it, live-login limits. | [choosing/login-kind.md](../docs/choosing/login-kind.md) |
| [choose-second-account](choose-second-account/README.md) | A new home, a named login on the same home, or a second profile on one home. | [choosing/second-account.md](../docs/choosing/second-account.md) |
| [choose-where-to-set](choose-where-to-set/README.md) | The same launch decided by a flag, `AGX_LOGIN`, a binding, the profile. | [choosing/where-to-set.md](../docs/choosing/where-to-set.md) |
| [choose-passing](choose-passing/README.md) | The model by every road: `--model`, `--option`, `--setting`, raw; env; conflicts; other agents. | [choosing/passing-options.md](../docs/choosing/passing-options.md) |
| [choose-settings-delivery](choose-settings-delivery/README.md) | Settings as a file or inline, Codex `-c`, a raw `--settings`. | [choosing/settings-delivery.md](../docs/choosing/settings-delivery.md) |
| [choose-usage](choose-usage/README.md) | `--cached` with Claude Code 2.1.323+ (no reading, stale), Codex, Gemini; refusals. | [choosing/usage-live-or-cached.md](../docs/choosing/usage-live-or-cached.md) |
| [choose-continue-elsewhere](choose-continue-elsewhere/README.md) | Resume on another login, `--live-login`, `conversation move`. | [choosing/continue-on-another-account.md](../docs/choosing/continue-on-another-account.md) |
| [choose-custom-agent](choose-custom-agent/README.md) | A config-defined agent next to a built-in one with raw args: what each can do. | [choosing/custom-agent.md](../docs/choosing/custom-agent.md) |
| [choose-shell-layer](choose-shell-layer/README.md) | What changes with and without the shell layer. | [choosing/shell-layer.md](../docs/choosing/shell-layer.md) |

## Run them

```sh
go test -count=1 ./internal/docscheck/ -run TestExamples                 # every example
go test -count=1 ./internal/docscheck/ -run 'TestExamples/quickstart$'   # one example
task examples                                                            # the same as the first line
```

A failure names the session file and line of the command, and prints the shown and the real output.

## Folder layout

| Entry | Required | What it is |
|---|---|---|
| `README.md` | yes | What the example shows, how to run it, its files. |
| `session.txt` | yes | The commands and their output. |
| `<x>.session.txt` | no | More sessions; each runs on its own fresh machine, so one example can show independent scenarios. |
| `config.jsonc` | no | The machine's `~/.agx/config.jsonc`. Without it the machine has no config. |
| `home/` | no | Files copied over the fake home after it is built (settings, env files, `.agx/tokens.jsonc`, conversations, a token to pipe in). git keeps only the executable bit, so the copy sets modes itself: executable files 0755, `tokens.jsonc` 0600 (the mode agx writes and requires), everything else 0644. |

Any other entry fails the test, so a misspelled session never silently stops running. Folder names are lowercase kebab-case.

## Session format

```text
#: A comment line; the runner skips it.
#options dir=~/projects/client env=NO_COLOR=1
#region first-steps
$ agx profile list
PROFILE    PROVIDER  HOME       ACCOUNT  STATE  LOGIN       BILLING
*personal  claude    ~/.claude           none   home login  plan

$ agx run --profile nosuch --dry-run
agx: unknown profile nosuch
  agx profile list
[exit 125]

#endregion
```

- `$ agx …` is a command. Leading `NAME=value` words set its environment; `'…'` and `"…"` quote; a leading `~` is the fake home. A trailing `< ~/file` (in the home) or `< file` (relative to the folder the commands run in) is its stdin. Pipes and substitutions are not supported.
- The lines under a command, up to the next command, are its output as a terminal shows it: stderr notes first, then stdout. Every run of spaces counts as one space, so columns may be aligned freely; run ids, times, session stamps and backup stamps (`20261118T061007Z`) match any value; a line `...` matches any number of lines.
- `[exit N]` alone on a line after the output is the exit code the command must return. Without it the command must exit 0.
- `#options` (only before the first command): `dir=` the folder the commands run in (default `~/projects/app`), `env=NAME=value` an environment variable for every command (repeatable), `mode=~/path=octal` the mode of one `home/` file (repeatable; `examples/cfg-doctor` uses `mode=~/tokens.jsonc=644` to show doctor failing a readable tokens file).
- `#: text` is a comment. `#region <name>` and `#endregion` mark a part a doc can show; the runner skips them. agx prints `# ` lines (the shell-init scripts), never `#:`, `#options`, `#region` or `[exit N]`.

To write the real output into a session, run its test with `-args -update`. A command whose shown output and exit code still match is left exactly as written, including any `#:` comment or region marker inside its output. For any other command the output and `[exit N]` lines are replaced by the real ones: a `#:` comment or region marker inside the old output stays after the same number of output lines, leading indentation is kept (a shell script stays readable), and table columns are realigned from the columns agx itself printed. Its `...` lines are lost; add them back by hand. Comments, options and regions outside command output stay:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/quickstart$' -args -update
```

## Regions in other files

Any file of an example can hold regions behind its own comment token, so it stays valid: `// #region profiles` and `// #endregion` in JSONC, `# #region tokens` in an env or shell file, `<!-- #region intro -->` in markdown. `#endregion <name>` may name the region it closes. Regions may nest; marker lines belong to no region. The marker lines stay in the files copied to the machine.

## Showing an example in a doc

Put a snippet marker directly above a fenced block, of any language:

~~~md
<!-- snippet: examples/quickstart/session.txt#init -->
```sh
$ agx config init --default-agent claude --discovered
wrote ~/.agx/config.jsonc (2 profiles)
...
```
~~~

The block must equal that region of the file, marker lines excluded and trailing whitespace ignored (the block above is cut short, so it would fail). Without `#region` the block must equal the whole file. A missing file or region fails. After an example changes, `go test -count=1 ./internal/docscheck/ -run TestSnippets -args -update` rewrites every differing block from its region (fences and marker kept). A difference reads `docs/x.md:12: snippet differs from examples/quickstart/session.txt#init (first difference at line 3: doc "..." vs file "")`.
