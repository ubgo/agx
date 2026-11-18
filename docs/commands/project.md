# Projects

A project binding tells agx which profile or login an agent uses in a folder and every folder below it. Bindings are per agent: one folder can bind a Claude profile and a Codex profile. The walk-through is in the [folder bindings guide](../guides/folder-bindings.md); where bindings sit among the other sources is in [how agx decides](../how-agx-decides.md#the-chain). Every example below comes from [`examples/decide-folder-binding`](../../examples/decide-folder-binding/README.md), which runs in the test suite.

Paths are normalized before they are stored and matched: absolute, `~` expanded, symlinks resolved, no trailing separator, and case-folded on case-insensitive volumes (macOS, Windows). For each answer (agent, profile, login) the nearest bound folder that gives it wins, walking up from the launch folder.

In config, bindings live under `projects.<folder>.<agent>` with the keys `profile`, `login` and `home_login` (see the [config reference](../config.md#projects)).

## agx project list

Every binding, one row per folder and agent, sorted by folder. Cost R. No flags of its own. `PROFILE` or `LOGIN` is `—` when the binding does not set it; `home login` means `home_login`.

### Try it

<!-- snippet: examples/decide-folder-binding/session.txt#list -->
```sh
$ agx project list
DIR                    AGENT   PROFILE     LOGIN
~/projects/client      claude  work        —
~/projects/client      codex   codex-work  —
~/projects/client/api  claude  —           carol
~/projects/client/web  claude  —           home login
```

With `--json`, `data` is a list of `{dir, agent, profile, login}`. An empty config prints the header only.

## agx project show

What a run in a folder would do: agent, profile, home, login and folder, each with its source. It runs the same resolver as `agx run --dry-run`, so the answers are identical, but it builds no launch: no binary, args, options or environment. Cost R.

| Flag | Meaning |
|---|---|
| `--dir` | the folder; default the current folder |
| `--agent` | as `run --agent` |
| `--profile`, `-p` | as `run --profile` |
| `--login` | as `run --login` |

The stderr notes are the ones a run would print (one per answer a binding, `default_agent` or `AGX_LOGIN` gave). With `--json`, `data` has `dir`, `agent`, `profile`, `home`, `login`, `binding` (the nearest bound folder that decided something, or empty) and `notes`, and `decisions` holds each line with its source.

### Try it

Inside the bound folder, then naming another agent:

<!-- runnable: dir=~/projects/client -->
```sh
$ agx project show
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
agent    claude             from folder binding ~/projects/client
profile  work               from folder binding ~/projects/client
home     ~/.claude-work     from profile work
login    home login         from default (no login named)
folder   ~/projects/client  from current folder
$ agx project show --agent codex
agent    codex              from --agent
profile  codex              from default profile of codex
home     ~/.codex           from profile codex
login    home login         from default (no login named)
folder   ~/projects/client  from current folder
```

In a sub-folder, the login comes from its own binding and the profile from the parent's:

<!-- snippet: examples/decide-folder-binding/session.txt#nearest -->
```sh
$ agx project show --dir ~/projects/client/api
agx: agent claude (folder binding ~/projects/client/api)
agx: profile work (folder binding ~/projects/client)
agx: login carol (folder binding ~/projects/client/api)
agent    claude                 from folder binding ~/projects/client/api
profile  work                   from folder binding ~/projects/client
home     ~/.claude-work         from profile work
login    carol                  from folder binding ~/projects/client/api
folder   ~/projects/client/api  from --dir
```

<!-- snippet: examples/decide-folder-binding/session.txt#json -->
```sh
$ agx project show --dir ~/projects/client/api --json --field data.binding --field data.notes
agx: agent claude (folder binding ~/projects/client/api)
agx: profile work (folder binding ~/projects/client)
agx: login carol (folder binding ~/projects/client/api)
{
  "data.binding": "~/projects/client/api",
  "data.notes": [
    "agent claude (folder binding ~/projects/client/api)",
    "profile work (folder binding ~/projects/client)",
    "login carol (folder binding ~/projects/client/api)"
  ]
}
```

### Pitfalls

- It never asks and never probes: there is no `--auto-profile`, `--auto-login`, `--home-login` or `--token-ref` here, and where `agx run` would show the agent picker, `project show` stops with `no profile, binding or agent given, and "default_agent" is not set` (exit 2).
- The same conflicts as a launch are refused, with exit 2 instead of 125: `--agent codex and --profile work (a claude profile) disagree; pick one`.
- A folder that does not exist is refused: `folder … does not exist` (exit 1).

## agx project use

Bind a folder for one agent. Cost W. Only the fields you give change: `--login` on a folder that already binds a profile keeps the profile. `--login` replaces a bound `home_login` and the other way round. An existing entry for the same folder spelled differently (by hand, or in another case on macOS and Windows) is edited in place, never doubled.

| Flag | Meaning |
|---|---|
| `--dir` | the folder; default the current folder |
| `--profile`, `-p` | bind this profile; its agent is the binding's agent |
| `--agent` | the agent; needed with `--login` or `--home-login` alone |
| `--login` | the login for this agent in the folder; it must serve the agent |
| `--home-login` | the home's own login in the folder |

```sh
agx project use --profile work
agx project use --dir ~/projects/client --agent claude --login carol
agx project use --agent claude --home-login
```

### Try it

<!-- snippet: examples/decide-folder-binding/session.txt#use -->
```sh
$ agx project use --dir ~/projects/client/ --profile work
~/projects/client: claude profile work

$ agx project use --dir ~/projects/client --profile codex-work
~/projects/client: codex profile codex-work

$ agx project use --dir ~/projects/client/api --agent claude --login carol
~/projects/client/api: claude login carol

$ agx project use --dir ~/projects/client/web --agent claude --home-login
~/projects/client/web: claude home login

$ agx project list
DIR                    AGENT   PROFILE     LOGIN
~/projects/client      claude  work        —
~/projects/client      codex   codex-work  —
~/projects/client/api  claude  —           carol
~/projects/client/web  claude  —           home login
```

The demo config of [getting started](../getting-started.md#step-1-write-a-config) already binds Claude in `~/projects/client`; binding Codex there adds a row:

<!-- runnable -->
```sh
$ agx project use --dir ~/projects/client --profile codex
~/projects/client: codex profile codex
$ agx project list
DIR                AGENT   PROFILE  LOGIN
~/projects/client  claude  work     —
~/projects/client  codex   codex    —
```

### Pitfalls

Every refusal exits 2 (a usage error) except a missing folder (exit 1):

| Message | Cause |
|---|---|
| `give --profile, --login or --home-login` | `--agent` or `--dir` alone binds nothing |
| `--agent is required with --login or --home-login alone (a binding is per agent)` | a login binding needs its agent |
| `login carol serves claude, not codex` | the login does not serve the binding's agent; checked now, not at launch |
| `--agent codex and --profile work (a claude profile) disagree; pick one` | with `--profile`, leave `--agent` out |
| `--login and --home-login both choose the login; pick one` | give one |
| `folder ~/projects/nosuch does not exist` | bind only existing folders |

## agx project remove

Remove a folder's binding, for one agent or for all. Cost W. Only the entry of that exact folder (normalized as above) is removed; bindings on parent and sub-folders stay. Removing the last agent of a folder removes the folder's entry.

| Flag | Meaning |
|---|---|
| `--dir` | the folder; default the current folder |
| `--agent` | only this agent's binding; default all |

```sh
agx project remove --dir ~/projects/client --agent codex
agx project remove --dir ~/projects/client
```

### Try it

<!-- snippet: examples/decide-folder-binding/session.txt#remove -->
```sh
$ agx project remove --dir ~/projects/client --agent codex
removed the codex binding of ~/projects/client
$ agx project remove --dir ~/projects/client/web
removed the claude binding of ~/projects/client/web
$ agx project list
DIR                    AGENT   PROFILE  LOGIN
~/projects/client      claude  work     —
~/projects/client/api  claude  —        carol
```

Without `--dir`, in the bound folder itself:

<!-- snippet: examples/decide-folder-binding/here.session.txt#here -->
```sh
$ agx project use --profile work
~/projects/client: claude profile work
$ agx project show
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
agent    claude             from folder binding ~/projects/client
profile  work               from folder binding ~/projects/client
home     ~/.claude-work     from profile work
login    home login         from default (no login named)
folder   ~/projects/client  from current folder
$ agx project remove
removed the claude binding of ~/projects/client
$ agx project list
no project bindings — bind this folder with `agx project use --profile PROFILE`
```

### Pitfalls

- `no binding for ~/projects/app` (exit 1): that folder has no entry of its own, even if a parent's binding applies there. `agx project list` shows the bound folders.
- `~/projects/client has no gemini binding` (exit 1): the folder is bound, but not for that agent.

## Related

- [Folder bindings guide](../guides/folder-bindings.md): bind, check, override, remove, step by step.
- [How agx decides](../how-agx-decides.md): bindings among the other sources; [shared homes](../guides/shared-homes.md) for the other non-flag profile source.
- Choosing: [where to set the profile and login](../choosing/where-to-set.md).
- Example: [`examples/decide-folder-binding`](../../examples/decide-folder-binding/README.md).
