# The shell layer's steps, by hand

The three commands the shell layer's `agx()` function runs for `agx new` and `agx resume`: write a plan (`--plan-file`), read its folder (`agx plan show --field data.dir`), start from it (`agx launch --plan-file`); then a second launch of the same plan and a stale plan, both refused. A second session shows the two lines `agx login use --terminal` prints for the layer, and `agx doctor` with and without `AGX_SHELL`. You never type the plan commands; run this to see what the layer does.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/shell-plan-file$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | One Claude profile and a stored login `carol`. |
| `home/old-plan.json` | A plan file written long ago, refused as stale. |
| [`session.txt`](session.txt) | Regions: `plan`, `launch` (start, then single use), `stale`. |
| [`terminal.session.txt`](terminal.session.txt) | Regions: `login-terminal`, `doctor` (`AGX_SHELL` unset, then `zsh`). |

Guide: [How the cd works](../../docs/shell.md#how-the-cd-works-the-plan-file), [AGX_LOGIN in this shell](../../docs/shell.md#set-agx_login-in-this-shell), [AGX_SHELL](../../docs/shell.md#check-it-is-loaded-agx_shell).
