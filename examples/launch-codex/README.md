# launch-codex

What agx passes to Codex: `--setting` becomes `-c key=value` with TOML quoting, typed options come from the profile's `codex` block (including `codex_profile`, Codex's own `-p`) or `--option`, `--autonomy ask` maps to Codex's sandbox and approval flags, and raw args go after `--`. It also shows the refusals: a bad option value, a setting of the wrong type, and a stored login (Codex has no token login).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-codex$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `codex` and `deep` (a `codex` block with `codex_profile`, `search` and `settings`), and a Claude login `carol`. Regions: `profiles`, `deep`. |
| [`session.txt`](session.txt) | Regions: `setting`, `option`, `block`, `bad-value`, `unknown-setting`, `no-token-login`. |

Explained in [Passing anything to an agent](../../docs/launch-options.md#recipe-9-codex).
