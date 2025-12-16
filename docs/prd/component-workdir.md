# PRD: Component Working Directory (Workdir) Provisioner

**Status:** Implemented
**Version:** 1.0
**Last Updated:** 2025-12-12
**Author:** Erik Osterman

---

## Executive Summary

The Workdir Provisioner enables isolated working directories for Terraform component execution. It supports Just-In-Time (JIT) vendoring of remote component sources via `metadata.source` and enables concurrent terraform operations on the same component by isolating execution environments.

**Key Benefits:**
- **JIT Vendoring:** Download components on-demand from remote sources (GitHub, S3, GCS, etc.)
- **Concurrency:** Run multiple terraform operations on the same component simultaneously
- **Isolation:** Each component instance runs in its own directory, preventing conflicts
- **Caching:** Remote sources are cached in XDG-compliant directories for efficiency

---

## Problem Statement

### Current Limitations

1. **No JIT Vendoring:** Components must be vendored upfront or stored locally. There's no way to reference a remote component source directly in stack configuration.

2. **Concurrency Conflicts:** Running `terraform plan` on the same component in multiple terminals causes conflicts because they share the same `.terraform/` directory.

3. **Version Conflicts:** Different stacks may need different versions of the same component. If they share the same component directory, this is impossible.

### User Stories

1. **As a developer**, I want to reference a component directly from GitHub so I don't need to vendor it locally.

2. **As a platform engineer**, I want different environments to use different versions of the same component without conflicts.

3. **As a CI/CD pipeline**, I want to run terraform plan on multiple stacks concurrently without interference.

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
│     │  • Check activation (metadata.source OR         │         │
│     │    metadata.workdir: true)                       │         │
│     │  • Create .workdir/terraform/<component>/        │         │
│     │  • Download to XDG cache (if remote source)      │         │
│     │  • Copy files to workdir                         │         │
│     │  • Set _workdir_path in componentConfig          │         │
│     └─────────────────────────────────────────────────┘         │
│          ↓                                                       │
│  3. Use _workdir_path for terraform execution                    │
│          ↓                                                       │
│  4. terraform init / plan / apply                                │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Two-Layer Architecture

| Layer | Location | Purpose |
|-------|----------|---------|
| **XDG Cache** | `~/.cache/atmos/components/` | Shared cache for downloaded remote sources |
| **Project Workdir** | `.workdir/terraform/<component>/` | Per-project execution directory |

---

## Configuration

### Activation Rules

The workdir provisioner activates when **either**:

1. `metadata.source` is present (JIT vendoring)
2. `metadata.workdir: true` is set (explicit opt-in for local components)

Otherwise, terraform runs directly in `components/terraform/<component>/` (default behavior).

### Configuration Schema

#### Simple Form (URI only)

```yaml
components:
  terraform:
    vpc:
      metadata:
        source: "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0"
      vars:
        cidr_block: "10.0.0.0/16"
```

#### Structured Form (with options)

```yaml
components:
  terraform:
    vpc:
      metadata:
        source:
          uri: "github.com/cloudposse/terraform-aws-vpc"
          version: "1.0.0"
          included_paths:
            - "*.tf"
            - "modules/**"
          excluded_paths:
            - "examples/**"
            - "test/**"
      vars:
        cidr_block: "10.0.0.0/16"
```

#### Local Component with Workdir (opt-in isolation)

```yaml
components:
  terraform:
    my-local-component:
      metadata:
        workdir: true  # Enable workdir for local component
      vars:
        name: "example"
```

### Source URI Formats

The `metadata.source` field supports all go-getter protocols:

| Protocol | Example |
|----------|---------|
| GitHub | `github.com/org/repo?ref=v1.0.0` |
| Git | `git::https://example.com/repo.git?ref=main` |
| HTTP/S | `https://example.com/module.zip` |
| S3 | `s3::https://s3.amazonaws.com/bucket/path` |
| GCS | `gcs::https://storage.googleapis.com/bucket/path` |

---

## Directory Structure

### Project Layout

```
project/
├── atmos.yaml
├── components/
│   └── terraform/
│       └── local-component/       # Local component (no workdir by default)
│           └── main.tf
├── .workdir/                       # Workdir location (gitignored)
│   └── terraform/
│       ├── vpc/                    # Workdir for JIT-vendored component
│       │   ├── main.tf             # Copied from source
│       │   ├── .terraform/         # Terraform state
│       │   └── .workdir-metadata.json
│       └── local-component/        # Workdir for local component (if opted-in)
│           ├── main.tf
│           └── .workdir-metadata.json
└── stacks/
    └── dev.yaml
```

