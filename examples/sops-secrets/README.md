---
title: SOPS Secrets
tags: [Stacks]
description: >-
  Declare, encrypt, and read secrets with a SOPS backend (age encryption) —
  the full lifecycle against a git-committed encrypted file, no cloud
  credentials.
cast:
  file: /casts/examples/sops-secrets/secret-lifecycle.cast
  title: atmos sops secrets lifecycle
---

# SOPS Secrets Example

Atmos **declarative secrets management** end to end with a **SOPS** backend (age encryption) — the
full lifecycle with **no cloud credentials**, against a git-committed, encrypted file.

> **Example only.** `secrets/keys.txt` is a throwaway age key committed so the demo is
> self-contained. **Never commit a real age private key** — distribute it out of band and
> reference it via `SOPS_AGE_KEY_FILE`.

**No external tools required.** Atmos encrypts and decrypts in-process via the getsops/sops Go SDK —
there's no `sops` or `age` binary to install. The age key is declared right in the stack, so the
example works out of the box.

## Give it a spin

Run the bundled `atmos test` command and watch the whole lifecycle — it sets values, lists and
validates status, deploys the `api` component that consumes the secrets, reads the output back,
shows masked-without-credentials inspection, then resets the encrypted file to its clean committed
state:

```shell
atmos test
```

Two cases worth watching:

- **Inspect with masking on** — `!secret` resolves to `<MASKED>` with no retrieval and no decryption,
  so you can review the stack with no key at all.
- **Reveal with the key removed** — decryption fails, confirming the value is genuinely encrypted at rest.

## Learn more

- `stacks/deploy/dev.yaml` — the SOPS provider, configured globally for the stack.
- `stacks/catalog/api.yaml` — the `!secret` declarations that consume it.
- [Secrets configuration guide](https://atmos.tools/cli/configuration/secrets) — the full reference,
  including how to keep the age key in your OS keychain instead of a file.
