# 1Password secrets example

Declarative secrets backed by [1Password](https://developer.1password.com/), resolved with the
`!secret` YAML function and the `atmos secret` CLI.

Unlike the [`sops-secrets`](../sops-secrets) example, this one is **not run by `atmos test`** in CI:
resolving values requires real 1Password credentials and a vault you control. The files here show
the configuration shape; the steps below let you try it against your own account.

## How it works

- **Store** (`atmos.yaml`): a `type: onepassword` store named `op`. `secret: true` is implied — a
  1Password store is always a secret backend (resolved via `!secret`, never `!store`). It supports
  full CRUD: `atmos secret set` writes the value to the field the reference points to (creating the
  item, as an **API Credential**, if it does not exist), and `delete` removes it.
- **Declarations** (`stacks/catalog/api.yaml`): each secret carries an `op://vault/item/field`
  [secret reference](https://developer.1password.com/docs/cli/secret-reference-syntax/) via the
  `reference` field. References support Go templating — `op://{{ .atmos_stack }}/postgres/password`
  resolves a different item per stack.
- **Usage**: `datadog_api_key: !secret DATADOG_API_KEY`.

## Authentication

No `op` CLI is required. The store auto-selects a backend:

| Backend | Set these | Typical use |
|---|---|---|
| Service Account | `OP_SERVICE_ACCOUNT_TOKEN` | local dev |
| Connect | `OP_CONNECT_HOST` + `OP_CONNECT_TOKEN` | CI / cloud |

Force one with `options.mode: service-account` or `options.mode: connect`.

> Service accounts cannot access your built-in Private/Personal/Employee vault — use a shared
> named vault.

## Try it

1. Create the referenced items in a vault you own (or edit the `reference` values to point at
   existing items), e.g. an item `Datadog` with a field `api_key` in a `Shared` vault.
2. Export a credential:
   ```shell
   export OP_SERVICE_ACCOUNT_TOKEN="ops_..."
   ```
3. Resolve a secret (masked by default):
   ```shell
   atmos secret get DATADOG_API_KEY --stack dev --component api
   atmos secret get DATADOG_API_KEY --stack dev --component api --mask=false
   ```
4. Write and remove a secret (creates/updates/deletes the referenced 1Password item):
   ```shell
   atmos secret set DB_PASSWORD=s3cr3t --stack dev --component api
   atmos secret delete DB_PASSWORD --stack dev --component api
   ```
5. Check declared-secret status and validation:
   ```shell
   atmos secret list --stack dev --component api
   atmos secret validate --stack dev --component api
   ```
