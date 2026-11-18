# Print a login's token

`agx login token --id NAME` prints one login's token, and only the value and a newline, so a pipe, a script or a Claude Code `apiKeyHelper` can read it. This example prints the token of a login of every source (an environment variable, a gopass reference, a `$(…)` command, a pasted token in `tokens.jsonc`), makes an agx login the key helper of another login, and shows the refusals: an `api-key-helper` login (exit 4), `--json` (exit 2), a reference that does not resolve and an unknown login (exit 1). Every value is fake.

The test runner gives each command a pipe for stdout, as `agx login token --id work-key | pbcopy` would. On a terminal the command is refused unless you add `--reveal`; the unit tests cover that case.

The guide is [Logins](../../docs/logins.md#print-a-logins-token); every flag is in the [login reference](../../docs/commands/login.md#agx-login-token).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-login-token$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `personal` and `work`, and six logins: `work-key` from `${WORK_KEY}`, `carol` from gopass, `derek` from a `$(…)` command, `pasted` stored in `tokens.jsonc`, `spare` from `${SPARE_CLAUDE_TOKEN}` (not set) and `vault`, a key helper. Region: `logins`. |
| [`session.txt`](session.txt) | `agx login token` for each source; `agx login add --kind api-key-helper --helper-command 'agx login token --id work-key'` and a dry run on it; the refusals. Regions: `sources`, `helper`, `refusals`. The session sets `WORK_KEY=fake-work-key` for every command. |
| [`home/.agx/tokens.jsonc`](home/.agx/tokens.jsonc) | The fake pasted token of `pasted`. |
