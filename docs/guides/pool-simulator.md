# The pool simulator

agx ships an offline simulator of a [login pool](login-pool.md): our sessions and teammates spend the plan quota of a set of accounts over two weeks while the pool's real pick code (package `pool`) decides, every minute, which account each session uses. It scores the pick on what matters for a pool and compares rule sets and settings on the same randomised weeks; the pool's [defaults](pool-rules.md) were chosen with it. It never calls an API and spends no quota.

It is for people changing the pool rules or choosing settings, not something you need to run a pool.

## Run it

```sh
task sim                                   # every scenario, every rule set, the knob sweeps: writes pool/testdata/sim/RESULTS.md (a few minutes)
task sim -- -scenario small-mixed          # one scenario, printed
task sim -- -strategies A,REC -seeds 4     # two rule sets, fewer seeds
task sim -- -step 15m                      # re-pick every 15 minutes instead of every minute
task sim -- -ratio 5                       # assume a week holds five full 5-hour windows
task sim -- -accounts -scenario solo-busy  # room left per account at each weekly reset
task sim -- -burn 2 -mates 0.5             # sessions burn twice as fast, teammates half as fast
```

A fast subset runs inside `go test` (and so `task check`) as a regression gate: the scenarios marked `"gate": true` run the real pick code twice, with the default rule values and with the `classic` preset (the rules before v0.4.16, written as config values: reference A), and must not waste more quota, hit more limits, stall more, lock teammates out more or serve less than the recorded numbers in `pool/testdata/sim/baseline.json`; the defaults must also not lose to the classic preset on waste, service, lockouts or switches. After a deliberate change of the rules, review the new numbers and record them with `go test ./internal/poolsim/ -run TestSimGate -args -update`.

## What it measures

