# Vendor Update Product Requirements Document

## Executive Summary

This PRD covers two related commands for vendor management:

1. **`atmos vendor update`** - Automated version management for vendored components. Checks upstream Git sources for newer versions and updates version references in vendor configuration files while **strictly preserving** YAML structure, comments, anchors, aliases, and formatting.

2. **`atmos vendor diff`** - Shows Git diffs between two versions of a component from a remote repository. Git-only feature that displays actual code/file changes between versions without requiring local checkout.

## Problem Statement

Currently, maintaining up-to-date versions of vendored components requires:
- Manual checking of upstream Git repositories for new releases and commits
- Manual editing of `vendor.yaml` and `component.yaml` files
- High risk of breaking complex YAML structures (anchors, aliases, merge keys)
- Loss of comments and documentation during updates
- No visibility into available updates across multiple components
- Tedious, error-prone process when managing many components with imports
- No way to see what's changed between current and available versions

## Goals

### Primary Goals
1. **Automated Version Checking**: Check Git repositories for newer tags and commits
2. **YAML Structure Preservation**: Maintain ALL YAML features during updates:
   - YAML anchors (`&anchor`)
   - YAML aliases (`*anchor`)
   - Merge keys (`<<: *anchor`)
   - Comments (inline and block)
   - Indentation and formatting
   - Quote styles (single, double, unquoted)
3. **Multi-File Support**: Handle `vendor.yaml`, `component.yaml`, and imported files
4. **Import Chain Processing**: Follow `imports:` directives recursively
5. **Filtering**: Filter by component name and tags
6. **Dry-Run Mode**: See what would change without modifying files
7. **Progressive UI**: Use Charm Bracelet TUI with progress indicators
8. **GitHub Rate Limit Respect**: Handle rate limits gracefully
9. **Test Coverage**: Achieve 80-90% test coverage
10. **Complete Documentation**: CLI docs, blog post, usage examples
11. **Version Constraints**: Support semver constraints and version exclusions via `constraints` field

### Non-Goals (Future Scope)
- OCI registry version checking
- S3/GCS bucket version checking
- HTTP/HTTPS direct file version checking (not applicable)
- Local file system version checking (not applicable)
- Automatic major version upgrades without user confirmation
- Version pinning with different folder names (complexity)
- Rollback capabilities
- Breaking change detection

## User Stories

### US-1: Check for Updates (Dry-Run)
**As a** platform engineer
**I want to** see what component updates are available
**So that** I can decide which updates to apply

```bash
atmos vendor update --check
```

**Output:**
```
Checking for vendor updates...

[=========>] 9/10 Checking terraform-aws-ecs...

✓ terraform-aws-vpc (1.323.0 → 1.372.0)
✓ terraform-aws-s3-bucket (4.1.0 → 4.2.0)
✓ terraform-aws-eks (2.1.0 - up to date)
⚠ custom-module (skipped - templated version {{.Version}})
⚠ terraform-aws-ecs (skipped - OCI registry not yet supported)
✓ terraform-aws-rds (5.0.0 → 5.1.0)

Found 3 updates available.
```

### US-2: Update Version References
**As a** platform engineer
**I want to** update version strings in my config files
**So that** vendor pull will fetch the latest versions

```bash
atmos vendor update
```

**Expected behavior:**
- Updates only the `version:` fields in YAML files
- Preserves all YAML anchors, aliases, merge keys
- Preserves all comments
- Preserves indentation and quote styles
- Updates the correct file (where the source was defined, not where it was imported)

### US-3: Update and Pull in One Command
**As a** platform engineer
**I want to** update versions and download components in one command
**So that** I can quickly get the latest components

```bash
atmos vendor update --pull
```

**Expected behavior:**
1. Update version references in config files
2. Execute `atmos vendor pull` automatically
3. Show combined progress UI

### US-4: Update Specific Component
**As a** platform engineer
**I want to** update only one component
**So that** I can test updates incrementally

```bash
atmos vendor update --component vpc
atmos vendor update --component vpc --pull
```

### US-5: Update Components by Tags
**As a** platform engineer
**I want to** update components with specific tags
**So that** I can update related components together

```bash
atmos vendor update --tags terraform,networking
atmos vendor update --tags production --check
```

### US-6: Work with Imports
**As a** platform engineer
**I want to** updates to work correctly with vendor config imports
**So that** the right files get updated

**Example:**
```yaml
# vendor.yaml
spec:
  imports:
    - vendor/terraform.yaml
    - vendor/helmfile.yaml
```

```yaml
# vendor/terraform.yaml
spec:
  sources:
    - component: vpc
      version: 1.0.0
```

**Expected:** When vpc is updated, `vendor/terraform.yaml` gets modified (not `vendor.yaml`)

### US-7: View Changes Between Versions
**As a** platform engineer
**I want to** see what changed between two versions of a component
**So that** I can assess the impact before updating

```bash
atmos vendor diff --component vpc
atmos vendor diff --component vpc --from 1.0.0 --to 2.0.0
```

**Output:**
```diff
Showing diff for component 'vpc' (1.0.0 → 2.0.0)
Source: github.com/cloudposse/terraform-aws-vpc.git

diff --git a/main.tf b/main.tf
...
+  enable_dns_support = var.enable_dns_support
...
```

**Expected behavior:**
- Show Git diff between two versions without local clone
- Support comparing current version to latest
- Support comparing any two specific versions
- Work with Git repositories only (not OCI, local, HTTP)

## Supported Upstream Sources

| Source Type | Version Detection | Priority | Notes |
|-------------|------------------|----------|-------|
| **Git Repositories** (GitHub, GitLab, Bitbucket, self-hosted) | Tags & commits via `git ls-remote` | P0 - MUST | Primary use case |
| **OCI Registries** | Registry API | P2 - Future | Complex, different API per registry |
| **HTTP/HTTPS Direct Files** | N/A | N/A | No versioning concept |
| **Local File System** | N/A | N/A | No versioning concept |
| **Amazon S3** | Object metadata | P3 - Future | Requires AWS SDK |
| **Google GCS** | Object metadata | P3 - Future | Requires GCP SDK |

