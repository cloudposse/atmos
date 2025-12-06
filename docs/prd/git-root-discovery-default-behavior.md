# PRD: Git Root Discovery as Default Behavior

## Problem Statement

Atmos currently requires users to run commands from the repository root directory (where `atmos.yaml` is located), unlike Git which can be run from any subdirectory. This creates friction in developer workflows, especially in monorepo structures.

### Current Behavior

**What users expect (Git-like behavior):**
```bash
cd /repo/components/terraform/vpc
atmos terraform plan vpc -s dev
# ✓ Works from any subdirectory (like git commands)
```

**What actually happens:**
```bash
cd /repo/components/terraform/vpc
atmos terraform plan vpc -s dev
# ✗ Error: Can't find atmos.yaml, stacks, or components

# Current workaround required:
atmos --chdir /repo terraform plan vpc -s dev
```

### Technical Challenge

We have the infrastructure to solve this problem:
1. **`!repo-root` YAML function** - Resolves to git repository root
2. **`base_path` configuration** - Determines root for all relative paths (stacks, components, workflows)
3. **Embedded `atmos.yaml`** - Default configuration loaded when no config file exists

**Ideal solution:** Set `base_path: !repo-root` as default in embedded config.

**Why it fails:**
- Embedded config is loaded **before** YAML function processing
- YAML functions (`!repo-root`, `!env`, `!exec`) only work in user config files
- Setting `base_path: !repo-root` in embedded config results in literal string `"!repo-root"`
- All path resolution breaks with invalid base path

### Previous Attempts

#### Attempt 1: `base_path: !repo-root` in Embedded Config (Commit 3088597a)
**Status:** Reverted
**Duration:** 1 day (Nov 4, 2025)

```yaml
# pkg/config/atmos.yaml
base_path: !repo-root  # ✗ Not processed, becomes literal string
```

**Problems:**
- Embedded config loaded via `loadEmbeddedConfig()` at line 50 of `load.go`
- YAML functions only processed in `mergeConfig()` (line 448) for external files
- Chicken-and-egg: Can't process YAML functions until config loaded, but need processed value during loading
- All tests broke - base path pointed to non-existent `"!repo-root"` directory

#### Attempt 2: `ATMOS_GIT_ROOT_ENABLED` Environment Variable (Commit 5c576c38)
**Status:** Reverted
**Duration:** 2 days (Nov 5-7, 2025)

```go
// Added git root as config file search location
func readGitRootConfig() {
    if os.Getenv("ATMOS_GIT_ROOT_ENABLED") != "false" {
        // Search for atmos.yaml in git root
    }
}
```

**Changes:**
- Added git root to config file search path (fallback after workdir, before env vars)
- Required `ATMOS_GIT_ROOT_ENABLED=false` in 5 `TestMain` functions for test isolation
- Created `setup_test.go` files in multiple packages

**Problems:**
- Doesn't solve the actual problem - only finds config file, doesn't change `base_path`
- Users still needed `base_path: !repo-root` in their own config
- Added complexity without delivering git-like behavior by default
- Test pollution - needed manual disabling across codebase

### Test Infrastructure

**Current test isolation mechanisms:**

1. **`TEST_GIT_ROOT` environment variable** (`pkg/utils/git.go:24`)
   ```go
   if testGitRoot := os.Getenv("TEST_GIT_ROOT"); testGitRoot != "" {
       return testGitRoot, nil  // Override git detection
   }
   ```
   - Set in `tests/cli_test.go:802` to mock git repository root
   - Each test gets isolated temporary directory

2. **`TEST_EXCLUDE_ATMOS_D` environment variable** (`pkg/config/load.go:467`)
   ```go
   excludePaths := os.Getenv("TEST_EXCLUDE_ATMOS_D")
   ```
   - Set in `tests/cli_test.go:807` to prevent loading `.atmos.d` from actual repo
   - Prevents test interference from repository-level configuration

3. **Test fixtures with `base_path: !repo-root`**
   - `tests/fixtures/scenarios/atmos-repo-root-yaml-function/`
   - Demonstrates that `!repo-root` works correctly in user configs
   - Tests in `pkg/config/config_test.go:245-278`

**Why tests break:**
- Tests rely on default `BasePath: "."` from `defaultCliConfig` (line 23 of `default.go`)
- Relative paths in tests expect current directory as base
- Changing default to git root makes all test paths resolve to repository root
- Test fixtures not designed for absolute repository-relative paths

## Requirements

### Functional Requirements

**FR1: Git-like Command Execution**
- Users MUST be able to run Atmos commands from any subdirectory within the repository
- Example: `cd components/terraform/vpc && atmos terraform plan vpc -s dev`
- Should work without `--chdir` flag or environment variables

**FR2: Backward Compatibility**
- Existing workflows MUST continue to work
- Users who run commands from repository root should see no change
- Users with explicit `base_path: "."` in their config must be respected

**FR3: Graceful Fallback**
- If not in a git repository, fall back to current directory behavior
- No errors or warnings if `.git` not found
- Should work for users without git installed

**FR4: Zero Configuration**
- Default behavior should "just work" for new users
- No configuration required to enable git-like behavior
- Explicit opt-out should be possible for users who want old behavior

