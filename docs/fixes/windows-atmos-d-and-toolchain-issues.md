# Windows Toolchain Fixes

## Summary

This document describes Windows-specific issues reported by users and the fixes applied.

| Issue                              | Status             | Description                                      |
|------------------------------------|--------------------|--------------------------------------------------|
| `.atmos.d` auto-import             | ✅ Verified Working | Configuration loading works correctly on Windows |
| Toolchain binary `.exe` extension  | ✅ Fixed            | Binaries now installed with `.exe` extension     |
| Archive extraction `.exe` handling | ✅ Fixed            | Files extracted correctly from archives          |
| Asset download URL `.exe` fallback | ✅ Fixed            | Downloads now try `.exe` suffix on Windows       |
| PowerShell hint message            | ✅ Fixed            | Shows correct `Invoke-Expression` syntax         |

---

## Issue #1: `.atmos.d` Auto-Import

### Status: ✅ Verified Working

After testing on Windows, the `.atmos.d` auto-import functionality works correctly. No code changes required.

### Improvements Made

Enhanced debug logging in `pkg/config/load.go`:

- Logs at **Debug** level when directories are found
- Users can diagnose issues with `ATMOS_LOGS_LEVEL=Debug`

---

## Issue #2: Toolchain Installation Failures

### Reported Problems

1. Binary installed without `.exe` extension - causes `terraform --version` to hang
2. Archive extraction fails for tools like helm - looking for `windows-amd64/helm` instead of `windows-amd64/helm.exe`
3. Asset download fails for tools like jq - URL missing `.exe` suffix
4. Hint message shows Unix `eval` syntax instead of PowerShell `Invoke-Expression`

### Fixes Applied

| File                               | Fix                                                |
|------------------------------------|----------------------------------------------------|
| `toolchain/installer/installer.go` | Append `.exe` to binary name on Windows            |
| `toolchain/installer/extract.go`   | Try `.exe` extension when extracting from archives |
| `toolchain/installer/download.go`  | Try `.exe` suffix in download URL on Windows       |
| `toolchain/install_helpers.go`     | Platform-aware hint message                        |

---

## Tests

### Unit Tests

Run toolchain installer tests:

```bash
go test ./toolchain/installer/... -v
```

### Integration Tests

Test file: `tests/toolchain_custom_commands_test.go`

| Test                                                  | Description                                                    |
|-------------------------------------------------------|----------------------------------------------------------------|
| `TestToolchainCustomCommands_InstallAllTools`         | Installs gum, k9s, helm, jq, tofu and verifies `.exe` binaries |
| `TestToolchainCustomCommands_ToolsExecutable`         | Verifies tools execute `--version` correctly                   |
| `TestToolchainCustomCommands_PathEnvOutput`           | Tests bash and PowerShell format output                        |
| `TestToolchainCustomCommands_WindowsExeExtension`     | Verifies `.exe` extension on Windows                           |
| `TestToolchainCustomCommands_CustomCommandsLoaded`    | Verifies custom commands appear in help                        |
| `TestToolchainCustomCommands_ExecuteWithDependencies` | Tests custom commands with `dependencies.tools`                |

**Run on macOS/Linux:**

```bash
go test -v -run "TestToolchainCustomCommands" ./tests/... -timeout 10m
```

**Run on Windows:**

```powershell
go test -v -run "TestToolchainCustomCommands" .\tests\... -timeout 10m
```

### Manual CLI Testing

**Test Fixture:** `tests/fixtures/scenarios/toolchain-custom-commands`

#### Windows (PowerShell)

```powershell
# Navigate to test fixture
cd tests\fixtures\scenarios\toolchain-custom-commands

# Build atmos
cd ..\..\..\..
go build -o atmos.exe .
cd tests\fixtures\scenarios\toolchain-custom-commands

# Install tools
..\..\..\..\atmos.exe toolchain install charmbracelet/gum@0.17.0

# Verify .exe extension in output
# Expected: ✓ Installed charmbracelet/gum@0.17.0 to .tools\bin\...\gum.exe

# Verify PowerShell hint
# Expected: 💡 Export the PATH ... using Invoke-Expression (atmos ... --format powershell)

# Set PATH and test
Invoke-Expression (..\..\..\..\atmos.exe toolchain env --format powershell)
gum --version

# Test custom command with dependencies
..\..\..\..\atmos.exe test-gum
```

