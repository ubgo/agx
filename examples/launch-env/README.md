# launch-env

Environment variables for one launch: a profile's literal `env`, its `env_refs` (references expanded strictly), its `env_file` (with and without `env_file_commands`), and for one run `--env-file`, `--env` and `--no-env`. It also shows `--resolved-secrets`, the strict expansion errors, the two refusals (the home variable and a second credential), and what output redacts. Use it when an account needs an API key, a region or a debug switch the agent reads from its environment.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-env$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `work` (`env`, `env_refs`, `env_file`), `builds` (`env_file_commands: true`), `nocmds` (the same file without it), `service` (literal `env` values, one whose name looks secret), and the login `carol`. Region: `profiles`. |
| [`home/.agx/envs/work.env`](home/.agx/envs/work.env) | The `work` dotenv file, relative to the config dir; one value is a `${gopass:…}` reference. Region: `work-env`. |
| [`home/.agx/envs/cmds.env`](home/.agx/envs/cmds.env) | A dotenv file with a `$(…)` command. Region: `cmds-env`. |
| [`home/projects/app/local.env`](home/projects/app/local.env) | The `--env-file` of one run, relative to the current folder. Region: `local-env`. |
| [`session.txt`](session.txt) | Regions: `profile`, `resolved` (`--resolved-secrets`), `resolved-error`, `one-run`, `launch` (what the agent receives), `commands`, `commands-off`, `unset`, `scheme`, `defaults`, `home-var`, `credential`, `missing-file`, `redaction` (redacted by source, never by name; doctor's hint for a literal that looks secret). |

Explained in [Passing anything to an agent](../../docs/launch-options.md#recipe-5-any-environment-variable) (recipe 5, [Strict expansion](../../docs/launch-options.md#strict-expansion)).
