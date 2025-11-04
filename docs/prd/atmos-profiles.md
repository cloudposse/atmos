# Product Requirements Document: Atmos Profiles

## Overview

This document describes the requirements and implementation for Atmos profiles, which enable users to maintain multiple configuration presets that can be activated via CLI flags or environment variables. Profiles provide environment-specific, role-based, or context-specific configuration overrides without modifying the base `atmos.yaml` configuration.

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

2. **Project-local profiles**:
   - `{atmos_cli_config_path}/.atmos/profiles/` (hidden directory, project-specific)
   - Example: `/infrastructure/atmos/.atmos/profiles/`

3. **XDG user profiles** (follows XDG Base Directory Specification):
   - `$XDG_CONFIG_HOME/atmos/profiles/` (default: `~/.config/atmos/profiles/`)
   - `$ATMOS_XDG_CONFIG_HOME/atmos/profiles/` (Atmos-specific override)
   - Platform-aware: Uses `~/.config` on Linux/macOS, `%APPDATA%` on Windows

4. **Project profiles (non-hidden)**:
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

**FR5.9**: `atmos describe config` MUST show currently active profiles in output

**FR5.10**: `atmos describe config` with active profiles MUST show:
```yaml
# Active profiles: developer, debug
active_profiles:
  - name: developer
    locations:
      - /infrastructure/atmos/.atmos/profiles/developer
  - name: debug
    locations:
      - ~/.config/atmos/profiles/debug
```

**FR5.11**: Debug logging (`--logs-level trace`) MUST show profile loading details:
- Which profiles are being loaded
- From which locations
- File merge order
- Configuration precedence

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

**TR2.3**: New top-level `Profiles` configuration MUST be added to `AtmosConfiguration` in `pkg/schema/schema.go`:
```go
type AtmosConfiguration struct {
    // ... existing fields ...
    Profiles ProfilesConfig `yaml:"profiles,omitempty" json:"profiles,omitempty" mapstructure:"profiles"`
}

type ProfilesConfig struct {
    BasePath string `yaml:"base_path,omitempty" json:"base_path,omitempty" mapstructure:"base_path"`
}
```

**TR2.4**: JSON Schema definitions in `pkg/datafetcher/schema/` MUST include top-level `profiles` configuration

**TR2.5**: Configuration errors in profiles MUST include profile name and source location in error messages

**TR2.6**: Profile imports MUST use the same validation and cycle detection as stack imports

#### TR3: Performance

**TR3.1**: Profile loading MUST complete within 100ms for typical profiles (<10 files)

**TR3.2**: Profile discovery MUST be cached during a single Atmos command execution

**TR3.3**: Memory usage MUST scale linearly with number of active profiles

**TR3.4**: Profile configuration MUST not impact performance when no profiles are active

#### TR4: Testing

**TR4.1**: Unit tests MUST verify profile precedence rules

**TR4.2**: Integration tests MUST verify profile merging with various configuration combinations

**TR4.3**: Tests MUST verify multiple profile activation and precedence

**TR4.4**: Tests MUST verify profile error handling (missing profiles, invalid syntax)

**TR4.5**: Tests MUST verify backward compatibility (no profiles specified)

#### TR5: Documentation

**TR5.1**: Documentation MUST include profile configuration examples for common scenarios (CI, developer, debug)

**TR5.2**: Documentation MUST explain precedence rules with visual diagrams

**TR5.3**: Migration guide MUST be provided for users converting environment-specific configurations to profiles

**TR5.4**: Blog post MUST be included announcing this minor feature (per CLAUDE.md requirement)

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
1. `profiles.base_path` (if configured in `atmos.yaml`)
2. `{atmos_cli_config_path}/.atmos/profiles/` (hidden)
3. `$XDG_CONFIG_HOME/atmos/profiles/` (e.g., `~/.config/atmos/profiles/`)
4. `{atmos_cli_config_path}/profiles/` (non-hidden)

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

#### CI Profile Example

```yaml
# profiles/ci/auth.yaml
auth:
  identities:
    github-oidc-identity:
      kind: aws/assume-role
      default: true  # This identity becomes the default when ci profile is active
      via:
        provider: github-oidc-provider
      principal:
        assume_role: "arn:aws:iam::123456789012:role/GitHubActionsDeployRole"
        role_session_name: "atmos-${GITHUB_RUN_ID}"

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

**How profiles interact with identity selection:**
- Profile defines `github-oidc-identity` with `default: true`
- Without `--identity` or `ATMOS_IDENTITY`: Uses `github-oidc-identity` from profile
- With `--identity different-identity`: Overrides profile default, uses `different-identity`
- With `ATMOS_IDENTITY=different-identity`: Overrides profile default, uses `different-identity`

#### Developer Profile Example

```yaml
# profiles/developer/auth.yaml
auth:
  identities:
    developer-sandbox:
      kind: aws/permission-set
      default: true  # This becomes default when developer profile is active
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

# Or per-command
atmos terraform plan vpc -s dev --profile developer
```

#### Debug Profile Example

```yaml
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

