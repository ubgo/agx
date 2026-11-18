# Profiles

A profile is a name for one agent home plus its launch defaults. agx only uses homes that are profiles; it never discovers homes at run time. [A second account](../guides/second-account.md) is the guide: find, add, import, change, make default and remove a profile, step by step. Most examples below come from [`examples/acct-second-account`](../../examples/acct-second-account/README.md), which runs in the test suite.

## agx profile list

Every profile. Cost R (N with `--plan`).

| Flag | Meaning |
|---|---|
| `--agent` | only this agent |
| `--profile`, `-p` | only these profiles (repeat) |
| `--plan` | add each home's plan and subscription status (cost N) |
| `--discovered` | list agent homes found on this machine instead, and which profile uses each |
| `--search` | with `--discovered`: also look in this folder and its sub-folders (repeat) |

<!-- runnable -->
```sh
$ agx profile list
PROFILE    PROVIDER  HOME            ACCOUNT  STATE  LOGIN       BILLING
*personal  claude    ~/.claude                none   home login  plan
work       claude    ~/.claude-work           none   home login  plan
*codex     codex     ~/.codex                 none   home login  plan
*gem       gemini    ~/.gemini                none   gem-work    plan
$ agx profile list --agent claude --json --field data.*.name
["personal","work"]
```

Profiles are listed in the order the config file has them; `*` marks each agent's default profile. `ACCOUNT` is the email and plan of the login stored in the home and `STATE` is that login's state (`ok`, with the time until it expires when known, `expired`, `none`, or `unknown` when the agent cannot tell); both are read from disk. `LOGIN` is what a plain run of the profile authenticates with.

`--discovered` looks in each agent's default home, the `~/.claude-*` pattern (Codex has no such convention), home variables in your shell (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, …), the start environment of running agents, custom agents' homes, and every `--search` folder:

<!-- snippet: examples/acct-second-account/session.txt#discover -->
```sh
$ agx profile list --discovered
AGENT   PATH            PROFILE   FOUND BY           LOOKS LIKE A HOME
claude  ~/.claude       personal  default home       yes
claude  ~/.claude-work  —         pattern .claude-*  yes
codex   ~/.codex        —         default home       no (name only)
gemini  ~/.gemini       —         default home       no (name only)
pi      ~/.pi/agent     —         default home       no (name only)

$ agx profile list --discovered --search ~/accounts
AGENT   PATH                    PROFILE   FOUND BY           LOOKS LIKE A HOME
claude  ~/.claude               personal  default home       yes
claude  ~/.claude-work          —         pattern .claude-*  yes
codex   ~/.codex                —         default home       no (name only)
gemini  ~/.gemini               —         default home       no (name only)
pi      ~/.pi/agent             —         default home       no (name only)
claude  ~/accounts/claude-acme  —         search ~/accounts  yes
```

