# Login pool

A login of kind `pool` holds no token: each time Claude Code asks for a key, agx picks one of its member logins by usage and hands out that member's token, so one long session moves between accounts as quotas fill up or expire. This example shows a pool of five fake Claude logins (`alice` and `bob` your own, `carol`, `derek` and `erin` teammates'), the pick and why (`agx login pick`), a one-run `--force` and a config `force`, the picked member's token (`agx login token`), a dry run of a live launch on the pool, adding a second pool with flags, setting its caps with the pool flags (with `--dry-run` first), and the refusals. Every value is fake.

No command probes an account: the login-usage cache in [`home/.agx/cache/login-usage.jsonc`](home/.agx/cache/login-usage.jsonc) was written 20 seconds before the session's clock, younger than the pool's `usage_max_age`, so agx reuses it. It mirrors example A of the selection rules: `carol`'s week ends in 38 hours, so her room is used before it is lost; `derek` waits (his week ends in 4 days); `bob` is at his 5-hour limit and `erin` over her 5-hour cap.

The guide is [Share a pool of Claude accounts](../../docs/guides/login-pool.md); every flag is in the [login reference](../../docs/commands/login.md#agx-login-pick).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-login-pool$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Five Claude token logins from `${…_TOKEN}` variables, a Codex key, the pool `auto` with every key written out, and the pool `picky` with a picker command. Regions: `pool`, `picker`. |
| [`session.txt`](session.txt) | `agx login list`, `agx login pick` (human and `--field`), `--force`, `login edit --pool-force`, `agx login token`, `agx run --dry-run`, `login add --kind pool`, and the refusals: a Codex pool (exit 4), an unknown member, removing a member a pool lists, `usage --login` on a pool, `login pick` on a token login (exit 2). Also `--time-format both` with a custom `--date-format`, `--duration-format`, the overrides (`--login MEMBER`, `AGX_LOGIN=auto`, `run --pool-force`) and the picker. Then the pool flags: caps, `--pool-disable`/`--pool-enable`, `--pool-key-ttl`, `--dry-run`, `no change`, `agx config set` on a cap, and their refusals. Regions: `list`, `pick`, `pick-json`, `force`, `token`, `times`, `overrides`, `picker`, `run`, `add`, `refusals`, `caps`, `cap-refusals`. |
| [`home/picker.sh`](home/picker.sh) | A tiny picker: prefers `alice` whenever she is ready, else the built-in pick. |
| [`home/.agx/cache/login-usage.jsonc`](home/.agx/cache/login-usage.jsonc) | The members' last probe: windows, resets, status. |
