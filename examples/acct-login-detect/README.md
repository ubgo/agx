# Is a login billed per use?

A login's `kind` is a label in config; agx reads the token's public prefix to say what it really is: `oauth-token` (a setup-token, billed to the plan), `api-key` (an API key, billed per use) or `unknown`. This example lists the detected kind of every login, checks them, shows the launch refusal of an API key filed under `oauth-token` and the two ways to allow it, and a pool skipping such a member. Every value is fake, and no output holds any part of a token: only the kind word.

The guide is [Logins](../../docs/logins.md#is-a-login-billed-per-use); the flags are in the [login reference](../../docs/commands/login.md#agx-login-list) and the [launch reference](../../docs/commands/launch.md#agx-run).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-login-detect$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profile `personal` and five logins: `alice` (a stored setup-token), `bob` (a stored API key declared `oauth-token`), `console` (a stored API key declared `api-key`), `carol` (a gopass reference) and the pool `team` (own `bob`, `alice`). Region: `logins`. |
| [`session.txt`](session.txt) | `agx login list` with its DETECTED column, `--verify` for the gopass reference, `agx login check --no-probe`, the refused `agx run --login bob`, `--allow-api-key` and `allow_api_key`, and `agx pool pick` skipping `bob`. Regions: `list`, `verify`, `check`, `refused`, `allow`, `pool`. |
| [`home/.agx/tokens.jsonc`](home/.agx/tokens.jsonc) | The fake pasted tokens, each only starting with its kind's public prefix. |
| [`home/.agx/cache/login-usage.jsonc`](home/.agx/cache/login-usage.jsonc) | The pool's login-usage cache: `alice`'s windows, and `bob` recorded as an API key that was never probed. |
