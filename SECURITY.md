# Security policy

agx handles the logins of your coding agents: plan tokens, API keys and the references that produce them. A flaw that could leak, misuse or swap one of them is a security issue.

## Reporting a vulnerability

Please do not report security vulnerabilities in public issues, pull requests or discussions.

Report them privately through GitHub's private vulnerability reporting: on the repository, open **Security**, then **Report a vulnerability** (`https://github.com/ubgo/agx/security/advisories/new`). Include:

- what the problem is and what an attacker could do with it;
- the steps to reproduce, with the agx version (`agx version`) and your operating system;
- whether a real token was exposed (never send the token itself: say where it appeared).

You will get an acknowledgement within a few days and updates while a fix is prepared. Please allow a reasonable window for a fix before any public disclosure; reporters are credited in the advisory unless they ask not to be.

## Supported versions

agx is pre-release: no version has been published yet. Security fixes go to the `main` branch, which is the development line of v0.4. The archived v0.3 (`github.com/khanakia/agx`) receives no fixes.

| Version | Supported |
|---|---|
| `main` (v0.4, pre-release) | yes |
| v0.3 (`github.com/khanakia/agx`) | no |

## How agx handles tokens

These are the rules the code follows. A case where agx breaks one of them is a vulnerability; please report it.

- **Never refreshed.** agx never refreshes, rewrites or prints an agent's own stored OAuth credential. Each agent refreshes its own login.
- **Never written into an agent home.** agx never writes a token into an agent's home or settings files. A token value is written only where you put it: a token pasted with `agx login add --stdin` (or at the hidden prompt) goes into `~/.agx/tokens.jsonc`, and a plain value passed to `--token-ref` goes into `~/.agx/config.jsonc` as you typed it. When the shell layer runs `agx new` or `agx resume` in two steps, the plan file between them (mode 0600, refused when it is old, foreign or writable by others) holds the flags as you typed them and never a resolved secret, so it carries a token only if you typed a plain value on the command line; the launch deletes it.
- **`tokens.jsonc` is private.** It is written atomically with mode 0600 in a 0700 config folder, and agx refuses to read it when group or others have any permission on it. `agx doctor` reports a wrong mode with the `chmod` that fixes it. The login-usage cache of [login pools](docs/guides/login-pool.md), `~/.agx/cache/login-usage.jsonc`, holds account labels (emails) and usage, never a token; it is written atomically with mode 0600 in a 0700 folder, and a pool's optional log gets login names only.
- **Never on argv by agx's own doing.** A token reaches the agent through its environment (for example `CLAUDE_CODE_OAUTH_TOKEN`), never as a command-line argument agx adds. Only arguments you pass yourself (a plain value to `--token-ref` or `--env`, or raw args after `--`) can put one on a command line, and they are then also in your shell's history. With `--live-login` (Claude only), Claude's `apiKeyHelper` runs agx's hidden `credential` command, which hands the token of the login chosen for that run to Claude on its stdout pipe; the run folder's `live.jsonc` records login names, never a token.
- **Never printed, unless you ask.** The one exception is `agx login token --id NAME`, which prints that login's token on request for a pipe or an `apiKeyHelper` (on a login pool, the token of the member it picks; `agx login pick` shows the pick and prints no token): only the value on stdout, refused when stdout is a terminal unless you add `--reveal`, and never inside `--json`. Everywhere else, token values read `[redacted]` in every output: human text, `--json` envelopes, `--dry-run`, errors and logs. A value is secret by its source, never by its variable's name: a login's token (plain or a reference), every value from a reference (`env_refs`, an `--env` reference) or an env file, and any value on a variable an agent reads a credential from (such as `ANTHROPIC_API_KEY`), whatever its source. Such a value is also hidden wherever else it appears (another variable, an argument, a decision). A literal `env` value or `--env NAME=value` on any other variable is shown as written, even when its name looks secret; `agx doctor` flags such a literal in config so you can move it to `env_refs`. See [what is redacted](docs/output.md#what-is-redacted).
- **References resolve at launch only.** A login normally holds a reference (`${gopass:path}`, `$(command)`, `${VAR}`), resolved only when the agent starts. `--dry-run` resolves nothing and shows `not resolved: --dry-run`; `--dry-run --resolved-secrets` resolves each reference and prints only "resolved" or why it failed, never the value. Expansion is strict: an empty or unresolved secret is an error, never an empty login.
- **No discovery behind your back.** agx reads only the homes and logins you configure; finding other homes on the machine happens only when you ask (`agx profile list --discovered`).

## Scope

In scope: agx itself (this repository). Problems in an agent (Claude Code, Codex, Gemini CLI, pi) or in its vendor's service belong to that vendor. Problems in [agentio](https://github.com/ubgo/agentio) or [textdata](https://github.com/ubgo/textdata) may be reported here when agx is affected.
