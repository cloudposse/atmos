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

### Two Separate Environment Variables

We need two distinct controls for configuration discovery:

#### 1. `ATMOS_CONFIG_GIT_ROOT_DISCOVERY` - Git Root Discovery Control

**Purpose:** Controls whether Atmos searches for `atmos.yaml` at the git repository root.

**Default:** `true` (ENABLED by default)

**Rationale:** This matches user expectations - Git-like tools naturally find configuration at repository root. Users expect this behavior by default.

```bash
# Default behavior (git root discovery ENABLED)
cd /path/to/repo/components/terraform/vpc
atmos terraform plan vpc --stack dev
# Works! Finds atmos.yaml at /path/to/repo/

# Disable git root discovery
export ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false
atmos terraform plan vpc --stack dev
# Fails - only looks in current directory
```

#### 2. `ATMOS_DISCOVERY_ACROSS_FILESYSTEM` - Broad Search Control

**Purpose:** Controls whether Atmos searches across multiple filesystem locations (system dir, home dir, etc.) or restricts search to current directory only.

**Default:** `true` (ENABLED by default - searches all locations)

**Rationale:** This is a performance/security control for environments where you want to restrict config discovery to the current working directory only.

```bash
# Default behavior (searches all locations)
# embedded → system → home → current dir → git root → env → args
atmos terraform plan

# Restrict to current directory only
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false
# Only searches: embedded → current dir → env → args
# Skips: system dir, home dir, git root
atmos terraform plan
```

### Git's Terminology for Reference

Git uses these environment variables:
- **`GIT_DISCOVERY_ACROSS_FILESYSTEM`** - Controls whether Git crosses filesystem boundaries during repository discovery (default: false)
- **`GIT_CEILING_DIRECTORIES`** - Limits how far Git searches for `.git` directory (performance optimization)

Our naming:
- **`ATMOS_CONFIG_GIT_ROOT_DISCOVERY`** - Atmos-specific control for git root config lookup (default: true)
- **`ATMOS_DISCOVERY_ACROSS_FILESYSTEM`** - Broader control, inspired by Git but different purpose (default: true)

### Updated Search Path Order

**Default behavior** (both variables true or unset):

1. **Embedded configuration** - Default configuration
2. **System directory** - `/usr/local/etc/atmos` or `%LOCALAPPDATA%/atmos`
3. **Home directory** - `~/.atmos/atmos.yaml`
4. **Current working directory** - `./atmos.yaml`
5. **Git repository root** - `<git-root>/atmos.yaml` ⬅️ **NEW**
6. **Environment variables** - `ATMOS_*` overrides
7. **Command-line arguments** - Final overrides

**With `ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false`** (restricted mode):

1. **Embedded configuration** - Default configuration
2. **Current working directory** - `./atmos.yaml` (only filesystem location searched)
3. **Git repository root** - `<git-root>/atmos.yaml` (if `ATMOS_CONFIG_GIT_ROOT_DISCOVERY=true`)
4. **Environment variables** - `ATMOS_*` overrides
5. **Command-line arguments** - Final overrides

Skips: system directory, home directory

**With `ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false`** (disable git root):

Same as default behavior but **skips step 5** (git repository root search).

**Why git root comes after current directory:** Local overrides (e.g., for testing) still work. This matches the principle of "most specific wins" in configuration precedence.

### Implementation Plan

#### 1. Add `readGitRootConfig()` Function

Location: `pkg/config/load.go`

