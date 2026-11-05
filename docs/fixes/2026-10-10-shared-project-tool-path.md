# Fix: Share installed project tool selections across Atmos commands

**Date:** 2026-10-10

## Summary

Every Atmos command inherits installed versions selected by the project's `.tool-versions` file on
`PATH`. Explicit `dependencies.tools` override that baseline. Installation policy controls downloads
separately from version selection.

## Context

This clarifies the [component tool defaults fix](2026-10-09-component-tool-versions-defaults.md) for
[#3341](https://github.com/cloudposse/atmos/issues/3341). Shared command startup previously exposed
proxy links only; installed manifest selections were available only through scoped dependency
environments. The `declared` policy also ignored the manifest outside workflows, coupling version
selection to installation policy.

Before changing production code, regression tests demonstrated both gaps: a command without its own
dependency setup found system Terraform, and `declared` omitted installed manifest selections.

## Changes

- Shared command startup adds only installed project selections, without provisioning tools under any
  policy. Unlisted cached tools and unselected cached versions are not added. Repeated initialization
  does not duplicate the baseline directories, and existing proxy precedence is preserved.
- Scoped environments keep explicit dependencies ahead of project defaults. `declared` now inherits
  installed manifest tools while retaining its previous automatic download policy. Edition pins
  continue to choose `declared` or `auto`; neither policy hides installed project selections.
- The shared baseline reuses alias resolution, first-version selection, configured manifest paths,
  and project base directory handling. Missing manifests leave PATH unchanged. Startup logs baseline
  preparation errors at debug level so diagnostic and repair commands remain usable; scoped execution
  still reports contextual manifest errors.
- Low-level, deliberately filtered environments retain their existing behavior. Tests use isolated
  installed fixtures and mocked provisioning, assert explicit override precedence, and verify that
  the manifest remains unchanged. Command test cleanup now restores process PATH.
- Updated the announcement, dependency reference, Terraform guide, hook and custom-command docs,
  toolchain policy reference, and edition journal description to separate selection from downloads.

## Validation

- Confirmed the new shared-startup and `declared` regressions failed before implementation and passed
  after implementation.
- `go test ./pkg/dependencies ./pkg/schema ./pkg/config ./cmd ./internal/exec -short -timeout 5m`:
  passed.
- `go build ./...`: passed.
- Terraform output regression (`TestExecutor_ExecuteWithSections_ToolchainResolvesExecutable`): passed.
- `atmos fix lint`: passed after correcting three lint findings.
- Fix-log format validation and `git diff --check`: passed.
- Website `npm run build`: passed. The output includes the final wording changes; existing warnings
  about other pages' anchors, dynamic imports, and Markdown normalization remain.
- Full `atmos test`: encountered the existing
  `pkg/ci/startup/TestPrintStartupStatus_PrintsWhenInCI` failure. The test does not clear the
  `ATMOS_STARTUP_NOTICES_SHOWN` sentinel inherited from the parent Atmos command. Running that test
  alone passes with the sentinel unset and reproduces the failure with it set to `1`; the package is
  unchanged by this fix. The `tests` package also exceeded the command's five-minute timeout while
  running `TestCLICommands` (the active subtest was the invalid `legacy-prod` component-name case).
  All other packages completed successfully. The full suite is therefore not green.

## Follow-ups

None.
