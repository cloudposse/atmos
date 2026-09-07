# Fix: bare `--identity` failed instead of showing the interactive selector on 6 commands

**Date:** 2026-08-21

## Summary

`atmos <custom-command> --identity` (bare, no value) failed with `Error: --identity flag needs an
argument` instead of popping up the interactive identity picker that other Atmos commands show.
The same defect existed, independently, on five other commands.

## Context

A user hit this running a downstream custom command (`atmos app build --identity`). `--identity`
is meant to be a single, centrally-defined flag: `flags.WithIdentityFlag()` (`pkg/flags/options.go`)
pulls the one canonical definition — including `NoOptDefVal: cfg.IdentityFlagSelectValue` (the
`"__SELECT__"` sentinel), which is what makes a bare `--identity` legal and triggers the
interactive `huh` selector (`pkg/auth/manager.go`'s `promptForIdentity`) — out of
`GlobalFlagsRegistry()`. ~150 command files already wire this correctly via
`flags.NewStandardParser(flags.WithIdentityFlag())`.

Six command files never got migrated onto that shared parser and instead hand-rolled their own
`cmd.Flags().StringP("identity", "i", ...)` registration, silently bypassing the single
implementation:

| File                         | Flag registration missing `NoOptDefVal` | Sentinel (`__SELECT__`) never resolved to a real identity |
| ----------------------------- | :--------------------------------------: | :---------------------------------------------------------: |
| `cmd/cmd_utils.go` (custom commands) | yes | yes — `Authenticate(ctx, "__SELECT__")` called directly, failed with `ErrIdentityNotFound` |
| `cmd/describe.go`            | yes (deliberately, per a stale comment)  | no — already correct downstream |
| `cmd/list/list.go`           | yes (same stale comment)                 | no — already correct downstream |
| `cmd/aws/ecr/login.go`       | no — already hand-patched                | no — already correct, but via a locally duplicated helper |
| `cmd/aws/eks/token.go`       | yes                                       | yes |
| `cmd/azure/aks/token.go`     | yes                                       | yes |

## Changes

- `pkg/auth/manager_helpers.go`: added exported `ResolveSelectedIdentity(authManager, identityName,
  selectValue)`, extracted from the existing local `resolveSelectedIdentity` in
  `cmd/aws/ecr/login.go`, so the sentinel → `GetDefaultIdentity(forceSelect=true)` resolution
  exists in exactly one place. `authenticateWithIdentity` now delegates to it internally.
- `cmd/cmd_utils.go` (`createCustomCommand`): replaced the hand-rolled `PersistentFlags().String()`
  identity registration with `flags.NewStandardParser(flags.WithIdentityFlag()).RegisterPersistentFlags(...)`.
  `prepareCustomCommandAuth` now resolves the sentinel via `auth.ResolveSelectedIdentity` before the
  credential-cache check and `Authenticate` call. Added a `newCustomCommandAuthManagerFn` seam for
  testing.
- `cmd/describe.go`, `cmd/list/list.go`: same flag-registration migration (their downstream sentinel
  resolution was already correct, via `GetIdentityFromFlags` / `CreateAndAuthenticateManagerWithStackScan`).
- `cmd/aws/ecr/login.go`: flag registration migrated to the shared builder; the local
  `resolveSelectedIdentity` now delegates its body to `auth.ResolveSelectedIdentity`.
- `cmd/aws/eks/token.go`, `cmd/azure/aks/token.go`: same flag-registration migration, plus
  `authenticateForToken` now resolves the sentinel via `auth.ResolveSelectedIdentity` before the
  existing empty-identity default-lookup fallback. Added a `newAuthManagerFn` seam for testing.

## Validation

- Added `TestResolveSelectedIdentity_*` (`pkg/auth/manager_helpers_test.go`),
  `TestCustomCommand_IdentityFlagNoOptDefVal` / `TestPrepareCustomCommandAuth_SelectValue`
  (`cmd/custom_command_identity_test.go`), `TestDescribeCmd_IdentityFlagNoOptDefVal`
  (`cmd/describe_identity_test.go`), `TestListCmd_IdentityFlagNoOptDefVal` (`cmd/list/list_test.go`),
  and `TestAuthenticateForToken_SelectValue*` in both `cmd/aws/eks/token_test.go` and
  `cmd/azure/aks/token_test.go`.
- `go build ./...` and `go test ./cmd/... ./pkg/auth/... ./cmd/aws/eks/... ./cmd/azure/aks/...
  ./cmd/aws/ecr/...` — all pass, no regressions.
- `./custom-gcl run --new-from-rev=origin/main ./...` — 0 issues.

## Follow-ups

None.
