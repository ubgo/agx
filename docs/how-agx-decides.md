# How agx decides

agx never guesses. Every value it uses (agent, profile, home, login, binary, folder, args, each option, each environment variable) comes from one documented source, chosen by a fixed order. This page is that order, every source with the exact `from …` text agx prints for it, and how to see it before anything starts.

Read it when a launch did something you did not expect, when you want to know which of two settings wins, or before you add a folder binding, a per-terminal login or a second profile on a home.

## Before you start

- agx installed and a config with at least one profile (`agx config init --discovered`, see [getting started](getting-started.md)).
- Nothing else: every command on this page is a dry run or a read, so it starts no agent, reads no secret and uses no quota.

The examples below come from the folders [`decide-launch-sources`](../examples/decide-launch-sources/README.md), [`decide-agent-profile`](../examples/decide-agent-profile/README.md), [`decide-login-chain`](../examples/decide-login-chain/README.md) and [`decide-inspect`](../examples/decide-inspect/README.md), which run in the test suite on a fake machine, so the output is what agx prints today.

## See every decision: `--dry-run`

Add `--dry-run` to any launching command (`run`, `new`, `resume`). agx prints each answer with its source and starts nothing. It never runs gopass, a `$(…)` command, a `before_run` script or a usage probe during a dry run.

With this profile (from [`decide-launch-sources/config.jsonc`](../examples/decide-launch-sources/config.jsonc)):

<!-- snippet: examples/decide-launch-sources/config.jsonc#profiles -->
```jsonc
  "profiles": {
    "personal": {
      "agent": "claude",
      "home": "~/.claude",
      "default": true,
      "model": "sonnet",
      "args": ["--verbose"],
      "env": { "TEAM": "core" },
      "claude": { "effort": "high" }
    },
    "work": {
      "agent": "claude",
      "home": "~/.claude-work",
      // skip agents.claude.defaults.args for this profile
      "inherit_defaults": false,
      "args": ["--debug"]
    }
  }
```

a plain `agx run --dry-run` in a folder with no binding prints:

<!-- snippet: examples/decide-launch-sources/session.txt#full -->
```sh
$ agx run --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                  from default_agent
profile  personal                                                from default profile of claude
home     ~/.claude                                               from profile personal
login    home login                                              from default (no login named)
binary   /usr/local/bin/claude version unknown                   from no install metadata
folder   ~/projects/app                                          from current folder
options  model=sonnet                                            from profile personal
args     --chrome --verbose                                      from agents.claude.defaults.args, profile personal
env      TEAM                                                    from profile personal env
options  effort=high                                             from profile personal
run      ~/.agx/runs/20261118-061045-u6sj                        from AGX_RUN_ID=20261118-061045-u6sj
command  claude --model sonnet --effort high --chrome --verbose  from built by agentio
env      set TEAM=core                                           from launch environment
         set AGX_RUN_ID=20261118-061045-u6sj                     from launch environment
         set AGX_RUN_AGENT=claude                                from launch environment
         set AGX_RUN_PROFILE=personal                            from launch environment
         set AGX_RUN_LOGIN=home                                  from launch environment
         set AGX_RUN_HOME=~/.claude                              from launch environment
note     installed Claude Code version unknown; flags not checked
```

Read it top to bottom: the left column is the question, the middle the answer, the right where the answer came from. The `agx:` lines above the table are stderr: a note for each answer that no flag of yours gave (here `default_agent`), and the warnings. The version is unknown on the example machine because its fake binary has no install metadata; on a real install the line shows the version.

The same run with flags for most questions: each source changes to the flag, and the args gain what follows `--`:

