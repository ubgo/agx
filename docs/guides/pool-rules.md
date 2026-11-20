# How the pool chooses

A [login pool](login-pool.md) hands each Claude session the token of one member each time the session asks for a key. This page is the whole decision: the order of the rules, every value with its default, the presets, the per-model weekly windows, and what to change for your team. There is no hidden mode: every rule is a config value, and `agx pool show --id POOL` and `agx pool pick --id POOL` print each value in effect and where it comes from (`default`, `pool`, `group` or `member`).

The defaults were chosen with the [pool simulator](pool-simulator.md) to do well for anyone: a solo user with two accounts, a small team, a large team pool, any plan sizes, with or without teammates using their own accounts. They are not tuned to one setup.

## The rules, in order

1. **Force.** `--pool-force` (one run, one `pool pick` or `login token` call), else the pool's `force`, wins outright; every rule below is ignored and each one the forced member breaks is a warning. A `--pool-only` member instead makes the pool that one member with every rule below still applied, and no force.
2. **Usable.** A member is skipped when any of these holds, and every reason is listed: disabled; no usage reading; a per-use API key in an `oauth-token` member without `allow_api_key`; a status other than `ok`; usage that is not a number; 5-hour use at or over `short_unavailable_at_percent` (99); 5-hour use at or over its 5-hour cap; weekly use at or over its weekly cap; a per-model weekly window of the session's model at or over the weekly cap, or refused by the vendor until its reset ([per-model windows](#per-model-weekly-windows)). Caps are exclusive (under the cap is usable) and count the account's total use.
3. **Tiers.** While any own member is usable, the pool is the usable own members plus the usable shared members whose weekly reset is within `shared_borrow_window` (48h); the other usable shared members wait (`WAIT`). With no own member usable, every usable shared member is in the pool.
4. **Weekly relax.** Over the last `week_relax_window` (24h) before a member's weekly reset, its weekly cap rises in a straight line to its `week_relax_max` (95): a teammate's account at cap 70 has 82.5 twelve hours before its reset and 95 at it. That room would be lost at the reset anyway. The 5-hour cap never rises (that window outlives the weekly reset and is the teammate's working time). A member whose cap is already higher (your own at 98) keeps it.
5. **Order.** `order: "urgency"` (the default) puts first the member with the most room to lose per hour: the room left under its weekly cap divided by the hours to the reset, times its `capacity`. `order: "deadline"` puts the soonest weekly reset first. An unknown or passed reset sorts last; ties keep config order, own before shared.
6. **A pick per session.** Each session (each `--login POOL` run) gets its own pick. It stays on its member while that member is still in the pool (`sticky`), unless another member is at least `sticky_leave_ratio` (3) times as urgent; a member takes at most `max_sessions` (4) sessions before the next one is used, then the waiting members, then the least loaded. A session counts while it asked for a key within two `key_ttl`s. With `max_sessions: 0` and `sticky: false` every session gets the first member.

An empty pool picks nothing: the key request fails with each member's reasons, and `agx pool pick` exits 1 (error code `auto-pick`).

## Every value and its default

Each is a key of `logins.<pool>.pool` in config.jsonc, set with a flag of `agx login add` / `agx login edit` or with `agx config set` (one value), and listed in the [config reference](../config.md#login-pools).

| Key | Default | Flag | Meaning |
|---|---|---|---|
| `order` | `urgency` | `--pool-order` | `urgency` or `deadline` (rule 5) |
| `week_relax_window` | `24h` | `--pool-week-relax-window` | how long before the weekly reset the weekly cap starts rising; `0s` = never |
| `defaults.week_relax_max` | 95 | `--pool-default-week-relax` | the weekly cap reached at the reset; also per group (`own_caps.week_relax_max`, `shared_caps.week_relax_max`) and per member (`accounts.NAME.week_relax_max`) |
| `max_sessions` | 4 | `--pool-max-sessions` | sessions one member takes before the next is used; `0` = no limit |
| `sticky` | `true` | `--pool-sticky` | a session stays on its member while it is still in the pool (keeps its prompt cache) |
| `sticky_leave_ratio` | 3 | `--pool-sticky-leave-ratio` | a sticky session leaves for a member this many times as urgent; `0` = never |
| `shared_borrow_window` | `48h` | `--pool-borrow-window` | a shared member goes before a usable own one only when its week resets within this; `0s` = never |
| `short_unavailable_at_percent` | 99 | `--pool-short-cutoff` | the 5-hour safety cutoff for every member |
| `own_caps.short`, `own_caps.week` | 95, 98 | `--pool-group-cap-short own=N`, `--pool-group-cap-week own=N` | caps of your own members without an entry of their own |
| `defaults.short`, `defaults.week` | 50, 70 | `--pool-default-short`, `--pool-default-week` | caps of shared members without a `shared_caps` or member entry |
| `shared_caps.short`, `shared_caps.week` | (`defaults`) | `--pool-group-cap-short shared=N`, `--pool-group-cap-week shared=N` | caps of every shared member at once |
| `accounts.NAME.short`, `.week` | (group) | `--pool-cap-short NAME=N`, `--pool-cap-week NAME=N` | one member's caps |
| `accounts.NAME.capacity` | 1 | `--pool-capacity NAME=N` | the member's plan size relative to the others (Pro 1, Max 5x 5, Max 20x 20); its room is weighed by it |
| `probe_models` | (none) | `--pool-probe-model` | the models to probe with when a session's own model is unknown ([per-model windows](#per-model-weekly-windows)) |
| `usage_max_age` | `60s` | `--pool-usage-max-age` | how old the shared usage cache may be before members are probed again |
| `key_ttl` | `60s` | `--pool-key-ttl` | how long Claude keeps a key before asking again |

A member's cap comes from its own entry (`member`), else its group's block (`group`), else for a shared member the pool's `defaults` (`pool`), else agx's default (`default`). Own members never take `defaults`: without `own_caps` they get 95/98, so your own quota is used almost fully.

## Presets

A preset is a named set of the rule values above, written into the config by one command. It is a shortcut, never a mode: afterwards each value is an ordinary key that `agx pool show` lists and you can change by hand. Caps are not touched (they are your team's agreement).

| Preset | `order` | `week_relax_window` | `week_relax_max` | `max_sessions` | `sticky` | `sticky_leave_ratio` | `shared_borrow_window` |
|---|---|---|---|---|---|---|---|
| `recommended` (the defaults) | `urgency` | `24h` | 95 | 4 | `true` | 3 | `48h` |
| `classic` (the rules before v0.4.16) | `deadline` | `0s` | 95 | 0 | `false` | 0 | `48h` |

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
[{"key":"force","value":"off","source":"default"},{"key":"order","value":"urgency","source":"pool"},{"key":"week_relax_window","value":"24h","source":"pool"},{"key":"defaults.week_relax_max","value":"95","source":"pool"},{"key":"max_sessions","value":"4","source":"pool"},{"key":"sticky","value":"true","source":"pool"},{"key":"sticky_leave_ratio","value":"3","source":"pool"},{"key":"shared_borrow_window","value":"48h","source":"pool"},{"key":"short_unavailable_at_percent","value":"99","source":"pool"},{"key":"own_caps.short","value":"95","source":"default"},{"key":"own_caps.week","value":"98","source":"default"},{"key":"defaults.short","value":"50","source":"pool"},{"key":"defaults.week","value":"70","source":"pool"},{"key":"usage_max_age","value":"60s","source":"pool"},{"key":"key_ttl","value":"60s","source":"pool"},{"key":"picker.command","value":"built-in rules","source":"default"},{"key":"picker.timeout","value":"10s","source":"default"},{"key":"log","value":"none","source":"default"}]
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

`agx pool pick` names the probe model and shows each window:

```text
Probe model: fable (--model)
- alice [own] skip: fable week is 99% (cap 98%) (5h 5%, week 20%, fable week 99%, resets in 2 d 18 hr, room 0%, caps 95/98 (default))
```

The usage cache keeps each model's windows separately, so sessions on different models share it: each model is probed about once per `usage_max_age` per member.

## Usage history

Every answered probe also adds a point to `~/.agx/cache/login-usage-history.jsonc`: per member the time, the probe model and every window's percent and reset. It costs no API call (it records what the probes already returned) and is bounded: a point at most every 10 minutes (or when a reset changes), 8 days and 1200 points per member. The pool log line (`log`) carries the picked member's percentages too (`usage="5h=12 week=40 fable_week=61" models="fable"`). Both feed calibration of the pool's assumptions later; nothing reads them to pick.

## For your team

- **Heavy-use teams: a 60% shared weekly cap.** When teammates use their own accounts hard, `--pool-group-cap-week shared=60` (or `defaults.week: 60`) keeps more of their week for them. In the simulator it cut teammates' lockouts and was about 0.6% cheaper overall, at the price of serving about 1.5 points less of your own demand. It is a team agreement, so it is an option, not the default.
- **Plans of different sizes.** Give each member its `capacity` (`--pool-capacity alice=20 --pool-capacity carol=5`): a Max 20x account's percent then counts four times a Max 5x one's. Without it the defaults still work; with it every group in the simulator did better.
- **One teammate who wants their whole week.** `--pool-cap-week-relax erin=70` (their weekly cap or lower) turns the weekly relax off for them alone.
- **The old behaviour.** `agx pool preset --name classic --id POOL` writes the rules agx used before v0.4.16 (soonest reset first, every session on one pick).

## Related

- [Share a pool of Claude accounts](login-pool.md): the guide, from making a pool to steering a running session
- [The pool simulator](pool-simulator.md): how the defaults were chosen, and how to test a change
- [Config reference: login pools](../config.md#login-pools)
- [`agx pool`](../commands/pool.md): list, show, pick and preset
