# Choose: where to set the profile and login

The same launch decided by each source in turn: the default profile, a folder binding (profile and login), `AGX_LOGIN`, a `--login` flag, `--profile` in a bound folder, and a profile `login`. Each command prints the profile and login decisions with their source; `agx project show` answers for a folder without launching.

Read it with [Where to set the profile and login](../../docs/choosing/where-to-set.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-where-to-set$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Regions: `default`, `binding`, `terminal`, `flag`, `profile-flag`, `show`, `conflict`. |
| [`config.jsonc`](config.jsonc) | Two Claude profiles, three logins, and a binding of `~/projects/client`. |

## Related

- Decision guide: [Where to set the profile and login](../../docs/choosing/where-to-set.md).
- Guide: [How agx decides](../../docs/how-agx-decides.md).
- All examples: [examples](../README.md).
