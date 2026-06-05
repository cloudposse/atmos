# SOPS Secrets Example

This example demonstrates Atmos **declarative secrets management** end to end using a Track 2
**SOPS** backend (age encryption). It proves the full lifecycle with **no cloud credentials** —
everything runs locally against a git-committed, encrypted file.

> **Example only.** `secrets/keys.txt` is a throwaway age key committed so the demo is
> self-contained. **Never commit a real age private key.** In a real project, distribute the key
> out of band and reference it via `SOPS_AGE_KEY_FILE`.

## Prerequisites

- [`sops`](https://github.com/getsops/sops) and [`age`](https://github.com/FiloSottile/age) on your `PATH`
  (`brew install sops age`).
- The age key file exported so `sops` can decrypt:

  ```shell
  export SOPS_AGE_KEY_FILE="$PWD/secrets/keys.txt"
  export ATMOS_CLI_CONFIG_PATH="$PWD"
  ```

## What's configured

- **`stacks/deploy/dev.yaml`** defines the SOPS provider **globally for the stack** (not in
  `atmos.yaml`, and not under a component) — it merges into every component in the stack:

  ```yaml
  secrets:
    providers:
      dev-sops:
        kind: sops/age
        spec:
          file: secrets/dev.enc.yaml
  ```

- **`stacks/catalog/api.yaml`** declares two secrets and uses them via `!secret`:

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

Run the bundled `atmos test` custom command (defined in `.atmos.d/test.yaml`), or follow along below:

```shell
atmos test
```

It sets values, reads them back, lists/validates status, and proves the masking matrix — then
resets the encrypted file to its clean committed state.

### 1. Inspect with NO key — masked, no retrieval, no credentials

```shell
$ env -u SOPS_AGE_KEY_FILE atmos describe component api --stack dev | grep -E 'datadog_api_key:|redis_url:'
  datadog_api_key: <MASKED>
  redis_url: <MASKED>
```

`describe` resolves `!secret` to `<MASKED>` **without decrypting the SOPS file** — you can review a
stack's shape with no access to the secret material.

### 2. Set values (encrypted in place)

```shell
$ atmos secret set DATADOG_API_KEY=dd-abc123secret --stack dev --component api
✓ Set secret DATADOG_API_KEY for component api in stack dev

$ grep DATADOG_API_KEY secrets/dev.enc.yaml
DATADOG_API_KEY: ENC[AES256_GCM,data:...,type:str]   # encrypted at rest
```

### 3. List status

```shell
$ atmos secret list --stack dev --component api
STACK  COMPONENT  SECRET           PROVIDER       STATUS
dev    api        DATADOG_API_KEY  sops:dev-sops  initialized
dev    api        REDIS_URL        sops:dev-sops  initialized
```

### 4. Reveal real values (requires the key)

```shell
$ ATMOS_MASK=false atmos secret get DATADOG_API_KEY --stack dev --component api
dd-abc123secret

# With the key, describe reveals the resolved values:
$ ATMOS_MASK=false atmos describe component api --stack dev | grep -E 'datadog_api_key:|redis_url:'
  datadog_api_key: dd-abc123secret
  redis_url: redis://prod:6379

# Without the key, revealing FAILS — retrieval genuinely needs to decrypt:
$ env -u SOPS_AGE_KEY_FILE ATMOS_MASK=false atmos describe component api --stack dev
Error: secret is not initialized in its backend: "DATADOG_API_KEY": sops ...
```

### 5. Validate (CI gate)

```shell
$ atmos secret validate --stack dev --component api
✓ All required secrets are initialized for component api in stack dev
$ echo $?
0
```

## The masking matrix

| Command class | masking on (default) | masking off (`ATMOS_MASK=false`) |
|---------------|----------------------|----------------------------------|
| **Inspection** (`describe`, `list`) | `!secret` → `<MASKED>`, **no retrieval, no key** | decrypt + reveal (needs key) |
| **Value-producing** (`secret get`) | always retrieves; output redacted | retrieves + reveals |

> **Note:** use the `ATMOS_MASK` environment variable to toggle masking for these flows.