**FR5: Test Isolation**
- Tests MUST NOT be affected by actual repository structure
- Tests should continue using relative paths from test fixtures
- Test suite should pass with git-like behavior enabled in production

### Non-Functional Requirements

**NFR1: Performance**
- Git root detection should not noticeably slow down command execution
- Caching of git root path per command invocation acceptable

**NFR2: Cross-Platform**
- Must work on Linux, macOS, and Windows
- Path resolution must handle platform-specific separators

**NFR3: Error Messages**
- Clear error messages if config/stacks/components not found
- Should guide users to run from repository or use `--chdir`

**NFR4: Logging**
- DEBUG level logs should show git root detection decisions
- TRACE level logs for detailed git root resolution

## Architecture

### Current Architecture

```
Config Loading Flow:
1. loadEmbeddedConfig() - Loads pkg/config/atmos.yaml directly
2. loadConfigSources() - Searches for user config files
3. mergeConfig() - Merges configs, processes YAML functions
4. v.Unmarshal() - Convert to AtmosConfiguration struct

YAML Function Processing:
- Only happens in mergeConfig() (line 448 of load.go)
- Uses preprocessAtmosYamlFunc() from process_yaml.go
- Calls handleGitRoot() for !repo-root tags
```

**Key insight:** Embedded config bypasses YAML function processing.

### Config Search Order (Current)

1. **Embedded config** - `pkg/config/atmos.yaml` (no YAML functions)
2. **System directory** - `/usr/local/etc/atmos/atmos.yaml`
3. **Home directory** - `~/.atmos/atmos.yaml`
4. **Current working directory** - `./atmos.yaml`
5. **Environment variable** - `ATMOS_CLI_CONFIG_PATH`
6. **CLI argument** - `--config-dir` flag

### BasePath Usage

**BasePath is the root for all relative paths:**
```go
// Line 190 of internal/exec/stack_utils.go
componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)

// Line 326 of internal/exec/workflow_utils.go
workflowsDir = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath)

// Line 575 of cmd/cmd_utils.go
stacksDir := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)
```

**Everything depends on BasePath:**
- Component directories (terraform, helmfile, packer)
- Stack configurations
- Workflows
- Vendor configs
- Schema directories

## Solution Options

### Option 1: Post-Process BasePath After Config Load ⭐ **RECOMMENDED**

**Approach:** Apply `!repo-root` processing to `BasePath` field after config is loaded.

**Implementation:**
```go
// In LoadConfig() after v.Unmarshal()
if atmosConfig.BasePath == "" || atmosConfig.BasePath == "." {
    // No explicit base_path - use git root if available
    gitRoot, err := u.ProcessTagGitRoot("!repo-root .")
    if err == nil && gitRoot != "." {
        atmosConfig.BasePath = gitRoot
        log.Debug("Using git repository root as base path", "path", gitRoot)
    }
}
```

**Flow:**
1. Load embedded config with `BasePath: "."`
2. Merge user configs (may override `base_path`)
3. Unmarshal to struct
4. **NEW STEP:** If `BasePath` is default (`.`), resolve to git root
5. Continue with existing logic

**Advantages:**
- ✅ No changes to embedded config or YAML processing
- ✅ Only affects behavior when user hasn't explicitly set `base_path`
- ✅ Works with existing `!repo-root` infrastructure
- ✅ Can be controlled by flag/env var for tests
- ✅ Respects user configuration (explicit `base_path: "."` honored)
- ✅ Clear separation of concerns

**Disadvantages:**
- ❌ Bypasses normal YAML function processing
- ❌ Special case handling for one field
- ❌ Tests still need isolation mechanism

**Test Strategy:**
```go
// Option A: Environment variable control
if os.Getenv("ATMOS_DISABLE_GIT_ROOT_BASEPATH") == "true" {
    // Skip git root resolution - use default "."
}

// Option B: Detect test environment
if strings.Contains(atmosConfig.BasePath, "test-fixtures") {
    // Already in test mode, don't override
}

// Option C: Check if Default flag is set
if atmosConfig.Default && os.Getenv("ATMOS_GIT_ROOT_BASEPATH") != "false" {
    // Only auto-detect git root for default config
}
```

### Option 2: Dual Embedded Configs

**Approach:** Ship two embedded configs, select based on environment.

**Implementation:**
```go
//go:embed atmos.yaml
var embeddedConfigDefault []byte

//go:embed atmos.test.yaml
var embeddedConfigTest []byte

func loadEmbeddedConfig(v *viper.Viper) error {
    data := embeddedConfigDefault
    if os.Getenv("ATMOS_TEST_MODE") == "true" {
        data = embeddedConfigTest
    }
    // ...
}
```

**atmos.yaml:**
```yaml
base_path: !repo-root  # Production default
```

**atmos.test.yaml:**
```yaml
base_path: .  # Test default
```

**Advantages:**
- ✅ Uses standard YAML function processing
- ✅ Clear separation between production and test configs
- ✅ No special-case code in config loading

**Disadvantages:**
- ❌ YAML functions still not processed in embedded config
- ❌ Doesn't actually solve the problem
- ❌ Requires environment variable in all test runners
- ❌ Maintenance burden of two configs
- ❌ Could drift over time

