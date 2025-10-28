# PRD: Vendor URI Normalization and URL Syntax

## Overview

This document describes Atmos's vendor URI parsing system, including go-getter URL syntax, Atmos-specific extensions, and the approach to URI normalization. It serves as the technical design document for understanding how vendor source URIs are processed and transformed for downloading external dependencies.

## Background

Atmos vendoring allows users to pull external components, stacks, and configurations from various sources. The system is built on top of [HashiCorp's go-getter library](https://github.com/hashicorp/go-getter), with custom extensions for:
- OCI registry support
- Token injection for private repositories
- SSH URL rewriting
- Custom Git operations (symlink removal, shallow clones)

### The Triple-Slash Problem

A common user pattern emerged where users write `///` to indicate "clone from the root of the repository":

```yaml
source: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git///?ref=v5.7.0"
```

This pattern broke in go-getter v1.7.9 due to CVE-2025-8959 security fixes. **The main branch has no triple-slash handling**, resulting in:
1. Silent failures - zero files downloaded
2. Empty directories created
3. No error messages to indicate the problem

**This PR solves the problem by:**
1. Detecting triple-slash patterns using go-getter's built-in `SourceDirSubdir()` parser
2. Converting `///` → `//.` (root) or `///path` → `//path` (subdirectory)
3. Working with all Git platforms: GitHub, GitLab, Azure DevOps, Gitea, self-hosted
4. Leveraging go-getter's official API instead of heuristics

## go-getter URL Syntax

### Basic Syntax

go-getter uses a specific URL syntax where `//` serves as a delimiter between the repository URL and the subdirectory path:

```text
<repository-url>//<subdirectory-path>
```

**Examples:**
- `github.com/owner/repo.git//examples/demo` - Clone repo, use `examples/demo` subdirectory
- `github.com/owner/repo.git//.` - Clone repo, use root directory (current directory)
- `github.com/owner/repo.git` - Clone repo (ambiguous, should add `//.` for clarity)

### Subdirectory Delimiter

The double-slash (`//`) is **not** a path separator—it's a **delimiter** that separates:
1. **Left side**: The source to download (repository URL, archive URL, etc.)
2. **Right side**: The subdirectory within that source to extract

**How go-getter interprets this:**
```text
github.com/cloudposse/atmos.git//examples/demo-library/weather?ref=v1.0.0
└─────────────────────────────┘  └──────────────────────────────┘ └────────┘
         Repository URL              Subdirectory Path           Query Params
```

### Root Directory Convention

To specify the root of a repository, use `//.` (double-slash-dot):

```yaml
source: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0"
```

The `.` represents the current directory (root of repository), following POSIX convention.

### Query Parameters

Query parameters are appended after the subdirectory path and apply to the source download:

```text
github.com/owner/repo.git//path/to/subdir?ref=v1.0.0&depth=1
```

Common parameters:
- `ref=<value>` - Git ref (branch, tag, commit SHA)
- `depth=<n>` - Git clone depth for shallow clones
- `sshkey=<path>` - Path to SSH private key

## Supported URL Schemes

### Native go-getter Schemes

These schemes are handled by the go-getter library directly:

| Scheme | Description | Example |
|--------|-------------|---------|
| `file://` | Local filesystem | `file:///path/to/components` |
| `http://` / `https://` | HTTP/HTTPS downloads | `https://example.com/archive.tar.gz` |
| `git::` | Git repositories | `git::https://github.com/owner/repo.git?ref=main` |
| `hg` | Mercurial repositories | `hg::https://bitbucket.org/owner/repo` |
| `s3::` | Amazon S3 (not enabled) | `s3::https://s3.amazonaws.com/bucket/key` |

### Atmos Custom Schemes

Atmos extends go-getter with additional schemes:

| Scheme | Description | Implementation |
|--------|-------------|----------------|
| `oci://` | OCI registries (ghcr.io, ECR, etc.) | `internal/exec/vendor_model.go` |

### Implicit Scheme Handling

Atmos's `CustomGitDetector` provides intelligent scheme detection:

| Input | Detected As | Example |
|-------|-------------|---------|
| No scheme | `https://` | `github.com/owner/repo` → `https://github.com/owner/repo` |
| SCP-style | `ssh://` | `git@github.com:owner/repo.git` → `ssh://git@github.com/owner/repo.git` |

## Atmos Custom Extensions

### 1. CustomGitDetector

**Location:** `pkg/downloader/custom_git_detector.go`

**Functionality:**
- **Token injection** for GitHub, GitLab, Bitbucket
- **SCP-style SSH URL rewriting** (`git@host:path` → `ssh://git@host/path`)
- **Automatic scheme defaulting** (no scheme → `https://`)
- **Automatic shallow clones** (adds `depth=1` if not specified)
- **Username injection** for known Git hosts

**Token Resolution:**
- GitHub: `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN`
- GitLab: `ATMOS_GITLAB_TOKEN` or `GITLAB_TOKEN`
- Bitbucket: `ATMOS_BITBUCKET_TOKEN` or `BITBUCKET_TOKEN`

**Default Usernames for Token Injection:**
- GitHub: `x-access-token`
- GitLab: `oauth2`
- Bitbucket: `x-token-auth` (or `ATMOS_BITBUCKET_USERNAME` if set)

### 2. CustomGitGetter

**Location:** `pkg/downloader/git_getter.go`

**Functionality:**
- **Symlink removal** after cloning (security measure)
- Extends go-getter's native `GitGetter`

### 3. OCI Registry Support

**Location:** `internal/exec/vendor_model.go`

**Functionality:**
- Downloads artifacts from OCI-compatible registries
- Supports GitHub Container Registry (ghcr.io), AWS ECR, Google Artifact Registry, etc.
- Processes OCI image layers and extracts to destination

**Example:**
```yaml
source: "oci://ghcr.io/cloudposse/terraform-aws-components:v1.0.0"
```

## URI Patterns and Examples

### Pattern Matrix

| Pattern | Scheme | Subdirectory | Auth | Example |
|---------|--------|-------------|------|---------|
| GitHub shorthand | Implicit https | No | Token injection | `github.com/owner/repo.git?ref=v1.0` |
| GitHub with subdir | Implicit https | Yes | Token injection | `github.com/owner/repo.git//examples?ref=v1.0` |
| GitHub root | Implicit https | Root (`.`) | Token injection | `github.com/owner/repo.git//.?ref=v1.0` |
| Explicit HTTPS | https:// | Optional | In URL | `https://github.com/owner/repo.git//path?ref=v1.0` |
| git:: HTTPS | git:: + https:// | Optional | In URL | `git::https://github.com/owner/repo.git?ref=v1.0` |
| git:: SSH | git:: + ssh:// | Optional | SSH key | `git::ssh://git@github.com/owner/repo.git?ref=v1.0` |
| SCP-style SSH | Implicit ssh:// | Optional | SSH key | `git@github.com:owner/repo.git` |
| HTTPS with creds | https:// | Optional | user:pass | `https://user:pass@github.com/owner/repo.git` |
| Raw HTTPS file | https:// | N/A | Optional | `https://raw.githubusercontent.com/.../file.tf` |
| Local relative | N/A | N/A | N/A | `../../../components/terraform/mock` |
| Local absolute | N/A | N/A | N/A | `/absolute/path/to/components` |
| file:// URI | file:// | N/A | N/A | `file:///path/to/components` |
| OCI registry | oci:// | N/A | Registry auth | `oci://ghcr.io/owner/image:tag` |
| GitLab | Implicit https | Optional | Token injection | `gitlab.com/group/project.git?ref=v1.0` |
| Bitbucket | Implicit https | Optional | Token injection | `bitbucket.org/user/repo.git?ref=master` |
| Azure DevOps | Implicit https | Optional | Token injection | `dev.azure.com/org/project/_git/repo` |

### Example Configurations

#### 1. GitHub Component with Subdirectory

```yaml
spec:
  sources:
    - component: "vpc"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0"
      targets:
        - "components/terraform/vpc"
```

**Normalized to:**
```text
git::https://github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0&depth=1
```

#### 2. GitHub Root Directory

```yaml
spec:
  sources:
    - component: "s3-bucket"
      source: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0"
      targets:
        - "components/terraform/s3-bucket"
```

**Normalized to:**
```text
git::https://github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0&depth=1
```

#### 3. Triple-Slash Pattern (Legacy)

```yaml
spec:
  sources:
    - component: "s3-bucket"
      source: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git///?ref=v5.7.0"
      targets:
        - "components/terraform/s3-bucket"
```

**Should normalize to:**
```text
git::https://github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0&depth=1
```

#### 4. SSH with SCP-Style URL

```yaml
spec:
  sources:
    - component: "null-label"
      source: "git@github.com:cloudposse/terraform-null-label.git?ref=0.25.0"
      targets:
        - "components/terraform/null-label"
```

**Normalized to:**
```text
git::ssh://git@github.com/cloudposse/terraform-null-label.git?ref=0.25.0&depth=1
```

#### 5. OCI Registry

```yaml
spec:
  sources:
    - component: "mock"
      source: "oci://ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v1.0.0"
      targets:
        - "components/terraform/mock"
```

**No normalization needed** (OCI scheme is handled separately).

#### 6. Local Relative Path

```yaml
spec:
  sources:
    - component: "mixins"
      source: "../../../fixtures/components/terraform/mock"
      targets:
        - "components/terraform/mock"
```

**Resolved to absolute path** (no go-getter processing).

#### 7. Raw HTTPS File

```yaml
spec:
  sources:
    - component: "context"
      source: "https://raw.githubusercontent.com/cloudposse/terraform-null-label/0.25.0/exports/context.tf"
      targets:
        - "components/terraform/mixins/context.tf"
```

**No normalization needed** (direct HTTP download).

## URI Normalization Algorithm

### Current Implementation Issues

The current implementation in `vendor_uri_helpers.go` uses hardcoded pattern matching:

```go
// Problematic approach
func containsTripleSlash(uri string) bool {
    return strings.Contains(uri, ".git///") ||
           strings.Contains(uri, ".com///") ||
           strings.Contains(uri, ".org///")
}
```

**Issues:**
1. Doesn't work for Azure DevOps (`dev.azure.com`), self-hosted Git, Gitea
2. Doesn't use go-getter's `SourceDirSubdir()` function (already available!)
3. Not extensible or maintainable

### Proposed Solution

**Use go-getter's built-in `SourceDirSubdir()` function:**

```go
import "github.com/hashicorp/go-getter"

// Proper approach using go-getter's API
func normalizeVendorURI(uri string) string {
    // Skip normalization for special types
    if isOCIURI(uri) || isS3URI(uri) || isLocalPath(uri) {
        return uri
    }

    // Use go-getter's built-in parser to extract source and subdirectory
    source, subdir := getter.SourceDirSubdir(uri)

    // If no subdirectory specified for Git URLs, add root indicator
    if isGitURI(source) && subdir == "" {
        // User didn't specify subdirectory, default to root
        return source + "//."
    }

    // If subdirectory starts with "/", it means triple-slash was used
    // Convert "///" to "//" for paths, or "//." for root
    if strings.HasPrefix(subdir, "/") {
        subdir = strings.TrimPrefix(subdir, "/")
        if subdir == "" {
            subdir = "." // Root directory
        }
    }

    // Reconstruct URI
    if subdir != "" {
        return source + "//" + subdir
    }
    return source
}
```

**Benefits:**
- Uses go-getter's official URL parsing
- Works for all Git hosting platforms uniformly
- Maintainable and extensible
- No hardcoded domain patterns

### Normalization Rules

1. **Skip special schemes:** OCI, S3, local paths
2. **Parse using go-getter:** Extract source and subdirectory
3. **Add `//.` for Git URLs without subdirectory**
4. **Convert `///` to `//.` for root** (empty path after delimiter)
5. **Convert `///path` to `//path`** (remove extra slash)
6. **Preserve query parameters**

## Test Coverage

### Critical: Original Issue (DEV-3639) Coverage ✅

**Test File:** `internal/exec/vendor_triple_slash_test.go`
**Test Fixture:** `tests/fixtures/scenarios/vendor-triple-slash/vendor.yaml`

The **original user-reported issue** is comprehensively tested:

```yaml
# User's exact pattern from the bug report
source: github.com/terraform-aws-modules/terraform-aws-s3-bucket.git///?ref={{.Version}}
included_paths:
  - "**/modules/**"
  - "**/*.tf"
  - "**/README.md"
```

**Test validates:**
- ✅ Files are actually pulled (not empty directories)
- ✅ Glob patterns work correctly (`**/*.tf`, `**/modules/**`)
- ✅ Expected files exist: `main.tf`, `outputs.tf`, `variables.tf`, `versions.tf`
- ✅ Documentation files: `README.md`, `LICENSE`
- ✅ Module subdirectories: `modules/notification/*.tf`

**Result:** The triple-slash normalization fix resolves the original issue where go-getter v1.7.9+ broke this pattern.

### Well-Tested Patterns

✅ GitHub shorthand (15+ tests)
✅ git:: prefixes (6+ tests)
✅ OCI registries (4+ tests)
✅ Local file paths (8+ tests)
✅ **Triple-slash root (2 dedicated tests including DEV-3639)**
✅ HTTPS with credentials (3 tests)
✅ SCP-style SSH (2 tests)
✅ GitLab URLs (1 test)
✅ Bitbucket URLs (1 test)

### Coverage Gaps

❌ Azure DevOps URLs (`dev.azure.com`)
❌ Self-hosted Git (custom domains like `git.company.com`)
❌ Gitea URLs
❌ Gogs URLs
❌ S3 scheme (code exists but not enabled/tested)
❌ Triple-slash with self-hosted Git
❌ Archive formats (.tar.gz, .zip URLs)

### Recommended Test Additions

```yaml
# tests/fixtures/scenarios/vendor-azure-devops/vendor.yaml
spec:
  sources:
    - component: "azure-test"
      source: "dev.azure.com/organization/project/_git/repository///?ref=main"
      targets:
        - "components/terraform/azure-test"

# tests/fixtures/scenarios/vendor-self-hosted-git/vendor.yaml
spec:
  sources:
    - component: "self-hosted"
      source: "git.company.com/team/repository.git///?ref=v1.0.0"
      targets:
        - "components/terraform/self-hosted"

# tests/fixtures/scenarios/vendor-gitea/vendor.yaml
spec:
  sources:
    - component: "gitea-test"
      source: "gitea.com/owner/repository///?ref=main"
      targets:
        - "components/terraform/gitea-test"
```

## Implementation Plan

### Phase 1: Documentation ✅

1. Create this PRD document
2. Create user-facing URL syntax documentation
3. Update existing vendor documentation

### Phase 2: Refactoring

1. Refactor `vendor_uri_helpers.go`:
   - Replace `containsTripleSlash()` with `getter.SourceDirSubdir()`
   - Remove hardcoded `.git///`, `.com///`, `.org///` patterns
   - Use proper URL parsing for all Git platforms

2. Maintain backward compatibility:
   - All existing tests must pass
   - Existing vendor.yaml files must continue working

3. Add new test cases:
   - Azure DevOps
   - Self-hosted Git
   - Gitea
   - Edge cases

### Phase 3: Validation

1. Run full test suite
2. Test with real-world vendor.yaml files
3. Validate cross-platform behavior (Windows, macOS, Linux)

## References

- [go-getter Documentation](https://github.com/hashicorp/go-getter)
- [go-getter URL Syntax](https://github.com/hashicorp/go-getter#url-format)
- [go-getter SourceDirSubdir Function](https://pkg.go.dev/github.com/hashicorp/go-getter#SourceDirSubdir)
- [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec)
- [CVE-2025-8959 - go-getter Path Traversal](https://github.com/hashicorp/go-getter/security/advisories)

## Related Files

- `internal/exec/vendor_uri_helpers.go` - URI normalization logic
- `internal/exec/vendor_utils.go` - Main vendor processing
- `internal/exec/vendor_model.go` - Vendor execution and OCI support
- `pkg/downloader/custom_git_detector.go` - Custom Git URL detection
- `pkg/downloader/gogetter_downloader.go` - go-getter integration
- `pkg/downloader/git_getter.go` - Custom Git getter
- `tests/fixtures/scenarios/*/vendor.yaml` - Test fixtures
