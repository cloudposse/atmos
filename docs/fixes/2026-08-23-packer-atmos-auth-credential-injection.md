# Fix: `atmos packer` honors Atmos Auth and injects credentials into the subprocess

**Date:** 2026-08-23

## Summary

`atmos packer build` ran completely unauthenticated when relying on Atmos Auth. Unlike
`atmos terraform` and `atmos helmfile`, the packer executor never created an `AuthManager`,
never evaluated the merged `auth:` config, and never injected credentials into the packer
subprocess environment. The packer process therefore inherited only the ambient environment,
and its AWS datasources failed with:

```
Error: Datasource.Execute failed: No valid credential sources found

  on main.pkr.hcl line 99:
  (source code not available)
```

Fixed by wiring the same component-auth setup and credential-injection path that
`helmfile` uses into `ExecutePacker`.

## Context

Reported against a real workload (`atmos packer build fatvm -s safire-use2-platform`) that
relies on Atmos Auth identities rather than pre-assumed ambient credentials.

The DriveWealth reference pipeline
(`packer-aws-al2023/.github/workflows/ami.yml`) sidesteps the bug entirely: it runs
`aws-actions/configure-aws-credentials` (GitHub OIDC, `role-to-assume`) **before**
`atmos packer build`, so the AWS SDK inside packer finds credentials in the process
environment. That masks the defect in CI but leaves local runs — and any flow that
depends on the `auth:` block — broken.

Tracing the three executors made the gap concrete:

- **terraform** (`internal/exec/terraform.go`): calls `setupTerraformAuth` (creates and
  authenticates the `AuthManager`, injects the store auth resolver), passes the manager to
  `ProcessStacks`, and later runs `auth.TerraformPreHook` which calls
  `PrepareShellEnvironment` and sets `info.SanitizedEnv`.
- **helmfile** (`internal/exec/helmfile.go`): calls `SetupComponentAuthForCLI`, passes the
  manager to `ProcessStacks`, and injects credentials via `prepareHelmfileAuthEnvironment`
  (→ `AuthManager.PrepareShellEnvironment`) into the subprocess env just before executing.
- **packer** (`internal/exec/packer.go`): called `ProcessStacks(..., nil)` with a **nil**
  `AuthManager` and **never** prepared an authenticated environment. Two distinct failures:
  1. YAML functions/stores (`!terraform.output`, `!store`, …) resolved during packer var
     processing ran unauthenticated.
  2. No AWS credentials were ever placed into the packer subprocess environment — the direct
     cause of "No valid credential sources found".

The `AuthManager` interface docstring (`pkg/auth/types/interfaces.go:303`) explicitly lists
Packer as a supported subprocess for `PrepareShellEnvironment`, so the interface contract and
the implementation were out of sync.

## Changes

- `internal/exec/packer.go`:
  - Call `SetupComponentAuthForCLI` (the shared helper helmfile uses) before stack processing
    to build and authenticate the component `AuthManager`.
  - Pass the resolved `authManager` to `ProcessStacks` instead of `nil`, so YAML
    functions/stores evaluated during packer var processing are authenticated.
  - Inject the resolved identity's credentials into the subprocess env via
    `prepareComponentAuthEnvironment` (→ `PrepareShellEnvironment`) just before executing
    packer — a no-op when auth is disabled or no identity is configured.
  - Added two test seams (`processStacksForPacker`, `executePackerShellCommand`) mirroring the
    existing `var x = Func` seam convention used elsewhere in the package.
- `internal/exec/utils_auth.go`: extracted `resolveDefaultIdentity` and the renamed
  `prepareComponentAuthEnvironment` (formerly `prepareHelmfileAuthEnvironment`) into the shared
  exec-layer auth file so helmfile and packer share one credential-injection path (Code Reuse
  mandate — extend, don't duplicate).
- `internal/exec/helmfile.go`: removed the now-shared local copies and call the shared
  `prepareComponentAuthEnvironment`.
- `internal/exec/helmfile_utils_test.go`: updated references to the renamed shared helper.
- `internal/exec/packer_auth_test.go` (new):
  - `TestExecutePacker_PassesAuthManagerToProcessStacks` — primary regression test; intercepts
    `ProcessStacks` and asserts a non-nil `AuthManager` is passed (fails against the old `nil`
    behavior). Requires no packer binary.
  - `TestExecutePacker_InjectsAuthCredentialsIntoSubprocessEnv` — a fake `AuthManager` injects a
    sentinel AWS credential via `PrepareShellEnvironment`; the packer shell invocation is
    intercepted and the env is asserted to contain the sentinel. Guarded by `RequirePacker`.

## Validation

- `go build ./...`
- `go test ./internal/exec/ -run 'TestExecutePacker_PassesAuthManagerToProcessStacks|TestExecutePacker_InjectsAuthCredentialsIntoSubprocessEnv|TestPrepareComponentAuthEnvironment' -count=1 -v`
  — all pass. Confirmed `TestExecutePacker_PassesAuthManagerToProcessStacks` **fails** against
  the pre-fix code (nil `AuthManager`) and passes after.
- `go test ./internal/exec/ -run 'Packer|Helmfile' -count=1` — ok.
- `atmos lint --changed` — 0 issues.

## Follow-ups

None.
