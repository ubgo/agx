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

It prints the pool's agents and members, one line per member with its `enabled` state, its 5-hour (`short`) and weekly (`week`) caps, its `week_relax_max` and its `capacity`, then one line per setting: `force`, the rule values (`order`, `week_relax_window`, `defaults.week_relax_max`, `max_sessions`, `sticky`, `sticky_leave_ratio`, `shared_borrow_window`, `short_unavailable_at_percent`), the group defaults (`own_caps.short`, `own_caps.week`, `defaults.short`, `defaults.week`), `usage_max_age`, `key_ttl`, `picker.command`, `picker.timeout`, `timezone` and `log`, each named by its key in the pool's config block. A member with an override shows it after its caps (`override short 90 until Fri 20 Nov 15:00 (active)`, or `(ended; ignored, agx pool prune removes it)`), and a `Schedules` section lists each schedule with its days, hours, zone, members, caps and `release_before`, and whether it applies now and until when ([schedules and overrides](../guides/pool-rules.md#schedules-and-overrides)). A value's source is `default` (agx's value for an omitted key), `pool` (a pool-wide key of the pool's block, `defaults.*` included), `group` (`own_caps` or `shared_caps`) or `member` (the member's `accounts` entry). Own members never take `defaults`: without `own_caps` they get 95/98. Defaults read as a config file would write them (`48h`, `60s`), so a value can be copied into `agx config set`.

<!-- snippet: examples/acct-login-pool/session.txt#pool-show -->
```sh
$ agx pool show --id auto
name    auto
agents  claude
own     alice, bob
shared  carol, derek, erin

Members (caps in percent):
- alice [own] enabled (default), short 95 (member), week 98 (member), week relax max 95 (default), capacity 1 (default)
- bob [own] enabled (default), short 95 (member), week 98 (member), week relax max 95 (default), capacity 1 (default)
- carol [shared] enabled (default), short 50 (pool), week 70 (pool), week relax max 95 (default), capacity 1 (default)
- derek [shared] enabled (default), short 50 (pool), week 70 (pool), week relax max 95 (default), capacity 1 (default)
- erin [shared] enabled (default), short 50 (pool), week 70 (pool), week relax max 95 (default), capacity 1 (default)

Schedules (caps that change on set days and hours):
—

Settings:
force                         off (default)
order                         urgency (default)
week_relax_window             24h (default)
defaults.week_relax_max       95 (default)
max_sessions                  4 (default)
sticky                        true (default)
sticky_leave_ratio            3 (default)
shared_borrow_window          48h (pool)
short_unavailable_at_percent  99 (pool)
own_caps.short                95 (default)
own_caps.week                 98 (default)
defaults.short                50 (pool)
defaults.week                 70 (pool)
usage_max_age                 60s (pool)
key_ttl                       60s (pool)
picker.command                built-in rules (default)
picker.timeout                10s (default)
timezone                      local (default)
log                           none (default)
```

`--json` carries `name`, `agents`, `own`, `shared`, `force`, `picker`, `members` (each `name`, `group`, `enabled`, `enabled_source`, `short`, `short_source`, `week`, `week_source`, `week_relax_max`, `week_relax_max_source`, `capacity`, `capacity_source`) and `settings` (each `key`, `value`, `source`).

## agx pool pick

Show which member a pool login picks now, and why, in the stand-alone selector's report line for line, under a first line naming the pool, its agent and its order (`Pool: auto (claude, order urgency)`): `Use: NAME (GROUP)` (or `(FORCED, ignoring rules)` with an indented `warning:` line per rule it ignores), the reason, any `warning:` lines (a probe that fell back to the cache, a picker that failed), `Source:` (`live agx result`, `cache (25s old)`, or both), `Probe model:` (the model the members were probed with and why, see [per-model weekly windows](../guides/pool-rules.md#per-model-weekly-windows)), `Rules:` (every rule value in effect with its source), a blank line, `Candidates (ready accounts in pick order):`, then one `- NAME [GROUP] …` line per member: `#N READY` in pick order, `WAIT` (usable, held back by the borrow window) or `skip:` with every reason, then the 5-hour and weekly use, each per-model window of the probe model (`fable week 80%`), the weekly reset (`reset passed` for stale data, `reset unknown` without one), the room left (the least over the windows), the caps with their source (`caps 50/70 (group)`), the urgency (`needs 1.2%/h`) and the other sessions on the member. It decides as a key request would: `--pool-force`, else the pool's `force`, else its picker command, else the built-in rules; with `--pool-only NAME` the report holds that member alone with its caps enforced (`Use: NAME`, or the "No configured account is currently usable." line with its skip reasons), and neither the pool's `force` nor its picker applies. It prints no token. Cost Q and W: a member whose cached usage is older than the pool's `usage_max_age` is probed once (a few tokens of that plan) and the login-usage cache is rewritten; with `--cached` nothing is probed. Guide: [Share a pool of Claude accounts](../guides/login-pool.md).

| Flag | Meaning |
|---|---|
| `--id` | the pool login |
| `--cached` | use the login-usage cache only, of any age; probe nothing |
| `--max-age` | reuse cached usage younger than this (a Go duration such as `30s`; `0` probes every member; default the pool's `usage_max_age`) |
| `--pool-force` | pick this member for this one run, over the pool's rules, its `force` and its picker |
| `--model` | the model the session uses: the members are probed with it, so its per-model weekly window applies (default: the pool's `probe_models`, else the default profile's model) |
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
Pool: auto (claude, order urgency)
Use: derek (shared)
the only member of this run (--pool-only); its caps and the pool's rules still apply
Source: cache (20s old)
Probe model: the agent's default (all-models windows only)
Rules: order urgency (default), week_relax_window 24h (default), defaults.week_relax_max 95 (default), max_sessions 4 (default), sticky true (default), sticky_leave_ratio 3 (default), shared_borrow_window 48h (pool), short_unavailable_at_percent 99 (pool), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (pool), defaults.week 70 (pool)

Candidates (ready accounts in pick order):
- derek [shared] #1 READY (5h 0%, week 15%, resets in 4 d 0 hr, room 55%, caps 50/70 (pool), needs 0.57%/h)

$ agx pool pick --id auto --pool-only erin
agx: erin: 5h is 55% (cap 50%)
Pool: auto (claude, order urgency)
No configured account is currently usable.
Source: cache (20s old)
Probe model: the agent's default (all-models windows only)
Rules: order urgency (default), week_relax_window 24h (default), defaults.week_relax_max 95 (default), max_sessions 4 (default), sticky true (default), sticky_leave_ratio 3 (default), shared_borrow_window 48h (pool), short_unavailable_at_percent 99 (pool), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (pool), defaults.week 70 (pool)

Candidates (ready accounts in pick order):
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min, room 48.9%, caps 50/91.9 (pool)) (weekly cap 70% rises to 95% over the last 24h before the reset: 91.9% now)
[exit 1]

$ agx pool pick --id auto --pool-only derek --pool-only-week 15
agx: derek: week is 15% (cap 15%)
Pool: auto (claude, order urgency)
No configured account is currently usable.
Source: cache (20s old)
Probe model: the agent's default (all-models windows only)
Rules: order urgency (default), week_relax_window 24h (default), defaults.week_relax_max 95 (default), max_sessions 4 (default), sticky true (default), sticky_leave_ratio 3 (default), shared_borrow_window 48h (pool), short_unavailable_at_percent 99 (pool), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (pool), defaults.week 70 (pool)

Candidates (ready accounts in pick order):
- derek [shared] skip: week is 15% (cap 15%) (5h 0%, week 15%, resets in 4 d 0 hr, room 0%, caps 50 (pool)/15 (--pool-only-week))
[exit 1]

$ agx run --login auto --pool-only derek --dry-run --quiet --json --field data.live.pool_only
derek
```

Try it (from [`examples/acct-login-pool`](../../examples/acct-login-pool/README.md)):

<!-- snippet: examples/acct-login-pool/session.txt#pick -->
```sh
$ agx pool pick --id auto
Pool: auto (claude, order urgency)
Use: carol (shared)
most room to lose: 44% left under its weekly cap with 38h to the reset (1.2%/h to use it all)
Source: cache (20s old)
Probe model: the agent's default (all-models windows only)
Rules: order urgency (default), week_relax_window 24h (default), defaults.week_relax_max 95 (default), max_sessions 4 (default), sticky true (default), sticky_leave_ratio 3 (default), shared_borrow_window 48h (pool), short_unavailable_at_percent 99 (pool), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (pool), defaults.week 70 (pool)

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr, room 44%, caps 50/70 (pool), needs 1.16%/h)
- alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr, room 69%, caps 95/98 (member), needs 0.48%/h)
- derek [shared] WAIT (5h 0%, week 15%, resets in 4 d 0 hr, room 55%, caps 50/70 (pool), needs 0.57%/h) (shared reset not within 48h while an own account is usable)
- bob [own] skip: status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 95%) (5h 100%, week 61%, resets in 3 d 0 hr, room 37%, caps 95/98 (member))
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min, room 48.9%, caps 50/91.9 (pool)) (weekly cap 70% rises to 95% over the last 24h before the reset: 91.9% now)
```

With `--time-format both`, a custom `--date-format` and `--tz`:

<!-- snippet: examples/acct-login-pool/session.txt#times -->
```sh
$ agx pool pick --id auto --time-format both --date-format "ddd HH:mm" --tz UTC
Pool: auto (claude, order urgency)
Use: carol (shared)
most room to lose: 44% left under its weekly cap with 38h to the reset (1.2%/h to use it all)
Source: cache (20s old)
Probe model: the agent's default (all-models windows only)
Rules: order urgency (default), week_relax_window 24h (default), defaults.week_relax_max 95 (default), max_sessions 4 (default), sticky true (default), sticky_leave_ratio 3 (default), shared_borrow_window 48h (pool), short_unavailable_at_percent 99 (pool), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (pool), defaults.week 70 (pool)

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr (Sat 00:00), room 44%, caps 50/70 (pool), needs 1.16%/h)
- alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr (Wed 10:00), room 69%, caps 95/98 (member), needs 0.48%/h)
- derek [shared] WAIT (5h 0%, week 15%, resets in 4 d 0 hr (Mon 10:00), room 55%, caps 50/70 (pool), needs 0.57%/h) (shared reset not within 48h while an own account is usable)
- bob [own] skip: status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 95%) (5h 100%, week 61%, resets in 3 d 0 hr (Sun 10:00), room 37%, caps 95/98 (member))
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min (13:00), room 48.9%, caps 50/91.9 (pool)) (weekly cap 70% rises to 95% over the last 24h before the reset: 91.9% now)