### Git Repository Version Detection

**Tag-based versions:**
```bash
# Get all tags from remote
git ls-remote --tags https://github.com/cloudposse/terraform-aws-vpc.git

# Parse and sort semantic versions
# Filter out pre-release versions (alpha, beta, rc)
# Compare with current version
# Return latest stable version
```

**Commit-based versions:**
```bash
# Get HEAD commit hash
git ls-remote https://github.com/cloudposse/terraform-aws-vpc.git HEAD

# Compare with current commit hash (7+ chars)
# Return if different
```

**Templated versions (skip):**
```yaml
version: "{{.Version}}"  # Skip - contains template syntax
version: "{{ atmos.Component }}"  # Skip - contains template syntax
```

## Version Constraints

### Overview

Version constraints allow users to control which versions are allowed when updating vendored components. This provides:
- **Safety**: Prevent breaking changes by constraining to compatible version ranges
- **Security**: Explicitly exclude versions with known vulnerabilities
- **Flexibility**: Use semver constraints familiar from npm, cargo, composer

### Configuration Schema

Constraints are specified in the vendor configuration using a `constraints` field:

```yaml
sources:
  - component: "vpc"
    source: "github.com/cloudposse/terraform-aws-components"
    version: "1.323.0"  # Current pinned version
    constraints:
      version: "^1.0.0"           # Semver constraint (allow 1.x)
      excluded_versions:           # Versions to skip
        - "1.2.3"                  # Has critical bug
        - "1.5.*"                  # Entire patch series broken
      no_prereleases: true         # Skip alpha/beta/rc versions
```

### Constraint Field Definitions

#### `constraints.version` (string, optional)

