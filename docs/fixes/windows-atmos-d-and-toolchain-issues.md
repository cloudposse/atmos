# Windows Toolchain Fixes

## Summary

This document describes Windows-specific issues reported by users and the fixes applied.

| Issue                              | Status             | Description                                              |
|------------------------------------|--------------------|----------------------------------------------------------|
| `.atmos.d` auto-import             | ✅ Verified Working | Configuration loading works correctly on Windows         |
| Toolchain binary `.exe` extension  | ✅ Fixed            | Centralized function ensures `.exe` extension on Windows |
| Download URL `.exe` handling       | ✅ Fixed            | GitHub release URLs get `.exe` for raw binaries          |
| Archive extraction `.exe` handling | ✅ Fixed            | Files extracted correctly from archives                  |
| PowerShell hint message            | ✅ Fixed            | Shows correct `Invoke-Expression` syntax                 |

---

## Issue #1: `.atmos.d` Auto-Import

### Status: ✅ Verified Working

After testing on Windows, the `.atmos.d` auto-import functionality works correctly. No code changes required.

### Improvements Made

Enhanced debug logging in `pkg/config/load.go`:

- Logs at **Debug** level when directories are found.
- Users can diagnose issues with `ATMOS_LOGS_LEVEL=Debug`.

---

## Issue #2: Toolchain Installation Failures

### Reported Problems

1. Binary installed without `.exe` extension - causes `terraform --version` to hang.
2. Download URL missing `.exe` - tools like jq fail with 404 on Windows (e.g., `jq-windows-amd64` vs `jq-windows-amd64.exe`).
3. Archive extraction fails for tools like helm - looking for `windows-amd64/helm` instead of `windows-amd64/helm.exe`.
4. Hint message shows Unix `eval` syntax instead of PowerShell `Invoke-Expression`.

### Architecture: Centralized Windows Extension Handling

Following [Aqua's Windows support approach](https://aquaproj.github.io/docs/reference/windows-support/), Windows executables need the `.exe` extension to be found by `os/exec.LookPath`. We use a centralized function for consistent handling.

**Key design decisions:**

- **Single utility function**: `EnsureWindowsExeExtension()` handles all Windows extension logic in one place.
- **Tool type determines URL handling**:
  - `github_release` type: Automatically adds `.exe` to download URLs for raw binaries (non-archive assets) on Windows. This matches Aqua's behavior.
  - `http` type: No automatic `.exe` handling - the asset template must specify the complete URL including `.exe` if needed.
- **Archive extraction**: Only attempts `.exe` fallback on Windows (not on Unix).

### Download URL Handling by Tool Type

| Tool Type        | Download URL `.exe` Handling                                           |
|------------------|------------------------------------------------------------------------|
| `github_release` | Automatic: adds `.exe` on Windows for raw binaries (no archive ext)    |
| `http`           | Manual: asset template must include `.exe` in URL if needed            |

**Example - `github_release` type (jq):**
```text
Asset template: jq-{{.OS}}-{{.Arch}}
On Windows:     https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-windows-amd64.exe  ✅
On Linux:       https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64       ✅
```

**Example - `http` type (must specify `.exe` in template):**
```text
Asset template: https://example.com/tool-{{.OS}}-{{.Arch}}{{if eq .OS "windows"}}.exe{{end}}
```

### Fixes Applied

| File                               | Fix                                                          |
|------------------------------------|--------------------------------------------------------------|
| `toolchain/installer/installer.go` | Added `EnsureWindowsExeExtension()` centralized function     |
| `toolchain/installer/installer.go` | Uses centralized function for installed binary naming        |
| `toolchain/installer/asset.go`     | Adds `.exe` to GitHub release URLs for raw binaries on Win   |
| `toolchain/installer/extract.go`   | Uses centralized function; `.exe` fallback only on Windows   |
| `toolchain/install_helpers.go`     | Platform-aware hint message for PowerShell                   |

### Centralized Function

```go
// EnsureWindowsExeExtension appends .exe to the binary name on Windows if not present.
// This follows Aqua's behavior where executables need the .exe extension on Windows
// to be found by os/exec.LookPath.
func EnsureWindowsExeExtension(binaryName string) string {
    defer perf.Track(nil, "installer.EnsureWindowsExeExtension")()

    if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(binaryName), ".exe") {
        return binaryName + ".exe"
    }
    return binaryName
}
```

### GitHub Release URL Builder (for raw binaries)

```go
// In buildGitHubReleaseURL():
// On Windows, add .exe to raw binary asset names that don't have an archive extension.
// This follows Aqua's behavior where Windows binaries need .exe extension in the download URL.
if !hasArchiveExtension(assetName) {
    assetName = EnsureWindowsExeExtension(assetName)
}
```

---

## Tests

### Unit Tests

Run toolchain installer tests:

```bash
go test ./toolchain/installer/... -v
```

### Integration Tests

Test file: `tests/toolchain_custom_commands_test.go`

Uses `testhelpers.NewAtmosRunner` for building and running atmos binary (shared infrastructure).

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
