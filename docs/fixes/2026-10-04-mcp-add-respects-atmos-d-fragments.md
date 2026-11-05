# Fix: `atmos mcp add`/`remove` edit the config file that is actually in effect

**Date:** 2026-10-04

**Issue:** [cloudposse/atmos#3269](https://github.com/cloudposse/atmos/issues/3269)

**PR:** [cloudposse/atmos#3270](https://github.com/cloudposse/atmos/pull/3270)

## Summary

`atmos mcp add` / `atmos mcp remove` always wrote the server declaration - and flipped
`mcp.enabled: true` - into the root `atmos.yaml`, ignoring the `.atmos.d/` config-fragment directory.
A project that keeps its MCP config in a fragment (for example `.atmos.d/mcp.yaml`) ended up with its
configuration split across two files.

The fix resolves the file to edit by the **effective provenance** of the specific key being changed,
not by the root file alone and not by top-level section presence. The command edits the file whose
value actually wins the merge, so an edit is never silently shadowed by a higher-precedence file, and
a modular-config project is not split.

## Context and precedence

Atmos auto-discovers and deep-merges `atmos.d/`/`.atmos.d/` fragments when it loads config, so
`mcp.servers` declared in a fragment are read correctly and show up in `atmos mcp list`. Crucially,
the merge precedence (verified empirically) is:

1. git-repository-root `atmos.d/` / `.atmos.d/` fragments (lowest);
2. the current working directory's fragments, **only when the CWD has its own root `atmos.yaml`**
    (otherwise the loader uses the git root and the CWD fragments are not merged);
3. the root `atmos.yaml` / `.atmos.yaml` itself (highest - Atmos reapplies it after its imports).

So an explicit value in the root `atmos.yaml` overrides a fragment, and within a fragment directory a
later file (by load order) overrides an earlier one.

**Profiles** add a fourth, highest-precedence layer: an active profile (selected via `--profile`,
`ATMOS_PROFILE`, or `profiles.default`) is merged *after* the root `atmos.yaml`, so a profile that
declares `mcp.servers.<name>` or `mcp.enabled` wins over the root. A plain invocation with no active
profile is unaffected.

## Root cause

`mcpconfig.ResolveFile` resolved the file to edit via `config.ResolveEditableConfigFile`, which only
considers `atmos.yaml`/`.atmos.yaml` in the current directory and the git root - never the fragment
directories, and never the specific key's owner. The read path merged fragments while the write path
targeted only the root, which is what produced the split.

## Changes

- `pkg/config/config_edit.go`: new `EffectiveConfigFilesAscending(atmosConfig)` returns the config
  files that participate in the merge, in ascending precedence (fragments, then root `atmos.yaml`, then
  active-profile files last). Helpers `fragmentDirsAscending` (git root, then CWD only when it carries
  its own root config; `"."` from a non-repo is treated as "no git root"), `fragmentFiles` (via
  `SearchAtmosConfig`), and `activeProfileFiles` (reuses the loader's `GetActiveProfiles`,
  `discoverProfileLocations`, and `findProfileDirectory`, so profile precedence matches config loading).
- `pkg/mcp/config/config.go`: replaced `ResolveFile` with
  - `ResolveServerFile(cmd, atmosConfig, name) (file, declared, err)` - picks the explicit `--config`
    override, else the highest-precedence file that already declares `mcp.servers.<name>` (correct
    target for an overwrite/remove), else the highest-precedence file that declares `mcp.servers` (a
    new server joins the existing servers, typically a fragment), else the root `atmos.yaml`. The
    `declared` flag tells `remove` whether the server exists.
  - `ResolveEnableFile(cmd, atmosConfig)` - targets the highest-precedence file declaring
    `mcp.enabled`, else the root `atmos.yaml`, because the root value wins and an enable written to a
    shadowed fragment would not take effect.
- `cmd/mcp/client/add.go`: server writes use `ResolveServerFile`; the `mcp.enabled` write uses
  `ResolveEnableFile`.
- `cmd/mcp/client/remove.go`: resolves by server name and reports "not configured" via the `declared`
  flag instead of checking a single guessed file.
- Help/docs: `cmd/mcp/client/add.go` (`Short`), `cmd/mcp/client/markdown/atmos_mcp_add.md`, and
  `website/docs/cli/commands/mcp/add.mdx` describe which file is edited and the `--config` escape hatch.

## Behavior

- New server, MCP config lives in a fragment, root declares no MCP → the fragment is edited (#3269).
- Server declared in both root and a fragment → the root is edited (its value is effective), so the
  overwrite takes effect instead of being silently shadowed.
- Same server in two fragments → the later (highest-precedence) fragment is edited.
- An active profile declares the server or `mcp.enabled` → the profile file is edited (it overrides the
  root), and `remove` finds a profile-only server instead of reporting "not configured".
- `remove <name>` when the server is not declared anywhere → "not configured" error.
- `mcp.enabled` → the root `atmos.yaml` (or the fragment that owns it when the root is silent).
- `--config <file>` → that file, regardless of provenance; multiple `--config` files → still rejected
  as ambiguous.
- Nothing declares MCP config → root `atmos.yaml` (unchanged behavior).

### Known limitation

When the same server is declared in more than one file, `add` edits the effective (highest-precedence)
file but does not delete the shadowed copy in a lower-precedence file. Deep-merge may therefore still
combine leftover keys from the lower-precedence copy. Removing a duplicate declaration remains a manual
edit (or a `--config`-targeted `remove`).

## Validation

- `pkg/mcp/config`: `ResolveServerFile` tests for override, new-server-joins-fragment (#3269),
  existing-server-in-fragment, **root-shadows-fragment** (the #3270 security case), new-server-no-MCP
  fallback, unknown-server-not-declared (remove), and multiple-`--config` ambiguity; `ResolveEnableFile`
  tests for root-wins, fragment-when-root-silent, and fallback-to-root.
- `pkg/config`: `EffectiveConfigFilesAscending` tests for root-is-highest-precedence,
  fragment-order-within-dir (later wins), and CWD-excluded-without-root-config.
- End-to-end with the built binary: a new server joined `.atmos.d/mcp.yaml` with the root untouched;
  an overwrite of a server declared in both root and fragment updated the root so the new URL became
  effective.
- `CGO_ENABLED=0 go build ./...`, `go test ./pkg/config/... ./pkg/mcp/config/... ./cmd/mcp/client/...`,
  `golangci-lint` (0 issues), and `cd website && npm run build` all pass.
