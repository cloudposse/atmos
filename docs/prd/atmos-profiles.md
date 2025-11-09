# Product Requirements Document: Atmos Profiles

## Overview

This document describes the requirements and implementation for Atmos profiles, which enable users to maintain multiple configuration presets that can be activated via CLI flags or environment variables. Profiles provide environment-specific, role-based, or context-specific configuration overrides without modifying the base `atmos.yaml` configuration.

## Implementation Resources

- **Detailed Implementation Plan**: `.scratch/profiles-implementation-plan.md` - Step-by-step tasks with code examples
- **Refactoring Plan**: `.scratch/profiles-loading-refactor.md` - Shared config directory loading function
- **Architecture Summary**: `.scratch/profiles-architecture-summary.md` - Package structure and modern command pattern

## Dependencies

This PRD depends on:
- **[Auth Default Settings PRD](./auth-default-settings.md)** - Provides `auth.defaults.identity` for deterministic default identity selection in profiles
- **PR #1763** - Error handling infrastructure must be merged before implementation

## Problem Statement

### Current State

Users currently manage Atmos configuration through a single `atmos.yaml` file with limited ability to switch between different configuration contexts:

```yaml
# atmos.yaml - Single configuration for all contexts
logs:
  level: Warning

auth:
  identities:
    developer-identity:
      kind: aws/permission-set
      default: true
      via:
        provider: aws-sso
      principal:
        account_id: "123456789012"
        permission_set: DeveloperAccess
```

### Challenges

1. **Multi-environment workflows** - Developers need different settings for local development vs CI/CD environments
2. **Role-based configuration** - Different team members (developers, DevOps, platform engineers) need different defaults
3. **Debugging complexity** - Temporarily enabling debug/trace logs requires editing configuration files
4. **CI/CD integration** - GitHub Actions, GitLab CI, and other automation need specialized settings (e.g., OIDC authentication)
5. **Configuration fragility** - Switching contexts requires manual file edits or complex environment variable management
6. **Testing scenarios** - QA and testing environments need isolated configuration profiles
7. **Identity selection in CI** - Multiple `identity.default: true` causes errors in non-interactive environments (see [Auth Default Settings PRD](./auth-default-settings.md))

### Use Cases

#### UC1: CI/CD Environment Profile
**Actor**: GitHub Actions workflow
**Scenario**: Automated deployments need specialized configuration
- **Requirements**:
  - Use GitHub OIDC for authentication instead of interactive SSO
  - Disable interactive prompts
  - Increase logging for troubleshooting
  - Set appropriate timeouts for CI environment

```bash
# In GitHub Actions workflow
ATMOS_PROFILE=ci atmos terraform apply component -s prod
```

#### UC2: Role-Based Defaults
**Actor**: Developer vs Platform Engineer
**Scenario**: Different team roles need different default identities and permissions
- **Developer profile**: Limited permissions, sandbox AWS account, verbose output
- **Platform Engineer profile**: Admin permissions, production access, concise output
- **Audit profile**: Read-only access, detailed logging, immutable operations

```bash
# Developer
atmos terraform plan vpc -s dev --profile developer

# Platform Engineer
atmos terraform apply vpc -s prod --profile platform-admin

# Security audit
atmos describe stacks --profile audit
```

#### UC3: Debug/Development Profile
**Actor**: Atmos core developer or user troubleshooting issues
**Scenario**: Temporarily enable comprehensive debugging without editing configuration files

```bash
# Enable trace logging and profiling
atmos terraform plan vpc -s dev --profile debug
```

#### UC4: Testing Profiles
**Actor**: Automated test suite
**Scenario**: Tests need isolated, reproducible configuration

```bash
# Run tests with test-specific configuration
ATMOS_PROFILE=test make test-integration
```

## Goals

- **Easy context switching**: Enable users to switch between configurations with a single flag or environment variable
- **Configuration inheritance**: Profiles should override base configuration, not replace it entirely
- **Lexicographic loading**: Follow the proven `.atmos.d/` pattern for predictable merge behavior
- **Zero breaking changes**: Existing configurations continue working without modification
- **Environment variable support**: Allow `ATMOS_PROFILE` for scripting and CI/CD
- **Multiple profiles**: Support activating multiple profiles simultaneously with precedence rules
- **Self-contained profiles**: Each profile is a complete directory of configuration files

## Non-Goals

- **Profile generation UI**: No interactive profile creation wizard (users edit YAML manually)
- **Profile versioning**: No version control or migration system for profiles
- **Remote profile storage**: Profiles are always local filesystem-based
- **Profile validation**: No schema validation specific to profiles (use existing Atmos config validation)
- **Profile discovery**: No automatic profile detection based on environment (explicit activation only)

## Requirements

### Functional Requirements

#### FR1: Profile Definition and Location

**FR1.1**: Profiles MUST be configured at the top level in `atmos.yaml`:

```yaml
# atmos.yaml
profiles:
  base_path: "./profiles"        # Base directory for profiles (relative or absolute)
  # OR
  base_path: "/shared/profiles"  # Absolute path
```

**FR1.2**: Profile discovery MUST search multiple locations in precedence order:

1. **Configurable profile directory** (highest precedence):
   - `profiles.base_path` in `atmos.yaml` (can be relative or absolute)
   - Example: `profiles.base_path: "./custom-profiles"`
   - If relative, resolved from `atmos.yaml` directory

2. **Project-local hidden profiles**:
   - `{atmos_cli_config_path}/.atmos/profiles/` (hidden directory, project-specific)
   - Example: `/infrastructure/atmos/.atmos/profiles/`
   - Higher precedence than non-hidden `profiles/` directory

3. **XDG user profiles** (follows XDG Base Directory Specification):
   - `$XDG_CONFIG_HOME/atmos/profiles/` (default: `~/.config/atmos/profiles/`)
   - `$ATMOS_XDG_CONFIG_HOME/atmos/profiles/` (Atmos-specific override)
   - Platform-aware: Uses `~/.config` on Linux/macOS, `%APPDATA%` on Windows

4. **Project-local non-hidden profiles** (lowest precedence):
   - `{atmos_cli_config_path}/profiles/` (non-hidden directory)
   - Example: `/infrastructure/atmos/profiles/`
   - Alternative to hidden `.atmos/profiles/` for users who prefer visible directories

**FR1.3**: Profile discovery MUST search all locations and merge profiles with same name (later locations override earlier)

**FR1.4**: Each profile MUST be a directory containing one or more YAML configuration files

**FR1.5**: Profile directory structure MUST mirror `.atmos.d/` structure with support for nested directories

**FR1.6**: Profile configuration files MUST follow the same naming conventions as `atmos.yaml` and `.atmos.d/` files

**FR1.7**: Profiles SHOULD support organizing configuration by domain (e.g., `auth.yaml`, `terraform.yaml`, `logging.yaml`)

**FR1.8**: Profile inheritance MUST be supported via the existing `import:` mechanism in profile YAML files

Example structure:
```
# Project-local profiles
infrastructure/
└── atmos/
    ├── atmos.yaml                    # Base configuration
    │   # Can configure custom profile location:
    │   # profiles:
    │   #   base_path: "./custom-profiles"
    ├── .atmos.d/                     # Default imports (always loaded)
    │   ├── commands.yaml
    │   └── workflows.yaml
    └── .atmos/profiles/              # Project-local profiles (hidden)
        ├── ci/
        │   ├── auth.yaml
        │   └── logging.yaml
        └── developer/
            └── auth.yaml

# User-global profiles (XDG)
~/.config/atmos/profiles/             # User-specific profiles (all projects)
├── debug/                            # Debug profile (reusable across projects)
│   ├── logging.yaml
│   └── profiler.yaml
└── personal-dev/                     # Personal development settings
    └── auth.yaml

# Profile with inheritance via imports
~/.config/atmos/profiles/ci-prod/
└── atmos.yaml
    import:
      - ../ci/auth.yaml               # Inherit from ci profile
      - ../ci/logging.yaml
    # Additional prod-specific overrides
    logs:
      level: Warning                  # Override ci profile's Info level
```

