---
title: Deny production authentication
description: Deny any production auth provider or identity, including intermediate identities and cached credentials.
command: atmos auth login --identity prod-admin
tags:
  - Admission
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Execution admission
    url: /policies/admission
---

# Deny production authentication

Deny any production auth provider or identity, including intermediate identities and cached credentials.

## Contract

| Field | Value |
|---|---|
| Input | `auth` |
| Engines | `cel`, `starlark` |
| Action | `deny` |
| Selects when | A production name anywhere in the resolved provider/identity chain. |
| Required data | Captured admission metadata; no cloud credentials required for evaluation. |

## Configuration

This policy always applies: there is no `when` condition or agent filter. It denies matching authentication locally and in CI, for humans and agents alike.

The rule treats `prod` or `production` as a case-insensitive name segment separated by `-`, `_`, or `/`.
Examples: `prod-admin`, `aws-prod`, `team/production/admin`, and `PROD_SSO`.
Add exact names that do not follow that convention to `with.production_names`:

```yaml
with:
  production_names: [release-admin, privileged-sso]
```

Atmos supplies the resolved target as `input.identity`, the root provider as `input.provider`,
and every identity in the chain as `input.identities`. A development target is still denied
if its chain crosses a production provider or intermediate identity. A standalone identity has
an empty provider; direct provider authentication has an empty identity and identity list.
No credentials, tokens, or provider configuration are exposed to the rule.

The check runs before provider/identity authentication, credential helpers, and process/keyring/file
cache use through the auth manager. It applies to `auth login`, `auth env`, `auth shell`,
`auth exec`, credential-process exports, and component commands that use Atmos authentication.
Cached credentials do not exempt an identity from the policy. Logout remains available.

Names are configuration, not proof of which account a credential reaches. Review the naming convention
and protect policy/auth configuration. This policy does not revoke previously issued credentials,
block ambient credentials used outside Atmos auth, or prevent direct cloud CLI calls.

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/auth-deny-production).
The default manifest uses CEL; the [Starlark variant](https://atmos.tools/policies/library/auth-deny-production/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View and copy policy.yaml](https://atmos.tools/policies/library/auth-deny-production/policy.yaml)

## Use

From a reviewed, pinned checkout of this repository:

```shell
mkdir -p policy.d
cp policy-library/auth-deny-production/policy.yaml policy.d/auth-deny-production.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

A denial exits with code `77` before the protected operation. See
[enforcement and trust requirements](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test cases](https://atmos.tools/policies/library/auth-deny-production/tests/cases.json).
Run the actual distributed manifests in both engines:

```shell
go test ./pkg/policy -run TestPolicyCatalog/auth-deny-production -v
```
