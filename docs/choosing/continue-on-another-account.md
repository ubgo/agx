# Continue on another account

A Claude session hits its plan limit, or you want the rest of a task billed to another account. agx gives you four ways out: switch the running session with live login (`--live-login`, then `agx live use`), restart and resume the conversation on another login (`agx resume --login`), move the folder's conversations to another home (`agx conversation move`), or start fresh on another profile. This page helps you choose. Setting up live login is in [Live login](../guides/live-login.md); resuming and moving are in [Conversations](../guides/conversations.md).

## Short answer

| You want | Choose | Session keeps running | History stays in | Agents |
|---|---|---|---|---|
| the running session to carry on, on another account, without a restart | live login: start with `--live-login`, switch with `agx live use --id RUN --login NAME` | yes | the same home | Claude |
| to quit and continue the same conversation on another account's quota | `agx resume --id ID --login NAME` (or `--last`) | no: a new process resumes it | the home that stores it | any agent with a token login (Claude, Gemini CLI, pi) |
| the conversation to live in the other account's home from now on (its settings, its history list) | `agx conversation move --dir DIR --to-profile NAME`, then `agx resume` | no | the other home | Claude |
| a clean start on the other account | `agx run --profile NAME` (or `--login NAME`) | no | the other home | all |

## Live login vs restart with `--login`

| | `--live-login` | restart with `agx resume --login` |
|---|---|---|
| decide | before you start: the session must be launched with it | any time after |
| switch | `agx live use --id RUN --login alice` from any terminal; applies at Claude's next key refresh (about 10 seconds) | quit the agent, run `agx resume --id ID --login alice` |
| context | kept: same process, same conversation | kept on disk: the conversation is reloaded by the agent |
| go back | `agx live use --id RUN --start-login` | resume again with the first login |
| needs | a stored token login (`oauth-token` or `api-key`) to start with; nothing in the profile that outranks the helper | any login the agent takes |
| how | Claude gets an `apiKeyHelper` that runs agx's hidden `credential` command, which prints the token of the login chosen for that run | the token goes into the new process's environment, as for any run |
| status | preview: relies on Claude behaviour still being verified | stable |

Starting live shows the run id to switch with:

<!-- snippet: examples/choose-continue-elsewhere/session.txt#live -->
```sh
$ agx run --login carol --live-login --dry-run --quiet --json --field decisions.6
{"name":"note","value":"live login: switch with agx live use --id 20261114-101500-k3x9 --login NAME (refresh 10s)","source":"--live-login"}

$ agx live list
RUN  AGENT  PROFILE  STARTED ON  NEXT  IN USE  LIVENESS
```

`agx live list` shows the live runs whose agent is still running; it is empty here because a dry run starts nothing.

Resuming on another login keeps the home that stores the conversation and its folder, and only changes the credential:

<!-- snippet: examples/choose-continue-elsewhere/session.txt#resume-login -->
```sh
$ agx resume --id 3f2a9c1e --login carol --dry-run --quiet
agent    claude                                                from conversation 3f2a9c1e-0000-4000-8000-000000000001
profile  personal                                              from home ~/.claude (default)
home     ~/.claude                                             from conversation 3f2a9c1e-0000-4000-8000-000000000001 (stored there)
login    carol (not resolved: --dry-run)                       from --login
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 3f2a9c1e-0000-4000-8000-000000000001
run      ~/.agx/runs/20261114-101500-k3x9                      from AGX_RUN_ID=20261114-101500-k3x9
command  claude --resume 3f2a9c1e-0000-4000-8000-000000000001  from built by agentio
env      set AGX_RUN_ID=20261114-101500-k3x9                   from launch environment
         set AGX_RUN_AGENT=claude                              from launch environment
         set AGX_RUN_PROFILE=personal                          from launch environment
         set AGX_RUN_LOGIN=carol                               from launch environment
         set AGX_RUN_HOME=~/.claude                            from launch environment
         set CLAUDE_CODE_OAUTH_TOKEN=[redacted]                from launch environment
note     installed Claude Code version unknown; flags not checked
```

## If this, then that

- If you often run long Claude sessions near a limit, start them with `--live-login` (or set `"live_login": true` on the profile) so the switch is always one command away.
- If the session already runs without live login, quit and `agx resume --last --login NAME`: you lose the process, not the conversation.
- If you are moving a project to another account for good, `conversation move` it, then bind the folder (`agx project use`) so plain `agx run` and `agx resume` land there.
- If the agent is Codex, there is no token login: continue in the original home, or log that home into the other account with `codex login`.

## Pitfalls

- Live login is refused for a run that starts on the home login, on a one-off `--token-ref`, or on an `api-key-helper` login, because `agx credential` must be able to fetch the chosen login again:

<!-- snippet: examples/choose-continue-elsewhere/session.txt#live-refused -->
```sh
$ agx run --live-login --dry-run --quiet
agx: --live-login needs a stored login; this run starts on the home login (default (no login named))
  agx run --login NAME --live-login
[exit 125]

$ agx live use --id 20261114-101500-k3x9 --login alice
agx: no run 20261114-101500-k3x9
  agx live list
[exit 1]
```

- A profile whose `env`, `env_refs`, `--env` or `--setting env.*` sets a variable that outranks the helper (such as `ANTHROPIC_AUTH_TOKEN` for an API-provider profile) cannot be live; agx names the input to drop.
- `agx live use` on a run whose agent has ended is refused: nothing would read the choice. `agx clean --yes` removes ended runs.
- Switch only to a login that resolves (`agx login check --id NAME --no-probe`). Claude Code treats a failing key helper as fatal: three tries, then it exits, with no fallback to the home's own login. `agx credential` puts the reason in one line that Claude shows in `/status`; after that, resume on a good login.
- The switch is not instant: it applies at Claude's next key refresh or right after a rejected key, and the first request after a timed refresh may still use the old key ([when a switch applies](../guides/live-login.md#when-a-switch-applies)).
- Live login is Claude only; other agents report why they are not switchable.
- `conversation move` is Claude only (Codex indexes conversations in its own database with absolute paths). It backs up what it moves unless you pass `--no-backup`; try it with `--dry-run` first:

<!-- snippet: examples/choose-continue-elsewhere/session.txt#move -->
```sh
$ agx conversation move --dir ~/projects/client --from-profile personal --to-profile work --dry-run
would move 1 claude conversation(s) of ~/projects/client from ~/.claude to ~/.claude-work
  3f2a9c1e-0000-4000-8000-000000000001
dry run: nothing changed
```

## Related

- Guide: [Logins: live login](../logins.md#live-login-switch-a-running-sessions-account), [How agx decides](../how-agx-decides.md) (what resume decides from the conversation), [Live login](../guides/live-login.md), [Conversations: resume on another account's quota](../guides/conversations.md#step-5-resume-on-another-accounts-quota), [move to another account](../guides/conversations.md#step-6-move-a-folders-conversations-to-another-account).
- Commands: [`agx live`](../commands/live.md), [`agx resume`](../commands/launch.md#agx-resume), [`agx conversation move`](../commands/conversation.md#agx-conversation-move).
- Example: [choose-continue-elsewhere](../../examples/choose-continue-elsewhere/README.md), [acct-live-login](../../examples/acct-live-login/README.md), [convo-resume](../../examples/convo-resume/README.md), [convo-move-to-work](../../examples/convo-move-to-work/README.md).
- Other decisions: [Which login kind](login-kind.md), [Where to set the profile and login](where-to-set.md).
