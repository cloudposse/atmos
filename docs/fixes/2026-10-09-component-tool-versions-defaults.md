# Fix: Every command and component inherits project tool versions as a baseline

**Date:** 2026-10-09

## Summary

Every command and component type now uses `.tool-versions` as a baseline. Components install their
own executable (and `helm` for Helmfile) from the file when it is pinned. Other tools in the file are
added to `PATH` only when they are already installed. Workflows keep their existing behavior and
install every listed tool that is missing. Explicit `dependencies.tools` entries are always installed
and override the baseline, including when an alias and a qualified name identify the same tool.

## Context

[Issue #3341](https://github.com/cloudposse/atmos/issues/3341) reported that the Terraform version
guide promised automatic selection from `.tool-versions`, but component commands selected whichever
executable appeared on the system `PATH`. `ForComponent` resolved only stack dependencies, and
`ForSections` (Terraform output and template lookups) only the supplied dependencies, so both built an
empty toolchain environment without explicit dependencies.

A first implementation made every component inherit and install the whole manifest. A hands-on field
test against stub binaries and a baseline binary built from `origin/main` found regressions in that
approach:

- Read-only commands (`describe stacks`, `describe component` with `!terraform.output`,
  `terraform version`, `terraform plan --dry-run`) downloaded unrelated manifest tools.
- asdf/mise manifests (`nodejs 20.1.0`, `terraform system`, `ref:`/`path:` versions, `~> 1.9.0` with a
  space) aborted component runs that work on `main`.
- Two manifest entries for one tool with different versions (`tofu 1.12.6` and
  `opentofu/opentofu 1.11.0`) selected a binary nondeterministically (15/5 over 20 runs).
- `toolchain.file_path`, documented as the primary setting, was ignored; component runs silently fell
  back to the system `PATH`.
- A manifest in a read-only directory failed every run because the shared `.tool-versions.lock` could
  not be created.
- Offline short-name resolution reported "not found in Aqua registry" instead of the network failure.
- Custom commands, hooks, and Ansible components ignored the manifest entirely.

## Changes

- Replaced the defaults overlay in `pkg/dependencies` with a plan that splits manifest entries into
  installed tools (explicit dependencies, the component's selected executable, or every entry for
  workflows) and PATH-only tools that are already installed. PATH-only tools never reach the
  installer.
- Derived a component's executable from its `command` section, then `components.<type>.command`, then
  the type default. Matching against manifest keys is offline: exact key, configured or built-in
  aliases, or the repo segment of an `owner/repo` key. Native Kubernetes components select nothing;
  native Helm selects `helm`.
- Made manifest entries best-effort: `system`, `ref:`, `path:`, bare constraint operators (with a
  warning), and unresolvable names are skipped. Explicit dependencies stay strict.
- Collapsed duplicate manifest identities with the same version and added `ErrToolVersionsConflict`
  for different versions, naming both entries.
- Added `ForCommand` and `ForDependencies` and routed custom commands, hooks, and Ansible components
  through the shared overlay.
- Honored `toolchain.file_path` before `versions_file` in manifest resolution and the file manager,
  and kept the `--tool-versions` flag and `ATMOS_TOOL_VERSIONS` overrides winning.
- Fell back to an unlocked manifest read when the shared lock cannot be created because of permissions
  or a read-only filesystem. Writes still require the lock.
- Added `ErrToolRegistryIndexUnavailable` so an unreachable Aqua registry index is reported as such,
  with hints to use `owner/repo` names or aliases.
- Updated the dependencies, Terraform versions, workflow, custom command, and hook docs, including the
  workflow install-all exception; corrected stale comments in the tflint scanner and the toolchain
  example.

## Validation

- Field test (fixture in a local scratch directory with stub binaries and a decoy `PATH`), rerun on the
  rebuilt binary:
  - The manifest default beats the decoy for `terraform version` and `plan`; a short-name component
    override selects `1.8.5`; the manifest is unchanged.
  - `describe stacks`, `describe component`, `terraform version`, `plan --dry-run`, and `plan` no
    longer install an unpinned `jqlang/jq 1.6`.
  - `nodejs`, `system`, `ref:`, and `~> 1.9.0` entries are skipped; `system` and the operator case
    fall back to `PATH`, the operator case with a warning.
  - Conflicting duplicates fail with an error naming both entries; same-version duplicates work.
  - `file_path`, a read-only manifest directory, custom-command PATH-only inheritance and explicit
    override, workflow install-all, and a workflow pin overriding the manifest all behave as
    documented.
  - Offline, an explicit short name reports that the Aqua registry index could not be loaded.
- `go test -count=1` passed for `./pkg/dependencies/...`, `./pkg/toolchain/...`,
  `./pkg/terraform/output/...`, `./pkg/scanners/tflint/...`, `./pkg/hooks/...`, `./pkg/component/...`,
  `./cmd/toolchain/...`, and `./errors/...`, plus scoped runs of `./cmd`
  (`Custom|Dependenc|Toolchain|Proxy`) and `./internal/exec` (`Workflow|Toolchain|Dependenc|ToolVersions`).
- `go build ./...` passed; `./custom-gcl run --new-from-rev=origin/main` reported 0 issues on the
  changed packages.
- `npm run build` in `website` succeeded. It reported broken anchors only on pages this change does not
  touch, plus the existing dynamic-import warnings.

## Follow-ups

None.
