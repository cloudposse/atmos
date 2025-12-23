# Atmos Secrets Management PRD

## Executive Summary

A GitOps-friendly, multi-cloud secrets management system for Atmos that provides a Vercel-like developer experience with explicit secret declarations, CRUD operations, and integration with the existing store infrastructure.

## Problem Statement

From the [Deployments PRD](docs/prd/deployments/problem-statement.md):

> **Secrets sprawl:** Deploying to prod loads secrets from dev (because inheritance), staging (because inheritance), and prod (what we actually need). Result: Prod pipeline has dev secrets. Security audit: CRITICAL FINDING.

Additionally:
- No unified CLI for secret CRUD operations (`init`, `set`, `get`, `delete`, `pull`, `push`)
- No declarative registry of what secrets exist and where they're stored
- Chamber (historical solution) is AWS-only
- Stores are designed for terraform output persistence, not user-managed secrets

## Design Principles

1. **Vercel-like DX** - Simple CRUD: `atmos secret init`, `atmos secret set`, etc.
2. **GitOps-friendly** - Explicit declarations in YAML, not opaque backend state
3. **Cloud-native** - Each cloud gets optimized backend (SSM, Key Vault, GSM), not cross-cloud abstraction
4. **Zero-config where possible** - Sensible defaults, auto-generated paths
5. **Works with deployments** - Scoped to avoid secrets sprawl
6. **Works with component registry** - Not just Terraform, but all component types

## Stores vs Secrets - Clear Separation

Atmos maintains **two distinct systems** for managing external state:

### Stores (`pkg/store/`)

Stores are designed for **machine-written, machine-read state** - primarily Terraform outputs that need to be shared between components. They're opaque key-value stores where Terraform writes values and other components read them.

```yaml
# Terraform component writes to store
outputs:
  vpc_id: !terraform.output vpc_id

# Another component reads from store
vars:
  vpc_id: !store my-store plat-ue1-prod vpc vpc_id
```

### Secrets (`pkg/secrets/`)

Secrets are designed for **human-managed configuration** - API keys, tokens, passwords that users provision and manage through CLI operations. They require explicit declaration (GitOps-friendly) and provide full CRUD operations.

```yaml
# Declare what secrets exist (committed to git)
secrets:
  vars:
    DATADOG_API_KEY:
      backend: aws/ssm
      required: true

# Use the secret (value resolved at runtime, never in git)
vars:
  api_key: !secret DATADOG_API_KEY
```

### Comparison

| Aspect | Stores | Secrets |
|--------|--------|---------|
| Purpose | State/output persistence | User configuration secrets |
| Updated by | Terraform outputs (`!terraform.output`) | Users via CLI (`atmos secret set`) |
| Scope | Terraform components | All component types |
| Listing | Not supported (opaque) | Required (declarative registry) |
| Interface | `!store`/`!store.get` functions | `!secret` function + CRUD CLI |
| Backend config | `stores:` in atmos.yaml | `secrets.backends:` in atmos.yaml |
| Declaration | Implicit (write creates key) | Explicit (must declare before use) |
| Validation | None (opaque) | Pre-flight validation of declarations |
| Masking | Manual | Automatic via I/O layer |

### Why Two Systems?

1. **Different lifecycles** - Store values change on every `terraform apply`; secrets change rarely (manual rotation)
2. **Different access patterns** - Stores need stack/component scoping for outputs; secrets may be global or scoped
3. **Different security models** - Store values are infrastructure state; secrets need audit trails and rotation policies
4. **Different tooling** - Stores integrate with Terraform workflow; secrets need standalone CRUD CLI
5. **Different backends** - Stores optimize for Terraform state backends; secrets optimize for secret managers with rotation/auditing

## Configuration Schema

### Backend Configuration (atmos.yaml only)

```yaml
# atmos.yaml
secrets:
  default_backend: aws/ssm

  backends:
    aws/ssm:
      kind: aws/ssm                    # cloud/thing format (consistent with auth)
      identity: aws/prod-admin         # Optional: use this auth identity
      options:
        region: us-east-1
        prefix: "/atmos/secrets"

    aws/asm:
      kind: aws/asm                    # AWS Secrets Manager
      identity: aws/prod-secrets       # Optional: different identity for ASM
      options:
        region: us-east-1

    sops:
      kind: sops/age                   # or: sops/aws-kms, sops/gcp-kms, sops/gpg
      options:
        file: secrets.enc.yaml

    vault:
      kind: hashicorp/vault
      options:
        url: https://vault.example.com

  # Global secret declarations
  vars: !include secrets/global.yaml
```

### Backend Kind Constants

