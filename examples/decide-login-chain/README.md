# Decide: the login chain

Every source of a run's login, from the last to the first: the home's own login, the agent default, the profile's `login` and `home_login`, a folder binding (a login, and `home_login` in a sub-folder), `AGX_LOGIN`, `AGX_LOGIN_CLAUDE`, and the flags `--login`, `--home-login` and `--token-ref`. Then the refusals: a login for another agent, two login flags, `--auto-login` with nothing to pick, an unknown login. Read it with [step 4: the login](../../docs/how-agx-decides.md#step-4-the-login).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/decide-login-chain$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | `default_agent` claude; `agents.claude.defaults.login` carol; profiles `personal` (default), `work` (login dave), `work-home` (`home_login`), `codex`; logins carol and dave (gopass references, never resolved here); `~/projects/client` binds login dave, `~/projects/client/vendor` binds the home login. Regions: `agents`, `profiles`, `logins`, `projects`. |
| [`session.txt`](session.txt) | One dry run per source, trimmed after the `login` line with `...`. Regions: `home-default`, `agent-default`, `profile-login`, `binding`, `env`, `flags`, `refusals`. |
| [`home/projects/client/vendor/.keep`](home/projects/client/vendor/.keep) | Creates the sub-folder `~/projects/client/vendor` on the fake machine. |
