# Where to set the profile and login

You can name the profile and login of a launch in five places: a flag on the command (`--profile`, `--login`, `--home-login`, `--auto-profile`, `--auto-login`), `AGX_LOGIN` in the terminal, a folder binding (`agx project use`), the profile itself (its `login`, `default`), and the agent's defaults (`agents.<agent>.defaults.login`). They do not compete at random: each answer comes from the first source in a fixed chain, and `--dry-run` names the source. This page helps you choose the place. The full chain is in [How agx decides](../how-agx-decides.md#the-chain); bindings are in [Folder bindings](../guides/folder-bindings.md).

## Short answer

| You want | Set it | Lasts |
|---|---|---|
| one run on another account | `--profile` or `--login` on that command | one command |
| every run in this terminal on one login | `AGX_LOGIN=NAME` (or `agx login use --id NAME --terminal` with the [shell layer](../shell.md)) | until the terminal closes or you unset it |
| a repository always on the work account, whatever terminal you are in | a folder binding: `agx project use --dir DIR --agent claude --profile work` | until `agx project remove` |
| an account always on its own plan token | the profile's `login`: `agx login use --id carol --profile work` | config |
| every Claude profile on one login unless something else says | `agx login use --id carol --agent claude` (`agents.claude.defaults.login`) | config |
| the account with the most quota left | `--auto-profile` or `--auto-login` (probes usage; see [Usage: live or cached](usage-live-or-cached.md)) | one command |
| a profile used when nothing names one | `agx profile use --id NAME` (its `default`) and `default_agent` | config |

## The order

The first source that answers wins.

| Question | 1 | 2 | 3 | 4 | 5 | 6 |
|---|---|---|---|---|---|---|
| profile | `--profile` | `--auto-profile` | the folder's binding for that agent | the agent's default profile | | |
| login | `--token-ref`, `--login`, `--home-login`, `--auto-login` | `AGX_LOGIN_<AGENT>`, then `AGX_LOGIN` | the folder's binding | the profile's `login` | `agents.<agent>.defaults.login` | the home's own login |

So the command line beats the terminal, the terminal beats the folder, and the folder beats the config. A broader place never silently overrides a narrower choice you made on purpose.

## See it

The example machine has two Claude profiles; `work` has `"login": "carol"`, and `~/projects/client` binds Claude to `work` with login `bob`:

<!-- snippet: examples/choose-where-to-set/config.jsonc -->
```jsonc
{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work", "login": "carol" }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude/carol}" },
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude/alice}" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude/bob}" }
  },
  "projects": {
    "~/projects/client": { "claude": { "profile": "work", "login": "bob" } }
  }
}
```

Nothing named: the default profile and its home login.

<!-- snippet: examples/choose-where-to-set/session.txt#default -->
```sh
$ agx run --dry-run --json --field decisions.1 --field decisions.3
agx: agent claude (default_agent)
{
  "decisions.1": {
    "name": "profile",
    "value": "personal",
    "source": "default profile of claude"
  },
  "decisions.3": {
    "name": "login",
    "value": "home login",
    "source": "default (no login named)"
  }
}
```

In the bound folder the binding decides both, and says so on stderr:

<!-- snippet: examples/choose-where-to-set/session.txt#binding -->
```sh
$ agx run --dir ~/projects/client --dry-run --json --field decisions.1 --field decisions.3
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
agx: login bob (folder binding ~/projects/client)
{
  "decisions.1": {
    "name": "profile",
    "value": "work",
    "source": "folder binding ~/projects/client"
  },
  "decisions.3": {
    "name": "login",
    "value": "bob (not resolved: --dry-run)",
    "source": "folder binding ~/projects/client"
  }
}
```

`AGX_LOGIN` beats the binding's login, not its profile:

<!-- snippet: examples/choose-where-to-set/session.txt#terminal -->
```sh
$ AGX_LOGIN=alice agx run --dir ~/projects/client --dry-run --json --field decisions.1 --field decisions.3
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
agx: login alice (AGX_LOGIN)
{
  "decisions.1": {
    "name": "profile",
    "value": "work",
    "source": "folder binding ~/projects/client"
  },
  "decisions.3": {
    "name": "login",
    "value": "alice (not resolved: --dry-run)",
    "source": "AGX_LOGIN"
  }
}
```

