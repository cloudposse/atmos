# Fix: `aws cfn`/`helm`/`kubernetes` no longer duplicate the global flag set as local persistent flags

**Date:** 2026-08-26

## Summary

`atmos aws cfn --help`, `atmos helm --help`, and `atmos kubernetes --help` showed every global CLI
flag (`--base-path`, `--chdir`, `--config`, `--cast`, `--ai`, `--mask`, `--no-color`, `--profile`,
`--profiler-*`, `--redirect-stderr`, `--settings-list-merge-strategy`, `--skill`, `--edition`,
`--identity`, ...) in their default "FLAGS" section, unlike `atmos terraform --help`/`atmos vendor
--help`, which show only their own genuinely local flags. All three command families were registering
the entire global flag set as their own local persistent flags — a real duplicate registration, not
just an inherited-flags display quirk.

## Context

User-reported: `atmos aws cfn --help` looked like it was defaulting to `--help=all`. Traced to
`cmd/aws/cloudformation/cloudformation.go`'s `cloudFormationParser = flags.NewStandardParser(flags.
WithCommonFlags())`. `flags.WithCommonFlags()` (`pkg/flags/options.go`) registers every flag in
`flags.GlobalFlagsRegistry()` (the same set `flags.NewGlobalOptionsBuilder().Build().
RegisterPersistentFlags(RootCmd)` already registers persistently on `RootCmd` at `cmd/root.go:2203`,
inherited by every subcommand automatically) plus `stack`/`dry-run` from `flags.CommonFlags()`. CFN
then calls `cloudFormationParser.RegisterPersistentFlags(CloudFormationCmd)` and its own separate
`cloudFormationParser.BindToViper(viper.GetViper())` — a second, independently-viper-bound local copy
of every global flag, not just a cosmetic display artifact. `cmd/terraform/terraform.go` and
`cmd/vendor/vendor.go` don't have this problem: they register only genuinely command-specific flag
registries (`WithTerraformFlags()`/`WithTerraformAffectedFlags()`, built from `flags.CommonFlags()`
plus terraform-only flags — never `flags.GlobalFlagsRegistry()`), and rely on cobra's normal
persistent-flag inheritance from `RootCmd` for the rest. Grepping every production (non-test) caller
of `flags.WithCommonFlags()` found the identical pattern in `cmd/helm/helm.go` and
`cmd/kubernetes/kubernetes.go` — fixed together since it's the same bug.

## Changes

`cmd/aws/cloudformation/cloudformation.go`, `cmd/helm/helm.go`, `cmd/kubernetes/kubernetes.go`:
changed `flags.NewStandardParser(flags.WithCommonFlags())` to `flags.NewStandardParser(flags.
WithStackFlag(), flags.WithDryRunFlag())` — `WithStackFlag()`/`WithDryRunFlag()` (already existing,
narrowly-scoped `pkg/flags/options.go` helpers) register exactly `--stack`/`--dry-run`, the two flags
each command family's own subcommands actually read, without pulling in the full global registry.
`--identity` was dropped too: it's part of `GlobalFlagsRegistry()` (already inherited from `RootCmd`,
confirmed unused by name anywhere in the CFN/Helm/Kubernetes command packages) — unlike terraform,
which intentionally re-registers its own `--identity` with a terraform-specific help description via
`cmd/terraform/shared/flags.go`'s `RegisterIdentityFlags`, none of these three families do anything
similar, so there's no reason to keep a duplicate.

`flags.WithCommonFlags()` itself is untouched (still used by `pkg/flags`'s own tests and one isolated
test-only synthetic command in `cmd/terraform/migrate/migrate_test.go` that never registers against
the real `RootCmd` tree) — the fix is scoped to removing its three buggy call sites, not the shared
option itself.

## Validation

- New tests (written first, confirmed failing against the old registration before the fix):
  `TestCloudFormationCmd_DoesNotDuplicateGlobalFlags`,
  `TestHelmCmd_DoesNotDuplicateGlobalFlags`, `TestKubernetesCmd_DoesNotDuplicateGlobalFlags` — each
  asserts every global-only flag name is absent from the command's own `PersistentFlags()`, and that
  `--stack`/`--dry-run` remain present.
- `go build ./... && go test ./cmd/... ./pkg/flags/...` — all pass.
- `atmos lint --changed` — clean (one pre-existing, unrelated finding remains:
  `cmd/terraform/utils.go:484`).
- Live: `atmos aws cfn --help`/`atmos helm --help`/`atmos kubernetes --help` now show only
  `--dry-run`, `-h/--help`, `-s/--stack` in their FLAGS section, matching `terraform`/`vendor`.
  Confirmed inherited global flags still work post-fix: `atmos aws cfn validate demo -s local
  --chdir=.` correctly resolved `--chdir` and `-s/--stack` and proceeded through normal execution
  (failing only on an unrelated, expected "emulator not running" error).

## Follow-ups

None.
