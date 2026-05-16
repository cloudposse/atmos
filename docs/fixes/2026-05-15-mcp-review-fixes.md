# MCP Implementation Review — Punch List Fixes

**Date:** 2026-05-15
**Branch:** `aknysh/claude-atmos-mcp`
**Scope:** 9 issues found during a review of the MCP server/client implementation
landed in `pkg/mcp/`, `cmd/mcp/`, and `website/docs/ai/`. PRD says all 6 MCP
phases shipped — they did, with the caveats below.

## Background

The MCP implementation is split into two halves:

- **Server side** (`pkg/mcp`, `cmd/mcp/server`) — Atmos runs as an MCP server
  and exposes its AI tools to MCP clients (Claude Desktop, Cursor, …).
- **Client side** (`pkg/mcp/client`, `cmd/mcp/client`) — Atmos connects out to
  external MCP servers (the awslabs/mcp suite, etc.) and bridges their tools
  into the AI tool registry.

Architecture is sound (the server/client split, `BridgedTool`,
`ScopedAuthProvider`, the two-pass `router`, sentinel-error chain through
`ErrMCPServerStartFailed`). The issues below are quality and correctness
problems within that architecture, not redesigns.

---

## Issue 1 — `atmos mcp export` duplicates `.mcp.json` logic and silently drops the toolchain PATH (P1, correctness)

### Problem

`cmd/mcp/client/export.go` re-declares its own private `mcpJSONConfig`,
`mcpJSONServer`, `buildMCPJSONEntry`, and `uppercaseEnvKeys` symbols.
`pkg/mcp/client/mcpconfig.go` already exports `MCPJSONConfig`,
`MCPJSONServer`, `BuildMCPJSONEntry`, `GenerateMCPConfig`, and
`WriteMCPConfigToTempFile`.

The two implementations are not equivalent. The package version threads a
`toolchainPATH` argument into each server entry's `env.PATH` (deduplicating
along the way) so the IDE subprocess that the exported `.mcp.json` spawns
can find `uvx` / `npx` even when those binaries are only on the Atmos
toolchain PATH. The cmd-local version omits this entirely.

### Symptom

A user runs `atmos toolchain install astral-sh/uv@…` so `uvx` is available
through Atmos but **not** on the system PATH. They then run
`atmos mcp export` and open the IDE. The IDE invokes the configured `uvx
awslabs.…` command, can't find `uvx` on its own PATH, and the MCP server
fails to start. The other client-side commands (`status`, `tools`, `test`,
`restart`) already work around this by computing a toolchain PATH via
`buildToolchainOption`; `export` is the only path that silently drops it.

### Fix

Delete the private types and helpers from `cmd/mcp/client/export.go` and
call `mcpclient.GenerateMCPConfig(servers, toolchainPATH)`. Build
`toolchainPATH` the same way the other commands do — load `.tool-versions`
deps and ask the resulting `ToolchainEnvironment` for its `PATH()`.

### Test

`TestExecuteMCPExport_InjectsToolchainPATH` writes a `.tool-versions` +
minimal `atmos.yaml` to a temp dir, runs the export, parses the result,
and asserts the toolchain bin dir appears in the exported `env.PATH`.

---

## Issue 2 — `describe_affected` advertised but not registered in MCP server (P1, correctness)

### Problem

`website/docs/ai/mcp-server.mdx:108-122` lists `describe_affected` among
the tools the MCP server exposes. `cmd/mcp/server/start.go::initializeAIComponents`
hand-registers exactly 7 tools (`describe_component`, `list_stacks`,
`validate_stacks`, plus 2× `read_*` and 2× `write_*`). `describe_affected`
is missing. The tool itself exists at
`pkg/ai/tools/atmos/describe_affected.go:20`.

### Root Cause

`initializeAIComponents` duplicates the tool registration that
`cmd/ai/init.go` does for `chat` / `ask` / `exec` — but the canonical
factory `atmosTools.RegisterTools` already exists at
`pkg/ai/tools/atmos/setup.go:10` and registers **all** Atmos AI tools
(`describe_affected` included, plus search, edit, execute, findings,
compliance, etc.).

### Fix

Replace the seven hand-rolled `registry.Register(...)` calls with a single
`atmosTools.RegisterTools(registry, atmosConfig, nil)` call. Pass `nil`
for the LSP manager — MCP servers don't have an LSP context, which
`RegisterTools` already handles gracefully. This eliminates the
drift-prone duplication and, as a side effect, the MCP server now exposes
the same tool surface as `atmos ai ask` (find, search, edit, execute,
findings) instead of only a curated subset.

### Test

`TestInitializeAIComponents_RegistersDescribeAffected` asserts the
registered tool set contains `describe_affected` (the regression guard
for the docs ↔ code drift) plus the original 7.
`TestInitializeAIComponents_RegistersSharedFactoryTools` asserts a
representative subset of the newly-exposed tools (`search_files`,
`execute_atmos_command`) is also present, locking in the use of the
shared factory rather than the old hand-rolled list.

