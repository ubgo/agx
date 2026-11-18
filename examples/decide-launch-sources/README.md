# Decide: everything a launch shows

One full `agx run --dry-run` and what each line's source becomes with flags: the folder from `--dir`, `new` and the current folder; the binary and its version source; the args from `agents.claude.defaults.args`, the profile's `args` and after `--`, and a profile with `inherit_defaults: false`; options from `--model`, `--option` and the profile; settings from `--setting` and how they are delivered; the environment from the profile's `env`, `--env` and `--no-env`; the run folder and the command. Read it with [how agx decides](../../docs/how-agx-decides.md#see-every-decision---dry-run).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/decide-launch-sources$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | `default_agent` claude; `agents.claude.defaults.args` `--chrome`; profile `personal` (default, model, args, env, a `claude` block with `effort`) and `work` (`inherit_defaults: false`, its own args). Regions: `agents`, `profiles`. |
| [`session.txt`](session.txt) | Regions: `full` (a plain dry run), `flags` (`--dir`, `--model`, `--option`, `--env`, `--no-env`, `--` args), `setting` (`--setting`, trimmed), `no-inherit` (the `work` profile), `new` (`agx new --name`), `missing-folder` (refused, exit 125). |

The example machine's fake binaries have no install metadata, so every `binary` line reads `from no install metadata`; a real install reads `from install metadata` with the version.
