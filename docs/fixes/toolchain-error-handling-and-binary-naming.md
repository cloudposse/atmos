# Toolchain Error Handling and Binary Naming Fixes

**Date:** 2026-01-25

## Summary

This document describes fixes for toolchain error handling, glamour warning suppression, binary naming issues,
and platform compatibility improvements discovered during Windows testing.

| Issue                                | Status  | Description                                                          |
|--------------------------------------|---------|----------------------------------------------------------------------|
| Glamour "Warning: unhandled element" | ✅ Fixed | Suppressed confusing warning messages from glamour markdown renderer |
| HTTP 404 error messages              | ✅ Fixed | Improved error messages with platform-specific hints and context     |
| Binary naming for 3-segment packages | ✅ Fixed | kubectl now correctly named `kubectl` instead of `kubernetes`        |
| Nested error message duplication     | ✅ Fixed | Eliminated "HTTP request failed: HTTP request failed:" pattern       |
| Pre-flight platform check            | ✅ Fixed | Check platform compatibility before attempting download              |
| Platform-specific hints              | ✅ Fixed | WSL hints for Windows, Rosetta hints for macOS arm64                 |
| Arch-only platform matching          | ✅ Fixed | Supports `amd64`/`arm64` entries in `supported_envs` (Aqua format)   |

---

## Issue #1: Glamour "Warning: unhandled element" Messages

### Reported Problem

Users saw confusing warning messages in terminal output:

```console
Warning: unhandled element String
Warning: unhandled element String
✗ Install failed : HTTP request failed: ...
```

These warnings were not informative and scared users into thinking something was wrong.

### Root Cause

The `charmbracelet/glamour` library (v0.10.0) internally prints warnings to stdout when it encounters markdown elements
it doesn't handle. This happens in `charmbracelet/glamour@v0.10.0/ansi/elements.go` using `fmt.Fprintf(os.Stdout, ...)`.

Since `glamour` writes directly to `os.Stdout`, these warnings appear in the terminal regardless of Atmos's UI output
configuration.

### Solution

Modified `pkg/ui/markdown/custom_renderer.go` to redirect stdout to `os.DevNull` during glamour rendering:

```go
// stdoutRedirectMu serializes stdout redirects during rendering to prevent races.
var stdoutRedirectMu sync.Mutex

func (r *CustomRenderer) Render(content string) (string, error) {
    // Suppress glamour's "Warning: unhandled element" messages that it prints to stdout.
    // These warnings are not useful to users and can be confusing.
    //
    // The mutex serializes stdout redirects to prevent races when Render() is called
    // concurrently or when other goroutines write to stdout during rendering.
    stdoutRedirectMu.Lock()
    defer stdoutRedirectMu.Unlock()

    oldStdout := os.Stdout
    devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
    if err == nil {
        os.Stdout = devNull
        defer func() {
            os.Stdout = oldStdout
            devNull.Close()
        }()
    }
    return r.glamour.Render(content)
}
```

This approach:

- Uses a mutex to prevent race conditions when Render() is called concurrently.
- Opens `/dev/null` with write-only mode (`os.O_WRONLY`) for correctness.
- Temporarily redirects stdout during rendering.
- Restores stdout after rendering completes.
- Uses `os.DevNull` which works on all platforms (Windows, macOS, Linux).
- Fails gracefully if `/dev/null` cannot be opened.

### Files Changed

| File                                 | Change                                            |
|--------------------------------------|---------------------------------------------------|
| `pkg/ui/markdown/custom_renderer.go` | Added stdout redirection during glamour rendering |

---

## Issue #2: Poor HTTP 404 Error Messages

### Reported Problem

When a tool download failed with HTTP 404, the error message was unhelpful:

```console
✗ Install failed : HTTP request failed: HTTP request failed: tried
https://github.com/replicatedhq/replicated/releases/download/0.124.1/replicated_0.124.1_windows_amd64.tar.gz
and https://github.com/replicatedhq/replicated/releases/download/v0.124.1/replicated_0.124.1_windows_amd64.tar.gz:
HTTP 404 Not Found
download failed
```

Issues with the old message:

1. **Nested error wrapping** caused duplicate "HTTP request failed: HTTP request failed:" text.
2. **No guidance** for users on what to do next.
3. **No platform context** to explain why the download might fail.

### Solution

Added `buildDownloadNotFoundError()` function in `toolchain/installer/download.go` that creates user-friendly errors
with hints:

```go
// buildDownloadNotFoundError creates a user-friendly error for when both URL attempts fail.
func buildDownloadNotFoundError(owner, repo, version, url1, url2 string) error {
    return errors.Join(
        ErrHTTP404,
        errUtils.Build(errUtils.ErrDownloadFailed).
            WithExplanationf("Asset not found for `%s/%s@%s`", owner, repo, version).
            WithHint("Verify the tool name and version are correct").
            WithHintf("Check if the tool publishes binaries for your platform (%s/%s)", getOS(), getArch()).
            WithHint("Some tools may not publish pre-built binaries - check the tool's releases page").
            WithContext("url_attempted", url1).
            WithContext("url_fallback", url2).
            WithExitCode(1).
            Err(),
    )
}

func getOS() string {
    return runtime.GOOS
}

func getArch() string {
    return runtime.GOARCH
}
```

### New Error Output

```console
✗ Install failed : tool download failed

# Error

**Error:** tool download failed

## Explanation

Asset not found for `replicatedhq/replicated@0.124.1`

## Hints

💡 Verify the tool name and version are correct

💡 Check if the tool publishes binaries for your platform (darwin/arm64)

💡 Some tools may not publish pre-built binaries - check the tool's releases page

## Context

- url_attempted: https://github.com/replicatedhq/replicated/releases/download/0.124.1/replicated_0.124.1_darwin_arm64.tar.gz
- url_fallback: https://github.com/replicatedhq/replicated/releases/download/v0.124.1/replicated_0.124.1_darwin_arm64.tar.gz
```

### Files Changed

| File                              | Change                                                   |
|-----------------------------------|----------------------------------------------------------|
| `toolchain/installer/download.go` | Added `buildDownloadNotFoundError()` function            |
| `toolchain/installer/download.go` | Added `getOS()` and `getArch()` helper functions         |
| `toolchain/installer/download.go` | Updated `tryFallbackVersion()` to use new error function |

---

## Issue #3: kubectl Binary Named "kubernetes" Instead of "kubectl"

### Reported Problem

When installing kubectl, the binary was incorrectly named `kubernetes` instead of `kubectl`:

```text
.tools/bin/kubernetes/kubectl/1.31.4/kubernetes    # Wrong!
```

This caused confusion and broke scripts expecting the `kubectl` binary name.

### Root Cause

The Aqua registry entry for kubectl has:

- `name: kubernetes/kubernetes/kubectl` (three-segment package name)
- `repo_name: kubernetes`

The code was falling back to `repo_name` for the binary name instead of extracting it from the package name. Aqua's
convention uses the **last segment** of multi-part package names as the binary name.

### Aqua Package Name Convention

| Package Name                    | Expected Binary | Explanation                              |
|---------------------------------|-----------------|------------------------------------------|
| `kubernetes/kubernetes/kubectl` | `kubectl`       | 3 segments → last segment is binary name |
| `hashicorp/terraform`           | `terraform`     | 2 segments → use repo_name               |
| `owner/repo/subdir/binary`      | `binary`        | 4 segments → last segment is binary name |

### Solution

Added `extractBinaryNameFromPackageName()` and `resolveBinaryName()` helper functions in `toolchain/registry/aqua/aqua.go`:

```go
// extractBinaryNameFromPackageName extracts the binary name from an Aqua package name.
// For packages with 3+ segments (e.g., "kubernetes/kubernetes/kubectl"), the last segment
// is the binary name. For standard 2-segment names (e.g., "hashicorp/terraform"),
// this returns empty string and the caller should fall back to repo_name.
func extractBinaryNameFromPackageName(packageName string) string {
    if packageName == "" {
        return ""
    }
    parts := strings.Split(packageName, "/")
    if len(parts) > 2 {
        return parts[len(parts)-1]
    }
    return ""
}

// resolveBinaryName determines the binary name using Aqua's resolution order:
// 1. Use explicit binary_name if set
// 2. Extract last segment from package name (e.g., "kubectl" from "kubernetes/kubernetes/kubectl")
// 3. Fall back to repo_name.
func resolveBinaryName(binaryName, packageName, repoName string) string {
    if binaryName != "" {
        return binaryName
    }
    if name := extractBinaryNameFromPackageName(packageName); name != "" {
        return name
    }
    return repoName
}
```

The `resolveBinaryName()` helper is used in both `parseRegistryFile()` and `resolveVersionOverrides()` to consolidate
the binary name resolution logic and reduce code duplication.

**Binary name resolution order:**

1. Explicit `binary_name` field (if set in registry)
2. **Extracted from package name** (for 3+ segment names)
3. Fall back to `repo_name`

