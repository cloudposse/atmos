# Data Model: Full Component Name in Atmos Pro Uploads

**Feature**: `specs/003-fix-upload-component-name` | **Date**: 2026-09-10

No new entities, fields, or schema changes. This documents the existing identity fields and the two payload entities whose `component` value changes source.

## Identity fields on `schema.ConfigAndStacksInfo`

| Field | Producer | Value for logical name `foo/bar/baz` | Contract |
|-------|----------|--------------------------------------|----------|
| `ComponentFromArg` | CLI arg (`internal/exec/cli_utils.go:409,441`); path args rewritten to logical name (`cmd/terraform/utils.go:1411`); per graph node: `nodeInfo.ComponentFromArg = node.Component` (`pkg/scheduler/adapters/terraform.go:682`) | `foo/bar/baz` | **Canonical component identity.** Full logical stack-manifest key. Never truncated. The ONLY correct source for upload payloads. |
| `Component` | Split in `ProcessStacks` (`internal/exec/utils.go:1106–1116`): last path segment of `ComponentFromArg` (after a possible interim overwrite from the `component:` attribute at :1475–1477) | `baz` | Working-directory leaf. Feeds `components/terraform/<prefix>/<leaf>/` resolution. MUST NOT be used as identity in uploads. Unchanged by this feature. |
| `ComponentFolderPrefix` | Same split (all-but-last segments); overwritten by base-component path when `metadata.component` inheritance applies (:1120–1129) | `foo/bar` | Working-directory prefix. Not a reliable complement to `Component` for reconstructing identity. Unchanged. |

## Payload entities (value source corrected, shape unchanged)

### `dtos.InstanceStatusUploadRequest` — instance-status upload (`--upload-status`)

Built in `uploadStatus`, `internal/exec/pro.go:243–257`. Sent per invocation (single) / per graph node (bulk) via `pro.AtmosProAPIClientInterface.UploadInstanceStatus`.

| Field | Current source | New source | Notes |
|-------|---------------|-----------|-------|
| `Component` | `info.Component` (truncated leaf) | `info.ComponentFromArg` (full logical name) | **The fix (site 1).** |
| `Stack` | `info.Stack` | unchanged | Already full stack name. |
| all others (`AtmosProRunID`, `GitSHA`, `RepoURL/Name/Owner/Host`, `Command`, `ExitCode`, version/os/arch) | — | unchanged | |

### TerraformExecData component entry — execution record (`components[]` element)

Built by `buildTerraformExecData` (`cmd/terraform/utils.go:873`); the `component` key is set only when non-empty (:915–917). Two callers:

| Caller | Invocation shape | Current `component` argument | New | Notes |
|--------|-----------------|------------------------------|-----|-------|
| `terraformNodeHooks.recordExecResult` (`cmd/terraform/utils.go:581`) | multi-component (`--affected`/`--all`/`--components`/`--query`) per graph node | `info.Component` (truncated leaf) | `info.ComponentFromArg` (full logical name) | **The fix (site 2).** |
| `terraformExecMetadataParserFunc` (`cmd/terraform/utils.go:958–966`) via `terraformCaptureShellOpts` | single-component | `args[0]` (full CLI argument) | unchanged | Already correct — regression-guard test only (spec FR-004). Known quirk: raw path for path-style args — deferred (FR-008). |

## Channels verified correct (no change; consistency targets for SC-002)

| Channel | Identity source | Value |
|---------|----------------|-------|
| Execution-record envelope `Args` | `info.ComponentFromArg` (`internal/exec/terraform.go:281–283`) | full |
| Stack lock/unlock key | user-supplied `--component` flag (`internal/exec/pro.go:204`) | full |
| Affected-component uploads / pact fixtures | logical names from stack processing | full |

## Validation rules (test assertions)

1. Nested name in → identical full name out, byte-for-byte, at both fixed sites (spec FR-001/002/003).
2. Flat name in → identical value out (control; FR-006).
3. Single-component parser path still emits the full CLI argument (FR-004).
4. `Component`/`ComponentFolderPrefix` values after `ProcessStacks` are unchanged for nested names (FR-005 — covered by existing tests passing unmodified).
