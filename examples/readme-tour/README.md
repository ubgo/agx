# README tour

The quick start of the repository [README](../../README.md): write a config from the homes already on the machine, add a login whose token comes from gopass, preview a launch on it, bind a folder to a profile, start Codex with an autonomy level, and resume an earlier conversation on the home that stores it. Every block the README shows is a region of this session, so the README always shows what agx prints today.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/readme-tour$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Run from `~/projects/client` on a machine with no config. Regions: `setup` (`config init --discovered`, `profile list`), `login` (`login add --token-ref`, `run --dry-run` on that login), `project` (`project use`, then a plain `run --dry-run` that the binding decides), `codex` (`profile add` for Codex, `run --autonomy ask`, which starts the fake `codex` and shows the args it received), `resume` (`conversation list`, `resume --id --dry-run`). |

There is no `config.jsonc` and no `home/`: the session starts from the standard fake machine, and `config init` writes the config.
