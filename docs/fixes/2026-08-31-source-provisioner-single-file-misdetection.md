# Fix: source provisioner no longer misdetects one-file module directories as template sources

**Date:** 2026-08-31

## Summary

`VendorSource`'s single-file detection — added to support bare template URIs like
`source: {uri: https://.../dns.yaml}` — inspected only the shape of the downloaded staging
directory (exactly one entry, not a directory) to decide whether to write the result as a file
or a directory. That heuristic misfired for any Git/S3/archive source whose resolved
subdirectory happened to contain exactly one file, writing a *file* at the target path instead
of a directory. Every downstream consumer that assumes a JIT-provisioned workdir is a directory
then failed.

## Context

Discovered while investigating a persistently-failing CI shard on an unrelated branch
(`osterman/cfn-phase4-migration-graduation`). The failing tests
(`TestJITSource_WorkdirWithLocalComponent`, its `_AllSubcommands` variants,
`TestJITSource_GenerateVarfile`, `TestJITSource_GenerateBackend`,
`TestTerraformOutputJITWorkdirFromSource`) were initially assumed to be a flaky, pre-existing
race condition unrelated to the branch under review — the affected test file had zero diff
against `main`. Re-running the exact same test locally 5/5 times reproduced the failure
deterministically, which ruled out a race and prompted a real root-cause investigation instead
of continuing to report it as an environment flake.

## Changes

- `pkg/provisioner/source/vendor.go`: excluded Git, S3, and archive sources from the
  `singleFileInDir` single-file heuristic in `VendorSource`. These getters always unpack to a
  directory — even a directory containing just one file (e.g.
  `github.com/cloudposse/terraform-null-label//exports`, which only has `context.tf`, or a
  module tarball whose sole member is `main.tf`) — so they must never be routed through
  `copySingleFileToTarget`.
- `pkg/vendor/uri.go`: added `IsArchiveURI`, matching go-getter's own directory-producing
  decompressor extensions (`.tar`, `.tar.gz`/`.tgz`, `.tar.bz2`/`.tbz2`, `.tar.xz`/`.txz`,
  `.tar.zst`/`.tzst`, `.zip`). Deliberately excludes the single-compressed-file formats (bare
  `.gz`, `.bz2`, `.xz`, `.zst`), which legitimately decompress to exactly one file and must keep
  going through the single-file path.
- `pkg/provisioner/source/vendor_test.go`: added
  `TestVendorSourceArchiveWithOneFileIsNotMisdetectedAsSingleFileURI`, a regression test that
  packages a one-file tarball and asserts the target is a directory, not a file.
- `pkg/vendor/uri_test.go`: added `TestIsArchiveURI` covering the directory-producing and
  single-compressed-file extension cases.

## Validation

- Reverted the fix locally and confirmed the new regression test fails with the exact
  `"...: not a directory"` signature seen in CI, then restored the fix and confirmed it passes.
- `go test ./tests -run 'TestJITSource_WorkdirWithLocalComponent$|TestJITSource_WorkdirWithLocalComponent_AllSubcommands|TestJITSource_GenerateVarfile|TestJITSource_GenerateBackend|TestTerraformOutputJITWorkdirFromSource' -v`: all 5 previously-failing tests now pass.
- `go test ./pkg/provisioner/source/... ./pkg/vendor/...`: pass.
- `atmos lint --changed`: 0 issues.
- Broader local sweep (`TestJITSource*`, `TestVendor*`, `TestCLICommands`) run to check for
  regressions: only pre-existing, unrelated failures remained
  (`atmos_vendor_pull#01`/`atmos_vendor_pull_oci`, both blocked by this sandbox having no network
  route to `ghcr.io`'s anonymous OCI auth — confirmed via the captured `UNAUTHORIZED` error, not
  caused by this change).

## Follow-ups

None.