A flag beats `AGX_LOGIN`:

<!-- snippet: examples/choose-where-to-set/session.txt#flag -->
```sh
$ AGX_LOGIN=alice agx run --dir ~/projects/client --login carol --dry-run --json --field decisions.1 --field decisions.3
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
{
  "decisions.1": {
    "name": "profile",
    "value": "work",
    "source": "folder binding ~/projects/client"
  },
  "decisions.3": {
    "name": "login",
    "value": "carol (not resolved: --dry-run)",
    "source": "--login"
  }
}
```

`agx project show` answers the same questions for a folder without launching:

<!-- snippet: examples/choose-where-to-set/session.txt#show -->
```sh
$ agx project show --dir ~/projects/client
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
agx: login bob (folder binding ~/projects/client)
agent    claude             from folder binding ~/projects/client
profile  work               from folder binding ~/projects/client
home     ~/.claude-work     from profile work
login    bob                from folder binding ~/projects/client
folder   ~/projects/client  from --dir
```

## If this, then that

- If the choice belongs to the code (client work always on the client account), bind the folder. It works the same in every terminal, IDE and script, and the stderr note keeps it visible.
- If the choice belongs to you right now (this afternoon on the personal plan), use `AGX_LOGIN` or a flag. Nothing is written.
- If the choice belongs to the account (the work home always on its own token), set the profile's `login`.
- If you script agx, pass flags: they are the only source that cannot be changed by the folder or the environment the script happens to run in.
- If quota decides, use `--auto-profile` or `--auto-login`. They make live usage calls (and probes for logins), so keep them off hot loops.

## Pitfalls

- `--profile` overrides a binding's profile but not its login: in a bound folder, `--profile personal` still runs on the binding's login. Add `--login` or `--home-login` to choose the login too.

<!-- snippet: examples/choose-where-to-set/session.txt#profile-flag -->
```sh
$ agx run --dir ~/projects/client --profile personal --dry-run --json --field decisions.1 --field decisions.3
agx: login bob (folder binding ~/projects/client)
{
  "decisions.1": {
    "name": "profile",
    "value": "personal",
    "source": "--profile"
  },
  "decisions.3": {
    "name": "login",
    "value": "bob (not resolved: --dry-run)",
    "source": "folder binding ~/projects/client"
  }
}

$ agx run --profile work --dry-run --json --field decisions.1 --field decisions.3
{
  "decisions.1": {
    "name": "profile",
    "value": "work",
    "source": "--profile"
  },
  "decisions.3": {
    "name": "login",
    "value": "carol (not resolved: --dry-run)",
    "source": "profile work"
  }
}
```

- Two flags for one choice are refused (exit 125), never ranked:

<!-- snippet: examples/choose-where-to-set/session.txt#conflict -->
```sh
$ agx run --login carol --home-login --dry-run --quiet
agx: --login and --home-login both choose the login; pick one
[exit 125]
```

- A login from `AGX_LOGIN` that does not serve the run's agent is refused, not skipped. Use `AGX_LOGIN_CLAUDE` to set a login for Claude runs only.
- A binding covers its folder and every folder below it; the nearest bound folder wins. Symlinks are resolved first, so a folder reached through a symlink finds its binding.
- In config, "the home login" has no magic name: leave `login` out, or set `"home_login": true` to override an inherited one. Setting both in one place is a config error.

## Related

- Guide: [How agx decides](../how-agx-decides.md) (the chain, folder bindings), [Logins](../logins.md#per-terminal-login-agx_login) (`AGX_LOGIN`), [Folder bindings](../guides/folder-bindings.md), [Let agx pick by headroom](../guides/auto-pick.md).
- Commands: [`agx project`](../commands/project.md), [`agx login use`](../commands/login.md), [`agx profile use`](../commands/profile.md).
- Example: [choose-where-to-set](../../examples/choose-where-to-set/README.md), [decide-login-chain](../../examples/decide-login-chain/README.md), [decide-folder-binding](../../examples/decide-folder-binding/README.md), [acct-use-login](../../examples/acct-use-login/README.md).
- Other decisions: [Which login kind](login-kind.md), [A second account](second-account.md), [With or without the shell layer](shell-layer.md).
