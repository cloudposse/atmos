# Windows Toolchain Issues - Analysis and Testing Guide

This document analyzes the issues reported in `issues.md` and provides instructions for testing on both macOS and
Windows.

## Issue Analysis

### Issue 1: kubectl Download Fails with HTTP 404

**Error:**

```
✗ Install failed : HTTP request failed: tried https://dl.k8s.io/1.31.4/bin/windows/amd64/kubectl
and https://dl.k8s.io/v1.31.4/bin/windows/amd64/kubectl: HTTP 404 Not Found
```

**Root Cause:**
The kubectl download URL for Windows requires the `.exe` extension. The Kubernetes download server serves:

- Linux/macOS: `https://dl.k8s.io/v1.31.4/bin/linux/amd64/kubectl`
- Windows: `https://dl.k8s.io/v1.31.4/bin/windows/amd64/kubectl.exe` (note the `.exe`)

**Gap Found in PR #2012:**
PR #2012 added `.exe` handling for `github_release` type tools but **NOT** for `http` type tools. The kubectl tool
uses `http` type with a URL template pointing to `dl.k8s.io`, so it wasn't getting the `.exe` extension.

**Fix Applied:**
Added the same `.exe` handling logic to `buildHTTPAssetURL()` in `toolchain/installer/asset.go`:

```go
// On Windows, add .exe to raw binary URLs that don't have any extension.
// This follows Aqua's behavior where Windows binaries need .exe extension in the download URL.
if !hasArchiveExtension(url) && filepath.Ext(url) == "" {
    url = EnsureWindowsExeExtension(url)
}
```

**Status:** ✅ Fixed in current branch

### Issue 2: replicated Download Fails with HTTP 404

**Error:**

```
✗ Install failed : HTTP request failed: tried
https://github.com/replicatedhq/replicated/releases/download/0.124.1/replicated_0.124.1_windows_amd64.tar.gz
```

**Root Cause:**
The `replicatedhq/replicated` tool does NOT publish Windows binaries. Checking their GitHub releases shows only:

- `replicated_X.X.X_darwin_all.tar.gz`
- `replicated_X.X.X_linux_amd64.tar.gz`
- `replicated_X.X.X_linux_arm64.tar.gz`

**Resolution:**
This is not an Atmos bug - the tool simply doesn't support Windows. The user should:

1. Remove `replicatedhq/replicated` from `.tool-versions` on Windows
2. Or add a platform condition to exclude it on Windows

### Issue 3: "gum" Executable Not Found in $PATH

**Error:**

```
"gum": executable file not found in $PATH
```

**Root Cause (Multiple Possibilities):**

1. **Missing `.exe` extension** - gum binary installed as `gum` instead of `gum.exe`
2. **PATH not updated** - The `.tools` directory is not in the system PATH
3. **Custom command execution order** - Commands using gum run before toolchain installation completes

**Expected Fix:**

- PR #2012 added `EnsureWindowsExeExtension()` to ensure binaries get `.exe` suffix
- Users must add the toolchain path to their PATH or use `atmos toolchain env` to get the correct PATH

### Issue 4: "bootstrap" Command Not Found

**Error:**

```
Error: Unknown command bootstrap for atmos
```

**Root Cause:**
The `bootstrap` command is a custom command defined in `.atmos.d/` directory. This suggests:

1. The `.atmos.d/` directory is not being auto-imported
2. Or the custom commands file doesn't exist

**Expected Fix:**
PR #2012 improved `.atmos.d` detection with better error handling.

---

## Test Fixture Setup

This fixture is missing required files. Create them before testing:

### 1. Create `.tool-versions`

```bash
# Create .tool-versions file
cat > .tool-versions << 'EOF'
charmbracelet/gum 0.17.0
derailed/k9s 0.32.7
helm/helm 3.16.3
jqlang/jq 1.7.1
kubernetes/kubectl 1.31.4
opentofu/opentofu 1.9.0
EOF
```

**Note:** Removed `replicatedhq/replicated` (no Windows support). kubectl is now included since the HTTP type `.exe`
fix has been applied.

### 2. Create `.atmos.d/commands.yaml`

```bash
mkdir -p .atmos.d
cat > .atmos.d/commands.yaml << 'EOF'
commands:
  - name: bootstrap
    description: "Bootstrap the development environment"
    steps:
      - command: echo "Running bootstrap..."
      - command: atmos toolchain install
      - command: echo "Bootstrap complete!"
EOF
```

---

## Testing Instructions

### Prerequisites

Build Atmos from the current branch:

```bash
# From the atmos repo root
go build -o atmos .
```

### macOS Testing

```bash
# Navigate to test fixture
cd tests/fixtures/scenarios/windows-test

# Test 1: Verify configuration is loaded
atmos describe configuration

# Test 2: Install toolchain
atmos toolchain install

# Test 3: Verify tools are installed
ls -la .tools/

# Test 4: Verify tools are executable
.tools/gum --version
.tools/helm version
.tools/jq --version

# Test 5: Test PATH export (bash/zsh)
eval "$(atmos toolchain env)"
which gum
gum --version

# Test 6: Verify .atmos.d commands are loaded (if exists)
atmos --help | grep bootstrap
```

