# PRD: Component Working Directory (Workdir) Provisioner

**Status:** Implemented
**Version:** 1.1
**Last Updated:** 2025-12-17
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
│     │  • Create .workdir/terraform/<component>/        │         │
│     │  • Copy local component to workdir               │         │
│     │  • Set _workdir_path in componentConfig          │         │
│     └─────────────────────────────────────────────────┘         │
│          ↓                                                       │
│  3. Use _workdir_path for terraform execution                    │
│          ↓                                                       │
│  4. terraform init / plan / apply                                │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Directory Structure

| Location | Purpose |
|----------|---------|
| `.workdir/terraform/<component>/` | Per-project isolated execution directory |

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
│       └── vpc/                    # Isolated copy of component
│           ├── main.tf             # Copied from components/terraform/vpc/
│           ├── .terraform/         # Isolated terraform directory
│           └── .workdir-metadata.json
└── stacks/
    └── dev.yaml
```

---

## Implementation Details

### Package Structure

```
pkg/provisioner/workdir/
├── types.go               # WorkdirMetadata, WorkdirConfig
├── interfaces.go          # FileSystem, Hasher, PathFilter interfaces
├── workdir.go             # Main provisioner with init() self-registration
├── fs.go                  # FileSystem and Hasher implementations
├── clean.go               # Clean operations for terraform clean command
├── workdir_test.go        # Unit tests
├── integration_test.go    # Integration tests
└── mock_interfaces_test.go # Generated mocks
```

### Self-Registration

```go
// pkg/provisioner/workdir/workdir.go

func init() {
    _ = provisioner.RegisterProvisioner(provisioner.Provisioner{
        Type:      "workdir",
        HookEvent: provisioner.HookEvent("before.terraform.init"),
        Func:      ProvisionWorkdir,
    })
}
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
    // 2. Create .workdir/terraform/<component>/
    // 3. Copy local component to workdir
    // 4. Set componentConfig["_workdir_path"]
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

## Terraform Clean Command

### Usage

```bash
# Clean workdir for specific component
atmos terraform clean vpc -s dev

# Clean all workdirs in project
atmos terraform clean --all
```

### Implementation

```go
// pkg/provisioner/workdir/clean.go

func CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component string) error
func CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration) error
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

---

## Testing

### Unit Tests

- `TestIsWorkdirEnabled` - Activation detection
- `TestCleanOptions_Structure` - Options struct validation
- `TestClean_AllTakesPrecedence` - Precedence behavior

### Integration Tests

- `TestWorkdirProvisionerRegistration` - Provisioner registration
- `TestProvisionWorkdir_NoActivation` - No-op when not activated
- `TestProvisionWorkdir_WithProvisionWorkdirEnabled` - Local component with workdir
- `TestService_Provision_WithMockFileSystem` - Mock-based provisioning
- `TestCleanWorkdir` / `TestCleanAllWorkdirs` - Clean operations

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
- [ ] Content hash comparison to skip unchanged files

### Not Planned

- Remote source downloading (use `source-provisioner` instead)
- Cross-project workdir sharing
- Workdir versioning/history

---

## References

- [Backend Provisioner PRD](backend-provisioner.md)
- [Source Provisioner](https://github.com/osterman/source-provisioner) (for remote source downloading)