```go
// readGitRootConfig attempts to find and load atmos.yaml from the git repository root.
// Only runs if ATMOS_CONFIG_GIT_ROOT_DISCOVERY is not explicitly set to "false" or "0".
func readGitRootConfig(v *viper.Viper) error {
    // Check if git root discovery is disabled.
    gitRootDiscovery := os.Getenv("ATMOS_CONFIG_GIT_ROOT_DISCOVERY")
    if gitRootDiscovery == "false" || gitRootDiscovery == "0" {
        log.Debug("Git root discovery disabled via ATMOS_CONFIG_GIT_ROOT_DISCOVERY")
        return nil
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

#### 2. Update `InitCliConfig()` to Respect Both Variables

Modify config loading to respect `ATMOS_DISCOVERY_ACROSS_FILESYSTEM`:

```go
func InitCliConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
    // ... existing code ...

    // Check if broad discovery is restricted.
    discoveryAcrossFS := os.Getenv("ATMOS_DISCOVERY_ACROSS_FILESYSTEM")
    restrictedMode := discoveryAcrossFS == "false" || discoveryAcrossFS == "0"

    // Always read embedded config.
    if err := readEmbeddedConfig(v); err != nil {
        return atmosConfig, err
    }

    // In restricted mode, skip system and home directories.
    if !restrictedMode {
        if err := readSystemDirConfig(v); err != nil {
            return atmosConfig, err
        }
        if err := readHomeDirConfig(v); err != nil {
            return atmosConfig, err
        }
    }

    // Always read current working directory.
    if err := readWorkDirConfig(v); err != nil {
        return atmosConfig, err
    }

    // NEW: Add git root discovery (controlled by separate variable).
    if err := readGitRootConfig(v); err != nil {
        return atmosConfig, err
    }

    // Always read environment variables and CLI args.
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

Modify `tests/cli_workdir_git_root_test.go` to test both variables:

```go
func TestWorkdirGitRootDetection(t *testing.T) {
    // Test 1: Default behavior (git root discovery enabled by default).
    t.Run("default behavior - git root discovery enabled", func(t *testing.T) {
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        // Should SUCCEED - atmos.yaml found via git root by default.
        assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
    })

    // Test 2: Git root discovery explicitly disabled.
    t.Run("with ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false", func(t *testing.T) {
        t.Setenv("ATMOS_CONFIG_GIT_ROOT_DISCOVERY", "false")
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        // Should FAIL - git root discovery disabled.
        assert.Contains(t, stderr, "atmos.yaml CLI config file was not found")
    })

    // Test 3: Restricted filesystem discovery mode.
    t.Run("with ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false", func(t *testing.T) {
        t.Setenv("ATMOS_DISCOVERY_ACROSS_FILESYSTEM", "false")
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        // Git root discovery still works (separate control).
        // Only system/home dirs are skipped.
        assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
    })

    // Test 4: Both disabled.
    t.Run("with both variables false", func(t *testing.T) {
        t.Setenv("ATMOS_CONFIG_GIT_ROOT_DISCOVERY", "false")
        t.Setenv("ATMOS_DISCOVERY_ACROSS_FILESYSTEM", "false")
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        // Should FAIL - only current dir searched, no git root.
        assert.Contains(t, stderr, "atmos.yaml CLI config file was not found")
    })
}
```

#### 5. Update Documentation

Add to `website/docs/cli/configuration.mdx`:

```markdown
## Configuration Discovery

Atmos searches for `atmos.yaml` in multiple locations. Two environment variables control this behavior:

### Git Repository Root Discovery

**Default:** Enabled

By default, Atmos automatically searches for `atmos.yaml` at the git repository root,
allowing you to run commands from any subdirectory (similar to how Git itself works).

```bash
# Repository structure
/path/to/repo/
├── atmos.yaml
└── components/terraform/vpc/

# Works from subdirectories by default
cd /path/to/repo/components/terraform/vpc
atmos terraform plan vpc --stack dev  # Finds atmos.yaml at repo root
```

To **disable** git root discovery:

```bash
export ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false
```

### Filesystem Discovery Control

**Default:** Enabled

By default, Atmos searches multiple locations for configuration:
- Embedded configuration
- System directory (`/usr/local/etc/atmos`)
- Home directory (`~/.atmos/`)
- Current working directory
- Git repository root
- Environment variables
- Command-line arguments

To **restrict** discovery to only current directory (and git root if enabled):

```bash
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false
```

This skips system and home directory searches, useful for:
- CI/CD environments with strict security requirements
- Performance optimization when system/home configs aren't needed
- Ensuring only project-local configuration is used

### Combined Examples

```bash
# Default: Full discovery (recommended for development)
atmos terraform plan

