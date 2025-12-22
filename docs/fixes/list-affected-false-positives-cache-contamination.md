# List Affected False Positives in Subdirectories

## Issue Summary

Running `atmos list affected` with `-C <directory>` flag (e.g., from a subdirectory like `examples/demo-stacks`) reports all components as affected with reason `stack.metadata` even when there are zero file differences between the compared branches.

## Symptoms

```bash
❯ atmos -C examples/demo-stacks list affected
✓ Computed affected components
ℹ Comparing  refs/remotes/origin/HEAD... refs/heads/feature-branch

    Component  Stack    Type       Affected        File
────────────────────────────────────────────────────────
 ●  myapp      dev      terraform  stack.metadata
 ●  myapp      prod     terraform  stack.metadata
 ●  myapp      staging  terraform  stack.metadata
```

Even when `git diff refs/remotes/origin/HEAD...refs/heads/feature-branch` shows no differences in the subdirectory.

## Root Cause Analysis

### Issue 1: Incorrect Path Calculation for Remote Worktree (Primary)

The primary bug was in `executeDescribeAffected()` in `internal/exec/describe_affected_utils.go`. When computing paths for the remote worktree, the code used `atmosConfig.BasePath` (e.g., `./`) directly instead of computing the actual relative path from the git root to the atmos configuration directory.

**Original (buggy) code:**
```go
basePath := atmosConfig.BasePath  // e.g., "./"

atmosConfig.StacksBaseAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Stacks.BasePath)
// Results in: /tmp/worktree/./stacks (WRONG)
// Should be:  /tmp/worktree/examples/demo-stacks/stacks
```

**The problem**: When running with `-C examples/demo-stacks`:
1. `atmosConfig.BasePath` is `./` (from the local atmos.yaml)
2. `localRepoFileSystemPath` is the git worktree root (e.g., `/path/to/repo`)
3. But the actual stacks are at `/path/to/repo/examples/demo-stacks/stacks`

The code failed to account for the relative path from the git root to the atmos configuration directory.

**Result**: Remote stacks were being looked up at `/tmp/worktree/stacks` instead of `/tmp/worktree/examples/demo-stacks/stacks`, resulting in **0 remote stacks found**, causing all local stacks to appear as "affected" since they had no remote counterpart to compare against.

### Issue 2: Base Component Cache Contamination (Secondary)

A secondary issue was that the `baseComponentConfigCache` (defined in `internal/exec/stack_processor_cache.go`) could potentially cause cross-contamination between current and remote stack processing since its cache key (`stack:component:baseComponent`) doesn't include path information.

## Fix

### Primary Fix: Compute Relative Paths from Git Root

The fix computes the relative paths from the git repo root to the current absolute paths, then uses those relative paths when constructing remote paths:

**File**: `internal/exec/describe_affected_utils.go`

```go
// Save current paths before modification.
currentStacksBaseAbsolutePath := atmosConfig.StacksBaseAbsolutePath
// ... save other paths

// Compute the relative paths from the git repo root to the current absolute paths.
// This handles the case where atmos is run from a subdirectory (e.g., -C examples/demo-stacks).
stacksRelPath, err := filepath.Rel(localRepoFileSystemPathAbs, currentStacksBaseAbsolutePath)
if err != nil {
    return nil, nil, nil, err
}
// ... similar for terraformRelPath, helmfileRelPath, packerRelPath, stackConfigFilesRelPaths

// Update paths to point to the remote repo dir using the computed relative paths.
atmosConfig.StacksBaseAbsolutePath = filepath.Join(remoteRepoFileSystemPath, stacksRelPath)
// ... similar for other paths
```

### Secondary Fix: Clear Cache Between Processing

Added a call to `ClearBaseComponentConfigCache()` between processing current and remote stacks to prevent any potential cache contamination:

```go
currentStacks, err := ExecuteDescribeStacks(atmosConfig, ...)
if err != nil {
    return nil, nil, nil, err
}

// Clear base component cache between current and remote stack processing
// to prevent cache contamination (cache keys don't include path information).
ClearBaseComponentConfigCache()

// ... compute relative paths and update atmosConfig for remote
remoteStacks, err := ExecuteDescribeStacks(atmosConfig, ...)
```

## Files Modified

1. **`internal/exec/describe_affected_utils.go`**
   - Replaced `basePath`-based path calculation with relative path computation from current absolute paths
   - Added `ClearBaseComponentConfigCache()` call between current and remote stack processing
   - Removed unused `u` import alias

## Testing

```bash
# Should show no affected components when comparing identical stacks
atmos -C examples/demo-stacks list affected

# Expected output:
✓ Computed affected components
ℹ Comparing refs/remotes/origin/HEAD...refs/heads/current-branch
ℹ No affected components found
```

## Related Files

- `internal/exec/describe_affected_utils.go` - Main fix location
- `internal/exec/stack_processor_cache.go` - Cache clear function
- `internal/exec/describe_affected_helpers.go` - Entry points for affected detection
- `internal/exec/describe_affected_components.go` - Metadata comparison logic

## Why This Approach

1. **Correctness**: Using the current absolute paths to compute relative paths ensures the correct subdirectory is preserved
2. **Consistency**: The relative path computation works regardless of whether atmos is run from the repo root or a subdirectory
3. **Robustness**: The cache clear call ensures no stale data affects the comparison
4. **Minimal changes**: The fix is localized to `describe_affected_utils.go` and doesn't require changes to cache key formats or other components
