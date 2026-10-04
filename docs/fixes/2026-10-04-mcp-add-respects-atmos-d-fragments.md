# Fix: `atmos mcp add` respects `.atmos.d/` config fragments

**Date:** 2026-10-04

**Issue:** [cloudposse/atmos#3269](https://github.com/cloudposse/atmos/issues/3269)

## Summary

`atmos mcp add` (and `atmos mcp remove`) always wrote the server declaration - and flipped
`mcp.enabled: true` - into the root `atmos.yaml`, ignoring the `.atmos.d/` config-fragment directory.
A project that keeps its MCP config in a fragment (for example `.atmos.d/mcp.yaml`) ended up with its
configuration split across two files, and could get duplicate or conflicting `mcp.servers` entries.

Now, when no explicit `--config` is given, `add`/`remove` detect an auto-discovered `atmos.d/` or
`.atmos.d/` fragment that already declares an `mcp:` section and edit that fragment instead of the
root `atmos.yaml`. When no such fragment exists, behavior is unchanged (the root `atmos.yaml` is
edited). An explicit `--config <file>` still wins over detection.

## Context

Atmos auto-discovers and deep-merges `atmos.d/` and `.atmos.d/` fragments when it loads config, so
`mcp.servers` declared in a fragment are read correctly and show up in `atmos mcp list`. Only the
*write* path lagged: `mcpconfig.ResolveFile` resolved the file to edit via
`config.ResolveEditableConfigFile`, which considers only `atmos.yaml`/`.atmos.yaml` in the current
directory and the git root - never the fragment directories. The reading convention and the writing
convention disagreed, which is what produced the split.

## Root cause

`config.ResolveEditableConfigFile` (used by `config set`/`delete`/`format` and `mcp add`/`remove`)
intentionally targets a single concrete file and had no notion of the default-import fragment
directories. `atmos mcp add` reused it directly, so every add landed in the root `atmos.yaml`.

## Changes

- `pkg/config/config_edit.go`:
  - New `ResolveEditableConfigFileForSection(atmosConfig, override, section)` - same precedence as
    `ResolveEditableConfigFile` (explicit override wins) but, before falling back to the root
    `atmos.yaml`, it prefers an auto-discovered fragment that already declares the given top-level
    `section`.
  - New `fragmentDeclaringSection` / `fragmentSearchDirs` - enumerate `atmos.d/` and `.atmos.d/`
    (current working directory first, then the git repository root) via the existing
    `SearchAtmosConfig`, and return the first fragment that declares the section. The CWD-before-git-root
    order mirrors `mergeDefaultImports`' load precedence.
  - New `fileDeclaresTopLevelKey` - a lightweight YAML-node check for a top-level key, so detection
    does not depend on a full config load.
- `pkg/mcp/config/config.go`: `ResolveFile` now calls `ResolveEditableConfigFileForSection(..., "mcp")`
  so both `add` and `remove` honor the fragment. Because `mcp.enabled` is written to the same resolved
  file, enabling MCP during `add self` also lands in the fragment, keeping the config in one place.
- Docs/help: `cmd/mcp/client/add.go` (`Short`), `cmd/mcp/client/markdown/atmos_mcp_add.md`, and
  `website/docs/cli/commands/mcp/add.mdx` document which file is edited and the `--config` escape hatch.

## Behavior

- No fragment declares `mcp:` → root `atmos.yaml` is edited (unchanged behavior).
- A `.atmos.d/mcp.yaml` (or `atmos.d/...`, nested, or `.yml`) declares `mcp:` → that fragment is
  edited.
- `--config <file>` → that file is edited regardless of any fragment.
- Multiple `--config` files → still rejected as ambiguous (`ErrAmbiguousConfigFile`), unchanged.

## Validation

- `pkg/mcp/config`: `TestResolveFile_PrefersAtmosDFragmentDeclaringMCP` (reproduces #3269; fails before
  the fix, passes after), plus `..._FallsBackToRootWithoutMCPFragment` and
  `..._ExplicitConfigOverridesFragment`.
- `pkg/config`: `TestResolveEditableConfigFileForSection_*` (fragment variants `atmos.d`/`.atmos.d`,
  nested, `.yml`; fallback; override wins; no-config error) and `TestFileDeclaresTopLevelKey` (declares,
  null value, absent, scalar root, missing file, invalid YAML). New functions covered at 90-100%.
- End-to-end with the built binary: in a project with `atmos.yaml` + `.atmos.d/mcp.yaml`, running
  `atmos mcp add https://mcp.example.com/demo --name demo --yes` added `demo` to `.atmos.d/mcp.yaml`
  and left `atmos.yaml` untouched.
- `CGO_ENABLED=0 go build ./...`, `go test ./pkg/config/... ./pkg/mcp/config/... ./cmd/mcp/client/...`,
  and `custom-gcl run --new-from-rev=origin/main` on the changed packages: all pass.
