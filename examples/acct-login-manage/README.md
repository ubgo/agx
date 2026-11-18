# Manage logins

Day-to-day work on logins that already exist: resolve them all without spending anything (`agx login check --no-probe`), make one the default of a profile or of an agent, override an agent default with the home login, see who uses a login, rotate a token, change a kind, an account label or a provider, and remove a login (refused while in use, then `--cascade`).

The guide is [Logins](../../docs/logins.md); every command is in the [login reference](../../docs/commands/login.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-login-manage$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `personal`, `work` and `pi`, and five logins of every source: gopass reference, command, key helper, an API key for Claude and pi with `provider`, and `spare` read from `${SPARE_CLAUDE_TOKEN}`, which is not set. Region: `logins`. |
| [`session.txt`](session.txt) | `agx login check --no-probe` (exit 3: `spare` does not resolve), also with `--id`; `agx login use --profile`, `--agent`, `--home-login`; the login decision of a dry run; `agx login show`; `agx login edit` with `--token-ref`, `--stdin`, `--account`, `--kind`, `--provider`, `--agent` (replaces the list), `--helper-command`, and `--login-setup` (refused without a terminal); `agx login remove` and `--cascade`. Regions: `check`, `use`, `edit`, `remove`. |
| [`home/new-token.txt`](home/new-token.txt) | The fake token `agx login edit --stdin` reads. |
