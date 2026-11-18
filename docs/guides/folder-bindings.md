# Folder bindings

A folder binding tells agx which profile or which login an agent uses in a folder and in every folder below it. Bind a client's repository to the client account once, and a plain `agx run` there starts on that account; everywhere else your default stays.

You want one when you keep typing `--profile work` in the same folders, when one repository must always run on a client's login, or when a sub-folder (a vendored project, a sandbox) should run on a different login than its parent. A binding is per agent: the same folder can use one Claude profile and another Codex profile.

## Before you start

- A config with the profiles and logins you want to bind (`agx profile list`, `agx login list`). A binding names them; it never creates them.
- The folder must exist.

The commands below come from [`examples/decide-folder-binding`](../../examples/decide-folder-binding/README.md), which runs in the test suite on a fake machine with these profiles and one login:

<!-- snippet: examples/decide-folder-binding/config.jsonc#profiles -->
```jsonc
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work" },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true },
    "codex-work": { "agent": "codex", "home": "~/.codex-work" }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude/carol}" }
  }
```

## Step 1: bind a folder to a profile

`agx project use --profile NAME` binds the profile's agent in `--dir` (default: the folder you are in). The agent comes from the profile, so you do not name it. The folder is stored normalized: absolute, `~` expanded, symlinks resolved, no trailing separator, so `~/projects/client/` and `~/projects/client` are one binding.

<!-- snippet: examples/decide-folder-binding/session.txt#use-profile -->
```sh
$ agx project use --dir ~/projects/client/ --profile work
~/projects/client: claude profile work
```

## Step 2: bind a second agent

Bindings are per agent, so binding a Codex profile on the same folder adds to the Claude binding instead of replacing it:

<!-- snippet: examples/decide-folder-binding/session.txt#use-codex -->
```sh
$ agx project use --dir ~/projects/client --profile codex-work
~/projects/client: codex profile codex-work
```

With two agents bound, the folder no longer chooses the agent on its own: a plain `agx run` there takes the agent from `default_agent` (or asks), and the binding then picks that agent's profile. `--agent codex` picks Codex and its bound profile:

<!-- snippet: examples/decide-folder-binding/session.txt#two-agents -->
```sh
$ agx project show --dir ~/projects/client
agx: agent claude (default_agent)
agx: profile work (folder binding ~/projects/client)
agent    claude             from default_agent
profile  work               from folder binding ~/projects/client
home     ~/.claude-work     from profile work
login    home login         from default (no login named)
folder   ~/projects/client  from --dir
$ agx project show --dir ~/projects/client --agent codex
agx: profile codex-work (folder binding ~/projects/client)
agent    codex              from --agent
profile  codex-work         from folder binding ~/projects/client
home     ~/.codex-work      from profile codex-work
login    home login         from default (no login named)
folder   ~/projects/client  from --dir
```

## Step 3: check what a run there does

`agx project show` answers what `agx run` would in that folder: agent, profile, home, login and folder, each with its source, through the same code as a launch, without building one. The `agx:` lines on stderr are the notes a run would print. Run without `--dir`, every project command works on the current folder:

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

`--json` gives the nearest bound folder that decided something (`data.binding`) and the notes (`data.notes`), for a status line or a script:

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

## Step 4: bind only a login, or the home's own login

`--login NAME` binds a stored login, `--home-login` the home's own login (the account you logged in with in that home). Either alone needs `--agent`, because a binding is per agent. Only the fields you give change, so a login binding keeps a profile bound on the same folder.

<!-- snippet: examples/decide-folder-binding/session.txt#use-login -->
```sh
$ agx project use --dir ~/projects/client/api --agent claude --login carol
~/projects/client/api: claude login carol

$ agx project use --dir ~/projects/client/web --agent claude --home-login
~/projects/client/web: claude home login
```

<!-- snippet: examples/decide-folder-binding/session.txt#list -->
```sh
$ agx project list
DIR                    AGENT   PROFILE     LOGIN
~/projects/client      claude  work        —
~/projects/client      codex   codex-work  —
~/projects/client/api  claude  —           carol
~/projects/client/web  claude  —           home login
```

## Step 5: rely on the nearest folder

agx walks up from the launch folder (`--dir` or the current folder) and, for each answer, takes the nearest folder that binds it, per agent and per field. In `~/projects/client/api` the login comes from the `api` binding and the profile from its parent:

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

The same happens on a launch, with one note per answer the binding gave; `--quiet` drops the notes:

<!-- snippet: examples/decide-folder-binding/session.txt#run -->
```sh
$ agx run --dir ~/projects/client/web --dry-run
agx: agent claude (folder binding ~/projects/client/web)
agx: profile work (folder binding ~/projects/client)
agx: home login (folder binding ~/projects/client/web)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                 from folder binding ~/projects/client/web
profile  work                                   from folder binding ~/projects/client
home     ~/.claude-work                         from profile work
login    home login                             from folder binding ~/projects/client/web
...
$ agx run --dir ~/projects/client/web --dry-run --quiet
agent    claude                                 from folder binding ~/projects/client/web
profile  work                                   from folder binding ~/projects/client
home     ~/.claude-work                         from profile work
login    home login                             from folder binding ~/projects/client/web
...
```

Written by hand, the same kind of bindings look like this in `~/.agx/config.jsonc` (from [`decide-login-chain`](../../examples/decide-login-chain/config.jsonc); `~` is fine in a key you write yourself, and `agx project use` writes the absolute path):

<!-- snippet: examples/decide-login-chain/config.jsonc#projects -->
```jsonc
  "projects": {
    "~/projects/client": { "claude": { "login": "dave" } },
    "~/projects/client/vendor": { "claude": { "home_login": true } }
  }
```

## Step 6: override for one run

A flag always beats a binding: `--profile`, `--agent`, `--login`, `--home-login`, `--token-ref`. So do `AGX_LOGIN` and `AGX_LOGIN_<AGENT>`, which beat a binding's login (see [the login chain](../how-agx-decides.md#step-4-the-login)).

<!-- snippet: examples/decide-folder-binding/session.txt#override -->
```sh
$ agx project show --dir ~/projects/client/api --profile personal --login carol
agent    claude                 from profile personal
profile  personal               from --profile
home     ~/.claude              from profile personal
login    carol                  from --login
folder   ~/projects/client/api  from --dir
```

## Step 7: remove a binding

`agx project remove` drops the binding of `--dir` (default: the current folder) for one agent with `--agent`, or for every agent without it. It removes only that folder's entry; bindings on parent or sub-folders stay.

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

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/decide-folder-binding$'
```

Or on your own machine, in any folder, with a profile `work`:

```sh
agx project use --profile work
agx project show
agx run --dry-run
agx project remove
```

## Pitfalls

Every refusal from the example, with its exit code:

<!-- snippet: examples/decide-folder-binding/session.txt#refusals -->
```sh
$ agx project use --dir ~/projects/client --agent claude
agx: give --profile, --login or --home-login
[exit 2]
$ agx project use --dir ~/projects/client --login carol
agx: --agent is required with --login or --home-login alone (a binding is per agent)
[exit 2]
$ agx project use --dir ~/projects/client --agent codex --login carol
agx: login carol serves claude, not codex
[exit 2]
$ agx project use --dir ~/projects/client --agent codex --profile work
agx: --agent codex and --profile work (a claude profile) disagree; pick one
[exit 2]
$ agx project use --dir ~/projects/client --agent claude --login carol --home-login
agx: --login and --home-login both choose the login; pick one
[exit 2]
$ agx project use --dir ~/projects/nosuch --profile work
agx: folder ~/projects/nosuch does not exist
[exit 1]
$ agx project remove --dir ~/projects/app
agx: no binding for ~/projects/app
  agx project list
[exit 1]
$ agx project remove --dir ~/projects/client --agent gemini
agx: ~/projects/client has no gemini binding
  agx project list
[exit 1]
```

- `give --profile, --login or --home-login`: `--agent` alone binds nothing.
- `--agent is required with --login or --home-login alone`: name the agent the login is for.
- `login carol serves claude, not codex`: a login binding must serve the binding's agent, checked when you bind, not later at launch.
- `--agent codex and --profile work (a claude profile) disagree`: with `--profile`, leave `--agent` out.
- `folder … does not exist`, `no binding for …`, `… has no gemini binding` exit 1: the folder or binding is not there; `agx project list` shows what is.
- An exported `AGX_LOGIN` beats every binding's login. If a bound folder runs on the wrong account, the stderr note (`login dave (AGX_LOGIN)`) names the variable.
- A binding for two agents on one folder does not choose the agent; set `default_agent` or pass `--agent`.
- Paths compare case-insensitively on macOS and Windows and exactly on Linux. A folder reached through a symlink matches the binding of its real path.
- `agx project show` never asks and never probes: it does not offer `--auto-profile` or `--auto-login`, and a run that would show the agent picker is refused there instead.

## Related

- [How agx decides](../how-agx-decides.md): where bindings sit in the chain for the agent, the profile and the login.
- [`agx project`](../commands/project.md): every flag of `use`, `show`, `list` and `remove`.
- [Config reference: projects](../config.md#projects): the `projects.<folder>.<agent>` keys.
- [Shared homes](shared-homes.md): when the profile comes from a home instead.
- Choosing: [where to set the profile and login](../choosing/where-to-set.md) (a binding, a flag, `AGX_LOGIN` or the profile).
- Example: [`examples/decide-folder-binding`](../../examples/decide-folder-binding/README.md).
