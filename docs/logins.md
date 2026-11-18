# Logins

A login is what a launch authenticates with. By default an agent uses the login stored in its home (whatever `claude /login` or `codex login` saved there). A named login lets you run any home on any account: the work home on a personal plan token, or one home on whichever account has quota left.

You want named logins when you have more plans than homes: a second Claude plan you want to use from the home you already have, an API key for pi and Claude at once, a token kept in gopass rather than in an agent's own files. If you want a second account with its own history and settings, add a profile instead ([A second account](guides/second-account.md)).

agx never refreshes, prints or rewrites an agent's own stored OAuth credential. It only hands a token you gave it to the agent, through the agent's environment, and prints a login's token only when you ask with [`agx login token`](commands/login.md#agx-login-token).

## Before you start

- A config with a profile per home ([Getting started](getting-started.md)).
- The token itself:
  - a Claude plan token: run `claude setup-token` (or let `--login-setup` run it for you);
  - an API key from the provider's console;
  - for a token kept in a secret store, the store's command on your PATH (`gopass` for `${gopass:…}` references).
- Codex is not covered: it logs in with `codex login` in its home, and has no token login.

## Kinds

| Kind | Holds | Reaches the agent as | Agents |
|---|---|---|---|
| home login | nothing: the home's own stored login | the agent reads its home | all |
| `oauth-token` | a plan token (from `claude setup-token`): a reference, a plain value, or stored | `CLAUDE_CODE_OAUTH_TOKEN` | claude |
| `api-key` | a pay-per-token key | `ANTHROPIC_API_KEY`, `GEMINI_API_KEY` or pi's provider variable | claude, gemini, pi, and a config-defined agent whose spec's `token_env` names the kind |
| `api-key-helper` | a command the agent itself runs to get a key | `apiKeyHelper` in a settings file of the run; the key never passes through agx | claude |

When agx sets a token, it also removes every variable that would outrank it for that agent (for Claude, `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_API_KEY` outrank `CLAUDE_CODE_OAUTH_TOKEN`), so the login you chose is the one used. `--dry-run` lists what it removed (values are always `[redacted]`):

<!-- snippet: examples/acct-login-kinds/session.txt#oauth -->
```sh
$ ANTHROPIC_API_KEY=old-key ANTHROPIC_AUTH_TOKEN=old-token agx run --profile work --login carol --dry-run --quiet
agent    claude                                  from profile work
profile  work                                    from --profile
home     ~/.claude-work                          from profile work
login    carol (not resolved: --dry-run)         from --login
binary   /usr/local/bin/claude version unknown   from no install metadata
folder   ~/projects/app                          from current folder
run      ~/.agx/runs/20261118-061009-7vne        from AGX_RUN_ID=20261118-061009-7vne
command  claude                                  from built by agentio
env      set AGX_RUN_ID=20261118-061009-7vne     from launch environment
         set AGX_RUN_AGENT=claude                from launch environment
         set AGX_RUN_PROFILE=work                from launch environment
         set AGX_RUN_LOGIN=carol                 from launch environment
         set AGX_RUN_HOME=~/.claude-work         from launch environment
         set CLAUDE_CODE_OAUTH_TOKEN=[redacted]  from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work    from launch environment
         removed ANTHROPIC_API_KEY               from launch environment
         removed ANTHROPIC_AUTH_TOKEN            from launch environment
note     installed Claude Code version unknown; flags not checked
```

An API key goes to the agent's own variable: pi's depends on the login's `provider` (`ANTHROPIC_API_KEY` for `anthropic`), Gemini's is `GEMINI_API_KEY`:

<!-- snippet: examples/acct-login-kinds/session.txt#api-key -->
```sh
$ agx run --agent pi --login anthropic-key --dry-run --quiet
agent    pi                                       from --agent
profile  pi                                       from default profile of pi
home     ~/.pi/agent                              from profile pi
login    anthropic-key (not resolved: --dry-run)  from --login
binary   /usr/local/bin/pi version unknown        from no install metadata
folder   ~/projects/app                           from current folder
run      ~/.agx/runs/20261118-061009-1ut7         from AGX_RUN_ID=20261118-061009-1ut7
command  pi                                       from built by agentio
env      set AGX_RUN_ID=20261118-061009-1ut7      from launch environment
         set AGX_RUN_AGENT=pi                     from launch environment
         set AGX_RUN_PROFILE=pi                   from launch environment
         set AGX_RUN_LOGIN=anthropic-key          from launch environment
         set AGX_RUN_HOME=~/.pi/agent             from launch environment
         set ANTHROPIC_API_KEY=[redacted]         from launch environment
note     pi runs with full autonomy and never asks

$ agx run --agent gemini --login gem-work --dry-run --quiet
agent    gemini                                 from --agent
profile  gem                                    from default profile of gemini
home     ~/.gemini                              from profile gem
login    gem-work (not resolved: --dry-run)     from --login
binary   /usr/local/bin/gemini version unknown  from no install metadata
folder   ~/projects/app                         from current folder
run      ~/.agx/runs/20261118-061009-aips       from AGX_RUN_ID=20261118-061009-aips
command  gemini                                 from built by agentio
env      set AGX_RUN_ID=20261118-061009-aips    from launch environment
         set AGX_RUN_AGENT=gemini               from launch environment
         set AGX_RUN_PROFILE=gem                from launch environment
         set AGX_RUN_LOGIN=gem-work             from launch environment
         set AGX_RUN_HOME=~/.gemini             from launch environment
         set GEMINI_API_KEY=[redacted]          from launch environment
```

A key helper sets no variable: the command goes into a settings file in the run folder, which Claude runs itself:

<!-- snippet: examples/acct-login-kinds/session.txt#helper -->
```sh
$ agx run --profile work --login console --dry-run --quiet
agent     claude                                                                                        from profile work
profile   work                                                                                          from --profile
home      ~/.claude-work                                                                                from profile work
login     console (not resolved: --dry-run)                                                             from --login
binary    /usr/local/bin/claude version unknown                                                         from no install metadata
folder    ~/projects/app                                                                                from current folder
settings  apiKeyHelper                                                                                  from login console
          delivered as file ~/.agx/runs/20261118-120000-zm29/settings-20261118-120000-zm29.json (0600)  from settings_delivery file
run       ~/.agx/runs/20261118-120000-zm29                                                              from AGX_RUN_ID=20261118-120000-zm29
command   claude --settings ~/.agx/runs/20261118-120000-zm29/settings-20261118-120000-zm29.json         from built by agentio
env       set AGX_RUN_ID=20261118-120000-zm29                                                           from launch environment
          set AGX_RUN_AGENT=claude                                                                      from launch environment
          set AGX_RUN_PROFILE=work                                                                      from launch environment
          set AGX_RUN_LOGIN=console                                                                     from launch environment
          set AGX_RUN_HOME=~/.claude-work                                                               from launch environment
          set CLAUDE_CONFIG_DIR=~/.claude-work                                                          from launch environment
note      installed Claude Code version unknown; flags not checked
```

