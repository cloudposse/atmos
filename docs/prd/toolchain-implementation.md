# PRD: Atmos Toolchain for Third-Party Tool Management

## Overview

The Atmos Toolchain provides a unified CLI tool management system that allows Atmos to manage external dependencies (Terraform, OpenTofu, Helm, etc.) with version pinning, automatic installation, and environment isolation.

## Status: In Development

**Current Version**: v0.1 (Initial Implementation)
**Last Updated**: 2025-10-23

---

## Problem Statement

Infrastructure-as-Code teams need to:
1. Pin specific versions of tools (Terraform, OpenTofu, Helm, etc.) per component/stack
2. Ensure consistent tool versions across team members and CI/CD
3. Avoid global tool installations that cause version conflicts
4. Automatically install missing tools without manual intervention
5. Support multiple versions of the same tool concurrently
6. Manage Atmos itself as a versioned dependency

### Current Pain Points

- Manual tool installation and version management
- Version drift between developers and CI/CD
- Global tool installations conflicting with project requirements
- No standard way to declare tool dependencies in IaC configurations
- Complex setup procedures for new team members

---

## Goals

### Primary Goals

1. **Automatic Tool Installation**: Automatically install tools when needed based on declared versions
2. **Version Isolation**: Support multiple concurrent versions of the same tool
3. **Self-Managed Atmos**: Atmos can manage its own version and auto-upgrade/downgrade
4. **Component Dependencies**: Components can declare their tool dependencies
5. **Stack Dependencies**: Stacks can override tool versions per environment
6. **Registry Abstraction**: Support multiple tool registries (Aqua, local overrides)

### Secondary Goals

1. **Offline Support**: Cache downloaded binaries for offline use
2. **Performance**: Fast tool resolution and execution
3. **Cross-Platform**: Work identically on Linux, macOS, and Windows
4. **Security**: Verify checksums and signatures where available
5. **Developer Experience**: Minimal configuration, maximum automation

---

## Solution Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         Atmos CLI                                │
├─────────────────────────────────────────────────────────────────┤
│  Toolchain Layer                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ Version      │  │ Installer    │  │ Resolver     │          │
│  │ Manager      │  │ Engine       │  │ Engine       │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
├─────────────────────────────────────────────────────────────────┤
│  Registry Layer                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ Aliases      │  │ Aqua Registry│  │ GitHub API   │          │
│  │ (atmos.yaml) │  │ (Remote)     │  │ (Fallback)   │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
├─────────────────────────────────────────────────────────────────┤
│  Storage Layer                                                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ .tool-       │  │ .tools/      │  │ Cache        │          │
│  │ versions     │  │ owner/repo/  │  │ Directory    │          │
│  │ (asdf compat)│  │ version/     │  │              │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

### File Structure

```
project/
├── .tool-versions              # asdf-compatible version declarations
├── .tools/                     # Installed binaries
│   └── owner/
│       └── repo/
│           ├── 1.0.0/
│           │   └── binary
│           ├── 1.1.0/
│           │   └── binary
│           └── latest          # Pointer to default version
├── atmos.yaml                  # Main Atmos config (includes toolchain.aliases)
└── stacks/
    └── catalog/
        └── terraform/
            └── vpc/
                └── component.yaml  # Component dependencies (future)
```

---

## Current Implementation Status

### ✅ Implemented Features

#### 1. Core Toolchain Commands

All commands implemented in `cmd/toolchain/`:

- **`toolchain install [tool@version]`** - Install tools from registry
- **`toolchain uninstall [tool@version]`** - Remove installed tools
- **`toolchain list`** - Show configured tools and installation status
- **`toolchain add <tool@version>`** - Add tool to .tool-versions
- **`toolchain remove <tool[@version]>`** - Remove from .tool-versions
- **`toolchain set <tool> [version]`** - Set default version (interactive)
- **`toolchain get [tool]`** - Show version information
- **`toolchain info <tool>`** - Display tool metadata
- **`toolchain exec <tool@version>`** - Execute tool with specific version
- **`toolchain path`** - Print PATH entries for installed tools
- **`toolchain which <tool>`** - Show path to installed binary
- **`toolchain clean`** - Clean tools and cache directories