### XDG Cache Layout

```
~/.cache/atmos/components/
├── blobs/
│   ├── ab/                         # First 2 chars of hash (sharding)
│   │   └── abcd1234.../
│   │       └── content/            # Downloaded component files
│   └── cd/
│       └── cdef5678.../
│           └── content/
├── index.json                      # Cache manifest
└── locks/                          # flock files for concurrent access
```

---

## Caching Strategy

### Cache Key Generation

Cache keys are content-addressable (SHA256):

```
key = SHA256(normalize(URI) + version)
```

### Cache Policies

| Source Type | Policy | Rationale |
|-------------|--------|-----------|
| Tagged version (`ref=v1.2.3`) | Permanent | Tags are immutable |
| Commit SHA (`ref=abc123...`) | Permanent | SHAs are immutable |
| Branch ref (`ref=main`) | TTL (1 hour) | Branches change |
| No version | TTL (1 hour) | May change |

### TTL Configuration

```yaml
# stacks/_defaults.yaml
terraform:
  provision:
    workdir:
      cache:
        ttl: 24h  # Override default 1 hour TTL
```

---

## Implementation Details

### Package Structure

```
pkg/provisioner/workdir/
├── types.go           # SourceConfig, WorkdirMetadata, CacheEntry
├── interfaces.go      # Downloader, FileSystem, Cache, Hasher interfaces
├── workdir.go         # Main provisioner with init() self-registration
├── cache.go           # XDG content-addressable cache
├── downloader.go      # go-getter integration
├── fs.go              # FileSystem and Hasher implementations
├── clean.go           # Clean operations for terraform clean command
├── workdir_test.go    # Unit tests
├── integration_test.go # Integration tests
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
    // 1. Check activation (metadata.source OR metadata.workdir: true)
    // 2. Create .workdir/terraform/<component>/
    // 3. Download to cache (if remote) or copy (if local)
    // 4. Copy from cache/local to workdir
    // 5. Set componentConfig["_workdir_path"]
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

# Clean source cache (XDG)
atmos terraform clean --cache
```

### Implementation

```go
// pkg/provisioner/workdir/clean.go

func CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component string) error
func CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration) error
func CleanSourceCache() error
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
| `ErrSourceDownload` | Failed to download component source |
| `ErrSourceCacheRead` | Failed to read from source cache |
| `ErrSourceCacheWrite` | Failed to write to source cache |
| `ErrInvalidSource` | Invalid metadata.source configuration |
| `ErrWorkdirCreation` | Failed to create working directory |
| `ErrWorkdirSync` | Failed to sync files to working directory |
| `ErrWorkdirMetadata` | Failed to read/write workdir metadata |
| `ErrWorkdirProvision` | Workdir provisioning failed |
| `ErrWorkdirClean` | Failed to clean working directory |

---

## Testing

### Unit Tests

- `TestExtractSourceConfig` - Source config extraction
- `TestIsWorkdirEnabled` - Activation detection
- `TestBuildFullURI` - URI construction with version
- `TestCacheGenerateKey` - Cache key generation
- `TestCacheGetPolicy` - Cache policy determination

### Integration Tests

- `TestWorkdirProvisionerRegistration` - Provisioner registration
- `TestProvisionWorkdir_NoActivation` - No-op when not activated
- `TestProvisionWorkdir_WithMetadataWorkdir` - Local component with workdir
- `TestService_Provision_WithRemoteSource` - Remote source provisioning
- `TestCleanWorkdir` / `TestCleanAllWorkdirs` - Clean operations

---

## Security Considerations

1. **Source Validation:** go-getter handles source validation and supports checksums
2. **Cache Isolation:** XDG cache uses content-addressable storage (no path traversal)
3. **Workdir Isolation:** Each component gets its own isolated directory
4. **Credential Handling:** Downloads use system credentials (AWS, GCS, etc.)

---

## Future Enhancements

### Planned

- [ ] `--refresh-workdir` flag to force re-download
- [ ] Workdir lock files for concurrent access
- [ ] Metadata inheritance for source configs
- [ ] Source checksum verification

### Not Planned

- Remote source authentication UI (use system credentials)
- Workdir versioning/history
- Cross-project cache sharing

---

## References

- [go-getter Documentation](https://github.com/hashicorp/go-getter)
- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
- [Backend Provisioner PRD](backend-provisioner.md)
- [Provisioner System Plan](../plans/tender-splashing-rossum.md)
