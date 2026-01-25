# Windows Testing Handoff

**Branch:** `aknysh/windows-test-2`
**Date:** 2026-01-24

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

## What to Report Back

1. Did kubectl install successfully? (This tests the HTTP type `.exe` fix)
2. Did all other tools get `.exe` extension?
3. Did `.atmos.d` custom commands (`hello`, `bootstrap`) work?
4. What was the exact error for replicated?

## Documentation

See `issues/issues-fixes.md` for full analysis and testing guide.