#### 2. Registry Support

**Current Status**: Hard-coded Aqua registry URLs with local override support

Implemented in `toolchain/aqua_registry.go`:
- Queries Aqua registry at `https://raw.githubusercontent.com/aquaproj/aqua-registry/refs/heads/main/pkgs`
- Falls back to multiple registry paths for common tools
- Caches registry metadata in temp directory

**Limitation**: Registry URLs are hard-coded, not configurable

#### 3. Alias Resolution

Implemented in `toolchain/installer.go` (`DefaultToolResolver`):

Aliases are configured in `atmos.yaml` under `toolchain.aliases`:

```yaml
# atmos.yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
    tf: hashicorp/terraform
    opentofu: opentofu/opentofu
    tofu: opentofu/opentofu
    helm: helm/helm
```

**Features**:
- Simple name-to-owner/repo mappings
- Resolved before registry lookups
- Works with all registry types (Aqua, inline, custom)
- Multiple aliases can point to same tool

**Resolution Order**:
1. Check if tool name is an alias in `atmos.yaml`
2. If aliased, replace with owner/repo value
3. If already in owner/repo format, use directly
4. Otherwise, search Aqua registry

#### 4. .tool-versions File Support

Implemented in `toolchain/tool_versions.go`:

**asdf-compatible format**:
```
terraform 1.13.1 1.11.4
opentofu 1.10.0
helm 3.12.0
```

**Features**:
- Multiple versions per tool (first is default)
- Automatic detection and loading
- Add/remove/set operations
- Duplicate prevention

#### 5. Version Resolution & Installation

Implemented in `toolchain/installer.go`:

**Resolution Chain**:
1. Check if tool name is an alias in `atmos.yaml`
2. Check `.tool-versions` for pinned version
3. Query configured registries (Aqua, inline, custom) for tool metadata
4. Fall back to GitHub API for version discovery

**Installation**:
- Download from GitHub releases or custom URLs
- Extract archives (tar.gz, zip, gzip, raw binaries)
- Install to `.tools/owner/repo/version/`
- Create `latest` pointer file
- PATH bomb protection (validates paths during extraction)
- Decompression size limits (prevents zip bombs)

#### 6. HTTP Client with GitHub Authentication

Implemented in `toolchain/http_client.go`:

**Features**:
- Automatic GitHub token injection from `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN`
- Rate limit handling
- Custom user agent
- Request retry logic

#### 7. Tool Execution (`exec` command)

Implemented in `toolchain/exec.go`:

**Features**:
- Auto-install if tool not present
- Process replacement via `syscall.Exec`
- Version-specific execution
- Environment passthrough

#### 8. Atmos Self-Management

**Status**: ✅ **Configured in tools.yaml**

The Atmos tool is defined in the default `tools.yaml`:
```yaml
cloudposse/atmos:
  type: github_release
  repo_owner: cloudposse
  repo_name: atmos
  binary_name: atmos
```

**How It Should Work** (Implementation Status: 🚧 **Partially Implemented**):

1. User runs `atmos` command
2. Atmos checks `.tool-versions` for required version
3. If current version doesn't match:
   - Install required version to `.tools/cloudposse/atmos/X.Y.Z/`
   - Execute via `syscall.Exec` to replace current process
4. Required version executes the actual command

**Current Gap**: The self-exec wrapper logic is not implemented in the main `atmos` binary entry point. The toolchain has all the pieces (`exec`, `install`, `version resolution`) but they're not wired up to the main CLI entry.

---

### ❌ Not Implemented

#### 1. Component/Stack Tool Dependencies

**Status**: 🚧 **Not Implemented**

**Design Decision**: Tool dependencies MUST be at the **top level** (not in `metadata`) to support inheritance.

### Configuration Structure

#### Stack-Level Dependencies - Global Tool Requirements

**Stack Catalog** (`stacks/catalog/base.yaml`):
```yaml
# Stack-level dependencies apply to ALL components in this stack
dependencies:
  tools:
    terraform: "~> 1.10.0"     # SemVer constraint
    tflint: "^0.54.0"          # Must satisfy constraint
    tfsec: "latest"            # Always use latest

components:
  terraform:
    vpc:
      vars:
        name: vpc
```

