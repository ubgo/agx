# How the pool chooses

A [login pool](login-pool.md) hands each Claude session the token of one member each time the session asks for a key. This page is the whole decision: the order of the rules, every value with its default, the presets, the per-model weekly windows, schedules and overrides that change caps for a while, and what to change for your team. There is no hidden mode: every rule is a config value, and `agx pool show --id POOL` and `agx pool pick --id POOL` print each value in effect and where it comes from (`default`, `pool`, `group` or `member`).

The defaults were chosen with the [pool simulator](pool-simulator.md) to do well for anyone: a solo user with two accounts, a small team, a large team pool, any plan sizes, with or without teammates using their own accounts. They are not tuned to one setup.

## The rules, in order

1. **Force.** `--pool-force` (one run, one `pool pick` or `login token` call), else the pool's `force`, wins outright; every rule below is ignored and each one the forced member breaks is a warning. A `--pool-only` member instead makes the pool that one member with every rule below still applied, and no force.
2. **Usable.** A member is skipped when any of these holds, and every reason is listed: disabled; no usage reading; a per-use API key in an `oauth-token` member without `allow_api_key`; a status other than `ok`; usage that is not a number; 5-hour use at or over `short_unavailable_at_percent` (99); 5-hour use at or over its 5-hour cap; weekly use at or over its weekly cap; a per-model weekly window of the session's model at or over the weekly cap, or refused by the vendor until its reset ([per-model windows](#per-model-weekly-windows)). Caps are exclusive (under the cap is usable) and count the account's total use.
3. **Tiers.** While any own member is usable, the pool is the usable own members plus the usable shared members whose weekly reset is within `shared_borrow_window` (48h); the other usable shared members wait (`WAIT`). With no own member usable, every usable shared member is in the pool.
4. **Weekly relax.** Over the last `week_relax_window` (24h) before a member's weekly reset, its weekly cap rises in a straight line to its `week_relax_max` (95): a teammate's account at cap 70 has 82.5 twelve hours before its reset and 95 at it. That room would be lost at the reset anyway. The 5-hour cap never rises (that window outlives the weekly reset and is the teammate's working time). A member whose cap is already higher (your own at 98) keeps it.
5. **Order.** `order: "urgency"` (the default) ranks the members in two groups.
   - First the **deadline group**: every member whose weekly reset is within `deadline_window` (24h) and that has at least `deadline_min_room` (2%) of room before it, soonest reset first. Using the room that expires first loses nothing; ranking these by urgency alone could put a member that resets later with more room first, and the sooner one's room would be lost at its reset.
   - Then everyone else, the most room to lose per hour first: the room left before the reset divided by the hours to it, times its `capacity`.
   - The room counts up to the weekly cap the member reaches at its reset once that cap is rising (inside `week_relax_window`), not only to the cap it has now. A teammate's account at week 87% seven hours before its reset has a cap of 87.7% now: 0.7% to use now, but 8% before the reset, about 1.1% an hour. Whether a member is usable still goes by its cap now. Before the relax window opens none of the rise can be used yet, so it is not counted.
   - `agx pool pick` shows each member's room now and, when it differs, the room by the reset (`room 0.7% (8% by the reset)`), the rate needed to use it all (`needs 1.14%/h`, never weighed by `capacity`, never `0` while there is room) and `resets within deadline_window` for the deadline group.

   `order: "deadline"` puts the soonest weekly reset first, with no minimum room. An unknown or passed reset sorts last (a reading whose weekly reset has passed is old as a whole, so it still blocks at its caps until the next probe); ties keep config order, own before shared.
6. **A pick per session.** Each session (each `--login POOL` run) gets its own pick. It stays on its member while that member is still in the pool (`sticky`), unless the member it would go to (the first in order that can take another session) is at least `sticky_leave_ratio` (3) times as urgent; a member takes at most `max_sessions` (4) sessions before the next one is used, then the waiting members, then the least loaded. A session counts while its agent still runs and it asked for a key within two `key_ttl`s: a run that ended a moment ago, or one that sits idle, holds no place. So `agx live list` (every run that still runs, busy or idle) can show more runs on a member than `max_sessions`, and more than `agx pool pick` counts: idle runs keep their member (sticky) when they wake, and when every member is at `max_sessions` the least loaded takes the next session. With `max_sessions: 0` and `sticky: false` every session gets the first member.

An empty pool picks nothing: the key request fails with each member's reasons, and `agx pool pick` exits 1 (error code `auto-pick`).

## Every value and its default

Each is a key of `logins.<pool>.pool` in config.jsonc, set with a flag of `agx login add` / `agx login edit` or with `agx config set` (one value), and listed in the [config reference](../config.md#login-pools).

| Key | Default | Flag | Meaning |
|---|---|---|---|
| `order` | `urgency` | `--pool-order` | `urgency` or `deadline` (rule 5) |
| `deadline_window` | `24h` | `--pool-deadline-window` | under `urgency`, members whose weekly reset is within this go first, soonest reset first; `0s` = never (urgency alone) |
| `deadline_min_room` | 2 | `--pool-deadline-min-room` | the least room (percent, up to the cap reached at the reset) a member inside `deadline_window` needs to go first |
| `week_relax_window` | `24h` | `--pool-week-relax-window` | how long before the weekly reset the weekly cap starts rising; `0s` = never |
| `defaults.week_relax_max` | 95 | `--pool-default-week-relax` | the weekly cap reached at the reset |
| `own_caps.week_relax_max`, `shared_caps.week_relax_max` | (`defaults`) | `--pool-group-cap-week-relax own=N`, `--pool-group-cap-week-relax shared=N` | the weekly cap a group's members reach at the reset |
| `accounts.NAME.week_relax_max` | (group) | `--pool-cap-week-relax NAME=N` | one member's weekly cap at the reset; its weekly cap or lower turns the relax off for it |
| `max_sessions` | 4 | `--pool-max-sessions` | sessions one member takes before the next is used; `0` = no limit |
| `sticky` | `true` | `--pool-sticky` | a session stays on its member while it is still in the pool (keeps its prompt cache) |
| `sticky_leave_ratio` | 3 | `--pool-sticky-leave-ratio` | a sticky session leaves for a member this many times as urgent; `0` = never |
| `shared_borrow_window` | `48h` | `--pool-borrow-window` | a shared member goes before a usable own one only when its week resets within this; `0s` = never |
| `short_unavailable_at_percent` | 99 | `--pool-short-cutoff` | the 5-hour safety cutoff for every member |
| `own_caps.short`, `own_caps.week` | 95, 98 | `--pool-group-cap-short own=N`, `--pool-group-cap-week own=N` | caps of your own members without an entry of their own |
| `defaults.short`, `defaults.week` | 50, 70 | `--pool-default-short`, `--pool-default-week` | caps of shared members without a `shared_caps` or member entry |
| `shared_caps.short`, `shared_caps.week` | (`defaults`) | `--pool-group-cap-short shared=N`, `--pool-group-cap-week shared=N` | caps of every shared member at once |
| `accounts.NAME.short`, `.week` | (group) | `--pool-cap-short NAME=N`, `--pool-cap-week NAME=N` | one member's caps |
| `accounts.NAME.enabled` | `true` | `--pool-disable NAME`, `--pool-enable NAME` | `false` leaves the member out of every pick |
| `accounts.NAME.capacity` | 1 | `--pool-capacity NAME=N` | the member's plan size relative to the others (Pro 1, Max 5x 5, Max 20x 20); its room is weighed by it |
| `probe_models` | (none) | `--pool-probe-model` | the models to probe with when a session's own model is unknown ([per-model windows](#per-model-weekly-windows)) |
| `usage_max_age` | `60s` | `--pool-usage-max-age` | how old the shared usage cache may be before members are probed again |
| `key_ttl` | `60s` | `--pool-key-ttl` | how long Claude keeps a key before asking again |
| `timezone` | local time | `--pool-timezone` | the zone of schedules and `--until` times ([schedules](#schedules-and-overrides)) |
| `schedules` | (none) | `--pool-schedule` (change the keys given, or create), `--pool-schedule-replace`, `--pool-schedule-remove` | caps that change on set days and hours, and `week_before_reset`, the weekly cap of members whose week resets before the schedule ends ([schedules](#schedules-and-overrides), [change one value](#change-one-value-of-a-schedule)) |
| `force` | `null` | `--pool-force NAME` (`''` clears it) | always pick this member, ignoring every rule and cap |
| `picker.command`, `picker.timeout` | (none), `10s` | `--pool-picker` (`''` removes the picker) | a command of yours that picks the member instead of the built-in rules ([your own picker](login-pool.md#your-own-picker)) |
| `log` | (none) | `agx config set` | a file that gets one line per key request (the pick, its source, the member's usage; never a token) |
| `accounts.NAME.override` | (none) | `--pool-cap-… NAME=N --until TIME`, `--pool-clear-override` | one member's caps until a time ([overrides](#use-one-persons-account-fully-for-a-while)) |

A member's cap comes from an override or a schedule that applies now ([which cap wins](#which-cap-wins)), else its own entry (`member`), else its group's block (`group`), else for a shared member the pool's `defaults` (`pool`), else agx's default (`default`). Own members never take `defaults`: without `own_caps` they get 95/98, so your own quota is used almost fully.

The [example](../../examples/acct-pool-rules/README.md) sets every rule value with its flag in one command, then `agx pool show` lists each with its source:

<!-- snippet: examples/acct-pool-rules/session.txt#show -->
```sh
$ agx pool show --id auto
name    auto
agents  claude
own     alice
shared  carol, derek, erin

Members (caps in percent):
- alice [own] enabled (default), short 95 (default), week 98 (default), week relax max 95 (default), capacity 20 (member)
- carol [shared] enabled (default), short 50 (default), week 70 (default), week relax max 90 (group), capacity 5 (member)
- derek [shared] enabled (default), short 50 (default), week 70 (default), week relax max 90 (group), capacity 1 (default)
- erin [shared] enabled (default), short 50 (default), week 70 (default), week relax max 70 (member), capacity 1 (default)

Schedules (caps that change on set days and hours):
—

Settings:
force                         off (default)
order                         deadline (pool)
deadline_window               12h (pool)
deadline_min_room             3 (pool)
week_relax_window             36h (pool)
defaults.week_relax_max       95 (default)
max_sessions                  2 (pool)
sticky                        false (pool)
sticky_leave_ratio            4 (pool)
shared_borrow_window          48h (default)
short_unavailable_at_percent  99 (default)
own_caps.short                95 (default)
own_caps.week                 98 (default)
defaults.short                50 (default)
defaults.week                 70 (default)
probe_models                  none (default)
usage_max_age                 60s (default)
key_ttl                       60s (default)
picker.command                built-in rules (default)
picker.timeout                10s (default)
timezone                      local (default)
log                           none (default)
```

## Presets

A preset is a named set of the rule values above, written into the config by one command. It is a shortcut, never a mode: afterwards each value is an ordinary key that `agx pool show` lists and you can change by hand. Caps are not touched (they are your team's agreement).

| Preset | `order` | `deadline_window` | `deadline_min_room` | `week_relax_window` | `week_relax_max` | `max_sessions` | `sticky` | `sticky_leave_ratio` | `shared_borrow_window` |
|---|---|---|---|---|---|---|---|---|---|
| `recommended` (the defaults) | `urgency` | `24h` | 2 | `24h` | 95 | 4 | `true` | 3 | `48h` |
| `classic` (the rules before v0.4.16) | `deadline` | `0s` | 2 | `0s` | 95 | 0 | `false` | 0 | `48h` |

```sh
agx pool preset --name classic --id auto --dry-run   # the keys it would write
agx pool preset --name classic --id auto
agx pool preset --name recommended --id auto         # back to the defaults, written out
```

<!-- snippet: examples/acct-login-pool/session.txt#preset -->
```sh
$ agx pool preset --name classic --id auto --dry-run
would change login auto (--dry-run)
  pool.defaults.week_relax_max  —  ->  95
  pool.order                    —  ->  deadline
  pool.max_sessions             —  ->  0
  pool.sticky                   —  ->  false
  pool.sticky_leave_ratio       —  ->  0
  pool.deadline_min_room        —  ->  2
  pool.week_relax_window        —  ->  0s
  pool.deadline_window          —  ->  0s

$ agx pool preset --name recommended --id auto
changed login auto (pool, pool 2 own, 3 shared)
  pool.defaults.week_relax_max  —  ->  95
  pool.order                    —  ->  urgency
  pool.max_sessions             —  ->  4
  pool.sticky                   —  ->  true
  pool.sticky_leave_ratio       —  ->  3
  pool.deadline_min_room        —  ->  2
  pool.week_relax_window        —  ->  24h
  pool.deadline_window          —  ->  24h

$ agx pool show --id auto --json --field data.settings
[{"key":"force","value":"off","source":"default"},{"key":"order","value":"urgency","source":"pool"},{"key":"deadline_window","value":"24h","source":"pool"},{"key":"deadline_min_room","value":"2","source":"pool"},{"key":"week_relax_window","value":"24h","source":"pool"},{"key":"defaults.week_relax_max","value":"95","source":"pool"},{"key":"max_sessions","value":"4","source":"pool"},{"key":"sticky","value":"true","source":"pool"},{"key":"sticky_leave_ratio","value":"3","source":"pool"},{"key":"shared_borrow_window","value":"48h","source":"pool"},{"key":"short_unavailable_at_percent","value":"99","source":"pool"},{"key":"own_caps.short","value":"95","source":"default"},{"key":"own_caps.week","value":"98","source":"default"},{"key":"defaults.short","value":"50","source":"pool"},{"key":"defaults.week","value":"70","source":"pool"},{"key":"probe_models","value":"none","source":"default"},{"key":"usage_max_age","value":"60s","source":"pool"},{"key":"key_ttl","value":"60s","source":"pool"},{"key":"picker.command","value":"built-in rules","source":"default"},{"key":"picker.timeout","value":"10s","source":"default"},{"key":"timezone","value":"local","source":"default"},{"key":"log","value":"none","source":"default"}]
```

## Per-model weekly windows

A plan can limit one model on its own. Claude reports this as an extra window in the probe's rate-limit headers, and only when the probe asks with that model: on a Max plan, a probe with Fable 5.2 answers `anthropic-ratelimit-unified-7d_oi-*` (Fable's own week) besides the 5-hour and all-models weekly windows, while a probe with Opus 5.5 answers only those two. agx reads every window the headers carry, known or not, with its status and whether the vendor names it as the window that limits now.

So the pool probes with the model the session uses. Which model, first set wins:

1. The run's model: the `--model` given to `agx run`, else its profile's `model`, else the model Claude's own settings choose for that home and folder (what `/model` saves). agx records it in the run's `live.jsonc` at launch; `agx pool pick --model NAME` stands in for it outside a run.
2. The pool's `probe_models`: every listed model is probed and its windows apply.
3. The model of the pool agent's default profile, else that home's own settings model.
4. None: the agent's default probe model (the cheapest), which shows only the all-models windows.

A user who runs Fable (by flag, profile or `/model`) therefore sees Fable's window without setting anything. A model the API refuses (a typo, a retired id) falls back to the default probe with a warning; it never blocks the pool.

Each per-model window applies only to the sessions of its model, as an extra weekly cap: the member's weekly cap and relax rules, measured to that window's own reset. The member is skipped for that model when the window is at or over the cap (`fable week is 99% (cap 98%)`) or the vendor refuses it until its reset (`fable week is refused by the vendor until its reset`), and its room is the least room over its windows. An Opus session still picks the same member. A full per-model window never marks the member `limit-reached` for other models.

`agx pool pick` names the probe model and shows each window. In the [example](../../examples/acct-pool-rules/README.md) derek's Fable week is 96%: a Fable session skips him, the pool's `probe_models` applies to a pick that names no model, and `--model opus` still picks him:

<!-- snippet: examples/acct-pool-rules/session.txt#model -->
```sh
#: derek's Fable week (7d_oi) is at 96%: a Fable session skips him, any other model still picks him.
$ agx pool pick --id auto --cached --model fable
Pool: auto (claude, order deadline)
Use: alice (own)
soonest weekly reset among usable accounts
Source: cache (20s old)
Probe model: fable (--model)
Rules: order deadline (pool), deadline_window 12h (pool), deadline_min_room 3 (pool), week_relax_window 36h (pool), defaults.week_relax_max 95 (default), max_sessions 2 (pool), sticky false (pool), sticky_leave_ratio 4 (pool), shared_borrow_window 48h (default), short_unavailable_at_percent 99 (default), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (default), defaults.week 70 (default)

Candidates (ready accounts in pick order):
- alice [own] #1 READY (5h 30%, week 60%, fable week 40%, resets in 4 d 0 hr, room 38%, caps 95/98 (default), needs 0.4%/h)
- carol [shared] WAIT (5h 10%, week 40%, resets in 3 d 0 hr, room 30%, caps 50/70 (default), needs 0.42%/h) (shared reset not within 48h while an own account is usable)
- erin [shared] WAIT (5h 5%, week 50%, resets in 5 d 22 hr, room 20%, caps 50/70 (default), needs 0.14%/h) (shared reset not within 48h while an own account is usable)
- derek [shared] skip: fable week is 96% (cap 73.3%) (5h 20%, week 66%, fable week 96%, resets in 1 d 6 hr, room 0%, caps 50/73.3 (default)) (weekly cap 70% rises to 90% over the last 36h before the reset: 73.3% now)

$ agx pool pick --id auto --cached --json --field data.candidates.3.scoped --model fable
[{"name":"fable week","percent":96,"cap":73.33333333333333,"room":0,"resets_at":"2026-11-19T16:00:00Z"}]

#: probe_models applies when the session's own model is unknown (a plain pick); --model still wins over it.
$ agx login edit --id auto --pool-probe-model claude-fable-5-2
changed login auto (pool, pool 1 own, 3 shared)
  pool.probe_models  —  ->  claude-fable-5-2

$ agx pool pick --id auto --cached
Pool: auto (claude, order deadline)
Use: alice (own)
soonest weekly reset among usable accounts
Source: cache (20s old)
Probe model: claude-fable-5-2 (pool probe_models)
Rules: order deadline (pool), deadline_window 12h (pool), deadline_min_room 3 (pool), week_relax_window 36h (pool), defaults.week_relax_max 95 (default), max_sessions 2 (pool), sticky false (pool), sticky_leave_ratio 4 (pool), shared_borrow_window 48h (default), short_unavailable_at_percent 99 (default), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (default), defaults.week 70 (default)

Candidates (ready accounts in pick order):
- alice [own] #1 READY (5h 30%, week 60%, fable week 40%, resets in 4 d 0 hr, room 38%, caps 95/98 (default), needs 0.4%/h)
- carol [shared] WAIT (5h 10%, week 40%, resets in 3 d 0 hr, room 30%, caps 50/70 (default), needs 0.42%/h) (shared reset not within 48h while an own account is usable)
- erin [shared] WAIT (5h 5%, week 50%, resets in 5 d 22 hr, room 20%, caps 50/70 (default), needs 0.14%/h) (shared reset not within 48h while an own account is usable)
- derek [shared] skip: fable week is 96% (cap 73.3%) (5h 20%, week 66%, fable week 96%, resets in 1 d 6 hr, room 0%, caps 50/73.3 (default)) (weekly cap 70% rises to 90% over the last 36h before the reset: 73.3% now)

$ agx pool pick --id auto --cached --model opus
Pool: auto (claude, order deadline)
Use: derek (shared)
shared account resets within 48h, before any usable own account; using its room before it is lost
Source: cache (20s old)
Probe model: opus (--model)
Rules: order deadline (pool), deadline_window 12h (pool), deadline_min_room 3 (pool), week_relax_window 36h (pool), defaults.week_relax_max 95 (default), max_sessions 2 (pool), sticky false (pool), sticky_leave_ratio 4 (pool), shared_borrow_window 48h (default), short_unavailable_at_percent 99 (default), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (default), defaults.week 70 (default)

Candidates (ready accounts in pick order):
- derek [shared] #1 READY (5h 20%, week 66%, resets in 1 d 6 hr, room 7.3% (24% by the reset), caps 50/73.3 (default), needs 0.8%/h) (weekly cap 70% rises to 90% over the last 36h before the reset: 73.3% now)
- alice [own] #2 READY (5h 30%, week 60%, resets in 4 d 0 hr, room 38%, caps 95/98 (default), needs 0.4%/h)
- carol [shared] WAIT (5h 10%, week 40%, resets in 3 d 0 hr, room 30%, caps 50/70 (default), needs 0.42%/h) (shared reset not within 48h while an own account is usable)
- erin [shared] WAIT (5h 5%, week 50%, resets in 5 d 22 hr, room 20%, caps 50/70 (default), needs 0.14%/h) (shared reset not within 48h while an own account is usable)
```

The usage cache keeps each model's windows separately, so sessions on different models share it: each model is probed about once per `usage_max_age` per member.

## Usage history

Every answered probe also adds a point to `~/.agx/cache/login-usage-history.jsonc`: per member the time, the probe model and every window's percent and reset. It costs no API call (it records what the probes already returned) and is bounded: a point at most every 10 minutes (or when a reset changes), 8 days and 1200 points per member. The pool log line (`log`) carries the picked member's percentages too (`usage="5h=12 week=40 fable_week=61" models="fable"`). Both feed calibration of the pool's assumptions later; nothing reads them to pick, and no agx command prints the history yet (read the file itself; it holds no token). Deleting it loses only the history.

## Schedules and overrides

A member's caps can change for a while without touching its normal caps. A **schedule** does it again and again, on set days and hours; an **override** does it once, for one member, until a time. `agx pool pick` and `agx pool show` name each one and when it ends.

### Use teammates' accounts more on their days off

Your teammates work Monday to Friday, so on the weekend nobody needs the 5-hour room their accounts' caps keep free for them. A schedule raises the 5-hour cap of every shared member on Saturday and Sunday, and the weekly cap of only those whose week resets before the weekend is over:

<!-- snippet: examples/acct-pool-schedules/config.jsonc#pool -->
```jsonc
    "auto": {
      "agents": ["claude"],
      "kind": "pool",
      "pool": {
        "own": ["alice"],
        "shared": ["carol", "derek", "erin", "fiona"], // teammates' accounts: caps 50/70 on their working days
        "timezone": "Europe/Istanbul",         // the teammates' zone: schedules and --until read times in it
        "schedules": [
          {
            "name": "weekend",
            "days": ["sat", "sun"],            // all day (no "hours"): Saturday 00:00 to Monday 00:00
            "members": ["shared"],
            // short: the 5-hour cap. week_before_reset: the weekly cap of a member whose weekly reset
            // comes before the schedule ends (its room is lost then anyway); the others keep 70.
            "caps": { "short": 95, "week_before_reset": 95 },
            "release_before": "5h"             // ends at Sunday 19:00, so Monday starts with a fresh 5-hour window
          }
        ],
        "accounts": {
          // An override that ended on Friday: ignored, listed by pool show, removed by pool prune.
          "erin": { "override": { "short": 90, "until": "2026-11-20T18:00:00+03:00" } }
        }
      }
    }
```

or with flags:

```sh
agx login edit --id auto --pool-timezone Europe/Istanbul \
  --pool-schedule 'weekend days=sat,sun members=shared short=95 week_before_reset=95 release_before=5h'
agx pool show --id auto        # lists it, and whether it applies now
agx pool pick --id auto        # caps 95 (schedule weekend until Sun 19:00)/70 (default)
```

On Saturday the [example](../../examples/acct-pool-schedules/README.md) pick uses fiona first: her weekly use is 91% and her week resets in 4 hours 45 minutes, before the schedule ends, so her weekly cap is 95 at once (the relax alone would give her about 90 now, and she would be skipped). Then carol and derek, whose 5-hour use (62%, 55%) is over their normal cap of 50 but under the weekend's 95. erin stays out: her weekly use is over 70 and her week resets on Friday, after the weekend, so the schedule leaves her weekly cap alone:

<!-- snippet: examples/acct-pool-schedules/session.txt#pick -->
```sh
$ agx pool pick --id auto
Pool: auto (claude, order urgency)
Use: fiona (shared)
resets soonest: 4% left under its weekly cap with 4.8h to the reset (0.84%/h to use it all); its reset is within 24h (deadline_window), so using it first loses nothing
Source: cache (20s old)
Probe model: the agent's default (all-models windows only)
Rules: order urgency (default), deadline_window 24h (default), deadline_min_room 2 (default), week_relax_window 24h (default), defaults.week_relax_max 95 (default), max_sessions 4 (default), sticky true (default), sticky_leave_ratio 3 (default), shared_borrow_window 48h (default), short_unavailable_at_percent 99 (default), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (default), defaults.week 70 (default)

Candidates (ready accounts in pick order):
- fiona [shared] #1 READY (5h 20%, week 91%, resets in 4 hr 45 min, room 4%, caps 95/95 (schedule weekend until Sun 16:00), needs 0.84%/h, resets within deadline_window) (weekly cap 95% (schedule weekend: resets before it ends))
- carol [shared] #2 READY (5h 62%, week 41%, resets in 2 d 20 hr, room 29%, caps 95 (schedule weekend until Sun 16:00)/70 (default), needs 0.43%/h)
- derek [shared] #3 READY (5h 55%, week 30%, resets in 4 d 23 hr, room 40%, caps 95 (schedule weekend until Sun 16:00)/70 (default), needs 0.34%/h)
- alice [own] skip: 5h is 96% (cap 95%) (5h 96%, week 81%, resets in 4 d 0 hr, room 17%, caps 95/98 (default))
- erin [shared] skip: week is 72% (cap 70%) (5h 12%, week 72%, resets in 5 d 22 hr, room 0%, caps 95 (schedule weekend until Sun 16:00)/70 (default)) (override ended Fri 20 Nov 18:00 +0300 and is ignored; agx pool prune removes it)
```

Why these two caps, and no other:

- **Raising the 5-hour cap on a teammate's day off is safe.** A 5-hour window that fills up on Saturday has emptied long before they start work on Monday.
- **Raising the weekly cap of every member is not.** A teammate's weekly room has to last their whole week, and a week does not restart on Monday: what you use on Saturday is gone on Tuesday. So `week` stays at 70.
- **Except for a member whose week resets before the schedule ends.** Its room is lost at that reset, before its teammate is back, so it cannot help the teammate at all; it can only help you. `week_before_reset` gives such a member its value as its weekly cap at once, instead of the slow rise of the relax: a teammate's account at 91% with its reset 4h45m away has a relaxed cap of about 90 and would sit skipped while the cap creeps up, and sessions would switch to it and away again as it crossed the cap. A member whose reset comes after the schedule ends keeps its normal weekly cap, because its room still has to last its teammate's week. The end counts `release_before` (the schedule's real end, Sunday 19:00 here), and a reset at the very end counts as before it.
- **It never lowers a cap.** A member whose cap is already higher (your own at 98, or a relax already above the value) keeps it, and an override that sets `week` wins outright. A schedule that names the member beats one that names its group, as for every cap. A per-model window (Fable's week) gets the value when its own reset comes before the end. It counts in the room and the urgency only while the schedule applies, as the relax's rise counts only once it has begun; `--pool-only-week` still caps it for one run.
- **The end of each week is already covered on working days.** `week_relax_window` raises the weekly cap toward 95 over the last 24 hours before each member's own weekly reset, whatever the day, because that room would be lost at the reset anyway (rule 4).
- **`release_before` keeps Monday fresh.** A schedule stops applying this long before its window ends. When a schedule raises `short` and does not say, it is `5h`, one 5-hour window: the last window the pool can fill at 95 starts before Sunday 19:00 and ends before midnight, so a teammate who starts on Monday morning finds their 5-hour window empty. `"release_before": "0s"` turns it off. A schedule that sets only `week_before_reset` defaults to `0s`.

What it does in the [simulator](pool-simulator.md). The scenarios `weekend-mates` (no schedule), `weekend-mates-schedule` (the 5-hour cap only) and `weekend-mates-before-reset` (with `week_before_reset` 95) are one own account and three to five teammates' accounts whose teammates work Monday to Friday, three to six sessions every day of the week, 60 seeded runs each:

| | no schedule | `short` 95 | `short` 95 and `week_before_reset` 95 |
|---|---|---|---|
| hours a week your sessions waited for an account | 11.8 | 11.3 | 10.7 |
| part of your sessions' demand served | 97.2% | 97.3% | 97.5% |
| weekly room wasted at the resets | 40.4% | 40.4% | 40.3% |
| teammates locked out of their 5-hour window (h/week, whole pool) | 0.10 | 0.10 | 0.09 |
| teammates locked out by their weekly limit (h/week, whole pool) | 0.37 | 0.49 | 0.51 |
| account switches per session per day | 6.21 | 6.13 | 5.84 |

The 5-hour cap does most of it, and `week_before_reset` adds a little on top: about 5% less waiting and 5% fewer switches, with no change to teammates' lockouts worth the name (0.02 hours a week). The weekend schedule itself costs teammates some weekly room: with the 5-hour cap out of the way on the weekend, more of each teammate's 70% gets used, and a teammate who uses more than the other 30% of their week themselves runs out sooner.

Over every scenario the simulator has (37 pool shapes, where by default one teammate in five also works on Saturdays; rows `REC+WE-real` and `REC+WEB-real`), `week_before_reset` cost the same as the schedule without it (0.922 of the classic rules' cost on average, worst case 1.008 instead of 1.013), wasted and served the same, and switched accounts about 5% less, but locked teammates out slightly more (4.61 instead of 4.56 hours a week, nearly all of it weekly). That rise sits in large teams and busy pools (up to half an hour a week more in the 15- and 20-account pools), where the simulator's teammates sometimes work on Saturday; the likely cause is a teammate whose week resets during the weekend while they work: the pool now uses the last 25% of that week before the reset, which they could have used themselves. If some teammates work on the schedule's days, name the others in it instead of `shared`; if your teammates use their own accounts heavily, keep the schedule off or give them a lower weekly cap (`--pool-group-cap-week shared=60`).

### Change one value of a schedule

`--pool-schedule` on a name that exists changes only the keys it gives; every other key of that schedule stays as it is. `key=` (empty) removes a key. `--pool-schedule-replace` writes the whole schedule and drops what it does not give, and `--pool-schedule-remove` removes it. Each changed key prints its own before/after line, so a value that goes away shows as `->  —`, in the dry run too:

<!-- snippet: examples/acct-pool-schedules/session.txt#schedule-flags -->
```sh
$ agx login edit --id auto --pool-schedule 'weekend week_before_reset=90' --dry-run
would change login auto (--dry-run)
  pool.schedules.weekend.caps.week_before_reset  95  ->  90

$ agx login edit --id auto --pool-schedule 'weekend hours=06:00-24:00 release_before=' --dry-run
would change login auto (--dry-run)
  pool.schedules.weekend.hours           —   ->  06:00-24:00
  pool.schedules.weekend.release_before  5h  ->  —

$ agx login edit --id auto --pool-schedule-replace 'weekend days=sat,sun members=shared short=95' --dry-run
would change login auto (--dry-run)
  pool.schedules.weekend.caps.week_before_reset  95  ->  —
  pool.schedules.weekend.release_before          5h  ->  —

$ agx login edit --id auto --pool-schedule 'nights days=mon,tue,wed,thu,fri hours=22:00-07:00 members=erin short=80' --dry-run
would change login auto (--dry-run)
  pool.schedules.nights.days        —  ->  mon,tue,wed,thu,fri
  pool.schedules.nights.hours       —  ->  22:00-07:00
  pool.schedules.nights.members     —  ->  erin
  pool.schedules.nights.caps.short  —  ->  80

$ agx login edit --id auto --pool-schedule 'holiday days=thu,fri members=shared short=95 week_before_reset=95' --dry-run
would change login auto (--dry-run)
  pool.schedules.holiday.days                    —  ->  thu,fri
  pool.schedules.holiday.members                 —  ->  shared
  pool.schedules.holiday.caps.short              —  ->  95
  pool.schedules.holiday.caps.week_before_reset  —  ->  95

$ agx login edit --id auto --pool-schedule-remove weekend --dry-run
would change login auto (--dry-run)
  pool.schedules.weekend.days                    sat,sun  ->  —
  pool.schedules.weekend.members                 shared   ->  —
  pool.schedules.weekend.caps.short              95       ->  —
  pool.schedules.weekend.caps.week_before_reset  95       ->  —
  pool.schedules.weekend.release_before          5h       ->  —
```

- Raise one cap: `--pool-schedule 'weekend week_before_reset=90'`. The days, hours, members, zone, the other caps and `release_before` stay.
- Remove a key: `'weekend timezone='` (the pool's zone applies again), `'weekend week='` (the members' own weekly cap applies again), `'weekend release_before='` (the default again).
- A new name creates a schedule; it needs `days`, `members` and a cap. A change that leaves a schedule without one of them is refused and names the key (`days= removes the schedule's days, but a schedule needs them`); remove the schedule with `--pool-schedule-remove` instead.
- Name a schedule once per command, across the three flags (`--pool-schedule-remove weekend and --pool-schedule weekend both change schedule weekend`, exit 2); several schedules in one command are fine.

### Use one person's account fully for a while

An override sets one member's caps until a time, then it is ignored:

```sh
agx login edit --id auto --pool-cap-short carol=95 --pool-cap-week carol=95 --until 'sun 23:59'
agx login edit --id auto --pool-cap-short carol=95 --until 6h
agx login edit --id auto --pool-cap-week carol=90 --until 2026-11-23T08:00
agx login edit --id auto --pool-clear-override carol      # end it now
agx pool prune --id auto                                  # remove the ones that have ended
```

`--until` takes a duration (`6h`, `90m`), a date and time (`2026-11-23T08:00`, or RFC 3339 with a zone), a date alone (the end of that day), `'DAY HH:MM'` (the next such moment: `'sun 23:59'`) or `HH:MM` (today, or tomorrow when that time has passed), read in the pool's `timezone`. It must be in the future. It changes this command's `--pool-cap-short`, `--pool-cap-week` and `--pool-cap-week-relax` only, and is written as `accounts.NAME.override` (`{"short": 95, "until": "2026-11-22T23:59:00+03:00"}`) next to the member's normal caps, which stay as they are. Giving `--until` again while the override applies keeps its other caps and moves its end; `NAME=` drops one cap from it. Once its end has passed the pick ignores it and says so (`override ended Fri 20 Nov 18:00 +0300 and is ignored; agx pool prune removes it`), `agx pool show` lists it as ended, and `agx pool prune` removes it. In the [example](../../examples/acct-pool-schedules/README.md), on Saturday:

<!-- snippet: examples/acct-pool-schedules/session.txt#override -->
```sh
$ agx login edit --id auto --pool-cap-short derek=95 --pool-cap-week derek=90 --until 'sun 23:59' --dry-run
would change login auto (--dry-run)
  pool.accounts.derek.override  —  ->  short 95, week 90 until 2026-11-22T23:59:00+03:00

$ agx login edit --id auto --pool-cap-short derek=95 --pool-cap-week derek=90 --until 'sun 23:59'
changed login auto (pool, pool 1 own, 4 shared)
  pool.accounts.derek.override  —  ->  short 95, week 90 until 2026-11-22T23:59:00+03:00

$ agx pool pick --id auto
Pool: auto (claude, order urgency)
Use: fiona (shared)
resets soonest: 4% left under its weekly cap with 4.8h to the reset (0.84%/h to use it all); its reset is within 24h (deadline_window), so using it first loses nothing
Source: cache (20s old)
Probe model: the agent's default (all-models windows only)
Rules: order urgency (default), deadline_window 24h (default), deadline_min_room 2 (default), week_relax_window 24h (default), defaults.week_relax_max 95 (default), max_sessions 4 (default), sticky true (default), sticky_leave_ratio 3 (default), shared_borrow_window 48h (default), short_unavailable_at_percent 99 (default), own_caps.short 95 (default), own_caps.week 98 (default), defaults.short 50 (default), defaults.week 70 (default)

Candidates (ready accounts in pick order):
- fiona [shared] #1 READY (5h 20%, week 91%, resets in 4 hr 45 min, room 4%, caps 95/95 (schedule weekend until Sun 16:00), needs 0.84%/h, resets within deadline_window) (weekly cap 95% (schedule weekend: resets before it ends))
- derek [shared] #2 READY (5h 55%, week 30%, resets in 4 d 23 hr, room 60%, caps 95/90 (override until Sun 20:59), needs 0.57%/h)
- carol [shared] #3 READY (5h 62%, week 41%, resets in 2 d 20 hr, room 29%, caps 95 (schedule weekend until Sun 16:00)/70 (default), needs 0.43%/h)
- alice [own] skip: 5h is 96% (cap 95%) (5h 96%, week 81%, resets in 4 d 0 hr, room 17%, caps 95/98 (default))
- erin [shared] skip: week is 72% (cap 70%) (5h 12%, week 72%, resets in 5 d 22 hr, room 0%, caps 95 (schedule weekend until Sun 16:00)/70 (default)) (override ended Fri 20 Nov 18:00 +0300 and is ignored; agx pool prune removes it)

$ agx login edit --id auto --pool-clear-override derek
changed login auto (pool, pool 1 own, 4 shared)
  pool.accounts.derek.override  short 95, week 90 until 2026-11-22T23:59:00+03:00  ->  —
```

### Which cap wins

For each cap (`short`, `week`, `week_relax_max`) on its own, the first of these that sets it:

1. the member's override, while it has not ended;
2. the schedules that apply now and name the member;
3. the schedules that apply now and name the member's group (`own`, `shared`);
4. the normal caps: the member's entry, its group's block, the pool's `defaults`, agx's default.

A schedule's `week_before_reset` then raises the weekly cap of a member whose weekly reset comes before that schedule ends: the weekly cap is the higher of the cap above (with the relax) and the value. It goes through steps 2 and 3 the same way (the schedules naming the member first, then those naming its group; the highest value of the first step with a schedule that ends at or after the reset), and is skipped when the member's override sets `week`.

The urgency (rule 5) counts up to the cap the member will have at its reset: a schedule or override that ends before the reset does not raise the room counted there. A higher weekly cap that applies now counts until it ends: the room above the cap that follows must be used before then, and when that needs the faster rate the pick says so (`29% left under its weekly cap of 99% before its override ends in 1h`).

When several schedules at the same step set the same cap, the highest value wins: each of them is meant to allow that much, so the most permissive one applies (a tie goes to the first in the file). A schedule that sets only `short` leaves the weekly cap to the steps below it. A schedule that names a group replaces the own caps of every member in it too: if one teammate asked for lower caps (`accounts.erin.short: 30`), name the other members in the schedule instead of `shared`. The safety cutoff (`short_unavailable_at_percent`, 99) and the weekly relax still apply on top, and `--pool-only-short` / `--pool-only-week` may lower a timed cap for one run but never raise it.

### Days, hours, zones

- `days` are `mon` to `sun`; a window starts on each listed day.
- `hours` is `HH:MM-HH:MM` (omitted = `00:00-24:00`). An end at or before the start ends on the next day: `"days": ["fri"], "hours": "22:00-06:00"` is Friday 22:00 to Saturday 06:00. Back-to-back windows join into one, so `sat` and `sun` all day end once, on Monday 00:00, and `release_before` counts back from there.
- Times are wall-clock times in the schedule's `timezone`, else the pool's, else this machine's local time. Across a daylight-saving change a day is 23 or 25 hours long and the window follows the clock; a time the change skips moves forward.
- `agx pool pick` and `agx pool show` print when a schedule or override ends in this machine's time zone (`--tz` on `agx pool pick` picks another), not the schedule's: the example runs in UTC, so the weekend schedule that ends at Sunday 19:00 in Istanbul shows `until Sun 16:00` there. The note on an override that has ended keeps the zone it was written with (`+0300`).
- Every value is checked when the config loads: known days, well-formed hours, an IANA zone, caps from 0 to 100, `week_before_reset` included (never `null`), members of the pool or a group name, a unique name, at least one cap, no unknown key.

## For your team

- **Heavy-use teams: a 60% shared weekly cap.** When teammates use their own accounts hard, `--pool-group-cap-week shared=60` (or `defaults.week: 60`) keeps more of their week for them. In the simulator it cut teammates' lockouts and was about 0.6% cheaper overall, at the price of serving about 1.5 points less of your own demand. It is a team agreement, so it is an option, not the default.
- **Plans of different sizes.** Give each member its `capacity` (`--pool-capacity alice=20 --pool-capacity carol=5`): a Max 20x account's percent then counts four times a Max 5x one's. Without it the defaults still work; with it every group in the simulator did better.
- **One teammate who wants their whole week.** `--pool-cap-week-relax erin=70` (their weekly cap or lower) turns the weekly relax off for them alone. A schedule with `week_before_reset` that names `shared` still raises their weekly cap when their week resets before it ends; name the other members in it instead if they work on those days.
- **Urgency alone.** `--pool-deadline-window 0s` turns the deadline group off. In the simulator the two came out even (within 0.3% of cost in every group of pools, the same waste); the group makes the order follow the reset that loses room first, and it costs a little more teammate lockout in large teams.
- **The old behaviour.** `agx pool preset --name classic --id POOL` writes the rules agx used before v0.4.16 (soonest reset first, every session on one pick).

## Related

- [Share a pool of Claude accounts](login-pool.md): the guide, from making a pool to steering a running session
- [The pool simulator](pool-simulator.md): how the defaults were chosen, and how to test a change
- [Config reference: login pools](../config.md#login-pools)
- [`agx pool`](../commands/pool.md): list, show, pick and preset
