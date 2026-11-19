# Launching: run, new, resume, whoami

The three launching commands resolve an agent, profile, home and login (see [How agx decides](../how-agx-decides.md)), create a [run folder](../guides/run-folders.md), then replace agx with the agent (on Windows agx stays as the parent). Their exit code is the agent's own; 125 to 127 mean agx failed before the agent started (see [exit codes](../output.md#exit-codes)).

Guides: [Passing anything to an agent](../launch-options.md), [Autonomy](../guides/autonomy.md), [Headless runs](../guides/headless.md), [Raw args](../guides/raw-args.md), [Run folders, whoami and clean](../guides/run-folders.md). Example: [examples/launch-commands](../../examples/launch-commands/README.md).

## agx run

Start an agent in the current folder (or `--dir`). Cost R, plus N for `--auto-profile`, Q for `--auto-login`, X for secret commands, then X.

| Flag | Meaning |
|---|---|
| `--profile`, `-p` | the profile |
| `--auto-profile` | the profile with the most plan headroom (reads usage; refused for agents without usage) |
| `--agent` | the agent, for its default profile or for `--auto-profile` |
| `--login` | a login name |
| `--home-login` | the home's own login |
| `--auto-login` | the stored login with the most headroom (probes each: spends a few tokens) |
| `--token-ref` | a one-off token: a reference (`${gopass:…}`, `$(…)`, `${VAR}`) or a plain value |
| `--token-kind` | the kind of `--token-ref`: `oauth-token` (default), `api-key` or `api-key-helper` |
| `--dir` | the start folder (must exist); default the current folder |
| `--model` | the model |
| `--prompt` | the first prompt |
| `--add-dir` | an extra folder the agent may access (repeat) |
| `--autonomy` | `default`, `read-only`, `ask`, `accept-edits` or `full`; see [Autonomy](../guides/autonomy.md) |
| `--consent` | `full-autonomy`: required with `--autonomy full` |
| `--headless` | a machine-readable event stream instead of the interactive UI; see [Headless runs](../guides/headless.md) |
| `--option` | a typed agent launch option `key=value` (repeat) |
| `--setting` | an agent settings key `key=value`; dotted keys nest (repeat) |
| `--settings-delivery` | `file` (default) or `inline` |
| `--env` | an environment variable `NAME=value`; references expand strictly (repeat) |
| `--no-env` | remove this variable (repeat) |
| `--env-file` | a dotenv file, relative to the current folder (repeat) |
| `--before-run` | a script to run first, for this run |
| `--live-login` | make the session's login switchable while it runs |
| `--pool-force` | on a [pool login](../guides/login-pool.md): every key of this run comes from this member, over the pool's rules, force and picker (`agx live use` ends it) |
| `--dry-run` | print every decision with its source; start nothing |
| `--resolved-secrets` | with `--dry-run` (refused without it, exit 125): resolve each reference and say "resolved" or why not, never the value (cost X) |
| `--` | everything after it goes to the agent untouched; see [Raw args](../guides/raw-args.md) |

Each option is explained with examples in [Passing anything to an agent](../launch-options.md).

```sh
agx run --profile work
agx run -p work --login carol --model opus
agx run --agent codex --autonomy ask -- --search
agx run --profile work --setting cleanupPeriodDays=30 --env DEBUG=1
```

<!-- runnable -->
```sh
$ agx run --profile work --model opus --dry-run --quiet --json --field data.command.line
claude --model opus --chrome
```

### Try it: agx run

A profile, a stored login, a model and another folder, as a dry run:

<!-- snippet: examples/launch-commands/session.txt#run -->
```sh
$ agx run --profile work --login carol --model opus --dir ~/projects/client --dry-run --quiet
agent    claude                                  from profile work
profile  work                                    from --profile
home     ~/.claude-work                          from profile work
login    carol (not resolved: --dry-run)         from --login
binary   /usr/local/bin/claude version unknown   from no install metadata
folder   ~/projects/client                       from --dir
options  model=opus                              from --model
args     --chrome                                from agents.claude.defaults.args
run      ~/.agx/runs/20261118-061706-k4vq        from AGX_RUN_ID=20261118-061706-k4vq
command  claude --model opus --chrome            from built by agentio
env      set AGX_RUN_ID=20261118-061706-k4vq     from launch environment
         set AGX_RUN_AGENT=claude                from launch environment
         set AGX_RUN_PROFILE=work                from launch environment
         set AGX_RUN_LOGIN=carol                 from launch environment
         set AGX_RUN_HOME=~/.claude-work         from launch environment
         set CLAUDE_CODE_OAUTH_TOKEN=[redacted]  from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work    from launch environment
note     installed Claude Code version unknown; flags not checked
```

### Pitfalls: agx run

- Two flags that choose the same thing in one command are refused (exit 125); the message names both:

<!-- snippet: examples/launch-commands/session.txt#run-conflicts -->
```sh
$ agx run --profile work --agent codex --dry-run --quiet
agx: --agent codex and --profile work (a claude profile) disagree; pick one
[exit 125]
$ agx run --login carol --home-login --dry-run --quiet
agx: --login and --home-login both choose the login; pick one
[exit 125]
```

