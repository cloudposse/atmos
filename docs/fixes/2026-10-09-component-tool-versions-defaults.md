# Fix: Every command and component inherits project tool versions as a baseline

**Date:** 2026-10-09

> **2026-10-10 update:** The later [shared command baseline fix](2026-10-10-shared-project-tool-path.md) separates PATH selection from downloads. It supersedes the `declared` behavior recorded below: all commands now inherit installed project selections under every policy.

## Summary

Every command and component type now uses `.tool-versions` as a baseline. Components install their
own executable (and `helm` for Helmfile) from the file when it is pinned. Other tools in the file are
added to `PATH` only when they are already installed. Workflows keep their existing behavior and
install every listed tool that is missing. Explicit `dependencies.tools` entries are always installed
and override the baseline, including when an alias and a qualified name identify the same tool.

How much Atmos installs on its own is the `toolchain.install` setting (`never`, `declared`, `auto`,
`always`; default `auto`). The behavior above is `auto`. Projects pinned to an edition before
2026-10-09 get `declared`, which restores the previous behavior (components, custom commands, and
hooks ignore `.tool-versions`).

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
  warning), and unresolvable names are skipped. Lines without a version are skipped with a warning
  through the new `LoadToolVersionsLenient`; commands that rewrite the file keep the strict parser so
  no line is ever dropped. Explicit dependencies stay strict.
- Collapsed duplicate manifest identities with the same version and added `ErrToolVersionsConflict`
  for different versions, naming both entries.
- Added `ForCommand` and `ForDependencies` and routed custom commands, hooks, and Ansible components
  through the shared overlay.
- Honored `toolchain.file_path` before `versions_file` in manifest resolution and the file manager,
  and kept the `--tool-versions` flag and `ATMOS_TOOL_VERSIONS` overrides winning. `atmos toolchain`
  now reads `ATMOS_TOOL_VERSIONS` and `ATMOS_TOOLCHAIN_PATH` themselves; it previously read the flag
  defaults (`.tool-versions`, `.tools`) whenever only the environment variable was set.
- Fell back to an unlocked manifest read when the shared lock cannot be created because of permissions
  or a read-only filesystem. Writes still require the lock.
- Added `ErrToolRegistryIndexUnavailable` so an unreachable Aqua registry index is reported as such,
  with hints to use `owner/repo` names or aliases.
- Replaced the hard-coded policy (including the workflow install-all exception) with the
  `toolchain.install` setting: `schema.ToolchainInstall` values `never`, `declared`, `auto`, `always`;
  default `auto` through `setDefaultConfiguration`, bound to `ATMOS_TOOLCHAIN_INSTALL`, and validated
  at config load (`ErrInvalidToolchainInstall`) and again where the policy is read
  (`dependencies.InstallPolicy`). An empty value means `auto`.
  - `never`: no installer call. Explicit dependencies and manifest tools resolve from what is already
    installed, constraints against installed versions. A missing explicit dependency fails with the new
    `ErrToolNotInstalled` and a hint to run `atmos toolchain install`; a missing manifest tool is skipped.
  - `declared`: explicit dependencies only; components, custom commands, and hooks never read the
    manifest. Workflows install every manifest tool.
  - `auto`: the behavior described above. `always`: every usable manifest tool for every run.
  - `NewEnvironmentFromDeps` stays manifest-free and honors `never` (installs nothing, uses what is
    installed), which also covers the MCP client, AI agent setup, and the tflint availability check.
  - `atmos cast render` skips downloading the managed `agg`/`ffmpeg` renderers under `never` and reports
    `ErrToolNotInstalled` with an install hint when one is missing.
  - Left alone: `atmos toolchain exec` and toolchain proxies (explicit tool invocations that install on
    demand) and Atmos version re-exec (a separate subsystem).
- Journaled `toolchain.install` in `pkg/edition` (`declared` to `auto`, 2026-10-09, PR #3346) with the
  default snapshot regenerated, replacing the earlier post-editions `KindBehavior` candidate note in
  `docs/prd/editions.md`. Regenerated the `atmos.yaml` JSON schema and the seven `describe config`
  golden snapshots that now include `toolchain.install: auto`.
- Updated the toolchain configuration, dependencies, Terraform versions, workflow, custom command, and
  hook docs, the changelog post, and the roadmap entry for the policy; corrected stale comments in the
  tflint scanner and the toolchain example.

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
- `go test -count=1` passed for `./pkg/dependencies/...`, `./pkg/toolchain/...`, `./pkg/config/...`,
  `./pkg/edition/...`, `./pkg/schema/...`, `./pkg/asciicast/...`, `./pkg/terraform/output/...`,
  `./pkg/scanners/tflint/...`, `./pkg/hooks/...`, `./pkg/component/...`, `./pkg/datafetcher/...`,
  `./cmd/mcp/...`, `./cmd/ai/...`, `./pkg/ai/agent/...`, and `./errors/...`, plus scoped runs of `./cmd`
  (`Custom|Dependenc|Toolchain|Proxy|MCP|AI`) and `./internal/exec`
  (`Workflow|Toolchain|Dependenc|ToolVersions`).
- Policy coverage in `pkg/dependencies/install_policy_test.go`: all four policies across component,
  sections, workflow, command, and dependencies constructors (installer call and map, PATH-only
  contents); `never` with installed, missing, unresolvable, and constraint dependencies; `declared`
  ignoring an installed manifest terraform while a decoy on `PATH` wins; the manifest not being read
  under `declared`; invalid values; and `NewEnvironmentFromDeps` under `never`. In `pkg/config`: the
  edition pin (`declared` before 2026-10-09, `auto` on and after), environment-variable and
  configuration-file precedence, and invalid values failing at config load.
- Not run: a field test of `toolchain.install` against the built binary; the policy is covered by the
  unit tests above only.
- `go build ./...` passed; `./custom-gcl run --new-from-rev=origin/main` reported 0 issues on the
  changed packages.
- `npm run build` in `website` succeeded. It reported broken anchors only on pages this change does not
  touch, plus the existing dynamic-import warnings.

## Follow-ups

None.
