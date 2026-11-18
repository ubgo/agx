# A custom agent

An agent CLI that agx does not ship with, described in `config.jsonc` under `agents.<agent>.spec`: its binary, home, argument templates, autonomy levels and process match. The stand-in binary is `echo`, so each launch prints the exact arguments agx built.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/cfg-custom-agent$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | The custom agent `mycli` (stand-in binary `echo`), a second custom agent `nocli` whose binary is not installed, profiles for both, and an `api-key` login for `mycli`. Regions: `agents`, `profiles`, `logins`. |
| [`session.txt`](session.txt) | Regions: `check` (`config check`), `list` (`agent list` columns), `show` (kind, autonomy levels, token kinds), `dry-run` (every decision; the `binary` line is elided because `echo` lives in a different folder on Linux and macOS), `dry-run-work` (a non-default home sets `MYCLI_HOME`), `run` (`--model`, `--prompt`, `--add-dir` through the templates), `autonomy` (`ask`, `full` with consent, `read-only` refused), `headless` (refused without `headless_args`), `login-add` (a login for `mycli`, a kind its `token_env` does not name refused, `login check` with the probe skipped), `login` (a dry run on that login: `MYCLI_API_KEY` set, redacted), `resume` (refused up front: a spec has no conversation reader), `missing` (binary not on `PATH`, exit 127). |
| `home/.mycli/config.json`, `home/.mycli-work/config.json`, `home/.nocli/config.json` | Marker files, so the folders count as homes. |

Guide: [A custom agent](../../docs/guides/custom-agent.md). Reference: [`agents.<agent>.spec`](../../docs/config.md#a-custom-agent).
