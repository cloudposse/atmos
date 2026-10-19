# CLI-Config Import DX: Parity with Stack Imports

**Issue:** [#3430](https://github.com/cloudposse/atmos/issues/3430)
**Status:** Implemented
**Label:** `minor`

## Problem

CLI-config imports — the top-level `import:` list in `atmos.yaml` and the auto-discovered
`.atmos.d/*.yaml` fragments — were a bare `[]string`, while stack-manifest imports already had a
mature object model (`skip_if_missing`, `nested_imports`, `ttl`, `context`, template controls).
A common pattern of sharing developer-tooling CLI config (`ai`, `mcp`, `toolchain`) from one
source-of-truth repo into many consumer repos via remote imports surfaced several rough edges:

1. **Every CLI-config import was implicitly optional (fail open).** A failed resolve or merge was a
  `log.Warn` + `continue`, so a security-bearing fragment (for example a read-only tool allowlist
  constraining the Atmos MCP server) could silently fail to load, leaving the tool running without
  its guardrail. There was no way to express "this import MUST load."
2. **Shared `mcp.servers` config could not degrade gracefully.** A server bound to an `identity`
  not defined in a given consumer's profiles failed hard, with no per-server opt-out.
3. **Unpinned remote imports were silent.** A `git::` import on a mutable ref (`?ref=main`, a
  branch, or no `?ref=`) silently absorbed upstream edits on every cache refresh, with no integrity
  check — the config-import analogue of an unpinned GitHub Action.
4. **No way to scope a fragment to interactive-only vs automation.** Developer-tooling config has no
  purpose in an automated `apply`, yet `.atmos.d` auto-discovery was all-or-nothing, and config
  loads before the subcommand is known. The workaround was to keep the fragment out of the
  committed config entirely (gitignored `*.yaml.example`).

## Goals

- Bring CLI-config imports toward parity with the existing stack-import object model, **additively**
  — a bare string entry stays valid and keeps today's fail-open warn-and-skip behavior.
- Let a security-bearing import fail closed.
- Let a shared MCP server be skipped gracefully where its identity is unavailable.
- Warn on unpinned remote-import refs at author time.
- Let a fragment be scoped to local vs CI without the gitignore/`.example` workaround.

## Non-Goals

- Per-import `ttl` override on CLI-config imports (the global `imports.ttl` is unchanged).
- Fragment-level `when:` on `.atmos.d` auto-discovery as a whole (per-import `when:` covers the
  motivating use case; `.atmos.d` fragments merge through raw Viper before any struct exists).
- A blunt env/setting to disable `.atmos.d` auto-discovery (issue proposal #5, deferred — the
  per-import `when:` removes most of the need).

## Design

### 1. Object-form imports (`skip_if_missing`)

`schema.AtmosConfiguration.Import` changes from `[]string` to `schema.ConfigImports`
(`[]schema.ConfigImport`). `ConfigImport` carries `Path`, `SkipIfMissing`, and `When`.

Decoding is handled by `schema.ConfigImportsDecodeHook()` (a mapstructure decode hook composed into
`atmosDecodeHook()` alongside the existing `ConditionDecodeHook`), mirroring how the `condition`
package decodes string/list/map `when` values:

- A **bare string** decodes to `ConfigImport{Path: <string>, SkipIfMissing: true}` — **optional**
  (fail open), preserving historical behavior exactly.
- An **object** decodes normally; `skip_if_missing` keeps its zero value (`false` = **required**,
  fail closed) when absent. `when` is decoded by the composed condition hook.

Code paths that read the raw `import:` value from Viper rather than unmarshaling into a typed struct
(the local/go-getter adapters processing nested imports, and profile loading via
`mergeConfigFileWithImports`) use `config.NormalizeConfigImports(raw any)`, the counterpart to the
decode hook, which accepts both the legacy string-list form and the object form.

In `processImports`, a resolve failure of a **required** import returns
`errUtils.ErrRequiredImportFailed` (fail closed); an **optional** import warns and continues. The
failure intent is stamped onto each resolved file (`ResolvedPaths.SkipIfMissing`) so a later merge
failure fails closed or warn-and-skips consistently.

**Nested-import contract:** `skip_if_missing`/`when` are honored per entry at resolve time in each
`processImports` pass. At merge time, the importing entry's intent governs its resolved subtree (a
required top-level import's files are required; a nested optional entry inside a required parent is
treated as required at merge). This keeps the core guarantee — a required top-level import fails
closed — simple and well-defined, without threading per-entry merge intent through the adapter
interface.

