# Windows Issues: .atmos.d Auto-Import and Toolchain Failures

## Summary

This document describes two user-reported issues on Windows:

1. **Issue #1**: Auto-import of `.atmos.d` directory not working on Windows - **VERIFIED WORKING**
2. **Issue #2**: Toolchain installation failures and PATH/config issues on Windows

---

## Issue #1: Auto-Import of .atmos.d Not Working on Windows

### Status: ✅ VERIFIED WORKING

After testing on Windows (January 2025), the `.atmos.d` auto-import functionality **works correctly**.

### Test Results

Testing was performed on Windows using the `atmos-configuration` fixture:

```
PS C:\...\atmos-configuration> $env:ATMOS_LOGS_LEVEL = "Debug"
PS C:\...\atmos-configuration> go run github.com/cloudposse/atmos --help
```

**Debug logs confirm successful loading:**
```
DEBU  Found atmos.d directory, loading configurations path=C:\...\atmos-configuration\atmos.d
DEBU  Loaded configuration directory source=atmos.d files=5 pattern=C:\...\atmos.d\**\*
```

**Custom command from `atmos.d/commands.yaml` appears in help:**
```
AVAILABLE COMMANDS
    ...
    test                                      Run all tests with custom command
```

**`describe config` shows the command loaded correctly:**
```json
{
  "name": "test",
  "description": "Run all tests with custom command",
  "steps": [{"command": "atmos describe config", "type": "shell"}]
}
```

### Original Problem Description

Users reported that configurations placed in the `.atmos.d/` directory were not being loaded on Windows.

### Resolution

The issue was likely caused by:
1. **Lack of visibility**: Errors were logged at `Trace` level, making debugging difficult
2. **User configuration issues**: The `.atmos.d` directory may not have existed or contained valid YAML

### Improvements Made

1. **Enhanced debug logging** (`pkg/config/load.go`):
   - Now logs at `Debug` level when directories are found
   - Users can diagnose issues with `ATMOS_LOGS_LEVEL=Debug` instead of requiring `Trace`

2. **Explicit directory existence check**:
   - Directory is checked with `os.Stat()` before attempting glob search
   - Clearer log messages distinguish between "directory not found" vs "failed to load"

### Test Plan for Manual Verification

#### Prerequisites

- Windows computer with Go installed
- Clone the Atmos repository
- Build Atmos: `go build -o atmos.exe .`

#### Option A: Use Existing Test Fixture

The repository includes a test fixture specifically for testing `atmos.d` configuration loading:

```powershell
# Navigate to the test fixture
cd tests\fixtures\scenarios\atmos-configuration

# Build atmos (from repo root)
cd ..\..\..\..
go build -o atmos.exe .

# Return to fixture
cd tests\fixtures\scenarios\atmos-configuration

# Enable debug logging to see atmos.d discovery
$env:ATMOS_LOGS_LEVEL = "Debug"

# Check if the "test" custom command from atmos.d/commands.yaml is loaded
..\..\..\..\atmos.exe --help
# Expected: "test" command should appear in help output

# Run the custom command
..\..\..\..\atmos.exe test
# Expected: Should run "atmos describe config"

# Check config loading and verify commands are loaded
..\..\..\..\atmos.exe describe config
# Look for "commands" section in output - should show the "test" command
```

**Fixture Structure** (`tests/fixtures/scenarios/atmos-configuration`):

```
atmos-configuration/
├── atmos.yaml           # Main config with base_path: "./"
└── atmos.d/
    ├── commands.yaml    # Custom command "test"
    ├── logs.yaml        # Logging configuration
    └── tools/           # Nested subdirectory
```

#### Option B: Create Fresh Test Directory

1. **Create test directory structure**:
   ```powershell
   mkdir test-atmos-d
   cd test-atmos-d
   git init
   mkdir .atmos.d
   ```

2. **Create `.atmos.d/custom-commands.yaml`**:
   ```yaml
   commands:
     - name: test-windows
       description: "Test command from .atmos.d on Windows"
       steps:
         - echo "Hello from .atmos.d on Windows!"
   ```

3. **Create minimal `atmos.yaml`**:
   ```yaml
   base_path: "."
   ```

4. **Run test commands**:
   ```powershell
   # Enable debug logging to see .atmos.d discovery
   $env:ATMOS_LOGS_LEVEL = "Debug"

   # Check if custom command is available
   .\atmos.exe --help

   # Look for the test-windows command in help output
   # Expected: "test-windows" command should appear

   # Run the custom command
   .\atmos.exe test-windows
   # Expected: "Hello from .atmos.d on Windows!"

   # Check config loading
   .\atmos.exe describe config
   # Look for commands in the output
   ```

