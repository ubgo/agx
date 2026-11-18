# Live login

Switch the login of a running Claude session. The dry run shows what `--live-login` adds (an `apiKeyHelper` that runs `agx credential`, delivered in a settings file). A seeded run folder stands in for a session running in another terminal, since the fake agent exits at once: `agx live list` (also `--agent`) and `agx live show` read it, `agx live use --login` chooses another login, `agx credential` is what the agent runs at its next key refresh, and `--start-login` goes back. The refusals: a run on the home login, a profile whose env sets a variable that outranks the helper, Codex, and two choices at once. Last, `agx credential` on three logins whose token is not one usable line (empty, two lines, an unset variable): nothing on stdout, one line on stderr, exit 1.

The guide is [Live login](../../docs/guides/live-login.md); the commands are in the [live reference](../../docs/commands/live.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-live-login$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `personal`, `work` (`"login": "carol"`, `"live_login": true`), `keyed` (its env sets `ANTHROPIC_API_KEY`) and `codex`; logins `carol` and `alice`, and `blank`, `twolines` and `unset`, whose token never resolves to one line. Regions: `profile`, `broken`. |
| [`session.txt`](session.txt) | Regions: `dry-run`, `list`, `use`, `credential`, `after`, `refusals`, `credential-refusals`. |
| [`home/.agx/runs/20261114-101500-k3x9/`](home/.agx/runs/20261114-101500-k3x9) | The seeded run folder: `run.jsonc` (the run record, with no process id, so its liveness is `unknown`) and `live.jsonc` (start login `carol`, never a token). |