### Files Changed

| File                                   | Change                                                 |
|----------------------------------------|--------------------------------------------------------|
| `toolchain/registry/aqua/aqua.go`      | Added `Name` field to `registryPackage` struct         |
| `toolchain/registry/aqua/aqua.go`      | Added `extractBinaryNameFromPackageName()` function    |
| `toolchain/registry/aqua/aqua.go`      | Added `resolveBinaryName()` helper function            |
| `toolchain/registry/aqua/aqua.go`      | Updated `parseRegistryFile()` to use new binary naming |
| `toolchain/registry/aqua/aqua.go`      | Updated `resolveVersionOverrides()` to use new naming  |
| `toolchain/registry/aqua/aqua_test.go` | Added tests for `extractBinaryNameFromPackageName()`   |

### Tests Added

```go
func TestExtractBinaryNameFromPackageName(t *testing.T) {
    tests := []struct {
        name        string
        packageName string
        expected    string
    }{
        {"three_segment_package_name", "kubernetes/kubernetes/kubectl", "kubectl"},
        {"two_segment_package_name_returns_empty", "hashicorp/terraform", ""},
        {"four_segment_package_name", "owner/repo/subdir/binary", "binary"},
        {"empty_package_name", "", ""},
        {"single_segment_returns_empty", "terraform", ""},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            result := extractBinaryNameFromPackageName(tc.packageName)
            assert.Equal(t, tc.expected, result)
        })
    }
}
```

### Result

```console
✓ Installed kubernetes/kubectl@1.31.4 to .tools/bin/kubernetes/kubectl/1.31.4/kubectl
```

---

## Issue #4: Pre-flight Platform Compatibility Check

### Reported Problem

When a user tried to install a tool that doesn't support their platform (e.g., `replicatedhq/replicated` on Windows or
macOS arm64), Atmos would attempt to download the binary and only fail after HTTP 404 errors from GitHub. This wasted
time and network resources, and the error messages weren't specific about platform compatibility.

### Root Cause

The Aqua registry includes a `supported_envs` field that specifies which platforms a tool supports (e.g., `["darwin",
"linux"]`), but Atmos wasn't reading or using this information before attempting downloads.

### Solution

Added a pre-flight platform compatibility check that runs before any download attempt:

1. **Added `SupportedEnvs` field to Tool struct** (`toolchain/registry/registry.go`):

```go
type Tool struct {
    // ... other fields ...
    SupportedEnvs    []string          `yaml:"supported_envs"` // Supported platforms (e.g., "darwin", "linux", "windows", "darwin/amd64").
}
```

2. **Propagated `SupportedEnvs` from Aqua registry** (`toolchain/registry/aqua/aqua.go`):

```go
type registryPackage struct {
    // ... other fields ...
    SupportedEnvs    []string                `yaml:"supported_envs"` // Supported platforms.
}

// In both parseRegistryFile() and resolveVersionOverrides():
tool := &registry.Tool{
    // ... other fields ...
    SupportedEnvs: pkgDef.SupportedEnvs,
}
```

3. **Created platform check function** (`toolchain/installer/platform.go`):

```go
// CheckPlatformSupport checks if the current platform is supported by the tool.
func CheckPlatformSupport(tool *registry.Tool) *PlatformError {
    if len(tool.SupportedEnvs) == 0 {
        return nil // No restrictions, all platforms supported.
    }
    currentOS := runtime.GOOS
    currentArch := runtime.GOARCH
    for _, env := range tool.SupportedEnvs {
        if isPlatformMatch(env, currentOS, currentArch) {
            return nil // Platform supported.
        }
    }
    return &PlatformError{
        Tool:          fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName),
        CurrentEnv:    fmt.Sprintf("%s/%s", currentOS, currentArch),
        SupportedEnvs: tool.SupportedEnvs,
        Hints:         buildPlatformHints(currentOS, currentArch, tool.SupportedEnvs),
    }
}

// isPlatformMatch checks if a supported_env entry matches the current platform.
// Supported formats (following Aqua registry conventions):
//   - "darwin" - matches any darwin architecture
//   - "linux" - matches any linux architecture
//   - "windows" - matches any windows architecture
//   - "amd64" - matches any OS with amd64 architecture
//   - "arm64" - matches any OS with arm64 architecture
//   - "darwin/amd64" - matches specific OS/arch
//   - "linux/arm64" - matches specific OS/arch
func isPlatformMatch(env, currentOS, currentArch string) bool {
    env = strings.ToLower(strings.TrimSpace(env))

    // Check for exact OS/arch match.
    if strings.Contains(env, "/") {
        parts := strings.Split(env, "/")
        if len(parts) == 2 {
            return parts[0] == currentOS && parts[1] == currentArch
        }
    }

    // Check for arch-only match (any OS with this architecture).
    // Aqua registry uses entries like "amd64" to mean "any OS with amd64".
    if isKnownArch(env) {
        return env == currentArch
    }

    // Check for OS-only match (any architecture).
    return env == currentOS
}

// isKnownArch returns true if the string is a known Go architecture name.
func isKnownArch(s string) bool {
    knownArchs := map[string]bool{
        "amd64": true, "arm64": true, "386": true, "arm": true,
        "ppc64": true, "ppc64le": true, "mips": true, "mipsle": true,
        "mips64": true, "s390x": true, "riscv64": true,
    }
    return knownArchs[s]
}
```

