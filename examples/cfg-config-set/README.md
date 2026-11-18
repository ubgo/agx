# Config set, path and init

Change one key with `agx config set` (comments and layout kept), see the result with `agx config show`, the refusals for a wrong value, a non-scalar key, an unknown key and a value that would make the file invalid, which file agx reads (`--config`, `AGX_CONFIG_DIR`, `AGX_HOME`, `~/.agx`), and `agx config init --dry-run`.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/cfg-config-set$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Two Claude profiles, with comments and a `$schema` line. |
| [`session.txt`](session.txt) | Regions: `set` (four keys), `show-file` (each value with its line and column), `set-refused` (exit 2 each), `where` (`config path` under each rule), `init` (`config init --dry-run` into a fresh folder). |

Guide: [Check your setup](../../docs/guides/doctor.md#step-5-change-one-key). Reference: [`agx config set`](../../docs/commands/setup.md#agx-config-set), [`agx config path`](../../docs/commands/setup.md#agx-config-path).
