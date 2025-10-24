# Atmos Deployments - Git-Based Deployment Tracking

This document describes how Atmos uses Git-native features (refs, annotated tags, and notes) to track deployments, build records, and metadata without requiring an external database.

## Overview

Atmos leverages Git's built-in capabilities to create a complete deployment tracking system:

- **Lightweight refs** - Current deployment state per environment
- **Annotated tags** - Immutable deployment history with YAML metadata
- **Git notes** - Per-commit metadata (SBOM, test results, scan results)

**Key principle:** Git is the single source of truth for deployment state. No external database required.

## Architecture Layers

### Layer 1: Git Operations (Core VCS)

```
pkg/git/
  interface.go      # Core Git operations (VCS)
  repository.go     # Local git command implementation
  tags.go          # Tag creation and parsing
  notes.go         # Notes management
  refs.go          # Custom ref management
```

**Git operations work with ANY Git remote** (GitHub, GitLab, Bitbucket, self-hosted).

### Layer 2: Git Hosting Platform Features

```
pkg/gitprovider/
  interface.go      # Platform abstraction
  github/
    github.go       # GitHub API integration (PRs, commit status, rulesets)
  gitlab/
    gitlab.go       # GitLab API integration (MRs, commit status, protected refs)
```

**Platform features are optional enhancements** on top of Git (PR comments, commit statuses, etc.).

## Git Ref Strategy

### Namespace: `refs/atmos/*`

All Atmos refs use the `atmos/` prefix for clear namespacing.

**Hierarchy:** `deployment → target → component`

```
refs/atmos/
  deployments/
    payment-service/        # Deployment name
      prod/                 # Target
        api               # Component
        worker
        database
      staging/
        api
        worker
        database
      dev/
        api
        worker
        database
    inventory-service/
      prod/
        api
        cache
  builds/
    abc123                  # Build record reference
  releases/
    xyz789                  # Release record reference
```

**Rationale:**
- Deployments are application-specific contexts
- Each deployment has its own set of targets (can be numerous and vary per deployment)
- Components are deployed within those targets

### Lightweight Refs for Current State

**Purpose:** Track "what's deployed now" for each target.

**Characteristics:**
- Mutable (updates in place)
- Protectable with GitHub rulesets / GitLab protected refs
- Fast lookup
- Atomic updates (no merge conflicts)

**Example:**
```bash
# Create/update deployment ref (deployment → target → component)
git update-ref refs/atmos/deployments/payment-service/prod/api abc123def

# Query current deployment
git show-ref refs/atmos/deployments/payment-service/prod/api
# Output: abc123def456789... refs/atmos/deployments/payment-service/prod/api

# What's deployed across all targets for this deployment?
git show-ref | grep refs/atmos/deployments/payment-service

# What's deployed to prod across all deployments?
git show-ref | grep refs/atmos/deployments/.*/prod
```

**Implementation:**
```go
// pkg/git/refs.go
func SetDeploymentRef(deployment, target, component, sha string) error {
    ref := fmt.Sprintf("refs/atmos/deployments/%s/%s/%s", deployment, target, component)
    return exec.Command("git", "update-ref", ref, sha).Run()
}

func GetDeploymentRef(deployment, target, component string) (string, error) {
    ref := fmt.Sprintf("refs/atmos/deployments/%s/%s/%s", deployment, target, component)
    output, err := exec.Command("git", "show-ref", "--hash", ref).Output()
    return strings.TrimSpace(string(output)), err
}

func ListDeploymentRefs(deployment, target string) ([]Ref, error) {
    pattern := fmt.Sprintf("refs/atmos/deployments/%s/%s/", deployment, target)
    output, _ := exec.Command("git", "show-ref").Output()

    var refs []Ref
    for _, line := range strings.Split(string(output), "\n") {
        if strings.Contains(line, pattern) {
            parts := strings.Fields(line)
            if len(parts) == 2 {
                refs = append(refs, Ref{Name: parts[1], SHA: parts[0]})
            }
        }
    }
    return refs, nil
}
```

**Protection (GitHub rulesets):**
```yaml
# .github/rulesets/protect-prod-deployments.yml
name: Protect Production Deployment Refs
target: branch
enforcement: active
conditions:
  ref_name:
    include:
      - refs/atmos/deployments/*/prod/**  # All deployments, prod target
rules:
  - type: deletion
  - type: required_signatures
  - type: non_fast_forward
bypass_actors:
  - actor_type: Integration
    actor_id: 1  # GitHub Actions
```

