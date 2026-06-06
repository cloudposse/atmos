# SOPS Secrets Example

This example demonstrates Atmos **declarative secrets management** end to end using a **SOPS**
backend (age encryption) — the full lifecycle with **no cloud credentials**, against a
git-committed, encrypted file.

> **Example only.** `secrets/keys.txt` is a throwaway age key committed so the demo is
> self-contained. **Never commit a real age private key** — distribute it out of band and
> reference it via `SOPS_AGE_KEY_FILE`.

## Prerequisites

**No external tools.** Atmos encrypts/decrypts in-process via the getsops/sops Go SDK — no `sops`
or `age` binary is required. This example declares the age key in the stack via `age_key_file`, so
it works **out of the box** — no environment variable to export.

> Prefer the env var? It still works as a fallback: `export SOPS_AGE_KEY_FILE="$PWD/secrets/keys.txt"`.

## What's configured

`stacks/deploy/dev.yaml` defines the SOPS provider **globally for the stack** (not in `atmos.yaml`,
and not under a component) — it merges into every component in the stack. `age_key_file` points at
the (throwaway) private key so decryption needs no env var (it supports `~` and `$ENV` expansion):

```yaml
secrets:
  providers:
    dev-sops:
      kind: sops/age
      spec:
        file: secrets/dev.enc.yaml
        age_key_file: secrets/keys.txt
```

> **Prefer the OS keychain?** Configure a `keychain` store and point the key at it instead of a file —
> then `atmos secret keygen dev-sops` writes the private key into the keychain and decryption reads it
> back, with nothing on disk or in the environment:
>
> ```yaml
> stores:
>   keychain:
>     type: keychain
> secrets:
>   providers:
>     dev-sops:
>       kind: sops/age
>       spec:
>         file: secrets/dev.enc.yaml
>         age_key:
>           store: keychain     # "file" (default) | a configured store name
> ```

`stacks/catalog/api.yaml` declares two secrets and consumes them via `!secret`:

```yaml
components:
  terraform:
    api:
      secrets:
        vars:
          DATADOG_API_KEY:
            sops: dev-sops
            required: true
          REDIS_URL:
            sops: dev-sops
      vars:
        datadog_api_key: !secret DATADOG_API_KEY
        redis_url: !secret REDIS_URL | default "redis://localhost:6379"
```

## End-to-end proof

Run the bundled `atmos test` custom command (defined in `.atmos.d/test.yaml`). It sets values,
reads them back, lists/validates status, proves the masking matrix, then resets the encrypted file
to its clean committed state:

```shell
atmos test
```

The two telling cases it exercises:

```shell
# Inspect with masking on — !secret resolves to <MASKED>, no retrieval, no decryption:
$ atmos describe component api --stack dev | grep -E 'datadog_api_key:|redis_url:'
  datadog_api_key: <MASKED>
  redis_url: <MASKED>

# Reveal with the key removed FAILS — retrieval genuinely decrypts:
$ mv secrets/keys.txt /tmp/keys.txt   # take the age key away
$ ATMOS_MASK=false atmos describe component api --stack dev
Error: failed to decrypt SOPS file ...
$ mv /tmp/keys.txt secrets/keys.txt   # put it back
```

## The masking matrix

| Command class | masking on (default) | masking off (`--mask=false`) |
|---------------|----------------------|------------------------------|
| **Inspection** (`describe`, `list`) | `!secret` → `<MASKED>`, **no retrieval, no key** | decrypt + reveal (needs key) |
| **Value-producing** (`secret get`) | always retrieves; output redacted | retrieves + reveals |

> **Note:** toggle masking with the global `--mask` flag (`--mask=false`) or the `ATMOS_MASK`
> environment variable.
