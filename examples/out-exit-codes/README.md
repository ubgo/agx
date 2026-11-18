# Every exit code

Each exit code agx returns, made to happen: for a launching command 0, the agent's own code, 125 and 127; for any other command 1, 2, 3 (partial) and 4 (unsupported), and the matching `error.code`. The [exit-codes](../exit-codes/README.md) example shows more refusals.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/out-exit-codes$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Claude, Codex and Gemini profiles, and a custom agent `nocli` whose binary is not installed. |
| [`session.txt`](session.txt) | Regions: `launch-0`, `launch-agent-code` (the fake agent exits 3, so agx does), `launch-125`, `launch-127`, `other-1`, `other-2`, `other-3` (`usage`: two homes answer, two fail), `other-4`, `other-1-usage` (every home failed), `codes`. |
| `home/.nocli/config.json` | Makes `~/.nocli` exist, so the launch gets as far as looking for the binary. |

Not shown, because a script cannot cause them: 126 (the binary was found but the system cannot run it, such as a wrong architecture) and 130 (Ctrl-C).

Guide: [Scripting with agx](../../docs/guides/scripting.md#step-4-branch-on-the-exit-code). Reference: [exit codes](../../docs/output.md#exit-codes).
