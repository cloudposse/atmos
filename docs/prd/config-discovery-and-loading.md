# PRD: Atmos Configuration Discovery and Loading

## Overview

This document describes how Atmos discovers and loads its CLI configuration file (`atmos.yaml`), including the current behavior, limitations, and proposed enhancements to support git repository root discovery.

## Current Behavior

### Configuration Search Paths

Atmos searches for `atmos.yaml` in the following locations, in order of precedence (last found wins due to Viper merging):

1. **Embedded configuration** - Default configuration embedded in the Atmos binary (`atmos/pkg/config/atmos.yaml`)
2. **System directory**
   - Linux/macOS: `/usr/local/etc/atmos`
   - Windows: `%LOCALAPPDATA%/atmos`
3. **Home directory** - `~/.atmos/atmos.yaml`
4. **Current working directory** - `./atmos.yaml`
5. **Environment variables** - `ATMOS_*` variables override config values
6. **Command-line arguments** - Flags override all previous sources

### Implementation Details

Configuration loading is implemented in `pkg/config/load.go` with these key functions:

- `InitCliConfig()` - Main entry point that orchestrates config loading
- `readEmbeddedConfig()` - Loads embedded default config
- `readSystemDirConfig()` - Searches system directories
- `readHomeDirConfig()` - Searches home directory
- `readWorkDirConfig()` - **Only searches current working directory** via `os.Getwd()`
- `readEnvVars()` - Processes `ATMOS_*` environment variables
- `readCliArgs()` - Processes command-line flags

### Current Limitations

**Issue 1: No Git Root Discovery**

When users run Atmos from a subdirectory within a repository (e.g., `components/terraform/vpc/`), Atmos does **not** automatically discover the git repository root to find `atmos.yaml`. This differs from Git's behavior and causes user confusion.

```bash
# Repository structure
/path/to/repo/
├── atmos.yaml                    # Configuration here
└── components/terraform/vpc/     # User runs from here
    └── main.tf

# Current behavior (FAILS)
$ cd /path/to/repo/components/terraform/vpc
$ atmos terraform plan vpc --stack dev
# Error: atmos.yaml CLI config file was not found

# Required workaround
$ atmos --chdir /path/to/repo terraform plan vpc --stack dev
```

**Issue 2: Workaround Flag Added Instead of Fix**

