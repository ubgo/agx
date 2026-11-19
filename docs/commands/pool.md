# Pools

Read login pools: list them, show one pool's settings, and see which member it picks now. A pool is a login of kind `pool`, so it is made and changed with the login commands: create it with [`agx login add --kind pool`](login.md#agx-login-add), change it with [`agx login edit --pool-…`](login.md#agx-login-edit), and run on it with `agx run --login POOL` (a live run whose every key request picks a member). [`agx login token --id POOL`](login.md#agx-login-token) prints the picked member's token. [Share a pool of Claude accounts](../guides/login-pool.md) is the guide. The examples below come from [`examples/acct-login-pool`](../../examples/acct-login-pool/README.md), which runs in the test suite.

`agx pool list` and `agx pool show` read config only: no probe, no cache. Given a login that is not a pool, `agx pool show` and `agx pool pick` refuse it (exit 2) with `pool show is for pool logins; login NAME is KIND` (or `pool pick …`), the agents that can have a pool today, and the hint `agx pool list`; an unknown login exits 1 (`not-found`).

`agx pool pick` was `agx login pick` in agx 0.4.7 to 0.4.10. `agx login pick` is an unknown command now (exit 2), whose hint names `agx pool pick`.

## agx pool list

Every login of kind `pool`, in the file's order: the agents it serves, its own and shared members, the member it always picks (`FORCE`) and its picker command (`PICKER`); `—` is off. Cost R. With no pool configured it says so and how to add one.

<!-- snippet: examples/acct-login-pool/session.txt#pool-list -->
```sh
$ agx pool list
NAME   AGENTS  OWN        SHARED            FORCE  PICKER
auto   claude  alice,bob  carol,derek,erin  —      —
picky  claude  alice,bob  carol             —      sh $HOME/picker.sh
```

`--json` carries `name`, `agents`, `own`, `shared`, `force` and `picker` (`""` when off) for each pool.

## agx pool show

One pool's full settings, each value with where it comes from. Cost R.

| Flag | Meaning |
|---|---|
| `--id` | the pool login |

It prints the pool's agents and members, one line per member with its `enabled` state and its 5-hour (`short`) and weekly (`week`) caps, then one line per setting: `force`, `defaults.short`, `defaults.week`, `shared_borrow_window`, `short_unavailable_at_percent`, `usage_max_age`, `key_ttl`, `picker.command`, `picker.timeout` and `log`, each named by its key in the pool's config block. A value's source is `config` (the pool's block sets it), `defaults` (a member cap taken from the pool's `defaults` key, as the member has none of its own) or `default` (agx's value for an omitted key). Defaults read as a config file would write them (`48h`, `60s`), so a value can be copied into `agx config set`.

<!-- snippet: examples/acct-login-pool/session.txt#pool-show -->
```sh
$ agx pool show --id auto
name    auto
agents  claude
own     alice, bob
shared  carol, derek, erin

Members (caps in percent):
- alice [own] enabled (default), short 95 (config), week 98 (config)
- bob [own] enabled (default), short 95 (config), week 98 (config)
- carol [shared] enabled (default), short 50 (defaults), week 70 (defaults)
- derek [shared] enabled (default), short 50 (defaults), week 70 (defaults)
- erin [shared] enabled (default), short 50 (defaults), week 70 (defaults)

Settings:
force                         off (default)
defaults.short                50 (config)
defaults.week                 70 (config)
shared_borrow_window          48h (config)
short_unavailable_at_percent  99 (config)
usage_max_age                 60s (config)
key_ttl                       60s (config)
picker.command                built-in rules (default)
picker.timeout                10s (default)
log                           none (default)
```

`--json` carries `name`, `agents`, `own`, `shared`, `force`, `picker`, `members` (each `name`, `group`, `enabled`, `enabled_source`, `short`, `short_source`, `week`, `week_source`) and `settings` (each `key`, `value`, `source`).

## agx pool pick

Show which member a pool login picks now, and why, in the stand-alone selector's report line for line, under a first line naming the pool and its agent (`Pool: auto (claude)`): `Use: NAME (GROUP)` (or `(FORCED, ignoring rules)` with an indented `warning:` line per rule it ignores), the reason, any `warning:` lines (a probe that fell back to the cache, a picker that failed), `Source:` (`live agx result`, `cache (25s old)`, or both), a blank line, `Candidates (ready accounts in pick order):`, then one `- NAME [GROUP] …` line per member: `#N READY` in pick order, `WAIT` (usable, held back by the borrow window) or `skip:` with every reason, then the 5-hour and weekly use, the weekly reset (`reset passed` for stale data, `reset unknown` without one) and the room under the weekly cap. It decides as a key request would: `--pool-force`, else the pool's `force`, else its picker command, else the built-in rules; with `--pool-only NAME` the report holds that member alone with its caps enforced (`Use: NAME`, or the "No configured account is currently usable." line with its skip reasons), and neither the pool's `force` nor its picker applies. It prints no token. Cost Q and W: a member whose cached usage is older than the pool's `usage_max_age` is probed once (a few tokens of that plan) and the login-usage cache is rewritten; with `--cached` nothing is probed. Guide: [Share a pool of Claude accounts](../guides/login-pool.md).

