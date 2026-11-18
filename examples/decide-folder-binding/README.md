# Decide: folder bindings

Bind folders per agent with `agx project use` (a profile, a second agent, a login, the home's own login), list them, see what a run would do with `agx project show` (the nearest folder per answer, two agents in one folder, flags that override, `--json`), launch with the notes on and with `--quiet`, remove bindings, and every refusal of `project use` and `project remove`. Read it with the [folder bindings guide](../../docs/guides/folder-bindings.md) and the [`agx project`](../../docs/commands/project.md) reference.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/decide-folder-binding$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | `default_agent` claude, Claude profiles `personal` (default) and `work`, Codex profiles `codex` (default) and `codex-work`, login carol; no `projects` yet. Region: `profiles`. |
| [`session.txt`](session.txt) | Every command names its folder with `--dir`. Regions: `use` (with `use-profile`, `use-codex`, `use-login`, `list` inside), `nearest`, `two-agents`, `override`, `run`, `json`, `remove`, `refusals`. |
| [`here.session.txt`](here.session.txt) | On its own fresh machine, run from `~/projects/client`: `use`, `show`, `remove` and `list` without `--dir`. Region: `here`. |
| `home/projects/client/api/.keep`, `home/projects/client/web/.keep`, `home/.codex-work/.keep` | Create the sub-folders and the second Codex home on the fake machine. |