#### FR2: Profile Activation

**FR2.1**: Profiles MUST be activated via `--profile` CLI flag (StringSlice flag)
```bash
# Single profile
atmos terraform plan vpc -s dev --profile developer

# Multiple profiles via comma-separated value
atmos terraform plan vpc -s dev --profile developer,debug

# Multiple profiles via repeated flag
atmos terraform plan vpc -s dev --profile developer --profile debug
```

**FR2.2**: Profiles MUST be activated via `ATMOS_PROFILE` environment variable (comma-separated)
```bash
ATMOS_PROFILE=ci atmos terraform apply vpc -s prod
ATMOS_PROFILE=developer,debug atmos describe stacks
```

**FR2.3**: Multiple profile specification methods:
- Comma-separated: `--profile developer,debug`
- Repeated flag: `--profile developer --profile debug`
- Environment variable: `ATMOS_PROFILE=developer,debug`

**FR2.4**: CLI flag MUST take precedence over environment variable

**FR2.5**: When no profile is specified, Atmos MUST behave identically to current behavior (no profile loading)

**FR2.6**: Profile selection is independent of identity selection
- Profiles can define identities with `default: true` to change which identity is default
- Users can still override with `--identity` flag or `ATMOS_IDENTITY` environment variable
- Identity selection precedence: `--identity` flag > `ATMOS_IDENTITY` env > default identity from active profile > default identity from base config

#### FR3: Configuration Merging and Precedence

**FR3.1**: Configuration loading order MUST be:
1. Embedded defaults (built into Atmos binary)
2. System directory (`/usr/local/etc/atmos` or `%LOCALAPPDATA%\atmos`)
3. Home directory (`~/.atmos/atmos.yaml`)
4. Working directory (`./atmos.yaml`)
5. Environment variables (`ATMOS_*`)
6. CLI config path (`ATMOS_CLI_CONFIG_PATH` or `--config-dir`)
7. `.atmos.d/` directories (lexicographic order)
8. **Active profiles** (left-to-right for multiple profiles, lexicographic within each profile)
9. Local `atmos.yaml` (final override)

**FR3.2**: Profile configuration files within a profile directory MUST be loaded in lexicographic order

**FR3.3**: Later configurations MUST override earlier configurations using deep merge semantics

**FR3.4**: Multiple profiles MUST be applied left-to-right (first profile lowest precedence, last profile highest precedence)

**FR3.5**: Profile configuration MUST merge with base configuration, not replace it entirely

**FR3.6**: Array fields (e.g., `commands`, `workflows`) MUST follow the same merge-by-name behavior as `.atmos.d/` imports

#### FR4: Profile Discovery and Validation

**FR4.1**: Atmos MUST validate that requested profiles exist before processing

**FR4.2**: If a profile does not exist, Atmos MUST return an error with a clear message listing available profiles

**FR4.3**: Empty profile directories MUST be valid (no configuration changes applied)

**FR4.4**: Profile configuration syntax errors MUST be reported with profile name and file path

#### FR5: Profile Management Commands

**FR5.1**: New command `atmos profile list` MUST list all available profiles across all locations

**FR5.2**: `atmos profile list` output format (using lipgloss table):
```
Available Profiles

┏━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━┓
┃ Profile      ┃ Description                                 ┃ Location  ┃
┡━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━┩
│ ci           │ CI/CD environment configuration             │ Project   │
│ developer    │ Developer workstation defaults              │ Project   │
│ debug        │ Debug logging and profiling                 │ User      │
│ personal-dev │ Personal development settings               │ User      │
└──────────────┴─────────────────────────────────────────────┴───────────┘

Tip: View profile details with 'atmos profile show <profile>'
     Use a profile with 'atmos <command> --profile <profile>'
```

**FR5.2.1**: Table styling MUST use lipgloss with:
- Header row with bold styling
- Border styles consistent with other Atmos tables
- Location column showing profile source (Project, User, Custom)
- Optional description extracted from profile metadata or first comment

**FR5.2.2**: Command alias `atmos list profiles` MUST be provided for consistency with other list commands (`atmos list components`, `atmos list stacks`, `atmos list instances`):
- Both `atmos profile list` and `atmos list profiles` MUST produce identical output
- Help text for both commands MUST reference the alias

**FR5.3**: `atmos profile list` MUST support JSON and YAML output formats via `--format` flag

**FR5.4**: New command `atmos profile show <profile>` MUST display merged configuration for a profile

**FR5.5**: `atmos profile show <profile>` output format (using lipgloss styling):
```
Profile: developer

Locations
  • /infrastructure/atmos/.atmos/profiles/developer (2 files)
  • ~/.config/atmos/profiles/developer (1 file)

Files (in merge order)
  1. /infrastructure/atmos/.atmos/profiles/developer/auth.yaml
  2. /infrastructure/atmos/.atmos/profiles/developer/logging.yaml
  3. /infrastructure/atmos/.atmos/profiles/developer/overrides.yaml

Merged Configuration

auth:
  identities:
    developer-sandbox:
      kind: aws/permission-set
      default: true
      via:
        provider: aws-sso-dev
      principal:
        account_id: "999888777666"
        permission_set: DeveloperAccess
  providers:
    aws-sso-dev:
      kind: aws/sso
      region: us-east-2
      start_url: https://dev.awsapps.com/start
logs:
  level: Warning
  file: /dev/stderr
```

**Note**: The "Merged Configuration" section uses `u.GetHighlightedYAML()` for syntax highlighting (keys in blue, values in appropriate colors, proper indentation).

**FR5.6**: `atmos profile show <profile>` MUST use the same pretty YAML formatting as `atmos describe config`
- Use `u.GetHighlightedYAML()` for YAML format (colorized, syntax highlighted)
- Use `u.GetHighlightedJSON()` for JSON format (colorized, syntax highlighted)
- Respects terminal color settings (`--color`, `--no-color`, `NO_COLOR` env var)
- Supports pager integration when enabled (`settings.terminal.pager`)

**FR5.7**: `atmos profile show <profile>` MUST support `--format` flag for output format selection
- `--format yaml` (default) - Colorized YAML output
- `--format json` - Colorized JSON output

**FR5.8**: `atmos profile show <profile>` MUST support `--files` flag to show file list only (no merged config)

**FR5.9**: `atmos profile show <profile>` MUST support `--provenance` flag to show where each configuration value originated

**FR5.10**: `atmos describe config` MUST support `--provenance` flag to show where configuration values originated (including active profiles)

**FR5.11**: Debug logging (`--logs-level trace`) MUST show profile loading details:
- Which profiles are being loaded
- From which locations
- File merge order
- Configuration precedence

#### FR6: Tag-Based Resource Filtering (Phase 2 Enhancement)

**Note**: This feature is scoped as a Phase 2 enhancement and is NOT included in the initial profiles implementation. This section documents the design for future implementation.

**FR6.1**: Profile tags MUST enable automatic filtering of resources when explicitly activated.

**FR6.2**: Tag-based filtering MUST be opt-in (disabled by default) to maintain backward compatibility.

**FR6.3**: Tag filtering activation methods (when implemented):
- **Global flag**: `--filter-by-profile-tags` enables filtering for the current command
- **Configuration**: `profiles.filter_by_tags: true` in `atmos.yaml` enables filtering by default
- **Default behavior**: Disabled (show all resources regardless of tags)

**FR6.4**: Initial implementation scope (Phase 2):
The following commands will implement tag filtering when this feature is developed:
1. `atmos auth list identities` - Filter identities by matching tags
2. `atmos list components` - Filter components by matching tags
3. `atmos describe stacks` - Filter stacks by matching tags

**Note**: These three commands are the initial scope. Additional commands can be enhanced in future iterations.

**FR6.5**: Tag matching logic (OR semantics):
- If active profile has tags `["developer", "local"]`
- Filter matches resources with **any** of those tags (OR logic)
- Example: Identity with `tags: ["developer"]` → **Shown** (matches one tag)
- Example: Identity with `tags: ["production"]` → **Hidden** (no matching tags)
- Example: Identity with `tags: ["developer", "production"]` → **Shown** (matches at least one tag)

