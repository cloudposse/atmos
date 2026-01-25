# Windows Testing Handoff

**Branch:** `aknysh/windows-test-2`
**Date:** 2026-01-24
**Status:** ✅ **TESTED ON WINDOWS - FIX CONFIRMED WORKING**

## Context

We're testing fixes for Windows toolchain issues reported in [Issue #2002](https://github.com/cloudposse/atmos/issues/2002).

## What Was Done

### 1. Analyzed Issues
- kubectl download failing (HTTP 404) - missing `.exe` in URL
- replicated download failing - no Windows binaries published
- gum not found in PATH - missing `.exe` extension on installed binary
- bootstrap command not found - `.atmos.d` not auto-imported

### 2. Found Gap in PR #2012
PR #2012 fixed `.exe` handling for `github_release` type tools but **NOT** for `http` type tools (like kubectl from dl.k8s.io).

### 3. Applied Fix
Added `.exe` handling to `buildHTTPAssetURL()` in `toolchain/installer/asset.go`:

```go
// On Windows, add .exe to raw binary URLs that don't have any extension.
if !hasArchiveExtension(url) && filepath.Ext(url) == "" {
    url = EnsureWindowsExeExtension(url)
}
```

### 4. Added Tests
- `TestBuildAssetURL_HTTPTypeWindowsExeExtension`
- `TestBuildAssetURL_HTTPTypeNoExeForArchives`

### 5. Created Test Fixture
- `.tool-versions` with 7 tools including kubectl and replicated
- `.atmos.d/commands.yaml` with custom commands
- `atmos.yaml` with `settings.experimental: true`

## Files Modified

1. `toolchain/installer/asset.go` - Added HTTP type `.exe` handling
2. `toolchain/installer/asset_test.go` - Added tests
3. `tests/fixtures/scenarios/windows-test/` - Test fixture files

## macOS Test Results

All 7 tools installed successfully on macOS.

## Windows Testing Instructions

```powershell
# 1. Pull the branch
git checkout aknysh/windows-test-2
git pull

# 2. Build atmos
go build -o atmos.exe .

# 3. Navigate to test fixture
cd tests\fixtures\scenarios\windows-test

# 4. Clean previous installs
Remove-Item -Recurse -Force .tools -ErrorAction SilentlyContinue

# 5. Install toolchain
.\..\..\..\..\atmos.exe toolchain install

# 6. Check installed files (should have .exe extension)
Get-ChildItem -Recurse .tools\bin\ -Filter *.exe

# 7. Test custom commands from .atmos.d
.\..\..\..\..\atmos.exe hello
.\..\..\..\..\atmos.exe bootstrap
```

## Expected Results on Windows

| Tool | Expected Result |
|------|-----------------|
| jqlang/jq | ✅ Should install with `.exe` |
| kubernetes/kubectl | ✅ Should install with `.exe` (HTTP type fix) |
| opentofu/opentofu | ✅ Should install with `.exe` |
| charmbracelet/gum | ✅ Should install with `.exe` |
| derailed/k9s | ✅ Should install with `.exe` |
| helm/helm | ✅ Should install with `.exe` |
| replicatedhq/replicated | ❌ Will fail - no Windows binaries published |

---

## Windows Test Results (2026-01-24)

### Actual Results

| Tool | Status | Binary Name | Notes |
|------|--------|-------------|-------|
| jqlang/jq@1.7.1 | ✅ Installed | `jq.exe` | GitHub release type |
| kubernetes/kubectl@1.31.4 | ✅ Installed | `kubernetes.exe` | **HTTP type `.exe` fix confirmed working** |
| opentofu/opentofu@1.9.0 | ✅ Installed | `tofu.exe` | GitHub release type |
| charmbracelet/gum@0.17.0 | ✅ Installed | `gum.exe` | GitHub release type |
| derailed/k9s@0.32.7 | ✅ Installed | `k9s.exe` | GitHub release type |
| helm/helm@3.16.3 | ✅ Installed | `helm.exe` | GitHub release type |
| replicatedhq/replicated@0.124.1 | ❌ Failed | N/A | 404 - No Windows binaries published (expected) |

**Result:** 6 of 7 tools installed successfully.

### Answers to Test Questions

1. **Did kubectl install successfully?** ✅ YES - Downloaded from `https://dl.k8s.io/v1.31.4/bin/windows/amd64/kubectl.exe`
2. **Did all other tools get `.exe` extension?** ✅ YES - All 6 installed tools have `.exe` extension
3. **Did `.atmos.d` custom commands work?**
   - `hello`: ✅ YES - Output: "Hello from .atmos.d custom commands!"
   - `bootstrap`: ❌ NO - Blocked by `replicated` tool failure (toolchain tries to install all tools first)
4. **What was the exact error for replicated?**
   ```
   ✗ Install failed : HTTP request failed: tried
   https://github.com/replicatedhq/replicated/releases/download/0.124.1/replicated_0.124.1_windows_amd64.tar.gz
   and https://github.com/replicatedhq/replicated/releases/download/v0.124.1/replicated_0.124.1_windows_amd64.tar.gz:
   HTTP 404 Not Found
   ```

### Installed Binary Structure

```
.tools\bin\
├── charmbracelet\gum\0.17.0\gum.exe (14 MB)
├── derailed\k9s\0.32.7\k9s.exe (102 MB)
├── helm\helm\3.16.3\helm.exe (59 MB)
├── jqlang\jq\1.7.1\jq.exe (985 KB)
├── kubernetes\kubectl\1.31.4\kubernetes.exe (58 MB)
└── opentofu\opentofu\1.9.0\tofu.exe (87 MB)
```

### Unit Tests

All HTTP-related tests pass:

```
=== RUN   TestBuildAssetURL_HTTPType
--- PASS: TestBuildAssetURL_HTTPType (0.00s)
=== RUN   TestBuildAssetURL_HTTPTypePreservesVersionAsIs
--- PASS: TestBuildAssetURL_HTTPTypePreservesVersionAsIs (0.00s)
=== RUN   TestBuildAssetURL_HTTPTypeWindowsExeExtension
--- PASS: TestBuildAssetURL_HTTPTypeWindowsExeExtension (0.00s)
=== RUN   TestBuildAssetURL_HTTPTypeNoExeForArchives
--- PASS: TestBuildAssetURL_HTTPTypeNoExeForArchives (0.00s)
```

### Conclusion

**The HTTP type `.exe` fix is confirmed working on Windows.** The gap in PR #2012 has been addressed. kubectl (which uses `http` type with `dl.k8s.io`) now downloads correctly with the `.exe` extension.

---

## Documentation

See `issues/issues-fixes.md` for full analysis and testing guide.
