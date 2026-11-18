# A second account

You have one account per agent and want another: a work plan next to a personal one, a client's subscription, a pay-per-token key. agx gives you three shapes: a new profile with its own home (`agx profile add`), a named login used on the home you already have (`agx login add`), or a second profile on the same home (a shared home, usually with `"billing": "api"`). They differ in what the two accounts share. This page helps you choose; the steps are in [A second account](../guides/second-account.md), [Logins](../logins.md) and [Shared homes](../guides/shared-homes.md).

## Short answer

| You want the second account to have | Choose |
|---|---|
| its own history, settings, MCP servers and its own `claude /login` | a new profile with its own home: `agx profile add --name work --agent claude` |
| its own history, but your instructions, skills and settings | the same, without `--bare`: agx links the default home's shared files into the new home |
| nothing in common with your first account | `agx profile add --name work --agent claude --bare` |
| the same history and settings, only another quota or bill | a named login on the existing home: `agx login add --name carol --agent claude --kind oauth-token …` |
| an API provider or pay-per-token key next to a plan, on the same home | a second profile on that home with `"billing": "api"` and its own `env_refs` or login |
| Codex | a new profile with its own home, then `codex login` in it: Codex has no token login |

## Compare

| | New profile, own home | Named login, same home | Second profile, same home |
|---|---|---|---|
| conversation history | separate | shared | shared |
| settings, `CLAUDE.md`, skills | linked from the default home (or separate with `--bare`) | shared | shared |
| login | the new home's own (`claude /login` there), or a named login | the named login, per run, folder or terminal | its own `login` or `env_refs` |
| quota and billing | the new account | the login's account | as its `billing` says |
| `agx usage` | one row for the new home | probe the login with `--login-probes` | one row per home; `api` homes show `per-token` |
| `agx resume` | finds conversations in the home that stores them | the same home, any login (`--login`) | the same home |
| works for | every agent | Claude, Gemini CLI, pi | every agent |

## See it

A new profile with its own home, linking the default home's shared files:

<!-- snippet: examples/choose-second-account/session.txt#profile -->
```sh
$ agx profile add --name client --agent claude --dry-run
would add profile client: claude home ~/.claude-client (derived from the agent's default home and --name), login home login, billing plan
  create ~/.claude-client
  link ~/.claude-client/CLAUDE.md -> ~/.claude/CLAUDE.md
  link ~/.claude-client/skills -> ~/.claude/skills
  link ~/.claude-client/settings.json -> ~/.claude/settings.json
```

The same with `--bare`:

<!-- snippet: examples/choose-second-account/session.txt#bare -->
```sh
$ agx profile add --name client --agent claude --bare --dry-run
would add profile client: claude home ~/.claude-client (derived from the agent's default home and --name), login home login, billing plan
  create ~/.claude-client
```

A named login used on the existing home. The home stays `~/.claude`; only the token changes:

<!-- snippet: examples/choose-second-account/session.txt#login -->
```sh
$ agx login add --name carol --agent claude --kind oauth-token --token-ref '${gopass:personal/claude/carol}'
added login carol (oauth-token, reference gopass)

$ agx run --login carol --dry-run --quiet --json --field data.home --field data.login --field data.command.env
{
  "data.home": "~/.claude",
  "data.login": "carol",
  "data.command.env": [
    "set AGX_RUN_ID=20261114-101500-k3x9",
    "set AGX_RUN_AGENT=claude",
    "set AGX_RUN_PROFILE=personal",
    "set AGX_RUN_LOGIN=carol",
    "set AGX_RUN_HOME=~/.claude",
    "set CLAUDE_CODE_OAUTH_TOKEN=[redacted]"
  ]
}
```

A second profile on the same home, billed per token:

<!-- snippet: examples/choose-second-account/session.txt#shared -->
```sh
$ agx profile add --name kimi --agent claude --home ~/.claude --billing api --dry-run
would add profile kimi: claude home ~/.claude (--home), login home login, billing api
```

## If this, then that

- If you want to keep work and personal chats apart, give the work account its own home. Bind the work repositories to it (`agx project use`) so you never have to remember.
- If you just want more quota for the same work, add a named login and switch with `--login`, `AGX_LOGIN` or live login; history stays in one place.
- If you have a pay-per-token key or an API provider, add a profile on the same home with `"billing": "api"`: `agx usage` and `--auto-profile` then treat it as per-token instead of reading plan windows.
- If you already have several homes from before agx, do not add them by hand: `agx profile import --discovered` turns found homes into profiles.

## Pitfalls

- The default home runs with its home variable absent. agx sets `CLAUDE_CONFIG_DIR` (or `CODEX_HOME`, …) only for other homes, because pointing Claude at its default home through the variable makes it look logged out:

<!-- snippet: examples/choose-second-account/session.txt#default-home -->
```sh
$ agx run --dry-run --quiet --json --field data.home --field data.command.env
{
  "data.home": "~/.claude",
  "data.command.env": [
    "set AGX_RUN_ID=20261114-101500-k3x9",
    "set AGX_RUN_AGENT=claude",
    "set AGX_RUN_PROFILE=personal",
    "set AGX_RUN_LOGIN=home",
    "set AGX_RUN_HOME=~/.claude"
  ]
}
```

- `agx profile remove` never deletes a home; remove the folder yourself if you want it gone.
- When a decision starts from a home (`agx resume` finds the conversation's home), a home with two profiles resolves to the default one, else its only `plan` profile, else agx asks at a terminal; without one it stops with `… has 2 profiles (…) and no default; add --profile`. Mark one with `agx profile use`.
- Linked shared files are symlinks into the default home: editing `CLAUDE.md` in either home edits both. Use `--bare` for a home that must not follow your personal setup.

## Related

- Guide: [Logins](../logins.md), [How agx decides: shared homes](../how-agx-decides.md#shared-homes), [A second account](../guides/second-account.md), [Shared homes](../guides/shared-homes.md).
- Commands: [`agx profile add`](../commands/profile.md#agx-profile-add), [`agx profile import`](../commands/profile.md#agx-profile-import), [`agx login add`](../commands/login.md#agx-login-add).
- Example: [choose-second-account](../../examples/choose-second-account/README.md), [acct-second-account](../../examples/acct-second-account/README.md) (the full setup), [decide-shared-home](../../examples/decide-shared-home/README.md).
- Other decisions: [Which login kind](login-kind.md), [Where to set the profile and login](where-to-set.md).