**FR6.6**: Multiple active profiles with tag filtering:
- When multiple profiles are active: `--profile developer,ci`
- Profile tags are **unioned** into a single tag set
- Example: If `developer` has tags `["developer", "local"]` and `ci` has tags `["ci", "github-actions"]`
- Combined tag set: `["developer", "local", "ci", "github-actions"]`
- Resources matching **any** tag in the combined set are shown

**FR6.7**: UX hints for filtered output:
When tag filtering is active, commands MUST indicate filtering is enabled:
```bash
# Example output
$ atmos auth list identities --profile developer --filter-by-profile-tags

Showing identities matching profile tags: developer, local
(Use --no-filter-by-profile-tags to show all identities)

Available Identities
┏━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━┓
┃ Name              ┃ Kind             ┃ Tags        ┃
┡━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━┩
│ dev-sandbox       │ aws/permission-… │ developer   │
│ local-testing     │ aws/static       │ local       │
└───────────────────┴──────────────────┴─────────────┘
```

**FR6.8**: Tag filtering configuration example:
```yaml
# profiles/developer/_metadata.yaml
metadata:
  name: developer
  tags: ["developer", "local"]

# When filtering is enabled, only resources with these tags will be shown
```

**FR6.9**: Implementation requirements (Phase 2):
When this feature is implemented, the following tasks must be completed:
1. **Flag implementation**: Add `--filter-by-profile-tags` flag to root command
2. **Configuration support**: Add `profiles.filter_by_tags` to `AtmosConfiguration` schema
3. **Per-command filtering hooks**: Each command listed in FR6.4 must implement filtering logic
4. **UX hints**: Add user-facing messages indicating when filtering is active
5. **Tests**: Unit and integration tests for tag filtering behavior
6. **Documentation**: User-facing docs explaining tag filtering feature

**FR6.10**: Resources without tags:
- Resources without a `tags` field are treated as having an empty tag set `[]`
- When tag filtering is active, resources without tags are **hidden** (no matching tags)
- Exception: If profile has no tags (empty tag set), filtering is effectively disabled for that profile

### Technical Requirements

#### TR1: Configuration Loading Implementation

**TR1.1**: Profile loading logic MUST reuse existing `.atmos.d/` merge logic from `pkg/config/load.go`

**TR1.2**: Profile loading MUST use the same `mergeConfigFile()` function as `.atmos.d/` imports

**TR1.3**: Profile directory discovery MUST use the same glob pattern logic as `.atmos.d/` discovery

**TR1.4**: Profile configuration MUST be loaded into Viper before final configuration unmarshaling

**TR1.5**: Profile path resolution MUST support both absolute and relative paths from the config directory

#### TR2: Schema and Validation

**TR2.1**: Profile configuration files MUST support all fields available in `atmos.yaml`

**TR2.2**: Profile configurations MUST be validated using existing Atmos configuration schema

**TR2.3**: New top-level `Profiles` and `Metadata` configuration MUST be added to `AtmosConfiguration` in `pkg/schema/schema.go`:
```go
type AtmosConfiguration struct {
    // ... existing fields ...
    Profiles ProfilesConfig `yaml:"profiles,omitempty" json:"profiles,omitempty" mapstructure:"profiles"`
    Metadata ConfigMetadata `yaml:"metadata,omitempty" json:"metadata,omitempty" mapstructure:"metadata"`
}

type ProfilesConfig struct {
    BasePath string `yaml:"base_path,omitempty" json:"base_path,omitempty" mapstructure:"base_path"`
}

type ConfigMetadata struct {
    Name        string   `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
    Description string   `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
    Version     string   `yaml:"version,omitempty" json:"version,omitempty" mapstructure:"version"`
    Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty" mapstructure:"tags"`
    Deprecated  bool     `yaml:"deprecated,omitempty" json:"deprecated,omitempty" mapstructure:"deprecated"`
}
```

**TR2.4**: JSON Schema definitions in `pkg/datafetcher/schema/` MUST include top-level `profiles` configuration

**TR2.5**: Configuration errors in profiles MUST include profile name and source location in error messages

**TR2.6**: Profile imports MUST use the same validation and cycle detection as stack imports

**TR2.7**: Metadata auto-population MUST occur after configuration loading:
- Base config (`atmos.yaml`): Auto-populate `metadata.name` to `"default"` if not set
- Profile configs: Auto-populate `metadata.name` to directory basename if not set
- Example: `profiles/ci/` → `metadata.name = "ci"`

**TR2.8**: Metadata merge behavior when profile has multiple files:
- **First non-empty wins** for singular fields (`name`, `description`, `version`, `deprecated`)
- **Union (append + deduplicate)** for array fields (`tags`)
- Best practice: Define metadata in `_metadata.yaml` or first alphabetical file

```go
// Metadata merge logic
func mergeConfigMetadata(existing *ConfigMetadata, incoming *ConfigMetadata) {
    // First non-empty wins
    if existing.Name == "" && incoming.Name != "" {
        existing.Name = incoming.Name
    }
    if existing.Description == "" && incoming.Description != "" {
        existing.Description = incoming.Description
    }
    if existing.Version == "" && incoming.Version != "" {
        existing.Version = incoming.Version
    }
    if !existing.Deprecated && incoming.Deprecated {
        existing.Deprecated = incoming.Deprecated
    }

    // Tags: Union (append and deduplicate)
    if len(incoming.Tags) > 0 {
        existing.Tags = appendUnique(existing.Tags, incoming.Tags)
    }
}
```

**Note**: Tag-based filtering has been moved to a separate Functional Requirement (FR6: Tag-Based Resource Filtering) and scoped as a Phase 2 enhancement. It is not part of the initial profiles implementation.

#### TR3: Performance

**TR3.1**: Profile loading MUST complete within 100ms for typical profiles, where "typical" is defined as:
- 5–10 YAML files per profile
- ≤1,000 lines per file
- ≤500KB total file size across all profile files
- Maximum YAML nesting depth of 6 levels
- No cross-file imports or runtime-evaluated merges (e.g., `!terraform.output`)
- Examples: 6 files, 800 lines each, 350KB total; no complex imports
- Test vectors: Benchmark suite in `tests/benchmarks/profiles/typical/` (profile-small-6files.yaml, profile-medium-10files.yaml) MUST meet 100ms target

**TR3.2**: Profile discovery MUST be cached during a single Atmos command execution

**TR3.3**: Memory usage MUST scale linearly with number of active profiles

**TR3.4**: Profile configuration MUST not impact performance when no profiles are active

#### TR4: Testing

**TR4.1**: Unit tests MUST verify profile precedence rules

**TR4.2**: Integration tests MUST verify profile merging with various configuration combinations

**TR4.3**: Tests MUST verify multiple profile activation and precedence

**TR4.4**: Tests MUST verify profile error handling (missing profiles, invalid syntax)

**TR4.5**: Tests MUST verify backward compatibility (no profiles specified)

**TR4.6**: Tests MUST verify XDG profile location discovery

**TR4.7**: Tests MUST cover profile commands (`atmos profile list`, `atmos profile show`)

**TR4.8**: Tests MUST verify provenance tracking for profile configurations

#### TR5: Provenance Support

**TR5.1**: Profile loading MUST enable provenance tracking using existing `pkg/merge` infrastructure

**TR5.2**: `atmos profile show --provenance` MUST use `p.RenderInlineProvenance()` for inline annotations

**TR5.3**: `atmos describe config --provenance` MUST use `p.RenderInlineProvenance()` for inline annotations

**TR5.4**: Provenance tracking for profiles MUST use the existing `MergeContext` pattern from stack/component merging

**TR5.5**: Profile merge operations MUST call `mergeContext.RecordProvenance()` for each configuration value

**TR5.6**: Profile source paths MUST be recorded with format: `profiles/<profile-name>/<filename>:<line>`

**TR5.7**: Provenance rendering MUST distinguish between:
- Base configuration: `atmos.yaml:<line>`
- `.atmos.d/` files: `.atmos.d/<filename>:<line>`
- Profile files: `profiles/<profile-name>/<filename>:<line>`
- XDG profile files: `profiles/<profile-name>/<filename>:<line> (XDG)`

**TR5.8**: When multiple profiles override the same value, provenance MUST show the winning profile with `(override)` annotation

**TR5.9**: `atmos describe config --provenance` MUST set `atmosConfig.TrackProvenance = true` before loading configuration

**TR5.10**: Profile loading in `pkg/config/load.go` MUST accept optional `mergeContext *m.MergeContext` parameter

**TR5.11**: Provenance implementation MUST reuse existing functions:
- `pkg/merge.NewMergeContext()` - Create merge context
- `pkg/merge.(*MergeContext).RecordProvenance()` - Record value source
- `pkg/provenance.RenderInlineProvenance()` - Render inline annotations
- `pkg/utils.GetHighlightedYAML()` - Syntax highlighting with provenance

**TR5.12**: `cmd/describe_config.go` MUST add `--provenance` flag similar to `cmd/describe_component.go`:
```go
describeConfigCmd.PersistentFlags().Bool("provenance", false, "Enable provenance tracking to show where configuration values originated")
```

**TR5.13**: `internal/exec/describe_config.go` MUST accept `Provenance bool` parameter in execution params

**TR5.14**: When provenance is enabled and configuration has merge context, MUST call `renderProvenance()` similar to describe component implementation

#### TR6: Documentation

**TR6.1**: Documentation MUST include profile configuration examples for common scenarios (CI, developer, debug)

**TR6.2**: Documentation MUST explain precedence rules with visual diagrams

**TR6.3**: Migration guide MUST be provided for users converting environment-specific configurations to profiles

**TR6.4**: Documentation MUST include provenance examples showing how to debug profile configuration sources

**TR6.5**: Blog post MUST be included announcing this minor feature (per CLAUDE.md requirement)

## Design

### Configuration Loading Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    Atmos Configuration Loading                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 1. Load Base Configuration Chain                                │
│    • Embedded defaults                                           │
│    • System directory (/usr/local/etc/atmos)                     │
│    • Home directory (~/.atmos/atmos.yaml)                        │
│    • Working directory (./atmos.yaml)                            │
│    • Environment variables (ATMOS_*)                             │
│    • CLI config path (--config-dir)                              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. Load .atmos.d/ Configurations (lexicographic)                 │
│    • atmos.d/commands.yaml                                       │
│    • atmos.d/workflows.yaml                                      │
│    • .atmos.d/auth.yaml                                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. Load Active Profiles (if specified)                           │
│    ┌───────────────────────────────────────────────────────────┐│
│    │ For each profile in --profile or $ATMOS_PROFILE:          ││
│    │   1. Validate profile directory exists                    ││
│    │   2. Discover all YAML files in profile directory         ││
│    │   3. Sort files lexicographically                         ││
│    │   4. Merge each file into configuration                   ││
│    └───────────────────────────────────────────────────────────┘│
│                                                                  │
│    Example: --profile developer,debug                           │
│    • profiles/developer/auth.yaml                               │
│    • profiles/developer/logging.yaml                            │
│    • profiles/debug/logging.yaml      (overrides developer)     │
│    • profiles/debug/profiler.yaml                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. Final Local Configuration Override                            │
│    • atmos.yaml (local overrides everything)                     │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. Unmarshal Final Configuration                                 │
│    • Return merged AtmosConfiguration                            │
└─────────────────────────────────────────────────────────────────┘
```