| Metric | Meaning |
|---|---|
| waste | room left unused when a weekly window resets (100% minus the week's use), per account-week |
| 429s | steps in which one of your sessions was refused at a limit, per week |
| stall | busy session-hours with no usable member (the pool hands out no key), per week |
| squeeze | hours a shared account you were using sat above its normal caps while its teammate was active, per week |
| lockout | teammate hours refused because of your use (they alone would have stayed under the limit), per week |
| own room, own drained | room your own accounts had left at their weekly reset; hours they sat at their cap before it |
| switches | account changes per session per day (each one loses the conversation's prompt cache) |
| served | the part of your busy session-hours that got quota |
| cost | one number to rank by: waste summed over the pool plus a price per event of the others; "cost vs A" divides it by the classic preset (reference A) on the same scenario, so pools of any size average fairly |

When there is more quota than your sessions want, waste is the same under every rule set (it is the supply minus everyone's use); it only drops when sessions want more than the pool holds, by serving more of them.

## The model

- **Windows.** Each account resets its week at its own time every seven days. The 5-hour window starts with the first use after the previous one ended and lasts five hours, independent of the weekly reset.
- **Plans.** Pro, Max 5x and Max 20x: the same work burns five times the percent on Pro and a quarter of it on Max 20x. A plan can also be unknown: a real plan is drawn, but the pick is not told (as for a user who sets no `capacity`).
- **Your sessions.** A busy session burns 12% of a Max 5x 5-hour window an hour; a full 5-hour window is an eighth of a week. Sessions are busy in 15-minute spells, steady or in shared bursts.
- **Teammates.** On a shared account a teammate works on their own schedule (light, medium, heavy or irregular; or none at all), whatever the pool does: from about 09:00 their time (UTC+3) for nine hours, Monday to Friday and for one in five also Saturday, unless the scenario names their `workdays`.
- **Schedules.** A scenario's `timezone` and `schedules` are read exactly as a pool's in config and handed to the real `pool.Select`, so the `-real` rows apply them (the hand-written rule sets do not). `weekend-mates`, `weekend-mates-schedule` and `weekend-mates-before-reset` are the same pools with no schedule, with the weekend schedule (5-hour cap 95) and with the weekend schedule plus `week_before_reset` 95; compare their `REC-real` rows ([what they show](pool-rules.md#use-teammates-accounts-more-on-their-days-off)). The rows `REC+WE-real` and `REC+WEB-real` add that weekend schedule, without and with `week_before_reset`, to every scenario in place of its own schedules, so the two compare on every pool shape, including teammates who also work some Saturdays.
- **Limits.** When a step would pass 100%, everyone on the account gets the same share of what is left; your refused part is a 429, the teammate's a lockout.
- **Per-model weeks.** A scenario can give some sessions a model with a weekly window of its own (like Fable 5.2's `7d_oi`): those sessions also fill that window, which holds fewer 5-hour windows than the all-models week, and only their picks see it, as agx's do when it probes with the session's model. `REC-blind` is the defaults without per-model windows (agx before v0.4.16 read them), so its 429s show what reading them saves.
- **What a pick sees.** Only what agx sees: whole-number percentages one step old, the reset times, each per-model window of the session's model, and which member each running session uses.

## Rule sets in the report

`A` is the classic preset and `REC` the default rule values, each also run through the real `pool.Select` (`A-real`, `REC-real`; a test proves the pairs make the same picks); the letters `B` to `H` and the `REC-…` and `REC+…` rows are variations used to choose the defaults, and `REC+W` adds each member's `capacity` where its plan is known. The v0.4.20 order has its own rows: `REC-K` is the defaults without the deadline group (urgency alone), `REC-N` the rules of v0.4.19 (urgency counting only the room under the cap now, and the sticky ratio measured against the first member even when it is full), `REC+A` counts the room up to the relax max for any reset rather than only inside the relax window, `REC-S` keeps the old sticky ratio, and `REC+KL` lets sticky sessions leave for the deadline group. `snapshot-21-evening` replays a recorded pool (made-up names) for the 24 hours after a moment when one teammate's account reset in 6.7 hours with 8% of room to come.

## Add a scenario

A scenario is one file in `pool/testdata/sim/`, JSONC, unknown keys refused. Each seed draws a pool from the ranges; seeds `seed`, `seed+1`, … give the same pools on every run.

```jsonc
{
  "about": "Two own accounts and three teammates on irregular schedules.",
  "seed": 901,           // the first seed
  "seeds": 12,           // how many runs
  "gate": false,         // true: part of the go test gate (record its baseline with -update)
  "own": { "count": [2, 2], "plans": ["max5", "max20"] },   // pro, max5, max20 or unknown
  "shared": { "count": [3, 3], "mates": ["irregular"], "workdays": ["mon", "tue", "wed", "thu", "fri"] },  // mates: light, medium, heavy, irregular, none
  "resets": "random",    // random, spread or bunched
  "sessions": { "count": [2, 5], "duty": [0.5, 0.7], "pattern": "steady", "hours": [8, 2], "utc_offset": "0s" },  // pattern: steady, bursty or any
  "caps": { "own": { "short": 95, "week": 98 }, "shared": { "short": 50, "week": 70 } },
  "model": { "share": 0.7, "week_ratio": 4 },  // optional: 70% of sessions on a model whose own week holds 4 full 5-hour windows
  "timezone": "Europe/Istanbul",                // optional: the pool's timezone and schedules, as in config
  "schedules": [ { "name": "weekend", "days": ["sat", "sun"], "members": ["shared"], "caps": { "short": 95 } } ]
}
```

`count` and `duty` are `[min, max]` ranges drawn per seed; `plans` and `mates` are drawn per member. Omit `own` or `shared` for an all-shared or all-own pool. `accounts` replaces both with fixed members (`name`, `group`, `plan`, `week`, `short`, `reset_hours`, `mate`) for a recorded pool; use made-up names. The group in the report follows from the members unless `group` says otherwise: solo (own accounts only), small team (up to six teammates' accounts and no own), large team (seven or more teammates' accounts and no own), mixed (both). A file with wide ranges and many seeds is a sampler: the `sample-*` files draw many pool shapes per group.

## Related

- [Share a pool of Claude accounts](login-pool.md): the pool
- [How the pool chooses](pool-rules.md): the rules, every value and its default, the presets
- [Config reference](../config.md): every pool key