```go
// pkg/secrets/kinds/kinds.go
package kinds

const (
    // AWS backends
    AWSSSM = "aws/ssm"    // AWS Systems Manager Parameter Store
    AWSASM = "aws/asm"    // AWS Secrets Manager

    // Azure backends
    AzureKeyVault = "azure/keyvault"

    // GCP backends
    GCPSecretManager = "gcp/secretmanager"

    // HashiCorp backends
    HashicorpVault = "hashicorp/vault"

    // SOPS backends (by encryption type)
    SOPSAge    = "sops/age"
    SOPSAwsKms = "sops/aws-kms"
    SOPSGcpKms = "sops/gcp-kms"
    SOPSGPG    = "sops/gpg"
)
```

### Store Migration (Legacy Compatibility)

The existing `pkg/store/` uses `type` field. For backwards compatibility:

```yaml
# Old format (stores) - continue to work
stores:
  mystore:
    type: aws-ssm-parameter-store  # Legacy format

# New format (secrets) - uses kind
secrets:
  backends:
    aws/ssm:
      kind: aws/ssm                 # New cloud/thing format
```

Update `pkg/store/registry.go` to support both:
```go
// Support both legacy "type" and new "kind" field
kind := storeConfig.Kind
if kind == "" {
    kind = mapLegacyType(storeConfig.Type)  // Translate old format
}
```

### Global Secret Declarations

```yaml
# secrets/global.yaml (or inline under secrets.vars)
ARTIFACTORY_TOKEN:
  description: "Artifactory access token for private packages"
  backend: aws/ssm
  required: true

GITHUB_APP_KEY:
  description: "GitHub App private key for CI"
  backend: sops
  required: true
```

### Component-Level Secrets (Stack Files)

```yaml
# stacks/prod/api.yaml
components:
  terraform:
    api:
      secrets:
        vars:
          DATADOG_API_KEY:
            description: "Datadog API key for monitoring"
            backend: aws/ssm
            required: true
          REDIS_URL:
            description: "Redis connection string"
            backend: aws/ssm
      vars:
        datadog_api_key: !secret DATADOG_API_KEY
        redis_url: !secret REDIS_URL
```

### Flexible Organization with `!include`

```yaml
# atmos.yaml - include from files
secrets:
  vars: !include secrets/global.yaml

# stacks/prod/api.yaml - per-component includes
components:
  terraform:
    api:
      secrets:
        vars: !include secrets/api.yaml
```

## Inheritance Model

**Secrets follow standard Atmos inheritance** with these considerations:

1. **Backend config** - Only in `atmos.yaml`, not inheritable
2. **Secret declarations** - Inherit through stack hierarchy
3. **Scope awareness** - Deployments can restrict which secrets are loaded (addressing secrets sprawl)

```yaml
# _defaults.yaml (base)
secrets:
  vars:
    SHARED_TOKEN:
      backend: aws/ssm

# prod/_defaults.yaml (inherits)
secrets:
  vars:
    # Inherits SHARED_TOKEN
    PROD_DB_PASSWORD:
      backend: aws/ssm

# prod/api.yaml (inherits both)
components:
  terraform:
    api:
      secrets:
        vars:
          # Inherits SHARED_TOKEN, PROD_DB_PASSWORD
          API_SPECIFIC_KEY:
            backend: aws/ssm
```

## CLI Commands

### `atmos secret init`

Initialize/provision secrets for a stack or component.

```bash
# Initialize all secrets for a stack (interactive prompts)
atmos secret init --stack prod

# Initialize secrets for specific component (auto-detect type if unique)
atmos secret init api --stack prod

# If "api" exists in multiple component types -> interactive selector
atmos secret init api --stack prod
# -> "Multiple components named 'api' found. Select one:"
# -> [1] terraform/api
# -> [2] helmfile/api
# -> [3] nixpack/api

# Explicit type for CI/non-interactive
atmos secret init api --stack prod --type terraform

# Dry-run to see what would be initialized
atmos secret init --stack prod --dry-run
```

**Behavior:**
- Scans declarations for stack/component
- **Component resolution**: Auto-detect if unique, selector if ambiguous, `--type` for explicit
- Prompts interactively for each missing required secret
- Writes values to configured backend
- Skips already-initialized secrets (unless `--force`)

### `atmos secret set`

Set a secret value (create or update).

**Aliases:** `add`

```bash
# Set global secret
atmos secret set ARTIFACTORY_TOKEN

# Set component-scoped secret
atmos secret set DATADOG_API_KEY --stack prod --component api

# Non-interactive (for CI)
atmos secret set DATADOG_API_KEY="value" --stack prod --component api

# From stdin (for large values like keys)
cat private-key.pem | atmos secret set GITHUB_APP_KEY --stdin

# Force overwrite existing
atmos secret set DATADOG_API_KEY --force
```

