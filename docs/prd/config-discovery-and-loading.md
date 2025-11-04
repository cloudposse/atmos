# PRD: Atmos Configuration Discovery and Loading

## Overview

This document describes how Atmos discovers and loads its CLI configuration file (`atmos.yaml`), including the current behavior, limitations, and the proposed fix to enable git repository root as the default base path.

## Current Behavior

### Configuration Search Paths

Atmos searches for `atmos.yaml` in the following locations, in order of precedence (last found wins due to Viper merging):

1. **Embedded configuration** - Default configuration embedded in the Atmos binary (`pkg/config/atmos.yaml`)
2. **System directory**
   - Linux/macOS: `/usr/local/etc/atmos`
   - Windows: `%LOCALAPPDATA%/atmos`
3. **Home directory** - `~/.atmos/atmos.yaml`
4. **Current working directory** - `./atmos.yaml`
5. **Environment variables** - `ATMOS_*` variables override config values
6. **Command-line arguments** - Flags override all previous sources

### Base Path Resolution

The `base_path` setting in `atmos.yaml` determines where Atmos looks for components and stacks:

- **Current behavior**: If `base_path` is not set, it defaults to empty string
- **Empty base_path**: Resolves to current working directory via `filepath.Abs()`
- **Result**: All paths are relative to wherever the user runs the command

### Current Limitations

**Issue: No Git Root as Default Base Path**

When users run Atmos from a subdirectory within a repository (e.g., `components/terraform/vpc/`), all paths are resolved relative to that subdirectory, not the repository root. This causes "atmos.yaml not found" errors.

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
# Paths resolve relative to components/terraform/vpc/

# Required workaround
$ atmos --chdir /path/to/repo terraform plan vpc --stack dev
```

**Why This Happens:**

The embedded `pkg/config/atmos.yaml` does not set a default `base_path`, so it remains empty and defaults to the current working directory.

### Why Git Root Discovery Wasn't Used by Default

Historical context from codebase investigation:

1. **YAML function support exists** - `!repo-root` YAML function already implemented in `pkg/config/process_yaml.go`
2. **Template processing works** - All `atmos.yaml` files are processed through `preprocessAtmosYamlFunc()`
3. **Simply never set** - The embedded config never used `base_path: "!repo-root"`
4. **Workaround added** - `--chdir` flag was added (PR #970325e1e, October 2025) instead of fixing the default

## Proposed Solution

### Set Default Base Path to Git Root

**The Fix:** Add `base_path: "!repo-root"` to the embedded `pkg/config/atmos.yaml`.

This makes the git repository root the default base path, matching user expectations and Git-like tool behavior.

### How It Works

1. **Embedded config sets default**: `base_path: "!repo-root"`
2. **YAML function processes tag**: `preprocessAtmosYamlFunc()` resolves `!repo-root` to git root path
3. **All paths relative to git root**: Components, stacks, etc. are now relative to repository root
4. **Works from any subdirectory**: Users can run Atmos commands from anywhere in the repository

### User Override

Users who want different behavior can override in their own `atmos.yaml`:

```yaml
# Use current directory instead of git root
base_path: "."

# Or use a specific absolute path
base_path: "/path/to/infrastructure"

# Or use environment variable
base_path: "${HOME}/my-infra"
```

### Implementation

#### 1. Update Embedded Configuration

**File**: `pkg/config/atmos.yaml`

```yaml
# Default base path - all component and stack paths are relative to this
# Users can override this in their own atmos.yaml
base_path: "!repo-root"

logs:
  file: "/dev/stderr"
  level: Info

settings:
  telemetry:
    enabled: true
    endpoint: "https://us.i.posthog.com"
    token: "phc_7s7MrHWxPR2if1DHHDrKBRgx7SvlaoSM59fIiQueexS"
    logging: false