**Stack Configuration** (`stacks/orgs/acme/prod/us-east-1.yaml`):
```yaml
import:
  - catalog/base    # Inherits stack-level tool dependencies

# Stack-level override (applies to all components)
dependencies:
  tools:
    terraform: "1.10.3"  # Satisfies ~> 1.10.0 from catalog
    # tflint inherited from catalog: ^0.54.0
    # tfsec inherited from catalog: latest

components:
  terraform:
    vpc:
      vars:
        environment: prod
```

#### Component-Level Dependencies - Component-Specific Requirements

**Component Catalog** (`stacks/catalog/terraform/vpc.yaml`):
```yaml
components:
  terraform:
    vpc:
      # Component-level dependencies override stack-level
      dependencies:
        tools:
          terraform: "~> 1.10.0"  # Component-specific constraint
          tflint: "^0.54.0"       # Component-specific requirement
      vars:
        name: vpc
```

**Component Instance Configuration** (`stacks/orgs/acme/prod/us-east-1.yaml`):
```yaml
import:
  - catalog/terraform/vpc    # Inherits component-level dependencies

# Stack-level dependencies (apply to all components)
dependencies:
  tools:
    terraform: "~> 1.9.0"      # Stack-wide constraint
    aws-cli: "^2.0.0"          # Stack-wide tool

components:
  terraform:
    vpc:
      # Component-level override (only affects this component)
      dependencies:
        tools:
          terraform: "1.10.3"   # Must satisfy both stack (~> 1.9.0) and component (~> 1.10.0)
          tflint: "0.54.2"      # Specific version satisfying ^0.54.0
          checkov: "latest"     # Component-specific tool
      vars:
        environment: prod

    # Different component with different tool requirements
    rds:
      dependencies:
        tools:
          terraform: "1.9.8"    # Different version for this component
          # aws-cli inherited from stack: ^2.0.0
      vars:
        engine: postgres
```

#### Workflows - Declaring Tool Dependencies

**Workflow Configuration** (`stacks/workflows/deploy.yaml`):
```yaml
workflows:
  deploy-infra:
    description: Deploy infrastructure with required tools

    # Tools needed for this workflow
    dependencies:
      tools:
        terraform: "~> 1.10.0"
        aws-cli: "^2.0.0"
        jq: "latest"

    steps:
      - name: plan
        command: terraform plan vpc -s {{ .stack }}
      - name: apply
        command: terraform apply vpc -s {{ .stack }}
```

#### Custom Commands - Tool Requirements

**Custom Command** (in `atmos.yaml`):
```yaml
commands:
  - name: deploy
    description: Deploy with version-controlled tools

    # Tools required for this command
    dependencies:
      tools:
        terraform: "~> 1.10.0"
        kubectl: "^1.32.0"

    steps:
      - atmos terraform plan vpc -s {{ .stack }}
      - atmos terraform apply vpc -s {{ .stack }}
```

### Inheritance Behavior

**Merge Strategy**: Deep merge with override (child overrides parent)

#### Stack-Level Inheritance

```yaml
# Parent stack (catalog/base.yaml)
dependencies:
  tools:
    terraform: "~> 1.10.0"
    helm: "^3.17.0"

# Child stack (prod.yaml)
import:
  - catalog/base

dependencies:
  tools:
    terraform: "1.10.3"    # Override: specific version (must satisfy ~> 1.10.0)
    kubectl: "^1.32.0"     # Add: new tool
    # helm: ^3.17.0        # Inherit: from parent

# Result after inheritance:
dependencies:
  tools:
    terraform: "1.10.3"    # From child (validated against parent constraint)
    helm: "^3.17.0"        # From parent
    kubectl: "^1.32.0"     # From child
```

#### Component-Level Inheritance

