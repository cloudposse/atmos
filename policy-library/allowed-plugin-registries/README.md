---
title: Allow approved plugin registries
description: Require plugin downloads, cached activation, and registry operations to use an approved OCI registry.
command: atmos plugin install oci://registry.example.com/team/oidc --as corporate --version 1.2.0
tags:
  - Admission
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Plugin policies
    url: /policies/plugins
---

# Allow approved plugin registries

Require plugin downloads, cached activation, and registry operations to use an approved OCI registry.

## Contract

| Field | Value |
|---|---|
| Input | `plugin` (`origin`, `registry`) |
| Engines | `cel`, `starlark` |
| Action | `deny` |

## Configuration

Set `with.allowed_registries` to the exact registry authorities your company permits, including a
port when required. Entries are host names such as `registry.example.com` or
`registry.example.com:5000`, without `oci://`, a repository path, or a wildcard. A lookalike host or
different port does not match.

The rule applies to both `source` and `package` phases and every plugin operation. It checks the
actual package repository as well as a discovery catalog's repository, so an approved catalog cannot
redirect installation to an unapproved registry. Cached activation is checked again against the
current policy without requiring a new download. Protect the project lockfile that selects those
cached bytes as part of the governing workflow.

The rule also denies `origin: development` and `origin: layout`. Local development and conformance
testing have no registry. Explicit `--from-layout` imports can claim an approved repository without
contacting it, so a matching registry alone is insufficient. Offline layouts require an explicit
governing policy exception.

If your governing policy permits local development while still rejecting layouts, use
`input.origin == 'layout' || (input.origin != 'development' && !(input.registry in params.allowed_registries))`
(or the equivalent Starlark). Do not add an empty registry entry accidentally.

Pair this with [approved plugin packages](https://atmos.tools/policies/library/allowed-plugin-packages)
to restrict package names, and with [signature verification](https://atmos.tools/cli/commands/plugin/usage#require-trusted-publishers)
to authenticate release publishers. `source` records a declared repository, not a cryptographically
authenticated publisher identity. Registry membership alone does not authenticate the code, and
origin exceptions cannot weaken signature requirements.

## Use

Copy this manifest from a reviewed source checkout into a protected governing directory:

```shell
mkdir -p policy.d
cp policy-library/allowed-plugin-registries/policy.yaml policy.d/allowed-plugin-registries.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Protect the selected policy revision and enforcement workflow independently of the candidate PR.
The Atmos binary, environment, and policy directory must remain trusted; a user who controls that
runtime can bypass its checks. See [enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/allowed-plugin-registries).
The default manifest uses CEL; the [Starlark variant](https://atmos.tools/policies/library/allowed-plugin-registries/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View policy.yaml](https://atmos.tools/policies/library/allowed-plugin-registries/policy.yaml).

## Tests

[View inputs and expected decisions](https://atmos.tools/policies/library/allowed-plugin-registries/tests/cases.json).

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/allowed-plugin-registries$' -v
```

`not_applicable` means this rule did not match, not that every policy passed.