| Flag | Meaning |
|---|---|
| `--id` | the pool login |
| `--cached` | use the login-usage cache only, of any age; probe nothing |
| `--max-age` | reuse cached usage younger than this (a Go duration such as `30s`; `0` probes every member; default the pool's `usage_max_age`) |
| `--pool-force` | pick this member for this one run, over the pool's rules, its `force` and its picker |
| `--pool-only` | pick only this member, with its caps and the pool's other rules enforced; none when it is blocked. Not with `--pool-force` (exit 2) |
| `--pool-only-short` | with `--pool-only`: a lower 5-hour cap for that member, for this pick (never above its own) |
| `--pool-only-week` | with `--pool-only`: a lower weekly cap for that member, for this pick (never above its own) |
| `--time-format` | how resets read, as in `agx usage`: `relative` (default), `day`, `absolute`, `both`; default config `usage.time_format` |
| `--duration-format` | how a reset a day or more away reads: `truncate` (default), `round`, `minutes`; default config `usage.duration_format` |
| `--tz` | the IANA zone of days and clock times (default this machine's) |

The global `--date-format` (and config `usage.date_format`, `date_format`) writes the clock and date parts, as for `agx usage`.

On a login that is not a pool it says `pool pick is for pool logins`, which agents can have one today (`Pools serve claude today`, from the agents' capabilities) and, for a login of another agent, why that agent cannot (`codex: …`). Exit 0 when a member is picked (or forced); 1 with error code `auto-pick` when no member is usable, the report still printed (the selector exited 2 there; in agx 2 is a usage error); 1 (`not-found`) when `--pool-force` names a login without a reading or not at all; 2 for a login that is not a pool, a bad flag value, `--pool-only` with `--pool-force`, a `--pool-only` name that is not a member, or a `--pool-only-short`/`--pool-only-week` above the member's cap. A blocked `--pool-only` member exits 1 (`auto-pick`) with one line naming it and every reason (`agx: erin: 5h is 55% (cap 50%)`), the report still printed:

<!-- snippet: examples/acct-login-pool/session.txt#only -->
```sh
$ agx pool pick --id auto --pool-only derek
Pool: auto (claude)
Use: derek (shared)
the only member of this run (--pool-only); its caps and the pool's rules still apply
Source: cache (20s old)

Candidates (ready accounts in pick order):
- derek [shared] #1 READY (5h 0%, week 15%, resets in 4 d 0 hr, room 55%)

$ agx pool pick --id auto --pool-only erin
agx: erin: 5h is 55% (cap 50%)
Pool: auto (claude)
No configured account is currently usable.
Source: cache (20s old)

Candidates (ready accounts in pick order):
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min, room 27%)
[exit 1]

$ agx pool pick --id auto --pool-only derek --pool-only-week 15
agx: derek: week is 15% (cap 15%)
Pool: auto (claude)
No configured account is currently usable.
Source: cache (20s old)

Candidates (ready accounts in pick order):
- derek [shared] skip: week is 15% (cap 15%) (5h 0%, week 15%, resets in 4 d 0 hr, room 0%)
[exit 1]

$ agx run --login auto --pool-only derek --dry-run --quiet --json --field data.live.pool_only
derek
```

Try it (from [`examples/acct-login-pool`](../../examples/acct-login-pool/README.md)):

<!-- snippet: examples/acct-login-pool/session.txt#pick -->
```sh
$ agx pool pick --id auto
Pool: auto (claude)
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
$ agx pool pick --id auto --time-format both --date-format "ddd HH:mm" --tz UTC
Pool: auto (claude)
Use: carol (shared)
shared account resets within 48h, before any usable own account; using its room before it is lost
Source: cache (20s old)

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr (Sat 00:00), room 44%)
- alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr (Wed 10:00), room 69%)
- derek [shared] WAIT (5h 0%, week 15%, resets in 4 d 0 hr (Mon 10:00), room 55%) (shared reset not within 48h while an own account is usable)
- bob [own] skip: status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 95%) (5h 100%, week 61%, resets in 3 d 0 hr (Sun 10:00), room 37%)
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min (13:00), room 27%)

$ agx pool pick --id auto --duration-format minutes --json --field data.selected
{"name":"carol","group":"shared","forced":false,"picked_by":"rules","reason":"shared account resets within 48h, before any usable own account; using its room before it is lost","warnings":[],"weekly_resets_at":"2026-11-21T00:00:00Z","room_percent":44}
```

`--json` carries the selector's `--json` fields in agx's envelope (snake_case): `selected` (`name`, `group`, `forced`, `forced_by`, `picked_by` `rules`/`force`/`picker`/`only`, `only` (true for a `--pool-only` pick), `reason`, `warnings`, `weekly_resets_at`, `room_percent`), `source`, `now`, `warnings`, `short_unavailable_at_percent`, `shared_borrow_window_hours`, and `candidates` (each with `state` `ready`, `waiting` or `blocked`, `rank` (null unless ready), `eligible`, `enabled`, `status`, `account`, `short`, `week`, `weekly_resets_at`, `hours_to_reset`, `room_percent`, `caps`, `reasons`, `notes`, `source`, `age_seconds`); `--field` reads any of them.

Pitfalls: `--pool-force` and the pool's `force` ignore every rule, including a member at its limit; each rule the forced member breaks is a `warning:` line. `--pool-only` is the opposite: it never goes over a cap, so a pinned member at its limit gives no pick at all. Do not loop it without `--cached`: every run older than `usage_max_age` probes.
