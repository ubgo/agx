# Agents and what they support

agx drives five built-in agents, Claude Code (`claude`), Codex (`codex`), Gemini CLI (`gemini`), pi (`pi`) and opencode (`opencode`), plus any agent you describe in config. They do not all support the same things: Codex and opencode have no token login, Gemini CLI, pi and opencode report no plan usage, pi has no permission modes and opencode only has `full`. agx never guesses what an agent can do. It asks the agent module, and when something is missing it refuses with the agent's own reason instead of half-working.

Read this page when a command says an agent cannot do something, when you want to know before you set up a second account or a login, or when a new agent version is out and you want to know whether agx was tested with it.

## Before you start

- agx installed (`agx version` prints its version).
- The agents you use installed on your `PATH`. agx never installs or updates them; `agx agent list` shows which it found.
- No config is needed for `agx agent list` and `agx agent show`. The examples on this page use one default profile per agent ([`examples/agent-capabilities/config.jsonc`](../../examples/agent-capabilities/config.jsonc)).

## Step 1: see every agent

`agx agent list` reads the agent modules and the disk. It runs no agent binary, makes no network call and resolves no secret, so it is safe to run anywhere (cost R).

<!-- snippet: examples/agent-capabilities/session.txt#list -->
```sh
$ agx agent list
AGENT     KIND      BINARY    VERSION  TESTED            COMPAT   CAPABILITIES
claude    built-in  claude    unknown  2.1.323..2.1.330  unknown  installer,binary-version,identity,usage,cached-usage,subscription,token-login,token-prober,login-commander,conversations,history-checker,history-mover,conversation-mover,prompt-history,live-sessions,process-classifier,turns,home-linker,instruction-files,mcp-servers,skills,plugins,config,run-events,doctor,live-credential,
                                                                  launch-config,process-sessions,stop-planner,turn-files,hooks
codex     built-in  codex     unknown  0.162.1..0.162.1  unknown  installer,binary-version,identity,usage,cached-usage,subscription,login-commander,conversations,history-checker,prompt-history,live-sessions,process-classifier,turns,home-linker,instruction-files,mcp-servers,skills,plugins,config,models,run-events,doctor,launch-config,process-sessions,stop-planner,turn-files,hooks
gemini    built-in  gemini    unknown  0.68.0..0.68.0    unknown  installer,binary-version,identity,token-login,conversations,history-checker,process-classifier,home-linker,instruction-files,mcp-servers,skills,plugins,config,doctor,process-sessions,stop-planner,turn-files,hooks
pi        built-in  pi        unknown  1.5.2..1.5.2      unknown  installer,binary-version,identity,token-login,conversations,history-checker,process-classifier,turns,instruction-files,mcp-servers,skills,config,doctor,process-sessions,stop-planner,turn-files
opencode  built-in  opencode  —        1.18.30..1.18.30  unknown  installer,binary-version,conversations,history-checker,process-classifier,turns,doctor,process-sessions,stop-planner,turn-files
```

| Column | Meaning |
|---|---|
| `KIND` | `built-in`, or `custom` for an agent defined in `agents.<agent>.spec` |
| `BINARY` | the executable agx looks up on `PATH` |
| `VERSION` | the installed version, read from the install metadata: the agent package's `package.json` (npm, pnpm, Bun, Volta), a Homebrew `Cellar/<formula>/<version>` folder, or the native installer's `versions/<version>` folder. `unknown` when none says; `agent show` names the install `method` too. agx never runs the binary to find out unless you ask (step 3) |
| `TESTED` | the version range the agent module was tested against; `—` for a custom agent |
| `COMPAT` | the installed version against that range: `in-range`, `newer`, `older` or `unknown` |
| `CAPABILITIES` | what the agent module supports, in a fixed order (step 2 explains each) |

For a script, the ids alone:

<!-- snippet: examples/agent-capabilities/session.txt#ids -->
```sh
$ agx agent list --json --field data.*.id
["claude","codex","gemini","pi","opencode"]
```

## Step 2: look at one agent

`agx agent show --id AGENT` lists every capability, supported or not, with its cost and, when it is missing, the agent's own reason. It also lists the autonomy levels a launch accepts and the token login kinds.