```

#### 2. No Code Changes Required

The existing YAML function processing already handles `!repo-root`:

- `preprocessAtmosYamlFunc()` in `pkg/config/process_yaml.go` processes all YAML functions
- `u.ProcessTagGitRoot()` in `pkg/utils/git.go` resolves `!repo-root` to git root path
- Both already work correctly - we just need to use them in the default config

#### 3. Update Tests

Update `tests/cli_workdir_git_root_test.go` to verify the fix:

```go
func TestWorkdirGitRootDetection(t *testing.T) {
    // Test default behavior - should work from subdirectory now.
    t.Run("terraform plan from component directory", func(t *testing.T) {
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        // Should now SUCCEED - base_path defaults to git root.
        assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
    })

    // Test that users can override to current directory.
    t.Run("user overrides base_path to current directory", func(t *testing.T) {
        // This test would require a custom atmos.yaml with base_path: "."
        // to verify override behavior
    })
}
```

#### 4. Update Documentation

**File**: `website/docs/cli/configuration.mdx`

```markdown
## Base Path

The `base_path` setting determines where Atmos looks for components, stacks, and other resources.

### Default Behavior

By default, Atmos sets `base_path` to the git repository root, allowing you to run commands
from any subdirectory within your repository:

```yaml
# Default (set in embedded atmos.yaml)
base_path: "!repo-root"
```

This means you can run Atmos commands from anywhere:

```bash
# Repository structure
/path/to/repo/
├── atmos.yaml
├── components/
│   └── terraform/
│       └── vpc/
└── stacks/

# All of these work identically:
cd /path/to/repo
atmos terraform plan vpc --stack dev

cd /path/to/repo/components/terraform/vpc
atmos terraform plan vpc --stack dev  # Works! Finds config at repo root

cd /path/to/repo/stacks
atmos terraform plan vpc --stack dev  # Works!
```

### Overriding Base Path

You can override the default in your `atmos.yaml`:

```yaml
# Use current directory instead
base_path: "."

# Use absolute path
base_path: "/infrastructure"

# Use environment variable
base_path: "${INFRA_PATH}"

# Use home directory
base_path: "~/.infrastructure"
```

### How It Works

The `!repo-root` YAML function finds the git repository root by walking up the directory tree
from your current location until it finds a `.git` directory. This matches Git's own behavior.

