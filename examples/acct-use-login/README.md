# Choose the login of a run

Every place a login can come from, and what wins: the agent default (`agents.claude.defaults.login`), the profile's `login` and `home_login`, `--login`, `--home-login`, a one-off `--token-ref` with `--token-kind`, `AGX_LOGIN` and `AGX_LOGIN_CLAUDE` for one terminal, `agx login use --terminal`, and a folder binding (`agx project use --login`). Each command prints the login decision of a dry run and its source. The refusals: a terminal login that does not serve the run's agent, and two flags that both choose the login.

The guide is [Using a login](../../docs/logins.md#using-a-login) and [Per-terminal login](../../docs/logins.md#per-terminal-login-agx_login); the full order is in [How agx decides](../../docs/how-agx-decides.md#the-chain).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-use-login$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Claude's default login `carol` (region `defaults`); profiles `personal` (no login of its own), `work` (`"login": "carol"`), `isolated` (`"home_login": true`, region `isolated`) and `gem` (Gemini, `"login": "gem-work"`); logins `carol`, `dave` and `gem-work`. |
| [`session.txt`](session.txt) | Regions: `per-run` (`--login`, `--home-login`), `home-login` (the agent default, and a profile's `home_login` over it), `one-off` (`--token-ref`, `--token-kind`), `terminal` (`AGX_LOGIN`, `AGX_LOGIN_CLAUDE`, `agx login use --terminal`), `terminal-wrong-agent`, `folder` (`agx project use --login`), `conflict`. |
