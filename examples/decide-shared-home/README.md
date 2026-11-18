# Decide: shared homes

Two profiles on one Claude home (`personal` on the plan, `kimi` through another provider's API) and how `agx resume` picks between them: refused with no default and two plan profiles, `--profile`, the only plan profile after `--billing api`, the default profile, and a home with one profile. Also a launch of each profile, `agx usage --cached` with one row per home, and the `kimi` profile made with `profile add`, `profile edit` and `profile use`. Read it with the [shared homes guide](../../docs/guides/shared-homes.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/decide-shared-home$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | `personal` and `kimi` on `~/.claude` (no default, both plan billing), `work` alone on `~/.claude-work`. `kimi` sets `ANTHROPIC_BASE_URL` and a gopass reference for `ANTHROPIC_AUTH_TOKEN`. Region: `profiles`. |
| [`session.txt`](session.txt) | Run from `~/projects/client`, where the fake machine holds conversation `3f2a9c1e` (in `~/.claude`) and `7b1d4e20` (in `~/.claude-work`). Resume dry runs are trimmed after the `folder` line. Regions: `list`, `ambiguous` (exit 125), `profile-flag`, `billing`, `default`, `one-profile`, `launch`, `usage`. |
| [`add.session.txt`](add.session.txt) | On its own fresh machine: remove `kimi`, add it back on `~/.claude` with `--billing api`, set its base URL, make `personal` the default. Region: `add`. |