5. **Verify glob pattern behavior**:
   ```powershell
   # Create nested structure
   mkdir .atmos.d\subdir
   echo "test_key: test_value" > .atmos.d\subdir\nested.yaml

   # Check if nested config is loaded
   .\atmos.exe describe config
   ```

---

## Issue #2: Toolchain Failures and Config Issues on Windows

### Problem Description

Users report multiple toolchain-related issues on Windows:

1. `atmos toolchain install gum@latest` fails with errors
2. After running `eval "$(atmos toolchain env)"` in PowerShell, `gum` is not found in PATH
3. General config issues (atmos.yaml not found)

### Test Results (January 2025)

Testing on Windows revealed:

**Working ✅:**
- `atmos toolchain install hashicorp/terraform@1.9.8` - installs successfully
- `atmos toolchain env --format powershell` - outputs correct PowerShell syntax
- PowerShell hint message now shows correct syntax
- `Invoke-Expression (atmos toolchain env --format powershell)` - no errors

**Not Working ❌:**
- `terraform --version` **hangs indefinitely** after PATH is updated
- Root cause: Binary installed without `.exe` extension

### Root Cause Analysis

#### 1. Missing .exe Extension on Windows (CONFIRMED BUG - FIXED)

**File**: `toolchain/installer/installer.go:448-450`

The binary was being installed without the `.exe` extension:
```go
binaryName := resolveBinaryName(tool)  // Returns "terraform"
binaryPath := filepath.Join(versionDir, binaryName)  // ".tools/.../terraform" (no .exe!)
```

Installation output showed:
```
✓ Installed hashicorp/terraform@1.9.8 to .tools\bin\hashicorp\terraform\1.9.8\terraform (86mb)
```

On Windows, the shell only recognizes executables with `.exe`, `.cmd`, `.bat`, or `.com` extensions. Without `.exe`, the binary cannot be found even though it's in the PATH.

**Fix Applied**: Added Windows-specific handling to append `.exe`:
```go
if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(binaryName), ".exe") {
    binaryName += ".exe"
}
```

#### 2. Unix-style Hint Message on Windows (FIXED)

**Files**: `toolchain/install.go:356`, `toolchain/install_helpers.go:130,139`

The hint message after installation was showing Unix/bash syntax on Windows.

**Fix Applied**: Added `getPlatformPathHint()` function that returns platform-appropriate syntax:
- **Windows**: `Invoke-Expression (atmos --chdir /path/to/project toolchain env --format powershell)`
- **Unix/macOS**: `eval "$(atmos --chdir /path/to/project toolchain env)"`

This is confusing for Windows users who try to use the Unix syntax in PowerShell.

#### 2. PATH Separator Issue

**File**: `toolchain/path_helpers.go:104-135`

The `getCurrentPath()` and `constructFinalPath()` functions use `os.PathListSeparator`:

```go
func constructFinalPath(pathEntries []string, currentPath string) string {
return strings.Join(pathEntries, string(os.PathListSeparator)) +
string(os.PathListSeparator) + currentPath
}
```

This should correctly use `;` on Windows. However, the issue may be in how the path is quoted or escaped in PowerShell
output.

#### 2. PowerShell Output Format

**File**: `toolchain/env.go:156-160`

