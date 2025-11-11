# PRD: Atmos Version Command

## Overview

The `atmos version` command is a diagnostic tool that displays version information for the Atmos CLI and optionally checks for updates. Unlike other commands, version is designed to **always work**, even when the Atmos configuration is invalid or missing.

## Problem Statement

Users need a reliable way to:
1. **Identify which version of Atmos they are running** - Essential for troubleshooting, bug reports, and compatibility verification
2. **Check for available updates** - Stay informed about new releases without leaving the terminal
3. **Diagnose installation issues** - Verify Atmos is installed correctly and executable
4. **Get system information** - Understand the platform (OS/architecture) Atmos is running on

The version command must be **resilient** - it serves as the first diagnostic step when troubleshooting, so it cannot depend on valid configuration files.

## Current Implementation

### Command Structure

The version command follows Atmos's command registry pattern:

```
cmd/version/version.go       - Command definition, flag parsing, provider registration
internal/exec/version.go     - Business logic, version checking, output formatting
pkg/version/version.go       - Version constant (set via ldflags during build)
```

### Command Registry Integration

Implements `CommandProvider` interface for self-registration:

```go
type VersionCommandProvider struct{}

func (v *VersionCommandProvider) GetCommand() *cobra.Command
func (v *VersionCommandProvider) GetName() string
func (v *VersionCommandProvider) GetGroup() string
func (v *VersionCommandProvider) GetFlagsBuilder() flags.Builder
func (v *VersionCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder
func (v *VersionCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag
```

Registered via blank import in `cmd/root.go`:
```go
_ "github.com/cloudposse/atmos/cmd/version"
```

### Dependency Injection Pattern

`versionExec` struct uses functional injection for testability:

```go
type versionExec struct {
    atmosConfig                               *schema.AtmosConfiguration
    printStyledText                           func(string) error
    getLatestGitHubRepoRelease                func() (string, error)
    printMessage                              func(string)
    printMessageToUpgradeToAtmosLatestRelease func(string)
    loadCacheConfig                           func() (cfg.CacheConfig, error)
    shouldCheckForUpdates                     func(lastChecked int64, frequency string) bool
}
```

All dependencies are injectable via `NewVersionExec()`, enabling unit tests with mocks.

### Flags and Options

#### `--check` / `-c`
Runs additional version checks after displaying version info.

**Behavior:**
- Always queries GitHub API for latest release (bypasses cache)
- Compares current version to latest release
- Displays upgrade message if newer version available
- Shows "You are running the latest version" if up to date

**Environment Variable:** `ATMOS_VERSION_CHECK`

#### `--format`
Specifies output format for machine-readable output.

**Valid Values:**
- `json` - JSON format with version, OS, arch, and optional `update_version`
- `yaml` - YAML format with same fields

**Environment Variable:** `ATMOS_VERSION_FORMAT`

**Example JSON output:**
```json
{
  "version": "1.96.0",
  "os": "darwin",
  "arch": "arm64",
  "update_version": "1.97.0"
}
```

### Invocation Methods

#### `atmos version`
- Subcommand invocation
- Prints styled ASCII art "ATMOS" logo
- Shows version with alien icon: `👽 Atmos 1.96.0 on darwin/arm64`
- Checks for updates based on cache frequency (respects `atmos.yaml` settings)

#### `atmos --version`
- Global flag invocation (Cobra built-in)
- Same behavior as `atmos version` subcommand
- Processed by same RunE handler

### Configuration-Independent Execution

The version command has special handling in `cmd/root.go` to bypass configuration errors:

#### In `PersistentPreRunE` (lines 207-209):
```go
} else if isVersionCommand() {
    // Version command should always work, even with invalid config
    log.Debug("Warning: CLI configuration error (continuing for version command)", "error", err)
} else {
    errUtils.CheckErrorPrintAndExit(err, "", "")
}
```

#### In `Execute()` function (lines 568-581):
```go
if initErr != nil {
    if isVersionCommand() {
        // Version command should always work, even with invalid config
        log.Debug("Warning: CLI configuration error (continuing for version command)", "error", initErr)
    } else if !errors.Is(initErr, cfg.NotFound) {
        return initErr
    }
}

// Initialize markdown renderers AFTER error check
utils.InitializeMarkdown(atmosConfig)
errUtils.InitializeMarkdown(atmosConfig)
```

**Key Design Decision:** `InitializeMarkdown()` must be called **after** the version command check, because it calls `CheckErrorPrintAndExit()` on invalid configs (a deep exit that would prevent version from running).

### Version Check Cache

Atmos caches version check results to avoid excessive GitHub API calls.

**Cache File:** `~/.atmos/cache.yaml`

**Cache Structure:**
```yaml
last_checked: 1699564800  # Unix timestamp
```

**Configuration (`atmos.yaml`):**
```yaml
version:
  check:
    enabled: true         # Enable/disable automatic checks
    frequency: "1d"       # Check frequency (1d, 12h, etc.)
```

**Check Logic:**
1. If `--check` flag used, always check (bypass cache)
2. If `version.check.enabled: false`, skip check
3. If cache file missing or frequency interval elapsed, perform check
4. Compare `latestVersion` vs `currentVersion` (both stripped of "v" prefix)
5. Display upgrade message if newer version available

### Update Notification

When a newer version is available:

```
⚡ A new version of Atmos is available: 1.97.0
   Your current version: 1.96.0

   To upgrade, visit: https://atmos.tools/install
```

### Output Channels

- **Default output (styled):** stderr (UI channel)
- **Machine-readable formats (`--format`):** stdout (data channel)

This separation allows:
```bash
atmos version                   # Human-readable to terminal
atmos version --format json     # Machine-readable to pipe
atmos version --format json | jq .version
```

## Error Handling

The version command uses **proper error returns** instead of deep exits:

```go
func (v versionExec) Execute(checkFlag bool, format string) error {
    if format != "" {
        return v.displayVersionInFormat(checkFlag, format)
    }

    err := v.printStyledText("ATMOS")
    if err != nil {
        return fmt.Errorf("failed to display styled text: %w", err)  // ✅ Returns error
    }
    // ...
}
```

**Errors are returned to `RunE`, which returns to Cobra, which handles exit codes properly.**

### Resilience to Configuration Errors

The version command tolerates:
- **Invalid YAML syntax** - Unclosed brackets, malformed indentation
- **Invalid command aliases** - Non-existent commands, circular references
- **Invalid config schema** - Wrong types (number instead of string), invalid nested structures
- **Missing config files** - No `atmos.yaml` found

All configuration errors are logged at debug level but **do not prevent version from executing**.

## Testing Strategy

### Unit Tests

Located in `internal/exec/version_test.go`:
- Test version display with mocked dependencies
- Test update checking with mocked GitHub API responses
- Test cache loading and frequency checks
- Test format output (JSON/YAML)

### Integration Tests

Located in `tests/test-cases/version-invalid-config.yaml`:

**10 comprehensive test cases:**

1. `atmos version` with invalid YAML syntax
2. `atmos version` with invalid command aliases
3. `atmos version` with invalid config schema
4. `atmos version --check` with invalid YAML syntax
5. `atmos version --format json` with invalid config
6. `atmos version --format yaml` with invalid config
7. `atmos --version` with invalid YAML syntax
8. `atmos --version` with invalid command aliases
9. `atmos --version` with invalid config schema

**Test fixtures:**
- `fixtures/scenarios/invalid-config-yaml/` - Malformed YAML
- `fixtures/scenarios/invalid-config-aliases/` - Invalid aliases
- `fixtures/scenarios/invalid-config-schema/` - Schema violations

**All tests verify:**
- Exit code 0 (success)
- Stdout matches expected pattern: `👽 Atmos (\d+\.\d+\.\d+|test) on [a-z]+/[a-z0-9]+`
- Stderr can contain warnings (ignored)

## Success Metrics

1. **Reliability:** Version command succeeds 100% of time, regardless of config state
2. **Testability:** 100% coverage of version command logic with unit tests
3. **Observability:** All config errors logged at debug level (visible with `ATMOS_LOGS_LEVEL=Debug`)
4. **Composability:** JSON/YAML output enables scripting and automation
5. **User Experience:** Clear, actionable upgrade messages when updates available

## References

- **Command Registry Pattern:** `docs/prd/command-registry-pattern.md`
- **Avoiding Deep Exits Pattern:** `docs/prd/avoiding-deep-exits-pattern.md`
- **Version Package:** `pkg/version/version.go`
- **Version Command:** `cmd/version/version.go`
- **Version Execution:** `internal/exec/version.go`
- **Integration Tests:** `tests/test-cases/version-invalid-config.yaml`

## Design Rationale

### Why Version Must Always Work

The version command is often the **first command users run** when:
- Installing Atmos for the first time
- Troubleshooting configuration issues
- Filing bug reports
- Verifying compatibility with documentation

If version fails due to configuration errors, users have no reliable diagnostic tool. This creates a bootstrapping problem: they can't fix their config because they can't determine what version they're running or access help resources.

### Why Version Checks Use Cache

GitHub API has rate limits (60 requests/hour unauthenticated). Without caching, users running `atmos version` repeatedly (common during development) would quickly exhaust the limit, causing version checks to fail.

Cache with configurable frequency (default: 1 day) provides:
- **Reasonable freshness:** Users notified within 24 hours of new releases
- **API efficiency:** Maximum 1 request per user per day
- **Override capability:** `--check` flag bypasses cache for immediate checking

### Why Two Invocation Methods

- **`atmos version`** (subcommand) - Discoverable via `atmos --help`, consistent with command structure
- **`atmos --version`** (flag) - Unix convention, expected by users familiar with other CLIs

Both are processed identically by checking `isVersionCommand()` which detects either `os.Args[1] == "version"` or `os.Args[1] == "--version"`.

## Future Enhancements

### Phase 1: Current Implementation
- ✅ Proper error returns (no deep exits)
- ✅ Configuration-independent execution
- ✅ Comprehensive integration tests
- ✅ Dependency injection for testability

### Phase 2: Rich Error Handling (After PR #1763)
- Replace `fmt.Errorf()` with error builder
- Add hints for common version-related issues
- Provide context for GitHub API failures

### Phase 3: Enhanced Version Information
- Display plugin versions (when plugin system added)
- Show last update check timestamp
- Display installed vs latest version in table format

### Phase 4: Advanced Update Management
- Optional auto-update capability (opt-in)
- Release notes preview for available updates
- Version pinning in `atmos.yaml`