Any agx login can be the helper, wherever its token lives: make the helper command [`agx login token`](#print-a-logins-token), which prints only that login's token. Claude then gets the key of `ci` (here from `${FAKE_TOK}`; it could be gopass, a command or a pasted token) each time it runs the helper:

<!-- snippet: examples/acct-login-token/session.txt#helper -->
```sh
$ agx login add --name console --agent claude --kind api-key-helper --helper-command 'agx login token --id ci'
added login console (api-key-helper, helper agx)

$ agx login show --id console
name     console
agents   claude
kind     api-key-helper
source   helper agx
account  —
users    —
```

## Adding a login

`agx login add` writes a login to config: its name, the agents it serves (`--agent`, repeat it for one key that serves several), its kind (default: the first agent's usual kind) and one token source. `--account` is a display label, such as the account's email; `--provider` is the model provider, for agents that read one key per provider (pi: `anthropic`, `openai`, …).

<!-- snippet: examples/acct-login-kinds/session.txt#add -->
```sh
$ agx login add --name carol --agent claude --kind oauth-token --token-ref '${gopass:personal/claude/carol}'
added login carol (oauth-token, reference gopass)

$ agx login add --name derek --agent claude --kind oauth-token --token-ref '$(gopass show -o personal/claude/derek)' --account e@example.com
added login derek (oauth-token, command gopass)

$ agx login add --name pasted --agent claude --kind oauth-token --stdin < ~/token.txt
added login pasted (oauth-token, stored (tokens.jsonc))

$ agx login add --name console --agent claude --kind api-key-helper --helper-command 'gopass show -o work/anthropic-api-key'
added login console (api-key-helper, helper gopass)

$ agx login add --name anthropic-key --agent claude --agent pi --kind api-key --provider anthropic --token-ref '${gopass:work/anthropic-api-key}'
added login anthropic-key (api-key, reference gopass)

$ agx login add --name gem-work --agent gemini --token-ref '${GEMINI_KEY_WORK}'
added login gem-work (api-key, env GEMINI_KEY_WORK)
```

pi reads a different variable per provider, so a pi login needs `--provider`, and it must be one pi knows for that kind; the message lists them either way:

<!-- snippet: examples/acct-login-kinds/session.txt#provider -->
```sh
$ agx login add --name gh --agent pi --kind api-key --token-ref '${COPILOT_GITHUB_TOKEN}'
agx: pi reads a different variable per provider; pass --provider: one of anthropic, openai, google, google-vertex, azure, deepseek, groq, cerebras, xai, openrouter, mistral, github-copilot, huggingface, fireworks, together, zai, nvidia, vercel-ai-gateway
[exit 2]

$ agx login add --name gh --agent pi --kind api-key --provider nope --token-ref '${COPILOT_GITHUB_TOKEN}'
agx: --provider "nope": pi has no api-key provider by that name; use one of anthropic, openai, google, google-vertex, azure, deepseek, groq, cerebras, xai, openrouter, mistral, github-copilot, huggingface, fireworks, together, zai, nvidia, vercel-ai-gateway
[exit 2]

$ agx login add --name gh --agent pi --kind api-key --provider github-copilot --token-ref '${COPILOT_GITHUB_TOKEN}'
added login gh (api-key, env COPILOT_GITHUB_TOKEN)
```

The token source is one of:

| Source | Where the value lives |
|---|---|
| `--token-ref REF` | in config as a reference (`${gopass:path}`, `$(command)`, `${VAR}`) or a plain value |
| `--stdin` | pasted into `~/.agx/tokens.jsonc` (mode 0600), never in `config.jsonc`; the login gets `"stored": true` |
| `--login-setup` | runs the agent's own token setup (`claude setup-token`), then stores the pasted result like `--stdin` |
| `--helper-command` | an `api-key-helper` command, run by the agent itself |
| none, at a terminal | a hidden prompt; stored like `--stdin` |

In config the logins look like this ([`examples/acct-login-manage/config.jsonc`](../examples/acct-login-manage/config.jsonc)):

<!-- snippet: examples/acct-login-manage/config.jsonc#logins -->
```jsonc
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude/carol}" },
    "derek": { "agents": ["claude"], "kind": "oauth-token", "token": "$(gopass show -o personal/claude/derek)", "account": "e@example.com" },
    "console": { "agents": ["claude"], "kind": "api-key-helper", "command": "gopass show -o work/anthropic-api-key" },
    "anthropic-key": { "agents": ["claude", "pi"], "kind": "api-key", "provider": "anthropic", "token": "${gopass:work/anthropic-api-key}" },
    "spare": { "agents": ["claude"], "kind": "oauth-token", "token": "${SPARE_CLAUDE_TOKEN}" }
  }
```

`agents` is a list, so one key can serve several agents. A plain token value in config is allowed: it is your choice. A login's token is secret by its source, so agx redacts it in every output, plain or not, and `agx doctor` lists plain values by key (never by value) as information. Redaction never goes by a variable's name: a literal profile `env` value is shown as written unless it sits on an agent's credential variable; see [what is redacted](output.md#what-is-redacted).

`agx login list` shows each login's kind and where its value comes from, never the value; `--agent` keeps the logins that serve one agent, and `agx login show --id NAME` shows one login with its provider and every profile, default and binding that uses it:

<!-- snippet: examples/acct-login-kinds/session.txt#list -->
```sh
$ agx login list
NAME           AGENTS     KIND            SOURCE                    ACCOUNT
carol          claude     oauth-token     reference gopass          —
derek          claude     oauth-token     command gopass            e@example.com
pasted         claude     oauth-token     stored (tokens.jsonc)     —
console        claude     api-key-helper  helper gopass             —
anthropic-key  claude,pi  api-key         reference gopass          —
gem-work       gemini     api-key         env GEMINI_KEY_WORK       —
gh             pi         api-key         env COPILOT_GITHUB_TOKEN  —

$ agx login list --agent pi
NAME           AGENTS     KIND     SOURCE                ACCOUNT
anthropic-key  claude,pi  api-key  reference gopass      —
gh             pi         api-key  env COPILOT_GITHUB_TOKEN  —

$ agx login show --id anthropic-key --agent pi
name     anthropic-key
agents   claude, pi
kind     api-key
source   reference gopass
account  —
provider anthropic
users    —
```

## Using a login

| To | Do |
|---|---|
| use it for one run | `agx run --profile work --login carol` |
| use the home's own login for one run | `agx run --profile work --home-login` |
| use a token that is not stored, for one run | `agx run --token-ref '${gopass:personal/claude/eve}' --token-kind oauth-token` |
| use the stored login with the most headroom | `agx run --profile work --auto-login` (probes each login: spends a few tokens; see [Let agx pick by headroom](guides/auto-pick.md)) |
| make it a profile's default | `agx login use --id carol --profile work` |
| make it an agent's default | `agx login use --id carol --agent claude` |
| go back to the home login for a profile | `agx login use --home-login --profile work` |
| use it in this terminal only | `agx login use --id carol --terminal` (see below) |
| use it in one folder | `agx project use --dir ~/projects/client --agent claude --login carol` |

The dry run's login decision names the source that won. Here profile `work` has `"login": "carol"`:

<!-- snippet: examples/acct-use-login/session.txt#per-run -->
```sh
$ agx run --profile work --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"carol (not resolved: --dry-run)","source":"profile work"}

$ agx run --profile work --login dave --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"dave (not resolved: --dry-run)","source":"--login"}

$ agx run --profile work --home-login --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"home login","source":"--home-login"}
```

A one-off token never enters config: `--token-ref` takes a reference or a value, and `--token-kind` says what it is (`oauth-token` by default; `api-key` or `api-key-helper`):

<!-- snippet: examples/acct-use-login/session.txt#one-off -->
```sh
$ agx run --profile personal --token-ref '${gopass:personal/claude/eve}' --token-kind oauth-token --dry-run --quiet
agent    claude                                   from profile personal
profile  personal                                 from --profile
home     ~/.claude                                from profile personal
login    one-off token (not resolved: --dry-run)  from --token-ref
binary   /usr/local/bin/claude version unknown    from no install metadata
folder   ~/projects/app                           from current folder
run      ~/.agx/runs/20261118-061056-j2p5         from AGX_RUN_ID=20261118-061056-j2p5
command  claude                                   from built by agentio
env      set AGX_RUN_ID=20261118-061056-j2p5      from launch environment
         set AGX_RUN_AGENT=claude                 from launch environment
         set AGX_RUN_PROFILE=personal             from launch environment
         set AGX_RUN_LOGIN=token-ref              from launch environment
         set AGX_RUN_HOME=~/.claude               from launch environment
         set CLAUDE_CODE_OAUTH_TOKEN=[redacted]   from launch environment
note     installed Claude Code version unknown; flags not checked
```

A default set with `agx login use` is written to config. An agent default applies to every profile of that agent that names no login of its own; `--home-login` on a profile overrides it:

<!-- snippet: examples/acct-login-manage/session.txt#use -->
```sh
$ agx login use --id carol --profile work
profile work now uses login carol

$ agx login use --id derek --agent claude
agent claude now uses login derek

$ agx run --profile personal --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"derek (not resolved: --dry-run)","source":"agents.claude.defaults"}

$ agx login use --home-login --profile personal
profile personal now uses the home login

$ agx run --profile personal --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"home login","source":"profile personal"}

$ agx login show --id carol
name     carol
agents   claude
kind     oauth-token
source   reference gopass
account  —
users    profile work
```

In config, an agent default is `agents.<agent>.defaults.login`, and a profile that must keep its home's own login sets `"home_login": true` ([`examples/acct-use-login/config.jsonc`](../examples/acct-use-login/config.jsonc)):

<!-- snippet: examples/acct-use-login/config.jsonc#defaults -->
```jsonc
  "agents": {
    "claude": { "defaults": { "login": "carol" } }
  },
```

<!-- snippet: examples/acct-use-login/config.jsonc#isolated -->
```jsonc
    "isolated": { "agent": "claude", "home": "~/.claude-work", "home_login": true },
```

<!-- snippet: examples/acct-use-login/session.txt#home-login -->
```sh
$ agx run --profile personal --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"carol (not resolved: --dry-run)","source":"agents.claude.defaults"}

$ agx run --profile isolated --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"home login","source":"profile isolated"}
```

A folder binding applies in that folder and below:

<!-- snippet: examples/acct-use-login/session.txt#folder -->
```sh
$ agx project use --dir ~/projects/client --agent claude --login dave
~/projects/client: claude login dave

$ agx run --dir ~/projects/client --profile personal --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"dave (not resolved: --dry-run)","source":"folder binding ~/projects/client"}
```

The full precedence is in [How agx decides](how-agx-decides.md#the-chain): `--token-ref`, then `--login` / `--home-login` / `--auto-login`, then `AGX_LOGIN_<AGENT>`, then `AGX_LOGIN`, then the folder binding, the profile's `login`, the agent default, and last the home's own login. In config, "the home login" has no magic name: leave `login` out, or set `"home_login": true` to override a login inherited from an agent default or a binding. Setting both `login` and `home_login` in one place is a config error.

## Per-terminal login: `AGX_LOGIN`

In one terminal you may want every launch on carol's plan, without typing `--login` each time. Set `AGX_LOGIN`: a plain environment variable, exported or prefixed to one command. `AGX_LOGIN_<AGENT>` (such as `AGX_LOGIN_CLAUDE`) applies only to that agent's runs and beats `AGX_LOGIN`:

```sh
AGX_LOGIN=carol agx run --profile work
export AGX_LOGIN=carol
export AGX_LOGIN_CLAUDE=carol     # only for Claude runs; beats AGX_LOGIN for that agent
```

`agx login use --id carol --terminal` checks the login and sets `AGX_LOGIN` in the current shell when the [shell layer](shell.md) is loaded. Without it, the command prints the line to run; `--home-login --terminal` prints the line that unsets it:

<!-- snippet: examples/acct-use-login/session.txt#terminal -->
```sh
$ AGX_LOGIN=dave agx run --profile personal --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"dave (not resolved: --dry-run)","source":"AGX_LOGIN"}

$ AGX_LOGIN=dave AGX_LOGIN_CLAUDE=carol agx run --profile personal --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"carol (not resolved: --dry-run)","source":"AGX_LOGIN_CLAUDE"}

$ agx login use --id dave --terminal
export AGX_LOGIN=dave

$ agx login use --home-login --terminal
unset AGX_LOGIN
```

A login from `AGX_LOGIN` that does not serve the run's agent is refused, never skipped (`AGX_LOGIN_CLAUDE` simply does not apply to a Gemini run):

<!-- snippet: examples/acct-use-login/session.txt#terminal-wrong-agent -->
```sh
$ AGX_LOGIN=dave agx run --agent gemini --dry-run --quiet
agx: login dave serves claude; this run is gemini
[exit 125]

$ AGX_LOGIN_CLAUDE=dave agx run --agent gemini --dry-run --quiet --json --field data.decisions.3
{"name":"login","value":"gem-work (not resolved: --dry-run)","source":"profile gem"}
```

## Inspecting and checking

`agx login check` resolves each login's token and probes it with a minimal request (cost Q: a few tokens of each plan). `--no-probe` only resolves, spending nothing; `--id` (repeat) narrows it to some logins. A login that does not resolve is shown with the reason, and the exit code is 3 when some failed. A key helper is never run by agx:

<!-- snippet: examples/acct-login-manage/session.txt#check -->
```sh
$ agx login check --no-probe
agx: 1 of 5 logins failed: spare
NAME           KIND            SOURCE                  RESOLVE  PROBE    DETAIL
carol          oauth-token     reference gopass        ok       skipped  --no-probe
derek          oauth-token     command gopass          ok       skipped  --no-probe
console        api-key-helper  helper gopass           skipped  skipped  the agent runs the helper itself; agx never does
anthropic-key  api-key         reference gopass        ok       skipped  --no-probe
spare          oauth-token     env SPARE_CLAUDE_TOKEN  failed   skipped  login spare: SPARE_CLAUDE_TOKEN is not set
[exit 3]

$ agx login check --no-probe --id carol --id derek
NAME   KIND         SOURCE            RESOLVE  PROBE    DETAIL
carol  oauth-token  reference gopass  ok       skipped  --no-probe
derek  oauth-token  command gopass    ok       skipped  --no-probe
```

## Rotating and removing

`agx login edit` changes one login in place: a new `--token-ref` or `--stdin` value, a new `--kind`, `--account` or `--provider` (an empty value clears the label), `--agent` (replaces the list), `--helper-command`, or `--login-setup` again. Every profile and default that uses the login keeps using it.

<!-- snippet: examples/acct-login-manage/session.txt#edit -->
```sh
$ agx login edit --id carol --token-ref '${gopass:personal/claude/carol-2}'
changed login carol (oauth-token, reference gopass)

$ agx login edit --id spare --stdin < ~/new-token.txt
changed login spare (oauth-token, stored (tokens.jsonc))

$ agx login edit --id derek --account derek@example.com
changed login derek (oauth-token, command gopass)

$ agx login edit --id console --kind api-key --token-ref '${gopass:work/anthropic-api-key}'
changed login console (api-key, reference gopass)

$ agx login edit --id anthropic-key --provider openai
changed login anthropic-key (api-key, reference gopass)

$ agx login edit --id anthropic-key --agent pi
changed login anthropic-key (api-key, reference gopass)

$ agx login edit --id console --kind api-key-helper --helper-command 'gopass show -o work/anthropic-helper'
changed login console (api-key-helper, helper gopass)

$ agx login edit --id spare --login-setup
agx: no terminal: --login-setup runs the setup command, then asks you to paste the token it prints; pipe the token with --stdin
[exit 2]

$ agx login list
NAME           AGENTS  KIND            SOURCE                 ACCOUNT
carol          claude  oauth-token     reference gopass       —
derek          claude  oauth-token     command gopass         derek@example.com
console        claude  api-key-helper  helper gopass          —
anthropic-key  pi      api-key         reference gopass       —
spare          claude  oauth-token     stored (tokens.jsonc)  —
```

`agx login remove` deletes a login from config and `tokens.jsonc`. It is refused while a profile, agent default or binding uses it; `--cascade` also clears it from those places, which then fall back to the next source (often the home login):

<!-- snippet: examples/acct-login-manage/session.txt#remove -->
```sh
$ agx login remove --id carol
agx: login carol is used by profile work
  agx login remove --id carol --cascade   (also clears it there)
[exit 2]

$ agx login remove --id carol --cascade
removed login carol
  cleared from profile work

$ agx profile list --agent claude
PROFILE    PROVIDER  HOME            ACCOUNT  STATE  LOGIN       BILLING
*personal  claude    ~/.claude                none   home login  plan
work       claude    ~/.claude-work           none   derek       plan
```

## Print a login's token

`agx login token --id NAME` resolves one login's token from its source and prints the value and a newline on stdout, nothing else, for a pipe (`| pbcopy`), a script or an `apiKeyHelper`. It reads only what is stored or referenced and never refreshes a token; a `$(…)` or gopass reference runs that local command (cost X). This is the one command that prints a token; every other output still shows `[redacted]`. From [`examples/acct-login-token`](../examples/acct-login-token/README.md), with stdout on a pipe:

<!-- snippet: examples/acct-login-token/session.txt#sources -->
```sh
$ agx login token --id ci
fake-ci-token

$ agx login token --id carol
fake-token-carol

$ agx login token --id derek
fake-token-derek

$ agx login token --id pasted
fake-pasted-token
```

Stdout on a terminal is refused (exit 2) unless you add `--reveal`, so a token does not land in your scrollback by accident. The other refusals: a key helper has no token of its own and agx does not run its command (exit 4), the output is never wrapped in `--json` or `--field` (exit 2), and a reference that does not resolve fails as it does in `agx login check`, without a value:

<!-- snippet: examples/acct-login-token/session.txt#refusals -->
```sh
$ agx login token --id vault
agx: login vault is an api-key-helper: it has no token to print; the agent runs its helper command itself
  agx login show --id vault
[exit 4]

$ agx login token --id ci --json
{
  "schema_version": 1,
  "command": "login token",
  "ok": false,
  "data": null,
  "decisions": [],
  "warnings": [],
  "error": {
    "code": "usage",
    "message": "agx login token prints the bare value for scripts and apiKeyHelper; it takes no --json or --field"
  }
}
[exit 2]

$ agx login token --id spare
agx: login spare: SPARE_CLAUDE_TOKEN is not set
  agx login check --id spare --no-probe
[exit 1]

$ agx login token --id nosuch
agx: unknown login nosuch
  agx login list
[exit 1]
```

## Live login: switch a running session's account

A long Claude session that hits its plan limit can continue on another account without a restart: start it with `--live-login` (or a profile with `"live_login": true`), then run `agx live use --id RUN --login NAME` from another terminal. Claude picks up the new login at its next key refresh, about ten seconds later, or at once after a rejected key. It is Claude only, needs the run to start on a stored login, and the login you switch to must resolve: Claude stops a session whose key helper fails, with no fallback to the home's own login. The whole flow, its refusals and how it works are in [Live login](guides/live-login.md).

```sh
agx run --profile work --login carol --live-login
agx live list
agx live use --id 20261114-101500-k3x9 --login alice      # applies at the agent's next key refresh (about 10 seconds)
agx live use --id 20261114-101500-k3x9 --start-login      # back to the login it started with
```

## Try it

Every block on this page comes from four tested examples, each on a fake machine (no real account, binary or network):

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-(login-kinds|login-manage|use-login|login-token)$'
```

See [`examples/acct-login-kinds`](../examples/acct-login-kinds/README.md), [`examples/acct-login-manage`](../examples/acct-login-manage/README.md), [`examples/acct-use-login`](../examples/acct-use-login/README.md) and [`examples/acct-login-token`](../examples/acct-login-token/README.md).

## Pitfalls

- Codex has no token login. Adding one is refused as unsupported (exit 4); name the Codex home's own login with `codex login` there:

<!-- snippet: examples/acct-login-kinds/session.txt#codex -->
```sh
$ agx login add --name codex-team --agent codex --token-ref '${gopass:work/codex}'
agx: Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
[exit 4]
```

- `--login-setup` needs a terminal, because you paste the token (exit 2 without one). In a script, pipe the token to `--stdin` instead:

<!-- snippet: examples/acct-login-kinds/session.txt#setup -->
```sh
$ agx login add --name newco --agent claude --kind oauth-token --login-setup
agx: no terminal: --login-setup runs the setup command, then asks you to paste the token it prints; pipe the token with --stdin
[exit 2]
```

- A login must serve the run's agent, from any source (`--login`, `AGX_LOGIN`, a binding, a profile): it is refused, never skipped (exit 125 for a launch, 2 for `profile add` or `edit`):

<!-- snippet: examples/acct-login-kinds/session.txt#wrong-agent -->
```sh
$ agx run --agent gemini --login carol --dry-run
agx: login carol serves claude; this run is gemini
[exit 125]
```

- Two flags that both choose the login are refused (exit 125): `--login`, `--home-login`, `--auto-login` and `--token-ref` are one choice:

<!-- snippet: examples/acct-use-login/session.txt#conflict -->
```sh
$ agx run --profile work --login dave --home-login --dry-run
agx: --login and --home-login both choose the login; pick one
[exit 125]

$ agx run --profile work --login dave --token-ref '${gopass:x}' --dry-run
agx: --token-ref and --login both choose the login; pick one
[exit 125]
```

- A pi API key needs a `provider`, since pi reads one variable per provider. Clearing it on a login that serves pi is refused (exit 2) with the list of providers.
- `--dry-run` never resolves a token (`not resolved: --dry-run`): a typo in a reference shows only at launch, or with `agx login check --no-probe`.
- A `${VAR}` reference reads the variable from the environment agx runs in. If it is not set, the login fails to resolve (`SPARE_CLAUDE_TOKEN is not set`) and the launch is refused.
- The home login is the home's own stored login, read by the agent. agx cannot check or probe it; `agx usage` shows whether the home is logged in.
- `profile list` shows the login a plain `agx run --profile NAME` uses: the profile's own `login`, else the agent default (`agents.<agent>.defaults.login`), else the home login. A folder binding, `AGX_LOGIN` or `--login` can still change it at launch; the dry run's login line shows the one that applies.

## Related

- [Login commands](commands/login.md): every flag.
- [A second account](guides/second-account.md): a new home for a new account.
- [Let agx pick by headroom](guides/auto-pick.md): `--auto-login`.
- [Live login](guides/live-login.md): switch a running session.
- [Plan usage](guides/usage.md): the quota of each home and stored login.
- Decision guides: [which login kind](choosing/login-kind.md), [where to set the profile and login](choosing/where-to-set.md), [continue on another account](choosing/continue-on-another-account.md).
- [How agx decides](how-agx-decides.md#the-chain): the full order.