```yaml
# Component catalog (catalog/terraform/vpc.yaml)
components:
  terraform:
    vpc:
      dependencies:
        tools:
          terraform: "~> 1.10.0"
          tflint: "^0.54.0"

# Stack file (stacks/orgs/acme/prod.yaml)
import:
  - catalog/terraform/vpc

# Stack-level dependencies
dependencies:
  tools:
    terraform: "~> 1.9.0"    # Stack-wide constraint
    aws-cli: "^2.0.0"

components:
  terraform:
    vpc:
      dependencies:
        tools:
          terraform: "1.10.3"  # Override component catalog (must satisfy both constraints)
          checkov: "latest"    # Add: component-specific tool
          # tflint inherited from component catalog: ^0.54.0

# Result after inheritance for vpc component:
components:
  terraform:
    vpc:
      dependencies:
        tools:
          terraform: "1.10.3"   # From component (validated against ~> 1.10.0 and ~> 1.9.0)
          tflint: "^0.54.0"     # From component catalog
          checkov: "latest"     # From component instance
          aws-cli: "^2.0.0"     # From stack-level
```

#### Resolution Order (Highest to Lowest Priority)

1. **Component instance** (`components.terraform.vpc.dependencies.tools`)
2. **Component catalog** (imported component's `dependencies.tools`)
3. **Stack instance** (top-level `dependencies.tools`)
4. **Stack catalog** (imported stack's `dependencies.tools`)

**Validation**: All overrides must satisfy constraints from parent levels.

### Execution Flow

#### Component Execution (terraform/helmfile/packer commands)

```
atmos terraform plan vpc -s prod/us-east-1
  ↓
1. Load stack configuration (with imports and inheritance)
2. Resolve dependencies.tools from merged stack config
3. Validate child constraints satisfy parent constraints
4. Check installed versions in .tool-versions or .tools/
5. Auto-install missing tools or incorrect versions
6. Set PATH to use toolchain-managed versions
7. Execute: terraform plan
```

#### Workflow Execution

```
atmos workflow deploy-infra -s prod/us-east-1
  ↓
1. Load workflow configuration
2. Resolve workflow dependencies.tools
3. Merge with stack dependencies.tools (workflow takes precedence)
4. Check/install required tools
5. Execute workflow steps with toolchain-managed tools
```

#### Custom Command Execution

```
atmos deploy prod/us-east-1
  ↓
1. Load custom command configuration
2. Resolve command dependencies.tools
3. Check/install required tools
4. Execute command steps with toolchain-managed tools
```

### Constraint Validation

**Rules**:
1. Child constraints MUST satisfy parent constraints
2. Use SemVer for validation: `github.com/Masterminds/semver/v3`
3. Fail fast if constraint conflict detected

**Example Validation**:
```go
// Parent: terraform: "~> 1.5.0"  (allows 1.5.0 <= version < 1.6.0)
// Child:  terraform: "1.5.7"     ✅ VALID (within range)
// Child:  terraform: "1.6.0"     ❌ INVALID (outside range)
// Child:  terraform: "^1.5.0"    ✅ VALID (more restrictive)
```

### Schema Updates Required

#### Stack Configuration Schema
```yaml
# Top-level (inheritable)
dependencies:
  tools:
    <tool-name>: <version-constraint>

components:
  terraform:
    <component>:
      # ... existing structure
```

#### Workflow Schema
```yaml
workflows:
  <workflow-name>:
    dependencies:
      tools:
        <tool-name>: <version-constraint>
    steps:
      # ... existing structure
```

#### Custom Command Schema
```yaml
commands:
  - name: <command-name>
    dependencies:
      tools:
        <tool-name>: <version-constraint>
    steps:
      # ... existing structure
```

### Implementation Phases

**Phase 1: Stack Dependencies** (Highest Priority)
- [ ] Update stack schema to support top-level `dependencies.tools`
- [ ] Implement inheritance logic for tool dependencies
- [ ] Add constraint validation (SemVer)
- [ ] Integrate with component execution (terraform/helmfile/packer commands)
- [ ] Auto-install missing tools before execution
- [ ] Tests: Stack inheritance, constraint validation, auto-install

**Phase 2: Workflow Dependencies**
- [ ] Update workflow schema for `dependencies.tools`
- [ ] Merge workflow and stack tool dependencies
- [ ] Tool installation before workflow execution
- [ ] Tests: Workflow tool dependencies

**Phase 3: Custom Command Dependencies**
- [ ] Update custom command schema for `dependencies.tools`
- [ ] Tool installation before command execution
- [ ] Tests: Custom command tool dependencies

**Phase 4: Component Catalog Support** (Optional)
- [ ] Support `dependencies.tools` in component.yaml files
- [ ] Merge component.yaml dependencies with stack dependencies
- [ ] Tests: Component-level tool requirements

#### 2. Atmos Self-Exec Wrapper

**Status**: 🚧 **Partially Implemented** (pieces exist, not wired together)

**Required Implementation**:

Add to `cmd/root.go` **before** command execution:

```go
func Execute() error {
    // BEFORE any command processing
    if err := ensureCorrectAtmosVersion(); err != nil {
        return err
    }

    // ... rest of Execute()
}

func ensureCorrectAtmosVersion() error {
    // 1. Load .tool-versions
    toolVersions, err := toolchain.LoadToolVersions(".tool-versions")
    if err != nil {
        return nil // No .tool-versions, proceed with current version
    }

    // 2. Get required Atmos version
    versions := toolVersions.Tools["cloudposse/atmos"]
    if len(versions) == 0 {
        return nil // No Atmos version pinned
    }
    requiredVersion := versions[0]

    // 3. Compare with current version
    if version.Version == requiredVersion {
        return nil // Already correct version
    }

    // 4. Install required version
    if err := toolchain.RunInstall("cloudposse/atmos@"+requiredVersion, false, false); err != nil {
        return fmt.Errorf("failed to install atmos@%s: %w", requiredVersion, err)
    }

    // 5. Exec into required version
    installer := toolchain.NewInstaller()
    binaryPath, err := installer.FindBinaryPath("cloudposse", "atmos", requiredVersion)
    if err != nil {
        return err
    }

    // Replace current process with required version
    return syscall.Exec(binaryPath, os.Args, os.Environ())
}
```

#### 3. Configurable Registries

**Status**: ❌ **Not Implemented**

**Current**: Hard-coded Aqua registry URLs in `aqua_registry.go`

**Desired Configuration** (`atmos.yaml`):
```yaml
toolchain:
  registries:
    - name: aqua
      type: aqua
      url: https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs
      priority: 100
    - name: local
      type: local
      path: ./tools.yaml
      priority: 200  # Higher priority = checked first
    - name: custom-corp
      type: aqua
      url: https://github.example.com/corp/tool-registry/main/pkgs
      priority: 150
```

**Implementation Needs**:
- Registry abstraction interface
- Registry configuration schema
- Priority-based resolution
- Registry health checking

#### 4. Checksum Verification

**Status**: ❌ **Not Implemented**

**Security Gap**: Downloaded binaries are not verified

**Required**:
- SHA256 checksum verification
- GPG signature verification (where available)
- Checksum file fetching from releases
- Verification before extraction

#### 5. Offline Mode / Full Binary Caching

**Status**: 🟡 **Partial** (downloads cached in temp, not permanent)

**Current**:
- Registry metadata cached in `/tmp/tools-cache/`
- Downloaded archives cached temporarily
- Cache cleared on system restart

**Needed**:
- Persistent cache in `~/.cache/atmos-toolchain/`
- Offline mode flag to skip network requests
- Cache expiration policies
- Cache size limits

---

## Technical Decisions

### 1. Why asdf-compatible `.tool-versions`?

**Decision**: Use asdf's `.tool-versions` format

**Rationale**:
- Industry standard (used by asdf, mise, rtx)
- Simple, human-readable format
- Allows gradual migration from asdf
- Team members can use asdf or Atmos toolchain

**Trade-offs**:
- Limited metadata (no constraints, dependencies)
- Requires separate `tools.yaml` for advanced config

### 2. Why Aqua Registry?

**Decision**: Use Aqua registry as primary tool metadata source

**Rationale**:
- Comprehensive tool catalog (1000+ tools)
- Active maintenance
- Supports version constraints
- Asset template system

**Challenges**:
- API not stable (per Aqua maintainer)
- GitHub raw URLs (rate limiting)
- No official API contract

**Mitigation**:
- Local `tools.yaml` overrides (first priority)
- Registry URL abstraction (future: allow custom registries)
- Caching to reduce API calls
- Fallback to GitHub API for version discovery

### 3. File Storage Structure

**Decision**: Store binaries in `.tools/owner/repo/version/`

**Rationale**:
- Avoids name collisions
- Supports multiple versions concurrently
- Clear ownership attribution
- Compatible with GitHub release patterns

**Alternative Considered**: `.tools/tool-name/version/`
- **Rejected**: Name collisions (e.g., multiple "cli" tools)

### 4. Process Replacement vs Wrapper Script

**Decision**: Use `syscall.Exec` for tool execution

**Rationale**:
- True process replacement (no wrapper overhead)
- Preserves exit codes correctly
- Signal handling works correctly
- Minimal performance impact

**Trade-offs**:
- Platform-specific (requires OS-specific imports)
- Cannot capture tool output (intended behavior)

---

## Configuration Reference

### atmos.yaml (Future)

```yaml
toolchain:
  # Enable/disable toolchain functionality
  enabled: true

  # Directory for installed binaries
  tools_dir: .tools

  # Path to .tool-versions file
  tool_versions_file: .tool-versions

  # Path to local tool configuration
  tools_config_file: tools.yaml

  # Registry sources (priority-ordered)
  registries:
    - name: local
      type: local
      path: ./tools.yaml
      priority: 200
    - name: aqua
      type: aqua
      url: https://raw.githubusercontent.com/aquaproj/aqua-registry/main/pkgs
      priority: 100

  # Cache settings
  cache:
    dir: ~/.cache/atmos-toolchain
    ttl: 24h
    max_size: 5GB

  # Auto-install behavior
  auto_install:
    enabled: true
    confirm: false  # Skip confirmation prompts

  # Atmos self-management
  self_manage:
    enabled: true
    check_version: true
```

### tools.yaml (Current)

```yaml
# Tool name aliases
aliases:
  terraform: hashicorp/terraform
  opentofu: opentofu/opentofu
  tofu: opentofu/opentofu
  helm: helm/helm
  kubectl: kubernetes-sigs/kubectl

# Custom tool definitions (override Aqua registry)
tools:
  # Atmos self-management
  cloudposse/atmos:
    type: github_release
    repo_owner: cloudposse
    repo_name: atmos
    binary_name: atmos
    version_constraints:
      - constraint: ">= 1.0.0"
        asset: atmos_{{trimV .Version}}_{{.OS}}_{{.Arch}}
        format: raw
      - constraint: ">= 1.0.0"
        asset: atmos_{{trimV .Version}}_{{.OS}}_{{.Arch}}.gz
        format: gzip

  # Terraform (HashiCorp releases)
  hashicorp/terraform:
    type: http
    url: https://releases.hashicorp.com/terraform/{{trimV .Version}}/terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip
    format: zip
    binary_name: terraform

  # OpenTofu (multiple asset formats)
  opentofu/opentofu:
    type: github_release
    repo_owner: opentofu
    repo_name: opentofu
    binary_name: tofu
    version_constraints:
      - constraint: ">= 1.10.0"
        asset: tofu_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz
        format: tar.gz
      - constraint: "< 1.10.0"
        asset: tofu_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip
        format: zip
```

### component.yaml (Future)

```yaml
metadata:
  type: real
  component: terraform/vpc

  # Tool dependencies
  dependencies:
    tools:
      terraform: "~> 1.5.0"  # SemVer constraint
      tflint: "^0.50.0"
      tfsec: "latest"
```

### Stack Configuration (Future)

```yaml
# stacks/dev/us-east-1.yaml
import:
  - catalog/vpc

terraform:
  vars:
    region: us-east-1

# Override tool versions for this stack
settings:
  tools:
    terraform: "1.5.7"  # Must satisfy component constraint
```

---

## Implementation Roadmap

### Phase 1: Current (Completed)

- [x] Core toolchain commands
- [x] Aqua registry integration
- [x] Local tools.yaml support
- [x] .tool-versions compatibility
- [x] Installation engine
- [x] Version resolution
- [x] Cross-platform support
- [x] Command registry pattern

### Phase 2: Self-Management (Next)

- [ ] Atmos self-exec wrapper in main CLI
- [ ] Version detection in Execute()
- [ ] Auto-install missing Atmos versions
- [ ] Process replacement logic
- [ ] Testing with multiple Atmos versions

**Priority**: HIGH - This is a key differentiator

### Phase 3: Component Dependencies (Critical)

- [ ] Schema updates for component.yaml
- [ ] Schema updates for stack configuration
- [ ] Dependency declaration format
- [ ] SemVer constraint parsing
- [ ] Stack-level tool overrides
- [ ] Component tool dependency resolution
- [ ] Integration with terraform/helmfile commands
- [ ] Auto-install before component execution

**Priority**: HIGH - Core value proposition

### Phase 4: Advanced Registry Support

- [ ] Configurable registry sources
- [ ] Registry priority system
- [ ] Custom registry format
- [ ] Registry health checks
- [ ] Fallback strategies
- [ ] Registry caching improvements

**Priority**: MEDIUM

### Phase 5: Security & Reliability

- [ ] Checksum verification
- [ ] GPG signature verification
- [ ] Offline mode
- [ ] Persistent caching
- [ ] Cache management (size limits, expiration)
- [ ] Retry logic for network failures

**Priority**: MEDIUM

### Phase 6: Developer Experience

- [ ] Interactive version selection (TUI)
- [ ] Upgrade notifications
- [ ] Tool update checking
- [ ] Bulk operations (update all)
- [ ] Tool search/discovery
- [ ] Shell completion for tool names

**Priority**: LOW

---

## Success Metrics

### User Experience Metrics

- **Setup Time**: New team member to first successful deployment < 5 minutes
- **Version Consistency**: 100% tool version match between dev and CI
- **Auto-Install Rate**: % of tool invocations that auto-install vs manual
- **Error Rate**: Tool-related errors per 1000 commands

### Technical Metrics

- **Installation Speed**: < 10s for binary download and extraction
- **Resolution Speed**: < 100ms for version resolution
- **Cache Hit Rate**: > 90% for repeated tool installations
- **Cross-Platform Parity**: 100% feature parity across Linux/macOS/Windows

---

## Open Questions

### 1. Aqua Registry Stability

**Question**: Given Aqua's API instability, should we fork/vendor the registry?

**Options**:
- A) Continue using live registry, accept breaking changes
- B) Vendor registry snapshot, update periodically
- C) Build our own registry from scratch
- D) Support multiple registry backends

