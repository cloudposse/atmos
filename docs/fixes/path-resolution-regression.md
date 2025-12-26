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

| `base_path` value      | Resolves to                                              | Rationale                                          |
| ---------------------- | -------------------------------------------------------- | -------------------------------------------------- |
| `""` (empty/unset)     | Git repo root, fallback to dirname(atmos.yaml)           | Smart default - most users want repo root          |
| `"."`                  | dirname(atmos.yaml)                                      | Explicit config-dir-relative                       |
| `"./foo"`              | dirname(atmos.yaml)/foo                                  | Explicit config-dir-relative path                  |
| `".."`                 | Parent of dirname(atmos.yaml)                            | Config-dir-relative parent traversal               |
| `"../foo"`             | dirname(atmos.yaml)/../foo                               | Config-dir-relative parent traversal               |
| `"foo"` or `"foo/bar"` | Git repo root/foo, fallback to dirname(atmos.yaml)/foo   | Simple relative paths anchor to repo root          |
| `"/absolute/path"`     | /absolute/path (unchanged)                               | Absolute paths are explicit                        |
| `!repo-root`           | Git repository root                                      | Explicit git root tag                              |
| `!cwd`                 | Current working directory                                | Explicit CWD tag                                   |

### Key Semantic Distinctions

1. **`.` vs `""`**: These are NOT the same
   - `"."` = explicit config directory (dirname of atmos.yaml)
   - `""` = smart default (git repo root with fallback to config dir)

2. **`./foo` vs `foo`**: These are NOT the same
   - `"./foo"` = config-dir/foo (explicit config-dir-relative)
   - `"foo"` = git-root/foo with fallback to config-dir/foo

3. **`../foo`**: Always relative to atmos.yaml location
   - Used for navigating from config location to elsewhere in repo
   - Common pattern: config in subdirectory, `base_path: ".."` to reach parent dir

4. **`!cwd` vs `"."`**: These are NOT the same
   - `!cwd` = current working directory (where you run atmos from)
   - `"."` = config directory (where atmos.yaml is located)

### Code Change

The core logic is in `resolveAbsolutePath()` in `pkg/config/config.go`:

```go
func resolveAbsolutePath(path string, cliConfigPath string) (string, error) {
    // Absolute paths unchanged.
    if filepath.IsAbs(path) {
        return path, nil
    }

    // Check for explicit relative paths: ".", "./...", "..", or "../..."
    // These resolve relative to atmos.yaml location (config-file-relative).
    isExplicitRelative := path == "." ||
        path == ".." ||
        strings.HasPrefix(path, "./") ||
        strings.HasPrefix(path, "../")

    // For explicit relative paths (".", "./...", "..", "../..."):
    // Resolve relative to config directory (cliConfigPath).
    if isExplicitRelative && cliConfigPath != "" {
        basePath := filepath.Join(cliConfigPath, path)
        return filepath.Abs(basePath)
    }

    // For empty path or simple relative paths (like "stacks", "components/terraform"):
    // Try git root first, fallback to config dir, then CWD.
    return tryResolveWithGitRoot(path, isExplicitRelative, cliConfigPath)
}

func tryResolveWithGitRoot(path string, isExplicitRelative bool, cliConfigPath string) (string, error) {
    gitRoot := getGitRootOrEmpty()
    if gitRoot == "" {
        return tryResolveWithConfigPath(path, cliConfigPath)
    }

    // Git root available - resolve relative to it.
    if path == "" {
        return gitRoot, nil
    }
    return filepath.Join(gitRoot, path), nil
}
```

## Test Coverage

Added comprehensive tests:

1. **`pkg/config/config_test.go`**: Unit tests for path resolution logic covering:
   - Absolute paths remain unchanged
   - Empty path resolves to git repo root (or atmos.yaml dir if not in git repo)
   - Dot path (`.`) resolves to config directory (dirname of atmos.yaml)
   - Paths starting with `./` resolve to config directory
   - Paths starting with `../` resolve relative to config directory
   - Simple relative paths resolve to git repo root (with fallback to config dir)

