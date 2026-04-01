# AI Tool `execute_atmos_command` Accepts Unvalidated Subcommand Arguments (CWE-88)

**Date:** 2026-04-01

**Vulnerability Class:** CWE-88 — Improper Neutralization of Argument Delimiters in a Command

**Severity:** High

**Affected File:** `pkg/ai/tools/atmos/execute_command.go`, lines 70–90 (prior to fix)

---

## Issue Description

The `execute_atmos_command` AI tool split the incoming `command` parameter with
`strings.Fields()` and forwarded the resulting tokens directly to the Atmos binary
as arguments, with no validation of which subcommands or flags were being requested.

An AI agent under a prompt-injection or LLM-jacking attack could be induced to run
state-modifying Terraform operations such as:

```
terraform apply vpc -s prod -var-file=/etc/passwd
terraform destroy vpc -s prod --auto-approve
terraform state rm module.vpc
```

Because the default permission mode was `ModeAllow` (i.e. no user confirmation), these
operations could be triggered automatically without any human gate.

### Attack Scenario

1. An adversary crafts a prompt that instructs the LLM to call
   `execute_atmos_command` with `terraform apply vpc -s prod`.
2. Because no subcommand validation exists, the tool passes the args directly to the
   Atmos binary.
3. With `ModeAllow`, the permission system auto-approves the call.
4. Production infrastructure is modified without operator awareness.

---

## Root Cause Analysis

### No Subcommand Allowlist

`Execute()` called `strings.Fields(command)` to split the user-controlled string into
args, then passed the result verbatim to `exec.CommandContext`. There was no inspection
of `args[0]` or `args[1]` to determine whether the requested operation was read-only or
state-modifying.

```go
// Before fix — unsafe:
args := strings.Fields(command)
cmd := exec.CommandContext(ctx, t.binaryPath, args...)
```

### No Permission-Mode Gate at the Tool Layer

The `execute_atmos_command` tool delegated all access control to the upstream permission
system (`pkg/ai/tools/permission`). When that system was configured to `ModeAllow`
(auto-approve), any command — including destructive ones — could run without prompting
the user.

---

## Fix

### 1. Subcommand Blocklists

Three static blocklists were added covering all Terraform operations that modify
infrastructure or workspace state:

| Blocklist | Entries |
|---|---|
| `destructiveTerraformSubcmds` | `apply`, `destroy`, `import`, `force-unlock` |
| `destructiveTerraformStateSubcmds` | `state rm`, `state mv`, `state push` |
| `destructiveTerraformWorkspaceSubcmds` | `workspace new`, `workspace delete` |

### 2. `isDestructiveAtmosCommand()` Helper

A new helper function inspects the first 1–3 tokens of the parsed args to determine
whether the requested operation is state-modifying. Matching is case-insensitive
(`strings.ToLower`) so `TERRAFORM APPLY` is treated identically to `terraform apply`.

```go
func isDestructiveAtmosCommand(args []string) bool {
    if len(args) < 2 || strings.ToLower(args[0]) != "terraform" {
        return false
    }
    subCmd := strings.ToLower(args[1])
    if destructiveTerraformSubcmds[subCmd] { return true }
    if subCmd == "state" && len(args) >= 3 {
        return destructiveTerraformStateSubcmds[strings.ToLower(args[2])]
    }
    if subCmd == "workspace" && len(args) >= 3 {
        return destructiveTerraformWorkspaceSubcmds[strings.ToLower(args[2])]
    }
    return false
}
```

### 3. Permission-Mode Gate in `Execute()`

The `ExecuteAtmosCommandTool` struct gains a `permissionMode` field (default:
`permission.ModePrompt`, the safest setting). Before spawning any subprocess, `Execute()`
checks whether the requested command is destructive and whether the current mode permits
it:

- **`ModeAllow`, `ModeDeny`, `ModeYOLO`** — state-modifying commands are rejected
  immediately with `ErrAICommandDestructive`, before any subprocess is created.
- **`ModePrompt`** — state-modifying commands are forwarded to the upstream permission
  system, which presents an explicit confirmation prompt to the operator. The subprocess
  is only created if the operator approves.

```go
if isDestructiveAtmosCommand(args) {
    if t.permissionMode != permission.ModePrompt {
        // Blocked — no subprocess created.
        return &tools.Result{
            Success: false,
            Error:   fmt.Errorf("%w: atmos %s", errUtils.ErrAICommandDestructive, command),
        }, nil
    }
    // ModePrompt: forwarded to upstream permission checker for user confirmation.
    log.Warnf("Destructive Atmos command will require user confirmation: atmos %s", command)
}
```

### 4. New Constructor and Safe Default

`NewExecuteAtmosCommandToolWithPermission(cfg, mode)` allows callers to configure the
permission mode explicitly. The existing `NewExecuteAtmosCommandTool(cfg)` constructor
now defaults to `ModePrompt` (previously there was no mode field at all).

---

## Files Modified

| File | Change |
|---|---|
| `errors/errors.go` | Added `ErrAICommandDestructive` sentinel error |
| `pkg/ai/tools/atmos/execute_command.go` | Added blocklists, `isDestructiveAtmosCommand()`, `permissionMode` field, gate in `Execute()`, new constructor, updated description |
| `pkg/ai/tools/atmos/execute_command_test.go` | Added 87 test cases covering classification, blocking, pass-through, and safe-command scenarios |

---

## Test Coverage

### Classification — `TestIsDestructiveAtmosCommand` (28 cases)

Covers all blocked subcommands, safe read-only subcommands, case-insensitive matching,
and edge cases (empty args, single token, compound subcommands without a secondary token).

### Blocking — `TestExecuteAtmosCommandTool_DestructiveBlocked` (27 cases)

Verifies that each of the 9 destructive command patterns is blocked across all three
non-prompt modes (`ModeAllow`, `ModeDeny`, `ModeYOLO`). Confirms that:

- `result.Success` is `false`.
- `result.Error` contains `"modifies state"`.
- The subprocess binary is never reached (the test binary is set as `binaryPath` but the
  validator fires before any invocation).

### Pass-Through — `TestExecuteAtmosCommandTool_DestructiveAllowedInPromptMode` (1 case)

Verifies that `terraform apply` is **not** blocked by the validator in `ModePrompt`,
confirming the command reaches the execution stage (where the upstream permission system
takes over for user confirmation).

### Safe Commands — `TestExecuteAtmosCommandTool_SafeCommandsAlwaysAllowed` (32 cases)

Verifies that read-only commands (`terraform plan`, `terraform show`, `terraform output`,
`terraform validate`, `terraform state list`, `terraform workspace list`, `describe stacks`,
`list stacks`) are never blocked by the validator in any permission mode.

---

## Backward Compatibility

- `NewExecuteAtmosCommandTool(cfg)` now defaults to `ModePrompt` instead of having no
  mode field. Callers that previously relied on `ModeAllow` to auto-execute destructive
  commands without confirmation will need to switch to
  `NewExecuteAtmosCommandToolWithPermission(cfg, permission.ModePrompt)` and accept the
  user confirmation prompt — or reconsider whether such automation is appropriate.
- Safe (read-only) commands are unaffected in all modes.
- The `execute_atmos_command` tool description now documents the restriction and the
  confirmation flow so AI agents have accurate information about what is permitted.