$ agx pool pick --id auto --duration-format minutes --json --field data.selected
{"name":"carol","group":"shared","forced":false,"picked_by":"rules","sessions":0,"reason":"most room to lose: 44% left under its weekly cap with 38h to the reset (1.2%/h to use it all)","warnings":[],"weekly_resets_at":"2026-11-21T00:00:00Z","room_percent":44}
```

`--json` carries the selector's `--json` fields in agx's envelope (snake_case): `selected` (`name`, `group`, `forced`, `forced_by`, `picked_by` `rules`/`force`/`picker`/`only`, `only` (true for a `--pool-only` pick), `sticky`, `sessions`, `reason`, `warnings`, `weekly_resets_at`, `room_percent`), `order`, `settings` (each `key`, `value`, `source`), `probe_models` and `probe_models_source`, `source`, `now`, `warnings`, `short_unavailable_at_percent`, `shared_borrow_window_hours`, and `candidates` (each with `state` `ready`, `waiting` or `blocked`, `rank` (null unless ready), `eligible`, `enabled`, `status`, `account`, `short`, `week`, `weekly_resets_at`, `hours_to_reset`, `room_percent`, `caps`, `caps_source`, `week_cap_configured`, `urgency_percent_per_hour`, `scoped` (each per-model window: `name`, `percent`, `cap`, `room`, `resets_at`, `rejected`, `stale`), `timed_caps` (only when a schedule or an override sets a cap now: per cap `short`, `week`, `week_relax_max` its `kind` `schedule` or `override`, the schedule's `name` and `until`; `caps_source` then says `schedule` or `override`), `expired_override` (the end of an override that has ended and is ignored), `sessions`, `reasons`, `notes`, `source`, `age_seconds`); `--field` reads any of them.

Pitfalls: `--pool-force` and the pool's `force` ignore every rule, including a member at its limit; each rule the forced member breaks is a `warning:` line. `--pool-only` is the opposite: it never goes over a cap, so a pinned member at its limit gives no pick at all. Do not loop it without `--cached`: every run older than `usage_max_age` probes.

## agx pool preset

Write a named set of rule values into a pool login's block in config.jsonc. Cost W (config only).

| Flag | Meaning |
|---|---|
| `--name` | the preset: `recommended` or `classic` |
| `--id` | the pool login |
| `--dry-run` | show each change; write nothing |

A preset writes `order`, `week_relax_window`, `defaults.week_relax_max`, `max_sessions`, `sticky`, `sticky_leave_ratio` and `shared_borrow_window` through the same comment-keeping edit as `agx login edit --pool-…`. It is a shortcut, not a mode: afterwards each is an ordinary key that `agx pool show` lists with source `pool`, and you can change any one by hand or with `agx config set`. Caps are not touched. `recommended` is the defaults (urgency order, weekly cap rising to 95 over the last 24 hours, at most 4 sessions per member, sticky sessions that leave for a member 3 times as urgent); `classic` is the rules before v0.4.16 (soonest weekly reset first, no relax, no session limit, not sticky). Values that are already set change nothing (`no change`). An unknown preset or a login that is not a pool is refused (exit 2). See [presets](../guides/pool-rules.md#presets).

<!-- snippet: examples/acct-login-pool/session.txt#preset -->
```sh
$ agx pool preset --name classic --id auto --dry-run
would change login auto (--dry-run)
  pool.defaults.week_relax_max  —  ->  95
  pool.order                    —  ->  deadline
  pool.max_sessions             —  ->  0
  pool.sticky                   —  ->  false
  pool.sticky_leave_ratio       —  ->  0
  pool.week_relax_window        —  ->  0s

