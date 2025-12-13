# Issue #1858: Path Resolution Regression Fix

## Summary

Fixed a regression introduced in v1.201.0 where relative paths in `atmos.yaml` (like `stacks` or `components/terraform`)
were incorrectly resolved relative to the `ATMOS_CLI_CONFIG_PATH` directory instead of the current working directory.

## Issue Description

**GitHub Issue**: [#1858](https://github.com/cloudposse/atmos/issues/1858)

**Symptoms**:

- After upgrading from `v1.200.0` to `v1.201.0`, `atmos validate stacks` fails with:
  ```text
  The atmos.yaml CLI config file specifies the directory for Atmos stacks as stacks, but the directory does not exist.
  ```

- This occurs when `ATMOS_CLI_CONFIG_PATH` points to a subdirectory (e.g., `./rootfs/usr/local/etc/atmos`) while the
  `stacks/` and `components/` directories are at the repository root.

**Example Configuration**:

```yaml
# Located at ./rootfs/usr/local/etc/atmos/atmos.yaml
base_path: ""
stacks:
  base_path: "stacks"
components:
  terraform:
    base_path: "components/terraform"
```

**Before v1.201.0**: Relative paths resolved relative to CWD (working correctly)
**After v1.201.0**: Relative paths resolved relative to atmos.yaml location (breaking change)

## Root Cause

Commit `2aad133f6` ("fix: resolve ATMOS_BASE_PATH relative to CLI config directory") introduced a new
`resolveAbsolutePath()` function that resolves **all** relative paths relative to `CliConfigPath` (the directory
containing `atmos.yaml`).

This was intended to fix a legitimate issue where `ATMOS_BASE_PATH="../../.."` wasn't working correctly when running
from component subdirectories. However, the change was too broad and affected all relative paths, not just those
explicitly referencing the config directory.

## Solution

Modified `resolveAbsolutePath()` to only resolve paths relative to `cliConfigPath` when they explicitly reference the
config directory location:

1. **Exactly `.` or `..`** (current or parent directory) → Resolve relative to atmos.yaml location
2. **Paths starting with `./` or `../`** (Unix-style explicit relative) → Resolve relative to atmos.yaml location
3. **Paths starting with `.\` or `..\`** (Windows-style explicit relative) → Resolve relative to atmos.yaml location
4. **All other relative paths** (like `stacks`, `components/terraform`, `.hidden`, `..foo`, empty string) → Resolve
   relative to CWD for backward compatibility

Note: The detection is strict to avoid misclassifying paths like `.hidden` or `..foo` as explicit relative paths.

### Why `.` Resolves to Config Directory

The single dot `.` is treated as an explicit relative path (resolving to config dir) rather than a simple path
(resolving to CWD) because:

- When a user sets `ATMOS_BASE_PATH="."` along with `ATMOS_CLI_CONFIG_PATH` pointing to a different directory, they're
  explicitly saying "use the directory where atmos.yaml is located as the base path"
- This is essential for **path-based component resolution**, where users run commands like `atmos terraform plan .` from
  within a component directory while `ATMOS_CLI_CONFIG_PATH` points back to the repo root
- An empty string `""` is the correct way to indicate "use CWD as the base path" when backward compatibility is needed

### Code Change

```go
// Before (v1.201.0):
func resolveAbsolutePath(path string, cliConfigPath string) (string, error) {
    if filepath.IsAbs(path) {
        return path, nil
    }
    // PROBLEM: ALL relative paths resolved relative to cliConfigPath
    if cliConfigPath != "" {
        basePath := filepath.Join(cliConfigPath, path)
        absPath, err := filepath.Abs(basePath)
        // ...
    }
}

// After (fix):
func resolveAbsolutePath(path string, cliConfigPath string) (string, error) {
    if filepath.IsAbs(path) {
        return path, nil
    }
    // Only resolve relative to config dir for explicit relative paths.
    // Strict detection avoids misclassifying ".hidden" or "..foo".
    sep := string(filepath.Separator)
    isExplicitRelativePath := path == "." || path == ".." ||
        strings.HasPrefix(path, "."+sep) || strings.HasPrefix(path, ".."+sep) ||
        strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../")
    if isExplicitRelativePath && cliConfigPath != "" {
        basePath := filepath.Join(cliConfigPath, path)
        // ...
    }
    // Otherwise, resolve relative to CWD for backward compatibility
}
```

## Test Coverage

Added comprehensive tests in `pkg/config/config_test.go`:

1. **`TestResolveAbsolutePath`**: Unit tests for the path resolution logic covering:

- Absolute paths remain unchanged
- Simple relative paths resolve to CWD
- Empty path resolves to CWD
- Dot path (`.`) resolves to config dir (explicit current directory reference)
- Path with `..` resolves to config dir
- Path starting with `../` resolves to config dir
- Path starting with `./` resolves to config dir
- Complex relative paths without `./` prefix resolve to CWD

2. **`TestCliConfigPathRegression`**: Integration tests for issue #1858 scenario:

- Config in subdirectory with `base_path: ..`
- Empty `base_path` with nested config should resolve paths relative to CWD

## Test Fixtures

Added test fixture in `tests/fixtures/scenarios/`:

- **`cli-config-path/`**: Tests the `base_path: ..` scenario where atmos.yaml is in a subdirectory and uses parent directory traversal to reference the repo root

## Backward Compatibility

This fix maintains backward compatibility with v1.200.0 behavior:

- Simple relative paths like `stacks` and `components/terraform` resolve relative to CWD
- Empty `base_path` (`""`) resolves relative to CWD
- Paths that explicitly use `..`, `./`, or `.` to reference the config directory location resolve relative to atmos.yaml
  as intended by PR #1774

## Manual Testing

You can manually verify the fix using the test fixture at `tests/fixtures/scenarios/cli-config-path/`.

### Fixture Structure

```text
tests/fixtures/scenarios/cli-config-path/
├── components/
│   └── terraform/
│       └── test-component/
│           └── main.tf
├── config/
│   └── atmos.yaml          # Config in subdirectory with base_path: ".."
└── stacks/
    └── dev.yaml
```

The `config/atmos.yaml` uses `base_path: ".."` to reference the parent directory (repo root), while `stacks` and
`components/terraform` are simple relative paths that should resolve relative to CWD.

### Test Commands

Run these commands from the fixture directory:

```bash
# Navigate to the test fixture
cd tests/fixtures/scenarios/cli-config-path

# Set ATMOS_CLI_CONFIG_PATH to point to the config subdirectory
export ATMOS_CLI_CONFIG_PATH=./config

# Test 1: Validate stacks (this was failing in v1.201.0)
atmos validate stacks
# Expected: "✓ All stacks validated successfully"

# Test 2: List stacks
atmos list stacks
# Expected: "dev"

# Test 3: List components
atmos list components
# Expected: "test-component  dev  terraform  real  true  false"

# Test 4: Describe component
atmos describe component test-component -s dev
# Expected: Component configuration output (not an error)

# Test 5: Describe config (verify path resolution)
atmos describe config | grep -E "(base_path|stacks_base_absolute|terraform_dir_absolute)"
# Expected: Paths should point to repo root, not inside config/
```

### Expected vs. Broken Behavior

| Command                 | v1.200.0 (Working) | v1.201.0 (Broken)                   | This Fix          |
|-------------------------|--------------------|-------------------------------------|-------------------|
| `atmos validate stacks` | ✅ Success          | ❌ "stacks directory does not exist" | ✅ Success         |
| `atmos list stacks`     | ✅ Shows "dev"      | ❌ Error                             | ✅ Shows "dev"     |
| `atmos list components` | ✅ Shows component  | ❌ Error                             | ✅ Shows component |

## Related PRs/Commits

- **Commit `2aad133f6`**: "fix: resolve ATMOS_BASE_PATH relative to CLI config directory" (introduced the regression)
- **PR #1774**: "Path-based component resolution for all commands" (contained the breaking commit)
