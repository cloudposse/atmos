# AWS Stores and Secrets Auth: Hook Resolver Injection and Full-Circle Examples

**Date:** 2026-06-17

## Problem

AWS-backed stores were not usable end to end in the workflows they were meant to
support.

Hook writes to AWS SSM Parameter Store could fail even when the Terraform
component itself had a valid AWS identity:

```text
failed to set parameter 'demoapp/dev/pen/global/global/a_secret':
operation error SSM: PutParameter, get identity: get credentials:
failed to refresh cached credentials, no EC2 IMDS role found
```

Adding the same identity explicitly to the store changed the failure instead of
fixing it:

```text
store identity is configured but auth resolver is not set
store requires identity "dev-pen-access" but no auth resolver was injected
```

At the same time, the store configuration UX was inconsistent:

- AWS stores still used the legacy hyphenated `type` form in docs and examples,
  while auth uses slash-style `kind` notation.
- AWS Secrets Manager lacked parity with SSM for explicit store identity and
  custom endpoint handling.
- Stores marked `secret: true` could still be read through cleartext store
  paths in some code paths.
- There were no runnable full-circle AWS examples proving hook writes, store
  reads, declared secrets, scopes, and Terraform consumption together.

## Root Cause

Terraform command execution creates and authenticates an auth manager for the
component, then persists it in `ConfigAndStacksInfo`. Hook execution loaded a
fresh Atmos configuration for the hook context, but did not inject an auth
resolver into `atmosConfig.Stores` before running store hooks. Stores with an
explicit identity therefore had no resolver. Stores without an explicit
identity continued using ambient AWS SDK credential resolution; changing that
inheritance behavior is a separate compatibility decision.

SSM and Secrets Manager clients were also initialized too early for the
explicit-identity path. A store could construct its default AWS client before
the hook or component context had a chance to inject the auth resolver.

Finally, the secret-store access rule was enforced by convention and
documentation more than by every read path. `!store`, `!store.get`, and
`atmos.Store` needed to reject `secret: true` stores consistently so `!secret`
and `atmos secret` remain the only secret access paths.

Floci validation also exposed two adjacent auth-path regressions:

- `auth.identities.*.credentials.aws.resolver.url: !env NAME` was not being
  evaluated because the raw-YAML identity key preservation pass bypassed Atmos
  YAML function processing.
- `describe component` created an auth manager for YAML functions but then
  resolved component config through a fresh store registry without the auth
  resolver, so `!store` reads against an explicit-identity ASM store still
  failed resolver setup.

## Fix

- Hook execution now injects an auth resolver into `atmosConfig.Stores` using
  the persisted Terraform auth manager before `hooks.RunAll`.
- Hook stores with an explicit `identity` can resolve that identity through
  Atmos auth. Stores without `identity` keep the existing ambient/default SDK
  credential behavior in this PR.
- SSM and AWS Secrets Manager stores now defer AWS client creation until first
  use, leaving time for auth resolver injection.
- AWS identity resolver endpoints are carried into SSM and Secrets Manager
  store SDK clients, so Floci or custom AWS-compatible endpoints are used for
  the actual store API calls, not only during authentication.
- AWS SSM and Secrets Manager also accept store-level `endpoint` or
  `endpoint_url` options. A store-level endpoint takes precedence over an
  endpoint inherited from the AWS identity resolver.
- Azure Key Vault accepts `endpoint` as an alias for `vault_url` and exposes
  `disable_challenge_resource_verification` for Key Vault-compatible local
  endpoints whose auth challenge resource does not match the endpoint host.
  Local HTTP endpoints can opt into `endpoint_insecure` and
  `without_authentication`.
- Google Secret Manager accepts `endpoint` / `endpoint_url`, plus
  `endpoint_insecure` for plaintext local gRPC endpoints and
  `without_authentication` for emulators that do not validate Google
  credentials.
- Raw-YAML auth identity parsing now evaluates Atmos YAML functions such as
  `!env`, preserving dotted identity names without losing dynamic values.
- `describe component` and `atmos secret` now inject store auth resolvers
  before resolving `!store` or `!secret` values, without assigning a default
  identity to stores that omitted `identity`.
- `kind: aws/ssm` and `kind: aws/asm` are the canonical store selectors.
  Legacy `type: aws-ssm-parameter-store` and `type: aws-secrets-manager`
  remain supported as aliases.
- `!store`, `!store.get`, and `atmos.Store` now return an error when pointed at
  a store marked `secret: true`.

## Examples

Two AWS gists were added:

- `gists/aws-store-hooks` exercises machine-written Terraform outputs:
  hook writes to SSM and Secrets Manager, same-stack and cross-stack reads,
  `!store`, `!store.get`, `atmos.Store`, `query`, defaults, AWS CLI
  verification, and cleanup.
- `gists/aws-secrets` exercises declared secrets without hooks:
  `secret: true` SSM and Secrets Manager stores, `atmos secret
  set/get/list/validate/delete`, `scope: instance`, `scope: stack`,
  `scope: global`, `!secret`, masked describe behavior, and Terraform
  consumption through `env`.