### `atmos secret get`

Retrieve a secret value.

```bash
# Get global secret
atmos secret get ARTIFACTORY_TOKEN

# Get component-scoped secret
atmos secret get DATADOG_API_KEY --stack prod --component api

# Output formats
atmos secret get DATADOG_API_KEY --format json
atmos secret get DATADOG_API_KEY --format env
```

### `atmos secret delete`

Remove a secret from the backend.

**Aliases:** `rm`

```bash
# Delete global secret
atmos secret delete ARTIFACTORY_TOKEN

# Delete component-scoped secret
atmos secret delete DATADOG_API_KEY --stack prod --component api

# Force (no confirmation)
atmos secret delete DATADOG_API_KEY --force
```

### `atmos secret list`

List declared secrets and their status.

```bash
# List all secrets for a stack
atmos secret list --stack prod

# List secrets for a component
atmos secret list --stack prod --component api

# Show detailed status
atmos secret list --stack prod --verbose
```

Output:
```
STACK       COMPONENT  SECRET            BACKEND    STATUS
prod        (global)   ARTIFACTORY_TOKEN aws/ssm    initialized
prod        api        DATADOG_API_KEY   aws/ssm    initialized
prod        api        REDIS_URL         aws/ssm    missing
```

### `atmos secret pull`

Download secrets to a local file for development.

```bash
# Pull to .env file
atmos secret pull --stack dev --component api

# Custom output file
atmos secret pull --stack dev --component api --output .env.local

# Format options
atmos secret pull --stack dev --format json --output secrets.json
```

### `atmos secret push`

Upload secrets from a local file to the backend.

```bash
# Push from .env file
atmos secret push --stack dev --component api --input .env

# Push specific file
atmos secret push --stack dev --component api --input .env.local
```

### `atmos secret validate`

Validate all declared secrets are initialized.

```bash
# Validate stack
atmos secret validate --stack prod

# Validate component
atmos secret validate --stack prod --component api

# Exit codes for CI
# 0 = all required secrets initialized
# 1 = missing required secrets
```

## YAML Function: `!secret`

```yaml
# Reference a declared secret
vars:
  api_key: !secret DATADOG_API_KEY

# With default value
vars:
  api_key: !secret DATADOG_API_KEY | default "dev-key"

# Path extraction for structured/serialized secrets (JSON in ASM)
vars:
  db_host: !secret DATABASE_CONFIG | path ".host"
  db_port: !secret DATABASE_CONFIG | path ".port"
  db_password: !secret DATABASE_CONFIG | path ".credentials.password"

# Combine path with default
vars:
  db_host: !secret DATABASE_CONFIG | path ".host" | default "localhost"
```

**Behavior:**
1. Validates secret is declared in current scope (component + inherited)
2. Resolves value from configured backend
3. If `path` modifier: extracts nested value from JSON/structured data
4. Registers value with I/O masker for automatic redaction
5. Returns value for template substitution

**CLI Path Support:**
```bash
# Get specific path from structured secret
atmos secret get DATABASE_CONFIG --path ".host"
atmos secret get DATABASE_CONFIG --path ".credentials.password"

# Output full structure as JSON
atmos secret get DATABASE_CONFIG --format json
```

## Backend Implementations

### AWS SSM Parameter Store (`aws/ssm`)

```yaml
secrets:
  backends:
    aws/ssm:
      kind: aws/ssm
      identity: aws/prod-admin           # Optional auth identity
      options:
        region: us-east-1
        prefix: "/atmos/secrets"
```

- Path generation: `/{prefix}/{stack}/{component}/{secret_name}`
- Best for: Simple key-value secrets, cost-effective
- Limitations: 10KB max value size, no automatic rotation

### AWS Secrets Manager (`aws/asm`)

```yaml
secrets:
  backends:
    aws/asm:
      kind: aws/asm
      identity: aws/prod-secrets         # Optional auth identity
      options:
        region: us-east-1
        prefix: "atmos/secrets"
```

- Path generation: `{prefix}/{stack}/{component}/{secret_name}`
- Best for: Structured/JSON secrets, rotation, larger values
- Features: Automatic rotation, cross-account access, up to 64KB

### SOPS Encrypted File (`sops/*`)