**Profile composition behavior:**
- When using `--profile developer,debug`:
  1. First loads `developer` profile (all files lexicographically)
  2. Then loads `debug` profile (all files lexicographically)
  3. Later profiles override earlier ones for conflicting settings
  4. In this example: `debug/logging.yaml` overrides `developer/logging.yaml` for `logs.level`

#### Platform Admin Profile Example

```yaml
# profiles/platform-admin/auth.yaml
auth:
  identities:
    platform-admin:
      kind: aws/permission-set
      default: true  # This becomes default when platform-admin profile is active
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
```

### Implementation Plan

#### Phase 1: Core Profile Loading (Week 1-2)

**Tasks:**
1. Add `--profile` flag (StringSlice) to root command in `cmd/root.go`
2. Add `ATMOS_PROFILE` environment variable binding (parse as comma-separated list)
3. Add top-level `Profiles` configuration to schema:
   - Add `ProfilesConfig` struct to `pkg/schema/schema.go`
   - Add `Profiles` field to `AtmosConfiguration`
   - Update JSON schemas in `pkg/datafetcher/schema/`
4. Implement profile discovery logic in `pkg/config/load.go`:
   - `discoverProfileLocations(atmosConfig *schema.AtmosConfiguration) []string` - Returns ordered list of profile search paths
   - `loadProfiles(v *viper.Viper, profileNames []string, searchPaths []string) error` - Main profile loading orchestrator
   - `findProfileDirectory(profileName string, searchPaths []string) (string, error)` - Searches for profile across all locations
   - `loadProfileFiles(v *viper.Viper, profileDir string) error` - Loads all YAML files from profile directory
5. Implement XDG profile location support:
   - Use `pkg/xdg.GetXDGConfigDir("profiles", 0o755)` for user-global profiles
   - Respect `ATMOS_XDG_CONFIG_HOME` and `XDG_CONFIG_HOME` environment variables
6. Reuse existing `mergeDefaultImports()` pattern for profile file loading
7. Support profile inheritance via existing `import:` field processing
8. Add profile precedence logic (multiple profiles, left-to-right; multiple locations, configurable > project-local > XDG > legacy)
9. Update configuration loading flow to inject profiles after `.atmos.d/` but before local `atmos.yaml`

**Deliverables:**
- Profile loading implementation in `pkg/config/`
- Top-level `profiles` configuration schema
- Unit tests for profile discovery and precedence
- Integration tests with sample profiles

#### Phase 2: Profile Management Commands (Week 3)

**Tasks:**
1. Create command registry provider in `cmd/profile/` directory:
   - `cmd/profile/profile.go` - Main profile command with CommandProvider
   - Register with command registry in `init()`
2. Implement `atmos profile list` subcommand:
   - `cmd/profile/list.go` - List all profiles across all locations
   - Support `--format json|yaml` flag
   - Group by location (Project, User, Custom)
   - Show brief description if available
3. Implement `atmos profile show <profile>` subcommand:
   - `cmd/profile/show.go` - Show detailed profile information
   - Support `--format json|yaml` flag
   - Support `--files` flag (show file list only)
   - **Reuse existing formatting utilities**:
     - Use `u.GetHighlightedYAML()` for colorized YAML output
     - Use `u.GetHighlightedJSON()` for colorized JSON output
     - Same formatting as `atmos describe config` (consistent UX)
   - Respect terminal color settings (honors `--color`, `--no-color`, `NO_COLOR`)
   - Support pager integration when enabled
   - Show all locations where profile is found
   - Display file merge order
4. Implement profile discovery helper in `internal/exec/`:
   - `internal/exec/profile.go` - Profile discovery and introspection logic
   - `DiscoverAllProfiles(atmosConfig) ([]ProfileInfo, error)`
   - `GetProfileDetails(atmosConfig, profileName) (*ProfileDetails, error)`
   - `MergeProfileConfiguration(profileName) (map[string]any, error)`
5. Update `atmos describe config`:
   - Add `active_profiles` section to output
   - Show profile names and locations
6. Add comprehensive debug logging:
   - Log profile discovery process
   - Log file merge order
   - Log configuration precedence
7. Enhance error messages:
   - Profile not found with available profiles list
   - Profile syntax errors with file path
   - Profile conflicts with clear explanation

**Deliverables:**
- `atmos profile` command with `list` and `show` subcommands
- Profile introspection API in `internal/exec/`
- Updated `describe config` with profile information
- Enhanced debugging and error messages
- Unit tests for profile commands
- CLI integration tests

#### Phase 3: Documentation and Examples (Week 4)

**Tasks:**
1. Create profile configuration examples for:
   - CI/CD environments (GitHub Actions, GitLab CI, CircleCI)
   - Developer roles (developer, platform-engineer, audit)
   - Debug scenarios (trace logging, profiling)
2. Document precedence rules with diagrams
3. Create migration guide for environment-specific configurations
4. Write blog post announcing profiles feature
5. Update JSON schemas for profile validation

**Deliverables:**
- Comprehensive documentation in `website/docs/`
- Blog post in `website/blog/`
- Example profile configurations in `examples/profiles/`

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

## Dependencies

- Existing configuration loading logic in `pkg/config/load.go`
- Viper configuration merging behavior
- Cobra CLI flag parsing in `cmd/root.go`
- JSON schema validation in `pkg/datafetcher/schema/`

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