### Option 3: Process Embedded Config Through YAML Functions

**Approach:** Enable YAML function processing for embedded config.

**Implementation:**
```go
func loadEmbeddedConfig(v *viper.Viper) error {
    // Process YAML functions in embedded config
    processed, err := preprocessAtmosYamlFunc(embeddedConfigData, v)
    if err != nil {
        return err
    }
    return v.MergeConfig(bytes.NewReader(processed))
}
```

**Advantages:**
- ✅ Treats embedded config like any other config file
- ✅ Can use `base_path: !repo-root` in embedded config
- ✅ Consistent processing for all configs

**Disadvantages:**
- ❌ Circular dependency: YAML functions need FuncMap, which needs config
- ❌ `preprocessAtmosYamlFunc()` expects Viper instance with config already loaded
- ❌ Would require significant refactoring of config loading order
- ❌ Risk of breaking existing YAML function infrastructure

### Option 4: Late-Binding BasePath Resolution

**Approach:** Keep `BasePath` as string, resolve `!repo-root` on first access.

**Implementation:**
```go
type AtmosConfiguration struct {
    basePath string  // Private field
    basePathResolved bool
}

func (c *AtmosConfiguration) GetBasePath() string {
    if !c.basePathResolved {
        if c.basePath == "" || c.basePath == "!repo-root" {
            gitRoot, _ := u.ProcessTagGitRoot("!repo-root .")
            c.basePath = gitRoot
        }
        c.basePathResolved = true
    }
    return c.basePath
}
```

**Advantages:**
- ✅ Lazy evaluation - only resolve when needed
- ✅ Can use `!repo-root` in embedded config
- ✅ Transparent to consumers

**Disadvantages:**
- ❌ Breaking change - all `atmosConfig.BasePath` access must change to `atmosConfig.GetBasePath()`
- ❌ 100+ references throughout codebase
- ❌ Mutation of config struct (not thread-safe)
- ❌ Hides important behavior in getter method

### Option 5: Default to Git Root in defaultCliConfig Struct

**Approach:** Compute git root when creating default config.

**Implementation:**
```go
// In default.go
func getDefaultBasePath() string {
    gitRoot, err := u.ProcessTagGitRoot("!repo-root .")
    if err != nil || gitRoot == "." {
        return "."  // Fallback to current directory
    }
    return gitRoot
}

var defaultCliConfig = schema.AtmosConfiguration{
    Default:  true,
    BasePath: getDefaultBasePath(),  // ✗ Error: can't call function in struct literal
    // ...
}
```

**Advantages:**
- ✅ Changes only affect default config (no user config loaded)
- ✅ Centralized in one location

**Disadvantages:**
- ❌ Can't call functions in struct literals (Go limitation)
- ❌ Would need to refactor from `var` to `func`
- ❌ `defaultCliConfig` used in multiple places
- ❌ Performance: Would compute git root even when config file exists

### Option 6: Environment Variable Toggle (Previous Attempt)

**Approach:** Add flag/env var to enable git root discovery.

**Implementation:**
```go
if os.Getenv("ATMOS_GIT_ROOT_ENABLED") != "false" {
    gitRoot, err := u.ProcessTagGitRoot("!repo-root .")
    if err == nil {
        atmosConfig.BasePath = gitRoot
    }
}
```

**Advantages:**
- ✅ Explicit opt-in/opt-out
- ✅ Tests can disable with `ATMOS_GIT_ROOT_ENABLED=false`
- ✅ Simple implementation

**Disadvantages:**
- ❌ Requires configuration to enable desired default behavior
- ❌ Not git-like by default
- ❌ Every test suite needs the environment variable
- ❌ Users must discover and enable feature
- ❌ Defeats "zero configuration" goal

### Option 7: Auto-Chdir to Git Root (Chdir-like) ⭐ **ALTERNATIVE RECOMMENDED**

**Approach:** Automatically `os.Chdir()` to git root early in startup, just like `--chdir` flag does.

**Implementation:**
```go
// In cmd/root.go PersistentPreRun, BEFORE processChdirFlag()
func autoChangeDirToGitRoot() error {
    // Skip if explicit --chdir flag or ATMOS_CHDIR provided
    if hasExplicitChdir() {
        return nil
    }

    // Skip if disabled for tests
    if os.Getenv("ATMOS_AUTO_CHDIR_GIT_ROOT") == "false" {
        return nil
    }

    // Detect git root
    gitRoot, err := detectGitRoot()
    if err != nil || gitRoot == "" {
        return nil  // Not in git repo, continue from current dir
    }

    // Get current directory
    cwd, err := os.Getwd()
    if err != nil {
        return err
    }

    // Already at git root?
    if cwd == gitRoot {
        return nil
    }

    // Change to git root
    if err := os.Chdir(gitRoot); err != nil {
        return err
    }

    log.Debug("Auto-changed to git repository root", "from", cwd, "to", gitRoot)
    return nil
}
```

**Flow:**
1. Process starts in subdirectory (e.g., `components/terraform/vpc`)
2. **Before config loading**, detect git root
3. `os.Chdir()` to git root (e.g., `/repo`)
4. Config loading finds `atmos.yaml` at root
5. Default `BasePath: "."` now resolves to git root
6. Everything works normally