# Restricted mode: Only current dir + git root
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false
atmos terraform plan

# Minimal mode: Only current dir (no git root, no system/home)
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false
export ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false
atmos terraform plan

# Disable only git root (keep system/home search)
export ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false
atmos terraform plan
```
```

### Backward Compatibility

**Breaking change (justified):**

- **Before:** Users running Atmos from subdirectories would get "atmos.yaml not found" error
- **After:** Git root discovery is **enabled by default** - configs are found automatically

**Why this is acceptable:**
1. Matches user expectations (Git-like behavior)
2. Reduces need for `--chdir` workarounds
3. Can be disabled if needed: `ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false`
4. Improves developer experience significantly

**Migration path:**
- No migration needed for users with `atmos.yaml` in project root
- Users who relied on "not found" behavior can set `ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false`
- `--chdir` flag still works and takes precedence

### Alternative Approaches Considered

#### ❌ Option 1: Add `settings.config.auto_discover_git_root` to atmos.yaml

**Rejected reason:** Circular dependency - can't use config file to control config file discovery.

#### ❌ Option 2: Make git root discovery opt-in (disabled by default)

**Rejected reason:** Forces users to continue using `--chdir` workarounds. The whole point is to make Atmos work like Git out of the box.

#### ❌ Option 3: Use `ATMOS_DISCOVERY_ACROSS_FILESYSTEM` for both purposes

**Rejected reason:** Conflates two separate concerns:
- Git root discovery (should be default behavior)
- Broad filesystem search control (performance/security optimization)

#### ✅ Option 4: Two separate variables (SELECTED)

**Selected reason:**
- **`ATMOS_CONFIG_GIT_ROOT_DISCOVERY`** - Affirmative control for git root (default: true)
- **`ATMOS_DISCOVERY_ACROSS_FILESYSTEM`** - Broader discovery control (default: true)
- Clear separation of concerns
- Flexible control for different use cases
- Default behavior matches user expectations

## Testing Strategy

### Unit Tests

Add tests to `pkg/config/load_test.go`:

```go
func TestReadGitRootConfig(t *testing.T) {
    tests := []struct {
        name                string
        gitRootDiscovery    string
        gitRootExists       bool
        configAtGitRoot     bool
        expectConfigLoaded  bool
    }{
        {
            name:               "default (enabled) - load from git root",
            gitRootDiscovery:   "",  // Unset = enabled
            gitRootExists:      true,
            configAtGitRoot:    true,
            expectConfigLoaded: true,
        },
        {
            name:               "explicitly enabled - load from git root",
            gitRootDiscovery:   "true",
            gitRootExists:      true,
            configAtGitRoot:    true,
            expectConfigLoaded: true,
        },
        {
            name:               "disabled - skip git root",
            gitRootDiscovery:   "false",
            gitRootExists:      true,
            configAtGitRoot:    true,
            expectConfigLoaded: false,
        },
        {
            name:               "enabled - no git root available",
            gitRootDiscovery:   "true",
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

func TestDiscoveryAcrossFilesystem(t *testing.T) {
    tests := []struct {
        name                    string
        discoveryAcrossFS       string
        expectSystemSearch      bool
        expectHomeSearch        bool
        expectGitRootSearch     bool
    }{
        {
            name:                "default (enabled) - search all",
            discoveryAcrossFS:   "",
            expectSystemSearch:  true,
            expectHomeSearch:    true,
            expectGitRootSearch: true,
        },
        {
            name:                "disabled - skip system/home",
            discoveryAcrossFS:   "false",
            expectSystemSearch:  false,
            expectHomeSearch:    false,
            expectGitRootSearch: true, // Git root is separate control
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Integration Tests

Comprehensive tests in `tests/cli_workdir_git_root_test.go` covering all combinations.

### Manual Testing

```bash
# Test 1: Default behavior (should work from subdirectory)
cd /path/to/repo/components/terraform/vpc
atmos terraform plan vpc --stack dev
# Expected: Success - finds atmos.yaml at repo root

