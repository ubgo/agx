# launch-before-run

A script that runs before the agent starts: from the profile (`before_run`, found in the config dir), from the launch folder (`before_run_from: folder`), or for one run (`--before-run`), and whether it sees the login token (`before_run_env`). Use it for a VPN check, a token refresh or any setup a launch needs; a failing script stops the launch.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-before-run$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `work` (a script in the config dir), `repo` (`before_run_from: folder`), `hidden` and `full` (`before_run_env`), and the login `carol`. Regions: `profiles`, `env`. |
| [`home/.agx/scripts/work-env.sh`](home/.agx/scripts/work-env.sh) | The `work` script: succeeds when the work home exists. Region: `work-env`. |
| [`home/.agx/scripts/vpn-check.sh`](home/.agx/scripts/vpn-check.sh) | A script that fails, for `--before-run`. Region: `vpn-check`. |
| [`home/.agx/scripts/show-token.sh`](home/.agx/scripts/show-token.sh) | Says whether the token reached it, then fails so the message shows. Region: `show-token`. |
| [`home/projects/app/scripts/setup.sh`](home/projects/app/scripts/setup.sh) | A repository's own script, for `before_run_from: folder`. Region: `setup`. |
| [`session.txt`](session.txt) | Regions: `dry-run`, `launch`, `from-folder`, `one-run`, `env`. |

Explained in [Passing anything to an agent](../../docs/launch-options.md#a-script-before-the-agent-before_run).
