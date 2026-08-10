---
title: Authentication
tags: [Stacks]
description: >-
  Structure an Atmos project with an `auth` section in atmos.yaml — list
  providers and identities without contacting real cloud APIs.
cast:
  file: /casts/examples/demo-auth/auth-list.cast
  title: atmos auth identities
---

# Demo: Auth

This demo showcases how to structure an Atmos project with an `auth` section in `atmos.yaml`.

It intentionally only validates stack manifests (no Terraform applies) to avoid requiring real cloud credentials in CI.

## What You'll See

- Multiple [providers](https://atmos.tools/cli/commands/auth/usage) (`github/oidc`, `aws/saml`, `aws/iam-identity-center`) and the identities that assume roles through them
- Plain `tags: [...]` on providers and identities — used to filter/select them with `--tags`, without contacting real cloud APIs
- `atmos auth list --tags` narrowing the tree to a subset of providers/identities
- `atmos auth login --tags` auto-selecting the single matching identity (or prompting when more than one matches)
- `atmos auth logout --tags` logging out of every provider that matches, in one command

## Try It

```shell
cd examples/demo-auth

# See the full provider/identity tree
atmos auth list

# Narrow it to only the enterprise-tagged providers/identities (any-match)
atmos auth list --tags enterprise

# --tags on login auto-selects when exactly one identity matches (here, "oidc").
# It then fails with a controlled "GitHub OIDC ... only available in GitHub
# Actions" error, since this stub provider has no real OIDC token to exchange
# outside of CI — confirming the right identity was selected without needing
# real cloud credentials.
atmos auth login --tags ci

# --tags on logout logs out of every matching provider (here, both SAML and SSO)
atmos auth logout --tags enterprise
```

> [!TIP]
> `--tags` composes with the existing `--providers`/`--identities` filters on
> `atmos auth list`. If zero identities match, `atmos auth login --tags` errors
> with the list of tags that do exist, so you can correct a typo quickly.

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | `auth.providers`/`auth.identities` with `tags: [...]` on each |

## Learn More

See [the Atmos docs](https://atmos.tools/cli/commands/auth/usage) for more information.
