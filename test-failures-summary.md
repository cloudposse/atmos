# GitHub Actions Test Failures Summary

## Build Information
- **Run ID**: 18052889205
- **Branch**: vendor-updates
- **Date**: 2025-09-27T01:07:51Z
- **Status**: FAILED

## Failed Test Summary

### 1. TestCheckForVendorUpdates/Git_repository_with_commit_hash
**Location**: internal/exec/vendor_update_test.go:88-90
**Error**:
- Expected `HasUpdate` to be `true`, got `false`
- Expected `LatestVersion` to be `"def456"`, got `"abc123"`
- The test expects the vendor update checker to detect when a git repository has a newer commit hash available

### 2. TestYAMLVersionUpdater_PreserveAnchors
**Location**: internal/exec/vendor_update_test.go
**Error**:
- Panic: runtime error - invalid memory address or nil pointer dereference
- Stack trace points to issue in `github.com/goccy/go-yaml/ast.(*MappingValueNode).String`
- The YAML parser is encountering a nil pointer when trying to preserve YAML anchors

### 3. TestCLICommands/atmos_describe_config
**Location**: tests/cli_test.go:1142
**Error**:
- Output mismatch in golden snapshot test
- Missing the `import` section with `process_without_context: false` field
- The actual output is missing:
  ```json
  "import": {
    "process_without_context": false
  }
  ```

### 4. TestCLICommands/atmos_doesnt_warn_if_not_in_git_repo_with_atmos_config
**Location**: tests/cli_test.go:1142
**Error**:
- Expected to see git warning message in stderr
- Expected: `WARN You're not inside a git repository. Atmos feels lonely outside - bring it home!`
- Got: Stack list output instead (tenant1-ue2-dev, tenant1-ue2-prod, etc.)
- The test expects the CLI to warn about not being in a git repo, but it's outputting stack names instead

## Affected Platforms
- Ubuntu Linux (ubuntu-latest)
- Windows (windows-latest)
- macOS (macos-latest)

## Root Causes

1. **Vendor Update Logic**: The commit hash comparison logic in the vendor update feature isn't correctly detecting newer commits
2. **YAML Parsing**: Nil pointer issue when preserving YAML anchors in vendor configurations
3. **Config Output**: The `import` configuration section isn't being included in the describe config output
4. **Git Repo Detection**: The CLI behavior changed - it's outputting stack names instead of showing the expected warning

## Next Steps

1. Fix the vendor update commit hash comparison logic
2. Add nil checks in YAML anchor preservation code
3. Ensure the import config section is included in describe output
4. Review git repo detection and warning logic