<!-- snippet: examples/decide-launch-sources/session.txt#flags -->
```sh
$ agx run --dir ~/projects/client --model opus --option effort=max --env TEAM=docs --no-env DEBUG --dry-run -- --ide
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                     from default_agent
profile  personal                                                   from default profile of claude
home     ~/.claude                                                  from profile personal
login    home login                                                 from default (no login named)
binary   /usr/local/bin/claude version unknown                      from no install metadata
folder   ~/projects/client                                          from --dir
options  model=opus                                                 from --model
args     --chrome --verbose --ide                                   from agents.claude.defaults.args, profile personal, after --
env      TEAM                                                       from profile personal env
         TEAM                                                       from --env
         -DEBUG                                                     from --no-env
options  effort=max                                                 from --option (overrides profile personal: high)
run      ~/.agx/runs/20261118-061113-ebor                           from AGX_RUN_ID=20261118-061113-ebor
command  claude --model opus --effort max --chrome --verbose --ide  from built by agentio
env      set TEAM=docs                                              from launch environment
         set AGX_RUN_ID=20261118-061113-ebor                        from launch environment
         set AGX_RUN_AGENT=claude                                   from launch environment
         set AGX_RUN_PROFILE=personal                               from launch environment
         set AGX_RUN_LOGIN=home                                     from launch environment
         set AGX_RUN_HOME=~/.claude                                 from launch environment
note     installed Claude Code version unknown; flags not checked
```

A token never appears: a stored login shows `(not resolved: --dry-run)` and the variable it fills shows `[redacted]`. Add `--resolved-secrets` to resolve each reference anyway: agx then says "resolved" or prints the error, never the value.

