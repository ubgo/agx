# Auto-pick refusals

`--auto-profile` and `--auto-login` read usage before they choose, so a successful pick needs a real account and the network. This example shows everything that is decided before any request: candidates that bill per token, a home that does not exist, a login whose token does not resolve, an agent with no usage or no token probe, an agent with no stored login, and the flag conflicts. Every refusal names each candidate and why it was skipped, and exits 125 (a launching command).

The guide is [Let agx pick by headroom](../../docs/guides/auto-pick.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-auto-pick$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Claude profiles `kimi` and `kimi-work` (`"billing": "api"`) and `old` (its home `~/.claude-old` does not exist); `codex` and `gem`; logins `carol` (from `${CAROL_TOKEN}`, not set), `console` (a key helper) and `gem-work`. |
| [`session.txt`](session.txt) | Regions: `profile-none`, `profile-per-token`, `profile-unsupported`, `login-none`, `login-unsupported`, `conflict`. |