## Annotated Tag Strategy

### Namespace: `atmos/*` tags

All Atmos tags use the `atmos/` prefix:

```
refs/tags/
  atmos/
    deployments/
      payment-service/      # Deployment name
        prod/               # Target
          api/              # Component
            2025-01-22T10-30-00Z  # Deployment history tag
            2025-01-21T14-20-00Z
          worker/
            2025-01-22T10-30-00Z
        staging/
          api/
            2025-01-22T11-00-00Z
```

### Annotated Tags for Deployment History

**Purpose:** Immutable audit trail of all deployments with rich metadata.

**Characteristics:**
- Immutable (never changes)
- Rich structured metadata (YAML)
- Queryable history
- Sortable by timestamp
- Git-native (works everywhere)

**Tag Message Format:**
```yaml
atmos.tools/v1alpha1
kind: Deployment
---
deployment: payment-service
target: prod
component: api
deployed_by: ci@example.com
build_id: abc123
git_sha: abc123def456789
timestamp: 2025-01-22T10:30:00Z
status: success
metadata:
  pr: "#482"
  jira: PROJ-123
  approver: user@example.com
rollback_of: null
```

**Why this format:**
- ✅ Version prefix (`atmos.tools/v1alpha1`) enables schema evolution
- ✅ Kind field distinguishes deployment from build/release records
- ✅ YAML is human-readable, Atmos-native
- ✅ Structured enables validation and parsing
- ✅ Extensible (add fields without breaking)

**Example:**
```bash
# Create deployment tag (deployment → target → component)
git tag -a -m "atmos.tools/v1alpha1
kind: Deployment
---
deployment: payment-service
target: prod
component: api
deployed_by: ci@example.com
build_id: abc123
timestamp: 2025-01-22T10:30:00Z
status: success" \
atmos/deployments/payment-service/prod/api/2025-01-22T10-30-00Z abc123def

# View deployment history for a component
git tag -l "atmos/deployments/payment-service/prod/api/*" --sort=-creatordate

# View all prod deployments for this deployment
git tag -l "atmos/deployments/payment-service/prod/*" --sort=-creatordate

# Show deployment details
git cat-file -p atmos/deployments/payment-service/prod/api/2025-01-22T10-30-00Z
```

**Implementation:**
```go
// pkg/git/tags.go
type DeploymentMetadata struct {
    APIVersion string            `yaml:"-"` // atmos.tools/v1alpha1
    Kind       string            `yaml:"kind"`
    Deployment string            `yaml:"deployment"`
    Target     string            `yaml:"target"`
    Component  string            `yaml:"component"`
    DeployedBy string            `yaml:"deployed_by"`
    BuildID    string            `yaml:"build_id"`
    GitSHA     string            `yaml:"git_sha"`
    Timestamp  time.Time         `yaml:"timestamp"`
    Status     string            `yaml:"status"`
    Metadata   map[string]string `yaml:"metadata,omitempty"`
    RollbackOf *string           `yaml:"rollback_of,omitempty"`
}

func CreateDeploymentTag(metadata *DeploymentMetadata) error {
    // Marshal YAML
    yamlBytes, _ := yaml.Marshal(metadata)
    message := fmt.Sprintf("%s\nkind: %s\n---\n%s",
        metadata.APIVersion, metadata.Kind, string(yamlBytes))

    // Tag name with timestamp (deployment → target → component)
    tagName := fmt.Sprintf("atmos/deployments/%s/%s/%s/%s",
        metadata.Deployment, metadata.Target, metadata.Component,
        metadata.Timestamp.Format("2006-01-02T15-04-05Z"))

    return exec.Command("git", "tag", "-a", "-m", message, tagName, metadata.GitSHA).Run()
}

func ParseDeploymentTag(tagName string) (*DeploymentMetadata, error) {
    // Get tag message
    output, _ := exec.Command("git", "cat-file", "-p", tagName).Output()

    // Split at blank line (tag header | message)
    parts := strings.Split(string(output), "\n\n")
    message := parts[1]

    // Parse version prefix
    lines := strings.Split(message, "\n")
    apiVersion := strings.TrimSpace(lines[0])

    // Parse YAML (skip version line and kind line)
    yamlContent := strings.Join(lines[2:], "\n") // Skip "kind: Deployment" and "---"

    var metadata DeploymentMetadata
    yaml.Unmarshal([]byte(yamlContent), &metadata)
    metadata.APIVersion = apiVersion

    return &metadata, nil
}

func GetDeploymentHistory(deployment, target, component string) ([]*DeploymentMetadata, error) {
    pattern := fmt.Sprintf("atmos/deployments/%s/%s/%s/*", deployment, target, component)
    output, _ := exec.Command("git", "tag", "-l", pattern, "--sort=-creatordate").Output()

    var history []*DeploymentMetadata
    for _, tagName := range strings.Split(string(output), "\n") {
        if tagName == "" {
            continue
        }
        metadata, err := ParseDeploymentTag(tagName)
        if err == nil {
            history = append(history, metadata)
        }
    }
    return history, nil
}

func GetTargetHistory(deployment, target string) ([]*DeploymentMetadata, error) {
    // Get all components for this deployment/target
    pattern := fmt.Sprintf("atmos/deployments/%s/%s/*/*", deployment, target)
    output, _ := exec.Command("git", "tag", "-l", pattern, "--sort=-creatordate").Output()

    var history []*DeploymentMetadata
    for _, tagName := range strings.Split(string(output), "\n") {
        if tagName == "" {
            continue
        }
        metadata, err := ParseDeploymentTag(tagName)
        if err == nil {
            history = append(history, metadata)
        }
    }
    return history, nil
}
```