### Profile Configuration Examples

#### Configuring Profile Location

```yaml
# atmos.yaml - Configure custom profile location
profiles:
  base_path: "./custom-profiles"  # Relative to atmos.yaml directory
  # OR
  base_path: "/shared/company-profiles"  # Absolute path

# This adds to the profile search locations
# (XDG and other locations are still searched)
```

**Profile location precedence:**

1. **Configurable profile directory** (highest precedence):
   - `profiles.base_path` in `atmos.yaml` (can be relative or absolute)
   - Example: `profiles.base_path: "./custom-profiles"`
   - If relative, resolved from `atmos.yaml` directory

2. **Project-local hidden profiles**:
   - `{atmos_cli_config_path}/.atmos/profiles/` (hidden directory, project-specific)
   - Example: `/infrastructure/atmos/.atmos/profiles/`
   - Higher precedence than non-hidden `profiles/` directory

3. **XDG user profiles** (follows XDG Base Directory Specification):
   - `$XDG_CONFIG_HOME/atmos/profiles/` (default: `~/.config/atmos/profiles/`)
   - `$ATMOS_XDG_CONFIG_HOME/atmos/profiles/` (Atmos-specific override)
   - Platform-aware: Uses `~/.config` on Linux/macOS, `%APPDATA%` on Windows

4. **Project-local non-hidden profiles** (lowest precedence):
   - `{atmos_cli_config_path}/profiles/` (non-hidden directory)
   - Example: `/infrastructure/atmos/profiles/`
   - Alternative to hidden `.atmos/profiles/` for users who prefer visible directories

**Note:** Profile configuration is meta - if a profile sets `profiles.base_path`, it affects subsequent profile loading. This is intentional to allow profiles to configure the system.

#### Listing Available Profiles

```bash
atmos profile list
```

Output:
```
Available Profiles

┏━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━┓
┃ Profile      ┃ Description                                 ┃ Location  ┃
┡━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━┩
│ ci           │ CI/CD environment configuration             │ Project   │
│ developer    │ Developer workstation defaults              │ Project   │
│ debug        │ Debug logging and profiling                 │ User      │
│ personal-dev │ Personal development settings               │ User      │
└──────────────┴─────────────────────────────────────────────┴───────────┘

Tip: View profile details with 'atmos profile show <profile>'
     Use a profile with 'atmos <command> --profile <profile>'
```

JSON output format:
```bash
atmos profile list --format json
```

Output:
```json
{
  "profiles": [
    {
      "name": "ci",
      "description": "CI/CD environment configuration",
      "location": "project",
      "path": "/infrastructure/atmos/.atmos/profiles/ci",
      "files": ["auth.yaml", "logging.yaml", "terminal.yaml"]
    },
    {
      "name": "developer",
      "description": "Developer workstation defaults",
      "location": "project",
      "path": "/infrastructure/atmos/.atmos/profiles/developer",
      "files": ["auth.yaml", "logging.yaml"]
    }
  ]
}
```

#### Viewing Profile Details

```bash
atmos profile show developer
```

Output:
```
Profile: developer

Locations
  • /infrastructure/atmos/.atmos/profiles/developer (2 files)
  • ~/.config/atmos/profiles/developer (1 file)

Files (in merge order)
  1. /infrastructure/atmos/.atmos/profiles/developer/auth.yaml
  2. /infrastructure/atmos/.atmos/profiles/developer/logging.yaml
  3. ~/.config/atmos/profiles/developer/overrides.yaml

Merged Configuration

auth:
  identities:
    developer-sandbox:
      kind: aws/permission-set
      default: true
      via:
        provider: aws-sso-dev
      principal:
        account_id: "999888777666"
        permission_set: DeveloperAccess
logs:
  level: Warning
  file: /dev/stderr
```

Note: The YAML configuration is syntax highlighted with colors (similar to `atmos describe config`).

Show only file list:
```bash
atmos profile show developer --files
```

Output:
```
Profile: developer

Locations
  • /infrastructure/atmos/.atmos/profiles/developer (2 files)
  • ~/.config/atmos/profiles/developer (1 file)

Files (in merge order)
  1. /infrastructure/atmos/.atmos/profiles/developer/auth.yaml
  2. /infrastructure/atmos/.atmos/profiles/developer/logging.yaml
  3. ~/.config/atmos/profiles/developer/overrides.yaml
```

JSON format:
```bash
atmos profile show developer --format json
```

Output:
```json
{
  "name": "developer",
  "locations": [
    {
      "path": "/infrastructure/atmos/.atmos/profiles/developer",
      "type": "project",
      "files": ["auth.yaml", "logging.yaml"]
    },
    {
      "path": "~/.config/atmos/profiles/developer",
      "type": "user",
      "files": ["overrides.yaml"]
    }
  ],
  "merged_config": {
    "auth": {
      "identities": {
        "developer-sandbox": {
          "kind": "aws/permission-set",
          "default": true
        }
      }
    },
    "logs": {
      "level": "Warning"
    }
  }
}
```

