# Decide: read decisions without launching

`agx profile show` (each value of a profile with the config line and column it came from), `agx config show` (every key with its source) and the JSON form of a dry run's decisions. Read it with [see every decision](../../docs/how-agx-decides.md#see-every-decision---dry-run).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/decide-inspect$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | The same config as [`decide-launch-sources`](../decide-launch-sources/README.md), so the line and column numbers point into it. |
| [`session.txt`](session.txt) | Regions: `profile-show`, `config-show`, `json` (`--json --field decisions.0 --field decisions.1`). |
