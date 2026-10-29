---
title: Allow approved plugin packages
description: Require statically inspected plugin packages to have an approved manifest name before installation or execution.
command: atmos plugin install oci://registry.example.com/team/oidc --as corporate --version 1.2.0
tags:
  - Admission
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Plugin policies
    url: /policies/plugins
---

# Allow approved plugin packages

Require statically inspected plugin packages to have an approved manifest name before installation or execution.

## Contract

| Field | Value |
|---|---|
| Input | `plugin` (`phase`, `name`) |
| Engines | `cel`, `starlark` |
| Action | `deny` |

## Configuration

Set `with.allowed_plugins` to exact `metadata.name` values from the plugin manifests you approve.
These are package names such as `acme/oidc`, not the project's aliases, OCI repository paths, or
capability names. Entries do not support wildcards or namespace-prefix matching.

The rule applies only in the `package` phase, after static inspection makes the manifest name
available. The earlier `source` phase carries an empty name and does not match this rule. Installation,
cached activation, publication, signing, and conformance testing must still pass the package check.
Approved local development packages pass this rule; a separate registry or signature policy can
still reject them. The registry library policy also denies explicit offline layout imports, even
when the layout claims an approved repository.

Use [approved plugin registries](https://atmos.tools/policies/library/allowed-plugin-registries) alongside
this policy to block unapproved destinations before network access. A package can claim an approved
name; pair these rules with [signature verification](https://atmos.tools/cli/commands/plugin/usage#require-trusted-publishers)
to authenticate its publisher. `source` is the declared repository, not proof of publisher identity.
Protect the project lockfile used to select cached releases. These policies cannot weaken signature
requirements.

## Use

Copy this manifest from a reviewed source checkout into a protected governing directory:

```shell
mkdir -p policy.d
cp policy-library/allowed-plugin-packages/policy.yaml policy.d/allowed-plugin-packages.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Protect the selected policy revision and enforcement workflow independently of the candidate PR.
The Atmos binary, environment, and policy directory must remain trusted; a user who controls that
runtime can bypass its checks. See [enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/allowed-plugin-packages).
The default manifest uses CEL; the [Starlark variant](https://atmos.tools/policies/library/allowed-plugin-packages/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View policy.yaml](https://atmos.tools/policies/library/allowed-plugin-packages/policy.yaml).

## Tests

[View inputs and expected decisions](https://atmos.tools/policies/library/allowed-plugin-packages/tests/cases.json).

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/allowed-plugin-packages$' -v
```

`not_applicable` means this rule did not match, not that every policy passed.