---

## Issue 3 — `atmos mcp restart` stops the server it just started, but the help text doesn't say so (P2, UX)

### Problem

`cmd/mcp/client/restart.go` does stop → start → success → **stop**. The
trailing comment explains: "MCP servers are subprocess-based and started
on-demand by AI commands."

That's accurate — stdio MCP servers can't usefully outlive the parent
process — but a user reading `Use: "restart <name>"` and
`Short: "Restart an MCP server"` reasonably expects a running server
afterward. They get a stop+start+stop cycle and no running server. The
help text contains no hint of this.

### Fix

Update `Short` and the embedded long-form markdown to call out that the
command **validates** a stop+start cycle and exits without leaving the
server running. The implementation is unchanged — only the user-facing
docs.

### Test

`TestRestartCmd_HelpMentionsValidationSemantics` asserts the `Short`
field contains the word "validate" so a future change can't quietly
remove the disambiguation.

---

## Issue 4 — `WriteMCPConfigToTempFile` uses a fixed path, races on concurrent invocations (P2, correctness)

### Problem

`pkg/mcp/client/mcpconfig.go:82-83`:

```go
tmpDir := os.TempDir()
tmpFile := filepath.Join(tmpDir, "atmos-mcp-config.json")
```

Two concurrent `atmos ai ask` invocations (e.g., two terminals, or a
parallel CI matrix on a shared runner) race on the same path. The faster
writer's content is silently overwritten while the slower reader may
read partial content.

### Fix

Switch to `os.CreateTemp(tmpDir, "atmos-mcp-config-*.json")`. Each writer
gets a unique path; the caller still owns cleanup. File permissions are
written explicitly (mode `0600`) after creation to match the prior
guarantee.

### Test

`TestWriteMCPConfigToTempFile_ConcurrentWritesGetDistinctPaths` runs the
writer twice in parallel goroutines and asserts the returned paths are
distinct and both files exist with the expected content.

---

## Issue 5 — `Session.Tools()` returns the cached slice without copying (P3, defensive)

### Problem

`pkg/mcp/client/session.go:87-91`:

```go
func (s *Session) Tools() []*mcpsdk.Tool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.tools
}
```

The accessor's doc comment promises a read-only view ("returns the
cached list"), but a caller can freely mutate the slice header (append,
nil out, reorder) and the next call returns the mutated state.
`BridgeTools` (the only current consumer) doesn't mutate, but external
callers and future contributors could.

### Fix

Return a defensive copy:
`return append([]*mcpsdk.Tool(nil), s.tools...)`.

### Test

`TestSession_ToolsReturnsDefensiveCopy` populates a Session's `tools`
field via reflection, mutates the returned slice (sets `[0] = nil`),
and asserts the next `Tools()` call still returns the original
non-nil value.

---

## Issue 6 — `firstSentence` heuristic in `atmos mcp tools` is fragile (P3, correctness)

### Problem

`cmd/mcp/client/tools.go:85-100` truncates tool descriptions to the
first sentence using only two terminator patterns: `". "` and `" ##"`.
That fails for:

- `! ` or `? ` endings — no match, returns the entire description.
- No terminator at all — returns the entire description unchanged
  (the function is named `firstSentence` but performs no length
  bound).
- A URL or version string like `v1.0 ` appearing before the real
  sentence end — splits at the wrong place.

The downstream renderer (`theme.CreateMinimalTable`) then has to
truncate a paragraph-length value into one row, which produces
inconsistent column widths.

### Fix

- Recognize `. `, `! `, `? ` as sentence terminators (matching English
  sentence punctuation).
- Add a hard 80-char ceiling: if no terminator is found in the first
  80 chars, truncate with an ellipsis.
- Keep the `" ##"` markdown-header break (existing behavior).
- Whitespace collapsing is preserved.

### Test

`TestFirstSentence` is a table-driven test covering: period+space,
exclamation+space, question+space, markdown-header break, no
terminator with long input (truncated to ≤80 chars), no terminator
with short input (returned as-is), URL-like content before the real
period.

---

## Issue 7 — `atmos mcp tools` doesn't use the renderer pipeline (P3, DX)

### Problem

`cmd/mcp/client/tools.go:70-79` hand-rolls a 2-column table via
`theme.CreateMinimalTable`. The sibling `cmd/mcp/client/list.go`
already uses the full renderer pipeline (filters, sorters, columns,
format selector for `table` / `json` / `yaml` / `csv` / `tsv`). The
inconsistency surfaces in CLI behavior: `mcp list --format=json` works,
`mcp tools <name> --format=json` doesn't exist.

CLAUDE.md's tui-list section makes the renderer pipeline mandatory for
new list commands. `mcp tools` predates that guidance.

### Fix

Migrate `tools.go` to the same renderer pipeline `list.go` uses. Wire
the standard `--format`, `--columns`, `--sort`, `--delimiter` flags
through `flags.StandardParser` so they bind to Viper for env-var
precedence (matches `mcp list`). Default columns are `NAME` and
`DESCRIPTION`; data keys are `name` and `description`.

