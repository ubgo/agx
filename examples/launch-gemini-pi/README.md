# launch-gemini-pi

What passes to Gemini CLI and pi: `--model`, `--add-dir`, `--prompt` and `--autonomy` mapped to Gemini's own flags, a Gemini API-key login, pi's read-only tools allowlist and its "never asks" warning, the refusal of `--setting` (neither has a settings flag), and raw args after `--`.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-gemini-pi$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `gemini` and `pi` on their default homes, and the Gemini login `gem`. Region: `profiles`. |
| [`session.txt`](session.txt) | Regions: `gemini`, `pi`, `no-setting`, `raw`. |

Explained in [Passing anything to an agent](../../docs/launch-options.md#recipe-10-gemini-pi-and-custom-agents).