Show with provenance tracking:
```bash
atmos profile show developer --provenance
```

Output:
```
Profile: developer

Locations
  • /infrastructure/atmos/.atmos/profiles/developer (2 files)
  • ~/.config/atmos/profiles/developer (1 file)

Files (in merge order)
  1. /infrastructure/atmos/.atmos/profiles/developer/auth.yaml
  2. /infrastructure/atmos/.atmos/profiles/developer/logging.yaml
  3. ~/.config/atmos/profiles/developer/overrides.yaml

Merged Configuration (with provenance)

auth:
  identities:
    developer-sandbox:
      kind: aws/permission-set  # .atmos/profiles/developer/auth.yaml:4
      default: true              # .atmos/profiles/developer/auth.yaml:5
      via:
        provider: aws-sso-dev    # .atmos/profiles/developer/auth.yaml:7
      principal:
        account_id: "999888777666"     # .atmos/profiles/developer/auth.yaml:9
        permission_set: DeveloperAccess # .atmos/profiles/developer/auth.yaml:10
logs:
  level: Warning               # profiles/developer/overrides.yaml:2 (XDG)
  file: /dev/stderr            # .atmos/profiles/developer/logging.yaml:3
```

Note: Provenance annotations show the source file and line number where each value was defined. If a value was overridden, the most recent source is shown.

#### CI Profile Example

**Note:** This example uses `auth.defaults.identity` from the [Auth Default Settings PRD](./auth-default-settings.md) to provide deterministic identity selection in non-interactive (CI) environments.

```yaml
# profiles/ci/_metadata.yaml (or in auth.yaml as first file)
metadata:
  name: ci
  description: "GitHub Actions CI/CD environment for production deployments"
  version: "2.1.0"
  tags:
    - ci
    - github-actions
    - production
    - non-interactive

# profiles/ci/auth.yaml
auth:
  # Use auth.defaults.identity for deterministic selection in CI (no TTY needed)
  defaults:
    identity: github-oidc-identity  # Selected default (overrides identity.default: true)
    session:
      duration: "12h"                # Global session duration for this profile

  identities:
    github-oidc-identity:
      kind: aws/assume-role
      via:
        provider: github-oidc-provider
      principal:
        assume_role: "arn:aws:iam::123456789012:role/GitHubActionsDeployRole"
        role_session_name: '{{ env "GITHUB_RUN_ID" }}'  # Gomplate syntax

  providers:
    github-oidc-provider:
      kind: github/oidc
      region: us-east-1

# profiles/ci/logging.yaml
logs:
  level: Info
  file: /dev/stderr

# profiles/ci/terminal.yaml
settings:
  terminal:
    color: false
    pager: false
```

Usage:
```bash
# In .github/workflows/deploy.yml
- name: Deploy infrastructure
  run: atmos terraform apply component -s prod --profile ci
  env:
    ATMOS_PROFILE: ci

# Override identity even with profile active
atmos terraform apply component -s prod --profile ci --identity different-identity

# Or via environment variable
ATMOS_PROFILE=ci ATMOS_IDENTITY=different-identity atmos terraform apply component -s prod
```

**How profiles interact with identity selection (with `auth.defaults`):**
- Profile sets `auth.defaults.identity: github-oidc-identity` (selected default)
- Without `--identity` or `ATMOS_IDENTITY`: Uses `github-oidc-identity` automatically
- With `--identity different-identity`: Overrides profile default, uses `different-identity`
- With `ATMOS_IDENTITY=different-identity`: Overrides profile default, uses `different-identity`

**Precedence chain:**
```
1. --identity=explicit         (CLI flag with value)
2. ATMOS_IDENTITY             (environment variable)
3. auth.defaults.identity     (selected default from profile)
4. identity.default: true     (favorites - interactive or error)
5. Error: no default identity
```

**Why this works in CI:**
- `auth.defaults.identity` provides deterministic selection (no TTY needed)
- No `identity.default: true` means no "multiple defaults" errors
- Profile encapsulates all CI-specific auth configuration

#### Developer Profile Example

**Note:** This example shows how profiles can use `auth.defaults.identity` to set a sensible default while still allowing multiple identities for quick switching.

```yaml
# profiles/developer/_metadata.yaml
metadata:
  name: developer
  description: "Developer workstation configuration with AWS SSO"
  version: "1.5.0"
  tags:
    - development
    - local
    - interactive
    - aws-sso

# profiles/developer/auth.yaml
auth:
  # Selected default for developer profile
  defaults:
    identity: developer-sandbox  # Automatic when --profile developer
    session:
      duration: "8h"              # Shorter sessions for development

  identities:
    developer-sandbox:
      kind: aws/permission-set
      default: true  # Also mark as favorite for --identity flag
      via:
        provider: aws-sso-dev
      principal:
        account_id: "999888777666"
        permission_set: DeveloperAccess

    developer-prod:
      kind: aws/permission-set
      default: true  # Favorite for quick switching with --identity
      via:
        provider: aws-sso-prod
      principal:
        account_id: "123456789012"
        permission_set: ReadOnlyAccess

  providers:
    aws-sso-dev:
      kind: aws/sso
      region: us-east-2
      start_url: https://dev.awsapps.com/start
    aws-sso-prod:
      kind: aws/sso
      region: us-east-1
      start_url: https://prod.awsapps.com/start

# profiles/developer/logging.yaml
logs:
  level: Warning
  file: /dev/stderr

# profiles/developer/terraform.yaml
components:
  terraform:
    auto_generate_backend_file: true
```

Usage:
```bash
# Developer's .zshrc or .bashrc
export ATMOS_PROFILE=developer

# Or per-command (uses developer-sandbox automatically)
atmos terraform plan vpc -s dev --profile developer

# Interactive identity selection from favorites (shows developer-sandbox and developer-prod)
atmos terraform plan vpc -s dev --profile developer --identity

# Explicit identity override
atmos terraform plan vpc -s dev --profile developer --identity developer-prod
```

**Benefits of combining `auth.defaults.identity` with `identity.default: true`:**
- `auth.defaults.identity: developer-sandbox` - Automatic default (no prompts)
- `identity.default: true` on both identities - Marks as favorites
- `--identity` flag without value - Shows interactive selector with favorites
- Best of both worlds: Sensible default + quick switching

#### Debug Profile Example

```yaml
# profiles/debug/_metadata.yaml
metadata:
  name: debug
  description: "Debug logging and CPU profiling for troubleshooting"
  tags:
    - debug
    - troubleshooting
    - verbose

# profiles/debug/logging.yaml
logs:
  level: Trace
  file: ./atmos-debug.log

# profiles/debug/profiler.yaml
profiler:
  enabled: true
  profile_type: cpu
  file: ./atmos-profile.prof

# profiles/debug/terminal.yaml
settings:
  terminal:
    color: true
    pager: false
```

Usage:
```bash
# Combine with other profiles for debugging (comma-separated)
atmos terraform plan vpc -s dev --profile developer,debug

# Or using repeated flags
atmos terraform plan vpc -s dev --profile developer --profile debug

# Or via environment variable
ATMOS_PROFILE=developer,debug atmos terraform plan vpc -s dev

# Debug output goes to ./atmos-debug.log
# CPU profile saved to ./atmos-profile.prof
```

#### Describe Config with Provenance

Show where configuration values originated (including active profiles):

```bash
atmos describe config --profile developer --provenance
```