`LOOKS LIKE A HOME` is `yes` when the folder holds the agent's files, `no (name only)` when only its name fits. In a searched folder only an agent's distinctive files count (a shared name such as `settings.json` alone counts only in the agent's own folders). A folder whose files fit several agents is listed once per agent; `profile import --agent` decides.

## agx profile show

One profile, with the source of each value (the profile itself, the agent defaults, or built-in defaults). Cost R.

| Flag | Meaning |
|---|---|
| `--id` | the profile |

<!-- runnable -->
```sh
$ agx profile show --id work
KEY          VALUE                          SOURCE
agent        claude                         profile work (config.jsonc:11:15)
home         ~/.claude-work                 profile work (config.jsonc:11:34)
home_exists  yes                            disk
default      false                          default
billing      plan                           default
login        home login                     default (no login named)
args         --chrome                       agents.claude.defaults (config.jsonc:7:31)
env_refs     API_KEY=${gopass:work/api-key} profile work (config.jsonc:11:74)
bound_in     ~/projects/client              projects (config.jsonc:20:40)
```

Each value names where it came from, down to the line and column in `config.jsonc`.

## agx profile add

Create a profile for a home, creating the folder when missing. Unless `--bare`, agx links the agent's shared files (such as instruction files and skills) from the default home into the new one, so a second account starts with the same setup. Cost W.

| Flag | Meaning |
|---|---|
| `--name` | the new profile's name |
| `--agent` | the agent |
| `--home` | the home folder; default the agent's default home plus `-NAME`, such as `~/.claude-work` |
| `--login` | the profile's login |
| `--home-login` | use the home's own login, also over an agent default login |
| `--billing` | `plan` (default) or `api` |
| `--bare` | do not link the default home's shared files |
| `--login-setup` | run the agent's token setup in the new home, then store the pasted token as login NAME (cost X; needs a terminal) |
| `--dry-run` | show the entry and the shared files it would link; write nothing |

```sh
agx profile add --name work --agent claude --login-setup
agx profile add --name kimi --agent claude --home ~/.claude --billing api
```

Try it: the dry run lists the shared files it would link (for Claude: `CLAUDE.md`, `skills` and `settings.json`, when the default home has them):

<!-- snippet: examples/acct-second-account/session.txt#add-dry -->
```sh
$ agx profile add --name client --agent claude --dry-run
would add profile client: claude home ~/.claude-client (derived from the agent's default home and --name), login home login, billing plan
  create ~/.claude-client
  link ~/.claude-client/CLAUDE.md -> ~/.claude/CLAUDE.md
  link ~/.claude-client/skills -> ~/.claude/skills
  link ~/.claude-client/settings.json -> ~/.claude/settings.json
```

<!-- snippet: examples/acct-second-account/session.txt#add-login -->
```sh
$ agx profile add --name team --agent claude --login carol
added profile team: claude home ~/.claude-team (derived from the agent's default home and --name), login carol, billing plan
  create ~/.claude-team
  link ~/.claude-team/CLAUDE.md -> ~/.claude/CLAUDE.md
  link ~/.claude-team/skills -> ~/.claude/skills
  link ~/.claude-team/settings.json -> ~/.claude/settings.json

$ agx profile add --name kimi --agent claude --home ~/.claude --billing api
added profile kimi: claude home ~/.claude (--home), login home login, billing api

$ agx profile add --name solo --agent claude --home-login --bare --dry-run
would add profile solo: claude home ~/.claude-solo (derived from the agent's default home and --name), login home login, billing plan
  create ~/.claude-solo
```

<!-- runnable -->
```sh
$ agx profile add --name scratch --agent codex --bare --dry-run
would add profile scratch: codex home ~/.codex-scratch (derived from the agent's default home and --name), login home login, billing plan
  create ~/.codex-scratch
```

Pitfalls: a name that exists is refused (exit 2, use `profile edit`); `--login` must serve the agent and cannot be combined with `--home-login` (exit 2); `--login-setup` needs a terminal (exit 2 without one). The first profile of an agent becomes its default.

<!-- snippet: examples/acct-second-account/session.txt#login-setup -->
```sh
$ agx profile add --name newco --agent claude --login-setup
agx: no terminal: --login-setup runs the setup command, then asks you to paste the token it prints; run it at a terminal, or use agx login add --stdin
[exit 2]
```

## agx profile import

Write profiles for discovered homes. Shows a plan unless `--yes`. Cost W.

| Flag | Meaning |
|---|---|
| `--discovered` | every discovered home that looks like a home and has no profile |
| `--id` | a home folder to import (repeat) |
| `--name` | the profile name (one `--id` only) |
| `--agent` | only this agent's homes; decides the agent of an `--id` folder |
| `--search` | also look in this folder and its sub-folders (repeat) |
| `--yes` | write the profiles; without it agx only shows the plan |

```sh
agx profile import --discovered
agx profile import --id ~/.claude-acmecorp --name acmecorp --yes
```

Try it: homes that are already profiles, or only have the right name, are skipped with the reason:

<!-- snippet: examples/acct-second-account/session.txt#import -->
```sh
$ agx profile import --discovered
NAME  AGENT   HOME            DEFAULT  FOUND BY
work  claude  ~/.claude-work  no       pattern .claude-*
skipped claude ~/.claude: already a profile
skipped claude ~/.claude-client: already a profile
skipped claude ~/.claude-team: already a profile
skipped codex ~/.codex: name only, no home markers (import it with --id)
skipped gemini ~/.gemini: name only, no home markers (import it with --id)
skipped pi ~/.pi/agent: name only, no home markers (import it with --id)
plan only: add --yes to write these profiles

$ agx profile import --discovered --yes
NAME  AGENT   HOME            DEFAULT  FOUND BY
work  claude  ~/.claude-work  no       pattern .claude-*
skipped claude ~/.claude: already a profile
skipped claude ~/.claude-client: already a profile
skipped claude ~/.claude-team: already a profile
skipped codex ~/.codex: name only, no home markers (import it with --id)
skipped gemini ~/.gemini: name only, no home markers (import it with --id)
skipped pi ~/.pi/agent: name only, no home markers (import it with --id)
wrote 1 profile

$ agx profile import --id ~/accounts/claude-acme --name acme --agent claude --search ~/accounts --yes
NAME  AGENT   HOME                    DEFAULT  FOUND BY
acme  claude  ~/accounts/claude-acme  no       search ~/accounts
wrote 1 profile
```

Pitfalls: `--name` needs exactly one `--id` (exit 2); a name-only home is imported only by `--id`.

## agx profile edit

Change a profile. Cost W.

| Flag | Meaning |
|---|---|
| `--id` | the profile |
| `--login` | set the profile's login |
| `--home-login` | use the home's own login |
| `--home` | move the profile to another home folder (the old folder is kept) |
| `--args` | replace the profile's raw args (repeat, one arg each; `--args ""` removes them) |
| `--inherit-defaults` | use `agents.<agent>.defaults.args` first (the default) |
| `--no-inherit-defaults` | drop `agents.<agent>.defaults.args` for this profile |
| `--env` | set a literal variable `NAME=value`; `NAME=` removes it (repeat) |
| `--billing` | `plan` or `api` |
| `--dry-run` | show the changes; write nothing |

```sh
agx profile edit --id work --login carol
agx profile edit --id kimi --no-inherit-defaults --args=--dangerously-skip-permissions
```

Try it: each change is printed, an `--env` value only as `(set)`:

<!-- snippet: examples/acct-second-account/session.txt#edit -->
```sh
$ agx profile edit --id client --login carol --billing plan --env TEAM=client --dry-run
profile client: would change (--dry-run)
  login     home login  ->  carol
  env.TEAM  —           ->  (set)
  billing   plan        ->  plan

$ agx profile edit --id client --login carol --env TEAM=client
profile client: changed
  login     home login  ->  carol
  env.TEAM  —           ->  (set)

$ agx profile edit --id client --args --chrome --args --verbose
profile client: changed
  args  —  ->  --chrome --verbose

$ agx profile edit --id client --no-inherit-defaults
profile client: changed
  inherit_defaults  true  ->  false

$ agx profile show --id client
KEY               VALUE               SOURCE
agent             claude              profile client (config.jsonc:7:7)
home              ~/.claude-client    profile client (config.jsonc:8:7)
home_exists       yes                 disk
default           false               default
billing           plan                default
login             carol               profile client (config.jsonc:9:7)
inherit_defaults  false               profile client (config.jsonc:14:7)
args              --chrome --verbose  profile client (config.jsonc:13:7)
env               TEAM=(set)          profile client (config.jsonc:11:9)
```

And undone:

<!-- snippet: examples/acct-second-account/session.txt#edit-undo -->
```sh
$ agx profile edit --id client --home-login --args '' --inherit-defaults --env TEAM= --billing api
profile client: changed
  login             carol               ->  home login
  args              --chrome --verbose  ->  —
  inherit_defaults  false               ->  true
  env.TEAM          (set)               ->  —
  billing           plan                ->  api
```

Pitfalls: `--home` must name a folder that exists (exit 1, with the `mkdir -p` to run); `--login` and `--home-login` are one choice.

## agx profile remove

Remove a profile entry. The home folder is never deleted. Cost W.

| Flag | Meaning |
|---|---|
| `--id` | the profile |
| `--yes` | remove it; without it agx only shows the plan |

<!-- snippet: examples/acct-second-account/session.txt#remove -->
```sh
$ agx profile remove --id scratch
would remove profile scratch (home ~/scratch/claude is kept)
plan only: add --yes to remove it

$ agx profile remove --id scratch --yes
removed profile scratch (home ~/scratch/claude is kept)
```

## agx profile use

Make a profile its agent's default. Cost W.

| Flag | Meaning |
|---|---|
| `--id` | the profile |

```sh
agx profile use --id work
```

Try it:

<!-- snippet: examples/acct-second-account/session.txt#use -->
```sh
$ agx profile list --profile client --profile personal
PROFILE    PROVIDER  HOME              ACCOUNT  STATE  LOGIN       BILLING
*personal  claude    ~/.claude                  none   home login  plan
client     claude    ~/.claude-client           none   home login  api

$ agx profile use --id client
profile client is the default claude profile
  (was: personal)

$ agx profile list
PROFILE   PROVIDER  HOME                    ACCOUNT  STATE  LOGIN       BILLING
personal  claude    ~/.claude                        none   home login  plan
*client   claude    ~/.claude-client                 none   home login  api
scratch   claude    ~/scratch/claude                 none   home login  plan
team      claude    ~/.claude-team                   none   carol       plan
kimi      claude    ~/.claude                        none   home login  api
work      claude    ~/.claude-work                   none   home login  plan
acme      claude    ~/accounts/claude-acme           none   home login  plan
```