### Schema Versioning

**API versions:**
```
atmos.tools/v1alpha1  - Initial implementation
atmos.tools/v1beta1   - After testing, before GA
atmos.tools/v1        - Stable API
atmos.tools/v2        - Breaking changes (if needed)
```

**Multiple Kinds:**
```yaml
# Deployment record
atmos.tools/v1alpha1
kind: Deployment

# Build record
atmos.tools/v1alpha1
kind: Build

# Release record
atmos.tools/v1alpha1
kind: Release
```

## Git Notes Strategy

### Namespace: `refs/notes/atmos/*`

All Atmos notes use the `atmos/` prefix:

```
refs/notes/
  atmos/
    payment-service/
      sbom              # SBOM data per commit
      test-results      # Test reports per commit
      approvals         # Approval records per commit
      scan-results      # Security scan results per commit
    inventory-service/
      sbom
      test-results
```

**Hierarchy:** `deployment → namespace`

Git notes are tied to commits, not targets/components, so the hierarchy is simpler.

### Git Notes for Per-Commit Metadata

**Purpose:** Attach metadata to commits after they exist (SBOM, test results, approvals).

**Characteristics:**
- Multiple namespaces (unlimited notes per commit)
- Added after commit creation
- Independent of deployments
- Tied to code (commit), not environment

**When to use notes vs tags:**

| Use Case | Storage | Why |
|----------|---------|-----|
| Current deployment | Lightweight ref | Mutable, protectable, fast lookup |
| Deployment history | Annotated tag (YAML) | Immutable, rich metadata, audit trail |
| SBOM | Git note | Per-commit, large data, not deployment-specific |
| Test results | Git note | Per-commit, can run tests after commit |
| Approvals | Git note | Per-commit, added after commit exists |
| Scan results | Git note | Per-commit, can scan after commit |

**Example:**
```bash
# Attach SBOM to commit (namespaced by deployment)
git notes --ref=atmos/payment-service/sbom add -F sbom.cdx.json abc123def

# Attach test results
git notes --ref=atmos/payment-service/test-results add -F junit.xml abc123def

# Attach approval
git notes --ref=atmos/payment-service/approvals add -m '{"approved_by":"user@example.com","approved_at":"2025-01-22T10:30:00Z"}' abc123def

# Retrieve SBOM
git notes --ref=atmos/payment-service/sbom show abc123def

# Push notes to remote
git push origin refs/notes/atmos/payment-service/sbom
git push origin refs/notes/atmos/payment-service/test-results
git push origin refs/notes/atmos/payment-service/approvals
```