Output:
```
Active Profiles: developer

Configuration (with provenance)

auth:
  identities:
    developer-sandbox:
      kind: aws/permission-set     # profiles/developer/auth.yaml:4
      default: true                 # profiles/developer/auth.yaml:5
      via:
        provider: aws-sso-dev       # profiles/developer/auth.yaml:7
      principal:
        account_id: "999888777666"  # profiles/developer/auth.yaml:9
  providers:
    aws-sso-dev:
      kind: aws/sso                 # profiles/developer/auth.yaml:12
      region: us-east-2             # profiles/developer/auth.yaml:13
      start_url: https://dev.awsapps.com/start  # profiles/developer/auth.yaml:14
base_path: ./infrastructure         # atmos.yaml:1
logs:
  level: Warning                    # profiles/developer/logging.yaml:2
  file: /dev/stderr                 # profiles/developer/logging.yaml:3
components:
  terraform:
    base_path: components/terraform # atmos.yaml:8
    auto_generate_backend_file: true # profiles/developer/terraform.yaml:3
settings:
  terminal:
    color: true                     # atmos.yaml:15
```

Note: Values from active profiles show `profiles/<name>/<file>` as the source. Values from base `atmos.yaml` show `atmos.yaml` as the source. This helps users understand which configurations are coming from profiles vs base config.

With multiple profiles:
```bash
atmos describe config --profile developer,debug --provenance
```

Output shows precedence (rightmost profile wins):
```
Active Profiles: developer, debug (left-to-right precedence)

Configuration (with provenance)

logs:
  level: Trace                      # profiles/debug/logging.yaml:2 (override)
  file: ./atmos-debug.log           # profiles/debug/logging.yaml:3 (override)
profiler:
  enabled: true                     # profiles/debug/profiler.yaml:2
settings:
  terminal:
    pager: false                    # profiles/debug/terminal.yaml:4 (override)
```

**Profile composition behavior:**
- When using `--profile developer,debug`:
  1. First loads `developer` profile (all files lexicographically)
  2. Then loads `debug` profile (all files lexicographically)
  3. Later profiles override earlier ones for conflicting settings
  4. In this example: `debug/logging.yaml` overrides `developer/logging.yaml` for `logs.level`

#### Tag-Based Resource Filtering

**Use Case:** Automatically show only relevant resources when a profile is active.

**Configuration:**

```yaml
# profiles/developer/_metadata.yaml
metadata:
  name: developer
  tags:
    - developer
    - local
    - development

# atmos.yaml - Identity configuration with tags
auth:
  identities:
    developer-sandbox:
      kind: aws/permission-set
      tags: ["developer", "sandbox"]  # Matches "developer" tag from profile
      via:
        provider: aws-sso-dev
      principal:
        account_id: "999888777666"
        permission_set: DeveloperAccess

    developer-prod:
      kind: aws/permission-set
      tags: ["developer", "production"]  # Matches "developer" tag from profile
      via:
        provider: aws-sso-prod
      principal:
        account_id: "123456789012"
        permission_set: ReadOnlyAccess

    platform-admin:
      kind: aws/permission-set
      tags: ["admin", "production"]  # Does NOT match profile tags
      via:
        provider: aws-sso-prod
      principal:
        account_id: "123456789012"
        permission_set: AdministratorAccess

    ci-github-oidc:
      kind: aws/assume-role
      tags: ["ci", "github-actions"]  # Does NOT match profile tags
      via:
        provider: github-oidc-provider
```

**Usage with Tag Filtering:**

```bash
# List identities with tag filtering enabled
atmos auth list identities --profile developer --filter-by-profile-tags
```

**Output (filtered):**
```
Available Identities (filtered by profile tags: developer, local, development)

┏━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ Identity          ┃ Kind               ┃ Tags                   ┃
┡━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━┩
│ developer-sandbox │ aws/permission-set │ developer, sandbox     │
│ developer-prod    │ aws/permission-set │ developer, production  │
└───────────────────┴────────────────────┴────────────────────────┘

Showing 2 of 4 total identities (filtered by tags)
```

**Without tag filtering:**

```bash
# Show all identities regardless of tags
atmos auth list identities --profile developer
```

**Output (unfiltered):**
```
Available Identities

┏━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ Identity          ┃ Kind               ┃ Tags                   ┃
┡━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━┩
│ developer-sandbox │ aws/permission-set │ developer, sandbox     │
│ developer-prod    │ aws/permission-set │ developer, production  │
│ platform-admin    │ aws/permission-set │ admin, production      │
│ ci-github-oidc    │ aws/assume-role    │ ci, github-actions     │
└───────────────────┴────────────────────┴────────────────────────┘

Showing 4 identities
```

**Benefits:**
- **Reduces noise** - Only see relevant resources for your current context
- **Improves UX** - Developer doesn't see CI/admin identities
- **Clear intent** - Tags communicate which resources belong to which profiles
- **Opt-in** - Tag filtering disabled by default (backward compatible)

**Multiple Profiles with Tag Filtering:**

```bash
# Activate both developer and ci profiles
atmos auth list identities --profile developer,ci --filter-by-profile-tags
```

**Result:**
- Profile tags combined: `["developer", "local", "development", "ci", "github-actions"]`
- Shows: `developer-sandbox`, `developer-prod`, `ci-github-oidc`
- Hides: `platform-admin` (no matching tags)

#### Platform Admin Profile Example

```yaml
# profiles/platform-admin/_metadata.yaml
metadata:
  name: platform-admin
  description: "Production platform administrator access"
  tags:
    - admin
    - production
    - privileged

# profiles/platform-admin/auth.yaml
auth:
  defaults:
    identity: platform-admin  # Selected default for this profile

  identities:
    platform-admin:
      kind: aws/permission-set
      tags: ["admin", "production"]  # Matches profile tags
      via:
        provider: aws-sso-prod
      principal:
        account_id: "123456789012"
        permission_set: AdministratorAccess

  providers:
    aws-sso-prod:
      kind: aws/sso
      region: us-east-1
      start_url: https://prod.awsapps.com/start
```

Usage:
```bash
atmos terraform apply vpc -s prod --profile platform-admin

# With tag filtering: Only shows admin-tagged identities
atmos auth list identities --profile platform-admin --filter-by-profile-tags
```

### Implementation Plan

**Architecture**: This implementation follows the modern Atmos command pattern where command logic lives in `cmd/` packages and shared functionality lives in focused `pkg/` packages. The `internal/exec/` folder is legacy and will NOT be extended.

**Reference implementation**: `cmd/theme/` demonstrates the exact pattern to follow.

**Package structure**:
- `cmd/profile/` - All profile command implementations
- `pkg/profile/` - Shared profile discovery, validation, and loading logic
- `pkg/config/` - Profile loading integration with config system

**Detailed implementation guide**: See `.scratch/profiles-implementation-plan.md` for step-by-step tasks with code examples.

#### Phase 1: Core Profile Loading (Week 1-2)

**Tasks:**

1. **Add `--profile` flag to global flags:**
   - Update `pkg/flags/global/flags.go` to add `Profile []string` field
   - Register flag in `pkg/flags/global_builder.go`:
     ```go
     b.options = append(b.options, func(cfg *parserConfig) {
         cfg.registry.Register(&StringSliceFlag{
             Name:        "profile",
             Shorthand:   "",
             Default:     []string{},
             Description: "Activate configuration profiles (comma-separated or repeated)",
             EnvVars:     []string{"ATMOS_PROFILE"},
         })
     })
     ```
   - Automatic Viper binding handles precedence: CLI flag > ENV var > config > defaults
   - Flag will be available globally across all commands

2. **Add top-level `Profiles` configuration to schema:**
   - Add `ProfilesConfig` struct to `pkg/schema/schema.go`:
     ```go
     type ProfilesConfig struct {
         BasePath string   `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
         Enabled  bool     `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
     }
     ```
   - Add `Profiles` field to `AtmosConfiguration`:
     ```go
     type AtmosConfiguration struct {
         // ... existing fields ...
         Profiles ProfilesConfig `yaml:"profiles" json:"profiles" mapstructure:"profiles"`
     }
     ```
   - Update JSON schemas in `pkg/datafetcher/schema/`

3. **Create shared config directory loading function:**
   - Add `loadAtmosConfigsFromDirectory(searchPattern, dst, source)` to `pkg/config/load.go`
   - Reuses existing `SearchAtmosConfig()` and `mergeConfigFile()` infrastructure
   - Refactor `.atmos.d/` loading to use this shared function (see `.scratch/profiles-loading-refactor.md`)
   - Benefits: Single source of truth, consistent behavior, better error messages