Both gists run Terraform through OpenTofu (`command: tofu`) and declare
`opentofu` as a component tool dependency so Atmos can install it automatically
through the toolchain.

## Floci Coverage

Opt-in Floci integration tests were added under `tests/` with dedicated
fixtures in `tests/fixtures/scenarios/aws-store-hooks-floci`,
`tests/fixtures/scenarios/aws-secrets-floci`,
`tests/fixtures/scenarios/gcp-secrets-floci`, and
`tests/fixtures/scenarios/azure-secrets-floci`. The gists remain manual,
runnable examples; automated E2E uses test fixtures so they can be
parameterized for isolation and CI.

The tests use the shared `newFlociHarness` helper, which centralizes endpoint
discovery, the `ATMOS_TEST_FLOCI=true` opt-in gate, temporary fixture copies,
Atmos command execution, and AWS-compatible verification clients. The harness
clears ambient AWS credentials from the Atmos subprocess environment for these
store tests so the Atmos `aws/user` identity is the only auth path.

The fixtures deliberately cover explicit store identity and ambient-credential
isolation:

- SSM stores get an explicit `identity: floci-superuser`, which would fail in
  the old hook path with `auth resolver is not set`.
- Secrets Manager stores use explicit identity where Atmos auth is the intended
  credential source; stores that omit `identity` are left to ambient/default SDK
  credentials by design in this PR.
- Store and secret fixtures use `!env` for the Floci endpoint and prefixes, so
  auth identity YAML function processing is covered end to end.

Run the AWS cases with a running Floci endpoint:

```shell
ATMOS_TEST_FLOCI=true FLOCI_ENDPOINT_URL=http://localhost:4566 go test ./tests -run 'TestAWS(StoreHooks|Secrets)FlociE2E' -count=1
```

GCP and Azure use their provider-specific Floci endpoints:

```shell
ATMOS_TEST_FLOCI=true FLOCI_GCP_ENDPOINT=http://localhost:4588 go test ./tests -run TestGCPSecretsFlociE2E -count=1
ATMOS_TEST_FLOCI=true FLOCI_AZURE_ENDPOINT=http://localhost:4577 go test ./tests -run TestAzureSecretsFlociE2E -count=1
```

GitHub Actions runs the AWS, GCP, and Azure commands in the dedicated
`[floci] go e2e` job with pinned Floci service images. The normal acceptance
matrix leaves `ATMOS_TEST_FLOCI` unset, so these opt-in tests skip everywhere
except the Floci job.

The test intentionally uses Floci, not LocalStack.

## Documentation

Stores and secrets documentation now leads with `kind: aws/ssm` and
`kind: aws/asm`, lists AWS Secrets Manager wherever AWS store support is
described, and documents that global secret scope is implemented.

The function docs for `!store`, `!store.get`, and `atmos.Store` now state that
secret stores are rejected and must be accessed through `!secret`.

The Atmos store agent skill was updated with the same slash notation and AWS
Secrets Manager coverage so future generated examples do not regress to the
legacy hyphenated form.

## Validation

The focused regression suite passes:

```shell
go test ./pkg/store ./pkg/hooks ./cmd/terraform ./cmd/secret ./internal/exec ./pkg/function
```

Custom endpoint option parsing and GCP SDK option construction are covered by:

```shell
go test ./internal/gcp ./pkg/store
```

The live Floci AWS E2E passes against `floci/floci:1.5.23`:

```shell
ATMOS_TEST_FLOCI=true FLOCI_ENDPOINT_URL=http://localhost:4566 go test ./tests -run 'TestAWS(StoreHooks|Secrets)FlociE2E' -count=1 -v
```

The live Floci GCP and Azure E2E cases pass against `floci/floci-gcp` and
`floci/floci-az`:

```shell
ATMOS_TEST_FLOCI=true FLOCI_GCP_ENDPOINT=http://localhost:4588 go test ./tests -run TestGCPSecretsFlociE2E -count=1 -v
ATMOS_TEST_FLOCI=true FLOCI_AZURE_ENDPOINT=http://localhost:4577 go test ./tests -run TestAzureSecretsFlociE2E -count=1 -v
```

The Floci tests compile and skip cleanly when Floci is not enabled:

```shell
go test ./tests -run Floci
```

`git diff --check` passes.

## Expected Behavior

- A Terraform component hook can write outputs to AWS SSM or AWS Secrets
  Manager when the store declares an Atmos auth `identity`.
- A store that omits `identity` keeps the existing ambient/default SDK
  credential behavior. Component-identity inheritance for identity-less stores
  is intentionally left for a follow-up design decision.
- `kind: aws/ssm` and `kind: aws/asm` are the examples users see first.
- Legacy hyphenated store `type` values continue to work for backward
  compatibility.
- Stores marked `secret: true` are readable only through `!secret` and the
  `atmos secret` CLI.
- The AWS gists can be used manually against real AWS; equivalent automated
  coverage lives in the opt-in Floci test fixtures.