**Recommendation**: D - Support multiple registries with local as fallback

### 2. Version Constraint Syntax

**Question**: Which constraint syntax should we support?

**Options**:
- A) SemVer only (`^1.5.0`, `~>1.5.0`)
- B) Exact versions only (`1.5.7`)
- C) Both + ranges (`>=1.5.0 <2.0.0`)

**Current**: Exact versions only in `.tool-versions`
**Recommendation**: C - Support all formats, use SemVer library

### 3. Atmos Self-Management Default

**Question**: Should Atmos self-exec be enabled by default?

**Concerns**:
- Unexpected behavior for users
- Potential for infinite loops
- Downloaded binary trust

**Recommendation**: Enabled by default, with clear messaging:
```
⚠️  Switching to Atmos v1.5.7 (required by .tool-versions)
    Installing cloudposse/atmos@1.5.7...
    ✓ Installed to .tools/cloudposse/atmos/1.5.7/
    Executing with required version...
```

### 4. Scope of Tool Support

**Question**: Should we support non-IaC tools (e.g., `jq`, `yq`, `grep`)?

**Arguments For**:
- Complete environment reproducibility
- Single tool management system
- Already supported by Aqua registry

**Arguments Against**:
- Scope creep
- Maintenance burden
- Most systems have these tools