4. **Implement profile discovery and loading in `pkg/profile/`:**
   - `pkg/profile/profile.go` - Core profile logic:
     - `DiscoverAllProfiles(atmosConfig) ([]ProfileInfo, error)` - Find all available profiles
     - `GetProfileDetails(atmosConfig, profileName) (*ProfileDetails, error)` - Get details for specific profile
     - `MergeProfileConfiguration(atmosConfig, profileName) (map[string]interface{}, error)` - Load and merge profile
   - `pkg/profile/interface.go` - Define `ProfileLoader` interface for testability
   - Generate mocks with `//go:generate mockgen` for testing

5. **Integrate profile loading into config system:**
   - Add `LoadProfiles(v *viper.Viper, profileNames []string, atmosConfig)` to `pkg/config/profiles.go`
   - Uses `loadAtmosConfigsFromDirectory()` for consistent loading behavior
   - Inject profiles after `.atmos.d/` but before local `atmos.yaml` in config loading chain

6. **Implement XDG profile location support:**
   - Use `pkg/xdg.GetXDGConfigDir("profiles", 0o755)` for user-global profiles
   - Respect `ATMOS_XDG_CONFIG_HOME` and `XDG_CONFIG_HOME` environment variables

7. **Profile file loading behavior:**
   - Lexicographic ordering of files within profile directory (via `SearchAtmosConfig()`)
   - Deep merge semantics for configuration values (via `mergeConfigFile()`)
   - Recursive directory support with depth-based sorting
   - Priority file handling (atmos.yaml loaded first)

8. **Profile precedence logic:**
   - Multiple profiles: left-to-right (first profile lowest precedence)
   - Multiple locations: configurable > project-local hidden > XDG > project-local non-hidden
   - Configuration loading chain: embedded defaults → system → home → working dir → env vars → config path → `.atmos.d/` → **profiles** → local `atmos.yaml`

**Deliverables:**
- Shared config directory loading function in `pkg/config/load.go`
- Profile discovery and loading package in `pkg/profile/`
- Profile integration in `pkg/config/profiles.go`
- Top-level `profiles` configuration schema in `pkg/schema/`
- Global `--profile` flag in `pkg/flags/global/`
- Unit tests for profile discovery, loading, and precedence
- Integration tests with sample profiles

#### Phase 2: Profile Management Commands & Provenance (Week 3)

**Tasks:**

1. **Create command registry provider in `cmd/profile/` directory:**
   - Follow `cmd/theme/` pattern as reference implementation
   - `cmd/profile/profile.go` - Main profile command with `CommandProvider` interface:
     ```go
     type ProfileCommandProvider struct{}
     func (p *ProfileCommandProvider) GetCommand() *cobra.Command { return profileCmd }
     func (p *ProfileCommandProvider) GetName() string { return "profile" }
     func (p *ProfileCommandProvider) GetGroup() string { return "Other Commands" }
     ```
   - Register with command registry in `init()`:
     ```go
     func init() {
         internal.Register(&ProfileCommandProvider{})
     }
     ```
   - Add blank import to `cmd/root.go`: `_ "github.com/cloudposse/atmos/cmd/profile"`

2. **Implement `atmos profile list` subcommand:**
   - `cmd/profile/list.go` - Command implementation with modern table rendering
   - Calls `profile.DiscoverAllProfiles(atmosConfigPtr)` from `pkg/profile/`
   - **Modern Design** (follows `cmd/version/list.go` pattern):
     - Green dot (●) indicator for active profiles
     - Clean lipgloss table with only header border (no borders around table)
     - Gray text for secondary information (location column)
     - Minimal, modern aesthetic matching `atmos version list`
   - **Table columns:**
     - Empty column with green dot (●) for active profiles
     - PROFILE - Profile name
     - DESCRIPTION - From `_metadata.yaml` or "-"
     - LOCATION - Gray text showing "Project", "User", or "Custom"
   - **Output handling:**
     - Structured output: `--format json|yaml` using `data.WriteJSON()` / `data.WriteYAML()`
     - Human output: lipgloss table to stderr, summary with `ui.Info()`
   - **Active profile detection:**
     - Check `--profile` flag or `ATMOS_PROFILE` env var
     - Show green dot for currently active profiles
     - Display count of active profiles in footer

3. **Implement `atmos profile show <profile>` subcommand:**
   - `cmd/profile/show.go` - Command implementation for profile details
   - Calls `profile.GetProfileDetails(atmosConfigPtr, profileName)` and `profile.MergeProfileConfiguration()` from `pkg/profile/`
   - **UI/Theme integration:**
     - Tables via lipgloss (follow `cmd/theme/show.go` pattern)
     - YAML syntax highlighting via `u.GetHighlightedYAML()`
     - Theme-aware styling adapts to terminal theme
   - **Flags:**
     - `--format json|yaml` - Structured output
     - `--files` - Show file list only
     - `--provenance` - Show configuration value origins (future)
   - **Output channels:**
     - Data (stdout): `data.WriteYAML()`, `data.WriteJSON()`
     - UI (stderr): `ui.Info()`, `ui.Success()`, `ui.Write()`
   - **Terminal capability handling:**
     - Automatic color degradation
     - Respects `--color`, `--no-color`, `--force-color`, `NO_COLOR`
     - Width adapts to terminal or config `max_width`
   - Show all locations where profile is found
   - Display file merge order

4. **Implement `atmos list profiles` alias command:**
   - `cmd/list_profiles.go` - Alias for consistency with other list commands
   - Delegates to `cmd/profile/list.go` implementation
   - Both commands produce identical output
   - Help text cross-references the alias

5. **Error handling with error builder pattern:**
   - Add profile-specific sentinel errors to `errors/errors.go`:
     - `ErrProfileNotFound`
     - `ErrProfileDirNotExist`
     - `ErrProfileInvalid`
   - Use `errUtils.Build()` pattern with `WithHintf()`, `WithContext()`, `WithExitCode()`
   - Clear, actionable error messages with context

6. **Debug logging:**
   - Structured logging via `log` package
   - Log profile discovery process at `log.Debug()` level
   - Log file merge order at `log.Trace()` level
   - Log configuration precedence decisions

**Deliverables:**
- Profile command implementation in `cmd/profile/` (profile.go, list.go, show.go)
- Profile discovery and loading in `pkg/profile/` with interface and mocks
- `atmos list profiles` alias command in `cmd/list_profiles.go`
- Theme-aware UI using `pkg/ui/theme` and `pkg/io` patterns
- Error builder pattern with profile-specific sentinel errors
- Structured debug logging
- Unit tests for `cmd/profile/*` commands
- Unit tests for `pkg/profile/` logic with mocks
- CLI integration tests with golden snapshots

#### Phase 3: Documentation and Examples (Week 4)

**Tasks:**
1. Create profile configuration examples for:
   - CI/CD environments (GitHub Actions, GitLab CI, CircleCI)
   - Developer roles (developer, platform-engineer, audit)
   - Debug scenarios (trace logging, profiling)
2. Document precedence rules with diagrams
3. Create migration guide for environment-specific configurations
4. Document provenance usage for debugging profile configurations
5. Write blog post announcing profiles feature
6. Update JSON schemas for profile validation

**Deliverables:**
- Comprehensive documentation in `website/docs/`
- Blog post in `website/blog/`
- Example profile configurations in `examples/profiles/`
- Updated JSON/YAML schemas in `pkg/datafetcher/schema/`

### Error Handling

#### Profile Not Found
```
Error: Profile 'ci' not found

Requested profile 'ci' does not exist.

Available profiles:
  • developer     (/infrastructure/atmos/profiles/developer)
  • debug         (/infrastructure/atmos/profiles/debug)
  • platform-admin (/infrastructure/atmos/profiles/platform-admin)

Profile location: /infrastructure/atmos/profiles/

Tip: Create profile directory with: mkdir -p /infrastructure/atmos/profiles/ci
```