4. **Added pre-flight check in installer** (`toolchain/installer/installer.go`):

```go
func (i *Installer) installFromTool(tool *registry.Tool, version string) (string, error) {
    tool.Version = version
    ApplyPlatformOverrides(tool)

    // Pre-flight platform check: verify the tool supports the current platform.
    if platformErr := CheckPlatformSupport(tool); platformErr != nil {
        return "", buildPlatformNotSupportedError(platformErr)
    }
    // ... rest of function (download only happens if platform check passes)
}
```

### New Error Output

When a tool doesn't support the current platform, users now see:

```console
✗ Install failed : tool does not support this platform

# Error

**Error:** tool does not support this platform

## Explanation

Tool `replicatedhq/kots` does not support your platform (windows/amd64)

## Hints

💡 This tool only supports: darwin, linux

💡 Consider using WSL (Windows Subsystem for Linux) to run this tool

💡 Install WSL: https://docs.microsoft.com/en-us/windows/wsl/install
```

### Files Changed

| File                                   | Change                                            |
|----------------------------------------|---------------------------------------------------|
| `toolchain/registry/registry.go`       | Added `SupportedEnvs` field to `Tool` struct      |
| `toolchain/registry/aqua/aqua.go`      | Added `SupportedEnvs` to `registryPackage` struct |
| `toolchain/registry/aqua/aqua.go`      | Propagated `SupportedEnvs` when creating tools    |
| `toolchain/installer/platform.go`      | NEW: Platform check functions                     |
| `toolchain/installer/installer.go`     | Added pre-flight platform check                   |
| `toolchain/installer/download.go`      | Added `buildPlatformNotSupportedError()` function |
| `errors/errors.go`                     | Added `ErrToolPlatformNotSupported` error         |
| `toolchain/installer/platform_test.go` | NEW: Unit tests for platform check                |

---

## Issue #5: Platform-Specific Hints (WSL/Rosetta)

### Reported Problem

When a tool installation failed due to platform incompatibility, users received generic error messages without guidance
on potential workarounds for their specific platform.

### Solution

Added platform-specific hints that suggest workarounds based on the user's current platform and the tool's supported
platforms. The implementation uses helper functions for maintainability and reduced cyclomatic complexity:

```go
// buildPlatformHints generates helpful hints based on the current platform.
func buildPlatformHints(currentOS, currentArch string, supportedEnvs []string) []string {
    hints := []string{
        fmt.Sprintf("This tool only supports: %s", strings.Join(supportedEnvs, ", ")),
    }

    // Add platform-specific suggestions using helper functions.
    hints = appendWindowsHints(hints, currentOS, supportedEnvs)
    hints = appendDarwinArm64Hints(hints, currentOS, currentArch, supportedEnvs)
    hints = appendLinuxArm64Hints(hints, currentOS, currentArch, supportedEnvs)

    return hints
}

// appendWindowsHints adds Windows-specific hints (WSL suggestion).
func appendWindowsHints(hints []string, currentOS string, supportedEnvs []string) []string {
    if currentOS != "windows" {
        return hints
    }
    if containsEnv(supportedEnvs, "linux") {
        hints = append(hints,
            "Consider using WSL (Windows Subsystem for Linux) to run this tool",
            "Install WSL: https://docs.microsoft.com/en-us/windows/wsl/install",
        )
    }
    return hints
}

// appendDarwinArm64Hints adds macOS arm64-specific hints (Rosetta/Docker suggestions).
func appendDarwinArm64Hints(hints []string, currentOS, currentArch string, supportedEnvs []string) []string {
    if currentOS != "darwin" || currentArch != "arm64" {
        return hints
    }
    // Check if darwin/amd64 is supported but not darwin/arm64.
    darwinSupported := containsEnv(supportedEnvs, "darwin/amd64") || containsEnv(supportedEnvs, "darwin")
    arm64Supported := containsEnv(supportedEnvs, "darwin/arm64")
    if darwinSupported && !arm64Supported {
        hints = append(hints,
            "Try installing the amd64 version and running under Rosetta 2",
            "Run: softwareupdate --install-rosetta",
        )
    }
    // Check if only Linux is supported.
    if !containsEnv(supportedEnvs, "darwin") && containsEnv(supportedEnvs, "linux") {
        hints = append(hints,
            "Consider using Docker to run this Linux-only tool on macOS",
        )
    }
    return hints
}

// appendLinuxArm64Hints adds Linux arm64-specific hints (QEMU suggestion).
func appendLinuxArm64Hints(hints []string, currentOS, currentArch string, supportedEnvs []string) []string {
    if currentOS != "linux" || currentArch != "arm64" {
        return hints
    }
    if containsEnv(supportedEnvs, "linux/amd64") {
        hints = append(hints,
            "This tool may only support amd64 architecture",
            "Consider using an emulation layer like qemu-user",
        )
    }
    return hints
}
```