2. **`pkg/config/config_path_comprehensive_edge_cases_test.go`**: Comprehensive edge case tests

3. **`pkg/utils/git_test.go`**: Tests for `!cwd` and `!repo-root` YAML tag processing:
   - `TestProcessTagCwd`: Tests `!cwd` tag with various path arguments
   - `TestProcessTagGitRoot`: Tests `!repo-root` tag with fallback behavior

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

The `config/atmos.yaml` uses:
- `base_path: ".."` - resolves to parent of config dir (the fixture root) because `..` is an explicit
  relative path that resolves relative to the config directory
- `stacks: { base_path: "stacks" }` - simple relative path, resolves to git-root/stacks
- `components: { terraform: { base_path: "components/terraform" } }` - simple relative path, resolves to
  git-root/components/terraform

### Test Commands

Run these commands from the **repository root** (not the fixture directory):

```bash
# Build atmos first
make build

# Navigate to the test fixture
cd tests/fixtures/scenarios/cli-config-path

# Set ATMOS_CLI_CONFIG_PATH to point to the config subdirectory
export ATMOS_CLI_CONFIG_PATH=./config

# Test 1: Validate stacks (this was failing in v1.201.0)
../../../../build/atmos validate stacks
# Expected: "✓ All stacks validated successfully"

# Test 2: List stacks
../../../../build/atmos list stacks
# Expected: "dev"

# Test 3: List components
../../../../build/atmos list components
# Expected: "test-component"

# Test 4: Describe component
../../../../build/atmos describe component test-component -s dev
# Expected: Component configuration output (not an error)

# Test 5: Describe config (verify path resolution)
../../../../build/atmos describe config | grep -E "(basePathAbsolute|stacksBaseAbsolutePath|terraformDirAbsolutePath)"
# Expected: Paths should point to fixture root, not inside config/
# - basePathAbsolute should end with "cli-config-path" (the fixture root)
# - stacksBaseAbsolutePath should end with "cli-config-path/stacks"
# - terraformDirAbsolutePath should end with "cli-config-path/components/terraform"
```

### Understanding the Path Resolution

Given this fixture structure:
```
cli-config-path/           <- This is the "fixture root" and git root for path resolution
├── config/
│   └── atmos.yaml         <- ATMOS_CLI_CONFIG_PATH points here
├── stacks/
│   └── dev.yaml
└── components/terraform/
    └── test-component/
```

With `ATMOS_CLI_CONFIG_PATH=./config`:
1. `base_path: ".."` → resolves to `config/..` = `cli-config-path/` (config-dir-relative)
2. `stacks.base_path: "stacks"` → resolves to `cli-config-path/stacks/` (anchored to base_path)
3. `components.terraform.base_path: "components/terraform"` → resolves to `cli-config-path/components/terraform/`

### Expected vs. Broken Behavior

| Command                 | v1.200.0 (Working) | v1.201.0 (Broken)                    | This Fix           |
| ----------------------- | ------------------ | ------------------------------------ | ------------------ |
| `atmos validate stacks` | ✅ Success          | ❌ "stacks directory does not exist" | ✅ Success          |
| `atmos list stacks`     | ✅ Shows "dev"      | ❌ Error                              | ✅ Shows "dev"      |
| `atmos list components` | ✅ Shows component  | ❌ Error                              | ✅ Shows component  |

## Related PRs/Commits

- **Commit `2aad133f6`**: "fix: resolve ATMOS_BASE_PATH relative to CLI config directory" (introduced the regression)
- **PR #1774**: "Path-based component resolution for all commands" (contained the breaking commit)
- **PR #1868**: This fix - implements correct base path resolution semantics
- **PR #1872**: Enhanced path resolution semantics (merged into #1868)
- **PRD**: `docs/prd/base-path-resolution-semantics.md` (defines correct semantics)
