# Choose: login kind

Every login kind and source side by side on one machine: a plan token from gopass, a pasted plan token in `tokens.jsonc`, an `api-key-helper`, an `api-key` serving Claude and pi, and a Gemini key. It shows how each one reaches its agent (the variable it sets, or the settings file), which kinds each agent refuses, `--resolved-secrets`, and why live login needs a stored token login.

Read it with [Which login kind](../../docs/choosing/login-kind.md) when you are deciding how to add an account.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-login-kind$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | `login list`, `login check --no-probe`, refused `login add` per agent, dry runs per kind, live-login refusals. Regions: `list`, `agents`, `plan-token`, `api-key`, `helper`, `one-off`, `resolved`, `live`, `wrong-agent`. |
| [`config.jsonc`](config.jsonc) | Four profiles (Claude, Codex, Gemini CLI, pi) and five logins. Region: `logins`. |
| [`home/.agx/tokens.jsonc`](home/.agx/tokens.jsonc) | The pasted value of login `pasted` (fake, mode 0600). |

## Related

- Decision guide: [Which login kind](../../docs/choosing/login-kind.md).
- Guide: [Logins](../../docs/logins.md).
- All examples: [examples](../README.md).