**Recommendation**: Support via registry, but don't ship default configs for non-IaC tools

---

## Security Considerations

### Current Gaps

1. **No checksum verification** - Downloaded binaries not validated
2. **No signature verification** - Cannot detect tampered releases
3. **HTTP transport** - Uses HTTPS but no certificate pinning
4. **Process replacement risk** - Self-exec could be exploited if .tool-versions compromised

### Mitigation Plan

1. **Checksum Verification** (Phase 5)
   - Fetch SHA256 checksums from releases
   - Verify before extraction
   - Fail installation on mismatch

2. **Signature Verification** (Phase 5)
   - Support GPG signature verification
   - Vendor public keys for known tools
   - Warn on unsigned binaries

3. **Secure Defaults**
   - Require HTTPS for all downloads
   - Validate download sources
   - Sandbox extraction (already implemented: path validation, size limits)

4. **Audit Trail**
   - Log all tool installations
   - Record checksums of installed binaries
   - Enable tamper detection

---

## Testing Strategy

### Current Coverage: 76.3%

**Target**: 80-90%

**Recent Improvements** (2025-10-23):
- Added comprehensive tests for `WhichExec` command
- Improved `LookupToolVersion` from 33.3% to 100% coverage
- Fixed `--help` flag handling in `exec` command
- Overall coverage increased from 67.5% to 76.3%