#### macOS/Linux

```bash
# Navigate to test fixture
cd tests/fixtures/scenarios/toolchain-custom-commands

# Build atmos
cd ../../../..
go build -o atmos .
cd tests/fixtures/scenarios/toolchain-custom-commands

# Install tools
../../../../atmos toolchain install charmbracelet/gum@0.17.0

# Set PATH and test
eval "$(../../../../atmos toolchain env)"
gum --version

# Test custom command with dependencies
../../../../atmos test-gum
```

---

## Test Results (Windows)

All tests pass on Windows.

### Installation Output

Tools are installed with `.exe` extension and display the PowerShell hint:

```text
✓ Installed charmbracelet/gum@0.17.0 to .tools\bin\charmbracelet\gum\0.17.0\gum.exe (13mb)
💡 Export the PATH environment variable for your toolchain tools using Invoke-Expression (atmos --chdir /path/to/project toolchain env --format powershell)

✓ Installed derailed/k9s@0.32.7 to .tools\bin\derailed\k9s\0.32.7\k9s.exe (97mb)

✓ Installed helm/helm@3.16.3 to .tools\bin\helm\helm\3.16.3\helm.exe (55mb)

✓ Installed jqlang/jq@1.7.1 to .tools\bin\jqlang\jq\1.7.1\jq.exe (962kb)

✓ Installed opentofu/opentofu@1.9.0 to .tools\bin\opentofu\opentofu\1.9.0\tofu.exe (83mb)
```

### Tool Version Verification

After setting PATH with `Invoke-Expression (atmos toolchain env --format powershell)`:

```text
> gum --version
gum version v0.17.0 (6045525)

> jq --version
jq-1.7.1

> helm version --short
v3.16.3+gcfd0749

> tofu version
OpenTofu v1.9.0
on windows_amd64
```

### Custom Command Execution with Dependencies

Running `atmos test-jq` automatically installs all dependencies and executes:

```text
✓ Installed charmbracelet/gum@0.17.0
✓ Installed derailed/k9s@0.32.7
✓ Installed helm/helm@3.16.3
✓ Installed jqlang/jq@1.7.1
✓ Installed opentofu/opentofu@1.9.0

✓ Installed 5 tools
```

### Integration Test Summary

```text
--- PASS: TestToolchainCustomCommands_InstallAllTools (14.04s)
    --- PASS: Install_gum (0.89s) - .tools\bin\charmbracelet\gum\0.17.0\gum.exe
    --- PASS: Install_k9s (1.36s) - .tools\bin\derailed\k9s\0.32.7\k9s.exe
    --- PASS: Install_helm (1.42s) - .tools\bin\helm\helm\3.16.3\helm.exe
    --- PASS: Install_jq (1.06s) - .tools\bin\jqlang\jq\1.7.1\jq.exe
    --- PASS: Install_tofu (1.09s) - .tools\bin\opentofu\opentofu\1.9.0\tofu.exe
--- PASS: TestToolchainCustomCommands_ToolsExecutable (12.33s)
    --- PASS: Execute_gum (0.96s)
    --- PASS: Execute_jq (0.87s)
    --- PASS: Execute_helm (1.54s)
    --- PASS: Execute_tofu (1.25s)
--- PASS: TestToolchainCustomCommands_PathEnvOutput (10.09s)
    --- PASS: BashFormat (0.63s)
    --- PASS: PowershellFormat (0.63s)
--- PASS: TestToolchainCustomCommands_WindowsExeExtension (8.91s)
--- PASS: TestToolchainCustomCommands_CustomCommandsLoaded (8.31s)
--- PASS: TestToolchainCustomCommands_ExecuteWithDependencies (14.50s)
    --- PASS: test-jq (4.40s)
    --- PASS: test-gum (0.59s)
    --- PASS: test-helm (0.78s)
    --- PASS: test-tofu (0.76s)
PASS
ok      github.com/cloudposse/atmos/tests       68.332s
```