<!-- snippet: examples/agent-capabilities/session.txt#show-pi -->
```sh
$ agx agent show --id pi
agent: pi (pi, built-in)
binary: pi
path: /usr/local/bin/pi
version: unknown
method: unknown
tested: 1.5.2..1.5.2
compat: unknown
autonomy: default, read-only, full
token kinds: api-key

CAPABILITY          SUPPORTED  COST     REASON
installer           yes        disk     —
binary-version      yes        process  —
identity            yes        process  —
usage               no         network  pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
cached-usage        no         disk     pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
subscription        no         network  pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
token-login         yes        pure     —
token-prober        no         quota    pi has no plan or quota to probe
login-commander     no         pure     pi logs in with /login inside its interactive UI; there is no login subcommand to build
conversations       yes        disk     —
history-checker     yes        disk     —
history-mover       no         disk     pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
conversation-mover  no         disk     pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
prompt-history      no         disk     pi keeps no prompt history file; prompts live only in session files
live-sessions       no         process  pi writes no live-session registry; only the process table shows a running pi
process-classifier  yes        pure     —
turns               yes        disk     —
home-linker         no         disk     pi has no documented set of files to share between agent dirs
instruction-files   yes        disk     —
mcp-servers         yes        disk     —
skills              yes        disk     —
plugins             no         disk     pi packages and extensions are code loaded into the process, not plugins with a manifest this library models
config              yes        disk     —
models              no         disk     pi's model catalog (models-store.json) has no recorded shape yet
run-events          no         pure     pi's --mode json event stream is recorded once but not yet modelled as a decoder
doctor              yes        disk     —
live-credential     no         pure     pi can read a key from a !command in auth.json, but when it re-runs that command is not documented or recorded
launch-config       no         disk     pi has no settings flag and no typed launch options in agentio; pass flags as raw args
process-sessions    yes        process  —
stop-planner        yes        pure     —
turn-files          yes        disk     —
hooks               no         disk     pi has no shell-command hooks: its extensions are TypeScript modules loaded into the process, so there is no settings key to add a command to
```

The `COST` column is what using the capability costs: `pure` computes from what agx already has, `disk` reads files, `process` runs or inspects a process, `network` makes a request, `quota` spends plan quota. The capabilities that matter most day to day:

| Capability | What agx uses it for |
|---|---|
| `usage`, `cached-usage`, `subscription` | `agx usage` (live, `--cached`, `--plan`) and `--auto-profile` |
| `token-login` | `--login`, `agx login add`, `AGX_LOGIN`: a token handed to the agent per run |
| `token-prober` | `agx login check`, `--login-probes`, `--auto-login`, login pools (with `live-credential`) |
| `conversations`, `history-checker` | `agx conversation list`, `agx resume` |
| `history-mover`, `conversation-mover` | `agx conversation move` |
| `live-credential` | live login (`--live-login`, `agx live use`) |
| `launch-config` | typed `--option` and `--setting`, profile agent blocks, settings files |
| `home-linker` | `agx profile add` linking shared files into a new home |
| `binary-version` | `--vendor-checks` |

The levels and kinds as data:

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

## What each built-in agent supports

From `agx agent show` of each agent ([Claude Code](../../examples/agent-capabilities/session.txt), Codex, Gemini CLI, pi and opencode are all in the example session):

