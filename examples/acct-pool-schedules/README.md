# Pool schedules and overrides

A pool's caps can change for a while without touching the normal caps: a schedule raises them on set days and hours (here the teammates' accounts get a 5-hour cap of 95 on Saturday and Sunday, when the teammates do not work), and an override raises one member's caps until a time. This example is a pool of four fake Claude logins (`alice` your own, `carol`, `derek` and `erin` teammates') on Saturday 21 November 2026, 13:00 in Istanbul: `agx pool show` lists the schedule and an override that ended, `agx pool pick` shows the caps each member gets now and where they come from, the same pick on Monday morning (the schedule has ended), an override with `--until`, a schedule added and removed with flags, `agx pool prune`, and the refusals. Every value is fake.

No command probes an account: the login-usage cache in [`home/.agx/cache/login-usage.jsonc`](home/.agx/cache/login-usage.jsonc) was written 20 seconds before the session's clock. On Saturday `carol` (5-hour use 62%) and `derek` (55%) are usable only because the weekend schedule raises their 5-hour cap from 50 to 95; `erin` is over her weekly cap of 70, which the schedule leaves as it is.

The guide is [How the pool chooses: schedules and overrides](../../docs/guides/pool-rules.md#schedules-and-overrides); every flag is in the [login reference](../../docs/commands/login.md) and the [pool reference](../../docs/commands/pool.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-pool-schedules$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Four Claude token logins from `${…_TOKEN}` variables and the pool `auto` with a timezone, the `weekend` schedule and an ended override. Region: `pool`. |
| [`session.txt`](session.txt) | `agx pool show`, `agx pool pick` now and on Monday (`--cached --now`), the JSON of a member's timed caps, an override with `--until` (dry run, write, pick, `--pool-clear-override`), `--pool-schedule` and `--pool-schedule-remove` (dry runs), `agx pool prune`, and the refusals: `--until` without a cap, an unknown day, a time in the past, a bad day, bad hours, an unknown member, a cap over 100, an unknown time zone. Regions: `show`, `pick`, `pick-monday`, `pick-json`, `override`, `schedule-flags`, `prune`, `refusals`. |
| [`home/.agx/cache/login-usage.jsonc`](home/.agx/cache/login-usage.jsonc) | The members' last probe: windows, resets, status. |