### 2. Optional MCP servers

`schema.MCPServerConfig` gains `Optional bool` (`optional`), threaded into
`client.ParsedConfig.Optional`. When a server start fails and
`shouldSkipOptionalServer(cfg, err)` holds — the server is optional **and** the failure is
`errUtils.ErrMCPServerAuthUnavailable` (its identity could not be resolved) — the red error is
downgraded to a warn-and-skip. Any other failure of an optional server, and any failure of a
non-optional server, remains an error. Defaults to today's behavior when unset.

### 3. Unpinned remote-import warning

The go-getter adapter's `Resolve` calls `warnIfUnpinnedRef(importPath)`. For a git-backed import
(`git::`, `git@`, `github.com/`, `bitbucket.org/`), it extracts the `?ref=` query parameter and
warns unless the ref is pinned — a commit SHA (7–40 hex) or a version tag (`v?\d+(\.\d+)*`). A
branch ref, a non-version tag, or an absent ref warns. Advisory only; the import still resolves.

### 4. Conditional loading (`when`)

`ConfigImport.When` is a `schema.Condition`, decoded by the composed `ConditionDecodeHook`, so it
accepts the full `when` vocabulary (the `local`/`ci`/`always`/`never` predicates, lists, objects,
and CEL). In `processImports`, each import whose `when` is set and does not match the evaluation
context is skipped (debug-logged); it is evaluated before resolution, so a `when`-skipped required
import is not an error. The context exposes the two facts meaningful at config-load time (before
the subcommand is known): `condition.Context{CI: ci.IsCI(), Env: osEnvMap()}` - CI detection (the
`ci`/`local` predicates) and the process environment (the `env` map in a CEL expression, e.g.
`when: '!cel env.DEPLOY_ENV == "prod"'`). In `atmos.yaml`, CEL is authored as a quoted `"!cel ..."`
string, not a `!cel` YAML tag (the CLI-config loader only recognizes its own YAML functions as
tags). `pkg/config` importing `pkg/ci` introduces no cycle (root `pkg/ci` does not depend on
`pkg/config`).

## Backward Compatibility

- String import entries are unchanged: still valid, still optional (fail open).
- `optional` / object fields default to today's behavior when unset.
- The unpinned-ref check is a warning, not an error.
- No config default value changes and no reinterpretation of an existing stored value → no
  `pkg/edition` journal entry required.

## Schema

The CLI config JSON Schema (`pkg/datafetcher/schema/atmos/config/1.0.json`) is regenerated via
`go generate ./pkg/config/schema`. The `import` key becomes an `anyOf` of string or `ConfigImport`,
and `MCPServerConfig` gains `optional`.

## Testing

- `pkg/schema`: decode hook (string → optional, object → required, explicit `skip_if_missing`/`when`,
  mixed list), `ConfigImportsFromPaths`, `Paths()`.
- `pkg/config`: `NormalizeConfigImports` (both forms, mixed, empty-dropped); `processImports`
  required-vs-optional (resolve + merge), `when` gating (`never`/`always`/`local` with
  `ci.SwapRegistryForTest`), string-import-still-resolves backward-compat guard.
- `pkg/config/adapters`: `isPinnedRef` / `isGitBackedImport` / `extractRef` / `warnIfUnpinnedRef`
  (warns on branch/absent, silent on SHA/version-tag/non-git).
- `pkg/mcp/client`: `ParseConfig` carries `optional`; `shouldSkipOptionalServer` truth table.