| | Claude Code | Codex | Gemini CLI | pi | opencode | custom |
|---|---|---|---|---|---|---|
| Plan usage (`agx usage`) | yes | yes | no | no | no | no |
| Token login (`--login`) | `oauth-token`, `api-key`, `api-key-helper` | no: log in inside the home with `codex login` | `api-key` | `api-key`, per provider | no: its per-provider `auth.json` (`opencode providers login`) | the kinds its `token_env` names (see [custom agent](custom-agent.md#step-6-a-token-login)) |
| Login probe (`login check`) | yes | no | no | no | no | no |
| Conversations and resume | yes | yes | yes | yes | yes (listing reads sessions from before opencode 1.2.0; resume takes any id) | no |
| Move conversations | yes | no: Codex indexes them by absolute path | no | no | no | no |
| Autonomy levels | all five | `default`, `read-only`, `ask`, `full` | all five | `default`, `read-only`, `full` | `default`, `full` | `default` plus the levels in `autonomy_args` |
| Typed options and settings | yes | yes (settings as `-c`) | no: raw args only | no: raw args only | no: raw args only | no: raw args only |
| Live login | yes | no | no | no | no | no |

The five autonomy levels are `default`, `read-only`, `ask`, `accept-edits` and `full`; see [autonomy](autonomy.md).

<!-- snippet: examples/agent-capabilities/session.txt#show-codex -->
```sh
$ agx agent show --id codex
agent: codex (Codex, built-in)
binary: codex
path: /usr/local/bin/codex
version: unknown
method: unknown
tested: 0.162.1..0.162.1
compat: unknown
autonomy: default, read-only, ask, full
token kinds: —

CAPABILITY          SUPPORTED  COST     REASON
installer           yes        disk     —
binary-version      yes        process  —
identity            yes        process  —
usage               yes        network  —
cached-usage        yes        disk     —
subscription        yes        network  —
token-login         no         pure     Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
token-prober        no         quota    Codex has no quota probe: no token login to probe
login-commander     yes        pure     —
conversations       yes        disk     —
history-checker     yes        disk     —
history-mover       no         disk     Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
conversation-mover  no         disk     Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
prompt-history      yes        disk     —
live-sessions       yes        process  —
process-classifier  yes        pure     —
turns               yes        disk     —
home-linker         yes        disk     —
instruction-files   yes        disk     —
mcp-servers         yes        disk     —
skills              yes        disk     —
plugins             yes        disk     —
config              yes        disk     —
models              yes        disk     —
run-events          yes        pure     —
doctor              yes        disk     —
live-credential     no         pure     Codex reads its login from auth.json or the keyring and refreshes it itself; it has no credential-helper setting
launch-config       yes        disk     —
process-sessions    yes        process  —
stop-planner        yes        pure     —
turn-files          yes        disk     —
hooks               yes        disk     —
```

## Step 3: check the version against the tested range

Each built-in agent module was tested against a range of vendor versions (`TESTED`). agx reads the installed version from the install metadata and places it against that range (`COMPAT`). A version outside the range is a warning, never a refusal: the agent still launches, and `agx doctor` reports it.

When the install metadata does not name a version (`unknown`), agx cannot check typed options against the version that introduced them, so a launch warns `installed Claude Code version unknown; flags not checked`. Ask the binary itself with `--vendor-checks`, which runs `<binary> --version` (cost X: it starts a process that may touch the login):

```sh
agx agent show --id claude --vendor-checks
agx doctor --agent claude --vendor-checks
```

`agent show` adds the answer as `vendor_version` (or `vendor_error`) in its data; `doctor` adds a `vendor` row such as `claude --version: claude 2.1.400 is newer than tested (2.1.330)`. See [check your setup](doctor.md#step-3-ask-for-more-checks).

## Step 4: read a refusal

A missing capability is refused with the agent's reason. On a launch (`run`, `new`, `resume`) the exit code is 125, before the agent starts:

<!-- snippet: examples/agent-capabilities/session.txt#refused -->
```sh
$ agx run --profile codex --autonomy accept-edits --dry-run --quiet
agx: codex: autonomy accept-edits: Codex has no edits-only mode; never mapped to a more permissive one
[exit 125]

$ agx run --profile pi --autonomy ask --dry-run --quiet
agx: pi: autonomy ask: pi has no permission modes; it always runs without prompts
[exit 125]

$ agx usage --agent gemini
gem  gem · gemini  ~/.gemini
  unsupported: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
```

On any other command it is exit 4 and `error.code` `unsupported`:

<!-- snippet: examples/out-exit-codes/session.txt#other-4 -->
```sh
$ agx conversation move --dir ~/projects/client --to-profile codex --dry-run
agx: codex cannot move conversations between homes: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
[exit 4]
```

A listing such as `agx usage` does not fail for an agent without the capability: it shows the row as `unsupported` with the reason, as the Gemini row above shows.

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/agent-capabilities$'
```

The session runs `agent list`, `agent show` for each agent and the refusals on a fake machine; [`examples/agent-capabilities/session.txt`](../../examples/agent-capabilities/session.txt) holds every command with its real output, including the full `agent show` of Claude Code and Gemini CLI.

## Pitfalls

- **An unknown id exits 1** and lists the known ones: `agx: unknown agent cursor; known: claude, codex, gemini, pi, opencode`. A custom agent's id joins the list once it is in config.
- **`version unknown` is common** for installs without metadata agx can read (a binary copied by hand, some native installers). It only means typed options are not version-checked; `--vendor-checks` settles it.
- **A second copy of a binary on `PATH` never runs.** `agent show` lists it as `shadowed:` and `agx doctor` as `binary-shadowed`; remove the stale one or fix `PATH` order.
- **`--vendor-checks` runs the binary.** It is the only way agx starts an agent outside a launch; keep it out of status bars and loops.
- **Codex has no edits-only mode.** `--autonomy accept-edits` on Codex is refused (`Codex has no edits-only mode; never mapped to a more permissive one`), never rounded up to `full`.
- **pi has no permission modes.** `ask` and `accept-edits` are refused; without `--autonomy` a pi launch warns `pi runs with full autonomy and never asks`.
- **Agents without plan usage cannot be auto-picked.** `--auto-profile` needs usage; choose a Gemini CLI or pi profile by name.
- **Codex accounts need their own home.** With no token login, a second Codex account is a second profile on its own home (`agx profile add --agent codex --home ~/.codex-work --login-setup`).

## Related

- [A custom agent](custom-agent.md): describe an agent agx does not ship with.
- [Check your setup](doctor.md): `agx doctor` checks every binary and its version.
- [Autonomy](autonomy.md): the five levels per agent.
- Reference: [`agx agent list`](../commands/setup.md#agx-agent-list), [`agx agent show`](../commands/setup.md#agx-agent-show), [exit codes](../output.md#exit-codes).
- Example: [`examples/agent-capabilities`](../../examples/agent-capabilities/README.md).
