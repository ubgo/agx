# Choosing

agx often offers more than one way to do a thing, on purpose: a flag for one run, an environment variable for one terminal, a binding for one folder, config for always. These pages help you pick. Each one starts with a short decision table, compares the options, shows them side by side with real output from a tested example, and ends with the traps.

| Decision | Read it when |
|---|---|
| [Which login kind](login-kind.md) | you add an account: home login, plan token from a reference or pasted, API key, key helper, live login |
| [A second account](second-account.md) | you want another account: a new home, a named login on the same home, or a second profile on one home |
| [Where to set the profile and login](where-to-set.md) | you choose between a flag, `AGX_LOGIN`, a folder binding, the profile and agent defaults |
| [Which way to pass something](passing-options.md) | you pass a model, option, setting, environment variable or raw flag to an agent |
| [Settings as a file or inline](settings-delivery.md) | you wonder whether `--settings-delivery inline` is worth it |
| [Usage: live, cached or probed](usage-live-or-cached.md) | you read quota by hand, in a status bar, or per stored login |
| [Continue on another account](continue-on-another-account.md) | a session hits its limit: live login, resume on another login, move the conversation |
| [Built-in or config-defined agent](custom-agent.md) | you want agx to run a CLI it does not know, or a flag it does not model |
| [With or without the shell layer](shell-layer.md) | you decide whether to load `agx shell-init` |

The examples behind these pages are the `choose-*` folders in [examples](../../examples/README.md); each runs in the test suite on a fake machine.
