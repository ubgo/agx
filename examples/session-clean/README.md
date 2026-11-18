# Clean up empty session folders

Session folders in `~/scratch` (`sessions.root`): one with a file and one made empty by the plan-file step. `agx session clean` shows a plan that keeps both (one has files, one is newer than `--min-age`), `--min-age 0s` plans the empty one, and `--yes` removes it. Use it to tidy the scratch folders you opened and never used.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/session-clean$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | One Claude profile and `sessions.root` set to `~/scratch`. Region: `sessions`. |
| `home/scratch/notes_20261102_090000/todo.md` | The folder that has a file, so clean keeps it. |
| [`session.txt`](session.txt) | Regions: `make-empty` (`agx new --plan-file`, `session list`), `plan`, `min-age` (`--min-age 0s`, `--yes`), `refusals` (bad `--min-age`, promote without `--to` or `promote_root`). |

Guide: [Session folders, step 4](../../docs/guides/session-folders.md#step-4-remove-the-folders-you-never-used). Reference: [`agx session clean`](../../docs/commands/conversation.md#agx-session-clean).
