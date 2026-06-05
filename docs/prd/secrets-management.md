# Atmos Secrets Management PRD

## Executive Summary

A GitOps-friendly, multi-cloud secrets management system for Atmos that provides a Vercel-like developer experience with explicit secret declarations, CRUD operations, and integration with the existing store infrastructure.

## Problem Statement

From the Deployments PRD (`docs/prd/deployments/problem-statement.md`, on the `origin/deployments-prd` branch):

> **Secrets sprawl:** Deploying to prod loads secrets from dev (because inheritance), staging (because inheritance), and prod (what we actually need). Result: Prod pipeline has dev secrets. Security audit: CRITICAL FINDING.

Additionally:
- No unified CLI for secret CRUD operations (`init`, `set`, `get`, `delete`, `pull`, `push`, `import`)
- No declarative registry of what secrets exist and where they're stored
- Chamber (historical solution) is AWS-only
- Stores are designed for terraform output persistence, not user-managed secrets

## Design Principles

1. **Vercel-like DX** - Simple CRUD: `atmos secret init`, `atmos secret set`, etc.
2. **GitOps-friendly** - Explicit declarations in YAML, not opaque provider state
3. **Cloud-native** - Each cloud gets optimized provider (SSM, Key Vault, GSM), not cross-cloud abstraction
4. **Zero-config where possible** - Sensible defaults, auto-generated paths
5. **Works with deployments** - Scoped to avoid secrets sprawl
6. **Works with component registry** - Not just Terraform, but all component types

## Stores vs Secrets - Two Concepts, One Backend Layer

Atmos exposes **two user-facing concepts** for external state — but they share a single
backend layer (the store registry) rather than forking a second one:

- **Stores** — the backend layer and the machine-to-machine concept.
- **Secrets** — a declarative/policy layer on top, gated to the `!secret` function and
  the `atmos secret` CLI. A store can be marked `secret: true` to make it a secret
  backend (see [Secret stores](#secret-stores-secret-true)).

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
# Declare what secrets exist (committed to git, in stack config)
components:
  terraform:
    api:
      secrets:
        vars:
          DATADOG_API_KEY:
            store: app-secrets       # backend a secret resolves from
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
| Backend config | `stores:` in atmos.yaml | `stores:` with `secret: true` (track 1) or `secrets.providers:` for SOPS (track 2) |
| Declaration | Implicit (write creates key) | Explicit (must declare before use, in stack config) |
| Validation | None (opaque) | Pre-flight validation of declarations |
| Masking | Manual | Automatic via I/O layer |

### Why Two Concepts (Not Two Backends)?

The two concepts differ in **lifecycle and tooling**, not in where bytes are stored:

1. **Different lifecycles** - Store values are populated by Terraform outputs and tied to Terraform workflow; secrets are provisioned manually and change rarely
2. **Different tooling** - Stores integrate with the Terraform workflow and the hooks system; secrets need a dedicated declarative registry + CRUD commands
3. **Different access** - `!store` reads any non-secret store; `!secret` is the *only* accessor for a `secret: true` store, enforcing the declarative registry

But the **backend layer is shared**: the store registry already implements
`Set/Get(stack, component, key)` against AWS SSM/ASM, Azure Key Vault, GCP Secret
Manager, Vault, Redis, and Artifactory. Secrets reuse it rather than maintaining a
parallel provider registry — see [Backend Architecture](#backend-architecture-two-tracks).

## Configuration Schema

### Backend Architecture (Two Tracks)

Not every secret backend fits a key-value store interface, so the backend layer has
two tracks. The imperative CRUD layer (`atmos secret …`) and the `!secret` function sit
on top of both — only the backend implementation differs.

| | Track 1 — store-backed | Track 2 — non-store |
|---|---|---|
| Backends | AWS SSM, AWS ASM, HashiCorp Vault, Azure Key Vault, GCP Secret Manager | SOPS (`age`/`aws-kms`/`gcp-kms`/`gpg`); vals-style loaders later |
| Shape | Remote KV, fits `Set/Get(stack, component, key)` | Git-committed encrypted **file**, edited imperatively |
| Config | `stores:` entry with `secret: true` | `secrets.providers:` entry |
| Resolution | Runtime; can participate in the hooks system | Decrypt-in-place; local file workflow |
| Implementation | One store-adapter in `pkg/secrets` over `pkg/store` | Native provider (`pkg/secrets/providers/sops.go`) |

#### Secret stores (`secret: true`)

A **store** becomes a secret backend by setting `secret: true`. This is *subsystem
membership*, distinct from the per-value `sensitive` data-handling mechanism (Terraform
`sensitive = true` / `SensitiveStore`) that a `secret: true` store uses internally to
encrypt and mask.

```yaml
# atmos.yaml — one registry; regular and secret stores side by side
stores:
  terraform-outputs:                   # regular store (machine outputs)
    type: aws-ssm-parameter-store
    options:
      region: us-east-1

  app-secrets:                         # TRACK 1: store-backed secret
    type: aws-secrets-manager
    secret: true                       # subsystem membership
    identity: aws/prod-secrets         # optional auth identity (top-level; resolved via pkg/auth)
    options:
      region: us-east-1
      prefix: atmos/secrets

# TRACK 2: non-store backend (SOPS) — defined under secrets.providers
secrets:
  providers:
    dev-sops:
      kind: sops/age                   # or: sops/aws-kms, sops/gcp-kms, sops/gpg
      spec:
        file: secrets/dev.enc.yaml
```

**Access rule:** `!store` against a `secret: true` store is an **error** ("use
`!secret`"). `!secret NAME` resolves via the declared registry to its backend. This
makes declarations mandatory-by-construction for secret stores and removes any
`!store`-vs-`!secret` ambiguity about whether a value is sensitive.

### Provider Kind Constants

The `cloud/thing` kinds below are the shared vocabulary. The store-backed kinds
(`aws/ssm`, `aws/asm`, `azure/keyvault`, `gcp/secretmanager`, `hashicorp/vault`)
correspond to **store types** in the registry (track 1); only the `sops/*` kinds are
**secrets-native provider** kinds (track 2).

```go
// pkg/secrets/kinds/kinds.go
package kinds

const (
    // AWS providers
    AWSSSM = "aws/ssm"    // AWS Systems Manager Parameter Store
    AWSASM = "aws/asm"    // AWS Secrets Manager

    // Azure providers
    AzureKeyVault = "azure/keyvault"

    // GCP providers
    GCPSecretManager = "gcp/secretmanager"

    // HashiCorp providers
    HashicorpVault = "hashicorp/vault"

    // SOPS providers (by encryption type)
    SOPSAge    = "sops/age"
    SOPSAwsKms = "sops/aws-kms"
    SOPSGcpKms = "sops/gcp-kms"
    SOPSGPG    = "sops/gpg"
)
```

### Store `type`/`kind` Compatibility

The existing `pkg/store/` uses a `type` field. The store registry accepts both the
legacy `type` and the new `cloud/thing` `kind` so all backends share one vocabulary
(track 1 stores and track 2 SOPS providers alike):

```yaml
# Legacy `type` — continues to work
stores:
  mystore:
    type: aws-ssm-parameter-store

# New `kind` (cloud/thing) — equivalent
stores:
  mystore:
    kind: aws/ssm
```

Update `pkg/store/registry.go` to support both:
```go
// Support both legacy "type" and new "kind" field
kind := storeConfig.Kind
if kind == "" {
    kind = mapLegacyType(storeConfig.Type)  // Translate old format
}
```

### Declarations Live in Stack Config ("global" = shared import)

For v1, secrets are declared **only in stack/component config** — there is no
`atmos.yaml`-level global `secrets.vars` block. This guarantees every secret has a
real `(stack, component, key)` coordinate, so the existing
`Store.Set(stack, component, key)` fits unchanged (no key-only write path needed).

A "global" secret is just a shared declaration **imported** wherever it is needed:

```yaml
# stacks/catalog/secrets/shared.yaml — a reusable declaration fragment
components:
  terraform:
    api:
      secrets:
        vars:
          ARTIFACTORY_TOKEN:
            description: "Artifactory access token for private packages"
            store: app-secrets       # track 1 (store-backed)
            required: true
          GITHUB_APP_KEY:
            description: "GitHub App private key for CI"
            sops: dev-sops           # track 2 (SOPS)
            required: true

# stacks/prod/api.yaml
import:
  - catalog/secrets/shared           # "global" = imported everywhere it is needed
```

Each declaration references its backend by name: `store: <name>` for a `secret: true`
store (track 1), or `sops: <name>` for a SOPS provider (track 2). The referenced
backend carries provider/region/prefix/identity, so the declaration stays terse.

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
            store: app-secrets
            required: true
          REDIS_URL:
            description: "Redis connection string"
            store: app-secrets
      vars:
        datadog_api_key: !secret DATADOG_API_KEY
        redis_url: !secret REDIS_URL
```

### Flexible Organization with `!include`

Declarations live in stack config, but the `vars` map can be pulled in from a file for
organization (and a shared file is how "global" declarations are reused):

```yaml
# stacks/catalog/secrets/shared.yaml - reusable, imported where needed
components:
  terraform:
    api:
      secrets:
        vars: !include secrets/shared.yaml

# stacks/prod/api.yaml - per-component includes
components:
  terraform:
    api:
      secrets:
        vars: !include secrets/api.yaml
```

## Inheritance Model

**Secrets follow standard Atmos inheritance** with these considerations:

1. **Backend config** - The `stores:` (incl. `secret: true`) and `secrets.providers:` blocks live only in `atmos.yaml`, not inheritable
2. **Secret declarations** - Inherit through stack hierarchy
3. **Scope awareness** - Deployments can restrict which secrets are loaded (addressing secrets sprawl)

```yaml
# _defaults.yaml (base)
secrets:
  vars:
    SHARED_TOKEN:
      store: app-secrets

# prod/_defaults.yaml (inherits)
secrets:
  vars:
    # Inherits SHARED_TOKEN
    PROD_DB_PASSWORD:
      store: app-secrets

# prod/api.yaml (inherits both)
components:
  terraform:
    api:
      secrets:
        vars:
          # Inherits SHARED_TOKEN, PROD_DB_PASSWORD
          API_SPECIFIC_KEY:
            store: app-secrets
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
- Writes values to configured provider
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

Remove a secret from the provider.

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
```text
STACK       COMPONENT  SECRET            PROVIDER   STATUS
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

Upload secrets from a local file to the provider (must be declared).

```bash
# Push from .env file (secrets must be declared)
atmos secret push --stack dev --component api --input .env

# Push specific file
atmos secret push --stack dev --component api --input .env.local
```

### `atmos secret import`

Import secrets from an env file for declared secrets.

```bash
# Import from .env file (sets values for declared secrets)
atmos secret import .env --stack prod --component api

# Import global secrets
atmos secret import .env.global

# Preview what would be imported (dry-run)
atmos secret import .env --stack prod --dry-run

# Import from stdin
cat .env | atmos secret import - --stack prod --component api

# Import from JSON format
atmos secret import secrets.json --stack prod --format json
```

**Behavior:**
- Parses env file (KEY=value format) or JSON/YAML
- For each key in the file:
  - If declared: sets value in configured provider
  - If not declared: warns and skips (maintains declarative registry principle)
- Supports `--dry-run` to preview changes
- Reports summary: X imported, Y skipped (undeclared)

**Difference from `push`:**
- `push` fails immediately on any undeclared key
- `import` warns and continues, importing what it can

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
2. Resolves value from configured provider
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

## Describe / List Behavior: Mask Secrets by Default

`atmos describe` (`component`, `stacks`, `affected`, `dependents`) and the `atmos list` family
(`values`, `vars`, `settings`, ...) are **inspection** commands. Today they resolve every YAML
function by default, which means resolving `!secret` requires live credentials for the configured
provider. This couples *inspecting a stack's shape* to *being able to authenticate to the secret
backend* — one of Atmos's most common friction points (e.g. you cannot diff or review a stack in
CI, or on a laptop without cloud access, without it failing on secret resolution).

**Behavior:** This behavior is driven by the **existing global `--mask` flag** (default
`true`) — no separate `--secrets` flag is introduced. The insight: in an *inspection*
command, if `--mask=true` the value would be redacted in the output anyway, so there is
no reason to retrieve it. So when `--mask=true` (the default), `describe`/`list` resolve
`!secret` to the existing `***MASKED***` placeholder **without contacting the provider** —
no credential acquisition is attempted, and the command succeeds with zero cloud access.
With `--mask=false`, real values are retrieved and revealed (requires access).

```bash
# Default (--mask=true) — no credentials needed; secrets render as ***MASKED***
atmos describe component api --stack=prod
atmos list values api --stack=prod

# Reveal real values (requires access to the secret provider)
atmos describe component api --stack=prod --mask=false
```

**The mask ⇒ skip-retrieval rule applies only to inspection commands** —
`describe` (`component`, `stacks`, `affected`, `dependents`) and the `list` family.

| Command class | `--mask=true` (default) | `--mask=false` |
|---------------|-------------------------|----------------|
| **Inspection** (`describe`, `list`) | `!secret` → `***MASKED***`, **no retrieval, no credentials** | Retrieve + reveal |
| **Value-producing** (`secret get`/`pull`/`push`, `terraform plan`/`apply`/`output`) | **Always retrieve** real values (needed to function); output is redacted | Retrieve + reveal |

`secret get`/`pull`/`push` and `terraform plan`/`apply`/`output` must produce the actual
value to do their job, so they always retrieve regardless of `--mask`; `--mask` only
controls whether the value is redacted in *display* output.

**Scope:** Only `!secret` (secret-bearing resolution) is masked-without-retrieval. Non-secret
functions (`!template`, `!env`, `!exec`, and plain `!terraform.output` / `!store` of non-secret
values) keep resolving normally. Values that are only *discovered* to be sensitive after
retrieval (sensitive Terraform outputs, SecureString stores) cannot be known to be sensitive
without retrieving, so they remain covered by their masking-after-retrieval PRDs
([sensitive-terraform-outputs](secrets-masking/sensitive-terraform-outputs.md),
[store-sensitivity](secrets-masking/store-sensitivity.md)).

### `--mask` (the single control)

`--mask` is the existing global boolean flag (default `true`, env `ATMOS_MASK`, config
`settings.terminal.mask.enabled`). It now carries the skip-retrieval semantics above for
inspection commands. There is no `--secrets` flag, no `ATMOS_SECRETS` env var, and no
`settings.secrets.reveal` config — those were folded into `--mask`.

- **Precedence:** CLI flag → ENV (`ATMOS_MASK`) → config (`settings.terminal.mask.enabled`) → default (`true`).
- **Composes with `--identity`:** `--identity` selects *which* credentials to use when you do want
  retrieval; `--mask=false --identity=<name>` authenticates and reveals.
- **Trade-off vs. a dedicated flag:** there is no longer a way to reveal a value in *one*
  command while keeping the global masker on elsewhere — to reveal, you disable masking
  with `--mask=false`.

### How it differs from existing controls

The mask ⇒ skip-retrieval behavior is distinct from the two pre-existing function controls:

| Control | Effect on `!secret` |
|---------|---------------------|
| `--process-functions=false` | Disables **all** YAML functions, not just secrets. |
| `--skip secret` | Leaves the **raw literal** `!secret NAME` in the output (no resolution). |
| `--mask=true` on inspection cmds *(this PRD)* | Resolves the function but substitutes `***MASKED***` **without retrieval**. |

### Implementation notes

- Reuse the existing `--mask` global flag (already wired through `pkg/flags` →
  `pkg/io`); no new flag registration. The `!secret` resolver reads the resolved mask
  state plus the command class (inspection vs value-producing).
- Thread the resolution mode into YAML-function processing
  (`ProcessCustomYamlTags` in `internal/exec/yaml_func_utils.go`) — carry it on the processing
  context (`schema.ConfigAndStacksInfo` / a resolution option) rather than a new positional
  param, consistent with the Options pattern.
- The `!secret` resolver (`pkg/secrets/` / `pkg/function/secret.go`) checks **first**: on an
  inspection command with masking enabled it returns `io.MaskReplacement` and returns early,
  before any provider or auth call.

**Testing:** unit test that an inspection command with `--mask=true` returns `***MASKED***` and
the provider mock is **never called** (proves the no-credentials path); unit test that
`--mask=false` resolves and registers with the masker; unit test that `secret get`/`terraform
output` retrieve even with `--mask=true`; precedence tests (flag > env > config > default);
golden-snapshot for `describe component` with a `!secret` var rendering masked output without
credentials.

## Provider Implementations

### Track 1: Store-backed (`secret: true` stores)

These backends fit the store interface (`Set/Get(stack, component, key)`), so they are
configured as `stores:` entries with `secret: true` and reused by the secrets layer via
a single store-adapter. The store types AWS SSM / Azure Key Vault / GCP Secret Manager
already exist in `pkg/store/`; **AWS Secrets Manager and HashiCorp Vault are added** as
new store types.

```yaml
stores:
  # AWS SSM Parameter Store — simple, cost-effective; SecureString when secret
  ssm-secrets:
    type: aws-ssm-parameter-store
    secret: true
    identity: aws/prod-admin
    options: { region: us-east-1, prefix: /atmos/secrets }

  # AWS Secrets Manager — structured/JSON secrets, rotation, larger values (NEW store type)
  asm-secrets:
    type: aws-secrets-manager
    secret: true
    identity: aws/prod-secrets
    options: { region: us-east-1, prefix: atmos/secrets }

  # HashiCorp Vault (NEW store type)
  vault-secrets:
    type: hashicorp-vault
    secret: true
    options: { url: https://vault.example.com, path: secret/data/atmos, auth_method: token }

  # Azure Key Vault
  azure-secrets:
    type: azure-key-vault
    secret: true
    identity: azure/prod-subscription
    options: { vault_url: https://myvault.vault.azure.net/ }

  # GCP Secret Manager
  gcp-secrets:
    type: google-secret-manager
    secret: true
    options: { project_id: my-project }
```

- Path generation reuses the store's existing namespacing: `{prefix}/{stack}/{component}/{secret_name}`.
- Encryption-at-rest + the sensitivity flag follow the [Store Sensitivity PRD](secrets-masking/store-sensitivity.md) — a `secret: true` store always writes the sensitive variant (e.g. SSM `SecureString`).

### Track 2: Non-store (SOPS)

SOPS is a git-committed encrypted **file** edited imperatively — it does not fit the
store interface, so it is configured under `secrets.providers:` and implemented as a
native provider (`pkg/secrets/providers/sops.go`).

```yaml
secrets:
  providers:
    dev-sops:
      kind: sops/age                     # or: sops/aws-kms, sops/gcp-kms, sops/gpg
      spec:
        file: secrets/dev.enc.yaml       # or: secrets/{stack}.enc.yaml
        age_recipients: age1...          # or from SOPS_AGE_RECIPIENTS env
```

- Best for: Git-committed secrets, local development.
- Encryption options: age (recommended), AWS KMS, GCP KMS, GPG.
- `atmos secret set/get` against a SOPS provider decrypts the file, mutates the key, and re-encrypts in place (vs. an API call for track 1).

## Integration Points

### I/O Layer (Automatic Masking)

All secret values are automatically registered with the masker:

```go
// When resolving !secret
value, err := provider.Get(secretPath)
if err == nil {
    // Registers strings and walks structured values (maps/lists) — shared with the
    // sensitive-terraform-outputs / store-sensitivity PRDs. Masks in all output.
    registerSensitiveValue(value)
}
```

### Auth Identity Integration

Store-backed secrets (track 1) authenticate through the Atmos auth system
(`pkg/auth`) instead of raw role ARNs. A `secret: true` store carries a top-level
`identity:` naming an entry in `auth.identities`; the secrets store-adapter resolves it
to credentials at the operation boundary:

```go
// secrets store-adapter, before a backend Get/Set
creds, err := authManager.GetCachedCredentials(ctx, store.Identity) // passive; Authenticate() if interactive
awsCfg := creds.ToAWSConfig()                                       // inject into the ASM/SSM/Vault client
```

```yaml
# atmos.yaml
auth:
  identities:
    aws/prod-secrets:
      kind: aws/assume-role
      # ...principal, via, etc.

stores:
  app-secrets:
    type: aws-secrets-manager
    secret: true
    identity: aws/prod-secrets    # ← top-level; resolved via pkg/auth (NOT under options)
    options: { region: us-east-1 }
```

**Identity precedence** (most specific wins):

```text
store/provider `identity:`                 # explicit backend identity (pins it)
  → component instance's effective identity # inherited when resolved in a component scope
  → --identity / ATMOS_IDENTITY            # standalone invocations (no component scope)
  → default identity (auth.identities.<name>.default: true)
```

**Inheritance:** when a store/provider does **not** pin an `identity:` and the secret is
resolved within a component instance (e.g. `!secret` in a component's vars, or
`atmos secret get NAME -s <stack> -c <component>`), it **inherits the component's
effective identity** — the same credentials the component runs under. The right scope's
secrets are read with the right scope's credentials by default, with no extra config.

Notes:
- `identity:` supersedes the stores' legacy `read_role_arn` / `write_role_arn`
  (which remain for back-compat, equivalent to an implicit `aws/assume-role` identity).
- There is **no** separate `settings.identity` field — the inherited identity *is* the
  component's runtime identity (itself derived from a component-level `auth:` override →
  `--identity` / `ATMOS_IDENTITY` → default identity). Standalone resolutions with no
  component scope fall back to `--identity` / `ATMOS_IDENTITY`, then the default identity.
- **SOPS (track 2):** only KMS-backed kinds (`sops/aws-kms`, `sops/gcp-kms`) take an
  `identity:` (to call KMS); `sops/age` and `sops/gpg` use local keys and need none.
- **No-auth inspection:** because `--mask=true` skips retrieval on inspection commands
  (see [Describe / List Behavior](#describe--list-behavior-mask-secrets-by-default)),
  no identity is resolved and **no credentials are required** to `describe`/`list`.
- `atmos secret get/set/...` accept the existing `--identity` flag / `ATMOS_IDENTITY`.
- **Observability (Phase 2):** because the precedence chain above is a cascade, the
  resolved identity must be observable. The retrieval pipeline logs the selected
  identity and the source that pinned it at INFO (e.g. `Using identity 'aws/prod-secrets'
  (store.identity) for secret 'DATADOG_API_KEY'`), surfaces it in `--dry-run` output, and
  explains the full 4-level chain (including nested-component inheritance) via
  `atmos secret get --explain`. This answers both "why can't I access this secret?"
  (debugging) and "which identity read which secret?" (security audit).

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

```text
pkg/secrets/
    secrets.go           # Service interface, types
    service.go           # SecretService implementation
    registry.go          # SecretRegistry (declaration tracking)
    resolver.go          # Secret value resolution
    validator.go         # Declaration validation
    providers/
        provider.go      # Provider interface (Set/Get/Delete/List-status)
        store.go         # Track 1: adapter fronting any `secret: true` store (pkg/store)
        sops.go          # Track 2: SOPS encrypted files (native, non-store)
        ...              # future non-store providers (e.g. vals-style loaders)
    errors.go            # Sentinel errors
    *_test.go

# Track 1 backends are NOT re-implemented here — SSM/ASM/Vault/Azure-KV/GCP-SM
# live in pkg/store/ as store types; providers/store.go adapts them.

cmd/secret/
    secret.go            # Main command, subcommand registration
    init.go              # atmos secret init
    set.go               # atmos secret set (alias: add)
    get.go               # atmos secret get
    delete.go            # atmos secret delete (alias: rm)
    list.go              # atmos secret list
    pull.go              # atmos secret pull
    push.go              # atmos secret push
    import.go            # atmos secret import
    validate.go          # atmos secret validate

pkg/function/
    secret.go            # !secret YAML function

pkg/schema/
    secrets.go           # Schema additions for secrets config
```

## Implementation Phases

### Phase 1: Core Infrastructure + Store-backed secrets (Track 1)
- Schema additions in `pkg/schema/` for secrets config + the `secret: true` store flag
- `pkg/store/`: add the top-level `secret: true` and `identity:` fields to `StoreConfig`,
  the `!store`-refusal access rule, identity resolution via `pkg/auth` (superseding
  `read_role_arn`/`write_role_arn`), and a `Delete(stack, component, key)` extension
  (`DeletableStore`)
- `pkg/store/`: add **AWS Secrets Manager** and **HashiCorp Vault** as store types
- `pkg/secrets/` package: service interface + the **store-adapter provider** over `pkg/store` (track 1)
- Path extraction for structured JSON secrets
- Update `pkg/store/registry.go` for `kind` field (legacy `type` compatibility)
- Secret declaration parsing from **stack config** (global = shared import)
- Basic validation
- Integration with auth identities for store access

### Phase 2: CLI Commands
- `cmd/secret/` command structure following **command registry pattern**
- Use **flag handler pattern** (`pkg/flags/`) for all command flags
- Component type resolution: auto-detect -> selector -> `--type` flag
- `init` command (interactive prompts for missing secrets)
- `set`, `get`, `delete` commands (with `add`/`rm` aliases)
- `--path` flag for structured secret extraction
- Identity-resolution observability: INFO log of the resolved identity + its source,
  `--dry-run` surfacing, and `--explain` to print the full precedence chain

### Phase 3: YAML Integration
- `!secret` YAML function in `pkg/function/`
- `path` modifier for JSON extraction
- `default` modifier for fallback values
- Auto-registration with I/O masker
- `--mask`-driven secret resolution on `describe`/`list` (mask without retrieval by default; see
  [Describe / List Behavior](#describe--list-behavior-mask-secrets-by-default))
- Pre-command validation hooks

### Phase 4: Developer Experience
- `list` command (show declarations + status)
- `pull`, `push` commands (local .env sync)
- `validate` command for CI
- Deployments integration (scoped secrets)

### Phase 5: Non-store backends (Track 2) + remaining store types
- **SOPS** native provider (`pkg/secrets/providers/sops.go`): `sops/age`, `sops/aws-kms`, `sops/gcp-kms`, `sops/gpg`
- Wire Azure Key Vault and GCP Secret Manager (existing store types) for `secret: true` use
- Future: vals-style read-only loaders as additional non-store providers

## Documentation Deliverables

### CLI Command Documentation (Docusaurus)

Location: `website/docs/cli/commands/secret/`

```text
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
    import.mdx                # atmos secret import
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
- `secret: true` store flag and the `!store`-refusal access rule (track 1)
- `secrets.providers` for non-store backends like SOPS (track 2)
- Stack-config secret declarations (`store:` / `sops:` references); "global" via import
- `identity` integration with auth
- Examples for each backend track

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
2. Configure a `secret: true` store in atmos.yaml
3. Declare secrets in stack config (`store:` reference)
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
- Key features: Multi-provider, path extraction, auth integration
- Getting started example
- Link to full documentation

### Schema Updates

Location: `pkg/datafetcher/schema/`

- Update JSON schema for `secrets` configuration
- Add `secrets.vars` to component schema
- Document all provider spec options

## Open Questions (Resolved)

1. **Schema collision** - Using `secrets.vars:` pattern (like Copilot's approach)
2. **Inheritance** - Follow standard Atmos inheritance, deployments scope to avoid sprawl
3. **Stores relationship** - **One backend layer, two concepts.** Store-backed secrets are `secret: true` stores in the shared store registry (track 1); only non-store backends like SOPS get a native secrets provider (track 2). `!store` is blocked on a `secret: true` store; `!secret` is the only accessor.
4. **File organization** - Support `!include` for flexible organization
5. **Component type ambiguity** - Auto-detect -> interactive selector -> `--type` flag
6. **Provider naming** - Use `kind` field with `cloud/thing` format (consistent with auth)
7. **Structured secrets** - Path extraction via `| path ".foo.bar"` modifier and `--path` CLI flag
8. **Provider auth** - Optional `identity` field on provider/store config
9. **Declaration scope** - Stack/component config only for v1; "global" is a shared import (every secret keeps a real `(stack, component, key)` coordinate, so `Store.Set` fits unchanged)
10. **Masking control** - No separate `--secrets` flag; the existing `--mask` drives it. On inspection commands `--mask=true` skips retrieval entirely (no credentials); value-producing commands always retrieve
11. **Flag vocabulary** - `secret: true` = subsystem membership (aligns with `!secret`); `sensitive` = the per-value data-handling mechanism it uses internally (Terraform `sensitive`, `SensitiveStore`)

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
- [Vercel Environment Variables](https://vercel.com/docs/cli/env) - CLI-first DX with `vercel env pull`
- [Turborepo env](https://turbo.build/repo/docs/crafting-your-repository/using-environment-variables) - Scoped env vars
- [Chamber](https://github.com/segmentio/chamber) - Service-based SSM secrets with `chamber import` from JSON/YAML
- [Doppler](https://docs.doppler.com/docs/importing-secrets) - Project/environment scoping with `doppler secrets upload` from .env/JSON/YAML
- [helmfile/vals](https://github.com/helmfile/vals) - Read-only configuration loader (20+ backends)
- [vercel-env-push](https://github.com/HiDeoo/vercel-env-push) - Third-party tool for pushing .env to Vercel (fills gap in official CLI)

### Import Functionality Prior Art

| Tool | Import Command | Formats | Notes |
|------|---------------|---------|-------|
| Doppler | `doppler secrets upload .env` | .env, JSON, YAML | Creates secrets directly |
| Chamber | `chamber import service file.json` | JSON, YAML | Imports from export format |
| Vercel | `vercel env pull` (pull only) | .env | No native push; third-party `vercel-env-push` |
| 1Password | `op inject < .env` | .env template | Template-based injection |

## References

- Deployments PRD: `docs/prd/deployments/` (origin/deployments-prd branch)
- I/O Handling Strategy PRD: `docs/prd/io-handling-strategy.md` (masking architecture)
- Auth Default Settings PRD: `docs/prd/auth-default-settings.md` (provider/defaults pattern)
- Existing store implementation: `pkg/store/`
- Auth implementation: `pkg/auth/`
- I/O masking: `pkg/io/masker.go`, `io.RegisterSecret()`