### Platform-Specific Hints

| Platform         | Condition                    | Hint                                                  |
|------------------|------------------------------|-------------------------------------------------------|
| Windows          | Linux supported              | WSL installation guide                                |
| macOS arm64      | darwin/amd64 supported       | Rosetta 2 installation instructions                   |
| macOS            | Only Linux supported         | Docker suggestion                                     |
| Linux arm64      | linux/amd64 supported        | QEMU emulation suggestion                             |

### Additional 404 Error Hints

Platform-specific hints are also added to HTTP 404 errors when both download URLs fail:

```go
func addPlatformSpecificHints(builder *errUtils.ErrorBuilder) {
    currentOS := getOS()
    currentArch := getArch()

    switch {
    case currentOS == "windows":
        builder.WithHint("Consider using WSL (Windows Subsystem for Linux) if this tool only supports Linux")

    case currentOS == "darwin" && currentArch == "arm64":
        builder.WithHint("Try running under Rosetta 2 if only amd64 binaries are available")
    }
}
```

### Files Changed

| File                                   | Change                                            |
|----------------------------------------|---------------------------------------------------|
| `toolchain/installer/platform.go`      | Added `buildPlatformHints()` function             |
| `toolchain/installer/platform.go`      | Added `isKnownArch()` for arch-only matching      |
| `toolchain/installer/platform.go`      | Added helper functions for platform hint building |
| `toolchain/installer/download.go`      | Added `addPlatformSpecificHints()` function       |
| `toolchain/installer/platform_test.go` | Added tests for platform-specific hints           |
| `toolchain/installer/platform_test.go` | Added tests for arch-only matching                |

### Unit Tests

```go
func TestBuildPlatformHints_Windows(t *testing.T) {
    hints := buildPlatformHints("windows", "amd64", []string{"darwin", "linux"})
    assert.Contains(t, hints[0], "darwin, linux")
    foundWSL := false
    for _, hint := range hints {
        if contains(hint, "WSL") {
            foundWSL = true
        }
    }
    assert.True(t, foundWSL, "Should suggest WSL for Windows users when Linux is supported")
}

func TestBuildPlatformHints_DarwinArm64(t *testing.T) {
    hints := buildPlatformHints("darwin", "arm64", []string{"darwin/amd64", "linux"})
    foundRosetta := false
    for _, hint := range hints {
        if contains(hint, "Rosetta") {
            foundRosetta = true
        }
    }
    assert.True(t, foundRosetta, "Should suggest Rosetta for darwin/arm64 when darwin/amd64 is supported")
}

func TestBuildPlatformHints_LinuxOnlyOnDarwin(t *testing.T) {
    hints := buildPlatformHints("darwin", "arm64", []string{"linux"})
    foundDocker := false
    for _, hint := range hints {
        if contains(hint, "Docker") {
            foundDocker = true
        }
    }
    assert.True(t, foundDocker, "Should suggest Docker for macOS when only Linux is supported")
}
```

---

## Testing Results

### Tool Installation Results