### Test Categories

1. **Unit Tests** (`toolchain/*_test.go`)
   - Tool resolution logic
   - Version constraint parsing
   - Registry querying
   - File operations

2. **Integration Tests**
   - End-to-end tool installation
   - Multi-version scenarios
   - Registry fallback behavior
   - Component dependency resolution (future)

3. **Cross-Platform Tests**
   - Linux, macOS, Windows
   - Different architectures (amd64, arm64)
   - Path handling differences

4. **Mock Infrastructure**
   - Mock HTTP client for registry calls
   - Mock file system for installation
   - Mock GitHub API responses

### Coverage Gaps (Functions at 0%)

Priority fixes for test coverage:
- `LookupToolVersionOrLatest`
- `AddToolToVersionsAsDefault`
- `getVersionsToUninstall`
- `uninstallAllVersionsOfTool`
- `LookupToolVersionOrLatest`

---

## Dependencies

### External Libraries

- `github.com/spf13/cobra` - CLI framework
- `github.com/Masterminds/semver/v3` - SemVer parsing
- `gopkg.in/yaml.v3` - YAML processing
- `github.com/charmbracelet/bubbletea` - TUI framework (for interactive prompts)
- `github.com/charmbracelet/lipgloss` - TUI styling

### External Services

- GitHub API (for releases and version discovery)
- Aqua Registry (for tool metadata)
- Tool distribution servers (HashiCorp releases, GitHub releases, etc.)