Semantic version constraint using [Masterminds/semver](https://github.com/Masterminds/semver) syntax:

| Constraint | Meaning | Example |
|------------|---------|---------|
| `^1.2.3` | Compatible with 1.2.3 (allows >=1.2.3 <2.0.0) | 1.2.3, 1.5.0, 1.999.0 ✓<br>2.0.0 ✗ |
| `~1.2.3` | Patch updates only (allows >=1.2.3 <1.3.0) | 1.2.3, 1.2.9 ✓<br>1.3.0 ✗ |
| `>=1.0.0 <2.0.0` | Range constraint | 1.x.x ✓<br>2.0.0 ✗ |
| `1.2.x` | Any patch version in 1.2 | 1.2.0, 1.2.99 ✓<br>1.3.0 ✗ |
| `*` or empty | Any version (no constraint) | All versions ✓ |

If not specified, defaults to no constraint (any version allowed).

#### `constraints.excluded_versions` (list of strings, optional)

List of specific versions to exclude from consideration. Supports:
- **Exact versions**: `"1.2.3"` - exclude this specific version
- **Wildcard patterns**: `"1.5.*"` - exclude all 1.5.x versions
- **Multiple entries**: Can exclude any number of versions

**Use cases:**
- Known security vulnerabilities (CVEs)
- Versions with critical bugs
- Breaking changes or incompatibilities
- Deprecated/yanked versions

#### `constraints.no_prereleases` (boolean, optional, default: false)

When `true`, exclude pre-release versions (alpha, beta, rc, pre, etc.).

**Examples of excluded versions when `true`:**
- `1.0.0-alpha`
- `2.3.0-beta.1`
- `1.5.0-rc.2`
- `3.0.0-pre`

**Production use case:** Ensure only stable releases are used in production environments.

### Constraint Resolution Logic

When `atmos vendor update` checks for updates:

1. **Fetch available versions** from Git repository (tags)
2. **Parse semantic versions** - skip non-semver tags
3. **Apply `no_prereleases` filter** - remove alpha/beta/rc if enabled
4. **Apply `excluded_versions` filter** - remove blacklisted versions
5. **Apply `constraints.version` filter** - keep only versions matching constraint
6. **Select latest version** from remaining candidates
7. **Update `version` field** in YAML while preserving structure

### Example Configurations

#### Example 1: Conservative Updates (Patch Only)
```yaml
sources:
  - component: "vpc"
    source: "github.com/cloudposse/terraform-aws-components"
    version: "1.323.0"
    constraints:
      version: "~1.323.0"  # Only allow 1.323.x patches
      no_prereleases: true
```

#### Example 2: Minor Updates with Exclusions
```yaml
sources:
  - component: "eks"
    source: "github.com/cloudposse/terraform-aws-components"
    version: "2.1.0"
    constraints:
      version: "^2.0.0"       # Allow 2.x minor/patch updates
      excluded_versions:
        - "2.3.0"             # Has CVE-2024-12345
        - "2.5.*"             # Entire 2.5.x series is broken
      no_prereleases: true
```

#### Example 3: Production-Safe Range
```yaml
sources:
  - component: "rds"
    source: "github.com/cloudposse/terraform-aws-components"
    version: "5.0.0"
    constraints:
      version: ">=5.0.0 <6.0.0"  # Allow 5.x only
      no_prereleases: true         # No beta/rc versions
```

#### Example 4: No Constraints (Always Latest)
```yaml
sources:
  - component: "test-module"
    source: "github.com/example/terraform-modules"
    version: "1.0.0"
    # No constraints - will update to absolute latest tag
```

### Schema Changes Required

Update `pkg/schema/schema.go` to add `Constraints` field to `AtmosVendorSource`:

```go
type AtmosVendorSource struct {
    Component        string              `yaml:"component,omitempty" json:"component,omitempty"`
    Source           string              `yaml:"source" json:"source"`
    Version          string              `yaml:"version,omitempty" json:"version,omitempty"`
    Constraints      *VendorConstraints  `yaml:"constraints,omitempty" json:"constraints,omitempty"`
    Targets          []string            `yaml:"targets,omitempty" json:"targets,omitempty"`
    IncludedPaths    []string            `yaml:"included_paths,omitempty" json:"included_paths,omitempty"`
    ExcludedPaths    []string            `yaml:"excluded_paths,omitempty" json:"excluded_paths,omitempty"`
    Tags             []string            `yaml:"tags,omitempty" json:"tags,omitempty"`
}

type VendorConstraints struct {
    Version          string   `yaml:"version,omitempty" json:"version,omitempty"`
    ExcludedVersions []string `yaml:"excluded_versions,omitempty" json:"excluded_versions,omitempty"`
    NoPrereleases    bool     `yaml:"no_prereleases,omitempty" json:"no_prereleases,omitempty"`
}
```

### JSON Schema Update

Update `/pkg/datafetcher/schema/vendor/package/1.0.json` to include constraints:

```json
{
  "constraints": {
    "type": "object",
    "properties": {
      "version": {
        "type": "string",
        "description": "Semantic version constraint (e.g., ^1.0.0, ~1.2.3)"
      },
      "excluded_versions": {
        "type": "array",
        "items": {
          "type": "string"
        },
        "description": "List of versions to exclude (supports wildcards)"
      },
      "no_prereleases": {
        "type": "boolean",
        "default": false,
        "description": "Exclude pre-release versions (alpha, beta, rc)"
      }
    }
  }
}
```

## YAML Structure Preservation Requirements

### Critical Requirements

**MUST preserve:**
1. **YAML Anchors**: `&anchor-name`
2. **YAML Aliases**: `*anchor-name`
3. **Merge Keys**: `<<: *anchor`
4. **Comments**: Both inline (`# comment`) and block comments
5. **Indentation**: Exact whitespace (spaces, not tabs)
6. **Quote Styles**: Single (`'`), double (`"`), or unquoted
7. **Line Ordering**: Maintain order of keys and list items
8. **Empty Lines**: Preserve spacing between sections

### YAML Preservation Strategy

**Use Established YAML Libraries - DO NOT implement custom parser**

**Recommended Approach:**

**Option 1: `gopkg.in/yaml.v3` (Preferred)**
- Industry-standard Go YAML library
- Full support for YAML 1.2
- Preserves comments via `yaml.Node` API
- Handles anchors and aliases correctly
- Used extensively in the Go ecosystem
- Battle-tested and maintained

**Implementation:**
```go
import "gopkg.in/yaml.v3"

// Parse YAML preserving structure
var node yaml.Node
err := yaml.Unmarshal(content, &node)

// Navigate and update version nodes
// node.Content contains the document structure

// Marshal back to YAML with comments preserved
output, err := yaml.Marshal(&node)
```

**Option 2: `goccy/go-yaml` (Alternative)**
- Better comment preservation in some cases
- AST-based approach
- May have limitations with complex anchors

**Implementation Decision:**
- **Use `gopkg.in/yaml.v3` for v1** - proven, stable, well-documented
- **Leverage `yaml.Node` API** for structure-preserving updates
- **Do NOT write custom YAML parser** - use established libraries
- **Test extensively** with real vendor.yaml files including anchors, aliases, merge keys
- **Document any discovered limitations** and file issues upstream if needed

**Key Requirements:**
1. Use `yaml.Node` for low-level node manipulation
2. Preserve `yaml.Node.Style` for quote preservation
3. Preserve `yaml.Node.HeadComment` and `yaml.Node.LineComment`
4. Maintain node ordering
5. Test with complex anchor/alias scenarios

### Test Cases for YAML Preservation

```yaml
# Test 1: Simple version update
spec:
  sources:
    - component: vpc
      version: 1.0.0  # Should update to 2.0.0

# Test 2: Anchors and aliases
spec:
  bases:
    - &defaults
      source: github.com/example
      version: 1.0.0  # Should update anchor definition
  sources:
    - <<: *defaults  # Should inherit updated version
      component: vpc

# Test 3: Comments preservation
spec:
  sources:
    # VPC component from CloudPosse
    - component: vpc
      version: 1.0.0  # Latest stable - Should update

# Test 4: Quote styles
spec:
  sources:
    - version: "1.0.0"  # Double quotes - preserve
    - version: '1.0.0'  # Single quotes - preserve
    - version: 1.0.0    # Unquoted - preserve

# Test 5: Multiple components
spec:
  sources:
    - component: vpc
      version: 1.0.0  # Update to 2.0.0
    - component: s3
      version: 3.0.0  # Update to 3.1.0

# Test 6: Indentation
spec:
  sources:
    - component: vpc
      version: 1.0.0
      targets:
        - target: components/vpc  # Nested - preserve indent
```

## Command Structure

### `atmos vendor update`

```bash
atmos vendor update [flags]
```

**Flags:**
- `--check` - Dry-run mode, show what would be updated without making changes
- `--pull` - Update version references AND pull the new components
- `--component <name>` / `-c <name>` - Update specific component only
- `--tags <tags>` - Update components with specific tags (comma-separated)
- `--type <type>` / `-t <type>` - Component type: `terraform` or `helmfile` (default: `terraform`)
- `--outdated` - Show only components with available updates (combined with `--check`)

**Examples:**
```bash
# Check for updates
atmos vendor update --check

# Update all components
atmos vendor update

# Update and pull
atmos vendor update --pull

# Update specific component
atmos vendor update --component vpc

# Update by tags
atmos vendor update --tags terraform,aws

# Show only outdated
atmos vendor update --check --outdated
```

### `atmos vendor diff`

Shows Git diffs between two versions of a vendored component from the remote repository.

```bash
atmos vendor diff [flags]
```

**Flags:**
- `--component <name>` / `-c <name>` - Component to diff (required)
- `--from <version>` - Starting version/tag/commit (defaults to current version in vendor.yaml)
- `--to <version>` - Ending version/tag/commit (defaults to latest)
- `--file <path>` - Show diff for specific file within component
- `--context <n>` - Number of context lines (default: 3)
- `--unified` - Show unified diff format (default: true)
- `--no-color` - Disable color output (overrides auto-detection)

**Examples:**
```bash
# Show diff between current version and latest (colorized if TTY)
atmos vendor diff --component vpc

# Show diff between two specific versions
atmos vendor diff --component vpc --from 1.0.0 --to 2.0.0

# Show diff for a specific file
atmos vendor diff --component vpc --from 1.0.0 --to 2.0.0 --file main.tf

# Show diff between current and a specific commit
atmos vendor diff --component vpc --to abc1234

# Disable colors (for piping or scripts)
atmos vendor diff --component vpc --no-color

# Pipe to file (colors auto-disabled)
atmos vendor diff --component vpc > changes.diff
```

**How It Works:**
1. Read vendor configuration to get component source URL
2. Determine versions to compare (from vendor.yaml or flags)
3. Use `git diff` with remote refs to show changes
4. Apply color formatting based on output context

**Color Handling:**
Diff output is colorized **automatically** when:
- ✅ Output is to a terminal (TTY detected via `isatty()`)
- ✅ Terminal is not `TERM=dumb`
- ✅ `--no-color` flag is not set
- ✅ Global `--no-color` flag is not set (from root command)

Diff output is **NOT** colorized when:
- ❌ Output is being piped (`| less`, `> file.diff`)
- ❌ `--no-color` flag is explicitly set
- ❌ `TERM=dumb` environment variable
- ❌ Not a TTY (scripting, CI/CD)

**Implementation:**
```go
func shouldColorize(cmd *cobra.Command) bool {
    // Check --no-color flag (command-specific or global)
    if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
        return false
    }

    // Check if stdout is a terminal
    if !isatty.IsTerminal(os.Stdout.Fd()) {
        return false
    }

    // Check for TERM=dumb
    if os.Getenv("TERM") == "dumb" {
        return false
    }

    return true
}
```

**Scope:**
- **GitHub repositories only** - leverages GitHub's native compare API
- For other Git sources (GitLab, Bitbucket, self-hosted), returns "not implemented"
- Not applicable to OCI registries, local files, or HTTP sources
- Shows actual code/file changes between Git refs
- No local clone required - uses provider's diff capabilities

**Technical Implementation:**
```bash
# For GitHub sources, uses GitHub Compare API:
# https://api.github.com/repos/{owner}/{repo}/compare/{from}...{to}

# For non-GitHub Git sources, a temporary bare repository workflow would be used:
# 1. Create temporary bare repository
# 2. Fetch both refs into the temporary repo
# 3. Run git diff between the two refs
# Example:
tmpdir=$(mktemp -d)
git init --bare "$tmpdir"
git -C "$tmpdir" fetch <remote-url> <from-ref>:<from-ref> <to-ref>:<to-ref>
git -C "$tmpdir" diff <from-ref> <to-ref> [-- <file-path>]
rm -rf "$tmpdir"
```

**Output Format:**
```diff
Showing diff for component 'vpc' (1.0.0 → 2.0.0)
Source: github.com/cloudposse/terraform-aws-vpc.git

diff --git a/main.tf b/main.tf
index abc1234..def5678 100644
--- a/main.tf
+++ b/main.tf
@@ -10,7 +10,7 @@ resource "aws_vpc" "default" {
   cidr_block = var.cidr_block
-  enable_dns_support = true
+  enable_dns_support = var.enable_dns_support
   enable_dns_hostnames = true

   tags = merge(
```

## Configuration File Support

### vendor.yaml Format

```yaml
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example-vendor-config
  description: Vendor configuration
spec:
  # Import other vendor configs
  imports:
    - vendor/terraform.yaml
    - vendor/helmfile.yaml

  # Vendor sources
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc.git
      version: 1.323.0  # Will be updated
      targets:
        - components/terraform/vpc
```

### component.yaml Format

```yaml
# components/terraform/vpc/component.yaml
version: 1.323.0  # Will be updated
source: github.com/cloudposse/terraform-aws-vpc.git
targets:
  - .
```

### Version Specification Format

Atmos vendor configurations support multiple version specification formats:

**1. Semantic Versions (Tags)**
```yaml
version: 1.323.0          # Specific semver tag
version: v1.323.0         # With 'v' prefix (normalized)
version: 2.0.0-beta.1     # Pre-release version
```

**2. Git Commit Hashes**
```yaml
version: abc1234          # Short hash (7+ chars)
version: abc1234567890    # Full hash
```

**3. Git Branches**
```yaml
version: main             # Branch name
version: develop          # Branch name
```

**4. Templated Versions (Skipped)**
```yaml
version: "{{.Version}}"             # Template syntax - skipped
version: "{{ atmos.Component }}"    # Template syntax - skipped
```

**Version Detection Logic:**
1. If version contains `{{` or `}}` → Skip (templated)
2. If version matches semver pattern → Compare as semantic version
3. If version is 7-40 hex chars → Treat as Git commit hash
4. Otherwise → Treat as Git branch/tag name

**Semantic Version Comparison:**
- Uses [Masterminds/semver](https://github.com/Masterminds/semver) library
- Supports standard semver format: `MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]`
- Normalizes 'v' prefix: `v1.0.0` → `1.0.0`
- Pre-release versions (alpha, beta, rc) are filtered by default
- Invalid semver falls back to string comparison

**Examples:**
```yaml
# These are considered newer than 1.0.0:
version: 1.0.1          # Patch bump
version: 1.1.0          # Minor bump
version: 2.0.0          # Major bump

# These are NOT considered newer (pre-release):
version: 1.0.0-alpha.1  # Filtered out by default
version: 1.0.0-beta     # Filtered out by default
version: 1.0.0-rc.1     # Filtered out by default

# Commit hashes - always considered "newer" if different:
version: abc1234        # Different hash = update available
```

### Import Chain Processing

**Requirement:** Must track which file defines each source to update the correct file.

**Example:**
```
vendor.yaml
  imports:
    - vendor/terraform.yaml
    - vendor/helmfile.yaml

vendor/terraform.yaml
  sources:
    - component: vpc
      version: 1.0.0  # Defined here

vendor/helmfile.yaml
  sources:
    - component: app
      version: 2.0.0  # Defined here
```

**Expected behavior:**
- When updating `vpc`, modify `vendor/terraform.yaml`
- When updating `app`, modify `vendor/helmfile.yaml`
- Track source file mapping during import processing

**Implementation:**
```go
type SourceFileMapping struct {
    Component     string
    SourceFile    string  // File where source was defined
    LineNumber    int     // Optional: for precise updates
}
```

## GitHub Rate Limit Handling

### Requirements

1. **Detect rate limits** from `git ls-remote` errors
2. **Show clear messages** to users when rate limited
3. **Suggest solutions**: Use SSH authentication, wait, or use GitHub token
4. **Don't fail completely** - show what was checked before hitting limit

### Rate Limit Scenarios

**Unauthenticated HTTPS:**
- 60 requests per hour per IP
- Most restrictive

**Authenticated HTTPS (with token):**
- 5,000 requests per hour
- Recommended approach

**SSH with keys:**
- No rate limits from GitHub API
- Requires SSH setup

### Error Handling

```bash
# Rate limit error message
⚠ GitHub rate limit exceeded after checking 45/100 components

Checked successfully:
✓ vpc (1.0.0 → 2.0.0)
✓ s3 (3.0.0 - up to date)
...

Remaining: 55 components not checked

To avoid rate limits:
1. Use SSH authentication: git@github.com:owner/repo.git
2. Set GITHUB_TOKEN environment variable
3. Wait 37 minutes for rate limit reset
```

## Terminal UI (TUI) Design

### Progress Indicator

Use existing Charm Bracelet (`bubbletea`) TUI pattern from `vendor pull`:

```go
type modelVendor struct {
    packages    []pkgVendor
    index       int
    done        bool
    spinner     spinner.Model
    progress    progress.Model
    width       int
    height      int
    dryRun      bool
    atmosConfig *schema.AtmosConfiguration
    isTTY       bool
}
```

### TUI States

**1. Checking State:**
```
Checking for vendor updates...

[=========>          ] 45/100 Checking terraform-aws-ecs...
```

**2. Results State:**
```
✓ terraform-aws-vpc (1.323.0 → 1.372.0)
✓ terraform-aws-s3-bucket (4.1.0 → 4.2.0)
✓ terraform-aws-eks (2.1.0 - up to date)
⚠ custom-module (skipped - templated version)
```

**3. Updating State (when not using --check):**
```
Updating version references...

[=========>          ] 2/3 Updating terraform-aws-s3-bucket...
```

**4. Pulling State (when using --pull):**
```
Pulling updated components...

[=========>          ] 2/3 Pulling terraform-aws-s3-bucket...
```

### Status Icons

- `✓` - Update available or operation succeeded
- `⚠` - Skipped (templated, unsupported source type)
- `✗` - Error occurred
- `—` - Up to date (no update available)

## Technical Architecture

### Package Structure

```
cmd/
  vendor_update.go         # Cobra command definition for update
  vendor_diff.go           # Cobra command definition for diff

internal/exec/
  vendor_update.go         # Update command execution logic
  vendor_diff.go           # Diff command execution logic
  vendor_version_check.go  # Version checking via git ls-remote
  vendor_yaml_updater.go   # YAML update with preservation
  vendor_filter.go         # Component/tag filtering
  vendor_git_diff.go       # Git diff operations
  vendor_model.go          # TUI model (shared with pull)
  vendor_model_helpers.go  # TUI helper functions
  vendor_interfaces.go     # Interfaces for testability

errors/
  errors.go
    - ErrVendorConfigNotFound
    - ErrVersionCheckingNotSupported
    - ErrNoValidCommitsFound
    - ErrNoTagsFound
    - ErrNoStableReleaseTags
    - ErrCheckingForUpdates
    - ErrComponentNotFound
    - ErrGitDiffFailed
    - ErrInvalidGitRef
```

### Core Functions

```go
// Vendor Update
func ExecuteVendorUpdateCmd(cmd *cobra.Command, args []string) error

// Vendor Diff
func ExecuteVendorDiffCmd(cmd *cobra.Command, args []string) error
func GetGitDiff(source *schema.AtmosVendorSource, fromRef, toRef string) (string, error)
func GetGitDiffForFile(source *schema.AtmosVendorSource, fromRef, toRef, filePath string) (string, error)
func ColorizeGitDiff(diff string, isTTY bool) string

// Version checking
func CheckForVendorUpdates(source *schema.AtmosVendorSource) (hasUpdate bool, latestVersion string, err error)
func GetLatestGitTag(repoURL string) (string, error)
func GetLatestGitCommit(repoURL string) (string, error)
func CompareVersions(current, latest string) (isNewer bool, err error)

// YAML updating using gopkg.in/yaml.v3
type YAMLVersionUpdater interface {
    UpdateVersionsInFile(filePath string, updates map[string]string) error
    UpdateVersionsInContent(content []byte, updates map[string]string) ([]byte, error)
}

// Implementation using yaml.Node API from gopkg.in/yaml.v3
type YAMLNodeVersionUpdater struct{}

// Key methods for node traversal and updates
func (u *YAMLNodeVersionUpdater) findComponentNodes(node *yaml.Node) map[string][]*yaml.Node
func (u *YAMLNodeVersionUpdater) updateVersionNode(node *yaml.Node, newVersion string)
func (u *YAMLNodeVersionUpdater) preserveNodeStyle(node *yaml.Node) // Preserve quotes, etc.

// Filtering
func FilterSources(sources []schema.AtmosVendorSource, component string, tags []string) []schema.AtmosVendorSource

// Import processing with file tracking
func ProcessVendorImportsWithFileTracking(
    atmosConfig *schema.AtmosConfiguration,
    vendorConfigFile string,
    imports []string,
    sources []schema.AtmosVendorSource,
    visitedFiles []string,
) ([]schema.AtmosVendorSource, map[string]string, error)
```

### Data Flow

```
1. Parse CLI flags
   ↓
2. Initialize Atmos config
   ↓
3. Read vendor.yaml (or component.yaml)
   ↓
4. Process imports recursively → Build source-to-file mapping
   ↓
5. Filter sources by component/tags
   ↓
6. For each source:
   - Check if version is templated → Skip
   - Determine source type (Git, OCI, etc.)
   - If Git: Call git ls-remote
   - Compare versions
   - Record updates available
   ↓
7. Display results (TUI)
   ↓
8. If not --check:
   - Group updates by file
   - For each file:
     - Load content
     - Update versions (preserve YAML)
     - Write back
   ↓
9. If --pull:
   - Execute vendor pull command
```

## Testing Strategy

### Unit Tests (80-90% coverage target)

**Version Checking Tests:**
```go
func TestCheckForVendorUpdates(t *testing.T)
func TestGetLatestGitTag(t *testing.T)
func TestGetLatestGitCommit(t *testing.T)
func TestCompareVersions(t *testing.T)
func TestIsTemplatedVersion(t *testing.T)
```

**YAML Preservation Tests:**
```go
func TestYAMLNodeVersionUpdater_PreserveComments(t *testing.T)
func TestYAMLNodeVersionUpdater_PreserveAnchors(t *testing.T)
func TestYAMLNodeVersionUpdater_PreserveAliases(t *testing.T)
func TestYAMLNodeVersionUpdater_PreserveMergeKeys(t *testing.T)
func TestYAMLNodeVersionUpdater_PreserveQuotes(t *testing.T)
func TestYAMLNodeVersionUpdater_PreserveIndentation(t *testing.T)
func TestYAMLNodeVersionUpdater_MultipleComponents(t *testing.T)
func TestYAMLNodeVersionUpdater_ComplexAnchors(t *testing.T)
```

**Import Processing Tests:**
```go
func TestProcessVendorImportsWithFileTracking(t *testing.T)
func TestSourceFileMapping(t *testing.T)
func TestCircularImportDetection(t *testing.T)
```

**Filtering Tests:**
```go
func TestFilterSources_ByComponent(t *testing.T)
func TestFilterSources_ByTags(t *testing.T)
func TestFilterSources_Combined(t *testing.T)
```

### Integration Tests

```go
func TestVendorUpdate_EndToEnd(t *testing.T)
func TestVendorUpdate_WithImports(t *testing.T)
func TestVendorUpdate_WithPull(t *testing.T)
func TestVendorUpdate_GitHubRateLimit(t *testing.T)
```

### Test Fixtures

```
tests/test-cases/vendor-update/
  vendor.yaml               # Main config
  vendor/terraform.yaml     # Imported config
  vendor/helmfile.yaml      # Imported config
  component.yaml            # Component-level config
  expected/
    vendor-updated.yaml     # Expected result after update
```

### Mock Strategy

**Mock Git operations for unit tests:**
```go
type GitVersionChecker interface {
    GetLatestTag(repoURL string) (string, error)
    GetLatestCommit(repoURL string) (string, error)
}

// Use gomock for mocking
//go:generate mockgen -source=vendor_version_check.go -destination=mock_version_check_test.go
```

**Use real git ls-remote for integration tests:**
- Test against public repos (cloudposse/terraform-aws-vpc)
- Skip if GitHub rate limit hit
- Use test preconditions pattern

## Error Handling

### Error Types

```go
// In errors/errors.go
var (
    ErrVendorConfigNotFound             = errors.New("vendor config file not found")
    ErrVersionCheckingNotSupported      = errors.New("version checking not supported for this source type")
    ErrNoValidCommitsFound              = errors.New("no valid commits found")
    ErrNoTagsFound                      = errors.New("no tags found in repository")
    ErrNoStableReleaseTags              = errors.New("no stable release tags found")
    ErrCheckingForUpdates               = errors.New("error checking for updates")
    ErrGitHubRateLimitExceeded          = errors.New("GitHub API rate limit exceeded")
    ErrInvalidGitLsRemoteOutput         = errors.New("invalid git ls-remote output")
)
```

### Error Scenarios

| Scenario | Error | User Message | Recovery |
|----------|-------|--------------|----------|
| No vendor.yaml | `ErrVendorConfigNotFound` | "Vendor config file not found: vendor.yaml" | Check file path |
| Git ls-remote fails | `ErrCheckingForUpdates` | "Failed to check updates for vpc: connection timeout" | Check network, try again |
| No tags in repo | `ErrNoTagsFound` | "No version tags found in repository" | Use commit hash versioning |
| Rate limit hit | `ErrGitHubRateLimitExceeded` | "GitHub rate limit exceeded. Use SSH or set GITHUB_TOKEN." | Wait or authenticate |
| Invalid version format | `errUtils.ErrInvalidVersion` | "Invalid version format: 'abc'. Expected semver or commit hash." | Fix version string |
| Templated version | (skip silently) | "⚠ vpc (skipped - templated version)" | No action needed |
| YAML parse error | `yaml.ParseError` | "Failed to parse vendor.yaml: line 5, invalid syntax" | Fix YAML syntax |

## Documentation Requirements

### 1. CLI Documentation (Docusaurus)

**File:** `website/docs/cli/commands/vendor/vendor-update.mdx`

**Sections:**
- Purpose note
- Usage syntax
- Examples (all flag combinations)
- Arguments (none)
- Flags (detailed descriptions)
- Supported upstream sources table
- How it works (step-by-step)
- YAML preservation explanation
- Troubleshooting

**File:** `website/docs/cli/commands/vendor/vendor-diff.mdx`

**Sections:**
- Purpose note (distinct command for viewing Git diffs between component versions)
- Usage syntax with command-specific flags (--from-version, --to-version, --context-lines, --output)
- Examples showing diff output between vendor versions
- See also: link to vendor-update.mdx for related version management context

### 2. Blog Post

**File:** `website/blog/YYYY-MM-DD-vendor-update-command.md`

**Frontmatter:**
```yaml
---
slug: vendor-update-command
title: "Introducing atmos vendor update: Automated Component Version Management"
authors: [atmos]
tags: [feature, terraform, helmfile, vendor, automation]
---
```

**Sections:**
- Introduction: The problem of manual version management
- What's New: Overview of vendor update command
- How It Works: Step-by-step with examples
- YAML Structure Preservation: Why this matters
- Supported Sources: What works today, what's coming
- Examples: Real-world use cases
- Get Started: Try it today
- Roadmap: Future enhancements

### 3. Usage Examples

**File:** `cmd/markdown/atmos_vendor_update_usage.md`

```markdown
- Check for updates without making changes

\`\`\`bash
atmos vendor update --check
\`\`\`

- Update version references in configuration files

\`\`\`bash
atmos vendor update
\`\`\`

- Update and pull new components in one command

\`\`\`bash
atmos vendor update --pull
\`\`\`

- Update specific component

\`\`\`bash
atmos vendor update --component vpc
\`\`\`

- Update components with specific tags

\`\`\`bash
atmos vendor update --tags terraform,networking
\`\`\`

- Show only components with available updates

\`\`\`bash
atmos vendor update --check --outdated
\`\`\`
```

## Implementation Phases

### Phase 1: Core Functionality (v1 - MVP)
**Goal:** Complete vendor management with update and diff commands

**Vendor Update:**
- [ ] Command structure and flags
- [ ] Git repository version checking (tags and commits)
- [ ] YAML updater using `gopkg.in/yaml.v3` (preserves comments, anchors, formatting)
- [ ] Import chain processing with file tracking
- [ ] Component/tag filtering
- [ ] TUI progress indicators
- [ ] Dry-run mode (--check)
- [ ] Update mode (modify files)
- [ ] Pull mode (--pull)
- [ ] Semantic version comparison with `Masterminds/semver`

**Vendor Diff:**
- [ ] Command structure and flags
- [ ] Git diff between remote refs (no local clone needed)
- [ ] Version to Git ref resolution
- [ ] File-specific diff support
- [ ] Diff colorization with TTY detection
- [ ] Context lines configuration

**Shared:**
- [ ] Error handling for all scenarios
- [ ] Unit tests (80-90% coverage)
- [ ] Integration tests
- [ ] CLI documentation for both commands
- [ ] Blog post

**Deliverables:**
- Working `atmos vendor update` command with all flags
- Working `atmos vendor diff` command with all flags
- Full documentation for both commands
- Blog post announcement

### Phase 2: Enhanced Features (v1.1)
**Goal:** Improved user experience and edge case handling

- [ ] SSH to HTTPS fallback for Git operations
- [ ] GitHub API integration for better rate limit handling
- [ ] Pre-release tag filtering options (--include-prerelease flag)
- [ ] Enhanced error messages with recovery suggestions
- [ ] Diff format options (side-by-side, stats)
- [ ] More test coverage (90%)

### Phase 3: Additional Sources (v2.0)
**Goal:** Support more source types

- [ ] OCI registry support
- [ ] S3 bucket support
- [ ] GCS bucket support
- [ ] GitLab-specific optimizations
- [ ] Bitbucket-specific optimizations

### Phase 4: Advanced Features (v2.1+)
**Goal:** Power user features

- [ ] Semantic version ranges (`~> 1.2.0`)
- [ ] Lock file generation
- [ ] Update policies (major/minor/patch)
- [ ] Rollback capabilities
- [ ] Breaking change detection
- [ ] Webhook notifications

## Success Metrics

### Functional Metrics
- ✅ Zero YAML corruption (anchors, comments preserved)
- ✅ Version checking accuracy > 99%
- ✅ 80-90% test coverage
- ✅ All supported source types work correctly

### Performance Metrics
- Version check: < 2 seconds per component (network dependent)
- YAML update: < 100ms per file (local operation)
- Full workflow: < 30 seconds for 50 components

### User Experience Metrics
- Clear progress indicators during operation
- Helpful error messages with recovery steps
- Intuitive command structure
- Comprehensive documentation

## Security Considerations

1. **No Credentials in Configs**: Never store tokens in YAML files
2. **Use Existing Auth**: Leverage SSH keys and git credentials
3. **No Auto Major Bumps**: Require user confirmation for major versions
4. **Audit Trail**: All updates tracked via Git commits
5. **Input Validation**: Validate all version strings and URLs
6. **No Command Injection**: Sanitize all inputs to git commands

## Backward Compatibility

1. **Existing Configs**: All existing vendor.yaml files work unchanged
2. **Templated Versions**: Automatically skipped, no errors
3. **Import Chains**: Fully supported
4. **vendor pull**: No changes to existing pull command
5. **vendor diff**: Implemented for GitHub sources, displays file-level diffs between component versions using GitHub's Compare API; returns "not implemented" for non-GitHub sources

## Migration Path

### For Users Currently Running vendor pull Manually

**Before:**
```bash
# 1. Check GitHub for new releases manually
# 2. Edit vendor.yaml by hand
# 3. Run vendor pull
atmos vendor pull
```

**After:**
```bash
# One command does it all
atmos vendor update --pull
```

### For Users Wanting to Check for Updates

**Use:**
```bash
atmos vendor update --check  # Check what updates are available
```

**Note:**
- `atmos vendor diff` shows Git code changes between versions (Git-only feature)
- For checking if updates are available, use `atmos vendor update --check`
- For viewing code changes between versions, use `atmos vendor diff --component <name>`

## Open Questions & Decisions

### Q1: What to do with complex YAML anchors the simple updater can't handle?

**Decision:**
- Use simple updater for v1 (handles 95% of cases)
- Document limitations for complex anchor structures
- Add AST updater in Phase 2 if user feedback requires it
- Test extensively with real-world vendor.yaml files

### Q2: Should we auto-update patch versions but require confirmation for major?

**Decision:**
- v1: All updates require explicit action (`atmos vendor update`)
- v2: Add update policies (auto-patch, manual-major)
- Keep it simple for initial release

### Q3: How to handle version pinning with folder names?

**Example:**
```yaml
source: github.com/example/repo.git
version: 1.0.0
targets:
  - components/terraform/vpc-1.0.0  # Folder name includes version
```

**Decision:**
- Out of scope for v1 (complexity)
- Users manually rename folders if needed
- Consider for v2 with folder template support
- Document this limitation

### Q4: Should --check and regular update show the same output?

**Decision:**
- Yes, same output format
- `--check` shows what would be updated
- Regular update shows what was updated
- Use same TUI, different past/future tense messages

### Q5: What about components with no version field?

**Decision:**
- Skip components without explicit version field
- Show warning: "⚠ vpc (skipped - no version field)"
- Don't add version field automatically
- User must add version: field manually if they want updates

## Implementation Checklist

### Code

**Vendor Update:**
- [ ] Create `cmd/vendor_update.go` with command structure
- [ ] Create `internal/exec/vendor_update.go` with main logic
- [ ] Create `internal/exec/vendor_version_check.go` for Git operations using `git ls-remote`
- [ ] Create `internal/exec/vendor_yaml_updater.go` using `gopkg.in/yaml.v3` with `yaml.Node` API
- [ ] Create `internal/exec/vendor_filter.go` for component/tag filtering

**Vendor Diff:**
- [ ] Create `cmd/vendor_diff.go` with command structure
- [ ] Create `internal/exec/vendor_diff.go` with main logic
- [ ] Create `internal/exec/vendor_git_diff.go` for Git diff operations

**Shared:**
- [ ] Add error types to `errors/errors.go` (update and diff errors)
- [ ] Update `internal/exec/vendor_model.go` to support update operations
- [ ] Add helper functions to `internal/exec/vendor_model_helpers.go`
- [ ] Add `Masterminds/semver` dependency for version comparison

**Key Implementation Requirements:**
- Use `gopkg.in/yaml.v3` for YAML parsing (DO NOT write custom parser)
- Use `yaml.Node` API for structure-preserving updates
- Preserve `yaml.Node.Style`, `yaml.Node.HeadComment`, `yaml.Node.LineComment`
- Use `Masterminds/semver` library for semantic version comparison
- Use `git diff` command for comparing remote refs
- Use `mattn/go-isatty` or equivalent for TTY detection
- Respect `--no-color` flag (command-specific and global)
- Auto-disable colors when piping (TTY check)
- Auto-disable colors when `TERM=dumb`
- Colorize diff output only when appropriate (see color handling rules)

### Tests

**Vendor Update Tests:**
- [ ] Unit tests for version checking
- [ ] Unit tests for YAML preservation (all node types)
- [ ] Unit tests for import processing
- [ ] Unit tests for filtering
- [ ] Integration tests for update workflows

**Vendor Diff Tests:**
- [ ] Unit tests for Git diff operations
- [ ] Unit tests for ref resolution (version → Git ref)
- [ ] Unit tests for diff colorization
- [ ] Integration tests for diff workflows

**Shared:**
- [ ] Test fixtures in `tests/test-cases/vendor-update/`
- [ ] Achieve 80-90% test coverage

### Documentation

**Vendor Update:**
- [ ] CLI docs: `website/docs/cli/commands/vendor/vendor-update.mdx`
- [ ] Usage markdown: `cmd/markdown/atmos_vendor_update_usage.md`

**Vendor Diff:**
- [ ] CLI docs: `website/docs/cli/commands/vendor/vendor-diff.mdx`
- [ ] Usage markdown: `cmd/markdown/atmos_vendor_diff_usage.md`

**Shared:**
- [ ] Blog post: `website/blog/YYYY-MM-DD-vendor-management-commands.md`
- [ ] Update existing vendor docs to mention new commands
- [ ] Build website to verify no broken links

### Release
- [ ] Follow PR template (what/why/references)
- [ ] Label PR as `minor` (new feature)
- [ ] Ensure blog post is included (required for minor releases)
- [ ] All CI checks pass (tests, lint, coverage)
- [ ] Get approval from maintainers
- [ ] Merge to main

## Conclusion

This PRD defines two complementary commands that together provide comprehensive vendor management:

1. **`atmos vendor update`** - Automates version checking and YAML updates while preserving file integrity
2. **`atmos vendor diff`** - Shows actual code changes between versions to assess impact

By implementing both commands in Phase 1, users get a complete workflow:
- Check what updates are available (`atmos vendor update --check`)
- Review what changed between versions (`atmos vendor diff --component vpc`)
- Apply updates with confidence (`atmos vendor update`)

**Key Technical Decisions:**
- Use `gopkg.in/yaml.v3` for YAML parsing (DO NOT write custom parser)
- Use `yaml.Node` API for structure preservation
- Use `Masterminds/semver` for version comparison
- Use `git diff` with remote refs for showing changes
- Git repositories only (not OCI, local, or HTTP sources)

The comprehensive testing strategy (80-90% coverage) and documentation requirements ensure high quality and excellent user experience from day one. By focusing exclusively on Git repository support, we deliver a focused, robust solution for the primary use case.
