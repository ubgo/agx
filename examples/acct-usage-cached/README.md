# Usage from disk

`agx usage --cached` reads what each agent left on disk, with no network: here an old Claude reading (stale), a Claude home with none (Claude Code 2.1.323 and later no longer write one), Codex with no reading yet, and Gemini, which keeps none. It also shows `--table`, `--sort`, `--tz`, `--plan` from disk, `--json --field`, the exit codes of partial (3) and failed (1) answers, and the refusals of the login flags (`--login-probes` or `--all-logins` with `--cached`, `--login-picker` without a terminal, an unknown `--login`). Live usage, `--plan` without `--cached` and the login probes need a real account and the network, so they are not run here.

The guide is [Plan usage](../../docs/guides/usage.md); the flags are in the [usage reference](../../docs/commands/usage.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-usage-cached$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Claude profiles `personal` and `kimi` (`"billing": "api"`) sharing `~/.claude`, `work`, and `old` (its home does not exist); `codex`, `gem`; the login `carol`. |
| [`session.txt`](session.txt) | Regions: `notes`, `table`, `stale`, `footer`, `partial`, `refusals`. |
| [`home/.claude-client/`](home/.claude-client) | A Claude home on disk that no profile uses; the usage footer names it. |
| [`home/.claude.json`](home/.claude.json) | The default Claude home's account file with a plan label and an old `cachedUsageUtilization` reading, as an older Claude Code left it. |