```go
func formatPowershellContent(finalPath string) string {
safe := strings.ReplaceAll(finalPath, "\"", "`\"")
safe = strings.ReplaceAll(safe, "$", "`$")
return fmt.Sprintf("$env:PATH = \"%s\"\n", safe)
}
```

This may not handle all Windows path edge cases:

- Paths with spaces (e.g., `C:\Program Files\...`)
- UNC paths (e.g., `\\server\share`)
- Very long paths (Windows MAX_PATH limit)

#### 3. XDG Directory on Windows

**File**: `toolchain/setup.go:54-81`

The `GetInstallPath()` function uses XDG directories:

```go
func GetInstallPath() string {
// If explicitly configured, use that path
if atmosConfig != nil && atmosConfig.Toolchain.InstallPath != "" {
return atmosConfig.Toolchain.InstallPath
}

// Try to use XDG-compliant data directory
dataDir, err := xdg.GetXDGDataDir("toolchain", defaultDirPermissions)
// ...
}
```

On Windows, `xdg.GetXDGDataDir()` may return a path in `%LOCALAPPDATA%` or `%APPDATA%` which could have permission or
path format issues.

#### 4. Binary Extraction and Permissions

Windows doesn't have Unix-style executable permissions. The toolchain installer needs to handle Windows `.exe`
extensions and may need to handle Windows Defender/SmartScreen issues.

### Affected Files

- `toolchain/install.go` - Main installation logic
- `toolchain/path_helpers.go` - PATH construction
- `toolchain/env.go` - Environment variable output
- `toolchain/setup.go` - Install path resolution
- `pkg/xdg/xdg.go` - XDG directory resolution
- `toolchain/installer/` - Binary download and extraction

### Proposed Fix

1. **Fix PowerShell escaping**: Ensure paths with spaces and special characters are properly escaped
2. **Add Windows binary extension handling**: Ensure `.exe` is appended when needed
3. **Test XDG paths on Windows**: Verify `%LOCALAPPDATA%\atmos\toolchain` is created correctly
4. **Add Windows integration tests**: Add Windows-specific toolchain tests

### Test Plan for Manual Verification

#### Prerequisites

- Windows computer with PowerShell
- Clone the Atmos repository
- Build Atmos: `go build -o atmos.exe .`

#### Option A: Use Existing Test Fixture

The repository includes a test fixture for toolchain integration:

```powershell
# Navigate to the test fixture
cd tests\fixtures\scenarios\toolchain-terraform-integration

# Build atmos (from repo root)
cd ..\..\..\..
go build -o atmos.exe .

# Return to fixture
cd tests\fixtures\scenarios\toolchain-terraform-integration

# Check existing .tool-versions file
type .tool-versions
# Contents: k9s 0.32.7, kubectl 1.28.0, terraform 1.9.8

# Check toolchain configuration in atmos.yaml
type atmos.yaml
# Shows: toolchain.install_path: ".tools"

# Test toolchain install (using terraform as example)
..\..\..\..\atmos.exe toolchain install hashicorp/terraform@1.9.8

# Verify the hint shows PowerShell syntax (not bash)
# Expected: "Invoke-Expression (atmos --chdir /path/to/project toolchain env --format powershell)"

# Check installation directory
dir .tools
# Expected: Should see terraform directory structure

# Test toolchain env output
..\..\..\..\atmos.exe toolchain env --format powershell
# Expected: $env:PATH = "..." with semicolon separators

# Apply PATH and verify
Invoke-Expression (..\..\..\..\atmos.exe toolchain env --format powershell)
terraform --version
# Expected: Terraform version output
```

**Fixture Structure** (`tests/fixtures/scenarios/toolchain-terraform-integration`):

```
toolchain-terraform-integration/
├── .tool-versions       # k9s 0.32.7, kubectl 1.28.0, terraform 1.9.8
├── atmos.yaml           # toolchain.install_path: ".tools"
├── components/terraform/
└── stacks/
```

#### Option B: Create Fresh Test Directory

1. **Test toolchain configuration**:
   ```powershell
   # Create test project
   mkdir test-toolchain
   cd test-toolchain
   git init

   # Create atmos.yaml
   @"
   base_path: "."
   toolchain:
     install_path: ".tools"
     versions_file: ".tool-versions"
   "@ | Out-File -FilePath atmos.yaml -Encoding utf8
   ```

2. **Test toolchain install**:
   ```powershell
   # Install gum
   .\atmos.exe toolchain install charmbracelet/gum@latest

   # Check installation directory
   dir .tools
   # Expected: Should see gum directory structure

   # Check .tool-versions file
   type .tool-versions
   # Expected: Should contain gum entry
   ```

3. **Test toolchain env output**:
   ```powershell
   # Get PowerShell format
   .\atmos.exe toolchain env --format powershell

   # Expected output format:
   # $env:PATH = "C:\path\to\tool;C:\existing\path"

   # Verify the path is valid
   $output = .\atmos.exe toolchain env --format powershell
   Write-Host "Output: $output"

   # Check for proper escaping of paths with spaces
   ```

4. **Test PATH update**:
   ```powershell
   # Apply PATH update
   Invoke-Expression (.\atmos.exe toolchain env --format powershell)

   # Verify gum is accessible
   gum --version
   # Expected: Should show gum version

   # Check PATH
   $env:PATH -split ";"
   # Expected: Should include .tools directory
   ```

5. **Test with paths containing spaces**:
   ```powershell
   # Create directory with spaces
   mkdir "C:\Test Path With Spaces\atmos-test"
   cd "C:\Test Path With Spaces\atmos-test"
   git init

   # Create atmos.yaml with custom install path
   @"
   base_path: "."
   toolchain:
     install_path: "my tools"
   "@ | Out-File -FilePath atmos.yaml -Encoding utf8

   # Test install
   .\atmos.exe toolchain install charmbracelet/gum@latest

   # Test env output - check for proper quoting
   .\atmos.exe toolchain env --format powershell
   ```

6. **Test error handling**:
   ```powershell
   # Test without atmos.yaml
   mkdir test-no-config
   cd test-no-config
   .\atmos.exe toolchain env --format powershell
   # Expected: Graceful error message about missing config
   ```

---

## Cross-Issue Considerations

### Common Patterns

Both issues share common Windows-specific concerns:

1. **Path separators**: Windows uses `\` vs Unix `/`
2. **Case insensitivity**: Windows paths are case-insensitive
3. **Environment variable format**: PATH uses `;` on Windows vs `:` on Unix
4. **File permissions**: Windows doesn't use Unix permission bits

### Testing Strategy

1. **Unit tests**: Add `_windows_test.go` files for Windows-specific tests
2. **Integration tests**: Add Windows CI workflow or test on Windows locally
3. **Manual testing**: Follow the test plans above on a Windows machine

### Debug Commands

For both issues, use these debug commands:

```powershell
# Enable verbose logging
$env:ATMOS_LOGS_LEVEL = "Trace"

