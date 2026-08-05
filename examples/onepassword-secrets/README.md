---
title: 1Password Secrets
tags: [Stacks]
cast:
  file: /casts/examples/onepassword-secrets/connect-mock.cast
  title: atmos 1Password Connect mock
---

# 1Password secrets example

Declarative secrets backed by [1Password](https://developer.1password.com/), resolved with the
`!secret` YAML function and the `atmos secret` CLI.

This example runs against a local [Mockoon](https://mockoon.com/) 1Password Connect mock through
Atmos emulator components, so it is testable without a 1Password account.

## How it works

- **Emulator** (`stacks/catalog/emulator/onepassword-connect.yaml`): a `mockoon/1password-connect`
  emulator that serves a local 1Password Connect mock data file.
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

```shell
cd examples/onepassword-secrets

# Start the local 1Password Connect mock
atmos emulator up onepassword-connect --stack dev --ephemeral

# Resolve a mocked secret
atmos secret get DATADOG_API_KEY --stack dev --component api --mask=false

# Check declared-secret status and validation
atmos secret list --stack dev --component api
atmos secret validate --stack dev --component api

# Write and remove a mocked secret
atmos secret set DB_PASSWORD=rotated-password --stack dev --component api
atmos secret delete DB_PASSWORD --stack dev --component api --force

# Stop the emulator
atmos emulator down onepassword-connect --stack dev
```
