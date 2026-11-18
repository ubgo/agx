# Live login

Switch the login of a running Claude session. Start the session with `--live-login` (or a profile with `"live_login": true`); [Live login](../guides/live-login.md) is the guide: how it works, the whole flow and its limits. The examples below come from [`examples/acct-live-login`](../../examples/acct-live-login/README.md), where a seeded run folder stands in for a session running in another terminal.

A raw `--settings` of your own is merged with the settings live login adds, so one `--settings` reaches Claude; a raw value that sets `apiKeyHelper` itself is refused at launch: `agx: raw --settings apiKeyHelper and --live-login both set the credential; pick one` (exit 125). See [the live-login pitfalls](../guides/live-login.md#pitfalls).

## agx live list

Running sessions started with live login. Runs whose agent has ended are left out (`agx clean` removes them); a run whose liveness cannot be read is listed as `unknown`. Cost R.

| Flag | Meaning |
|---|---|
| `--agent` | only this agent |

<!-- runnable -->
```sh
$ agx live list
RUN  AGENT  PROFILE  STARTED ON  NEXT  IN USE  LIVENESS
```

Each row is one run started with `--live-login`: the login it started on, the login it gets at its next refresh, the login in use, and whether its process is still alive:

<!-- snippet: examples/acct-live-login/session.txt#list -->
```sh
$ agx live list --agent claude
RUN                   AGENT   PROFILE  STARTED ON  NEXT   IN USE  LIVENESS
20261114-101500-k3x9  claude  work     carol       carol  —       unknown

$ agx live show --id 20261114-101500-k3x9
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      —
next        carol
in use      —
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

## agx live show

One run's live state, ended or not: the login it started with, the login chosen, the login it gets next, and the login in use at the agent's last refresh. Never a token. Cost R.

| Flag | Meaning |
|---|---|
| `--id` | the run id |

```sh
agx live show --id 20261114-101500-k3x9
```

Pitfalls: an id that is not a run is exit 1, a malformed id exit 2, and a run started without `--live-login` is refused (exit 1):

<!-- runnable: exit=1 -->
```sh
$ agx live show --id 20261114-101500-k3x9
agx: no run 20261114-101500-k3x9
  agx live list
```

## agx live use

Choose the login of a live run. The agent picks it up at its next key refresh (about 10 seconds) or at once after a rejected key. Cost W.

| Flag | Meaning |
|---|---|
| `--id` | the run id |
| `--login` | the login to switch to |
| `--start-login` | go back to the login the run started with |

Try it:

<!-- snippet: examples/acct-live-login/session.txt#use -->
```sh
$ agx live use --id 20261114-101500-k3x9 --login alice
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      alice
next        alice
in use      —
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

<!-- snippet: examples/acct-live-login/session.txt#after -->
```sh
$ agx live show --id 20261114-101500-k3x9
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      alice
next        alice
in use      alice
liveness    unknown (process info unreadable: run.jsonc records no process id)

$ agx live use --id 20261114-101500-k3x9 --start-login
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      —
next        carol
in use      alice
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

Pitfalls:

- Give exactly one of `--login` and `--start-login` (exit 2).
- The login must exist, serve the run's agent and be a token login (a key-helper login is refused).
- A run whose agent has ended is refused (exit 1): nothing would read the choice.
- The switch applies at the next timed refresh (agx sets ten seconds) or right after Claude gets a rejected key, and the first request after a timed refresh may still use the old key; `in use` shows when the helper ran. See [when a switch applies](../guides/live-login.md#when-a-switch-applies).
- Check the login first (`agx login check --id NAME --no-probe`): if it does not resolve, `agx credential` fails with one line, and Claude Code stops after three tries instead of falling back to another login ([a helper that fails stops the session](../guides/live-login.md#a-helper-that-fails-stops-the-session)).
