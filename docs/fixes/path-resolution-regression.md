# Issue #1858: Path Resolution Regression Fix

## Summary

Fixed a regression introduced in v1.201.0 where relative paths in `atmos.yaml` were resolved incorrectly. The fix
implements correct base path resolution semantics that allow users to run `atmos` from anywhere inside a repository.

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

## Root Cause

Commit `2aad133f6` ("fix: resolve ATMOS_BASE_PATH relative to CLI config directory") introduced a new
`resolveAbsolutePath()` function that resolves **all** relative paths relative to `CliConfigPath` (the directory
containing `atmos.yaml`).

This was intended to fix a legitimate issue where `ATMOS_BASE_PATH="../../.."` wasn't working correctly when running
from component subdirectories. However, the change was too broad and affected all relative paths, not just those
explicitly referencing the config directory.

## Solution

Implemented correct base path resolution semantics as defined in the PRD (`docs/prd/base-path-resolution-semantics.md`):

### Base Path Resolution Rules

| `base_path` value    | Resolves to                                            | Rationale                                          |
| -------------------- | ------------------------------------------------------ | -------------------------------------------------- |
| `""` (empty/unset)   | Git repo root, fallback to dirname(atmos.yaml)         | Smart default - most users want repo root          |
| `"."`                | CWD                                                    | Explicit "where I'm standing"                      |
| `"./foo"`            | CWD/foo                                                | Explicit CWD-relative path                         |
| `"foo"` or `"foo/bar"` | Git repo root/foo, fallback to dirname(atmos.yaml)/foo | Simple relative paths anchor to repo root          |
| `"../foo"`           | dirname(atmos.yaml)/../foo                             | Parent traversal navigates from config location    |
| `"/absolute/path"`   | /absolute/path (unchanged)                             | Absolute paths are explicit                        |

### Key Semantic Distinctions

1. **`.` vs `""`**: These are NOT the same
   - `"."` = explicit CWD (user wants current working directory)
   - `""` = smart default (repo root with fallback)

2. **`./foo` vs `foo`**: These are NOT the same
   - `"./foo"` = CWD/foo (explicit CWD-relative)
   - `"foo"` = repo root/foo (anchored to repo root)

3. **`../foo`**: Always relative to atmos.yaml location
   - Used for navigating from config location to elsewhere in repo
   - Common pattern: config in subdirectory, `base_path: "../.."` to reach repo root

### Code Change

```go
func resolveBasePath(path string, cliConfigPath string) (string, error) {
    // Absolute paths unchanged.
    if filepath.IsAbs(path) {
        return path, nil
    }

    // Explicit CWD: "." or "./...".
    if path == "." || strings.HasPrefix(path, "./") {
        return filepath.Abs(path)
    }

    // Parent traversal: "../..." - relative to atmos.yaml dir.
    if strings.HasPrefix(path, "../") {
        return filepath.Abs(filepath.Join(cliConfigPath, path))
    }

    // Empty or simple relative: try git root, fallback to atmos.yaml dir.
    gitRoot, err := getGitRoot()
    if err == nil && gitRoot != "" {
        if path == "" {
            return gitRoot, nil
        }
        return filepath.Join(gitRoot, path), nil
    }

    // Fallback: relative to atmos.yaml dir.
    if path == "" {
        return filepath.Abs(cliConfigPath)
    }
    return filepath.Abs(filepath.Join(cliConfigPath, path))
}
```

## Test Coverage

Added comprehensive tests in `pkg/config/config_test.go`:

1. **`TestResolveBasePath`**: Unit tests for the path resolution logic covering:

- Absolute paths remain unchanged
- Empty path resolves to git repo root (or atmos.yaml dir if not in git repo)
- Dot path (`.`) resolves to CWD
- Paths starting with `./` resolve to CWD
- Paths starting with `../` resolve relative to atmos.yaml dir
- Simple relative paths resolve to git repo root (or atmos.yaml dir if not in git repo)

2. **`TestCliConfigPathRegression`**: Integration tests for issue #1858 scenario:

- Config in subdirectory with `base_path: ..`
- Empty `base_path` with nested config should resolve paths relative to git repo root

## Test Fixtures

Added test fixture in `tests/fixtures/scenarios/`:

- **`cli-config-path/`**: Tests the `base_path: ..` scenario where atmos.yaml is in a subdirectory and uses parent
  directory traversal to reference the repo root

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
`components/terraform` are simple relative paths that should resolve relative to the git repo root.

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

| Command                 | v1.200.0 (Working) | v1.201.0 (Broken)                    | This Fix           |
| ----------------------- | ------------------ | ------------------------------------ | ------------------ |
| `atmos validate stacks` | ✅ Success          | ❌ "stacks directory does not exist" | ✅ Success          |
| `atmos list stacks`     | ✅ Shows "dev"      | ❌ Error                              | ✅ Shows "dev"      |
| `atmos list components` | ✅ Shows component  | ❌ Error                              | ✅ Shows component  |

## Related PRs/Commits

- **Commit `2aad133f6`**: "fix: resolve ATMOS_BASE_PATH relative to CLI config directory" (introduced the regression)
- **PR #1774**: "Path-based component resolution for all commands" (contained the breaking commit)
- **PR #1868**: Initial fix attempt (reverted to inconsistent CWD behavior)
- **PRD**: `docs/prd/base-path-resolution-semantics.md` (defines correct semantics)
