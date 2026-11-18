# Logins

Named logins: plan tokens, API keys and key helpers. [Logins](../logins.md) is the guide: the kinds, how each reaches the agent, and where a login can be set. No login command ever prints a token value. The examples below come from [`examples/acct-login-kinds`](../../examples/acct-login-kinds/README.md), [`examples/acct-login-manage`](../../examples/acct-login-manage/README.md) and [`examples/acct-use-login`](../../examples/acct-use-login/README.md), which run in the test suite.

## agx login list

Every login: kind, source (reference scheme, command, env variable, plain, stored, helper), agents and account. Cost R.

| Flag | Meaning |
|---|---|
| `--agent` | only logins that serve this agent |

<!-- runnable -->
```sh
$ agx login list
NAME      AGENTS  KIND         SOURCE            ACCOUNT
carol     claude  oauth-token  reference gopass  —
gem-work  gemini  api-key      reference gopass  —
$ agx login list --agent claude --json --field data.*.name
["carol"]
```

## agx login show

One login: kind, source, account, provider, agents, and which profiles, defaults and bindings use it. Cost R.

| Flag | Meaning |
|---|---|
| `--id` | the login |
| `--agent` | show it as seen by this agent |

<!-- runnable -->
```sh
$ agx login use --id carol --profile work
profile work now uses login carol
$ agx login show --id carol
name     carol
agents   claude
kind     oauth-token
source   reference gopass
account  —
users    profile work
```

## agx login add

Add a login. Cost W (X with `--login-setup`). Guide: [Adding a login](../logins.md#adding-a-login).

| Flag | Meaning |
|---|---|
| `--name` | the login name |
| `--agent` | an agent it serves (repeat) |
| `--kind` | `oauth-token`, `api-key` or `api-key-helper`; default the first agent's usual kind |
| `--account` | a display label, such as an email |
| `--provider` | the model provider, for agents with one key per provider (pi: `anthropic`, `openai`, …) |
| `--token-ref` | the token as a reference (`${gopass:…}`, `$(…)`, `${VAR}`) or a plain value, kept in config |
| `--stdin` | read the token from stdin into `tokens.jsonc` (0600) |
| `--login-setup` | run the agent's own token setup (`claude setup-token`), then store the pasted result (needs a terminal) |
| `--helper-command` | for `api-key-helper`: the command the agent runs |

Give one source. With none at a terminal, agx asks with a hidden prompt.

Try it:

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

Pitfalls:

- Codex has no token login (exit 4). `--login-setup` without a terminal exits 2:

<!-- snippet: examples/acct-login-kinds/session.txt#codex -->
```sh
$ agx login add --name codex-team --agent codex --token-ref '${gopass:work/codex}'
agx: Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
[exit 4]
```

<!-- snippet: examples/acct-login-kinds/session.txt#setup -->
```sh
$ agx login add --name newco --agent claude --kind oauth-token --login-setup
agx: no terminal: --login-setup runs the setup command, then asks you to paste the token it prints; pipe the token with --stdin
[exit 2]
```

- A name that exists is refused; change it with `agx login edit`.
- A pi `api-key` needs `--provider`: pi reads a different variable per provider. Without it: `pi reads a different variable per provider; pass --provider: one of …`; with a name pi does not know: `--provider "nope": pi has no api-key provider by that name; use one of …` (exit 2). See [`examples/acct-login-kinds`](../../examples/acct-login-kinds/README.md) region `provider`.

## agx login edit

Rotate or change a login. Every profile, default and binding that uses it keeps using it. Cost W (X with `--login-setup`). Guide: [Rotating and removing](../logins.md#rotating-and-removing).

| Flag | Meaning |
|---|---|
| `--id` | the login |
| `--agent` | replace the agents it serves (repeat) |
| `--kind` | a new kind |
| `--account` | a new display label; empty clears it |
| `--provider` | a new model provider; empty clears it |
| `--token-ref` | a new reference or plain value |
| `--stdin` | a new stored value from stdin |
| `--login-setup` | run the agent's token setup again |
| `--helper-command` | a new helper command |

Try it:

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

## agx login remove

Remove a login from config and `tokens.jsonc`. Refused while a profile, agent default or binding uses it, unless `--cascade`. Cost W.

| Flag | Meaning |
|---|---|
| `--id` | the login |
| `--cascade` | also clear it from the profiles, defaults and bindings that use it |

Try it:

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

## agx login use

Set a default login for a profile, an agent, or this terminal. Cost W (`--terminal` only prints). Guide: [Using a login](../logins.md#using-a-login).

| Flag | Meaning |
|---|---|
| `--id` | the login |
| `--home-login` | the home's own login instead of a named one |
| `--profile` | make it this profile's default |
| `--agent` | make it this agent's default |
| `--terminal` | this terminal only: sets `AGX_LOGIN` through the [shell layer](../shell.md), or prints the `export` line |

Try it:

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

Without the shell layer, `--terminal` prints the line to run:

<!-- runnable -->
```sh
$ agx login use --id carol --terminal
export AGX_LOGIN=carol
$ agx login use --home-login --terminal
unset AGX_LOGIN
```

Pitfalls: `--id` and `--home-login` are one choice; the login must serve the profile's or agent's agent.

## agx login check

Resolve logins and probe them with a minimal request. Cost X, and Q unless `--no-probe`. Exit 3 when some logins fail, 1 when all do.

| Flag | Meaning |
|---|---|
| `--id` | the login (repeat); default every login |
| `--no-probe` | only resolve; spend nothing |

```sh
agx login check --id carol
```

Try it (`spare` reads `${SPARE_CLAUDE_TOKEN}`, which is not set; a key helper is run by the agent, never by agx):

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

Pitfalls: a probe spends a few tokens of each plan; do not run it in a loop. An API key bills per token and has no plan windows to probe.