**Advantages:**
- ✅ **Identical to `--chdir` behavior** - Users already understand this model
- ✅ **Zero special cases** - No `BasePath` manipulation needed
- ✅ **Affects all file operations** - Even non-Atmos paths resolve from git root
- ✅ **Config search "just works"** - Finds `atmos.yaml` at root automatically
- ✅ **No config changes** - Default `BasePath: "."` is perfect
- ✅ **Simple implementation** - Reuse existing chdir logic

**Disadvantages:**
- ❌ **Changes global process state** - `os.Chdir()` affects everything after
- ❌ **Surprising behavior** - `cat main.tf` won't work from subdirectory anymore
- ❌ **Breaks relative paths** - Any scripts using relative paths fail
- ❌ **Can't distinguish** - User's pwd vs Atmos's internal working dir
- ❌ **Test isolation harder** - Need to restore directory after each test

**Critical issue: User expectations**

When user does:
```bash
cd components/terraform/vpc
ls  # Shows vpc files (user's mental model: "I'm in vpc directory")
atmos terraform plan vpc -s dev
cat main.tf  # ✗ ERROR: Not found! (Atmos changed us to git root)
```

User expects to still be in `components/terraform/vpc` for shell commands, but Atmos has changed the working directory globally.

**Verdict:** This approach works technically but violates principle of least surprise. The `--chdir` flag is **explicit** - user says "change to this directory". Auto-chdir is **implicit** - surprising side effect.

### Option 8: Virtual Chdir (Config Search Only) ⭐ **BEST OF BOTH WORLDS**

**Approach:** Search for config as if we chdir'd to git root, but don't actually change directory.

**Implementation:**
```go
// In pkg/config/load.go, modify loadConfigSources()
func loadConfigSources(v *viper.Viper, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
    // Standard search paths
    searchPaths := []string{
        getSystemDir(),
        getHomeDir(),
        ".",  // Current directory
    }

    // If no --chdir and not disabled, also search git root
    if !hasExplicitChdir() && os.Getenv("ATMOS_AUTO_CHDIR_GIT_ROOT") != "false" {
        gitRoot, err := detectGitRoot()
        if err == nil && gitRoot != "" {
            // Add git root to search path
            searchPaths = append(searchPaths, gitRoot)
        }
    }

    // Try each path
    for _, path := range searchPaths {
        configPath := filepath.Join(path, "atmos.yaml")
        if fileExists(configPath) {
            v.SetConfigFile(configPath)
            // Set BasePath to the directory containing atmos.yaml
            configAndStacksInfo.BasePath = path
            break
        }
    }

    // ... rest of loading logic ...
}
```

**Advantages:**
- ✅ **No global state changes** - Working directory unchanged
- ✅ **User's pwd preserved** - `cat main.tf` still works
- ✅ **Config discovery improved** - Finds `atmos.yaml` from git root
- ✅ **Minimal changes** - Only affects config search logic
- ✅ **Test-friendly** - No directory restoration needed

**Disadvantages:**
- ❌ **Not quite chdir-like** - Different mechanism than `--chdir`
- ❌ **BasePath still needs setting** - Must set to git root when config found there
- ❌ **More complex** - Multiple search paths with priority

**Key insight:** This is actually what was tried in Commit 5c576c38 (`ATMOS_GIT_ROOT_ENABLED`), but that version **didn't set BasePath** when finding config at git root.

**Improved version:**
```go
// When config found at git root, set BasePath appropriately
if configFound && configDir == gitRoot && configDir != "." {
    // Found config at git root (not current dir)
    // Set BasePath to git root so all paths resolve correctly
    atmosConfig.BasePath = gitRoot
    log.Debug("Found config at git root, using as base path", "path", gitRoot)
}
```

This combines:
- Config discovery from git root (like chdir)
- No working directory changes (unlike chdir)
- Automatic BasePath setting (for correct path resolution)

## Recommended Solution

There are two viable approaches, each with different trade-offs:

### Approach A: Post-Process BasePath (Simpler) ⭐ **RECOMMENDED FOR INITIAL IMPLEMENTATION**

**Option 1: Post-Process BasePath After Config Load** with **Option C: Check Default Flag**

**Philosophy:** Detect git root and set `BasePath` to it when using default config.

**CRITICAL CONSTRAINT:** Only activate if current working directory does NOT have any Atmos configuration.

**Atmos configuration indicators (any of these means "local config exists"):**
- `atmos.yaml` - Main config file
- `.atmos.yaml` - Hidden config file
- `.atmos/` directory - Atmos config directory
- `.atmos.d/` directory - Default imports directory
- `atmos.d/` directory - Default imports directory (alternate)

**Activation conditions (ALL must be true):**
1. No Atmos configuration in current working directory (none of the above exist)
2. Using default embedded config (Default flag is true)
3. BasePath is default value (".")
4. `ATMOS_GIT_ROOT_BASEPATH` is not set to "false"
5. Currently in a git repository

**Pros:**
- Simpler implementation (~50 lines of code)
- Clear ownership (config package)
- Easy to test in isolation
- Minimal risk
- Works even if config file doesn't exist
- **Respects local config** - If user has `atmos.yaml` in subdirectory, use that instead

