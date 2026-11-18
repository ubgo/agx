# A second account

A second account of the same agent (a work Claude plan next to your personal one, a client's Codex login) lives in its own agent home: its own login, history, settings and quota. In agx that home is a profile. This guide adds one end to end: find the homes you already have, add a new one that starts with your usual setup, give it a login, change it later, make it the default, and remove it.

You want this when one account is not enough: separate quota, separate billing, or a client who must not see your personal history. If you only want another account's quota for a run, a named login on the home you have may be enough; see [Logins](../logins.md).

## Before you start

- agx installed and a config file (`agx config init` writes one; [Getting started](../getting-started.md)).
- The agent itself installed (`claude`, `codex`, …). agx never logs in for you: the new home's own login comes from running the agent there once (`claude /login`, `codex login`), or you give it a named login.
- Every example below starts from this config ([`examples/acct-second-account/config.jsonc`](../../examples/acct-second-account/config.jsonc)): one profile on the default home, and one stored login.

<!-- snippet: examples/acct-second-account/config.jsonc -->
```jsonc
{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude/carol}" }
  }
}
```

## Step 1: see which homes exist

agx only uses homes that are profiles; it never looks for homes at run time. `--discovered` looks once, now: each agent's default home, the `~/.claude-*` pattern (Codex has no such convention), home variables in your shell, running agents and custom agents' homes. `LOOKS LIKE A HOME` says whether the folder holds the agent's files or only has the right name.

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

A home in an unusual place is found only with `--search FOLDER` (repeat for more folders). A searched folder counts as a home only when it holds a file distinctive of the agent (`.claude.json` or `.credentials.json` for Claude Code): a name other tools write too, such as `settings.json` or `sessions`, counts alone only in the agent's own folders (`~/.claude`, `~/.claude-*`, the `CLAUDE_CONFIG_DIR` folder). A folder whose distinctive files fit several agents is listed once per agent; you pick the agent when you import it.

## Step 2: preview the new profile

`agx profile add --dry-run` shows the entry it would write and every folder and link it would create, and writes nothing. Without `--home`, the home is the agent's default home plus `-NAME`.

<!-- snippet: examples/acct-second-account/session.txt#add-dry -->
```sh
$ agx profile add --name client --agent claude --dry-run
would add profile client: claude home ~/.claude-client (derived from the agent's default home and --name), login home login, billing plan
  create ~/.claude-client
  link ~/.claude-client/CLAUDE.md -> ~/.claude/CLAUDE.md
  link ~/.claude-client/skills -> ~/.claude/skills
  link ~/.claude-client/settings.json -> ~/.claude/settings.json
```

The `link` lines are the default home's shared files (for Claude: `CLAUDE.md`, `skills` and `settings.json`). Linking them means the second account starts with the same instructions, skills and settings, and a change in one home shows in both. Only files that exist in the default home are linked.

## Step 3: add it

Drop `--dry-run`. The profile is written to config and the folder is created; the dry run of a launch then shows the new home, passed to Claude as `CLAUDE_CONFIG_DIR` (the default home runs with that variable absent).

<!-- snippet: examples/acct-second-account/session.txt#add -->
```sh
$ agx profile add --name client --agent claude
added profile client: claude home ~/.claude-client (derived from the agent's default home and --name), login home login, billing plan
  create ~/.claude-client
  link ~/.claude-client/CLAUDE.md -> ~/.claude/CLAUDE.md
  link ~/.claude-client/skills -> ~/.claude/skills
  link ~/.claude-client/settings.json -> ~/.claude/settings.json

$ agx run --profile client --dry-run --quiet
agent    claude                                  from profile client
profile  client                                  from --profile
home     ~/.claude-client                        from profile client
login    home login                              from default (no login named)
binary   /usr/local/bin/claude version unknown   from no install metadata
folder   ~/projects/app                          from current folder
run      ~/.agx/runs/20261118-061122-5lb0        from AGX_RUN_ID=20261118-061122-5lb0
command  claude                                  from built by agentio
env      set AGX_RUN_ID=20261118-061122-5lb0     from launch environment
         set AGX_RUN_AGENT=claude                from launch environment
         set AGX_RUN_PROFILE=client              from launch environment
         set AGX_RUN_LOGIN=home                  from launch environment
         set AGX_RUN_HOME=~/.claude-client       from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-client  from launch environment
note     installed Claude Code version unknown; flags not checked
```

Now log in once in the new home with the agent's own login (`agx run --profile client`, then `/login` inside Claude Code), or give the profile a named login in step 5.

Variations:

- `--bare`: an empty home, nothing linked (a clean test account). `--home DIR` puts it anywhere:

<!-- snippet: examples/acct-second-account/session.txt#add-bare -->
```sh
$ agx profile add --name scratch --agent claude --home ~/scratch/claude --bare
added profile scratch: claude home ~/scratch/claude (--home), login home login, billing plan
  create ~/scratch/claude
```

- `--login NAME` gives the profile a stored login at once; `--billing api` marks a profile that pays per token (such as an API provider on the default home: it shares `~/.claude` with `personal`, so nothing is created or linked); `--home-login` pins the home's own login even when the agent has a default login:

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

- `--login-setup` runs `claude setup-token` in the new home and stores the pasted token as login NAME, all in one step. It needs a terminal, because you paste the token:

<!-- snippet: examples/acct-second-account/session.txt#login-setup -->
```sh
$ agx profile add --name newco --agent claude --login-setup
agx: no terminal: --login-setup runs the setup command, then asks you to paste the token it prints; run it at a terminal, or use agx login add --stdin
[exit 2]
```

## Step 4: or import homes you already have

If the second home already exists (you ran `CLAUDE_CONFIG_DIR=~/.claude-work claude` before agx), import it instead of adding it. `agx profile import --discovered` shows a plan; `--yes` writes it. Homes that only have the right name are skipped with the reason; import those with `--id`. `--id DIR --name NAME` imports one folder under a name of your choice, and `--agent` decides the agent of a folder that fits several.

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

## Step 5: change a profile

`agx profile edit` changes one profile and prints each change. `--dry-run` shows the changes and writes nothing. An `--env` value is never printed, only `(set)`.

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

`--args` replaces the profile's raw args (one arg per `--args`); `--no-inherit-defaults` drops `agents.<agent>.defaults.args` for this profile. Each change has an undo: `--home-login` clears the login, `--args ''` removes the args, `--inherit-defaults` restores the defaults, `--env NAME=` removes a variable, and `--billing` switches between `plan` and `api`:

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

`--home DIR` moves the profile to another folder that already exists; the old folder is kept.

## Step 6: make it the default, or remove it

The default profile of an agent is the one `agx run --agent claude` uses when nothing else names a profile; the first profile you add for an agent becomes its default. `agx profile use` moves the default; `--profile` (repeat) narrows `profile list`:

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

`agx profile remove` shows a plan without `--yes`, and never deletes the home folder:

<!-- snippet: examples/acct-second-account/session.txt#remove -->
```sh
$ agx profile remove --id scratch
would remove profile scratch (home ~/scratch/claude is kept)
plan only: add --yes to remove it

$ agx profile remove --id scratch --yes
removed profile scratch (home ~/scratch/claude is kept)
```

## Try it

The whole session runs on a fake machine in the test suite (no real account, binary or network):

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-second-account$'
```

See [`examples/acct-second-account`](../../examples/acct-second-account/README.md) for the files.

## Pitfalls

<!-- snippet: examples/acct-second-account/session.txt#pitfalls -->
```sh
$ agx profile add --name client --agent claude
agx: profile client already exists
  agx profile edit --id client
[exit 2]

$ agx profile add --name helper --agent codex --login carol
agx: login carol serves claude, not codex
[exit 2]

$ agx profile import --id ~/.claude-work --id ~/.codex --name both
agx: --name needs exactly one --id
[exit 2]

$ agx profile edit --id client --home ~/.claude-elsewhere
agx: home ~/.claude-elsewhere does not exist
  mkdir -p ~/.claude-elsewhere
[exit 1]
```

- A profile name is unique: `profile add` with a name that exists exits 2 and points at `profile edit`.
- A login must serve the profile's agent. Codex has no token login at all, so a Codex profile always uses its home's own login.
- `profile import --name` needs exactly one `--id`.
- `profile edit --home` and `profile add --home` with a missing folder: `edit` refuses (exit 1, with the `mkdir -p` to run), `add` creates it.
- `--login` and `--home-login` together are refused (exit 2): both choose the login.
- `--login-setup` needs a terminal (exit 2 without one). In a script, run `claude setup-token` yourself and pipe the token to `agx login add --stdin`.
- Two profiles may share one home (as `kimi` and `personal` do above): they share its login, history and quota. `agx profile show` names the other profiles in `shares_home_with`. See [How agx decides](../how-agx-decides.md).
- The default home runs with its home variable absent. Setting `CLAUDE_CONFIG_DIR=~/.claude` yourself makes Claude Code look up its stored login under another name (on macOS, another keychain entry), so it looks logged out.
- `profile remove` keeps the folder. Delete it yourself if you want the history and login gone.

## Related

- [Logins](../logins.md): named logins for any home, and per-terminal logins.
- [Profile commands](../commands/profile.md): every flag.
- [How agx decides](../how-agx-decides.md): which profile a launch uses.
- [Plan usage](usage.md): the quota left on each home.
- [Let agx pick by headroom](auto-pick.md): `--auto-profile` chooses among your profiles.
- [`examples/acct-second-account`](../../examples/acct-second-account/README.md): the session this guide shows.
- [A second account: profile or login](../choosing/second-account.md): the decision guide.
