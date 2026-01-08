# PRD: Component Working Directory (Workdir) Provisioner

**Status:** Implemented
**Version:** 1.2
**Last Updated:** 2025-12-28
**Author:** Erik Osterman

---

## Executive Summary

The Workdir Provisioner enables isolated working directories for Terraform component execution. It copies local components to a `.workdir/` directory, enabling concurrent terraform operations on the same component by isolating execution environments.

**Key Benefits:**
- **Concurrency:** Run multiple terraform operations on the same component simultaneously
- **Isolation:** Each component instance runs in its own directory, preventing conflicts
- **Clean Separation:** Terraform state and lock files are isolated per component

> **Note:** Remote source downloading (JIT vendoring) is handled by the separate `source-provisioner`. This provisioner focuses exclusively on local component isolation.

---

## Problem Statement

### Current Limitations

1. **Concurrency Conflicts:** Running `terraform plan` on the same component in multiple terminals causes conflicts because they share the same `.terraform/` directory.

2. **State File Conflicts:** Multiple terraform operations on the same component can corrupt state files or lock files.

3. **CI/CD Parallelism:** Running terraform across multiple stacks in parallel is risky when they share component directories.

### User Stories

1. **As a platform engineer**, I want to run terraform plan on multiple stacks concurrently without interference.

2. **As a CI/CD pipeline**, I want to safely parallelize terraform operations across components.

3. **As a developer**, I want to test changes in one terminal while running plan in another without conflicts.

---

## Solution Overview

### Architecture

The Workdir Provisioner is a self-registering provisioner that runs before `terraform init`:

```
┌─────────────────────────────────────────────────────────────────┐
│                     Terraform Execution Flow                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Load Component Config                                        │
│          ↓                                                       │
│  2. Execute Provisioners (before.terraform.init)                 │
│          ↓                                                       │
│     ┌─────────────────────────────────────────────────┐         │
│     │           Workdir Provisioner                    │         │
│     │                                                  │         │
│     │  • Check activation (provision.workdir.enabled)  │         │
│     │  • Create .workdir/terraform/<stack>-<component>/│         │
│     │  • Copy local component to workdir               │         │
│     │  • Compute content hash for change detection     │         │
│     │  • Set _workdir_path in componentConfig          │         │
│     └─────────────────────────────────────────────────┘         │
│          ↓                                                       │
│  3. Use _workdir_path for terraform execution                    │
│          ↓                                                       │
│  4. terraform init / plan / apply                                │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Component Architecture

The implementation follows a two-layer architecture:

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI Layer                                │
│                  cmd/terraform/workdir/                          │
├─────────────────────────────────────────────────────────────────┤
│  workdir.go          │ Root command and helpers                  │
│  workdir_list.go     │ List all workdirs                         │
│  workdir_show.go     │ Show workdir details                      │
│  workdir_describe.go │ Output workdir as manifest                │
│  workdir_clean.go    │ Clean workdir(s)                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Business Logic Layer                        │
│                  pkg/provisioner/workdir/                        │
├─────────────────────────────────────────────────────────────────┤
│  workdir.go     │ Provisioner with Service pattern              │
│  clean.go       │ Clean operations                              │
│  types.go       │ WorkdirMetadata, constants                    │
│  interfaces.go  │ FileSystem, Hasher, WorkdirManager            │
│  fs.go          │ Default implementations                       │
└─────────────────────────────────────────────────────────────────┘
```

### Directory Structure

| Location | Purpose |
|----------|---------|
| `.workdir/terraform/<stack>-<component>/` | Per-stack isolated execution directory |

---

## Configuration

### Activation Rules

The workdir provisioner activates when `provision.workdir.enabled: true` is set in the component configuration.

Otherwise, terraform runs directly in `components/terraform/<component>/` (default behavior).

### Configuration Schema

#### Enable Workdir for a Component

```yaml
components:
  terraform:
    vpc:
      provision:
        workdir:
          enabled: true
      vars:
        cidr_block: "10.0.0.0/16"
```

#### Enable Workdir via Stack Defaults

```yaml
# stacks/_defaults.yaml
terraform:
  provision:
    workdir:
      enabled: true  # Enable for all components
```