**Cons:**
- Doesn't help find `atmos.yaml` file itself
- User must have config at repository root OR no config at all
- If user has `atmos.yaml` in subdirectory, won't search parent directories

### Approach B: Virtual Chdir (More Complete) ⭐ **RECOMMENDED FOR FUTURE**

**Option 8: Virtual Chdir (Config Search + BasePath)**

**Philosophy:** Search for config file from git root (as if we chdir'd), then set BasePath accordingly.

**CRITICAL CONSTRAINT:** Same as Approach A - only activate if current directory does NOT have any Atmos configuration.

**Priority:**
1. Current directory Atmos config (highest priority - local override)
   - `atmos.yaml`, `.atmos.yaml`, `.atmos/`, `.atmos.d/`, `atmos.d/`
2. System directory
3. Home directory
4. Git repository root (NEW - fallback)
5. Environment variable
6. CLI argument

**Pros:**
- Finds `atmos.yaml` even if in parent directory
- More git-like behavior (searches upward like `.git`)
- Helps users who put `atmos.yaml` in subdirectory by mistake
- Matches user mental model better
- **Respects local config** - Current directory always wins

**Cons:**
- More complex implementation (~150 lines of code)
- Changes config discovery logic (higher risk)
- Harder to test (integration tests needed)
- May find "wrong" config file if multiple exist

### Why the "No Local Config" Constraint is Critical

**Problem without constraint:**
```bash
# User intentionally creates subdirectory config for testing
cd experiments/
echo "base_path: ." > atmos.yaml  # Want to test with different settings

# Run atmos
atmos terraform plan

# ✗ UNEXPECTED: Atmos ignores local config, uses git root instead!
# User's local override is silently ignored
```

**With constraint:**
```bash
# Same scenario - ANY Atmos config indicator prevents git root discovery
cd experiments/
echo "base_path: ." > atmos.yaml  # OR mkdir .atmos.d, OR mkdir .atmos

# Run atmos
atmos terraform plan

# ✓ EXPECTED: Atmos uses local config/directory, base_path is "." (experiments/)
# User's local override is respected
```

**Why check all indicators:**

Users may have:
- `.atmos.d/` with custom commands but no `atmos.yaml`
- `.atmos/` directory for local auth/cache
- `atmos.d/` with component configs
- `.atmos.yaml` (hidden config)

If ANY of these exist in current directory, user has intentionally set up local Atmos configuration and expects it to be respected.

**This constraint ensures:**
1. **Predictability** - If user has config in current dir, that's what gets used
2. **No silent overrides** - Local config always takes precedence
3. **Debugging works** - Can create local config to test different settings
4. **Explicit > Implicit** - Explicit local config overrides implicit git root
5. **Matches user expectations** - "If I'm in a directory with `atmos.yaml`, use that"

**Behavior matrix:**

| Location | Has Atmos Config? | Git Repo? | Result |
|----------|-------------------|-----------|--------|
| `/repo/` | ✓ Yes (`atmos.yaml`) | ✓ Yes | Use `/repo/atmos.yaml` with `base_path` from config |
| `/repo/experiments/` | ✓ Yes (`atmos.yaml`) | ✓ Yes | Use `/repo/experiments/atmos.yaml` (local override) |
| `/repo/experiments/` | ✓ Yes (`.atmos.d/`) | ✓ Yes | Use local `.atmos.d/`, don't search git root |
| `/repo/components/terraform/vpc/` | ✗ No | ✓ Yes | Use git root `/repo/` as base path |
| `/repo/components/terraform/vpc/` | ✗ No | ✗ No | Use current dir `.` as base path (fallback) |
| `/tmp/standalone/` | ✗ No | ✗ No | Use current dir `.` as base path (fallback) |

**Note:** "Has Atmos Config" means ANY of: `atmos.yaml`, `.atmos.yaml`, `.atmos/`, `.atmos.d/`, `atmos.d/`

### Decision: Start with Approach A, Consider B Later

**Rationale:**

1. **Most users have config at root** - Standard setup is `atmos.yaml` at repository root
2. **Lower risk** - Post-processing is isolated change
3. **Incremental improvement** - Can add config search later if needed
4. **Faster to implement** - Can ship in 1-2 weeks
5. **Test isolation easier** - Single environment variable
6. **Local override always works** - Critical constraint protects user expectations

**Future enhancement:** If users report issues with config discovery, implement Approach B as Phase 2.

### Implementation Plan (Approach A)

#### Phase 1: Core Implementation

**File:** `pkg/config/load.go`

```go
// After line 94 (after v.Unmarshal(&atmosConfig))
func LoadConfig(configAndStacksInfo *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
    // ... existing code ...

    err := v.Unmarshal(&atmosConfig)
    if err != nil {
        return atmosConfig, err
    }

    // NEW: Apply git root discovery for default base path
    if err := applyGitRootBasePath(&atmosConfig); err != nil {
        log.Debug("Failed to apply git root base path", "error", err)
        // Don't fail config loading, just log
    }

    // ... rest of function ...
}

// applyGitRootBasePath resolves BasePath to git repository root when using default config.
// This enables running Atmos from any subdirectory, similar to Git.
//
// CRITICAL: Only applies when current directory does NOT have atmos.yaml.
// This ensures local configs take precedence over git root discovery.
//
// Only applies when ALL conditions are true:
// 1. No atmos.yaml in current working directory
// 2. Using default embedded config (Default flag is true)
// 3. BasePath is default value (".")
// 4. ATMOS_GIT_ROOT_BASEPATH is not explicitly set to "false"
// 5. Currently in a git repository
//
// Tests can disable by setting ATMOS_GIT_ROOT_BASEPATH=false.
func applyGitRootBasePath(atmosConfig *schema.AtmosConfiguration) error {
    // Allow tests to disable git root discovery
    if os.Getenv("ATMOS_GIT_ROOT_BASEPATH") == "false" {
        log.Trace("Git root base path disabled via ATMOS_GIT_ROOT_BASEPATH=false")
        return nil
    }

    // CRITICAL: Only apply if current directory does NOT have any Atmos configuration
    // This ensures local configs/directories take precedence over git root discovery
    cwd, err := os.Getwd()
    if err != nil {
        log.Trace("Failed to get current directory", "error", err)
        return nil
    }

    // Check for any Atmos configuration indicators in current directory
    if hasLocalAtmosConfig(cwd) {
        return nil
    }

    // Only apply to default config with default base path
    if !atmosConfig.Default {
        log.Trace("Skipping git root base path (not default config)")
        return nil
    }

    if atmosConfig.BasePath != "." && atmosConfig.BasePath != "" {
        log.Trace("Skipping git root base path (explicit base_path set)", "base_path", atmosConfig.BasePath)
        return nil
    }

    // Resolve git root
    gitRoot, err := u.ProcessTagGitRoot("!repo-root .")
    if err != nil {
        log.Trace("Git root detection failed", "error", err)
        return err
    }

    // Only update if we found a different root
    if gitRoot != "." {
        atmosConfig.BasePath = gitRoot
        log.Debug("Using git repository root as base path", "path", gitRoot)
    } else {
        log.Trace("Git root same as current directory")
    }

    return nil
}

// hasLocalAtmosConfig checks if the current directory has any Atmos configuration.
// This includes config files, config directories, and default import directories.
//
// Returns true if ANY of these exist in the given directory:
// - atmos.yaml (main config file)
// - .atmos.yaml (hidden config file)
// - .atmos/ (config directory)
// - .atmos.d/ (default imports directory)
// - atmos.d/ (default imports directory - alternate)
func hasLocalAtmosConfig(dir string) bool {
    configIndicators := []string{
        "atmos.yaml",       // Main config file
        ".atmos.yaml",      // Hidden config file
        ".atmos",           // Config directory
        ".atmos.d",         // Default imports directory
        "atmos.d",          // Default imports directory (alternate)
    }

    for _, indicator := range configIndicators {
        indicatorPath := filepath.Join(dir, indicator)
        if _, err := os.Stat(indicatorPath); err == nil {
            log.Trace("Found Atmos configuration in current directory, skipping git root discovery",
                "indicator", indicator, "path", indicatorPath)
            return true
        }
    }

    return false
}
```

#### Phase 2: Test Isolation

**Update test setup in `tests/cli_test.go`:**

```go
// Around line 802, add environment variable
tc.Env["TEST_GIT_ROOT"] = testGitRoot
tc.Env["TEST_EXCLUDE_ATMOS_D"] = repoRoot
tc.Env["ATMOS_GIT_ROOT_BASEPATH"] = "false"  // NEW: Disable git root base path in tests
```

**Create TestMain in packages that test config loading:**

```go
// internal/exec/setup_test.go
package exec

import (
    "os"
    "testing"
)

func TestMain(m *testing.M) {
    // Disable git root base path discovery for tests
    os.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
    os.Exit(m.Run())
}
```

Similar files needed in:
- `pkg/config/setup_test.go`
- Any other package that tests config loading behavior

#### Phase 3: Documentation

**Update `atmos.yaml` schema documentation:**

```yaml
# base_path sets the root directory for all relative paths in configuration.
# When omitted or set to ".", Atmos automatically detects the git repository root.
# This allows running commands from any subdirectory (git-like behavior).
#
# Default (git repository): Uses git root if detected
# Fallback (no git): Uses current directory (".")
#
# To explicitly use current directory instead of git root:
base_path: "."  # But set Default: false in config
```

**Add to CLI configuration docs:**

#### Git Root Discovery

By default, Atmos uses your git repository root as the base path for all configuration. This allows you to run Atmos commands from any subdirectory, similar to git commands:

```bash
# Works from any directory in your repository
cd components/terraform/vpc
atmos terraform plan vpc -s dev
```

**How it works:**
1. When no `base_path` is configured, Atmos searches for `.git` directory
2. Uses repository root as base for stacks, components, workflows
3. Falls back to current directory if not in a git repository

**Disabling git root discovery:**

If you prefer the old behavior (current directory as base), create `atmos.yaml`:

```yaml
base_path: "."
```

**Environment variable (for CI/tests):**
```bash
export ATMOS_GIT_ROOT_BASEPATH=false
```

#### Phase 4: Verification Tests

**Add test for git root base path:**

```go
// pkg/config/git_root_basepath_test.go
package config

import (
    "os"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestApplyGitRootBasePath(t *testing.T) {
    tests := []struct {
        name           string
        config         schema.AtmosConfiguration
        env            map[string]string
        createLocalCfg bool   // Create atmos.yaml in current directory
        createLocalDir string // Create directory in current directory (e.g., ".atmos.d")
        expectChange   bool
        expectedPath   string
    }{
        {
            name: "default config uses git root",
            config: schema.AtmosConfiguration{
                Default:  true,
                BasePath: ".",
            },
            createLocalCfg: false,
            expectChange:   true,  // Depends on test environment
        },
        {
            name: "local atmos.yaml exists - skip git root discovery",
            config: schema.AtmosConfiguration{
                Default:  true,
                BasePath: ".",
            },
            createLocalCfg: true,  // NEW: Critical test case
            expectChange:   false,
            expectedPath:   ".",
        },
        {
            name: "local .atmos.d/ exists - skip git root discovery",
            config: schema.AtmosConfiguration{
                Default:  true,
                BasePath: ".",
            },
            createLocalCfg: false,
            createLocalDir: ".atmos.d",  // NEW: Directory indicator
            expectChange:   false,
            expectedPath:   ".",
        },
        {
            name: "local .atmos/ exists - skip git root discovery",
            config: schema.AtmosConfiguration{
                Default:  true,
                BasePath: ".",
            },
            createLocalCfg: false,
            createLocalDir: ".atmos",  // NEW: Directory indicator
            expectChange:   false,
            expectedPath:   ".",
        },
        {
            name: "explicit base_path preserved",
            config: schema.AtmosConfiguration{
                Default:  true,
                BasePath: "/custom/path",
            },
            createLocalCfg: false,
            expectChange:   false,
            expectedPath:   "/custom/path",
        },
        {
            name: "disabled via environment variable",
            config: schema.AtmosConfiguration{
                Default:  true,
                BasePath: ".",
            },
            env: map[string]string{
                "ATMOS_GIT_ROOT_BASEPATH": "false",
            },
            createLocalCfg: false,
            expectChange:   false,
            expectedPath:   ".",
        },
        {
            name: "non-default config skipped",
            config: schema.AtmosConfiguration{
                Default:  false,
                BasePath: ".",
            },
            createLocalCfg: false,
            expectChange:   false,
            expectedPath:   ".",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create temporary directory for test
            tmpDir := t.TempDir()
            t.Chdir(tmpDir)

            // Create local atmos.yaml if test requires it
            if tt.createLocalCfg {
                err := os.WriteFile("atmos.yaml", []byte("base_path: .\n"), 0o644)
                require.NoError(t, err, "Failed to create local atmos.yaml")
            }

            // Create local directory if test requires it
            if tt.createLocalDir != "" {
                err := os.Mkdir(tt.createLocalDir, 0o755)
                require.NoError(t, err, "Failed to create local directory")
            }

            // Set environment variables
            for k, v := range tt.env {
                t.Setenv(k, v)
            }

            originalPath := tt.config.BasePath
            err := applyGitRootBasePath(&tt.config)
            assert.NoError(t, err)

            if tt.expectChange {
                assert.NotEqual(t, originalPath, tt.config.BasePath,
                    "Expected base path to change from default")
            } else {
                assert.Equal(t, tt.expectedPath, tt.config.BasePath,
                    "Base path should not change")
            }
        })
    }
}

// TestLocalConfigPrecedence verifies that local atmos.yaml always takes precedence.
func TestLocalConfigPrecedence(t *testing.T) {
    // Simulate being in a subdirectory with local config
    tmpDir := t.TempDir()
    t.Chdir(tmpDir)

    // Create local atmos.yaml
    localConfig := `base_path: "."
components:
  terraform:
    base_path: "./local-components"
`
    err := os.WriteFile("atmos.yaml", []byte(localConfig), 0o644)
    require.NoError(t, err)

    // Load config
    atmosConfig, err := cfg.LoadConfig(&schema.ConfigAndStacksInfo{})
    require.NoError(t, err)

    // Verify local config was used (not git root)
    assert.Equal(t, ".", atmosConfig.BasePath,
        "Local config should be used instead of git root")
    assert.Equal(t, "./local-components", atmosConfig.Components.Terraform.BasePath,
        "Local config settings should be preserved")
}
```

### Migration Path

**For existing users:**

1. **No action required** - Existing `atmos.yaml` files with `base_path` continue to work
2. **Current directory users** - Add `base_path: "."` to `atmos.yaml` to preserve behavior
3. **Git root users** - Can remove `base_path: !repo-root` from config (now default)

**For test suites:**

1. **Integration tests** - Set `ATMOS_GIT_ROOT_BASEPATH=false` in test environment
2. **Unit tests** - Add `TestMain` to set environment variable
3. **CI/CD** - No changes needed if tests already isolated

**For new users:**

1. **Zero configuration** - Run Atmos from anywhere, it just works
2. **No `--chdir` needed** - Commands work from any subdirectory
3. **Git-like experience** - Matches user mental model

## Risks and Mitigations

### Risk 1: Breaking Existing Workflows

**Probability:** Medium
**Impact:** High

**Scenario:** Users running Atmos from subdirectories expect current directory as base.

**Mitigation:**
- Only applies when using default embedded config
- Users with `atmos.yaml` see no change
- Clear documentation on migration
- Environment variable escape hatch

### Risk 2: Test Suite Failures

**Probability:** High
**Impact:** Medium

**Scenario:** Tests fail because base path resolution changes.

**Mitigation:**
- Environment variable to disable: `ATMOS_GIT_ROOT_BASEPATH=false`
- Set in test infrastructure by default
- Gradual rollout - fix tests before enabling feature
- Comprehensive test coverage for new behavior

### Risk 3: Performance Degradation

**Probability:** Low
**Impact:** Low

**Scenario:** Git root detection adds latency to every command.

**Mitigation:**
- Git root detection is already fast (<1ms)
- Only runs once per command invocation
- Uses go-git library (no shell exec)
- Cached in `atmosConfig.BasePath` for command duration

### Risk 4: Windows Compatibility

**Probability:** Low
**Impact:** Medium

**Scenario:** Path resolution behaves differently on Windows.

**Mitigation:**
- go-git is cross-platform
- `filepath.Join()` handles platform separators
- Existing `!repo-root` infrastructure already tested on Windows
- CI tests on Windows platform

### Risk 5: Non-Git Repositories

**Probability:** Low
**Impact:** Low

**Scenario:** Users without git repositories see errors or unexpected behavior.

**Mitigation:**
- `ProcessTagGitRoot()` has graceful fallback to "."
- No errors if `.git` not found
- Seamless degradation to current directory
- Clear logging at DEBUG level

## Success Metrics

**User Experience:**
- ✅ Users can run Atmos from any subdirectory without `--chdir`
- ✅ Zero configuration required for new users
- ✅ Existing workflows continue to work

**Testing:**
- ✅ All existing tests pass with feature enabled
- ✅ New tests cover git root base path behavior
- ✅ Test isolation prevents interference

**Performance:**
- ✅ No measurable increase in command latency (<10ms)
- ✅ Git root detection completes in <5ms

**Adoption:**
- ✅ Documentation explains behavior clearly
- ✅ Migration guide for existing users
- ✅ Blog post announcing feature

## Open Questions

1. **Should we add a config option to disable instead of env var?**
   ```yaml
   settings:
     git_root_basepath: false  # Explicit config option
   ```
   **Answer:** No - chicken-and-egg problem. Need to load config to read setting, but setting affects config loading.

2. **Should we warn users when falling back to current directory?**
   ```
   WARN: Not in git repository, using current directory as base path
   ```
   **Answer:** No - this is expected behavior for non-git users. Use DEBUG log instead.

3. **Should we support `.git` file (git worktrees)?**
   **Answer:** Yes - go-git library already handles this. No code changes needed.

4. **Should git root discovery respect `.gitignore` or git submodules?**
   **Answer:** No - just find repository root. Submodules have their own `.git` directories.

5. **Should we cache git root across commands (daemon mode)?**
   **Answer:** Out of scope - current architecture doesn't support daemon mode.

## Alternatives Considered

### Do Nothing
Users continue using `--chdir` flag or running from repository root.

**Rejected because:**
- Doesn't match user mental model (Git works from anywhere)
- Adds friction to developer workflow
- Common user complaint

### Document `base_path: !repo-root` as Best Practice
Tell users to add this to their `atmos.yaml`.

**Rejected because:**
- Requires configuration for desired default behavior
- Many users won't discover this
- Not zero-configuration

### Add `--auto-chdir` Flag
Auto-detect git root only when flag provided.

**Rejected because:**
- Requires flag for desired default behavior
- Doesn't match Git UX (no `git --auto-root`)
- More flags = more complexity

### Use `.atmos` Marker File Instead of `.git`
Search for `.atmos` file to determine repository root.

**Rejected because:**
- Adds new concept (marker file)
- Users already have `.git` directory
- Extra maintenance burden

## Timeline

**Phase 1: Implementation (1 week)**
- Implement `applyGitRootBasePath()` function
- Add environment variable control
- Update test infrastructure

**Phase 2: Testing (1 week)**
- Add unit tests for new function
- Fix any test failures
- Test on all platforms (Linux, macOS, Windows)

**Phase 3: Documentation (3 days)**
- Update CLI configuration docs
- Add migration guide
- Write blog post announcement

**Phase 4: Release (1 day)**
- Create PR
- Code review
- Merge and release

**Total: ~2.5 weeks**

## References

- **Issue:** Git root discovery as default behavior
- **Previous attempts:**
  - Commit 3088597a: `base_path: !repo-root` in embedded config (reverted)
  - Commit 5c576c38: `ATMOS_GIT_ROOT_ENABLED` environment variable (reverted)
  - Commit bd2ef1bd: Cleanup of git root detection
- **Related features:**
  - `!repo-root` YAML function implementation: `pkg/utils/git.go:15-67`
  - YAML function processing: `pkg/config/process_yaml.go:191-207`
  - Config loading: `pkg/config/load.go`
- **Test infrastructure:**
  - `TEST_GIT_ROOT`: `pkg/utils/git.go:24`
  - `TEST_EXCLUDE_ATMOS_D`: `pkg/config/load.go:467`
  - CLI test setup: `tests/cli_test.go:795-824`