#### Invalid Profile Configuration
```
Error: Failed to load profile 'ci'

Syntax error in /infrastructure/atmos/profiles/ci/auth.yaml:
  line 5: duplicate key 'identities'

Profile: ci
File: auth.yaml
Path: /infrastructure/atmos/profiles/ci/auth.yaml
```

#### Multiple Profiles with Conflicts
```
Warning: Configuration conflict in profiles

Setting 'logs.level' is defined in multiple profiles:
  • developer: Warning
  • debug: Trace

Using value from last profile: debug (Trace)
```

### No Impact on Existing Functionality

**Requirement**: Since profiles are a new feature, they have zero impact on existing Atmos usage.

**Verification**:
1. If no `--profile` flag or `ATMOS_PROFILE` environment variable is set, Atmos behaves exactly as it does today (no profile loading)
2. Existing `atmos.yaml` configurations continue to work unchanged
3. `.atmos.d/` imports continue functioning as before
4. All existing tests pass without modification
5. Users can adopt profiles incrementally without breaking existing workflows

## Success Metrics

1. **Adoption**: 30% of Atmos users create at least one profile within 6 months
2. **CI/CD Integration**: 50% of CI/CD pipelines use profile-based configuration within 3 months
3. **Issue Reduction**: 20% reduction in configuration-related support issues
4. **Performance**: Profile loading adds <100ms overhead for typical profiles
5. **Documentation**: Profile documentation page among top 10 most visited docs pages

## Integration with Auth Default Settings

**Dependency**: This feature depends on the [Auth Default Settings PRD](./auth-default-settings.md) which introduces `auth.defaults.identity` for deterministic identity selection.

### Problem Solved

**Without `auth.defaults.identity`:**
```yaml
# Base config with multiple favorites
auth:
  identities:
    developer-sandbox:
      default: true  # Favorite
    developer-prod:
      default: true  # Favorite

# In TTY: Interactive selection (works)
# In CI: Error - "multiple default identities" (breaks)
```

**With `auth.defaults.identity` in profiles:**
```yaml
# profiles/ci/auth.yaml
auth:
  defaults:
    identity: github-oidc-identity  # Deterministic selection

# In TTY: Uses github-oidc-identity (works)
# In CI: Uses github-oidc-identity (works)
```

### How Profiles Use Auth Defaults

**Pattern 1: CI Profile (Non-Interactive)**
```yaml
# profiles/ci/auth.yaml
auth:
  defaults:
    identity: github-oidc-identity  # Required for CI
    # No identity.default: true needed
```

**Pattern 2: Developer Profile (Interactive + Default)**
```yaml
# profiles/developer/auth.yaml
auth:
  defaults:
    identity: developer-sandbox     # Automatic default
  identities:
    developer-sandbox:
      default: true                  # Also mark as favorite
    developer-prod:
      default: true                  # Favorite for quick switching
```

**Pattern 3: Base Config (Favorites Only)**
```yaml
# atmos.yaml (no profile active)
auth:
  # No auth.defaults - use favorites pattern
  identities:
    identity-a:
      default: true  # Favorite
    identity-b:
      default: true  # Favorite
  # TTY: Interactive selection
  # CI: Error (intentional - forces explicit --identity or profile usage)
```

### Precedence with Profiles

When profiles are active, the full precedence chain is:

```
1. --identity=explicit            (CLI flag)
2. ATMOS_IDENTITY                 (env var)
3. auth.defaults.identity         (from active profile) ← Profiles use this
4. identity.default: true         (favorites from base or profile)
5. Error: no default identity
```

**Example with multiple profiles:**
```bash
# Both profiles set auth.defaults.identity
atmos terraform plan --profile developer,ci
# Result: Uses ci profile's auth.defaults.identity (rightmost wins)
```

### Key Benefits for Profiles

1. **Deterministic CI behavior** - No "multiple defaults" errors
2. **Profile encapsulation** - Auth config stays within profile
3. **Base config flexibility** - Can use favorites without breaking CI
4. **Clear override** - Profile's selected default overrides base favorites
5. **Backward compatible** - Existing `identity.default: true` still works

## Technical Dependencies

- Existing configuration loading logic in `pkg/config/load.go`
- Viper configuration merging behavior
- Cobra CLI flag parsing in `cmd/root.go`
- JSON schema validation in `pkg/datafetcher/schema/`
- **[Auth Default Settings PRD](./auth-default-settings.md)** - `auth.defaults` schema and logic

## Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Profile merge behavior confusion | High | Medium | Clear documentation with examples, verbose debug logging |
| Performance degradation with many profiles | Medium | Low | Performance tests, profile caching, lazy loading |
| Profile naming conflicts | Low | Medium | Profile validation, clear error messages |
| Complex multi-profile precedence | Medium | Medium | Visual precedence diagrams, `profile show` command |
| Unintended profile activation in CI/CD | Medium | Low | Clear error messages when profile not found, explicit activation only |

## Open Questions

1. **Should profile location precedence be configurable?**
   - **Decision**: Fixed precedence order (configurable > project-local > XDG > legacy) for predictability

2. **Should profiles support environment-specific auto-activation?**
   - **Decision**: No, explicit activation only. Use wrapper scripts or aliases if needed

3. **Should profiles support conditional loading?** (e.g., `if: ${CI} == "true"`)
   - **Decision**: No, profiles are static. Use shell logic to select profile: `atmos --profile ${CI:+ci}`

4. **How should profile name conflicts across locations be handled?**
   - **Decision**: Merge profiles with same name (later locations override earlier), document clearly in error messages

## Design Decisions

### Profile Inheritance
**Decision**: Profile inheritance is **already supported** via the existing `import:` field mechanism.

Profiles can use `import:` to reference other profiles or configuration files:
```yaml
# profiles/ci-prod/atmos.yaml
import:
  - ../ci/auth.yaml          # Import from sibling profile
  - ../ci/logging.yaml
  - ~/.config/atmos/profiles/base/settings.yaml  # Import from XDG location

# Override specific settings
logs:
  level: Warning             # Override ci profile's Info level
```

This leverages Atmos's existing import resolution, cycle detection, and merge logic without requiring new code.

### Global Profiles (XDG)
**Decision**: User-global profiles are **included in Phase 1** via XDG Base Directory Specification support.

Location: `$XDG_CONFIG_HOME/atmos/profiles/` (default: `~/.config/atmos/profiles/`)

Benefits:
- Consistent with Atmos's XDG support for keyring and cache
- Enables sharing debug/development profiles across projects
- Follows CLI tool conventions (same path on Linux and macOS)

## Future Enhancements

### Profile Templates (Future)
Provide official profile templates via `atmos profile init`:
```bash
atmos profile init --template github-actions --name ci
atmos profile init --template developer --name dev
```

This would generate starter profile directories with common configurations. May not be needed if profiles are simple enough to create manually.

### Profile Validation (Future)
Validate profile configuration syntax without activating:
```bash
atmos profile validate ci

# Output:
# Validating profile: ci
#
# ✓ Profile found at: /infrastructure/atmos/.atmos/profiles/ci
# ✓ auth.yaml - Valid syntax
# ✓ logging.yaml - Valid syntax
# ✗ terminal.yaml - Syntax error at line 5: unexpected key 'invalid_field'
#
# Profile validation failed: 1 error found
```

**Use cases:**
- Pre-commit hooks to validate profile changes
- CI/CD to ensure profile configurations are valid
- Development workflow to catch errors before using profiles

**Implementation:**
- Parse all YAML files in profile directory
- Validate against Atmos configuration schema
- Check for import cycles
- Report syntax errors with file path and line number
- Exit code 0 for success, 1 for validation errors

## References

- [Command Merging PRD](./command-merging.md) - Similar precedence and merge behavior for configuration inheritance
- [Command Registry Pattern PRD](./command-registry-pattern.md) - Modular architecture inspiration
- [IO Handling Strategy PRD](./io-handling-strategy.md) - Output patterns for profile commands
- [XDG Base Directory Specification PRD](./xdg-base-directory-specification.md) - XDG support for user-global profiles
- [Atmos Configuration Documentation](https://atmos.tools/cli/configuration)
- [Viper Configuration Library](https://github.com/spf13/viper)
- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