| Tool                      | Status        | Binary Name               | Notes                                     |
|---------------------------|---------------|---------------------------|-------------------------------------------|
| jqlang/jq@1.7.1           | ✅ Installed   | `jq` / `jq.exe`           | GitHub release type                       |
| kubernetes/kubectl@1.31.4 | ✅ Installed   | `kubectl` / `kubectl.exe` | Binary naming fix confirmed               |
| opentofu/opentofu@1.9.0   | ✅ Installed   | `tofu` / `tofu.exe`       | GitHub release type                       |
| charmbracelet/gum@0.17.0  | ✅ Installed   | `gum` / `gum.exe`         | GitHub release type                       |
| derailed/k9s@0.32.7       | ✅ Installed   | `k9s` / `k9s.exe`         | GitHub release type                       |
| helm/helm@3.16.3          | ✅ Installed   | `helm` / `helm.exe`       | GitHub release type                       |
| replicatedhq/kots@1.127.0 | ✅/❌ Platform  | `kubectl-kots`            | ✅ Linux/macOS, ❌ Windows (platform error) |

### Unit Tests

```bash
go test ./toolchain/registry/aqua/... -v -run TestExtractBinaryNameFromPackageName
```

```console
=== RUN   TestExtractBinaryNameFromPackageName
=== RUN   TestExtractBinaryNameFromPackageName/three_segment_package_name
=== RUN   TestExtractBinaryNameFromPackageName/two_segment_package_name_returns_empty
=== RUN   TestExtractBinaryNameFromPackageName/four_segment_package_name
=== RUN   TestExtractBinaryNameFromPackageName/empty_package_name
=== RUN   TestExtractBinaryNameFromPackageName/single_segment_returns_empty
--- PASS: TestExtractBinaryNameFromPackageName (0.00s)
PASS
```

---

## Test Fixture

A dedicated test fixture was created to validate these fixes:

**Location:** `tests/fixtures/scenarios/toolchain-aqua-tools/`

### Fixture Contents

| File             | Description                                            |
|------------------|--------------------------------------------------------|
| `atmos.yaml`     | Toolchain config with Aqua registry and aliases        |
| `.tool-versions` | Tools list including kubectl (tests binary naming fix) |
| `stacks/deploy/` | Minimal stack configuration                            |

### Tools Tested

| Tool                   | Version | Key Test                                       |
|------------------------|---------|------------------------------------------------|
| charmbracelet/gum      | 0.17.0  | GitHub release type (cross-platform)           |
| derailed/k9s           | 0.32.7  | GitHub release type (cross-platform)           |
| helm/helm              | 3.16.3  | GitHub release type (cross-platform)           |
| jqlang/jq              | 1.7.1   | GitHub release type (cross-platform)           |
| **kubernetes/kubectl** | 1.31.4  | **Binary naming fix (3-segment package)**      |
| opentofu/opentofu      | 1.9.0   | GitHub release type (cross-platform)           |
| **replicatedhq/kots**  | 1.127.0 | **Platform detection (darwin/linux only)**     |

**Platform-specific tool:** `replicatedhq/kots` - Only supports darwin and linux. Used to test platform
detection and WSL hints on Windows.

**Non-existent tool:** `replicatedhq/replicated` - Does NOT exist in the Aqua registry. Used to test
"tool not in registry" error handling.

### Why `replicatedhq/replicated` for "Not in Registry" Testing

| Aspect             | Status                                                  |
|--------------------|---------------------------------------------------------|
| GitHub repo exists | ✅ Yes - <https://github.com/replicatedhq/replicated>    |
| Has releases       | ✅ Yes - v0.124.1, v0.124.0, etc.                        |
| Has binary assets  | ✅ Yes - darwin_all, linux_386, linux_amd64, linux_arm64 |
| In Aqua registry   | ❌ **No** - only `kots` and `outdated` exist             |
| Windows binaries   | ❌ No Windows builds                                     |

`replicatedhq/replicated` is a **real tool with GitHub releases**, but it's **not in the Aqua registry**.
The Aqua registry only contains these tools under the `replicatedhq` organization:

- `replicatedhq/kots`
- `replicatedhq/outdated`

Our "tool not in registry" error is **correct** - the tool exists on GitHub but hasn't been added to the
Aqua registry. If a user wants to use this tool, they would need to:

1. Request it be added to the [Aqua registry](https://github.com/aquaproj/aqua-registry), or
2. Use a different registry/method to install it

### Error Type Distinction

The tests verify two distinct error scenarios:

| Error Type             | Example Tool              | Error Message                    | When It Occurs                        |
|------------------------|---------------------------|----------------------------------|---------------------------------------|
| Tool not in registry   | `replicatedhq/replicated` | "tool not in registry"           | Tool doesn't exist in any registry    |
| Platform not supported | `replicatedhq/kots`       | "does not support your platform" | Tool exists but not for this platform |

---

## Integration Tests

**Test File:** `tests/toolchain_aqua_tools_test.go`

### Test Cases

| Test                                              | Platform    | Description                                                  |
|---------------------------------------------------|-------------|--------------------------------------------------------------|
| `TestToolchainAquaTools_KubectlBinaryNaming`      | All         | Verifies kubectl installed as `kubectl` NOT `kubernetes`     |
| `TestToolchainAquaTools_KubectlExecutable`        | All         | Verifies kubectl binary executes and reports correct version |
| `TestToolchainAquaTools_InstallAllTools`          | All         | Installs cross-platform tools; kots on Linux/macOS only      |
| `TestToolchainAquaTools_ToolsList`                | All         | Verifies `atmos toolchain list` shows correct binary names   |
| `TestToolchainAquaTools_WindowsKubectl`           | Windows     | Verifies `kubectl.exe` NOT `kubernetes.exe`                  |
| `TestToolchainAquaTools_KotsInstall`              | Linux/macOS | Verifies kots installs successfully on supported platforms   |
| `TestToolchainAquaTools_WindowsKotsPlatformError` | Windows     | Verifies platform error with WSL hint for kots on Windows    |
| `TestToolchainAquaTools_NonExistentToolError`     | All         | Verifies "not in registry" error for non-existent tools      |

### Running Tests

```bash
# Run all Aqua tools tests
go test ./tests/... -v -run "TestToolchainAquaTools" -timeout 10m

# Run only kubectl binary naming test
go test ./tests/... -v -run "TestToolchainAquaTools_KubectlBinaryNaming" -timeout 5m

# Run kots installation test (Linux/macOS only)
go test ./tests/... -v -run "TestToolchainAquaTools_KotsInstall" -timeout 5m

# Run Windows platform error test (Windows only)
go test ./tests/... -v -run "TestToolchainAquaTools_WindowsKotsPlatformError" -timeout 2m

# Run non-existent tool error test (all platforms)
go test ./tests/... -v -run "TestToolchainAquaTools_NonExistentToolError" -timeout 2m

# Run unit tests for binary name extraction
go test ./toolchain/registry/aqua/... -v -run "TestExtractBinaryNameFromPackageName"

# Run unit tests for platform detection
go test ./toolchain/installer/... -v -run "TestIsPlatformMatch|TestCheckPlatformSupport|TestBuildPlatformHints"
```

### Test Output Example

```console
=== RUN   TestToolchainAquaTools_KubectlBinaryNaming
    toolchain_aqua_tools_test.go:46: Install output:
        ✓ Installed kubernetes/kubectl@1.31.4 to .tools/bin/kubernetes/kubectl/1.31.4/kubectl (53mb)
    toolchain_aqua_tools_test.go:56: ✓ Binary correctly named 'kubectl' at: .tools/bin/kubernetes/kubectl/1.31.4/kubectl
--- PASS: TestToolchainAquaTools_KubectlBinaryNaming (12.34s)
```

---

## Testing Platform Detection on Windows

This section describes how to manually test the platform detection feature on Windows to verify that users see
helpful error messages when trying to install tools that don't support Windows.

### Prerequisites

- Windows 10/11 with PowerShell or Command Prompt
- Go 1.24+ installed
- Atmos built from source or installed

### Build Atmos on Windows

```powershell
# Clone the repository
git clone https://github.com/cloudposse/atmos.git
cd atmos

# Build atmos
go build -o atmos.exe .
```

### Test 1: Install a Cross-Platform Tool (Should Succeed)

```powershell
# Navigate to the test fixture
cd tests\fixtures\scenarios\toolchain-aqua-tools

# Install a cross-platform tool (should succeed)
..\..\..\..\atmos.exe toolchain install kubernetes/kubectl@1.31.4
```

**Expected output:**

```console
✓ Installed kubernetes/kubectl@1.31.4 to .tools\bin\kubernetes\kubectl\1.31.4\kubectl.exe (53mb)
```

### Test 2: Install a Platform-Restricted Tool (Should Show Platform Error)

```powershell
# Try to install kots (only supports darwin and linux)
..\..\..\..\atmos.exe toolchain install replicatedhq/kots@1.127.0
```

**Expected output:**

```console
✗ Install failed : tool does not support this platform

# Error

**Error:** tool does not support this platform

## Explanation

Tool `replicatedhq/kots` does not support your platform (windows/amd64)

## Hints

💡 This tool only supports: darwin, linux

💡 Consider using WSL (Windows Subsystem for Linux) to run this tool

💡 Install WSL: https://docs.microsoft.com/en-us/windows/wsl/install
```

### Test 3: Verify All Cross-Platform Tools Install

```powershell
# Install all cross-platform tools
..\..\..\..\atmos.exe toolchain install charmbracelet/gum@0.17.0
..\..\..\..\atmos.exe toolchain install derailed/k9s@0.32.7
..\..\..\..\atmos.exe toolchain install helm/helm@3.16.3
..\..\..\..\atmos.exe toolchain install jqlang/jq@1.7.1
..\..\..\..\atmos.exe toolchain install opentofu/opentofu@1.9.0

# List installed tools
..\..\..\..\atmos.exe toolchain list
```

### Test 4: Run Automated Tests on Windows

```powershell
# Run the Windows-specific platform error test
go test ./tests/... -v -run "TestToolchainAquaTools_WindowsKotsPlatformError" -timeout 2m

# Run all cross-platform tool installation tests
go test ./tests/... -v -run "TestToolchainAquaTools_InstallAllTools" -timeout 15m
```

### Key Assertions for Windows Platform Error

When testing the platform error on Windows, verify:

1. **Error type**: The error should be "tool does not support this platform" (not HTTP 404)
2. **Platform identification**: Error should show "windows/amd64" (or appropriate arch)
3. **Supported platforms listed**: Error should show "darwin, linux"
4. **WSL hint present**: Error should suggest using WSL since Linux is supported
5. **No download attempted**: The error should occur immediately without network requests

### Cleanup

```powershell
# Remove installed tools
Remove-Item -Recurse -Force .tools
```

---

## Testing Non-Existent Tool Error

This section describes how to test the error message when attempting to install a tool that doesn't exist
in any configured registry.

### Test: Install a Non-Existent Tool

```bash
# Navigate to the test fixture
cd tests/fixtures/scenarios/toolchain-aqua-tools

# Try to install replicatedhq/replicated (doesn't exist in Aqua registry)
atmos toolchain install replicatedhq/replicated@0.124.1
```

**Expected output:**

```console
✗ Install failed : tool not in registry

# Error

**Error:** tool not in registry

## Explanation

Tool replicatedhq/replicated@0.124.1 was not found in any configured registry.
Atmos searches registries in priority order: Atmos registry → Aqua registry →
custom registries.

## Hints

💡 Run atmos toolchain registry search to browse available tools

💡 Verify network connectivity to registries

💡 Check registry configuration in atmos.yaml
```

### Key Assertions for Non-Existent Tool Error

When testing the non-existent tool error, verify:

1. **Error type**: The error should be "tool not in registry" (not "platform not supported")
2. **Tool name shown**: Error should mention `replicatedhq/replicated`
3. **Registry search hint**: Error should suggest running `atmos toolchain registry search`
4. **Configuration hint**: Error should suggest checking `atmos.yaml` configuration

### Run Automated Test

```bash
# Run the non-existent tool error test (works on all platforms)
go test ./tests/... -v -run "TestToolchainAquaTools_NonExistentToolError" -timeout 2m
```

---

## Summary of All Fixes

| Fix                           | File                                 | Description                                              |
|-------------------------------|--------------------------------------|----------------------------------------------------------|
| Glamour warning suppression   | `pkg/ui/markdown/custom_renderer.go` | Redirect stdout during rendering to suppress warnings    |
| Improved HTTP 404 errors      | `toolchain/installer/download.go`    | User-friendly error messages with platform hints         |
| Binary naming for 3+ segments | `toolchain/registry/aqua/aqua.go`    | Extract binary name from package name (e.g., `kubectl`)  |
| Pre-flight platform check     | `toolchain/installer/platform.go`    | Check platform compatibility before download attempt     |
| Platform-specific hints       | `toolchain/installer/platform.go`    | WSL for Windows, Rosetta for macOS arm64, Docker hints   |
| Arch-only platform matching   | `toolchain/installer/platform.go`    | Support `amd64`/`arm64` entries in Aqua `supported_envs` |

---

## Related Documentation

- [Windows Toolchain Fixes](./windows-atmos-d-and-toolchain-issues.md) - Initial Windows fixes for `.exe` handling
- [Issue #2002](https://github.com/cloudposse/atmos/issues/2002) - Atmos on Windows: Toolchain failures
- [PR #2012](https://github.com/cloudposse/atmos/pull/2012) - Windows toolchain installation fixes