# Test 2: Disable git root discovery
export ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false
atmos terraform plan vpc --stack dev
# Expected: Error - atmos.yaml not found

# Test 3: Restricted filesystem mode
unset ATMOS_CONFIG_GIT_ROOT_DISCOVERY
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false
atmos terraform plan vpc --stack dev
# Expected: Success - still finds via git root (separate control)

# Test 4: Both disabled (minimal mode)
export ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false
atmos terraform plan vpc --stack dev
# Expected: Error - only current dir searched

# Test 5: --chdir still works (highest precedence)
unset ATMOS_CONFIG_GIT_ROOT_DISCOVERY
unset ATMOS_DISCOVERY_ACROSS_FILESYSTEM
cd /path/to/somewhere/else
atmos --chdir /path/to/repo terraform plan vpc --stack dev
# Expected: Success - explicit chdir overrides everything
```

## Success Criteria

1. ✅ Git root discovery works by default without any configuration
2. ✅ Users can run Atmos from any subdirectory (matches Git behavior)
3. ✅ Can be disabled via `ATMOS_CONFIG_GIT_ROOT_DISCOVERY=false`
4. ✅ `ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false` restricts broad search
5. ✅ All existing tests pass
6. ✅ New tests verify both enabled and disabled modes
7. ✅ `--chdir` flag continues to work and takes precedence
8. ✅ Documentation updated with clear examples

## Future Enhancements

### Potential Future Addition: `ATMOS_CEILING_DIRECTORIES`

If users need to limit git root search (e.g., for performance with slow network mounts):

```bash
# Stop searching when reaching these directories
export ATMOS_CEILING_DIRECTORIES="/mnt/slow-network:/mnt/tape-backup"
```

This would match Git's `GIT_CEILING_DIRECTORIES` exactly. Implementation would modify `readGitRootConfig()` to check ceiling directories before searching.

**Not implementing now** because:
- No user requests for this feature
- Adds complexity without clear need
- Can be added later without breaking changes
- Git's implementation is for performance optimization with slow network drives

## References

- Git Environment Variables: https://git-scm.com/book/en/v2/Git-Internals-Environment-Variables
- Git Documentation: https://git-scm.com/docs/git
- `GIT_DISCOVERY_ACROSS_FILESYSTEM`: Controls whether Git crosses filesystem boundaries during repository discovery
- `GIT_CEILING_DIRECTORIES`: Limits how far Git searches for `.git` directory
- Issue #1746: Unified flag parsing overhaul
- PR #970325e1e: Added `--chdir` flag as workaround (October 2025)

## Implementation Checklist

- [ ] Add `readGitRootConfig()` function to `pkg/config/load.go`
- [ ] Update `InitCliConfig()` to respect both `ATMOS_CONFIG_GIT_ROOT_DISCOVERY` and `ATMOS_DISCOVERY_ACROSS_FILESYSTEM`
- [ ] Add unit tests to `pkg/config/load_test.go` for both variables
- [ ] Update integration tests in `tests/cli_workdir_git_root_test.go` with all combinations
- [ ] Update documentation in `website/docs/cli/configuration.mdx`
- [ ] Add examples to `website/docs/core-concepts/projects/configuration.mdx`
- [ ] Update schemas in `pkg/datafetcher/schema/` if needed
- [ ] Run full test suite: `make testacc`
- [ ] Verify no linting errors: `make lint`
- [ ] Manual testing with real repository (all 5 test scenarios)
- [ ] Update CHANGELOG.md with breaking change notice