# Check config loading
.\atmos.exe describe config 2>&1 | Out-File debug.log

# Check toolchain paths
.\atmos.exe toolchain info

# Check environment
Get-ChildItem Env: | Where-Object { $_.Name -like "ATMOS*" }
```

---

## Fixes Applied

### Fix 1: Platform-Aware PATH Hint Message

**Files Modified**:

- `toolchain/install_helpers.go` - Added `getPlatformPathHint()` function and updated hint calls
- `toolchain/install.go` - Updated `printSuccessSummary()` to use platform-aware hint

**Change**:
The hint message after `atmos toolchain install` now shows platform-appropriate syntax:

- **Windows**: `Invoke-Expression (atmos --chdir /path/to/project toolchain env --format powershell)`
- **Unix/macOS**: `eval "$(atmos --chdir /path/to/project toolchain env)"`

### Fix 2: Improved Debug Logging for .atmos.d Discovery

**File Modified**: `pkg/config/load.go`

**Change**:
The `loadAtmosDFromDirectory()` function now:

1. Explicitly checks if `atmos.d/` or `.atmos.d/` directories exist before searching
2. Logs at **Debug** level when directories are found (previously only logged errors at Trace level)
3. Users can now see `.atmos.d` discovery by setting `ATMOS_LOGS_LEVEL=Debug` instead of requiring Trace

**Debug output example**:

```
DEBUG Found .atmos.d directory, loading configurations path=C:\Users\user\project\.atmos.d
```

---

## Implementation Priority

1. **High**: Fix `.atmos.d` loading on Windows (Issue #1) - affects all custom commands
2. **High**: Fix toolchain PATH output for PowerShell (Issue #2) - affects toolchain usability
3. **Medium**: Add Windows-specific tests to prevent regression
4. **Low**: Improve error messages for Windows-specific failures

## Related Code Locations

| Component          | File                              | Key Functions                                        |
|--------------------|-----------------------------------|------------------------------------------------------|
| .atmos.d loading   | `pkg/config/load.go`              | `loadAtmosDFromDirectory()`, `mergeDefaultImports()` |
| Git root discovery | `pkg/config/git_root.go`          | `hasLocalAtmosConfig()`, `applyGitRootBasePath()`    |
| Toolchain install  | `toolchain/install.go`            | `InstallSingleTool()`, `RunInstall()`                |
| PATH helpers       | `toolchain/path_helpers.go`       | `buildPathEntries()`, `constructFinalPath()`         |
| Env output         | `toolchain/env.go`                | `EmitEnv()`, `formatPowershellContent()`             |
| XDG directories    | `pkg/xdg/xdg.go`                  | `GetXDGDataDir()`                                    |
| Windows tests      | `pkg/config/load_windows_test.go` | Existing Windows path tests                          |

## Success Criteria

After fixes are implemented:

1. `.atmos.d/` configs are loaded on Windows (verify with `atmos describe config`)
2. Custom commands from `.atmos.d/` appear in `atmos --help`
3. `atmos toolchain install` succeeds on Windows
4. `Invoke-Expression (atmos toolchain env --format powershell)` correctly updates PATH
5. Installed tools are executable after PATH update
6. All Windows CI tests pass