**Note**: If you're not in a git repository, `!repo-root` falls back to the current directory.
```

### Backward Compatibility

**Breaking Change (Justified):**

- **Before**: Paths resolved relative to current working directory (empty `base_path`)
- **After**: Paths resolve relative to git repository root (`base_path: "!repo-root"`)

**Why this is acceptable:**

1. **Matches user expectations** - Git-like behavior is what users expect
2. **Fixes common pain point** - Eliminates need for `--chdir` workarounds
3. **Easy to revert** - Users can set `base_path: "."` to get old behavior
4. **Documented intent** - This was always the intended behavior, just never implemented

**Migration Path:**

For users who relied on current directory behavior (unlikely):

```yaml
# Add to your atmos.yaml to restore old behavior
base_path: "."
```

### Alternative Approaches Considered

#### ❌ Option 1: Environment Variable `ATMOS_CONFIG_GIT_ROOT_DISCOVERY`

**Considered:** Add environment variable to control git root discovery for config files.

**Rejected reason:**
- Unnecessary complexity - config file discovery is separate from base path
- The real issue is that `base_path` should default to git root
- YAML functions already exist for this purpose

#### ❌ Option 2: Environment Variable `ATMOS_DISCOVERY_ACROSS_FILESYSTEM`

**Considered:** Control whether Atmos searches system/home directories.

**Rejected reason:**
- Solves wrong problem - issue is base path, not config discovery
- Config discovery already works correctly
- Adding environment variable is more complex than using existing YAML functions

#### ❌ Option 3: Add Git Root to Config Search Path

**Considered:** Modify config loading to also search at `<git-root>/atmos.yaml`.

**Rejected reason:**
- Config discovery already works - it finds `atmos.yaml` in current directory
- Real problem is that `base_path` is empty, not that config isn't found
- After finding config, paths resolve incorrectly from subdirectories

#### ✅ Option 4: Set `base_path: "!repo-root"` in Embedded Config (SELECTED)

**Selected reason:**
- **Simplest solution** - One line change in embedded config
- **Uses existing infrastructure** - YAML function processing already exists
- **Matches original intent** - This is what should have been done from the start
- **Easily overridable** - Users can change `base_path` in their own config
- **No new code** - No environment variables, no discovery logic changes

## Testing Strategy

### Unit Tests

No new unit tests needed - existing YAML function processing is already tested.

### Integration Tests

Update `tests/cli_workdir_git_root_test.go`:

```go
func TestWorkdirGitRootDetection(t *testing.T) {
    // Test that commands work from subdirectories.
    t.Run("terraform commands from component directory", func(t *testing.T) {
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        // Should SUCCEED - base_path defaults to git root.
        assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
    })

    t.Run("helmfile commands from component directory", func(t *testing.T) {
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("helmfile", "generate", "varfile", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
    })

    t.Run("packer commands from component directory", func(t *testing.T) {
        t.Setenv("TEST_GIT_ROOT", absFixturesDir)
        t.Chdir(componentDir)

        cmd := atmosRunner.Command("packer", "version", "mycomponent", "--stack", "nonprod")
        stdout, stderr, err := cmd.Run()

        assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
    })
}
```

### Manual Testing

```bash
# Test 1: Default behavior (should work from subdirectory)
cd /path/to/repo/components/terraform/vpc
atmos terraform plan vpc --stack dev
# Expected: Success - base_path resolves to repo root

# Test 2: Override to current directory
# Create atmos.yaml with: base_path: "."
atmos terraform plan vpc --stack dev
# Expected: Behavior changes based on override

# Test 3: --chdir still works (highest precedence)
cd /anywhere
atmos --chdir /path/to/repo terraform plan vpc --stack dev
# Expected: Success - explicit chdir overrides base_path

# Test 4: Outside git repository
cd /tmp
mkdir test-no-git && cd test-no-git
echo 'base_path: "!repo-root"' > atmos.yaml
atmos version
# Expected: Falls back to current directory (no error)
```

## Success Criteria

1. ✅ Users can run Atmos from any subdirectory without `--chdir`
2. ✅ Git repository root becomes default base path
3. ✅ Users can override with `base_path: "."` if needed
4. ✅ All existing tests pass
5. ✅ New tests verify subdirectory behavior
6. ✅ `--chdir` flag continues to work and takes precedence
7. ✅ Documentation updated with examples
8. ✅ Works in non-git directories (falls back gracefully)

## Future Considerations

### Optional: `ATMOS_DISCOVERY_ACROSS_FILESYSTEM`

If users request the ability to restrict config discovery to only current directory (skipping system/home directories):

```bash
# Restrict to current directory only
export ATMOS_DISCOVERY_ACROSS_FILESYSTEM=false
```

This would be a separate feature for security/performance optimization in restricted environments.

**Not implementing now** because:
- No user requests for this feature
- Current config discovery works fine
- Base path fix solves the reported problem
- Can be added later if needed

## References

- YAML Function Processing: `pkg/config/process_yaml.go`
- Git Root Resolution: `pkg/utils/git.go:ProcessTagGitRoot()`
- Base Path Configuration: `pkg/config/atmos.yaml` (embedded config)
- Issue #1746: Unified flag parsing overhaul
- PR #970325e1e: Added `--chdir` flag as workaround (October 2025)

## Implementation Checklist

- [ ] Add `base_path: "!repo-root"` to `pkg/config/atmos.yaml`
- [ ] Update integration tests in `tests/cli_workdir_git_root_test.go`
- [ ] Update documentation in `website/docs/cli/configuration.mdx`
- [ ] Add examples to `website/docs/core-concepts/projects/configuration.mdx`
- [ ] Run full test suite: `make testacc`
- [ ] Verify no linting errors: `make lint`
- [ ] Manual testing with real repository (all 4 test scenarios)
- [ ] Update CHANGELOG.md with breaking change notice
- [ ] Verify existing `--chdir` tests still pass
