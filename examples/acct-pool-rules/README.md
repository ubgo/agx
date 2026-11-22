# Pool rules and per-model windows

Every rule of a pool is a config value with a flag of `agx login edit`. This example is a pool of four fake Claude logins (`alice` your own, `carol`, `derek` and `erin` teammates') on Wednesday 18 November 2026, 10:00 UTC: `agx pool pick` under the default `order: urgency`, every rule flag as a dry run and then written in one `agx login edit`, `agx pool show` with each value and its source, the same pick under `order: deadline`, a pick with `--model fable` that skips a member whose Fable week is full, `probe_models` written and applied to a plain pick, `--model` winning over it, and the refusals. Every value is fake.

No command probes an account: every pick passes `--cached` and reads the login-usage cache in [`home/.agx/cache/login-usage.jsonc`](home/.agx/cache/login-usage.jsonc), written 20 seconds before the session's clock. Under `urgency` `alice` goes first (38% of room over 4 days, 0.4%/h) and `derek` second (his week resets in 30 hours with 4% of room, which is why he is in the pool at all: `shared_borrow_window` is 48h); `carol` and `erin` wait. Under `deadline` the soonest weekly reset goes first, so `derek` leads even though `alice` now has capacity 20. `derek`'s cache entry also holds the per-model windows of earlier probes: under the keys `fable` and `claude-fable-5-2` a Fable week (`7d_oi`) at 96%, and none for `opus`. A Fable session skips him (`fable week is 96% (cap 73.3%)`: his weekly cap of 70 already rises toward the group's relax max of 90 over the 36-hour relax window), an Opus session still picks him.

The guides are [How the pool chooses](../../docs/guides/pool-rules.md) (the rules, every value and its default, [per-model weekly windows](../../docs/guides/pool-rules.md#per-model-weekly-windows)) and [Share a pool of Claude accounts](../../docs/guides/login-pool.md); every flag is in the [login reference](../../docs/commands/login.md) and the [pool reference](../../docs/commands/pool.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-pool-rules$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Four Claude token logins from `${…_TOKEN}` variables and the pool `auto` with no rule key yet (every value is agx's default). Region: `pool`. |
| [`session.txt`](session.txt) | `agx pool pick --cached` under `order: urgency`; `agx login edit` dry runs of `--pool-order`, `--pool-deadline-window`, `--pool-deadline-min-room`, `--pool-week-relax-window`, `--pool-cap-week-relax`, `--pool-group-cap-week-relax`, `--pool-max-sessions`, `--pool-sticky`, `--pool-sticky-leave-ratio`, `--pool-capacity` and `--pool-probe-model`, then one write of all but the last; `agx pool show`; the pick under `order: deadline`; `--model fable` (text and the JSON of the skipped member's per-model window), `--pool-probe-model` written, a plain pick that applies it and `--model opus` over it; the refusals: an unknown order, a sticky that is not true or false, a capacity of 0, an unknown member, an unknown group, a window that is not a duration. Regions: `pick-urgency`, `edit`, `show`, `pick-deadline`, `model`, `refusals`. |
| [`home/.agx/cache/login-usage.jsonc`](home/.agx/cache/login-usage.jsonc) | The members' last probe: windows, resets, status, and for `alice` and `derek` the per-model windows by probe model (`models`). |
