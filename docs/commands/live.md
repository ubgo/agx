# Live login

Switch the login of a running Claude session. Start the session with `--live-login` (or a profile with `"live_login": true`); [Live login](../guides/live-login.md) is the guide: how it works, the whole flow and its limits. The examples below come from [`examples/acct-live-login`](../../examples/acct-live-login/README.md), where a seeded run folder stands in for a session running in another terminal.

A raw `--settings` of your own is merged with the settings live login adds, so one `--settings` reaches Claude; a raw value that sets `apiKeyHelper` itself is refused at launch: `agx: raw --settings apiKeyHelper and --live-login both set the credential; pick one` (exit 125). See [the live-login pitfalls](../guides/live-login.md#pitfalls).

## agx live list

Running sessions started with live login. Runs whose agent has ended are left out (`agx clean` removes them); a run whose liveness cannot be read is listed as `unknown`. Cost R.

| Flag | Meaning |
|---|---|
| `--agent` | only this agent |

<!-- runnable -->
```sh
$ agx live list
RUN  AGENT  PROFILE  STARTED ON  NEXT  IN USE  LIVENESS
```

Each row is one run started with `--live-login`: the login it started on, the login it gets at its next refresh, the login in use, and whether its process is still alive:

<!-- snippet: examples/acct-live-login/session.txt#list -->
```sh
$ agx live list --agent claude
RUN                   AGENT   PROFILE  STARTED ON  NEXT   IN USE  LIVENESS
20261114-101500-k3x9  claude  work     carol       carol  —       unknown

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

$ AGX_RUN_ID=20261119-095000-p7q2 agx credential --json --field data.login
agx: erin: 5h is 55% (cap 50%)
[exit 1]

$ AGX_RUN_ID=20261119-095000-p7q2 AGX_RUN_AGENT=claude AGX_RUN_PROFILE=personal AGX_RUN_LOGIN=auto AGX_RUN_HOME=~/.claude agx whoami
run:      20261119-095000-p7q2
agent:    claude
profile:  personal
login:    auto (pool)
home:     ~/.claude
only:     erin (caps enforced)
in use:   none (erin over cap: 5h is 55% (cap 50%))

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
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      alice
next        alice
in use      alice
liveness    unknown (process info unreadable: run.jsonc records no process id)

$ agx live use --id 20261114-101500-k3x9 --start-login
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      —
next        carol
in use      alice
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

Pitfalls:

- Give exactly one of `--login` and `--start-login` (exit 2).
- The login must exist, serve the run's agent and be a token login (a key-helper login is refused).
- A run whose agent has ended is refused (exit 1): nothing would read the choice.
- The switch applies at the next timed refresh (agx sets ten seconds) or right after Claude gets a rejected key, and the first request after a timed refresh may still use the old key; `in use` shows when the helper ran. See [when a switch applies](../guides/live-login.md#when-a-switch-applies).
- Check the login first (`agx login check --id NAME --no-probe`): if it does not resolve, `agx credential` fails with one line, and Claude Code stops after three tries instead of falling back to another login ([a helper that fails stops the session](../guides/live-login.md#a-helper-that-fails-stops-the-session)).
