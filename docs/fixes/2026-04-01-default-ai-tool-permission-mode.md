# Fix: Default AI Tool Permission Mode Silently Allows All Tool Execution

**Date:** 2026-04-01
**Severity:** Critical (CWE-284 — Improper Access Control)
**Affected file:** `cmd/mcp/server/start.go`

---

## Problem

When a developer enables AI tools (`ai.tools.enabled: true`) in `atmos.yaml` without explicitly
setting `require_confirmation: true`, the `getPermissionMode()` function in `cmd/mcp/server/start.go`
returned `ModeAllow` by default. This caused `CheckPermission()` to return `true` for every tool call
without any user prompt — even for tools annotated with `RequiresPermission() bool { return true }`.

The AI could invoke `execute_bash_command` (or any other tool) with arbitrary payloads without
interactive confirmation, despite the tool's own permission annotation implying the opposite behavior.

### Vulnerable code (before fix)

```go
// cmd/mcp/server/start.go
func getPermissionMode(atmosConfig *schema.AtmosConfiguration) permission.Mode {
    if atmosConfig.AI.Tools.YOLOMode {
        return permission.ModeYOLO
    }
    if atmosConfig.AI.Tools.RequireConfirmation != nil && *atmosConfig.AI.Tools.RequireConfirmation {
        return permission.ModePrompt
    }
    return permission.ModeAllow  // ← default: allow all tools without prompting
}
```

---

## Root Cause

The `getPermissionMode()` function in `cmd/mcp/server/start.go` only returned `ModePrompt` when
`RequireConfirmation` was **explicitly set to `true`**. When the field was `nil` (not set at all —
the common case for new or minimal configs), the function fell through to `ModeAllow`.

This was inconsistent with the equivalent function in `cmd/ai/chat.go`, which correctly treated
`nil` as "not configured, default to safe" and returned `ModePrompt`:

```go
// cmd/ai/chat.go — safe reference implementation
func getPermissionMode(atmosConfig *schema.AtmosConfiguration) permission.Mode {
    if atmosConfig.AI.Tools.YOLOMode {
        return permission.ModeYOLO
    }
    // Not set - default to prompting for security.
    if atmosConfig.AI.Tools.RequireConfirmation == nil {
        return permission.ModePrompt
    }
    if *atmosConfig.AI.Tools.RequireConfirmation {
        return permission.ModePrompt
    }
    // Explicitly set to false - opt-out of prompting.
    return permission.ModeAllow
}
```

---

## Fix

Updated `getPermissionMode()` in `cmd/mcp/server/start.go` to match the secure behavior already
present in `cmd/ai/chat.go`: return `ModePrompt` when `RequireConfirmation` is `nil` (not set).
`ModeAllow` is only returned when the user **explicitly** sets `require_confirmation: false`.

```go
// cmd/mcp/server/start.go — after fix
func getPermissionMode(atmosConfig *schema.AtmosConfiguration) permission.Mode {
    if atmosConfig.AI.Tools.YOLOMode {
        return permission.ModeYOLO
    }
    // Not set - default to prompting for security.
    if atmosConfig.AI.Tools.RequireConfirmation == nil {
        return permission.ModePrompt
    }
    if *atmosConfig.AI.Tools.RequireConfirmation {
        return permission.ModePrompt
    }
    // Explicitly set to false - opt-out of prompting.
    return permission.ModeAllow
}
```

---

## Behavior Change

| `require_confirmation` value | Before fix | After fix |
|------------------------------|-----------|-----------|
| Not set (`nil`)              | `ModeAllow` (silent allow) | `ModePrompt` (prompt user) |
| `true`                       | `ModePrompt` | `ModePrompt` |
| `false`                      | `ModeAllow` | `ModeAllow` |
| `yolo_mode: true`            | `ModeYOLO` | `ModeYOLO` |

> **Note (MCP server):** The table above shows the behavior of `getPermissionMode()` alone.
> However, `cmd/mcp/server/start.go` also builds a `permission.Config` struct whose `YOLOMode`
> field is checked by `CheckPermission()` **before** the `Mode` field.  A previous version of
> the code unconditionally forced `permConfig.YOLOMode = true` in `initializeAIComponents`,
> which bypassed `getPermissionMode()` entirely for MCP servers.  That unconditional override
> has been removed as part of this fix.  `permConfig.YOLOMode` now mirrors
> `atmosConfig.AI.Tools.YOLOMode`, consistent with the rest of the permission pipeline.
>
> To guarantee deterministic behavior in MCP environments, operators should explicitly configure
> their intent in `atmos.yaml`:
>
> - **Disable prompting entirely:** set `yolo_mode: true` or `require_confirmation: false`
> - **Require prompting:** set `require_confirmation: true` (or leave unset — the default is
>   now `ModePrompt`)

Users who want to opt out of prompting must now **explicitly** set `require_confirmation: false`
in their `atmos.yaml`:

```yaml
ai:
  tools:
    enabled: true
    require_confirmation: false  # explicit opt-out required
```

---

## Files Changed

- `cmd/mcp/server/start.go` — `getPermissionMode()` default changed from `ModeAllow` to `ModePrompt`; removed unconditional `permConfig.YOLOMode = true` override in `initializeAIComponents`
- `cmd/mcp/server/start_test.go` — updated test cases expecting `ModeAllow` for nil config to expect `ModePrompt`; renamed `TestInitializeAIComponents_YOLOModeOverride` to `TestInitializeAIComponents_YOLOModeRespected`

---

## Related

- `cmd/ai/chat.go` — reference implementation with the correct secure default (unchanged)
- `pkg/ai/tools/permission/checker.go` — `CheckPermission()`, `ModeAllow` short-circuits all checks
- `pkg/ai/tools/permission/types.go` — `Mode` constants definition