**Implementation:**
```go
// pkg/git/notes.go
func AttachNote(deployment, namespace, commit, content string) error {
    ref := fmt.Sprintf("atmos/%s/%s", deployment, namespace)

    // Add note
    cmd := exec.Command("git", "notes", "--ref="+ref, "add", "-f", "-m", content, commit)
    if err := cmd.Run(); err != nil {
        return err
    }

    // Push notes
    return exec.Command("git", "push", "origin", "refs/notes/"+ref).Run()
}

func AttachNoteFromFile(deployment, namespace, commit, filePath string) error {
    ref := fmt.Sprintf("atmos/%s/%s", deployment, namespace)

    // Add note from file
    cmd := exec.Command("git", "notes", "--ref="+ref, "add", "-f", "-F", filePath, commit)
    if err := cmd.Run(); err != nil {
        return err
    }

    // Push notes
    return exec.Command("git", "push", "origin", "refs/notes/"+ref).Run()
}

func GetNote(deployment, namespace, commit string) (string, error) {
    ref := fmt.Sprintf("atmos/%s/%s", deployment, namespace)
    output, err := exec.Command("git", "notes", "--ref="+ref, "show", commit).Output()
    return string(output), err
}
```

### Production Usage Validation

**Industry precedent (Digital Frontiers):**
- Store JUnit reports under `refs/notes/junit`
- Store SonarQube findings under `refs/notes/sonarqube`
- Store SBOMs under `refs/notes/sbom`

**Atmos approach (namespaced by deployment):**
- Store SBOMs under `refs/notes/atmos/payment-service/sbom`
- Store test results under `refs/notes/atmos/payment-service/test-results`
- Store approvals under `refs/notes/atmos/payment-service/approvals`

**Their workflow:**
1. CI builds and tests code
2. Generates reports (JUnit XML, SonarQube JSON, SBOM)
3. Attaches reports to commit as git notes
4. Developers/QA retrieve reports for any commit

### Size Limits and Best Practices

**GitHub/GitLab limits:**
- ⚠️  GitHub warns at 50 MB
- ❌ GitHub blocks files > 100 MB
- ⚠️  Repository limit: 5 GB recommended

**Typical Atmos note sizes:**
- SBOM (CycloneDX JSON): 50 KB - 5 MB
- JUnit test results: 10 KB - 500 KB
- Security scan results: 100 KB - 5 MB

**All well under limits!** ✅

**Size policy:**
```go
func AttachNote(namespace, commit, content string) error {
    size := len(content)

    if size > 50*1024*1024 {
        return fmt.Errorf("note too large (%d MB), use external storage", size/1024/1024)
    }

    if size > 10*1024*1024 {
        log.Warn("Large note detected", "size_mb", size/1024/1024,
            "consider", "compression or external storage")
    }

    ref := fmt.Sprintf("atmos/%s", namespace)
    return exec.Command("git", "notes", "--ref="+ref, "add", "-f", "-m", content, commit).Run()
}
```

## Target Promotion

See **[target-promotion.md](./target-promotion.md)** for detailed documentation on promotion paths, validation, and workflows.

## Complete Workflow Example

### 1. Build and Deploy to Dev

```bash
# Build containers
atmos deployment build payment-service --target dev
# → Builds nixpack components (api, worker, etc.)
# → Generates SBOM
# → Attaches SBOM to commit as git note: refs/notes/atmos/payment-service/sbom
# → Output: Built images for api, worker, database

# Run tests
atmos deployment test payment-service --target dev
# → Runs tests in containers
# → Attaches test results as git note: refs/notes/atmos/payment-service/test-results

# Deploy to dev (all components)
atmos deployment rollout payment-service --target dev
# → Updates refs:
#   refs/atmos/deployments/payment-service/dev/api → abc123def
#   refs/atmos/deployments/payment-service/dev/worker → abc123def
#   refs/atmos/deployments/payment-service/dev/database → abc123def
# → Creates tags:
#   atmos/deployments/payment-service/dev/api/2025-01-22T10-30-00Z
#   atmos/deployments/payment-service/dev/worker/2025-01-22T10-30-00Z
#   atmos/deployments/payment-service/dev/database/2025-01-22T10-30-00Z
```

### 2. Promote to Staging

```bash
# Promote to staging (same build, all components)
atmos deployment rollout payment-service --target staging --promote-from dev
# → Validates promotion path (dev → staging allowed)
# → Updates refs:
#   refs/atmos/deployments/payment-service/staging/api → abc123def
#   refs/atmos/deployments/payment-service/staging/worker → abc123def
#   refs/atmos/deployments/payment-service/staging/database → abc123def
# → Creates tags with timestamp 2025-01-22T11-00-00Z
# → Same commit, different target, no rebuild
```

### 3. Promote to Production (with Approval)