```yaml
secrets:
  backends:
    sops-dev:
      kind: sops/age                     # or: sops/aws-kms, sops/gcp-kms, sops/gpg
      options:
        file: secrets.enc.yaml           # or: secrets/{stack}.enc.yaml
        age_recipients: age1...          # or from SOPS_AGE_RECIPIENTS env
```

- Best for: Git-committed secrets, local development
- Encryption options: age (recommended), AWS KMS, GCP KMS, GPG

### HashiCorp Vault (`hashicorp/vault`)

```yaml
secrets:
  backends:
    vault:
      kind: hashicorp/vault
      options:
        url: https://vault.example.com
        path: secret/data/atmos
        auth_method: token               # or: kubernetes, aws-iam
```

### Azure Key Vault (`azure/keyvault`)

```yaml
secrets:
  backends:
    azure:
      kind: azure/keyvault
      identity: azure/prod-subscription  # Optional auth identity
      options:
        vault_url: https://myvault.vault.azure.net/
```

### Google Secret Manager (`gcp/secretmanager`)

```yaml
secrets:
  backends:
    gcp:
      kind: gcp/secretmanager
      options:
        project_id: my-project
```

## Integration Points

### I/O Layer (Automatic Masking)

All secret values are automatically registered with the masker:

```go
// When resolving !secret
value, err := backend.Get(secretPath)
if err == nil {
    io.RegisterSecret(value)  // Masks in all output
}
```

### Auth Integration

Secrets can use component's auth identity:

```yaml
components:
  terraform:
    api:
      settings:
        identity: aws/prod-admin
      secrets:
        vars:
          API_KEY:
            backend: aws/ssm  # Uses aws/prod-admin credentials
```

### Deployments Integration

Deployments scope which secrets are loaded:

```yaml
# deployments/api.yaml
deployment:
  name: api
  stacks:
    - "prod/api"  # Only prod secrets loaded, not dev/staging
```

This addresses the "secrets sprawl" problem from the deployments PRD.

## Package Structure

```
pkg/secrets/
    secrets.go           # Service interface, types
    service.go           # SecretService implementation
    registry.go          # SecretRegistry (declaration tracking)
    resolver.go          # Secret value resolution
    validator.go         # Declaration validation
    backends/
        backend.go       # Backend interface
        ssm.go           # AWS SSM (wraps pkg/store)
        sops.go          # SOPS encrypted files
        vault.go         # HashiCorp Vault
        ...
    errors.go            # Sentinel errors
    *_test.go

cmd/secret/
    secret.go            # Main command, subcommand registration
    init.go              # atmos secret init
    set.go               # atmos secret set (alias: add)
    get.go               # atmos secret get
    delete.go            # atmos secret delete (alias: rm)
    list.go              # atmos secret list
    pull.go              # atmos secret pull
    push.go              # atmos secret push
    validate.go          # atmos secret validate

pkg/function/
    secret.go            # !secret YAML function

pkg/schema/
    secrets.go           # Schema additions for secrets config
```

## Implementation Phases

### Phase 1: Core Infrastructure + AWS Backends
- Schema additions in `pkg/schema/` for secrets config
- `pkg/secrets/kinds/` package with kind constants
- `pkg/secrets/` package with service interface and backend abstraction
- **AWS SSM backend** (`aws/ssm`) - reusing/extending `pkg/store/` code
- **AWS Secrets Manager backend** (`aws/asm`) - new implementation
- Path extraction for structured JSON secrets
- Update `pkg/store/registry.go` for `kind` field (legacy `type` compatibility)
- Secret declaration parsing from atmos.yaml and stacks
- Basic validation
- Integration with auth identities for backend access

### Phase 2: CLI Commands
- `cmd/secret/` command structure following **command registry pattern**
- Use **flag handler pattern** (`pkg/flags/`) for all command flags
- Component type resolution: auto-detect -> selector -> `--type` flag
- `init` command (interactive prompts for missing secrets)
- `set`, `get`, `delete` commands (with `add`/`rm` aliases)
- `--path` flag for structured secret extraction

### Phase 3: YAML Integration
- `!secret` YAML function in `pkg/function/`
- `path` modifier for JSON extraction
- `default` modifier for fallback values
- Auto-registration with I/O masker
- Pre-command validation hooks

### Phase 4: Developer Experience
- `list` command (show declarations + status)
- `pull`, `push` commands (local .env sync)
- `validate` command for CI
- Deployments integration (scoped secrets)

### Phase 5: Additional Backends
- SOPS encrypted file backends (`sops/age`, `sops/aws-kms`, etc.)
- HashiCorp Vault backend
- Azure Key Vault, GCP Secret Manager

## Documentation Deliverables

### CLI Command Documentation (Docusaurus)