$ agx pool preset --name recommended --id auto
changed login auto (pool, pool 2 own, 3 shared)
  pool.defaults.week_relax_max  —  ->  95
  pool.order                    —  ->  urgency
  pool.max_sessions             —  ->  4
  pool.sticky                   —  ->  true
  pool.sticky_leave_ratio       —  ->  3
  pool.week_relax_window        —  ->  24h

$ agx pool show --id auto --json --field data.settings
[{"key":"force","value":"off","source":"default"},{"key":"order","value":"urgency","source":"pool"},{"key":"week_relax_window","value":"24h","source":"pool"},{"key":"defaults.week_relax_max","value":"95","source":"pool"},{"key":"max_sessions","value":"4","source":"pool"},{"key":"sticky","value":"true","source":"pool"},{"key":"sticky_leave_ratio","value":"3","source":"pool"},{"key":"shared_borrow_window","value":"48h","source":"pool"},{"key":"short_unavailable_at_percent","value":"99","source":"pool"},{"key":"own_caps.short","value":"95","source":"default"},{"key":"own_caps.week","value":"98","source":"default"},{"key":"defaults.short","value":"50","source":"pool"},{"key":"defaults.week","value":"70","source":"pool"},{"key":"usage_max_age","value":"60s","source":"pool"},{"key":"key_ttl","value":"60s","source":"pool"},{"key":"picker.command","value":"built-in rules","source":"default"},{"key":"picker.timeout","value":"10s","source":"default"},{"key":"timezone","value":"local","source":"default"},{"key":"log","value":"none","source":"default"}]
```

## agx pool prune

Remove a pool's member overrides that have ended. Cost W (config only).

| Flag | Meaning |
|---|---|
| `--id` | the pool login |
| `--dry-run` | show each change; write nothing |

An override (`accounts.NAME.override`, written by `agx login edit --pool-cap-short NAME=N --until TIME`) is ignored by every pick once its end has passed, and `agx pool show` lists it as ended; prune removes it from config.jsonc, keeping comments. Nothing ended writes nothing. To end an override before its time use `agx login edit --pool-clear-override NAME`. A login that is not a pool is refused (exit 2). See [schedules and overrides](../guides/pool-rules.md#schedules-and-overrides).

<!-- snippet: examples/acct-pool-schedules/session.txt#prune -->
```sh
$ agx pool prune --id auto --dry-run
would change login auto (--dry-run)
  pool.accounts.erin.override  short 90 until 2026-11-20T18:00:00+03:00  ->  —

$ agx pool prune --id auto
changed login auto (pool, pool 1 own, 3 shared)
  pool.accounts.erin.override  short 90 until 2026-11-20T18:00:00+03:00  ->  —
```
