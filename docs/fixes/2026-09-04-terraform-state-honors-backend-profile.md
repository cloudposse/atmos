# Fix: `!terraform.state` honors the S3 backend `profile` attribute

**Date:** 2026-09-04

**Issue:** [#3055](https://github.com/cloudposse/atmos/issues/3055)

## Summary

`!terraform.state` now uses the S3 backend's `profile` attribute to select AWS credentials when neither an
Atmos auth AWS identity nor the component's `env.AWS_PROFILE` does, matching what `terraform init` already
does with the same attribute in `backend.tf.json`.

## Context

`!terraform.output` resolves credentials through the `terraform` subprocess, whose S3 backend reads `profile`
from the generated backend config. The in-process reader behind `!terraform.state` only read `region`,
`bucket`, `key`, `workspace_key_prefix`, `sse_customer_key` and `assume_role.role_arn`; credentials came from
Atmos auth, the component `env` whitelist added in #2501, or the SDK default chain.

A project whose stages each map to a named profile in a shared backend mixin
(`profile: '{{ .vars.stage }}:tofu_admin'`) therefore worked with `!terraform.output` and failed with
`!terraform.state`. With no ambient credentials the SDK fell through to the instance metadata endpoint,
retried S3 and STS three times each, and surfaced an IMDS timeout about a minute later. The only workaround
was restating the profile a second time as `env.AWS_PROFILE` on every component.

The GCS reader already builds its client from the backend's own `credentials` attribute, so honoring the S3
backend `profile` brings the two readers in line rather than adding a new concept.

## Changes

- `internal/terraform_backend/terraform_backend_s3.go`: new `ResolveBackendProfileOverlay` injects the backend
  `profile` as `AWS_PROFILE` into the env overlay when no Atmos auth AWS context is active and the overlay does
  not already set `AWS_PROFILE`. `ReadTerraformBackendS3` passes its overlay through it. The overlay already
  participates in the S3 client cache key, so backends with different profiles never share a client.
- `internal/terraform_backend/terraform_backend_profile_fallback_test.go`: table of six cases covering the
  fallback, the no-profile no-op, explicit env precedence, merge without mutating the input, Atmos auth
  precedence, and an auth context without an AWS section.
- `website/docs/functions/yaml/terraform.state.mdx`: precedence list gains the backend profile, plus a new
  "Backend `profile` attribute" subsection with an example.

## Validation

- `go test ./internal/terraform_backend/` passes, including the new test and the existing S3 reader tests.
- `go vet ./internal/terraform_backend/` and `gofumpt -l` are clean.
- The repo's custom `golangci-lint` (built via `atmos lint custom-gcl` under the go.mod toolchain, Go 1.26.4)
  reports zero new issues for the change with `--new-from-rev`; the package's pre-existing findings are untouched.
- `cd website && pnpm install --frozen-lockfile && pnpm run build` succeeds; the only broken-anchor warnings are on
  an unrelated changelog page that already exists on `main`.
- Manual, against a real multi-account layout with `backend.s3.profile` set and no `env` section: a component
  whose transitive `!terraform.state` closure spans 19 upstream components in three AWS accounts resolved in
  about 7 seconds with values identical to `!terraform.output`; the debug trace shows each account's profile
  being loaded and its `assume_role.role_arn` assumed. The unpatched build on the same configuration failed
  after 68 seconds of credential retries.
- Not automated: the end-to-end `ReadTerraformBackendS3` path builds a real S3 client, so the credential
  resolution is covered by the helper's unit tests rather than an integration test.

## Follow-ups

None.