Location: `website/docs/cli/commands/secret/`

```
website/docs/cli/commands/secret/
    _category_.json           # Sidebar category config
    index.mdx                 # Overview: atmos secret
    init.mdx                  # atmos secret init
    set.mdx                   # atmos secret set (alias: add)
    get.mdx                   # atmos secret get
    delete.mdx                # atmos secret delete (alias: rm)
    list.mdx                  # atmos secret list
    pull.mdx                  # atmos secret pull
    push.mdx                  # atmos secret push
    validate.mdx              # atmos secret validate
```

Each file follows the mandatory documentation requirements:
- Frontmatter (title, sidebar_label, id, description)
- Intro component
- Screengrab component
- Usage section with shell code block
- Arguments/Flags with `<dl><dt>` format
- Examples section

### Configuration Documentation

**atmos.yaml secrets config:**
Location: `website/docs/core-concepts/configuration/secrets.mdx`

Contents:
- `secrets.default_backend` configuration
- `secrets.backends` with all supported kinds
- `secrets.vars` for global declarations
- `identity` integration with auth
- Examples for each backend type

**Stack secrets config:**
Location: `website/docs/core-concepts/stacks/secrets.mdx`

Contents:
- Component-level `secrets.vars` declarations
- Inheritance behavior
- `!secret` YAML function usage
- Path extraction for structured secrets

### Tutorials

**Getting Started with Secrets (AWS SSM):**
Location: `website/docs/tutorials/secrets-aws-ssm.mdx`

Contents:
1. Prerequisites (AWS account, IAM permissions)
2. Configure backend in atmos.yaml
3. Declare secrets in stack config
4. Initialize secrets with `atmos secret init`
5. Use `!secret` in component vars
6. Verify with `atmos secret list`

### Blog Announcement

Location: `website/blog/YYYY-MM-DD-secrets-management.mdx`

```yaml
---
slug: secrets-management
title: "Introducing Secrets Management in Atmos"
authors: [cloudposse]
tags: [feature]
---
```

Contents:
- Problem: Managing secrets across stacks and components
- Solution: Declarative secrets with CRUD CLI
- Key features: Multi-backend, path extraction, auth integration
- Getting started example
- Link to full documentation

### Schema Updates

Location: `pkg/datafetcher/schema/`

- Update JSON schema for `secrets` configuration
- Add `secrets.vars` to component schema
- Document all backend options

## Open Questions (Resolved)

1. **Schema collision** - Using `secrets.vars:` pattern (like Copilot's approach)
2. **Inheritance** - Follow standard Atmos inheritance, deployments scope to avoid sprawl
3. **Stores relationship** - Keep both; stores for outputs, secrets for user config
4. **File organization** - Support `!include` for flexible organization
5. **Component type ambiguity** - Auto-detect -> interactive selector -> `--type` flag
6. **Backend naming** - Use `kind` field with `cloud/thing` format (consistent with auth)
7. **Structured secrets** - Path extraction via `| path ".foo.bar"` modifier and `--path` CLI flag
8. **Backend auth** - Optional `identity` field on backend config

## Alternatives Considered

### helmfile/vals as Go SDK

[helmfile/vals](https://github.com/helmfile/vals) is a configuration values loader supporting 20+ backends (SSM, ASM, Vault, Azure KV, GCP SM, SOPS, 1Password, Doppler, etc.).

**Why ruled out:**
- **Read-only** - vals is designed for retrieving/rendering secrets, not CRUD operations
- No write/create/update/delete capabilities
- Our primary need (`init`, `add`, `rm`) requires write operations
- We already have `pkg/store/` with SSM, Azure KV, GCP SM implementations

**Decision:** Build on `pkg/store/` which already has write capabilities. Vals could be a future consideration for expanding read-only backend support.

## Prior Art

- [AWS Copilot secret init](https://aws.github.io/copilot-cli/docs/commands/secret-init/) - YAML-based declarations
- [Vercel Environment Variables](https://vercel.com/docs/environment-variables) - CLI-first DX
- [Turborepo env](https://turbo.build/repo/docs/crafting-your-repository/using-environment-variables) - Scoped env vars
- [Chamber](https://github.com/segmentio/chamber) - Service-based SSM secrets
- [Doppler](https://docs.doppler.com/) - Project/environment scoping
- [helmfile/vals](https://github.com/helmfile/vals) - Read-only configuration loader (20+ backends)

## References

- Deployments PRD: `docs/prd/deployments/` (origin/deployments-prd branch)
- Existing store implementation: `pkg/store/`
- Auth implementation: `pkg/auth/`
- I/O masking: `pkg/io/masker.go`
