# Fix: `!terraform.state` honors `remote_state_backend`

**Date:** 2026-10-20

## Summary

The `!terraform.state` YAML function always read another component's state through that component's
primary `backend`/`backend_type`, ignoring its `remote_state_backend`/`remote_state_backend_type`.
When a stack followed the documented pattern of a write-access role on `backend` and a read-only role on
`remote_state_backend`, every `!terraform.state` lookup assumed the write role instead of the read-only
role. The read path now prefers the component's `remote_state_backend` when configured, falling back to the
primary `backend` otherwise.

Fixes [#3446](https://github.com/cloudposse/atmos/issues/3446).

## Context

Atmos exposes two backend configurations per Terraform component:

- `backend`/`backend_type` - how the component reads **and writes** its own state (typically a role with
  write access to the state bucket).
- `remote_state_backend`/`remote_state_backend_type` - how **other** components read this component's state
  (typically a read-only role). The docs in `website/docs/stacks/_partials/_backend-config.mdx` describe this
  precisely: "Different credentials for writing vs. reading state".

During stack processing, `processTerraformRemoteStateBackend` (`internal/exec/stack_processor_backend.go`)
pre-computes the effective `remote_state_backend` section as the component's `backend` overlaid with any
`remote_state_backend` overrides, flattened to the resolved backend type, and stores it on the component
sections alongside the primary `backend`.

The in-process state reader, however, never looked at it. `GetTerraformBackend`
(`internal/terraform_backend/terraform_backend_utils.go`) resolved the backend type via
`GetComponentBackendType` (the `backend_type` key) and each per-backend reader (`s3`, `gcs`, `azurerm`,
`local`) read the `backend` key via `GetComponentBackend`. The `remote_state_backend` section was only ever
consulted for the `static` remote-state type. As a result, the S3 reader's
`GetS3BackendAssumeRoleArn` returned the write-access `assume_role.role_arn` from `backend`, producing the
`AccessDenied` / `sts:AssumeRole ... example-tfstate-write` failures reported in the issue.

This is the in-process read path only; it does not affect `terraform plan`/`apply`, which Terraform runs
through its own backend.

## Changes

- `internal/terraform_backend/terraform_backend_utils.go`:
  - Added `GetComponentRemoteStateBackend` and `GetComponentRemoteStateBackendType` accessors (mirroring the
    existing `GetComponentBackend`/`GetComponentBackendType`).
  - Added `resolveRemoteStateBackendSections`, which returns the component sections unchanged when no remote
    state backend is configured, and otherwise returns a shallow copy with the `backend`/`backend_type` keys
    overlaid by the `remote_state_backend`/`remote_state_backend_type` values. Only those two keys change; the
    original sections map is never mutated, and all other keys (`workspace`, `env`, `component`, etc.) are
    carried through so the per-backend readers behave identically.
  - `GetTerraformBackend` now calls `resolveRemoteStateBackendSections` before resolving the backend type and
    invoking the per-backend reader. This is the single read-path entry point (its only production caller is
    the `!terraform.state` flow in `internal/exec/terraform_state_utils.go`), so the change is scoped exactly
    to reading another component's state.
- `internal/terraform_backend/terraform_backend_remote_state_backend_test.go`: new tests (see below).
- `website/docs/functions/yaml/terraform.state.mdx`: clarified the "Terraform State Backend Lookup and Read"
  execution-flow step to state that Atmos prefers `remote_state_backend` when configured and falls back to
  the primary `backend` otherwise.

Because the pre-computed `remote_state_backend` section always starts from the primary `backend` (even when
the user sets no override, `remote_state_backend_type` defaults to `backend_type` and the merged section
equals `backend`), preferring it is fully backward compatible: with no override it is byte-for-byte the same
config the read path used before.

## Validation

- New `TestResolveRemoteStateBackendSections` covers: nil sections; no remote state backend (same pointer
  returned, no copy); remote backend overriding `backend`/`backend_type` with the read-only `assume_role`
  ARN resolved via `GetS3BackendAssumeRoleArn`; `remote_state_backend_type` differing from `backend_type`;
  type-only override; and the empty-map no-op case. Source-map isolation is asserted (the original `backend`
  section is unchanged after resolution).
- New `TestGetTerraformBackend_PrefersRemoteStateBackend` drives the public `GetTerraformBackend` through the
  real local backend reader: with `backend` pointing at one state file and `remote_state_backend` at another,
  it confirms the read comes from `remote_state_backend`; with no `remote_state_backend`, it confirms the
  fall-back to `backend`.
- Reproduction confirmed: with the one-line fix disabled, `reads from remote_state_backend when configured`
  fails with `from-write-backend` instead of `from-remote-state-backend`; with the fix it passes.
- `go test ./internal/terraform_backend/` and `go test ./internal/exec/ -run 'TerraformState|TerraformBackend'`
  both pass. Coverage of `resolveRemoteStateBackendSections`, `GetComponentRemoteStateBackend`, and
  `GetComponentRemoteStateBackendType` is 100%.
