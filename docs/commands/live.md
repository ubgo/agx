# Live login

Switch the login of a running Claude session. Start the session with `--live-login` (or a profile with `"live_login": true`); [Live login](../guides/live-login.md) is the guide: how it works, the whole flow and its limits. The examples below come from [`examples/acct-live-login`](../../examples/acct-live-login/README.md), where a seeded run folder stands in for a session running in another terminal.

A raw `--settings` of your own is merged with the settings live login adds, so one `--settings` reaches Claude; a raw value that sets `apiKeyHelper` itself is refused at launch: `agx: raw --settings apiKeyHelper and --live-login both set the credential; pick one` (exit 125). See [the live-login pitfalls](../guides/live-login.md#pitfalls).

## agx live list

Running sessions started with live login (every pool run is one). Runs whose agent has ended are left out (`agx clean` removes them); a run whose liveness cannot be read is listed as `unknown`. Cost R: run folders, the process table and the agent's own session files; no network.

| Flag | Meaning |
|---|---|
| `--agent` | only this agent |
| `--wide` | every column: also the agent, profile, terminal, parent process, the login it started on and gets next, since when it is on its login, and the conversation with its title |

<!-- runnable -->
```sh
$ agx live list
RUN  LABEL  FOLDER  PID  IN USE  LAST KEY  LIVENESS
```

The columns tell the runs apart and say what each is on now:

| Column | Meaning |
|---|---|
| `RUN` | the run id (`AGX_RUN_ID` inside the session) |
| `LABEL` | the run's `--label` (or `AGX_RUN_LABEL` at launch); `—` when it has none |
| `FOLDER` | the folder it runs in, `~`-shortened and cut to its last 32 characters |
| `PID` | the agent's process id |
| `IN USE` | the login whose key the agent holds now (for a pool, the member); `none (all over caps) since 10:52` when its last key request was refused and it was handed no key ([when no account is usable](../guides/pool-rules.md#when-no-account-is-usable)); `alice (no key since 10:52: all over caps; the agent keeps its key)` when the pool's `when_none_usable` is `keep`; `—` before the first request |
| `LAST KEY` | when a real key was last handed out |
| `LIVENESS` | `alive`, or `unknown` with its reason |

`--wide` adds `AGENT`, `PROFILE`, `TTY` (the terminal at launch, such as `pts/3`), `PARENT` (the program that started agx: a shell, a terminal tool, a script), `STARTED ON`, `NEXT` (the login of the next key request), `SINCE` (when `IN USE` last changed), `CONVERSATION` and `TITLE` (the conversation running in that process, when the agent lists its live sessions). `--json` has every field of every run, a stable part of the output any tool may read: `id`, `dir`, `agent`, `profile`, `home`, `folder`, `folder_short`, `label`, `pid` (left out when unknown), `tty`, `parent`, `env` (the `live.record_env` values), `conversation_id`, `conversation_title`, `start_login`, `chosen_login`, `in_use`, `in_use_at` (the agent's last key request), `in_use_since`, `in_use_text` (the `IN USE` cell), `key_at` (the last real key), `next`, `only`, `no_key` (why the last request got no key, in one line), `no_key_brief` (the same in a few words), `no_key_since`, `when_none_usable` (how that refusal was answered: `stop` or `keep`), `liveness` and `reason`.

Each row is one run started with `--live-login`: the login it started on, the login it gets at its next refresh, the login in use, and whether its process is still alive:

<!-- snippet: examples/acct-live-login/session.txt#list -->
```sh
$ agx live list --agent claude
RUN                   LABEL  FOLDER          PID  IN USE  LAST KEY  LIVENESS
20261114-101500-k3x9  —      ~/projects/app  —    —       —         unknown

$ agx live show --id 20261114-101500-k3x9
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      —
next        carol
in use      —
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

## agx live show

One run's live state, ended or not: the login it started with, the login chosen, the login it gets next, and the login in use at the agent's last refresh. Never a token. Cost R.

| Flag | Meaning |
|---|---|
| `--id` | the run id |

```sh
agx live show --id 20261114-101500-k3x9
```

Besides the lines above it shows, when known: `label`, `pid`, `tty`, `parent`, one `env NAME` line per `live.record_env` value, `conversation` (its id and title), `in use since` (when the run moved to its login), `no key` (the full reason of a refused key request) and `last key` (when a real key was last handed out). `--json` has the same fields as `agx live list --json`.

Pitfalls: an id that is not a run is exit 1, a malformed id exit 2, and a run started without `--live-login` is refused (exit 1):

<!-- runnable: exit=1 -->
```sh
$ agx live show --id 20261114-101500-k3x9
agx: no run 20261114-101500-k3x9
  agx live list
```

## agx live use

Choose the login of a live run. The agent picks it up at its next key refresh (about 10 seconds) or at once after a rejected key. Cost W.

| Flag | Meaning |
|---|---|
| `--id` | the run id |
| `--login` | the login to switch to; on a pool run, a member pins the run to it and the pool's name hands it back to the pool's rules |
| `--start-login` | go back to the login the run started with (on a pool run, the pool) |
| `--pool-only` | pin a pool run to this member with its caps enforced: it gets that member's key while the pool's rules allow it, and no key while they do not. The pool is `--login POOL` when given, the start login with `--start-login`, else the run's current pool |
| `--pool-only-short` | with `--pool-only`: a lower 5-hour cap for that member, for this run only (never above its own) |
| `--pool-only-week` | with `--pool-only`: a lower weekly cap for that member, for this run only (never above its own) |

On a [pool](../guides/login-pool.md) run, `--login` or `--start-login` without `--pool-only` also ends a `--pool-force` and a `--pool-only` the run was started with: `--login auto` hands the run back to the pool's rules, and `--login MEMBER` is the pin that ignores caps. `agx live use --id RUN --pool-only alice` keeps the run on alice while she is under her caps ([Pin one account but keep its limits](../guides/login-pool.md#pin-one-account-but-keep-its-limits)); `agx live show` then adds `only  alice (caps enforced)` and, while she is over a cap, `in use  none (alice over cap: …)` (`--json`: `only`, `no_key`). `agx whoami` inside the session shows the login in use and the next one.

Pinning a pool run with `--pool-only` (from [`examples/acct-login-pool`](../../examples/acct-login-pool/README.md); erin is over her 5-hour cap):

<!-- snippet: examples/acct-login-pool/session.txt#live-only -->
```sh
$ agx live use --id 20261119-095000-p7q2 --pool-only derek
run         20261119-095000-p7q2
agent       claude
profile     personal
folder      ~/projects/app
started on  auto
chosen      —
next        auto
only        derek (caps enforced)
in use      carol
liveness    unknown (process info unreadable: run.jsonc records no process id)

$ AGX_RUN_ID=20261119-095000-p7q2 AGX_RUN_AGENT=claude AGX_RUN_PROFILE=personal AGX_RUN_LOGIN=auto AGX_RUN_HOME=~/.claude agx whoami
run:      20261119-095000-p7q2
agent:    claude
profile:  personal
login:    auto (pool)
home:     ~/.claude
only:     derek (caps enforced)
in use:   carol
next:     derek

$ agx live use --id 20261119-095000-p7q2 --pool-only erin
run         20261119-095000-p7q2
agent       claude
profile     personal
folder      ~/projects/app
started on  auto
chosen      —
next        auto
only        erin (caps enforced)
in use      carol
liveness    unknown (process info unreadable: run.jsonc records no process id)

$ AGX_RUN_ID=20261119-095000-p7q2 agx credential --json --field data.refused
erin: 5h is 55% (cap 50%)

$ AGX_RUN_ID=20261119-095000-p7q2 AGX_RUN_AGENT=claude AGX_RUN_PROFILE=personal AGX_RUN_LOGIN=auto AGX_RUN_HOME=~/.claude agx whoami
run:      20261119-095000-p7q2
agent:    claude
profile:  personal
login:    auto (pool)
home:     ~/.claude
only:     erin (caps enforced)
in use:   none (erin over cap) since 10:00
no key:   erin over cap: 5h is 55% (cap 50%)

$ agx live show --id 20261119-095000-p7q2 --json --field data.no_key
erin over cap: 5h is 55% (cap 50%)

$ agx live use --id 20261119-095000-p7q2 --login auto
run         20261119-095000-p7q2
agent       claude
profile     personal
folder      ~/projects/app
started on  auto
chosen      auto
next        auto
in use      —
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

Try it:

<!-- snippet: examples/acct-live-login/session.txt#use -->
```sh
$ agx live use --id 20261114-101500-k3x9 --login alice
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      alice
next        alice
in use      —
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

<!-- snippet: examples/acct-live-login/session.txt#after -->
```sh
$ agx live show --id 20261114-101500-k3x9
run           20261114-101500-k3x9
agent         claude
profile       work
folder        ~/projects/app
started on    carol
chosen        alice
next          alice
in use        alice
in use since  2026-11-18 12:00:00
last key      2026-11-18 12:00:00
liveness      unknown (process info unreadable: run.jsonc records no process id)

$ agx live use --id 20261114-101500-k3x9 --start-login
run           20261114-101500-k3x9
agent         claude
profile       work
folder        ~/projects/app
started on    carol
chosen        —
next          carol
in use        alice
in use since  2026-11-18 12:00:00
last key      2026-11-18 12:00:00
liveness      unknown (process info unreadable: run.jsonc records no process id)
```

Pitfalls:

- Give exactly one of `--login` and `--start-login` (exit 2).
- The login must exist, serve the run's agent and be a token login (a key-helper login is refused).
- A run whose agent has ended is refused (exit 1): nothing would read the choice.
- The switch applies at the next timed refresh (agx sets ten seconds) or right after Claude gets a rejected key, and the first request after a timed refresh may still use the old key; `in use` shows when the helper ran. See [when a switch applies](../guides/live-login.md#when-a-switch-applies).
- Check the login first (`agx login check --id NAME --no-probe`): if it does not resolve, `agx credential` hands Claude no key (by default the placeholder below, so the session waits; with `when_none_usable: "keep"` it fails with one line, and Claude Code stops after three tries instead of falling back to another login). See [when the helper has no key](../guides/live-login.md#when-the-helper-has-no-key).

## The key helper Claude runs

The hidden command Claude runs as its `apiKeyHelper` in a live or pool session (`<agx> credential`, set at launch); you never type it. It finds its run by `AGX_RUN_ID`, resolves the chosen login (on a pool, picks a member), records it in the run's `live.jsonc` and prints its token alone on stdout. `--json` carries only the names: `run_id`, `login`, `pool`, and on a refusal `refused` and `refused_brief`. Cost X.

When it cannot hand out a key for its run (no pool member usable, a `--pool-only` member over its cap, a refused per-use API key, a login that does not resolve), the run's `when_none_usable` decides ([when no account is usable](../guides/pool-rules.md#when-no-account-is-usable)):

| `when_none_usable` | stdout | stderr | exit | what Claude does |
|---|---|---|---|---|
| `stop` (default) | `agx-no-account-usable` | `agx: warning: no key handed out: <reason>` | 0 | its next request fails authentication, so nothing more is spent; it retries about ten times over four to five minutes, asking for a key each time, and goes on by itself if one becomes usable; after that, type `continue` |
| `keep` | nothing | `agx: <reason>` | 1 | goes on with the key it already holds, past the caps |

`agx-no-account-usable` is a fixed public word, not a token: no vendor prefix, never stored. Either way `live.jsonc` records the refusal (`no_key`, `no_key_brief`, `no_key_since`, `when_none_usable`), so `agx live list`, `agx whoami` and `agx statusline` show `none (all over caps) since 10:52`. A failure before the run is known (`AGX_RUN_ID` unset, no such run) is always an error with exit 1 or 2. In a pinned session the reason names the cap (from [`examples/acct-login-pool`](../../examples/acct-login-pool/README.md), above):

```sh
$ AGX_RUN_ID=20261119-095000-p7q2 agx credential --json --field data.refused
erin: 5h is 55% (cap 50%)
```

## agx statusline

One short line naming the account the current live or pool session is on now, for the agent's status line. Cost R: it reads only the run's `live.jsonc` and the login-usage cache (no probe, no network), so the agent can run it on every refresh. Outside a live session (no `AGX_RUN_ID`, or a run without live login) it prints nothing and exits 0, so it can sit in any status line script.

```sh
agx statusline
agx statusline --json
```

| The run | It prints |
|---|---|
| on a pool, a member in use | `pool auto: alice · 5h 40% · week 61%` (usage from the cache, left out when unknown) |
| on a pool, last key request refused | `pool auto: none (all over caps) since 10:52` |
| on a pool, refused under `keep` | `pool auto: alice (no key since 10:52: all over caps; the agent keeps its key) · 5h 100% · week 72%` |
| a live run on one login | `login carol · 5h 12% · week 30%` |
| before the first key request | `pool auto: waiting for the first key` |

`--json` fields: `run_id`, `login`, `pool`, `in_use`, `in_use_text`, `short_percent`, `week_percent` (`null` when unknown) and `line`.

A live run's launch writes it as Claude Code's `statusLine` (`{"type": "command", "command": "<agx> statusline"}`) in the run's own settings, so every pool session shows its current account at the bottom and follows each switch. The dry run says so:

```text
settings  statusLine agx statusline (the account in use)    from default
```

It never replaces a status line you set: one in any of Claude's settings files for that home and folder (user, project, local, managed), in the profile's settings or a `--setting statusLine…` is kept, and the line reads `statusLine of ~/.claude/settings.json kept; agx statusline not added`. To show the account in your own status line, run `agx statusline` from your script and print its line next to yours (it prints nothing outside an agx session):

```sh
#!/bin/sh
# ~/.claude/statusline.sh: your own line, then agx's when in a live session
input=$(cat)
echo "$(printf '%s' "$input" | jq -r .model.display_name) $(agx statusline)"
```

`"live": { "status_line": false }` in config.jsonc turns it off for every run ([config](../config.md#live)).
