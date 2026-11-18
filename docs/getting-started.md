# Getting started

This guide takes you from nothing to launching agents on the right account in about five minutes: install agx, write a config, import the agent homes you already have, check quota, preview a launch, then launch. It is the first stop of the [learning path](README.md). Every block of commands and output below comes from a tested example, so what you read is what agx prints.

## Before you start

- Go 1.26.4 or newer to install agx with `go install github.com/ubgo/agx@latest`, or a prebuilt binary from the [Releases page](https://github.com/ubgo/agx/releases).
- macOS or Linux. Windows builds work but are a preview until they are verified on a real Windows machine (see [Platforms](troubleshooting.md#windows)).
- At least one coding agent installed the way its vendor documents: Claude Code, OpenAI Codex, Gemini CLI or pi. agx does not install agents; it finds them on `PATH`. Run each agent once and log in, so its home (such as `~/.claude`) exists.

agx never changes those homes on its own: it reads them, and passes your choices to one launch at a time.

## The words agx uses

| Word | Meaning |
|---|---|
| Agent | a coding-agent CLI: `claude`, `codex`, `gemini`, `pi`, or one you describe in config |
| Home | one agent config folder, such as `~/.claude` or `~/.claude-work`; it holds that agent's login, settings and history |
| Profile | a name for one home plus launch defaults; you pick it with `--profile` (or `-p`) |
| Login | what a launch authenticates with: a named login from config, or the home's own stored login |
| Project | a folder bound to a profile or login, per agent, so a plain `agx run` there does the right thing |
| Session folder | a timestamped scratch folder made by `agx new` |
| Conversation | one chat stored in a home |
| Run | one launch, with its own run folder and `AGX_RUN_ID` |

## Step 1 — install

agx is one Go binary:

```sh
go install github.com/ubgo/agx@latest
agx version
```

## Step 2 — write a starter config

<a id="step-1-write-a-config"></a>

agx reads one file, `~/.agx/config.jsonc` (JSON with comments). Set `AGX_CONFIG_DIR` to keep it elsewhere. `config init` writes a commented starter file; `--default-agent` names the agent a bare `agx run` starts:

<!-- snippet: examples/getting-started/session.txt#init -->
```sh
$ agx config init --default-agent claude
wrote ~/.agx/config.jsonc (0 profiles)
```

## Step 3 — import the homes you have

agx never searches your disk while launching. Instead, `profile import --discovered` finds the agent homes on this machine once and writes a profile for each. Homes that look real (they hold the agent's own files) are imported; a folder that only has the right name is skipped with a hint:

<!-- snippet: examples/getting-started/session.txt#import -->
```sh
$ agx profile import --discovered --yes
NAME      AGENT   HOME            DEFAULT  FOUND BY
personal  claude  ~/.claude       yes      default home
work      claude  ~/.claude-work  no       pattern .claude-*
skipped codex ~/.codex: name only, no home markers (import it with --id)
skipped gemini ~/.gemini: name only, no home markers (import it with --id)
skipped pi ~/.pi/agent: name only, no home markers (import it with --id)
wrote 2 profiles

$ agx profile list
PROFILE    PROVIDER  HOME            ACCOUNT  STATE  LOGIN       BILLING
*personal  claude    ~/.claude                none   home login  plan
work       claude    ~/.claude-work           none   home login  plan
```

Import a skipped home by its path once you know it is the one you use:

<!-- snippet: examples/getting-started/session.txt#import-id -->
```sh
$ agx profile import --id ~/.codex --yes
NAME   AGENT  HOME      DEFAULT  FOUND BY
codex  codex  ~/.codex  yes      default home
wrote 1 profile
```

Without `--yes`, `profile import` prints the same table as a plan and writes nothing. Steps 2 and 3 in one command are `agx config init --default-agent claude --discovered` (see the [quickstart example](../examples/quickstart/README.md)).

## Step 4 — check quota

`agx usage` asks each home's vendor for its plan usage: one row per home, one request each. `--cached` reads only what the agents left on disk and never touches the network:

<!-- snippet: examples/getting-started/session.txt#usage -->
```sh
$ agx usage --cached
personal  personal · claude  ~/.claude
  no cache: no reading on disk: Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage

work  work · claude  ~/.claude-work
  no cache: no reading on disk: Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage

codex  codex · codex  ~/.codex
  no cache: no reading on disk yet; run the agent once, or drop --cached
```

Current Claude Code no longer writes that reading, so for Claude run plain `agx usage`. [Usage: live, cached or probed](choosing/usage-live-or-cached.md) explains the choice.

## Step 5 — see what a launch would do

`--dry-run` prints every decision and where it came from, and starts nothing. Nothing is resolved either: no secret is read and no script runs. When something other than a flag decided (here `default_agent`), agx also prints a note on stderr:

<!-- snippet: examples/getting-started/session.txt#dry-run -->
```sh
$ agx run --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                 from default_agent
profile  personal                               from default profile of claude
home     ~/.claude                              from profile personal
login    home login                             from default (no login named)
binary   /usr/local/bin/claude version unknown  from no install metadata
folder   ~/projects/app                         from current folder
run      ~/.agx/runs/20261114-101500-k3x9       from AGX_RUN_ID=20261114-101500-k3x9
command  claude                                 from built by agentio
env      set AGX_RUN_ID=20261114-101500-k3x9    from launch environment
         set AGX_RUN_AGENT=claude               from launch environment
         set AGX_RUN_PROFILE=personal           from launch environment
         set AGX_RUN_LOGIN=home                 from launch environment
         set AGX_RUN_HOME=~/.claude             from launch environment
note     installed Claude Code version unknown; flags not checked
```

The version is unknown on the test machine because its `claude` has no install metadata; on a real install the line shows the version and the range agx was tested with.

## Step 6 — launch

<a id="step-3-launch"></a><a id="step-4-a-scratch-session"></a><a id="step-5-come-back-to-a-conversation"></a>

Drop `--dry-run`. agx resolves everything, then replaces itself with the agent (on macOS and Linux it execs), so the agent is the process you started and nothing of agx waits in between ([what a launch leaves behind](guides/run-folders.md)). Anything after `--` goes to the agent untouched. The test machine's `claude` prints what it received:

<!-- snippet: examples/getting-started/session.txt#run -->
```sh
$ agx run --profile work -- --verbose
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv [--verbose]
pwd ~/projects/app
...
env AGX_RUN_AGENT=claude
env AGX_RUN_HOME=~/.claude-work
env AGX_RUN_ID=20261114-101500-k3x9
env AGX_RUN_LOGIN=home
env AGX_RUN_PROFILE=work
env CLAUDE_CONFIG_DIR=~/.claude-work
...
```

The `work` home reached Claude as `CLAUDE_CONFIG_DIR`. The `personal` profile runs on Claude's default home, so the variable is left unset for it: that is how Claude itself finds its default home and its login.

More ways to launch:

```sh
agx run                           # the default profile here, the home's own login
agx run --profile work            # the work account
agx run --agent codex             # Codex, on its default profile
agx new --name "pglite golang"    # a timestamped scratch folder, and the agent in it
agx resume --last                 # the newest conversation of this folder, on the home that stores it
```

## Step 7 — load the shell layer (optional)

A program cannot `cd` your shell or set a variable in it. The shell layer is one small `agx()` function that does exactly that for `agx new` and `agx resume` (your shell follows the agent into its folder), for `agx login use --terminal` (sets `AGX_LOGIN` in this terminal), and for your config `aliases`. Add one line to your shell's startup file:

```sh
eval "$(agx shell-init --shell zsh)"     # in ~/.zshrc; the bash, fish and PowerShell lines are in the shell guide
```

Everything else works without it. See [Shell layer](shell.md#load-it), and [with or without the shell layer](choosing/shell-layer.md) to decide.

## Try it: the config the rest of the docs use

You can also write the file by hand. This is the config the examples on the other guide pages run against:

<!-- fixture: demo -->
```jsonc
{
  "$schema": "https://raw.githubusercontent.com/ubgo/agx/main/schema/config.schema.json",
  "version": 1,
  "default_agent": "claude",
  "agents": {
    // raw args every Claude launch gets first
    "claude": { "defaults": { "args": ["--chrome"] } }
  },
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work", "env_refs": { "API_KEY": "${gopass:work/api-key}" } },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true },
    "gem": { "agent": "gemini", "home": "~/.gemini", "default": true, "login": "gem-work" }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude/carol}" },
    "gem-work": { "agents": ["gemini"], "kind": "api-key", "token": "${gopass:google/gemini-key}" }
  },
  "projects": {
    "~/projects/client": { "claude": { "profile": "work" } }
  },
  "aliases": { "cl": "run --profile personal", "clw": "run --profile work" }
}
```

What it says:

- Two Claude accounts live in two homes, `~/.claude` (profile `personal`, the default) and `~/.claude-work` (profile `work`).
- Codex and Gemini each have one profile.
- `carol` is a Claude plan token kept in gopass; `gem-work` is a Gemini API key, used by the `gem` profile.
- Inside `~/projects/client`, Claude runs on the `work` profile.

The `$schema` line gives your editor completion and checks. agx checks the file itself on every load and reports a typo at its line and column. See the [config reference](config.md) for every key.

Inside `~/projects/client` the folder binding chooses `work`:

<!-- runnable: dir=~/projects/client -->
```sh
$ agx run --dry-run --quiet --json --field data.profile --field data.home --field data.login
{
  "data.profile": "work",
  "data.home": "~/.claude-work",
  "data.login": "home"
}
```

Anywhere else, the default profile wins:

<!-- runnable -->
```sh
$ agx run --dry-run --quiet --json --field data.profile
personal
```

`--quiet` turns off the stderr notes; `--json --field` keeps only the values you ask for (see [Output and exit codes](output.md)).

## Pitfalls

- Two choices for the same thing in one command are refused instead of guessed (exit 125 for a launching command):

<!-- runnable: exit=125 -->
```sh
$ agx run --login carol --home-login
agx: --login and --home-login both choose the login; pick one
```

- A name agx does not know stops the launch and says where to look:

<!-- snippet: examples/getting-started/session.txt#refused -->
```sh
$ agx run --profile nosuch
agx: unknown profile nosuch
  agx profile list
[exit 125]
```

- With no `default_agent`, no `--agent`, no `--profile` and no folder binding, agx asks at a terminal (and offers to remember the answer); without a terminal it stops and prints the commands that would decide. Set `default_agent` in Step 2 to avoid both.
- `profile import --discovered` skips a home that only has the right name (`no home markers`), usually because the agent was never run there. Run the agent once, or import it with `--id`.
- `agx usage` makes one request per home and vendors rate-limit it: do not loop it. `--cached` is free but, for current Claude Code, empty.
- The version warning means agx could not read the installed version without running the binary; flags are then passed unchecked. It does not stop the launch.

## Where next

- The [learning path](README.md) continues from here: logins from gopass, folder bindings, settings and environment, the shell layer, live login, scripting.
- [How agx decides](how-agx-decides.md): the resolution chain behind every `from …`.
- [Logins](logins.md): run any home on any account.
- [Choosing](choosing/README.md): which login kind, where to set it, which way to pass an option.
- [Command reference](commands/README.md): every command and flag.
- Example: [getting-started](../examples/getting-started/README.md), [quickstart](../examples/quickstart/README.md).