```bash
# Promote to prod (requires approval)
atmos deployment rollout payment-service --target prod --promote-from staging
# → Validates promotion path (staging → prod allowed)
# → Requires approval (promotion.requires_approval: true)
# → Requests Atmos Pro approval
# → Waits for approval
# → Records approval as git note: refs/notes/atmos/payment-service/approvals
# → Updates all component refs to abc123def
# → Creates tags with timestamp 2025-01-22T12-00-00Z
```

### 4. Query Deployment State

```bash
# What's deployed to prod?
atmos deployment status payment-service --target prod
# Queries: refs/atmos/deployments/payment-service/prod/*
# Output:
# api      → abc123def (deployed 1h ago)
# worker   → abc123def (deployed 1h ago)
# database → abc123def (deployed 1h ago)

# Show deployment history for a component
atmos deployment history payment-service --target prod --component api
# Queries: git tag -l "atmos/deployments/payment-service/prod/api/*"
# Output:
# 2025-01-22 12:00:00  abc123  ci@example.com  success
# 2025-01-20 09:15:00  def456  ci@example.com  success

# Show SBOM for deployed commit
atmos deployment sbom payment-service --target prod
# Queries: refs/notes/atmos/payment-service/sbom for commit abc123def
# Output: CycloneDX JSON

# Compare prod vs staging
atmos deployment diff payment-service --from-target prod --to-target staging
# Compares commits referenced by refs
# Shows which components differ, file changes, commit log
```

### 5. Rollback

```bash
# List previous releases
atmos deployment history payment-service --target prod --component api

# Rollback to previous deployment
atmos deployment rollout payment-service --target prod --git-sha def456
# → Updates all component refs to def456
# → Creates tags with rollback_of: abc123
```

## Git Provider Integration

### Core Git Operations (Required)

All deployment tracking uses pure Git operations - works with ANY Git remote:

```go
// pkg/git/interface.go
type Repository interface {
    // Refs
    CreateRef(ref, sha string) error
    GetRef(ref string) (string, error)
    ListRefs(pattern string) ([]Ref, error)

    // Tags
    CreateAnnotatedTag(name, sha, message string) error
    ListTags(pattern string) ([]string, error)
    GetTagMessage(tag string) (string, error)

    // Notes
    AddNote(namespace, commit, content string) error
    GetNote(namespace, commit string) (string, error)

    // Remote operations
    Push(remote, refspec string) error
    Fetch(remote, refspec string) error
}
```

### Git Hosting Platform Features (Optional)

Optional enhancements when available:

```go
// pkg/gitprovider/interface.go
type Provider interface {
    // Platform detection
    DetectContext() (Context, error)
    GetPlatform() Platform
    IsAvailable() bool

    // Optional features - return nil if not supported
    GetCommentManager() CommentManager        // PR/MR comments
    GetStatusWriter() StatusWriter            // Commit statuses
    GetRefProtector() RefProtector            // Ref protection
    GetWorkflowDispatcher() WorkflowDispatcher // CI/CD workflows
}
```

**Platform-specific features:**
- PR/MR comments with deployment status
- Commit status checks
- Ref protection (GitHub rulesets, GitLab protected refs)
- Workflow dispatch for deployments

## Benefits

### No External Database

- ✅ Git is the single source of truth
- ✅ No database to maintain, backup, or scale
- ✅ Deployment history versioned with code
- ✅ Works offline (local git repository)

### Git-Native Security

- ✅ GitHub rulesets protect production refs
- ✅ GitLab protected refs prevent unauthorized changes
- ✅ GPG signatures for tags (optional)
- ✅ Audit trail via Git history

### Universal Compatibility

- ✅ Works with GitHub, GitLab, Bitbucket, self-hosted Git
- ✅ No vendor lock-in
- ✅ Standard Git operations
- ✅ CLI tools work identically everywhere

### Performance

- ✅ 10-20x faster than scanning entire repository
- ✅ Atomic ref updates (no merge conflicts)
- ✅ Local Git operations are fast
- ✅ Notes cached by Git

### Developer Experience

- ✅ Familiar Git commands
- ✅ Browse deployment history: `git tag -l "atmos/deployments/**"`
- ✅ Diff deployments: `git diff <sha1>..<sha2>`
- ✅ Rollback: point ref to previous commit

## See Also

- **[overview.md](./overview.md)** - Core concepts and definitions
- **[configuration.md](./configuration.md)** - Deployment schema
- **[target-promotion.md](./target-promotion.md)** - Target promotion paths and validation
- **[cli-commands.md](./cli-commands.md)** - Complete command reference
