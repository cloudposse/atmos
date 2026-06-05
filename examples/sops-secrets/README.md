# SOPS Secrets Example

This example demonstrates Atmos **declarative secrets management** end to end using a **SOPS**
backend (age encryption) — the full lifecycle with **no cloud credentials**, against a
git-committed, encrypted file.

> **Example only.** `secrets/keys.txt` is a throwaway age key committed so the demo is
> self-contained. **Never commit a real age private key** — distribute it out of band and
> reference it via `SOPS_AGE_KEY_FILE`.

## Prerequisites

- [`sops`](https://github.com/getsops/sops) and [`age`](https://github.com/FiloSottile/age) on
  your `PATH` (`brew install sops age`).
- Export the key file and config path so `sops` can decrypt:

  ```shell
  export SOPS_AGE_KEY_FILE="$PWD/secrets/keys.txt"
  export ATMOS_CLI_CONFIG_PATH="$PWD"
  ```

## What's configured

`stacks/deploy/dev.yaml` defines the SOPS provider **globally for the stack** (not in `atmos.yaml`,
and not under a component) — it merges into every component in the stack:

```yaml
secrets:
  providers:
    dev-sops:
      kind: sops/age
      spec:
        file: secrets/dev.enc.yaml
```

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
# Inspect with NO key — masked, no retrieval, no credentials:
$ env -u SOPS_AGE_KEY_FILE atmos describe component api --stack dev | grep -E 'datadog_api_key:|redis_url:'
  datadog_api_key: <MASKED>
  redis_url: <MASKED>

# Reveal without the key FAILS — retrieval genuinely decrypts:
$ env -u SOPS_AGE_KEY_FILE ATMOS_MASK=false atmos describe component api --stack dev
Error: secret is not initialized in its backend: "DATADOG_API_KEY": sops ...
```

## The masking matrix

| Command class | masking on (default) | masking off (`--mask=false`) |
|---------------|----------------------|------------------------------|
| **Inspection** (`describe`, `list`) | `!secret` → `<MASKED>`, **no retrieval, no key** | decrypt + reveal (needs key) |
| **Value-producing** (`secret get`) | always retrieves; output redacted | retrieves + reveals |

> **Note:** toggle masking with the global `--mask` flag (`--mask=false`) or the `ATMOS_MASK`
> environment variable.
