# Auth Profiles

Atmos auth profiles are sibling `atmos.yaml` files under a `profiles/<name>/` directory that
deep-merge into the base `atmos.yaml` when `ATMOS_PROFILE=<name>` is set.

## Identity resolution rule

For a stack named `{org}-{tenant}-{region}-{stage}` (e.g., `e98d-gov-gbl-iam`), Atmos looks
up identities under the name `{tenant}-{stage}/<role>` (e.g., `gov-iam/gha-tf-plan`).

The skill generates one `identities:` block per `{tenant}-{stage}` pair discovered in the stack
hierarchy. The role suffix distinguishes the profile:

- `github-plan` profile → `<tenant>-<stage>/gha-tf-plan`
- `github-apply` profile → `<tenant>-<stage>/gha-tf-apply`

Identity names are not configurable in v1. The skill always emits this exact pattern.

## Deep-merge mechanics

When `ATMOS_PROFILE=github-plan` is set:

1. Atmos reads the base `atmos.yaml` as usual.
2. Atmos discovers `profiles/github-plan/atmos.yaml` by convention.
3. The profile's YAML is deep-merged **on top of** the base — profile keys override base keys,
   lists are replaced, not concatenated.
4. Identity references resolved later (during `atmos terraform plan`) pick up the profile's
   `identities:` table.

This is how the same stack authenticates differently in CI versus locally:

- CI sets `ATMOS_PROFILE=github-plan`; identities resolve to GitHub OIDC → IAM role.
- Local dev has no `ATMOS_PROFILE`; identities resolve to whatever the base `auth:` block
  declares (SSO, static credentials, etc.).

## Structure of a profile

```yaml
# profiles/github-plan/atmos.yaml

auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-1
      spec:
        audience: sts.amazonaws.com

  identities:
    gov-iam/gha-tf-plan:
      kind: aws/assume-role
      via:
        provider: github-oidc
      principal:
        assume_role: arn:aws:iam::662021896431:role/e98d-gov-gbl-iam-gha-tf-plan
    # ... one entry per {tenant}-{stage}
```

Key fields:

- `kind: github/oidc` — provider kind that exchanges the GitHub OIDC token for AWS creds via STS.
- `audience: sts.amazonaws.com` — matches the value the `github-oidc-provider` component
  configured on the identity provider.
- `kind: aws/assume-role` — identity kind that uses the provider's credentials to assume an IAM
  role in a specific account.

## Handling a repo without Atmos Auth

If the repo has no `auth:` block in its base `atmos.yaml`, the profile still works — the
profile provides the entire `auth:` config that CI needs. Local dev continues to use the
repo's pre-existing authentication (SSO, static keys, Okta federation, whatever).

The skill does **not** retrofit an `auth:` block into the user's primary `atmos.yaml`. That
would force a local-dev migration the user has not asked for.

A note in the generated `docs/atmos-pro.md` tells the user how to adopt Atmos Auth for local
dev later if they want to — but it's never a precondition for Atmos Pro.

## Handling a repo with Atmos Auth already configured

If `auth:` is present, the profile still works via deep merge — `providers.github-oidc` is
added, and `identities` are added under the profile's namespace. The user's existing
providers and identities stay intact when no profile is active.

If the user already has a provider named `github-oidc`, the skill adds the new provider under
a different name (`github-oidc-atmos-pro`) and updates the identities' `via.provider` reference
accordingly. This avoids clobbering existing config.

## Root-account safety

In the apply profile, the root account's identity points at the plan role's ARN (not the apply
role's). See [`iam-trust-model.md`](iam-trust-model.md) for rationale.

## Adding accounts later

To onboard a new account to an existing setup, the skill (or user) adds one identity block per
profile:

```yaml
# In profiles/github-plan/atmos.yaml
gov-new-account/gha-tf-plan:
  kind: aws/assume-role
  via:
    provider: github-oidc
  principal:
    assume_role: arn:aws:iam::123456789012:role/e98d-gov-gbl-new-account-gha-tf-plan
```

And the matching entry in `profiles/github-apply/atmos.yaml`. No other file changes are
required (the mixin and workflows stay identical).

Re-running the skill against a repo that already has Atmos Pro set up performs this delta
automatically.