### Windows Testing (PowerShell)

```powershell
# Navigate to test fixture
cd tests\fixtures\scenarios\windows-test

# Test 1: Verify configuration is loaded
.\atmos.exe describe configuration

# Test 2: Install toolchain
.\atmos.exe toolchain install

# Test 3: Verify tools are installed with .exe extension
Get-ChildItem .tools\

# Expected output should show:
# gum.exe
# helm.exe
# jq.exe
# tofu.exe
# k9s.exe

# Test 4: Verify tools are executable
.\.tools\gum.exe --version
.\.tools\helm.exe version
.\.tools\jq.exe --version

# Test 5: Test PATH export (PowerShell)
Invoke-Expression (.\atmos.exe toolchain env)
Get-Command gum
gum --version

# Test 6: Verify .atmos.d commands are loaded (if exists)
.\atmos.exe --help | Select-String "bootstrap"
```

### Windows Testing (Command Prompt)

```cmd
REM Navigate to test fixture
cd tests\fixtures\scenarios\windows-test

REM Test 1: Verify configuration is loaded
atmos.exe describe configuration

REM Test 2: Install toolchain
atmos.exe toolchain install

REM Test 3: Verify tools are installed with .exe extension
dir .tools\

REM Test 4: Verify tools are executable
.tools\gum.exe --version
.tools\helm.exe version
.tools\jq.exe --version
```

---

## Expected Results

### Successful Installation

```
✓ Installed 6 tools
  • charmbracelet/gum 0.17.0
  • derailed/k9s 0.32.7
  • helm/helm 3.16.3
  • jqlang/jq 1.7.1
  • kubernetes/kubectl 1.31.4
  • opentofu/opentofu 1.9.0
```

### Installed Files on Windows

```
.tools/
├── gum.exe
├── helm.exe
├── jq.exe
├── k9s.exe
├── kubectl.exe
└── tofu.exe
```

### Installed Files on macOS/Linux

```
.tools/
├── gum
├── helm
├── jq
├── k9s
├── kubectl
└── tofu
```

---

## Debugging Commands

### Check if tools are in registry

```bash
atmos toolchain list
```

### Check download URLs (verbose mode)

```bash
atmos toolchain install --log-level=debug 2>&1 | grep -i "download\|url"
```

### Check PATH configuration

```bash
# bash/zsh
atmos toolchain env

# PowerShell
atmos.exe toolchain env
```

### Check .atmos.d loading

```bash
atmos describe configuration --log-level=debug 2>&1 | grep -i "atmos.d"
```

---

## Known Limitations

1. **Tools without Windows support** - Some tools (like `replicatedhq/replicated`) don't publish Windows binaries. These
   will fail on Windows regardless of Atmos fixes.

2. **PATH persistence** - `atmos toolchain env` exports PATH temporarily. For persistent PATH, users must add `.tools`
   to their system PATH manually.

---

## Fixes Applied in This Branch

### Fix 1: HTTP Type Tools Now Get `.exe` on Windows

**Problem:** PR #2012 only added `.exe` handling for `github_release` type tools. Tools using `http` type (like kubectl
from `dl.k8s.io`) were missing the `.exe` extension in download URLs.

**Solution:** Added `.exe` handling to `buildHTTPAssetURL()` in `toolchain/installer/asset.go`.

**Before:**

```go
func (i *Installer) buildHTTPAssetURL(tool *registry.Tool, version string) (string, error) {
    data := buildTemplateData(tool, version)
    return executeAssetTemplate(tool.Asset, tool, data)  // No .exe handling!
}
```

**After:**

```go
func (i *Installer) buildHTTPAssetURL(tool *registry.Tool, version string) (string, error) {
    data := buildTemplateData(tool, version)
    url, err := executeAssetTemplate(tool.Asset, tool, data)
    if err != nil {
        return "", err
    }

    // On Windows, add .exe to raw binary URLs that don't have any extension.
    if !hasArchiveExtension(url) && filepath.Ext(url) == "" {
        url = EnsureWindowsExeExtension(url)
    }

    return url, nil
}
```

**Tests Added:**

- `TestBuildAssetURL_HTTPTypeWindowsExeExtension` - Verifies HTTP type gets `.exe` on Windows
- `TestBuildAssetURL_HTTPTypeNoExeForArchives` - Verifies archives don't get `.exe`

**Test Results:**

```
=== RUN   TestBuildAssetURL_HTTPTypeWindowsExeExtension
--- PASS: TestBuildAssetURL_HTTPTypeWindowsExeExtension (0.00s)
=== RUN   TestBuildAssetURL_HTTPTypeNoExeForArchives
--- PASS: TestBuildAssetURL_HTTPTypeNoExeForArchives (0.00s)
```

---

## Related Issues and PRs

- Issue: [#2002 - Atmos on Windows: Toolchain failures and config issues](https://github.com/cloudposse/atmos/issues/2002)
- PR: [#2012 - fix: Windows toolchain installation issues](https://github.com/cloudposse/atmos/pull/2012)