---

## Migration Guide

### From asdf to Atmos Toolchain

1. **Keep existing `.tool-versions`** - Atmos reads this directly
2. **Optional**: Create `tools.yaml` for custom configurations
3. **Install tools**: Run `atmos toolchain install` in project root
4. **Update PATH** (optional): `atmos toolchain path --export >> ~/.bashrc`

### From Manual Tool Management

1. **Create `.tool-versions`**: `atmos toolchain add terraform@1.5.7`
2. **Install**: `atmos toolchain install`
3. **Use**: `atmos terraform plan` (auto-uses correct version)

---

## Appendix

### Glossary

- **Tool**: A third-party CLI binary (e.g., terraform, helm, kubectl)
- **Registry**: A metadata repository describing tools and their releases
- **Resolver**: Logic that determines which version of a tool to use
- **Installer**: Logic that downloads, extracts, and installs tools
- **Component Dependency**: A tool version requirement declared in component.yaml
- **Stack Override**: A tool version specified in stack configuration

### References

- [Aqua Registry](https://github.com/aquaproj/aqua-registry)
- [asdf Version Manager](https://asdf-vm.com/)
- [SemVer Specification](https://semver.org/)
- [CLAUDE.md](../../CLAUDE.md) - Atmos development guidelines

### File Locations

- Implementation: `/toolchain/*.go`
- Commands: `/cmd/toolchain/*.go`
- Tests: `/toolchain/*_test.go`
- Docs: `/docs/prd/toolchain-implementation.md`