`--json` gives the same decisions as data, one object per line of the table, in the same order (see [JSON output](output.md#the-json-envelope)):

<!-- snippet: examples/decide-inspect/session.txt#json -->
```sh
$ agx run --dry-run --quiet --json --field decisions.0 --field decisions.1
{
  "decisions.0": {
    "name": "agent",
    "value": "claude",
    "source": "default_agent"
  },
  "decisions.1": {
    "name": "profile",
    "value": "personal",
    "source": "default profile of claude"
  }
}
```

Three more commands show decisions without launching:

- `agx project show`: the agent, profile, home, login and folder a run in a folder would use, with sources (see [folder bindings](guides/folder-bindings.md#step-3-check-what-a-run-there-does)).
- `agx profile show --id NAME`: each value of one profile with the config line and column it came from.
- `agx config show`: every key of the config with its source (a line and column, or `default`).

<!-- snippet: examples/decide-inspect/session.txt#profile-show -->
```sh
$ agx profile show --id personal
KEY          VALUE       SOURCE
agent        claude      profile personal (config.jsonc:13:7)
home         ~/.claude   profile personal (config.jsonc:14:7)
home_exists  yes         disk
default      true        profile personal (config.jsonc:15:7)
billing      plan        default
login        home login  default (no login named)
args         --chrome    agents.claude.defaults (config.jsonc:7:31)
args         --verbose   profile personal (config.jsonc:17:7)
model        sonnet      profile personal (config.jsonc:16:7)
env          TEAM=(set)  profile personal (config.jsonc:18:16)
```

`agx whoami`, run inside an agent's session, says what that session is running on (see [`agx whoami`](commands/launch.md#agx-whoami)).

## The chain

Each answer comes from the first source that has one. The last column is the `from …` text a dry run prints.

| # | Question | Sources, first match wins | Printed as `from …` |
|---|---|---|---|
| 1 | Agent | the agent of `--profile`, then `--agent`, then the nearest folder binding (if that folder binds one agent), then `default_agent`, then ask | `profile X`, `--agent`, `folder binding DIR`, `default_agent`, `picker`; `conversation ID` for `resume` |
| 2 | Profile | `--profile`, then `--auto-profile` (the profile with the most headroom), then the nearest folder binding for that agent, then the agent's default profile | `--profile`, `--auto-profile (REASON)`, `folder binding DIR`, `default profile of AGENT`; `resume` without `--profile`: `home DIR (default)`, `home DIR (plan profile)`, `home DIR (asked)`, `home DIR` |
| 3 | Home | the profile's `home`; it must exist | `profile X`; `conversation ID (stored there)` for `resume` |
| 4 | Login | `--token-ref`, then `--login` / `--home-login` / `--auto-login`, then `AGX_LOGIN_<AGENT>`, then `AGX_LOGIN`, then the folder binding's login, then the profile's `login` or `home_login`, then the agent default, then the home's own login | `--token-ref`, `--login`, `--home-login`, `--auto-login (REASON)`, `AGX_LOGIN_CLAUDE`, `AGX_LOGIN`, `folder binding DIR`, `profile X`, `agents.AGENT.defaults`, `default (no login named)` |
| 5 | Binary | the agent's binary, the first on `PATH`; the version comes from install metadata, never by running the binary | `install metadata`, `no install metadata` |
| 6 | Folder | `run`: `--dir`, else the current folder; `new`: the new session folder; `resume`: the conversation's folder | `--dir`, `current folder`, `new session folder`, `conversation ID` |
| 7 | Args | `agents.<agent>.defaults.args` (unless the profile sets `"inherit_defaults": false`), then the profile's `args`, then everything after `--`, in that order; a raw `--settings` is taken out and merged in step 8 | the sources that gave any, joined: `agents.claude.defaults.args, profile personal, after --` |
| 8 | Options, settings | a flag (`--model`, `--autonomy`, `--option`, `--setting`), else the profile; a raw `--settings` value is merged over the settings key by key | `--model`, `--option (overrides profile X: VALUE)`, `--setting`, `profile X`, `raw --settings (overrides …)`, `settings_delivery file` |
| 9 | Environment | the profile's `env_file`, `env`, `env_refs`, then `--env-file`, `--env`, `--no-env`; a later source wins a variable | `profile X env_file F`, `profile X env`, `profile X env_refs`, `--env-file F`, `--env`, `--no-env`; the final changes `from launch environment` |
| 10 | Run, command | a new run id; the command is built by agentio from everything above | `AGX_RUN_ID=ID`, `built by agentio` |

Steps 1 to 4 are the same code for `run`, `new`, `resume` and `project show`, so `project show` answers them exactly as a launch would.

## Step 1: the agent

Why: the agent decides which profiles, logins and options can apply at all. With `--profile` the profile names its agent; with `--agent` you name it; otherwise a folder binding or `default_agent` does.

From [`decide-agent-profile`](../examples/decide-agent-profile/README.md), where `~/projects/client` binds only Codex:

<!-- snippet: examples/decide-agent-profile/session.txt#agent-from-profile -->
```sh
$ agx project show --profile work
agent    claude          from profile work
profile  work            from --profile
home     ~/.claude-work  from profile work
login    home login      from default (no login named)
folder   ~/projects/app  from current folder
```

<!-- snippet: examples/decide-agent-profile/session.txt#agent-flag -->
```sh
$ agx project show --agent codex
agent    codex           from --agent
profile  codex           from default profile of codex
home     ~/.codex        from profile codex
login    home login      from default (no login named)
folder   ~/projects/app  from current folder
```

<!-- snippet: examples/decide-agent-profile/session.txt#agent-binding -->
```sh
$ agx project show --dir ~/projects/client
agx: agent codex (folder binding ~/projects/client)
agx: profile codex (folder binding ~/projects/client)
agent    codex              from folder binding ~/projects/client
profile  codex              from folder binding ~/projects/client
home     ~/.codex           from profile codex
login    home login         from default (no login named)
folder   ~/projects/client  from --dir
```

`--agent` and `--profile` together must agree; a mismatch is refused, never resolved in favour of one:

<!-- snippet: examples/decide-agent-profile/session.txt#agent-conflict -->
```sh
$ agx project show --agent codex --profile work
agx: --agent codex and --profile work (a claude profile) disagree; pick one
[exit 2]
```

A folder binding decides the agent only when the nearest bound folder binds exactly one agent. When it binds two (say Claude and Codex), the binding cannot choose between them, so `default_agent` (or `--agent`) does, and the binding then still picks that agent's profile. See [two agents in one folder](guides/folder-bindings.md#step-2-bind-a-second-agent).

## When nothing names an agent

Problem: you typed `agx run` in a folder with no binding, and no `default_agent` is set.

Fix: at a terminal agx shows a picker of the agents that have a profile, with a "remember as default_agent" option; the source then reads `from picker`, and ticking the option writes `default_agent` to the config. Without a terminal it stops (exit 125 for a launching command, 2 for `project show`) and prints the commands that would decide:

<!-- snippet: examples/decide-agent-profile/session.txt#no-agent -->
```sh
$ agx run --dry-run
agx: no profile, binding or agent given, and "default_agent" is not set
  agx run --agent claude  (default profile: personal)
  agx run --agent codex   (default profile: codex)
  agx run --agent gemini
  agx config set --key default_agent --value claude
[exit 125]
```

Set it once, and the next run picks it up with a note on stderr:

<!-- snippet: examples/decide-agent-profile/session.txt#default-agent -->
```sh
$ agx config set --key default_agent --value claude
default_agent = claude
$ agx run --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                 from default_agent
profile  personal                               from default profile of claude
home     ~/.claude                              from profile personal
login    home login                             from default (no login named)
binary   /usr/local/bin/claude version unknown  from no install metadata
folder   ~/projects/app                         from current folder
...
```

## Step 2: the profile

Why: the profile gives the home, and with it the account, history and settings the agent uses.

- `--profile NAME` (or `-p NAME`) always wins.
- `--auto-profile` picks the agent's profile with the most plan headroom from live usage (cost N); the source reads `--auto-profile (REASON)`. See [picking by headroom](guides/auto-pick.md) and [`agx run`](commands/launch.md#agx-run).
- The nearest folder binding that names a profile for this agent (`from folder binding DIR`).
- The agent's default profile, the one with `"default": true` (`from default profile of AGENT`).

`--profile` and `--auto-profile` both choose the profile, so giving both is refused:

<!-- snippet: examples/decide-agent-profile/session.txt#auto-conflict -->
```sh
$ agx run --profile work --auto-profile --dry-run
agx: --profile and --auto-profile both choose the profile; pick one
[exit 125]
```

An agent with profiles but no default, and nothing else naming one, is refused with the two ways out:

<!-- snippet: examples/decide-agent-profile/session.txt#no-default-profile -->
```sh
$ agx project show --agent gemini
agx: no profile chosen for gemini and none is its default
  agx run --profile gem
  agx profile use --id gem
[exit 2]
```

`agx resume` is the one command that starts from a home rather than a profile: the conversation's home decides, and the [shared-home rule](#shared-homes) picks the profile when several profiles use that home.

## Step 3: the home

The home is the profile's `home` folder, printed `from profile X`; for `resume` it is the home that stores the conversation, `from conversation ID (stored there)`. It must exist: agx never creates a home on a launch. A missing one is refused with `profile work: home ~/.claude-work does not exist` and a hint to create it or change the profile's `home`.

On the agent's default home (`~/.claude`, `~/.codex`, `~/.gemini`, `~/.pi/agent`) agx leaves the home variable unset; on any other home it sets it (`CLAUDE_CONFIG_DIR=~/.claude-work` in the `env` lines above), because Claude Code treats a home variable that names its default folder as a different account.

## Step 4: the login

Why: one home can run on its own login (the account you logged in with there) or on a stored login, a token agx hands the agent for this run only. The chain decides which. Every source below is shown in [`decide-login-chain`](../examples/decide-login-chain/README.md), whose config is:

<!-- snippet: examples/decide-login-chain/config.jsonc -->
```jsonc
{
  "version": 1,
  "default_agent": "claude",
  "agents": {
    // every Claude profile without its own login uses carol
    "claude": { "defaults": { "login": "carol" } }
  },
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work", "login": "dave" },
    // the same home as work, on the home's own login: overrides agents.claude.defaults.login
    "work-home": { "agent": "claude", "home": "~/.claude-work", "home_login": true },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude/carol}" },
    "dave": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:work/claude/dave}" }
  },
  "projects": {
    "~/projects/client": { "claude": { "login": "dave" } },
    "~/projects/client/vendor": { "claude": { "home_login": true } }
  }
}
```

From the last source to the first. Nothing names a login, so the home's own login is used:

<!-- snippet: examples/decide-login-chain/session.txt#home-default -->
```sh
$ agx run --profile codex --dry-run
agx: warning: installed codex version unknown; flags not checked against it
agent    codex                                 from profile codex
profile  codex                                 from --profile
home     ~/.codex                              from profile codex
login    home login                            from default (no login named)
...
```

The agent default (`agents.claude.defaults.login`) applies to every Claude profile that does not name its own:

<!-- snippet: examples/decide-login-chain/session.txt#agent-default -->
```sh
$ agx run --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                  from default_agent
profile  personal                                from default profile of claude
home     ~/.claude                               from profile personal
login    carol (not resolved: --dry-run)         from agents.claude.defaults
...
```

A profile's `login` beats the agent default, and a profile's `"home_login": true` turns an inherited default off:

<!-- snippet: examples/decide-login-chain/session.txt#profile-login -->
```sh
$ agx run --profile work --dry-run
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                  from profile work
profile  work                                    from --profile
home     ~/.claude-work                          from profile work
login    dave (not resolved: --dry-run)          from profile work
...
$ agx run --profile work-home --dry-run
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                 from profile work-home
profile  work-home                              from --profile
home     ~/.claude-work                         from profile work-home
login    home login                             from profile work-home
...
```

A folder binding beats the profile; the nearest bound folder that names a login (or `home_login`) wins, and a note says so:

<!-- snippet: examples/decide-login-chain/session.txt#binding -->
```sh
$ agx run --dir ~/projects/client --dry-run
agx: agent claude (folder binding ~/projects/client)
agx: login dave (folder binding ~/projects/client)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                  from folder binding ~/projects/client
profile  personal                                from default profile of claude
home     ~/.claude                               from profile personal
login    dave (not resolved: --dry-run)          from folder binding ~/projects/client
...
$ agx run --dir ~/projects/client/vendor --dry-run
agx: agent claude (folder binding ~/projects/client/vendor)
agx: home login (folder binding ~/projects/client/vendor)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                 from folder binding ~/projects/client/vendor
profile  personal                               from default profile of claude
home     ~/.claude                              from profile personal
login    home login                             from folder binding ~/projects/client/vendor
...
```

`AGX_LOGIN` beats bindings and profiles, for every agent in that terminal; `AGX_LOGIN_<AGENT>` (`AGX_LOGIN_CLAUDE`, `AGX_LOGIN_CODEX`, …) beats `AGX_LOGIN` for one agent. See [per-terminal login](logins.md#per-terminal-login-agx_login).

<!-- snippet: examples/decide-login-chain/session.txt#env -->
```sh
$ AGX_LOGIN=dave agx run --dry-run
agx: agent claude (default_agent)
agx: login dave (AGX_LOGIN)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                  from default_agent
profile  personal                                from default profile of claude
home     ~/.claude                               from profile personal
login    dave (not resolved: --dry-run)          from AGX_LOGIN
...
$ AGX_LOGIN_CLAUDE=carol AGX_LOGIN=dave agx run --profile work --dry-run
agx: login carol (AGX_LOGIN_CLAUDE)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                  from profile work
profile  work                                    from --profile
home     ~/.claude-work                          from profile work
login    carol (not resolved: --dry-run)         from AGX_LOGIN_CLAUDE
...
```

A flag beats everything: `--login NAME`, `--home-login`, `--auto-login` (the stored login with the most headroom, by probing, cost Q) or `--token-ref REF` (a one-off token, printed as `one-off token`):

<!-- snippet: examples/decide-login-chain/session.txt#flags -->
```sh
$ AGX_LOGIN=dave agx run --login carol --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                  from default_agent
profile  personal                                from default profile of claude
home     ~/.claude                               from profile personal
login    carol (not resolved: --dry-run)         from --login
...
$ agx run --profile work --home-login --dry-run
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                 from profile work
profile  work                                   from --profile
home     ~/.claude-work                         from profile work
login    home login                             from --home-login
...
$ agx run --token-ref '${gopass:shared/claude/ci}' --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                   from default_agent
profile  personal                                 from default profile of claude
home     ~/.claude                                from profile personal
login    one-off token (not resolved: --dry-run)  from --token-ref
...
```

Every login, from any source, must serve the run's agent. A Claude login named by `AGX_LOGIN` on a Codex run is refused, never skipped; two login flags in one command are refused; `--auto-login` with no stored login for the agent is refused; an unknown name says where it came from:

<!-- snippet: examples/decide-login-chain/session.txt#refusals -->
```sh
$ AGX_LOGIN=carol agx run --profile codex --dry-run
agx: login carol serves claude; this run is codex
[exit 125]
$ agx run --login carol --token-ref '${gopass:shared/claude/ci}' --dry-run
agx: --token-ref and --login both choose the login; pick one
[exit 125]
$ agx run --profile codex --auto-login --dry-run
agx: auto login: no stored login serves codex
[exit 125]
$ agx run --login nosuch --dry-run
agx: unknown login nosuch (from --login)
  agx login list
[exit 125]
```

## Step 5: the binary

agx runs the agent's binary found first on `PATH` (`claude`, `codex`, `gemini`, `pi`, or a custom agent's `binary`). It reads the version from install metadata (the package's `package.json`, a Homebrew `Cellar` path, a native `versions/` folder), never by running the binary, so a dry run costs nothing:

- `from install metadata`: the version is known; typed options are checked against it, and a version outside the tested range gets a warning.
- `from no install metadata`: the version is unknown; agx warns `installed Claude Code version unknown; flags not checked` and builds the command anyway.

A binary missing from `PATH` stops the launch with exit 127 and the install command, see [troubleshooting](troubleshooting.md#claude-not-found-on-path-install--exit-127).

## Step 6: the folder

The agent starts in one folder (more in [start in another folder with `--dir`](guides/run-folders.md#step-4-start-in-another-folder-with---dir)):

- `agx run`: `--dir DIR` (`from --dir`), else the folder you are in (`from current folder`). The folder must exist; a missing one is refused before anything else: `agx: folder ~/projects/nosuch does not exist` (exit 125).
- `agx new`: a new session folder under `sessions.root` (`from new session folder`), created only when the run really starts:

<!-- snippet: examples/decide-launch-sources/session.txt#new -->
```sh
$ agx new --name spike --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                  from default_agent
profile  personal                                                from default profile of claude
home     ~/.claude                                               from profile personal
login    home login                                              from default (no login named)
binary   /usr/local/bin/claude version unknown                   from no install metadata
folder   ~/.agx/sessions/spike_20261118_061045                   from new session folder
options  model=sonnet                                            from profile personal
args     --chrome --verbose                                      from agents.claude.defaults.args, profile personal
env      TEAM                                                    from profile personal env
options  effort=high                                             from profile personal
run      ~/.agx/runs/20261118-061045-uv21                        from AGX_RUN_ID=20261118-061045-uv21
command  claude --model sonnet --effort high --chrome --verbose  from built by agentio
env      set TEAM=core                                           from launch environment
         set AGX_RUN_ID=20261118-061045-uv21                     from launch environment
         set AGX_RUN_AGENT=claude                                from launch environment
         set AGX_RUN_PROFILE=personal                            from launch environment
         set AGX_RUN_LOGIN=home                                  from launch environment
         set AGX_RUN_HOME=~/.claude                              from launch environment
note     installed Claude Code version unknown; flags not checked
```

- `agx resume`: the folder the conversation ran in (`from conversation ID`), because the agent resumes a conversation only in its own folder. `resume` has no `--dir`.

The folder also chooses the folder binding: bindings are looked up from the launch folder, so `agx run --dir ~/projects/client` uses that folder's binding even when you type it elsewhere.

## Step 7: the args

Raw args reach the agent untouched, in this order: `agents.<agent>.defaults.args`, then the profile's `args`, then everything after `--` on the command line. The `args` line names every source that gave any (see the full dry run above: `--chrome --verbose` `from agents.claude.defaults.args, profile personal`). A profile with `"inherit_defaults": false` skips the agent defaults ([Raw args](guides/raw-args.md) covers all of it):

<!-- snippet: examples/decide-launch-sources/session.txt#no-inherit -->
```sh
$ agx run --profile work --dry-run
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                 from profile work
profile  work                                   from --profile
home     ~/.claude-work                         from profile work
login    home login                             from default (no login named)
binary   /usr/local/bin/claude version unknown  from no install metadata
folder   ~/projects/app                         from current folder
args     --debug                                from profile work
run      ~/.agx/runs/20261118-061045-zjcz       from AGX_RUN_ID=20261118-061045-zjcz
command  claude --debug                         from built by agentio
env      set AGX_RUN_ID=20261118-061045-zjcz    from launch environment
         set AGX_RUN_AGENT=claude               from launch environment
         set AGX_RUN_PROFILE=work               from launch environment
         set AGX_RUN_LOGIN=home                 from launch environment
         set AGX_RUN_HOME=~/.claude-work        from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work   from launch environment
note     installed Claude Code version unknown; flags not checked
```

agx passes raw args as written: a flag in both the defaults and the profile is passed twice, and the agent decides what that means. The one exception is the agent's settings flag (Claude `--settings`), which agx reads and merges into its own settings. See [passing anything to an agent](launch-options.md#recipe-8-fully-raw-passed-as-written-a-raw---settings-merged).

## Steps 8 to 10: options, settings, environment, the command

These are covered in depth in [passing anything to an agent](launch-options.md); the rule is the same everywhere: a flag beats the profile, and each line names its source. An `--option` that replaces a profile value says so (`from --option (overrides profile personal: high)` above), and a `--setting` shows only its keys, then how the settings reach the agent (a raw `--settings` adds a line `from raw --settings`, naming the keys it overrode; see [recipe 8](launch-options.md#a-raw---settings-of-your-own)):

<!-- snippet: examples/decide-launch-sources/session.txt#setting -->
```sh
$ agx run --setting cleanupPeriodDays=30 --dry-run
...
options   effort=high                                                    from profile personal
settings  cleanupPeriodDays                                              from --setting
          delivered as file ~/.agx/runs/20261118-061113-rnho/settings-20261118-061113-rnho.json (0600)  from settings_delivery file
run       ~/.agx/runs/20261118-061113-rnho                               from AGX_RUN_ID=20261118-061113-rnho
...
```

The `env` lines come in two parts: first one line per source with the variable names it sets (`TEAM from profile personal env`, `TEAM from --env`, `-DEBUG from --no-env`), then the final changes the agent receives (`set TEAM=docs from launch environment`), with every secret `[redacted]`: a value is secret by its source (a login token, a reference, an env file, an agent's credential variable), never by its name, so a literal `env` value such as `TEAM=docs` or `CLAUDE_CODE_API_KEY_HELPER_TTL_MS=10000` is shown as written ([what is redacted](output.md#what-is-redacted)). When two sources set the same variable the later one wins, in the order of the chain table. The `run` line is the run folder agx creates for the launch, and `command` the exact command agentio built.

## Notes on stderr

When something other than a flag decided, agx prints one note per answer on stderr, so a binding or a variable is never a silent surprise:

| Note | Printed when |
|---|---|
| `agx: agent claude (default_agent)` | `default_agent` chose the agent |
| `agx: agent codex (folder binding ~/projects/client)` | a binding chose the agent |
| `agx: profile work (folder binding ~/projects/client)` | a binding chose the profile |
| `agx: profile personal (home ~/.claude (plan profile))` | the shared-home rule chose the profile on `resume` |
| `agx: login dave (AGX_LOGIN)`, `agx: login carol (AGX_LOGIN_CLAUDE)` | a variable chose the login |
| `agx: login dave (folder binding ~/projects/client)`, `agx: home login (folder binding DIR)` | a binding chose the login |
| `agx: default_agent set to claude in FILE` | the picker's "remember" option was ticked |

`--quiet` turns these notes off, and the `agx: warning:` lines with them; stdout is unchanged. The notes are also in `data.notes` of `agx project show --json`. See [notes and warnings](output.md#notes-and-warnings).

## Folder bindings

A folder binding chooses the profile or the login for one agent in a folder and every folder below it, so you stop typing `--profile` in a client's repository. In config:

```jsonc
"projects": {
  "~/projects/client": {
    "claude": { "profile": "work", "home_login": true },
    "codex": { "profile": "codex" },
    "gemini": { "login": "gem-work" }
  }
}
```

The nearest bound folder wins, per agent and per field: a sub-folder that binds only a login keeps the profile its parent binds. Paths are stored normalized (absolute, `~` expanded, symlinks resolved), so a folder you reach through a symlink matches its real path. When a binding decides, agx says so on stderr:

<!-- runnable: dir=~/projects/client -->
```sh
$ agx run --dry-run --json --field data.profile
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
...
work
```

The full walk-through (bind, check, override, remove, and every refusal) is in [folder bindings](guides/folder-bindings.md); the commands are in [`agx project`](commands/project.md).

## Shared homes

Several profiles may share one home. For example `personal` and `kimi` can both use `~/.claude`, with `kimi` sending requests to another provider through its own environment. Where a command starts from a home and needs a profile (`resume`, and the home rows of `usage`), agx picks:

1. the agent's default profile (`"default": true`), if it is on that home (`from home DIR (default)`);
2. otherwise the home's only plan profile (`"billing": "plan"`, the default, beats `"billing": "api"`; `from home DIR (plan profile)`);
3. otherwise, at a terminal, it asks and offers to remember the answer as the default (`from home DIR (asked)`); without a terminal it stops and lists the profiles.

A home with a single profile needs no rule (`from home DIR`), and `--profile` always overrides. The full walk-through is in [shared homes](guides/shared-homes.md).

## No discovery at run time

agx only uses homes that are profiles. A `~/.claude-old` folder on disk is ignored until you add it. To find homes, ask explicitly:

```sh
agx profile list --discovered                 # default homes, ~/.claude-* and ~/.codex-* patterns, shell variables, running agents
agx profile list --discovered --search ~/accounts
agx profile import --discovered               # shows a plan; add --yes to write the profiles
```

The `usage` and `conversation list` footers name homes that are not in config, so a forgotten account is visible.

## Pitfalls

- Two choices for one answer in one command are refused, never ranked: `--agent codex and --profile work (a claude profile) disagree; pick one`, `--profile and --auto-profile both choose the profile; pick one`, `--token-ref and --login both choose the login; pick one`. A launching command exits 125, any other command 2.
- `AGX_LOGIN` applies to every agent in the terminal. A login that does not serve the run's agent is refused (`login carol serves claude; this run is codex`), so set `AGX_LOGIN_CLAUDE` instead when you also run other agents there.
- `AGX_LOGIN` beats folder bindings and the profile's own `login`. If a client folder suddenly runs on the wrong account, look for an exported `AGX_LOGIN` (the note on stderr names it).
- `--quiet` hides the notes that tell you a binding or a variable decided. Use it in scripts, not while you are finding out why a run chose something.
- `"default": true` is per agent, not per home: an agent has at most one default profile (`agx config check` reports two). It matters to the shared-home rule only when that default is on the home in question.
- A dry run resolves nothing: `(not resolved: --dry-run)` means the login was chosen, not that its token works. `--dry-run --resolved-secrets` or `agx login check` tests it.

## Where this comes from in config

| Decision | Config key |
|---|---|
| agent when nothing names one | `default_agent` |
| an agent's default profile | `profiles.<profile>.default` |
| plan or pay-per-token, for the shared-home rule and `--auto-profile` | `profiles.<profile>.billing` |
| default args, login | `agents.<agent>.defaults.args`, `agents.<agent>.defaults.login`, `agents.<agent>.defaults.home_login` |
| a profile's args, and whether it takes the agent defaults | `profiles.<profile>.args`, `profiles.<profile>.inherit_defaults` |
| a profile's login | `profiles.<profile>.login`, or `profiles.<profile>.home_login` to override an inherited one |
| folder bindings | `projects.<folder>.<agent>.profile`, `.login`, `.home_login` |

See the [config reference](config.md) for each.

## Related

- [Folder bindings](guides/folder-bindings.md) and [shared homes](guides/shared-homes.md): the two non-flag sources, step by step.
- Choosing: [where to set the profile and login](choosing/where-to-set.md), [a second account](choosing/second-account.md).
- [`agx project`](commands/project.md), [`agx run`, `new`, `resume`](commands/launch.md), [`agx profile show`](commands/profile.md#agx-profile-show), [`agx config show`](commands/setup.md#agx-config-show).
- [Logins](logins.md) for what a login is and how to add one; [passing anything to an agent](launch-options.md) for options, settings and the environment.
- [Output](output.md) for `--json`, `--field` and exit codes; [troubleshooting](troubleshooting.md) for the refusals.
- Examples: [`decide-agent-profile`](../examples/decide-agent-profile/README.md), [`decide-login-chain`](../examples/decide-login-chain/README.md), [`decide-launch-sources`](../examples/decide-launch-sources/README.md), [`decide-inspect`](../examples/decide-inspect/README.md).
