# Snippet fixture

Test data for TestSnippetFixture: every block below must equal its example file region.

<!-- snippet: examples/quickstart/session.txt#init -->
```sh
$ agx config init --default-agent claude --discovered
wrote ~/.agx/config.jsonc (2 profiles)

$ agx profile list
PROFILE    PROVIDER  HOME            ACCOUNT  STATE  LOGIN       BILLING
*personal  claude    ~/.claude                none   home login  plan
work       claude    ~/.claude-work           none   home login  plan
```

A refusal, inside a list item (the fence is indented):

- Conflicting flags:

  <!-- snippet: examples/exit-codes/session.txt#conflict -->
  ```sh
  $ agx run --login carol --home-login
  agx: --login and --home-login both choose the login; pick one
  [exit 125]
  ```

<!-- snippet: examples/exit-codes/config.jsonc#profiles -->
```jsonc
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true }
  },
```

<!-- snippet: examples/quickstart/home/token.txt -->
```text
fake-token-pasted
```