---

## Directory Structure

### Project Layout

```
project/
├── atmos.yaml
├── components/
│   └── terraform/
│       └── vpc/                    # Local component source
│           └── main.tf
├── .workdir/                       # Workdir location (gitignored)
│   └── terraform/
│       └── dev-vpc/                # Isolated copy for stack 'dev', component 'vpc'
│           ├── main.tf             # Copied from components/terraform/vpc/
│           ├── .terraform/         # Isolated terraform directory
│           └── .workdir-metadata.json
└── stacks/
    └── dev.yaml
```

---

## CLI Commands

The workdir feature includes a full CLI for managing working directories:

### Command Overview

```bash
atmos terraform workdir <subcommand> [options]
```

### Subcommands

#### List Workdirs

List all working directories in the project.

```bash
# List in table format (default)
atmos terraform workdir list

# List in JSON format
atmos terraform workdir list --format json

# List in YAML format
atmos terraform workdir list --format yaml
```

#### Show Workdir Details

Display detailed information about a component's working directory.

```bash
atmos terraform workdir show vpc --stack dev
```

Output includes:
- Component name
- Stack
- Source path
- Workdir path
- Content hash
- Created/Updated timestamps

#### Describe Workdir as Manifest

Output the workdir configuration as a valid Atmos stack manifest snippet.

```bash
atmos terraform workdir describe vpc --stack dev
```

#### Clean Workdirs

Remove component working directories.

```bash
# Clean a specific workdir
atmos terraform workdir clean vpc --stack dev

# Clean all workdirs
atmos terraform workdir clean --all
```

---

## Implementation Details

### Package Structure

```
pkg/provisioner/workdir/
├── types.go               # WorkdirMetadata, constants, source types
├── interfaces.go          # FileSystem, Hasher, WorkdirManager interfaces
├── workdir.go             # Main provisioner with Service pattern and init() self-registration
├── fs.go                  # FileSystem and Hasher default implementations
├── clean.go               # Clean operations (CleanWorkdir, CleanAllWorkdirs, Clean)
├── workdir_test.go        # Unit tests for provisioner
├── clean_test.go          # Unit tests for clean operations
├── integration_test.go    # Integration tests
└── mock_interfaces_test.go # Generated mocks for testing

cmd/terraform/workdir/
├── workdir.go             # Root command and helpers
├── workdir_list.go        # List subcommand
├── workdir_show.go        # Show subcommand
├── workdir_describe.go    # Describe subcommand
├── workdir_clean.go       # Clean subcommand
├── workdir_helpers.go     # Shared helper functions
├── *_test.go              # Unit tests for each command
└── mock_workdir_manager_test.go # Generated mocks
```

### Key Interfaces

```go
// FileSystem abstracts filesystem operations for testability.
type FileSystem interface {
    MkdirAll(path string, perm os.FileMode) error
    CopyDir(src, dst string) error
    WriteFile(path string, data []byte, perm os.FileMode) error
    Exists(path string) bool
    ReadDir(path string) ([]os.DirEntry, error)
    ReadFile(path string) ([]byte, error)
}

// Hasher abstracts hash computation for testability.
type Hasher interface {
    HashDir(path string) (string, error)
}

// WorkdirManager provides workdir operations for CLI commands.
type WorkdirManager interface {
    ListWorkdirs(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error)
    GetWorkdirInfo(atmosConfig *schema.AtmosConfiguration, component, stack string) (*WorkdirInfo, error)
    DescribeWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) (map[string]any, error)
    CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) error
    CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration) error
}
```

### Self-Registration

```go
// pkg/provisioner/workdir/workdir.go

const HookEventBeforeTerraformInit = provisioner.HookEvent("before.terraform.init")

func init() {
    _ = provisioner.RegisterProvisioner(provisioner.Provisioner{
        Type:      "workdir",
        HookEvent: HookEventBeforeTerraformInit,
        Func:      ProvisionWorkdir,
    })
}
```

### Service Pattern

The provisioner uses a Service pattern for dependency injection and testability:

```go
// Service coordinates workdir provisioning operations.
type Service struct {
    fs     FileSystem
    hasher Hasher
}

// NewService creates a new workdir service with default implementations.
func NewService() *Service

// NewServiceWithDeps creates a new workdir service with injected dependencies.
func NewServiceWithDeps(fs FileSystem, hasher Hasher) *Service

// Provision creates an isolated working directory and populates it with component files.
func (s *Service) Provision(
    ctx context.Context,
    atmosConfig *schema.AtmosConfiguration,
    componentConfig map[string]any,
) error
```

### Provisioner Function

```go
func ProvisionWorkdir(
    ctx context.Context,
    atmosConfig *schema.AtmosConfiguration,
    componentConfig map[string]any,
    authContext *schema.AuthContext,
) error {
    // 1. Check activation (provision.workdir.enabled: true)
    // 2. Create .workdir/terraform/<stack>-<component>/
    // 3. Copy local component to workdir
    // 4. Compute content hash for change detection
    // 5. Write metadata file
    // 6. Set componentConfig["_workdir_path"]
}
```

### Terraform Integration

```go
// internal/exec/terraform.go

// After provisioner execution:
if workdirPath, ok := info.ComponentSection["_workdir_path"].(string); ok && workdirPath != "" {
    componentPath = workdirPath  // Use workdir instead of component path
}
```

---

## Provider Cache Consideration

With workdirs, each component runs `terraform init` separately. To avoid redundant provider downloads:

```bash
# Set globally via environment variable
export TF_PLUGIN_CACHE_DIR="$HOME/.terraform.d/plugin-cache"
```

Or via stack defaults:

```yaml
# stacks/_defaults.yaml
terraform:
  env:
    TF_PLUGIN_CACHE_DIR: /tmp/.terraform.d/plugin-cache
```

---

## Error Handling

### Sentinel Errors

| Error | Description |
|-------|-------------|
| `ErrWorkdirCreation` | Failed to create working directory |
| `ErrWorkdirSync` | Failed to sync files to working directory |
| `ErrWorkdirMetadata` | Failed to read/write workdir metadata |
| `ErrWorkdirProvision` | Workdir provisioning failed |
| `ErrWorkdirClean` | Failed to clean working directory |
| `ErrSourceDownload` | Failed to download component source |
| `ErrSourceCacheRead` | Failed to read source cache |
| `ErrSourceCacheWrite` | Failed to write source cache |
| `ErrInvalidSource` | Invalid source configuration |

---

## Testing

### Unit Tests

- `TestIsWorkdirEnabled` - Activation detection from component config
- `TestExtractComponentName` - Component name extraction priority
- `TestExtractComponentPath` - Component path resolution
- `TestCleanWorkdir_Success` - Single workdir cleanup
- `TestCleanAllWorkdirs_Success` - All workdirs cleanup
- `TestClean_AllTakesPrecedence` - Precedence behavior when both flags set

### Integration Tests

- `TestWorkdirProvisionerRegistration` - Provisioner registration with registry
- `TestProvisionWorkdir_NoActivation` - No-op when not activated
- `TestProvisionWorkdir_WithProvisionWorkdirEnabled` - Local component with workdir
- `TestService_Provision_WithMockFileSystem` - Mock-based provisioning
- `TestCleanWorkdir` / `TestCleanAllWorkdirs` - Clean operations with permission tests

### CLI Command Tests

- `TestListCmd_*` - List command output formats (table, JSON, YAML)
- `TestShowCmd_*` - Show command output and error handling
- `TestDescribeCmd_*` - Describe command manifest output
- `TestCleanCmd_*` - Clean command validation and execution

---

## Security Considerations

1. **Workdir Isolation:** Each component gets its own isolated directory
2. **Path Validation:** Component paths are validated before copying
3. **Gitignore:** `.workdir/` should be added to `.gitignore` to prevent committing state

---

## Future Enhancements

### Planned

- [ ] `--refresh-workdir` flag to force re-copy
- [ ] Workdir lock files for concurrent access
- [ ] Skip unchanged files using content hash comparison (hash infrastructure implemented)

### Not Planned

- Remote source downloading (use `source-provisioner` instead)
- Cross-project workdir sharing
- Workdir versioning/history

---

## References

- [Backend Provisioner PRD](backend-provisioner.md)
