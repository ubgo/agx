# Logins

Named logins: plan tokens, API keys, key helpers and login pools. [Logins](../logins.md) is the guide: the kinds, how each reaches the agent, and where a login can be set. No login command prints a token value, except [`agx login token`](#agx-login-token) when you ask for it. The examples below come from [`examples/acct-login-kinds`](../../examples/acct-login-kinds/README.md), [`examples/acct-login-manage`](../../examples/acct-login-manage/README.md), [`examples/acct-use-login`](../../examples/acct-use-login/README.md) [`examples/acct-login-token`](../../examples/acct-login-token/README.md) and [`examples/acct-login-pool`](../../examples/acct-login-pool/README.md), which run in the test suite.

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
| `--kind` | `oauth-token`, `api-key`, `api-key-helper` or `pool`; default the first agent's usual kind |
| `--account` | a display label, such as an email |
| `--provider` | the model provider, for agents with one key per provider (pi: `anthropic`, `openai`, …) |
| `--pool-own` | with `--kind pool`: a member login that is your own account, used up almost fully (repeat) |
| `--pool-shared` | with `--kind pool`: a member login that is a teammate's account, borrowed up to its caps (repeat) |
| `--token-ref` | the token as a reference (`${gopass:…}`, `$(…)`, `${VAR}`) or a plain value, kept in config |
| `--stdin` | read the token from stdin into `tokens.jsonc` (0600) |
| `--login-setup` | run the agent's own token setup (`claude setup-token`), then store the pasted result (needs a terminal) |
| `--helper-command` | for `api-key-helper`: the command the agent runs |

Give one source. With none at a terminal, agx asks with a hidden prompt. A `pool` login takes no source: it names member logins with `--pool-own` and `--pool-shared`, serves exactly one agent (Claude Code), and its caps, windows and timings are keys in `config.jsonc` (see [Share a pool of Claude accounts](../guides/login-pool.md)):

<!-- snippet: examples/acct-login-pool/session.txt#add -->
```sh
$ agx login add --name team --agent claude --kind pool --pool-own alice --pool-shared derek --pool-shared erin
added login team (pool, pool 1 own, 2 shared)

$ agx login show --id team
name     team
agents   claude
kind     pool
source   pool 1 own, 2 shared
account  —
users    —

$ agx login pick --id team
Use: alice (own)
soonest weekly reset among usable accounts
Source: cache (20s old)

Candidates (ready accounts in pick order):
- alice [own] #1 READY (5h 3%, week 29%, resets in 6 d 0 hr, room 41%)
- derek [shared] WAIT (5h 0%, week 15%, resets in 4 d 0 hr, room 55%) (shared reset not within 48h while an own account is usable)
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min, room 27%)
```

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
| `--pool-own` | a pool's own members, replacing the list (repeat) |
| `--pool-shared` | a pool's shared members, replacing the list (repeat) |
| `--pool-force` | make a pool always pick this member, over every rule; empty clears it |

A pool changes only its members, its force and its account label here; its other keys are edited in `config.jsonc`. A login never becomes a pool in place, or the reverse: add a new one.

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
| `--cascade` | also clear it from the profiles, defaults, bindings and pools that use it |

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

## agx login pick

Show which member a pool login picks now, and why, in the stand-alone selector's report line for line: `Use: NAME (GROUP)` (or `(FORCED, ignoring rules)` with an indented `warning:` line per rule it ignores), the reason, any `warning:` lines (a probe that fell back to the cache, a picker that failed), `Source:` (`live agx result`, `cache (25s old)`, or both), a blank line, `Candidates (ready accounts in pick order):`, then one `- NAME [GROUP] …` line per member: `#N READY` in pick order, `WAIT` (usable, held back by the borrow window) or `skip:` with every reason, then the 5-hour and weekly use, the weekly reset (`reset passed` for stale data, `reset unknown` without one) and the room under the weekly cap. It decides as a key request would: `--pool-force`, else the pool's `force`, else its picker command, else the built-in rules. It prints no token. Cost Q and W: a member whose cached usage is older than the pool's `usage_max_age` is probed once (a few tokens of that plan) and the login-usage cache is rewritten; with `--cached` nothing is probed. Guide: [Share a pool of Claude accounts](../guides/login-pool.md).

| Flag | Meaning |
|---|---|
| `--id` | the pool login |
| `--cached` | use the login-usage cache only, of any age; probe nothing |
| `--max-age` | reuse cached usage younger than this (a Go duration such as `30s`; `0` probes every member; default the pool's `usage_max_age`) |
| `--pool-force` | pick this member for this one run, over the pool's rules, its `force` and its picker |
| `--time-format` | how resets read, as in `agx usage`: `relative` (default), `day`, `absolute`, `both`; default config `usage.time_format` |
| `--duration-format` | how a reset a day or more away reads: `truncate` (default), `round`, `minutes`; default config `usage.duration_format` |
| `--tz` | the IANA zone of days and clock times (default this machine's) |

The global `--date-format` (and config `usage.date_format`, `date_format`) writes the clock and date parts, as for `agx usage`.

Exit 0 when a member is picked (or forced); 1 with error code `auto-pick` when no member is usable, the report still printed (the selector exited 2 there; in agx 2 is a usage error); 1 (`not-found`) when `--pool-force` names a login without a reading or not at all; 2 for a login that is not a pool or a bad flag value.

Try it (from [`examples/acct-login-pool`](../../examples/acct-login-pool/README.md)):

<!-- snippet: examples/acct-login-pool/session.txt#pick -->
```sh
$ agx login pick --id auto
Use: carol (shared)
shared account resets within 48h, before any usable own account; using its room before it is lost
Source: cache (20s old)

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr, room 44%)
- alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr, room 69%)
- derek [shared] WAIT (5h 0%, week 15%, resets in 4 d 0 hr, room 55%) (shared reset not within 48h while an own account is usable)
- bob [own] skip: status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 95%) (5h 100%, week 61%, resets in 3 d 0 hr, room 37%)
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min, room 27%)
```

With `--time-format both`, a custom `--date-format` and `--tz`:

<!-- snippet: examples/acct-login-pool/session.txt#times -->
```sh
$ agx login pick --id auto --time-format both --date-format "ddd HH:mm" --tz UTC
Use: carol (shared)
shared account resets within 48h, before any usable own account; using its room before it is lost
Source: cache (20s old)

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr (Sat 00:00), room 44%)
- alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr (Wed 10:00), room 69%)
- derek [shared] WAIT (5h 0%, week 15%, resets in 4 d 0 hr (Mon 10:00), room 55%) (shared reset not within 48h while an own account is usable)
- bob [own] skip: status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 95%) (5h 100%, week 61%, resets in 3 d 0 hr (Sun 10:00), room 37%)
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min (13:00), room 27%)

$ agx login pick --id auto --duration-format minutes --json --field data.selected
{"name":"carol","group":"shared","forced":false,"picked_by":"rules","reason":"shared account resets within 48h, before any usable own account; using its room before it is lost","warnings":[],"weekly_resets_at":"2026-11-21T00:00:00Z","room_percent":44}
```

`--json` carries the selector's `--json` fields in agx's envelope (snake_case): `selected` (`name`, `group`, `forced`, `forced_by`, `picked_by` `rules`/`force`/`picker`, `reason`, `warnings`, `weekly_resets_at`, `room_percent`), `source`, `now`, `warnings`, `short_unavailable_at_percent`, `shared_borrow_window_hours`, and `candidates` (each with `state` `ready`, `waiting` or `blocked`, `rank` (null unless ready), `eligible`, `enabled`, `status`, `account`, `short`, `week`, `weekly_resets_at`, `hours_to_reset`, `room_percent`, `caps`, `reasons`, `notes`, `source`, `age_seconds`); `--field` reads any of them.

Pitfalls: `--pool-force` and the pool's `force` ignore every rule, including a member at its limit; each rule the forced member breaks is a `warning:` line. Do not loop it without `--cached`: every run older than `usage_max_age` probes.

## agx login token

Print one login's token: the value and a newline on stdout, nothing else, for a pipe, a script or a Claude Code `apiKeyHelper`. It resolves the login's source (a stored value, `${gopass:…}`, `$(…)`, `${VAR}` or a plain value) and never refreshes a token. Cost X: a `$(…)` or gopass reference runs that local command; nothing goes over the network.

| Flag | Meaning |
|---|---|
| `--id` | the login |
| `--reveal` | print it even when stdout is a terminal |
| `--pool-force` | on a pool login: print this member's token, over the pool's rules (each rule it breaks is a `warning:` line on stderr) |

```sh
agx login token --id work-key | pbcopy
agx login add --name console --agent claude --kind api-key-helper --helper-command 'agx login token --id work-key'
```

On a `pool` login it picks a member by the pool's rules first (as [`agx login pick`](#agx-login-pick) does, probing members whose cached usage is old) and prints that member's token; a forced pick or a probe that fell back to the cache is a `warning:` line on stderr.

The second line makes any agx login, wherever its token lives, the `apiKeyHelper` of a Claude Code launch. The login may be a plan token (`oauth-token`) or an API key: Claude Code accepts either from a helper, which is how [live login](../guides/live-login.md) delivers plan tokens.

Check a token without showing it: `wc -c` prints its length plus one for the newline, and the first 12 hex digits of its SHA-256 are a fingerprint to compare with the one you expect (`printf '%s\n' "$EXPECTED" | shasum -a 256 | cut -c1-12`):

<!-- snippet: examples/acct-login-token/session.txt#check -->
```sh
$ agx login token --id ci | wc -c
14

$ agx login token --id ci | shasum -a 256 | cut -c1-12
2ca17f7da649
```

Refusals. Each is exactly this one line on stderr, without hint lines, and stdout stays empty:

| Case | Exit | stderr |
|---|---|---|
| stdout is a terminal, no `--reveal` | 2 | `agx: stdout is a terminal; pipe it (\| pbcopy) or add --reveal` |
| `--json` or `--field` | 2 | `agx: login token prints the bare value for scripts and apiKeyHelper; it takes no --json or --field` |
| no `--id` | 2 | `agx: --id is required: the login name (agx login list)` |
| unknown login | 1 | `agx: unknown login NAME` |
| `api-key-helper` login | 4 | `agx: login NAME is an api-key-helper: it has no token to print; the agent runs its helper command itself` |
| `pool` login, no member usable | 1 | `agx: pool NAME: no member is usable: …`, each member with its reasons |
| a reference that does not resolve | 1 | the reason, as `agx login check` gives it, such as `agx: login NAME: VAR is not set`; never a value |

The whole sequence, to try on your own machine with one of your logins in place of `work-key` and a Claude profile in place of `work`:

```sh
agx login token --id work-key                                # refused: stdout is a terminal
agx login token --id work-key --reveal                       # prints it on the terminal
agx login token --id work-key | wc -c                        # length + 1
agx login token --id work-key | shasum -a 256 | cut -c1-12   # fingerprint
agx login token --id work-key | pbcopy                       # to the clipboard (macOS)
agx login token --id work-key --json                         # refused: the output is the bare value
agx login token --id nosuch                                  # refused: unknown login
agx login add --name console --agent claude --kind api-key-helper --helper-command 'agx login token --id work-key'
agx run --profile work --login console --dry-run             # settings apiKeyHelper … from login console
agx login remove --id console
```

Try it (from [`examples/acct-login-token`](../../examples/acct-login-token/README.md); the test runner gives stdout a pipe):

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

<!-- snippet: examples/acct-login-token/session.txt#refusals -->
```sh
$ agx login token --id vault
agx: login vault is an api-key-helper: it has no token to print; the agent runs its helper command itself
[exit 4]

$ agx login token --id ci --json
agx: login token prints the bare value for scripts and apiKeyHelper; it takes no --json or --field
[exit 2]

$ agx login token --id spare
agx: login spare: SPARE_CLAUDE_TOKEN is not set
[exit 1]

$ agx login token
agx: --id is required: the login name (agx login list)
[exit 2]

$ agx login token --id nosuch
agx: unknown login nosuch
[exit 1]
```

Pitfalls: stdout on a terminal is refused (exit 2) unless you add `--reveal`, so a token does not land in scrollback by accident. `--json` and `--field` are refused (exit 2): the output is the bare value. An `api-key-helper` login has no token of its own and is refused (exit 4); agx does not run its command. An unknown login is `not-found` (exit 1), and a reference that does not resolve fails with the same message as `agx login check`, never a value. Every error is exactly one line on stderr, without hint lines, and stdout stays empty, so a script or an `apiKeyHelper` (Claude shows its stderr in `/status`) gets just the reason.
