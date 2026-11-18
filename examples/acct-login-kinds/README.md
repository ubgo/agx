# Login kinds

Add one login of every kind (a plan token from gopass, from a command, pasted on stdin, a key helper the agent runs itself, an API key for two agents, a key from an environment variable), list them, and see with `--dry-run` how each one reaches its agent: `CLAUDE_CODE_OAUTH_TOKEN`, a settings file with `apiKeyHelper`, `ANTHROPIC_API_KEY` for pi, `GEMINI_API_KEY` for Gemini. It also shows the refusals: Codex has no token login, `--login-setup` needs a terminal, and a login must serve the run's agent.

Use it when you want to see what each `--kind` does before you add your own. The guide is [Logins: kinds](../../docs/logins.md#kinds) and [Adding a login](../../docs/logins.md#adding-a-login).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-login-kinds$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Five profiles: `personal` (`~/.claude`), `work` (`~/.claude-work`), `codex`, `gem` (Gemini) and `pi`. No logins yet. |
| [`session.txt`](session.txt) | `agx login add` with `--token-ref` (`${gopass:…}`, `$(…)`, `${VAR}`), `--stdin`, `--helper-command`, `--provider`, `--account`, repeated `--agent`; `agx login list` (also `--agent`), `agx login show --agent`; the Codex and `--login-setup` refusals; dry runs per kind, one with `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` set to show them removed. Regions: `add`, `setup`, `provider` (pi without `--provider`, an unknown provider, then `github-copilot`), `list`, `codex`, `oauth`, `helper`, `api-key`, `wrong-agent`. |
| [`home/token.txt`](home/token.txt) | The fake plan token `--stdin` reads; it ends up in `~/.agx/tokens.jsonc` (mode 0600) and the login gets `"stored": true` in config. |
