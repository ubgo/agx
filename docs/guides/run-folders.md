# Run folders, whoami and clean

Every launch (`agx run`, `agx new`, `agx resume`) gets its own run folder under `~/.agx/runs/` and a set of `AGX_RUN_*` variables in the agent's environment, and then agx replaces itself with the agent. This page shows what lands in the folder, which variables the agent sees (including the home variable that points it at its account), how `agx whoami` reads them back from inside a session, and how `agx clean` deletes the folders of runs that ended.

You want this when you script around agx (a status line or a hook that shows the profile), when you wonder what `~/.agx/runs/` is and whether you may delete it, or when you need to know exactly which environment the agent got.

## Before you start

- A config with at least one profile (`agx config init --discovered` writes one; see [Getting started](../getting-started.md)).
- `agx run --dry-run` shows every decision of a launch, including its run folder and environment, without starting anything or writing a folder. Use it to follow along on your own machine.

The examples on this page run against this config (from [examples/launch-run-folder](../../examples/launch-run-folder/README.md)): `personal` is on Claude Code's default home, `work` on a second home with one settings key.

<!-- snippet: examples/launch-run-folder/config.jsonc#profiles -->
```jsonc
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work", "claude": { "settings": { "cleanupPeriodDays": 20 } } },
```

## Step 1: launch, and see what the agent receives

Why: a launch is not just a command line. agx creates the run folder, writes what it started into it, sets the run variables and the home variable, and only then becomes the agent.

```sh
agx run --profile work
```

In the example machine the `claude` binary is a stand-in that prints the arguments, folder and environment it received, so the launch shows exactly what the agent gets:

<!-- snippet: examples/launch-run-folder/session.txt#launch -->
```sh
$ agx run --profile work
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv [--settings] [~/.agx/runs/20261118-061107-6gmx/settings-20261118-061107-6gmx.json]
pwd ~/projects/app
env AGX_CONFIG_DIR=~/.agx
env AGX_RUN_AGENT=claude
env AGX_RUN_HOME=~/.claude-work
env AGX_RUN_ID=20261118-061107-6gmx
env AGX_RUN_LOGIN=home
env AGX_RUN_PROFILE=work
env CLAUDE_CONFIG_DIR=~/.claude-work
env COLUMNS=400
env HOME=~
env LANG=C
env PATH=/usr/local/bin:/usr/bin:/bin
env PWD=~/projects/app
env TMPDIR=/tmp/agx-demo/tmp
env TZ=UTC
```

What happened, in order:

1. agx picked a run id `<YYYYMMDD-HHMMSS>-<4 random letters or digits>` and created `~/.agx/runs/<run id>/` (mode 0700).
2. The profile has settings, so agx wrote them to `settings-<run id>.json` (mode 0600) in that folder and passed its path (see [how settings reach the agent](../launch-options.md#how-settings-reach-the-agent)).
3. It wrote `run.jsonc` (mode 0600): the run id, its own pid and start time, the agent, binary, profile, home, login and folder.
4. It set the `AGX_RUN_*` variables and the home variable (`CLAUDE_CONFIG_DIR`, because `~/.claude-work` is not Claude Code's default home).
5. It replaced itself with the agent (exec). agx is gone; the agent keeps agx's pid, which is the pid `run.jsonc` recorded.

### What a run folder holds

| File | Written when | What it is |
|---|---|---|
| `run.jsonc` | every launch, just before the agent starts | who was started, where, with which pid and start time; `agx clean` reads it |
| `settings-<run id>.json` | the launch has settings and `settings_delivery` is `file` (the default) | the merged settings the agent reads via `--settings` |
| `live.jsonc` | the launch used `--live-login` | the live login state that `agx live` and `agx login use --live` change (see [Logins](../logins.md)) |
| `plan.jsonc` | the [shell layer](../shell.md) launched the run | what the launch resolved (agent, profile, home, login, folder), never a secret |

`--dry-run` builds in a temporary folder and deletes it again, so it never leaves a run folder behind. It still shows the run id and path the launch would use (the `run` line).

## Step 2: read the run variables

Why: the agent, and anything it runs (hooks, a status line, a shell inside the agent), can see which profile, login and home it runs on without parsing agx output.

| Variable | Value |
|---|---|
| `AGX_RUN_ID` | the run id, also the run folder's name |
| `AGX_RUN_AGENT` | the agent id: `claude`, `codex`, `gemini`, `pi` or a custom id |
| `AGX_RUN_PROFILE` | the profile name |
| `AGX_RUN_LOGIN` | the login name, `home` (the home's own login) or `token-ref` (a one-off `--token-ref`) |
| `AGX_RUN_HOME` | the home folder |

They are outputs only: agx never reads them to choose anything. An `agx run` started inside an agx session makes its own decisions and its own run:

<!-- snippet: examples/launch-run-folder/whoami.session.txt#nested -->
```sh
$ AGX_RUN_ID=20261118-101500-k3x9 AGX_RUN_PROFILE=work agx run --profile personal --dry-run --quiet --json --field data.profile
personal
```

`agx whoami` reads them back. Outside a session it says so, and lists any `AGX_LOGIN` variables of the shell (the per-terminal login, see [Logins](../logins.md)):

<!-- snippet: examples/launch-run-folder/whoami.session.txt#outside -->
```sh
$ agx whoami
session:  not inside a session started by agx (AGX_RUN_ID is not set)
$ AGX_LOGIN=carol agx whoami
session:   not inside a session started by agx (AGX_RUN_ID is not set)
AGX_LOGIN: carol
```

Inside a session (here the variables are set by hand, as a launch would set them), it prints the run, and `--json --field` gives one value for a prompt or status line:

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

`data.run_dir` is set only when the run folder exists in this config dir; for a run with `--live-login`, `data.live` holds its live state.

## Step 3: know which home variable the agent gets

Why: the home variable is how one agent binary serves several accounts. agx sets it for a profile on a non-default home, and leaves it absent for the agent's default home.

| Agent | Variable | Value for a non-default home |
|---|---|---|
| Claude Code | `CLAUDE_CONFIG_DIR` | the home |
| Codex | `CODEX_HOME` | the home |
| Gemini CLI | `GEMINI_CLI_HOME` | the parent of the `.gemini` home folder |
| pi | `PI_CODING_AGENT_DIR` | the home |
| opencode | `XDG_DATA_HOME` | the parent of the `opencode` data dir (this base dir is shared by every XDG tool the agent starts) |
| a custom agent | its `home_env` | the home, or its parent with `home_env_names_parent` |

<!-- snippet: examples/launch-run-folder/session.txt#home-vars -->
```sh
$ agx run --profile codex --dry-run --quiet --json --field data.command.env
["set AGX_RUN_ID=20261118-061107-xw4d","set AGX_RUN_AGENT=codex","set AGX_RUN_PROFILE=codex","set AGX_RUN_LOGIN=home","set AGX_RUN_HOME=~/.codex"]
$ agx run --profile codex-work --dry-run --quiet --json --field data.command.env
["set AGX_RUN_ID=20261118-061107-sts5","set AGX_RUN_AGENT=codex","set AGX_RUN_PROFILE=codex-work","set AGX_RUN_LOGIN=home","set AGX_RUN_HOME=~/.codex-work","set CODEX_HOME=~/.codex-work"]
$ agx run --profile gemini-work --dry-run --quiet --json --field data.command.env
["set AGX_RUN_ID=20261118-061107-by9p","set AGX_RUN_AGENT=gemini","set AGX_RUN_PROFILE=gemini-work","set AGX_RUN_LOGIN=home","set AGX_RUN_HOME=~/.gemini-work/.gemini","set GEMINI_CLI_HOME=~/.gemini-work"]
$ agx run --profile pi-work --dry-run --quiet --json --field data.command.env
["set AGX_RUN_ID=20261118-061107-dr1b","set AGX_RUN_AGENT=pi","set AGX_RUN_PROFILE=pi-work","set AGX_RUN_LOGIN=home","set AGX_RUN_HOME=~/.pi-work","set PI_CODING_AGENT_DIR=~/.pi-work"]
$ CLAUDE_CONFIG_DIR=~/.claude-work agx run --profile personal --dry-run --quiet --json --field data.command.env
["set AGX_RUN_ID=20261118-061328-igl8","set AGX_RUN_AGENT=claude","set AGX_RUN_PROFILE=personal","set AGX_RUN_LOGIN=home","set AGX_RUN_HOME=~/.claude","removed CLAUDE_CONFIG_DIR"]
$ agx run --profile work --env CLAUDE_CONFIG_DIR=~/.claude --dry-run --quiet
agx: CLAUDE_CONFIG_DIR is set by the profile's home; use --profile
[exit 125]
```

Why absent for the default home: the agent then behaves exactly as when you start it by hand. Claude Code in particular stores its login under a different keychain name when `CLAUDE_CONFIG_DIR` is set, even to `~/.claude`, so setting it would make the default home look logged out. A home variable inherited from your shell is removed for a default-home launch (the `removed CLAUDE_CONFIG_DIR` above), so the profile you chose is the home you get.

## Step 4: start in another folder with `--dir`

Why: run an agent for a project without `cd`-ing into it first. The folder decides [folder bindings](../how-agx-decides.md) and where the agent works.

<!-- snippet: examples/launch-run-folder/session.txt#dir -->
```sh
$ agx run --profile personal --dir ~/projects/client
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv
pwd ~/projects/client
env AGX_CONFIG_DIR=~/.agx
env AGX_RUN_AGENT=claude
env AGX_RUN_HOME=~/.claude
env AGX_RUN_ID=20261118-061107-qz8a
env AGX_RUN_LOGIN=home
env AGX_RUN_PROFILE=personal
env COLUMNS=400
env HOME=~
env LANG=C
env PATH=/usr/local/bin:/usr/bin:/bin
env PWD=~/projects/client
env TMPDIR=/tmp/agx-demo/tmp
env TZ=UTC
```

The agent runs in `~/projects/client` and, on the default home, gets no `CLAUDE_CONFIG_DIR`. `agx new` and `agx resume` choose their own folder (a new session folder, the conversation's folder), so they have no `--dir`.

## Step 5: clean up ended runs

Why: agx exec's into the agent, so nothing is left running to delete the folder when the agent exits. Run folders stay until you run `agx clean`.

```sh
agx clean          # show the plan: which folders would go, which stay, and why
agx clean --yes    # delete the folders of runs that ended
```

`agx clean` checks each folder's liveness from its `run.jsonc`: the pid, the recorded start time and the `AGX_RUN_ID` in that process's start environment, so a pid reused by another process never keeps or deletes the wrong folder. A folder whose agent still runs is kept; a folder agx cannot judge is kept and listed with the reason, never guessed.

<!-- snippet: examples/launch-run-folder/clean.session.txt#clean -->
```sh
$ agx clean
agx: warning: kept 20261118-101500-k3x9: process info unreadable: no run.jsonc yet
RUN                   AGENT   PID       LIVENESS  ACTION  REASON
20261116-090000-ab12  claude  99999999  ended     delete  process ended
20261118-101500-k3x9  —       0         unknown   keep    process info unreadable: no run.jsonc yet
would delete 1, keep 1; run agx clean --yes to delete
$ agx clean --dry-run
agx: warning: kept 20261118-101500-k3x9: process info unreadable: no run.jsonc yet
RUN                   AGENT   PID       LIVENESS  ACTION  REASON
20261116-090000-ab12  claude  99999999  ended     delete  process ended
20261118-101500-k3x9  —       0         unknown   keep    process info unreadable: no run.jsonc yet
would delete 1, keep 1; run agx clean --yes to delete
$ agx clean --yes --dry-run
agx: --yes and --dry-run: one deletes, one only shows; pick one
[exit 2]
$ agx clean --yes
agx: warning: kept 20261118-101500-k3x9: process info unreadable: no run.jsonc yet
RUN                   AGENT   PID       LIVENESS  ACTION   REASON
20261116-090000-ab12  claude  99999999  ended     deleted  process ended
20261118-101500-k3x9  —       0         unknown   keep     process info unreadable: no run.jsonc yet
deleted 1, kept 1
$ agx clean --json --field data.runs.*.id
["20261118-101500-k3x9"]
```

The second folder has no `run.jsonc`: agx creates the folder a moment before it writes that file, so a young folder without it may be a launch in progress and is kept. Once such a folder is older than a minute, it counts as a launch that never started and `--yes` deletes it.

| Liveness | Meaning | `--yes` |
|---|---|---|
| `ended` | the recorded process no longer runs (or it is another process with the same pid) | deletes |
| `running` | the agent still runs | keeps |
| `unknown` | agx cannot read the process information or `run.jsonc` | keeps, with a warning naming the reason |

## Step 6: exit codes, exec and Windows

On macOS and Linux agx exec's into the agent, so the exit code you see is the agent's own:

<!-- snippet: examples/launch-run-folder/session.txt#exit-code -->
```sh
$ FAKE_AGENT_EXIT=3 agx run --profile personal
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
...
env FAKE_AGENT_EXIT=3
...
[exit 3]
```

125 to 127 mean agx refused or failed before the agent started (see [exit codes](../output.md#exit-codes)). Because agx is gone after the exec, `--json` and `--field` have nothing to print on a real launch: they apply to `--dry-run` and to refusals.

Windows has no exec. There agx stays as the parent process: it starts the agent as a child with the same terminal, ignores Ctrl-C itself (the console delivers it to the agent too), waits, and exits with the agent's exit code. `--dry-run` on Windows adds the note `Windows has no exec: agx stays as the parent and returns the agent's exit code`. See [Windows](../troubleshooting.md#windows) for the preview status.

## Try it

From the repository:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-run-folder$'
```

On your own machine, without starting anything:

```sh
agx run --profile work --dry-run       # the run line, the AGX_RUN_* and home variables
agx clean                              # what clean would delete
```

## Pitfalls

- `--env CLAUDE_CONFIG_DIR=…` (or any agent's home variable) is refused with `agx: CLAUDE_CONFIG_DIR is set by the profile's home; use --profile` (exit 125). The home is the profile's job; pick another profile or add one with `agx profile add`.
- `--dir` must exist: `agx: folder ~/projects/nope does not exist` (exit 125).

<!-- snippet: examples/launch-run-folder/session.txt#dir-missing -->
```sh
$ agx run --profile personal --dir ~/projects/nope --dry-run --quiet
agx: folder ~/projects/nope does not exist
[exit 125]
```

- `agx clean --yes --dry-run` is refused: `agx: --yes and --dry-run: one deletes, one only shows; pick one` (exit 2). Plain `agx clean` already only shows.
- Run folders pile up: nothing deletes them automatically, by design (agx is gone once the agent runs). Run `agx clean --yes` now and then; it never touches a running agent's folder.
- Setting `AGX_RUN_*` yourself does not change what agx launches; they only change what `agx whoami` reports.
- A settings file in a run folder holds whatever settings the profile passes (for example an `apiKeyHelper` command). The folder is 0700 and the file 0600; `agx clean --yes` removes them once the run ended.

## Related

- Reference: [`agx run`](../commands/launch.md#agx-run), [`agx whoami`](../commands/launch.md#agx-whoami), [`agx clean`](../commands/setup.md#agx-clean)
- Example: [examples/launch-run-folder](../../examples/launch-run-folder/README.md)
- [How agx decides](../how-agx-decides.md): where the profile, home and folder come from
- [Passing anything to an agent](../launch-options.md): settings files, environment variables
- [Logins](../logins.md): `AGX_LOGIN`, `--live-login` and `live.jsonc`