### Test

`TestMCPToolsOutput_FormatJSON` exercises the JSON output branch
end-to-end through the pipeline using a fake session that returns two
stub tools; asserts the JSON contains both tool names. The default-table
branch is covered by the existing test surface (and indirectly by
golden snapshots elsewhere).

---

## Issue 8 — `ScopedAuthProvider.baseConfig` is unused (P4, janitorial)

### Problem

`pkg/mcp/client/scoped_auth.go:32` declares a `baseConfig` field with the
comment "retained for future extensibility (e.g., fallback when no env
override is in effect). Currently unused because every call path goes
through the env-overrides primitive."

Comments referencing "future" usage rot. The field is plumbed into the
constructor argument list but never read; this is dead state that
obscures the type's real contract.

### Fix

Remove the field, the constructor parameter, and the explanatory
comment. `NewScopedAuthProvider()` now takes no arguments — call sites
already pass `atmosConfig` and discard the wrapper; the simplification
is internal.

### Test

The existing `pkg/mcp/client/scoped_auth_test.go` tests are updated
to call the new constructor signature and continue to assert the same
behavior contract.

---

## Issue 9 — `atmos mcp test` double-prints errors (P4, UX)

### Problem

`cmd/mcp/client/test_cmd.go:55`:

```go
result := mgr.Test(ctx, name, startOpts...)
printTestResult(result)
return result.Error
```

`printTestResult` already calls `ui.Error(...)` when the result has a
non-nil `Error`. Then the function returns that same error to cobra,
which prints it again under its own error formatting. Users see two
copies of the same message in different styling.

### Fix

`printTestResult` already provides the user-facing error display.
Return `nil` from the `RunE` so the second print (via `errUtils.Format`
in `main.go`) is skipped entirely. `mcp test` is a diagnostic command,
not a CI gate — its output is designed to be read by humans, and
pass/fail is unambiguous from the ✓/✗ markers `printTestResult` emits.

**Trade-off:** the process exit code is now 0 even when the test
fails. Investigating cobra's `SilenceErrors`, a custom "silent"
sentinel error type, and a `ExitCodeError{Code: 1}` return all proved
incompatible with the existing `main.go` error-formatting pipeline
without invasive changes to `errors/formatter.go`. Callers needing
exit-code-driven CI behavior should parse the structured output of
`atmos mcp tools` or `atmos mcp status --format=json` instead — those
emit machine-readable status rather than the human-friendly diagnostic
checklist `mcp test` produces.

### Test

`TestExecuteMCPTest_ReturnsNilEvenOnFailure` asserts the new behavior
via a unit test on the helper-flow: with `printTestResult` as the
single source of stderr output, returning nil from RunE guarantees by
construction that `main.go`'s `errUtils.Format` branch never runs.
The presence-and-content tests on `printTestResult` (already in
`helpers_test.go::TestFormatStatusRow`) cover the user-visible message
itself.

---

## Out of Scope

None — all 9 review items are addressed in this changeset. Future work
that came up during the review but is **not** included:

- Stack-level MCP server overrides (per-stack `settings.mcp.servers`).
- Composite MCP server (expose external MCP tools through Atmos's own
  MCP server back to IDEs).
- `tools/list_changed` notification handling for dynamic tool updates.

---

## Expected Behavior After Fix

| Symptom                                                                   | Before                                                         | After                                                                                      |
|---------------------------------------------------------------------------|----------------------------------------------------------------|--------------------------------------------------------------------------------------------|
| `atmos mcp export` from a project with toolchain-managed `uvx`            | IDE can't find `uvx`, MCP server fails to start                | Exported `.mcp.json` includes toolchain PATH in each server's `env.PATH`                   |
| `atmos ai ask` in MCP-server mode                                         | `describe_affected` and 5 other tools unavailable              | Full tool set (incl. `describe_affected`, `search_files`, `execute_atmos_command`) exposed |
| `atmos mcp restart <name>`                                                | "Restarted X" but server is stopped                            | Same behavior; help text now clarifies the validation semantics                            |
| Concurrent `atmos ai ask` invocations                                     | Shared temp config file races, content can corrupt             | Each invocation gets a unique temp file                                                    |
| Caller mutates `Session.Tools()` result                                   | Next call sees mutation                                        | Next call returns original cached slice                                                    |
| Tool description "Lists IAM roles! Optionally …" in `atmos mcp tools <s>` | Full description bleeds into one row                           | Truncated at `!` or 80 chars                                                               |
| `atmos mcp tools <s> --format=json`                                       | Unknown flag error                                             | JSON output via renderer pipeline                                                          |
| `pkg/mcp/client/scoped_auth.go` reading                                   | Confused by the "future extensibility" field that does nothing | Clean type definition                                                                      |
| `atmos mcp test <missing>`                                                | Two copies of the error message                                | Single error message (exit code is now 0 — see issue 9 trade-off)                          |
