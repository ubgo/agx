## What and why

<!-- What this changes and why. Link the issue it closes. -->

Closes #

## Checklist

- [ ] `task check` passes locally (gofmt, vet, staticcheck, golangci-lint, race tests, Windows compile, cross-compile, docs gates)
- [ ] New behaviour has tests that use fakes, fixture homes or httptest: no real account, vendor binary, network or quota
- [ ] New or changed commands, flags and config keys are documented (`docs/commands/`, `docs/config.md`), with a runnable block or an example where output matters
- [ ] No vendor-specific logic in agx: agent knowledge lives in agentio
- [ ] No token can be printed, written into an agent home or put on argv by this change
- [ ] User-facing changes are listed under `[Unreleased]` in CHANGELOG.md
- [ ] Markdown is not hard-wrapped
