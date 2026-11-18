# A second account

Set up a second Claude account in its own home: find the homes already on the machine (`agx profile list --discovered`, `--search`), preview and add a profile (`agx profile add`, with and without `--bare`, with `--home`, `--login`, `--home-login`, `--billing api`), import found homes (`agx profile import`), change a profile (`agx profile edit`: `--login`, `--env`, `--args`, `--no-inherit-defaults`, `--home`, `--billing`, `--dry-run`, and undoing each), make it the default (`agx profile use`) and remove one (`agx profile remove`, which keeps the folder).

The guide is [A second account](../../docs/guides/second-account.md); every flag is in the [profile reference](../../docs/commands/profile.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-second-account$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | One profile, `personal` on `~/.claude`, and the login `carol`. |
| [`session.txt`](session.txt) | Regions: `discover`, `add-dry`, `add`, `add-bare`, `add-login`, `login-setup`, `import`, `edit`, `edit-undo`, `use`, `remove`. |
| [`home/.claude/`](home/.claude) | The default home's shared files that `profile add` links into a new home: `CLAUDE.md`, `settings.json` and `skills/review/SKILL.md`. |
| [`home/accounts/claude-acme/`](home/accounts/claude-acme) | A home outside the usual places, found only with `--search ~/accounts`; its `.claude.json` makes it a Claude home (a `settings.json` alone would not, outside Claude's own folders). |
