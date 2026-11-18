# Agent defaults

`agents.<agent>.defaults` sets raw args and a login for every profile of an agent. A profile inherits them, replaces the args (`inherit_defaults: false` plus its own `args`), names its own login, or asks for the home's own login (`home_login`). A flag still wins over all of them. The session also shows the profile identity keys (`agent`, `home`, `default`, `billing`) in `agx profile list`.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/cfg-agent-defaults$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | `agents.claude.defaults` with `args` and `login`, and three profiles: one that inherits, one with its own login and args, one with `home_login` and `billing: api`. Regions: `agents`, `profiles`. |
| [`session.txt`](session.txt) | Regions: `personal` (the full dry run: login and args from the defaults), `work`, `own`, `list` (`profile list`), `flag` (`--home-login` over the default login), `default-home-login` (`config set` refused: `login` and `home_login` both set). |

Reference: [Agents](../../docs/config.md#agents), [Profiles](../../docs/config.md#profiles).