- `--dir` must name an existing folder: `agx: folder ~/projects/nope does not exist` (exit 125).
- `--autonomy full` needs `--consent full-autonomy`, and a level the agent cannot honour exactly is refused; see [Autonomy](../guides/autonomy.md#pitfalls).
- `--resolved-secrets` only works with `--dry-run`: alone it is refused (`agx: --resolved-secrets only works with --dry-run; add --dry-run`, exit 125) and nothing starts.
- agx flags go before `--`; everything after it reaches the agent, including `--dry-run`.
- `--json` and `--field` print nothing on a real launch (agx has become the agent); they apply to `--dry-run` and to refusals.

## agx new

Create a timestamped session folder under `sessions.root` and start an agent in it. Cost W then X. Guide: [Session folders](../guides/session-folders.md). It takes every `run` flag except `--dir`, plus `--name`.

| Flag | Meaning |
|---|---|
| `--name` | slug words for the folder: `--name "pglite golang"` makes `pglite_golang_20261114_101500` |
| `--profile`, `-p` | as `run` |
| `--auto-profile` | as `run` |
| `--agent`, `--login`, `--home-login`, `--auto-login`, `--token-ref`, `--token-kind` | as `run` |
| `--model`, `--prompt`, `--add-dir`, `--autonomy`, `--consent`, `--headless` | as `run` |
| `--option`, `--setting`, `--settings-delivery` | as `run` |
| `--env`, `--no-env`, `--env-file`, `--before-run`, `--live-login`, `--pool-force` | as `run` |
| `--dry-run`, `--resolved-secrets` | as `run`; with `--dry-run` no folder is created |

```sh
agx new --name "pglite golang" --profile work
agx new --agent codex
```

With the [shell layer](../shell.md) your shell also moves into the new folder. Session folders are listed, cleaned and promoted with the [`session` commands](conversation.md#agx-session-list).

### Try it: agx new

<!-- snippet: examples/launch-commands/session.txt#new -->
```sh
$ agx new --name 'pglite golang' --agent codex --dry-run --quiet
agent    codex                                          from --agent
profile  codex                                          from default profile of codex
home     ~/.codex                                       from profile codex
login    home login                                     from default (no login named)
binary   /usr/local/bin/codex version unknown           from no install metadata
folder   ~/.agx/sessions/pglite_golang_20261118_061706  from new session folder
run      ~/.agx/runs/20261118-061706-j5zw               from AGX_RUN_ID=20261118-061706-j5zw
command  codex                                          from built by agentio
env      set AGX_RUN_ID=20261118-061706-j5zw            from launch environment
         set AGX_RUN_AGENT=codex                        from launch environment
         set AGX_RUN_PROFILE=codex                      from launch environment
         set AGX_RUN_LOGIN=home                         from launch environment
         set AGX_RUN_HOME=~/.codex                      from launch environment
note     installed codex version unknown; flags not checked against it
```

### Pitfalls: agx new

- `agx new` has no `--dir`: the folder is always a new one under `sessions.root`.

<!-- snippet: examples/launch-commands/session.txt#new-dir -->
```sh
$ agx new --dir ~/projects/client --dry-run --quiet
agx: unknown flag: --dir
[exit 125]
```

- With `--dry-run` the folder is shown but not created, so its timestamp may differ from the real launch a moment later.

## agx resume

Continue a conversation on the home that stores it, in its own folder. Cost R then X. Guide: [Conversations: resume](../guides/conversations.md#step-4-resume-a-conversation).

agx asks every configured home whose agent lists conversations whether it stores the id. One match decides the agent and home; the profile comes from the [shared-home rule](../guides/shared-homes.md#step-3-let-a-resume-find-its-profile) unless `--profile` limited the search to that profile's home. History comes from the conversation's home; the login (quota) is resolved as usual, so `--login carol` resumes a work chat on carol's plan.

| Flag | Meaning |
|---|---|
| (none) | a picker of this folder's conversations, newest first; without a terminal, exit 125 with the named form |
| `--id` | the conversation: its id, or a unique prefix |
| `--last` | the newest conversation, no picker |
| `--query` | narrow the picker by title, folder or id |
| `--picker` | `chat` (default) or `folder`: pick a folder first, then a chat |
| `--scope` | `folder` (default) or `all` folders |
| `--limit` | how many conversations the picker considers, newest first; default 50, `0` for all |
| `--profile`, `-p` | search only this profile's home, and run with this profile; a conversation stored in another home is `not found` (to continue it on other quota, use `--login`) |
| `--agent` | only search this agent's homes |
| `--login`, `--home-login`, `--auto-login`, `--token-ref`, `--token-kind` | as `run`: the quota comes from this login, the history from the conversation's home |
| `--model`, `--prompt`, `--add-dir`, `--autonomy`, `--consent`, `--headless` | as `run` |
| `--option`, `--setting`, `--settings-delivery` | as `run` |
| `--env`, `--no-env`, `--env-file`, `--before-run`, `--live-login`, `--pool-force` | as `run` |
| `--dry-run`, `--resolved-secrets` | as `run` |
| `--` | raw args to the agent |

```sh
agx resume                          # pick from this folder's conversations
agx resume --last
agx resume --id 3f2a9c1e --login carol
agx resume --picker folder --scope all --query parser
```

From any folder, `--id` finds the home that stores the conversation and resumes in the conversation's own folder:

<!-- runnable -->
```sh
$ agx resume --id 3f2a --dry-run --quiet
agent    claude              from conversation 3f2a9c1e-0000-4000-8000-000000000001
profile  personal            from home ~/.claude (default)
home     ~/.claude           from conversation 3f2a9c1e-0000-4000-8000-000000000001 (stored there)
login    home login          from default (no login named)
binary   /usr/local/bin/claude version unknown   from no install metadata
folder   ~/projects/client   from conversation 3f2a9c1e-0000-4000-8000-000000000001
args     --chrome            from agents.claude.defaults.args
run      ~/.agx/runs/20261114-101500-k3x9   from AGX_RUN_ID=20261114-101500-k3x9
command  claude --resume 3f2a9c1e-0000-4000-8000-000000000001 --chrome   from built by agentio
...
```

<!-- runnable: exit=125 -->
```sh
$ agx resume --last
agx: no conversations in ~/projects/app
  agx resume --scope all
```

### Try it: agx resume

Resume the work conversation `7b1d…` (stored in `~/.claude-work`) on carol's quota: the history and folder come from the conversation, the login from `--login`:

<!-- snippet: examples/launch-commands/session.txt#resume -->
```sh
$ agx resume --id 7b1d --login carol --dry-run --quiet
agent    claude                                                         from conversation 7b1d4e20-0000-4000-8000-000000000002
profile  work                                                           from home ~/.claude-work
home     ~/.claude-work                                                 from conversation 7b1d4e20-0000-4000-8000-000000000002 (stored there)
login    carol (not resolved: --dry-run)                                from --login
binary   /usr/local/bin/claude version unknown                          from no install metadata
folder   ~/projects/client                                              from conversation 7b1d4e20-0000-4000-8000-000000000002
args     --chrome                                                       from agents.claude.defaults.args
run      ~/.agx/runs/20261118-061706-s5kf                               from AGX_RUN_ID=20261118-061706-s5kf
command  claude --resume 7b1d4e20-0000-4000-8000-000000000002 --chrome  from built by agentio
env      set AGX_RUN_ID=20261118-061706-s5kf                            from launch environment
         set AGX_RUN_AGENT=claude                                       from launch environment
         set AGX_RUN_PROFILE=work                                       from launch environment
         set AGX_RUN_LOGIN=carol                                        from launch environment
         set AGX_RUN_HOME=~/.claude-work                                from launch environment
         set CLAUDE_CODE_OAUTH_TOKEN=[redacted]                         from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work                           from launch environment
note     installed Claude Code version unknown; flags not checked
```

### Pitfalls: agx resume

- Refused (exit 125): an id that matches conversations in two homes (add `--profile`), a conversation whose folder no longer exists, and a login of another agent.
- Codex conversations resume on Codex's home login (Codex has no token login).
- `agx resume` has no `--dir`: the conversation's own folder is where it continues.
- `--headless` and `--prompt` work on a resume too; see [Headless runs](../guides/headless.md#step-4-continue-a-conversation-headless).

## agx whoami

What the current session runs on: profile, login, home and run id, read from the `AGX_RUN_*` variables agx sets for every launch. Outside an agx session it lists any `AGX_LOGIN` variables set in this shell. Cost R. No flags of its own.

<!-- runnable -->
```sh
$ agx whoami
session:  not inside a session started by agx (AGX_RUN_ID is not set)
```

Inside a session started by agx, it prints the run's id, agent, profile, login and home as `label:  value` lines; `agx whoami --json --field data.login` gives one value for a prompt or status line. The `--json` data also holds `run_dir` (when the run folder exists in this config dir), `live` (the live login state of a `--live-login` run) and `terminal_logins` (the `AGX_LOGIN` variables of this shell).

### Try it: agx whoami

Here the `AGX_RUN_*` variables are set by hand, as a launch sets them for the agent:

<!-- snippet: examples/launch-run-folder/whoami.session.txt#inside -->
```sh
$ AGX_RUN_ID=20261118-101500-k3x9 AGX_RUN_AGENT=claude AGX_RUN_PROFILE=work AGX_RUN_LOGIN=home AGX_RUN_HOME=~/.claude-work agx whoami
run:      20261118-101500-k3x9
agent:    claude
profile:  work
login:    home
home:     ~/.claude-work
$ AGX_RUN_ID=20261118-101500-k3x9 AGX_RUN_AGENT=claude AGX_RUN_PROFILE=work AGX_RUN_LOGIN=home AGX_RUN_HOME=~/.claude-work agx whoami --json --field data.profile
work
$ AGX_RUN_ID=20261118-101500-k3x9 agx whoami --json --field data.run_dir
~/.agx/runs/20261118-101500-k3x9
```

### Pitfalls: agx whoami

- The variables are outputs only. Setting them yourself changes what `whoami` prints, never what agx launches; a nested `agx run` makes its own decisions and its own run.
- A shell outside the agent (another terminal tab) has no `AGX_RUN_ID`, so `whoami` there says `not inside an agx session`.

More in [Run folders, whoami and clean](../guides/run-folders.md#step-2-read-the-run-variables); example: [examples/launch-run-folder](../../examples/launch-run-folder/README.md).
