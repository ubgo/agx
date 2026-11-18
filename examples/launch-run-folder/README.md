# Run folders, whoami and clean

Two real launches (the fake `claude` prints the arguments, folder and environment it received), then `agx clean` listing, refusing and deleting run folders. It also shows the home variable each agent gets for a non-default home and that a default home gets none. A second session shows `agx whoami` outside and inside a session.

When you want it: you script around agx (a status line, a hook that reads `AGX_RUN_PROFILE`), you wonder what `~/.agx/runs/` is, or you want to clean it.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-run-folder$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Claude profiles `personal` (the default home `~/.claude`) and `work` (`~/.claude-work`, one settings key, so a settings file lands in the run folder); Codex, Gemini and pi profiles on default and non-default homes. Regions: `profiles`, `other-agents`. |
| [`session.txt`](session.txt) | `agx run` (exec into the agent), `agx run --dir`, the home variable per agent, a missing `--dir`, the agent's exit code. Regions: `launch`, `dir`, `home-vars`, `dir-missing`, `exit-code` (the fake agent exits 3, so agx does). |
| [`clean.session.txt`](clean.session.txt) | `agx clean` on two run folders copied in from `home/.agx/runs/`: one whose agent ended, one without `run.jsonc`. The plan, `--dry-run`, the `--yes --dry-run` refusal, `--yes`. Region: `clean`. |
| [`whoami.session.txt`](whoami.session.txt) | `agx whoami` outside a session, with `AGX_LOGIN`, inside one (the `AGX_RUN_*` variables set by hand), and a nested `agx run` that ignores them. Regions: `outside`, `inside`, `nested`. |
| `home/` | The non-default homes `~/.codex-work`, `~/.gemini-work/.gemini` and `~/.pi-work`; two run folders under `~/.agx/runs/` for the clean session (a `run.jsonc` with a pid that no longer runs, and a folder with only a settings file). |

Guide: [Run folders, whoami and clean](../../docs/guides/run-folders.md). Reference: [`agx run`](../../docs/commands/launch.md#agx-run), [`agx whoami`](../../docs/commands/launch.md#agx-whoami).
