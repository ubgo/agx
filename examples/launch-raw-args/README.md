# Raw args: agent defaults, profile args and after --

Raw args reach the agent exactly as written, appended after everything agx builds: first `agents.<agent>.defaults.args`, then the profile's `args`, then everything after `--`. A profile with `"inherit_defaults": false` drops the agent defaults. The session also shows the note when a raw arg repeats a typed choice, a raw `--settings` winning over agx's own settings, and `agx profile edit --args` / `--no-inherit-defaults`.

When you want it: a flag agx has no option for, a flag every launch of an agent should get, or one profile (an API provider on a shared home, say) that must not get the agent's usual flags.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-raw-args$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | `agents.claude.defaults.args` (`--chrome`), a profile `work` with `args`, a profile `kimi` with `"inherit_defaults": false`. Regions: `agents`, `profiles`. |
| [`session.txt`](session.txt) | Regions: `defaults`, `profile`, `after`, `no-inherit`, `duplicate`, `raw-settings`, `edit`, `dashes`. |

Guide: [Raw args](../../docs/guides/raw-args.md). Reference: [`agx run`](../../docs/commands/launch.md#agx-run), [`agx profile edit`](../../docs/commands/profile.md#agx-profile-edit).