The `--chdir` flag was added as a workaround (PR #970325e1e, October 2025) rather than implementing proper git root discovery. While `--chdir` is useful for other purposes, it shouldn't be required for basic repository navigation.

**Issue 3: Inconsistent with Git Behavior**

Users expect Atmos to behave like Git - automatically finding the repository root from any subdirectory. This is a common pattern in developer tools (`git`, `npm`, `cargo`, etc.).

### Why Git Root Discovery Wasn't Implemented

Historical context from codebase investigation:

1. **October 2025 redesign** - Atmos config loading was redesigned to use explicit search paths
2. **Workaround added** - `--chdir` flag was added instead of git root discovery
3. **ProcessTagGitRoot() exists** - `pkg/utils/git.go` already has git root detection for template functions
4. **Never connected** - Git root detection was never integrated into config loading

## Proposed Solution

### Git-Aligned Discovery Behavior

Add git repository root to the configuration search path, following Git's model for repository discovery.

### Environment Variable: `ATMOS_DISCOVERY_ACROSS_FILESYSTEM`

**Name rationale:** Matches Git's `GIT_DISCOVERY_ACROSS_FILESYSTEM` environment variable exactly, making the behavior intuitive for users familiar with Git.

**Git's variables for reference:**
- `GIT_DISCOVERY_ACROSS_FILESYSTEM` - Boolean to enable crossing filesystem boundaries (default: false)
- `GIT_CEILING_DIRECTORIES` - Colon-separated paths to stop searching (performance optimization)

**Atmos implementation:**

```bash
# Default behavior (not set or false) - Current behavior maintained
# Searches only: embedded → system → home → current dir → env → args
ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false

# Enable git root discovery (opt-in)
# Searches: embedded → system → home → current dir → git root → env → args
ATMOS_DISCOVERY_ACROSS_FILESYSTEM=true
```

### Updated Search Path Order

With `ATMOS_DISCOVERY_ACROSS_FILESYSTEM=true`:

1. **Embedded configuration** - Default configuration
2. **System directory** - `/usr/local/etc/atmos` or `%LOCALAPPDATA%/atmos`
3. **Home directory** - `~/.atmos/atmos.yaml`
4. **Current working directory** - `./atmos.yaml`
5. **Git repository root** - `<git-root>/atmos.yaml` ⬅️ **NEW**
6. **Environment variables** - `ATMOS_*` overrides
7. **Command-line arguments** - Final overrides

**Why this order:** Git root comes *after* current directory so that local overrides (e.g., for testing) still work. This matches the principle of "most specific wins" in configuration precedence.

### Implementation Plan

#### 1. Add `readGitRootConfig()` Function

Location: `pkg/config/load.go`

```go
// readGitRootConfig attempts to find and load atmos.yaml from the git repository root.
// Only runs if ATMOS_DISCOVERY_ACROSS_FILESYSTEM is set to "true" or "1".
func readGitRootConfig(v *viper.Viper) error {
    // Check if discovery is enabled.
    discoveryEnabled := os.Getenv("ATMOS_DISCOVERY_ACROSS_FILESYSTEM")
    if discoveryEnabled != "true" && discoveryEnabled != "1" {
        return nil // Skip git root discovery
    }

    // Use existing ProcessTagGitRoot() to find repository root.
    gitRoot, err := u.ProcessTagGitRoot("")
    if err != nil {
        // Not in a git repository or git not available - not an error.
        log.Debug("Git root not found, skipping git root config search", "error", err)
        return nil
    }

    // Try to load atmos.yaml from git root.
    err = mergeConfig(v, gitRoot, CliConfigFileName, true)
    if err != nil {
        // Config not found at git root is not an error.
        log.Debug("No atmos.yaml found at git root", "path", gitRoot)
        return nil
    }

    log.Debug("Loaded configuration from git root", "path", gitRoot)
    return nil
}
```

#### 2. Update `InitCliConfig()` Call Chain

Add `readGitRootConfig()` after `readWorkDirConfig()`:

```go
func InitCliConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
    // ... existing code ...

    // Read configs from search paths in order.
    if err := readEmbeddedConfig(v); err != nil {
        return atmosConfig, err
    }
    if err := readSystemDirConfig(v); err != nil {
        return atmosConfig, err
    }
    if err := readHomeDirConfig(v); err != nil {
        return atmosConfig, err
    }
    if err := readWorkDirConfig(v); err != nil {
        return atmosConfig, err
    }
    // NEW: Add git root discovery.
    if err := readGitRootConfig(v); err != nil {
        return atmosConfig, err
    }
    if err := readEnvVars(v); err != nil {
        return atmosConfig, err
    }
    // ... rest of function ...
}
```

#### 3. Update `ProcessTagGitRoot()` for Testing

The function already supports `TEST_GIT_ROOT` environment variable for tests:

```go
func ProcessTagGitRoot(input string) (string, error) {
    // Check if we're in test mode and should use a mock Git root.
    if testGitRoot := os.Getenv("TEST_GIT_ROOT"); testGitRoot != "" {
        log.Debug("Using test Git root override", "path", testGitRoot)
        return testGitRoot, nil
    }
    // ... existing git detection code ...
}
```

No changes needed here - testing infrastructure is already in place.

#### 4. Update Tests

Modify `tests/cli_workdir_git_root_test.go` to:

1. Enable discovery: `t.Setenv("ATMOS_DISCOVERY_ACROSS_FILESYSTEM", "true")`
2. Update assertions to expect **success** instead of failure
3. Verify that `atmos.yaml` is found from subdirectories

```go
func TestWorkdirGitRootDetection(t *testing.T) {
    // ... setup code ...

    // Enable git root discovery.
    t.Setenv("ATMOS_DISCOVERY_ACROSS_FILESYSTEM", "true")
    t.Setenv("TEST_GIT_ROOT", absFixturesDir)

    t.Run("terraform plan from component directory", func(t *testing.T) {
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        // Should now SUCCEED - atmos.yaml found via git root.
        assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
        assert.NoError(t, err) // Component-level errors are OK, config must be found.
    })
}
```

#### 5. Update Documentation

Add to `website/docs/cli/configuration.mdx`:

```markdown
## Configuration Discovery

Atmos searches for `atmos.yaml` in multiple locations...

### Git Repository Root Discovery

By default, Atmos only searches for `atmos.yaml` in the current working directory.
To enable automatic discovery of configuration files at the git repository root
(similar to how Git itself works), set:

```bash
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=true
```

This allows you to run Atmos commands from any subdirectory within your repository:

```bash
# Repository structure
/path/to/repo/
├── atmos.yaml
└── components/terraform/vpc/

# Works from subdirectories when discovery is enabled
cd /path/to/repo/components/terraform/vpc
ATMOS_DISCOVERY_ACROSS_FILESYSTEM=true atmos terraform plan vpc --stack dev
```

**Note:** This variable name matches Git's `GIT_DISCOVERY_ACROSS_FILESYSTEM` for consistency.
```

### Backward Compatibility

**No breaking changes:**

- Default behavior remains unchanged (discovery disabled)
- Users must opt-in via environment variable
- All existing configurations continue to work
- `--chdir` flag still works and takes precedence (command-line args override all)

### Alternative Approaches Considered

#### ❌ Option 1: Add `settings.config.auto_discover_git_root` to atmos.yaml

**Rejected reason:** Circular dependency - can't use config file to control config file discovery.

```yaml
# This doesn't work - we need to find atmos.yaml to read this setting
settings:
  config:
    auto_discover_git_root: true
```

#### ❌ Option 2: Always enable git root discovery

**Rejected reason:** Could break existing workflows that rely on current behavior. Opt-in is safer.

#### ❌ Option 3: Use `ATMOS_DISABLE_GIT_ROOT_DISCOVERY`

**Rejected reason:** Negative control (disable) is less intuitive than positive control (enable). Also doesn't match Git's terminology.

#### ✅ Option 4: Use `ATMOS_DISCOVERY_ACROSS_FILESYSTEM` (SELECTED)

**Selected reason:**
- Matches Git's exact terminology
- Positive control (enable) is more intuitive
- Leaves door open for future `ATMOS_CEILING_DIRECTORIES` if needed
- Users familiar with Git immediately understand it

## Testing Strategy

### Unit Tests

Add tests to `pkg/config/load_test.go`:

```go
func TestReadGitRootConfig(t *testing.T) {
    tests := []struct {
        name                string
        discoveryEnabled    string
        gitRootExists       bool
        configAtGitRoot     bool
        expectConfigLoaded  bool
    }{
        {
            name:               "discovery disabled - skip git root",
            discoveryEnabled:   "false",
            gitRootExists:      true,
            configAtGitRoot:    true,
            expectConfigLoaded: false,
        },
        {
            name:               "discovery enabled - load from git root",
            discoveryEnabled:   "true",
            gitRootExists:      true,
            configAtGitRoot:    true,
            expectConfigLoaded: true,
        },
        {
            name:               "discovery enabled - no git root",
            discoveryEnabled:   "true",
            gitRootExists:      false,
            configAtGitRoot:    false,
            expectConfigLoaded: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation using TEST_GIT_ROOT
        })
    }
}
```

### Integration Tests

Modify existing `tests/cli_workdir_git_root_test.go`:

```go
func TestWorkdirGitRootDetection(t *testing.T) {
    // Test with discovery enabled.
    t.Run("with ATMOS_DISCOVERY_ACROSS_FILESYSTEM=true", func(t *testing.T) {
        t.Setenv("ATMOS_DISCOVERY_ACROSS_FILESYSTEM", "true")
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        // Test should PASS - config found.
    })

    // Test with discovery disabled (default).
    t.Run("with ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false", func(t *testing.T) {
        t.Setenv("ATMOS_DISCOVERY_ACROSS_FILESYSTEM", "false")
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        // Test should FAIL - config not found (current behavior).
    })
}
```

### Manual Testing

```bash
# Setup
cd ~/Dev/cloudposse/infra/infra-live

# Test 1: Default behavior (discovery disabled)
cd components/terraform/vpc
atmos terraform plan vpc --stack dev
# Expected: Error - atmos.yaml not found

# Test 2: Enable discovery
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=true
atmos terraform plan vpc --stack dev
# Expected: Success - atmos.yaml found at git root

# Test 3: --chdir still works
unset ATMOS_DISCOVERY_ACROSS_FILESYSTEM
atmos --chdir ~/Dev/cloudposse/infra/infra-live terraform plan vpc --stack dev
# Expected: Success - explicit chdir overrides
```

## Success Criteria

1. ✅ Users can run Atmos from any subdirectory when `ATMOS_DISCOVERY_ACROSS_FILESYSTEM=true`
2. ✅ Default behavior (discovery disabled) remains unchanged
3. ✅ All existing tests pass
4. ✅ New tests verify both enabled and disabled discovery
5. ✅ `--chdir` flag continues to work and takes precedence
6. ✅ Documentation updated with clear examples
7. ✅ No breaking changes for existing users

## Future Enhancements

### Potential Future Addition: `ATMOS_CEILING_DIRECTORIES`

If users need to limit git root search (e.g., for performance with slow network mounts):

```bash
# Stop searching when reaching these directories
export ATMOS_CEILING_DIRECTORIES="/mnt/slow-network:/mnt/tape-backup"
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=true
```

This would match Git's `GIT_CEILING_DIRECTORIES` exactly. Implementation would modify `readGitRootConfig()` to check ceiling directories before searching.

**Not implementing now** because:
- No user requests for this feature
- Adds complexity without clear need
- Can be added later without breaking changes

## References

- Git Environment Variables: https://git-scm.com/book/en/v2/Git-Internals-Environment-Variables
- Git Documentation: https://git-scm.com/docs/git
- `GIT_DISCOVERY_ACROSS_FILESYSTEM`: Controls whether Git crosses filesystem boundaries during repository discovery
- `GIT_CEILING_DIRECTORIES`: Limits how far Git searches for `.git` directory
- Issue #1746: Unified flag parsing overhaul
- PR #970325e1e: Added `--chdir` flag as workaround (October 2025)

## Implementation Checklist

- [ ] Add `readGitRootConfig()` function to `pkg/config/load.go`
- [ ] Update `InitCliConfig()` to call `readGitRootConfig()` after `readWorkDirConfig()`
- [ ] Add unit tests to `pkg/config/load_test.go`
- [ ] Update integration tests in `tests/cli_workdir_git_root_test.go`
- [ ] Update documentation in `website/docs/cli/configuration.mdx`
- [ ] Add example to `website/docs/core-concepts/projects/configuration.mdx`
- [ ] Update schemas in `pkg/datafetcher/schema/` if needed
- [ ] Run full test suite: `make testacc`
- [ ] Verify no linting errors: `make lint`
- [ ] Manual testing with real repository
- [ ] Update CHANGELOG.md
