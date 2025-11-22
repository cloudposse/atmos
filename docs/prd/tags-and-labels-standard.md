# PRD: Tags and Labels Standard for Atmos

## Overview

This document establishes the official Atmos standard for using **tags** (lists) and **labels** (maps) consistently across the entire tool. This standard applies to all Atmos entities: components, stacks, workflows, vendors, auth identities, auth providers, and any future features.

**Scope**: Atmos-wide standard, not limited to authentication.

**Practical Examples**: This PRD uses auth identities and providers as primary examples, with auto-discovery from AWS IAM Identity Center PermissionSets demonstrating real-world usage.

## Terminology: Tags vs Labels (Data Structure Based)

Atmos distinguishes tags and labels by **data structure**, not naming preference:

| Term | Data Structure | Go Type | YAML Example | Purpose |
|------|---------------|---------|--------------|---------|
| **Tags** | **List/Array** | `[]string` | `tags: [production, admin]` | Simple categorization, quick filtering by presence |
| **Labels** | **Map/Dictionary** | `map[string]string` | `labels: {environment: production, team: platform}` | Rich key-value metadata, structured queries |

### Why This Distinction Matters

**Industry confusion**: AWS and Azure call them "tags" but they're structurally key-value maps (labels). GCP and Kubernetes correctly call key-value maps "labels".

**Atmos position**: Data structure determines terminology:
- If it's a list of strings → **tags**
- If it's a map of key-value pairs → **labels**

### Industry Usage Summary

Most cloud providers **misname** their data structures:

| Platform | What They Call It | Actual Data Structure | Atmos Terminology |
|----------|------------------|----------------------|-------------------|
| **AWS** | Tags | Map (key-value) | **Labels** |
| **Azure** | Tags | Map (key-value) | **Labels** |
| **GCP** | Labels | Map (key-value) | **Labels** ✅ |
| **GCP** | Tags | List (hierarchical) | **Tags** ✅ |
| **Kubernetes** | Labels | Map (key-value) | **Labels** ✅ |
| **Docker** | Labels | Map (key-value) | **Labels** ✅ |
| **Atmos Vendor** | Tags | List (strings) | **Tags** ✅ |

**Key Insight**: AWS and Azure call them "tags" but they're structurally key-value maps (should be "labels"). Atmos uses correct terminology based on data structure.

### AWS PermissionSet Tags → Atmos Labels

**AWS SSO PermissionSets have "tags"** (AWS terminology), but they are **key-value pairs**:

```yaml
# AWS Console / CloudFormation
PermissionSet:
  Name: AdministratorAccess
  Tags:  # AWS calls them "tags"
    - Key: environment       # But they're key-value pairs
      Value: production
    - Key: cost-center
      Value: engineering
```

**Atmos classification**: These are **labels** (maps), not tags (lists).

**Auto-discovery mapping**:
- AWS PermissionSet tags (key-value) → Atmos identity **labels** (map)
- Additionally, auto-generate Atmos identity **tags** (list) from label values for simple filtering

This allows users to filter by both:
- Simple tags: `atmos auth list --tags production`
- Rich labels: `atmos auth list --label environment=production`

### Decision: Support BOTH Tags and Labels

**Rationale**:
1. **Flexibility** - Users can choose based on needs (simple tags or rich labels)
2. **Cloud provider compatibility** - AWS PermissionSet tags (maps) → Atmos labels, with auto-generated tags for filtering
3. **Consistency** - Tags align with Atmos vendor pull, labels align with Kubernetes/Docker
4. **Future-proof** - Works with all cloud providers (AWS, Azure, GCP, K8s)

## Current State

### What We Have

**Vendor Sources with Tags** (`pkg/schema/schema.go`):
```go
type AtmosVendorSource struct {
    Tags []string `yaml:"tags" json:"tags" mapstructure:"tags"`
}
```

**Usage**:
```bash
atmos vendor pull --tags networking,storage
```

**Spacelift with Labels** (`pkg/spacelift/spacelift_stack_processor.go`):
```yaml
settings:
  spacelift:
    labels:
      - admin
      - admin-infrastructure-tenant1
```

### What We Don't Have

1. **No tags on auth identities** - Cannot categorize or filter identities
2. **No tags on auth providers** - Cannot categorize or filter providers
3. **No tag-based filtering in `atmos auth list`** - Shows all identities
4. **No tag-based filtering in `atmos auth login`** - Must specify exact identity name
5. **No tag-based filtering in `atmos auth logout`** - Must specify exact provider name
6. **No auto-discovery of tags** - Even when cloud resources have tags

## Proposed Schema Changes

### 1. Identity Schema (pkg/schema/schema_auth.go)

```go
type Identity struct {
    Kind        string                 `yaml:"kind" json:"kind" mapstructure:"kind"`
    Default     bool                   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`
    Via         *IdentityVia           `yaml:"via,omitempty" json:"via,omitempty" mapstructure:"via"`
    Principal   map[string]interface{} `yaml:"principal,omitempty" json:"principal,omitempty" mapstructure:"principal"`
    Credentials map[string]interface{} `yaml:"credentials,omitempty" json:"credentials,omitempty" mapstructure:"credentials"`
    Alias       string                 `yaml:"alias,omitempty" json:"alias,omitempty" mapstructure:"alias"`
    Env         []EnvironmentVariable  `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
    Tags        []string               `yaml:"tags,omitempty" json:"tags,omitempty" mapstructure:"tags"` // NEW
    Metadata    map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty" mapstructure:"metadata"` // NEW
}
```

**Rationale**:
- `Tags` - User-defined categorical metadata for filtering
- `Metadata` - System/discovery metadata (replaces ad-hoc metadata in auto-discovery)

### 2. Provider Schema (pkg/schema/schema_auth.go)

```go
type Provider struct {
    Kind                  string                 `yaml:"kind" json:"kind" mapstructure:"kind"`
    StartURL              string                 `yaml:"start_url,omitempty" json:"start_url,omitempty" mapstructure:"start_url"`
    URL                   string                 `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url"`
    Region                string                 `yaml:"region,omitempty" json:"region,omitempty" mapstructure:"region"`
    Username              string                 `yaml:"username,omitempty" json:"username,omitempty" mapstructure:"username"`
    Password              string                 `yaml:"password,omitempty" json:"password,omitempty" mapstructure:"password"`
    Driver                string                 `yaml:"driver,omitempty" json:"driver,omitempty" mapstructure:"driver"`
    ProviderType          string                 `yaml:"provider_type,omitempty" json:"provider_type,omitempty" mapstructure:"provider_type"` // Deprecated: use driver.
    DownloadBrowserDriver bool                   `yaml:"download_browser_driver,omitempty" json:"download_browser_driver,omitempty" mapstructure:"download_browser_driver"`
    Session               *SessionConfig         `yaml:"session,omitempty" json:"session,omitempty" mapstructure:"session"`
    Console               *ConsoleConfig         `yaml:"console,omitempty" json:"console,omitempty" mapstructure:"console"`
    Default               bool                   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`
    Spec                  map[string]interface{} `yaml:"spec,omitempty" json:"spec,omitempty" mapstructure:"spec"`
    Tags                  []string               `yaml:"tags,omitempty" json:"tags,omitempty" mapstructure:"tags"` // NEW
}
```

## Configuration Examples

### Manual Tags in atmos.yaml

```yaml
auth:
  providers:
    sso-prod:
      kind: aws/iam-identity-center
      start_url: https://my-org.awsapps.com/start
      region: us-east-1
      tags:
        - production
        - aws
        - sso
      auto_provision_identities: true
      spec:
        discovery:
          include_tags: true  # NEW: Include tags from AWS PermissionSets

    sso-dev:
      kind: aws/iam-identity-center
      start_url: https://dev-org.awsapps.com/start
      region: us-east-1
      tags:
        - development
        - aws
        - sso
      auto_provision_identities: true
      spec:
        discovery:
          include_tags: true

  identities:
    prod-admin:
      kind: aws/permission-set
      via:
        provider: sso-prod
      principal:
        name: AdministratorAccess
        account:
          name: production
      tags:
        - admin
        - production
        - elevated-access
      env:
        - key: TF_VAR_environment
          value: production

    dev-readonly:
      kind: aws/permission-set
      via:
        provider: sso-dev
      principal:
        name: ReadOnlyAccess
        account:
          name: development
      tags:
        - readonly
        - development
        - auditor
```

### Auto-Discovered Tags (Generated by Discovery)

When `discovery.include_tags: true` is enabled, discovered identities include tags from AWS PermissionSets:

```yaml
# {XDG_CACHE_HOME}/atmos/aws/sso-prod/discovered-identities.yaml
auth:
  identities:
    production/AdministratorAccess:
      kind: aws/permission-set
      via:
        provider: sso-prod
      principal:
        name: AdministratorAccess
        account:
          name: production
          id: "123456789012"
      # Auto-discovered tags from AWS PermissionSet
      tags:
        - admin
        - production
        - cost-center:engineering
        - compliance:sox
      metadata:
        discovered: true
        discovered_at: "2025-10-29T14:30:00Z"
        permission_set_arn: "arn:aws:sso:::permissionSet/ssoins-1234/ps-abcd1234"
        tags_source: "aws_permission_set"

    production/PowerUserAccess:
      kind: aws/permission-set
      via:
        provider: sso-prod
      principal:
        name: PowerUserAccess
        account:
          name: production
          id: "123456789012"
      tags:
        - poweruser
        - production
        - cost-center:engineering
      metadata:
        discovered: true
        discovered_at: "2025-10-29T14:30:00Z"
        permission_set_arn: "arn:aws:sso:::permissionSet/ssoins-1234/ps-efgh5678"
        tags_source: "aws_permission_set"
```

## CLI Command Enhancements

### 1. `atmos auth list` - Filter by Tags

**Syntax**:
```bash
atmos auth list [--tags TAG1,TAG2] [--tag-mode any|all]
```

**Flags**:
- `--tags` - Comma-separated list of tags to filter by
- `--tag-mode` - Filter mode: `any` (OR) or `all` (AND). Default: `any`

**Examples**:

```bash
# Show all identities
$ atmos auth list

# Show only production identities
$ atmos auth list --tags production

# Show admin identities in production (AND)
$ atmos auth list --tags admin,production --tag-mode all

# Show identities that are either admin OR readonly (OR)
$ atmos auth list --tags admin,readonly

# Show development identities
$ atmos auth list --tags development
```

**Example Output**:
```bash
$ atmos auth list --tags production

Providers (2):
  ✓ sso-prod (aws/iam-identity-center) [tags: production, aws, sso]
  ⚠ sso-dev (aws/iam-identity-center) [tags: development, aws, sso] - filtered out

Identities (23):
  Manual (1):
    ✓ prod-admin (production / AdministratorAccess) [tags: admin, production, elevated-access]

  Auto-Discovered (22):
    ✓ production/AdministratorAccess [tags: admin, production, cost-center:engineering, compliance:sox]
    ✓ production/PowerUserAccess [tags: poweruser, production, cost-center:engineering]
    ✓ production/ReadOnlyAccess [tags: readonly, production]
    ...

Filtered: Showing 23 of 47 identities (24 filtered by tags)
```

### 2. `atmos auth login` - Select by Tags

**Use Case**: Login with an identity matching specific tags without knowing exact name.

**Syntax**:
```bash
atmos auth login [--provider PROVIDER] [--identity IDENTITY] [--tags TAG1,TAG2] [--tag-mode any|all]
```

**Behavior**:
- If `--identity` specified: Use that identity (existing behavior)
- If `--tags` specified: Filter identities by tags, then:
  - **Single match**: Use that identity automatically
  - **Multiple matches**: Interactive selection (TUI picker)
  - **No matches**: Error with helpful message

**Examples**:

```bash
# Login with default identity for provider
$ atmos auth login --provider sso-prod

# Login with specific identity (existing behavior)
$ atmos auth login --provider sso-prod --identity production/AdministratorAccess

# Login with identity tagged as production + admin (single match = automatic)
$ atmos auth login --provider sso-prod --tags production,admin --tag-mode all
🔐 AWS SSO Authentication Required
Found 1 identity matching tags [production, admin]:
  → production/AdministratorAccess

Verification Code: ABCD-EFGH
Opening browser to: https://device.sso.us-east-1.amazonaws.com/...
✓ Authentication successful

# Login with identity tagged as production (multiple matches = interactive)
$ atmos auth login --provider sso-prod --tags production
🔐 AWS SSO Authentication Required
Found 23 identities matching tags [production]:

Select identity:
  ↓ production/AdministratorAccess [tags: admin, production, compliance:sox]
    production/PowerUserAccess [tags: poweruser, production]
    production/ReadOnlyAccess [tags: readonly, production]
    ...

[Use arrow keys to navigate, Enter to select, Esc to cancel]

# No matches
$ atmos auth login --provider sso-prod --tags nonexistent-tag
Error: No identities found matching tags [nonexistent-tag]

Available tags: admin, readonly, production, development, auditor, elevated-access
```

### 3. `atmos auth logout` - Logout by Provider Tags

**Use Case**: Logout from all providers matching specific tags (e.g., all production providers).

**Syntax**:
```bash
atmos auth logout [--provider PROVIDER] [--tags TAG1,TAG2] [--all]
```

**Behavior**:
- If `--provider` specified: Logout from that provider (existing behavior)
- If `--tags` specified: Logout from all providers matching tags
- If `--all` specified: Logout from all providers

**Examples**:

```bash
# Logout from specific provider (existing behavior)
$ atmos auth logout --provider sso-prod
✓ Logged out from sso-prod

# Logout from all production providers
$ atmos auth logout --tags production
Found 2 providers matching tags [production]:
  - sso-prod (aws/iam-identity-center)
  - okta-prod (saml)

Logout from 2 providers? [y/N]: y
✓ Logged out from sso-prod
✓ Logged out from okta-prod
✓ Removed 2 discovery files

# Logout from all providers
$ atmos auth logout --all
Found 5 authenticated providers:
  - sso-prod (aws/iam-identity-center)
  - sso-dev (aws/iam-identity-center)
  - okta-prod (saml)
  - azure-prod (azure-ad)
  - gcp-prod (workload-identity)

Logout from all providers? [y/N]: y
✓ Logged out from 5 providers
✓ Removed 5 discovery files
```

## Auto-Discovery Tag Support

### AWS IAM Identity Center PermissionSet Tags

AWS PermissionSets support tags, which can be retrieved using the SSO Admin API.

**Discovery Flow with Tags**:

1. **ListAccounts** - Get all accounts assigned to user
2. **ListAccountRoles** - Get all roles/permission sets for each account
3. **For each permission set** (NEW):
   - **DescribePermissionSet** - Get PermissionSet ARN
   - **ListTagsForResource** - Get tags for the PermissionSet
4. **Generate identity config** - Include tags in discovered identity

**Implementation** (`pkg/auth/providers/aws/sso_discovery.go`):

```go
// DiscoverIdentities discovers available identities from AWS SSO with optional tag retrieval.
func (p *ssoProvider) DiscoverIdentities(ctx context.Context, creds types.ICredentials) (*types.DiscoveryResult, error) {
    defer perf.Track(nil, "aws.ssoProvider.DiscoverIdentities")()

    awsCreds, ok := creds.(*types.AWSCredentials)
    if !ok {
        return nil, fmt.Errorf("%w: AWS credentials required for discovery", errUtils.ErrInvalidCredentials)
    }

    // Check if tag discovery is enabled
    includeTags := p.shouldIncludeTags()

    // Create SSO client for portal API (ListAccounts, ListAccountRoles)
    ssoClient, err := p.newSSOClientForDiscovery(ctx, awsCreds)
    if err != nil {
        return nil, err
    }

    // NEW: Create SSO Admin client for admin API (DescribePermissionSet, ListTagsForResource)
    var ssoAdminClient *ssoadmin.Client
    if includeTags {
        ssoAdminClient, err = p.newSSOAdminClientForDiscovery(ctx, awsCreds)
        if err != nil {
            log.Warn("Failed to create SSO Admin client, proceeding without tags", "error", err)
            includeTags = false // Graceful degradation
        }
    }

    // Discover accounts
    accounts, err := p.listAccounts(ctx, ssoClient, awsCreds.AccessKeyID)
    if err != nil {
        return nil, err
    }

    result := &types.DiscoveryResult{
        Identities:   make(map[string]*types.DiscoveredIdentity),
        Provider:     p.name,
        DiscoveredAt: time.Now(),
        Metadata: types.DiscoveryMetadata{
            TotalAccounts: len(accounts),
        },
    }

    // For each account, discover roles
    for _, account := range accounts {
        roles, err := p.listAccountRoles(ctx, ssoClient, account.AccountId, awsCreds.AccessKeyID)
        if err != nil {
            log.Warn("Failed to list roles for account", "account", account.AccountName, "error", err)
            continue
        }

        // For each role, create identity
        for _, role := range roles {
            identityName := p.generateIdentityName(account.AccountName, role.RoleName)

            // Build principal info
            principal := map[string]interface{}{
                "name": role.RoleName,
                "account": map[string]interface{}{
                    "name": account.AccountName,
                    "id":   account.AccountId,
                },
            }

            // Base metadata
            metadata := map[string]interface{}{
                "discovered":    true,
                "discovered_at": result.DiscoveredAt.Format(time.RFC3339),
            }

            // NEW: Discover tags if enabled
            var tags []string
            if includeTags && ssoAdminClient != nil {
                permissionSetTags, permSetARN, err := p.getPermissionSetTags(
                    ctx,
                    ssoAdminClient,
                    account.AccountId,
                    role.RoleName,
                )
                if err != nil {
                    log.Debug("Failed to get tags for permission set",
                        "account", account.AccountName,
                        "role", role.RoleName,
                        "error", err)
                    // Continue without tags - not a fatal error
                } else {
                    tags = permissionSetTags
                    metadata["permission_set_arn"] = permSetARN
                    metadata["tags_source"] = "aws_permission_set"
                }
            }

            result.Identities[identityName] = &types.DiscoveredIdentity{
                Kind: "aws/permission-set",
                Via: &types.IdentityVia{
                    Provider: p.name,
                },
                Principal: principal,
                Tags:      tags,     // NEW
                Metadata:  metadata,
            }
        }
    }

    result.Metadata.TotalIdentities = len(result.Identities)
    return result, nil
}

// shouldIncludeTags checks if tag discovery is enabled in provider config.
func (p *ssoProvider) shouldIncludeTags() bool {
    if p.config.Spec == nil {
        return false
    }
    discoverySpec, ok := p.config.Spec["discovery"].(map[string]interface{})
    if !ok {
        return false
    }
    includeTags, ok := discoverySpec["include_tags"].(bool)
    return ok && includeTags
}

// getPermissionSetTags retrieves tags for a PermissionSet using SSO Admin API.
// Returns tags, PermissionSet ARN, and error.
func (p *ssoProvider) getPermissionSetTags(
    ctx context.Context,
    ssoAdminClient *ssoadmin.Client,
    accountID string,
    roleName string,
) ([]string, string, error) {
    // Step 1: Get instance ARN from SSO instance (cached in provider)
    instanceARN := p.getInstanceARN()
    if instanceARN == "" {
        return nil, "", fmt.Errorf("SSO instance ARN not available")
    }

    // Step 2: List permission sets for the instance
    // We need to find the PermissionSet ARN that matches our roleName
    permSetARN, err := p.findPermissionSetARN(ctx, ssoAdminClient, instanceARN, roleName)
    if err != nil {
        return nil, "", fmt.Errorf("failed to find permission set ARN: %w", err)
    }

    // Step 3: Get tags for the PermissionSet
    tagsResp, err := ssoAdminClient.ListTagsForResource(ctx, &ssoadmin.ListTagsForResourceInput{
        ResourceArn: awssdk.String(permSetARN),
    })
    if err != nil {
        return nil, permSetARN, fmt.Errorf("failed to list tags: %w", err)
    }

    // Step 4: Convert AWS tags to string array
    tags := make([]string, 0, len(tagsResp.Tags))
    for _, tag := range tagsResp.Tags {
        // Format: "key:value" (consistent with label patterns in Spacelift)
        if tag.Value != nil && *tag.Value != "" {
            tags = append(tags, fmt.Sprintf("%s:%s", *tag.Key, *tag.Value))
        } else {
            // Tag with no value, just use key
            tags = append(tags, *tag.Key)
        }
    }

    return tags, permSetARN, nil
}

// newSSOAdminClientForDiscovery creates an AWS SSO Admin client for tag discovery.
// Note: SSO Admin API requires different permissions than SSO Portal API.
func (p *ssoProvider) newSSOAdminClientForDiscovery(ctx context.Context, awsCreds *types.AWSCredentials) (*ssoadmin.Client, error) {
    // Build config options
    configOpts := []func(*config.LoadOptions) error{
        config.WithRegion(p.region),
        config.WithCredentialsProvider(awssdk.CredentialsProviderFunc(func(ctx context.Context) (awssdk.Credentials, error) {
            return awssdk.Credentials{
                AccessKeyID:     awsCreds.AccessKeyID,
                SecretAccessKey: awsCreds.SecretAccessKey,
                SessionToken:    awsCreds.SessionToken,
            }, nil
        })),
    }

    // Add custom endpoint resolver if configured
    if resolverOpt := awsCloud.GetResolverConfigOption(nil, p.config); resolverOpt != nil {
        configOpts = append(configOpts, resolverOpt)
    }

    // Load config with isolated environment
    cfg, err := awsCloud.LoadIsolatedAWSConfig(ctx, configOpts...)
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config for SSO Admin API: %w", err)
    }

    return ssoadmin.NewFromConfig(cfg), nil
}

// findPermissionSetARN finds the PermissionSet ARN for a given role name.
func (p *ssoProvider) findPermissionSetARN(
    ctx context.Context,
    ssoAdminClient *ssoadmin.Client,
    instanceARN string,
    roleName string,
) (string, error) {
    // List all permission sets for the instance
    var permissionSetARN string
    paginator := ssoadmin.NewListPermissionSetsPaginator(ssoAdminClient, &ssoadmin.ListPermissionSetsInput{
        InstanceArn: awssdk.String(instanceARN),
    })

    for paginator.HasMorePages() {
        page, err := paginator.NextPage(ctx)
        if err != nil {
            return "", fmt.Errorf("failed to list permission sets: %w", err)
        }

        // For each permission set, describe it to get the name
        for _, psARN := range page.PermissionSets {
            descResp, err := ssoAdminClient.DescribePermissionSet(ctx, &ssoadmin.DescribePermissionSetInput{
                InstanceArn:      awssdk.String(instanceARN),
                PermissionSetArn: awssdk.String(psARN),
            })
            if err != nil {
                log.Debug("Failed to describe permission set", "arn", psARN, "error", err)
                continue
            }

            // Check if name matches
            if descResp.PermissionSet != nil && descResp.PermissionSet.Name != nil {
                if *descResp.PermissionSet.Name == roleName {
                    permissionSetARN = psARN
                    break
                }
            }
        }

        if permissionSetARN != "" {
            break
        }
    }

    if permissionSetARN == "" {
        return "", fmt.Errorf("permission set not found: %s", roleName)
    }

    return permissionSetARN, nil
}

// getInstanceARN retrieves the SSO instance ARN (cached from provider initialization).
func (p *ssoProvider) getInstanceARN() string {
    // Implementation: This should be cached during provider initialization
    // For now, simplified - in real implementation, this would be discovered
    // during initial SSO authentication and cached in the provider
    if p.config.Spec != nil {
        if instanceARN, ok := p.config.Spec["instance_arn"].(string); ok {
            return instanceARN
        }
    }
    return ""
}
```

### Discovery Config with Tag Support

```yaml
providers:
  sso-prod:
    kind: aws/iam-identity-center
    start_url: https://my-org.awsapps.com/start
    region: us-east-1
    auto_provision_identities: true
    spec:
      instance_arn: "arn:aws:sso:::instance/ssoins-1234567890abcdef"  # Optional: Cache instance ARN
      discovery:
        include_tags: true  # Enable tag discovery
        filters:
          accounts:
            - production
            - staging
          # NEW: Filter by tags
          tags:
            - admin
            - cost-center:engineering
          tag_mode: any  # any (OR) or all (AND)
```

## Implementation Plan

### Phase 1: Schema and Basic Tag Support

**Files to modify**:
- `pkg/schema/schema_auth.go` - Add `Tags []string` to Identity and Provider
- `pkg/auth/types/discovery.go` - Add `Tags []string` to DiscoveredIdentity
- `pkg/datafetcher/schema/atmos_auth_schema.json` - Update JSON schema

**Schema validation**:
- Tags must be string arrays
- Tags are optional (backward compatible)
- No duplicate tags (de-dupe automatically)

### Phase 2: CLI Filtering - `atmos auth list`

**Files to modify**:
- `cmd/auth_list/command.go` - Add `--tags` and `--tag-mode` flags
- `internal/exec/auth_list.go` - Implement tag filtering logic

**Implementation**:
```go
// pkg/auth/filter.go (NEW)
package auth

import "strings"

// TagMode defines how multiple tags are combined for filtering.
type TagMode string

const (
    TagModeAny TagMode = "any" // OR: Match if ANY tag matches
    TagModeAll TagMode = "all" // AND: Match if ALL tags match
)

// MatchesTags checks if a set of tags matches the filter.
func MatchesTags(tags []string, filterTags []string, mode TagMode) bool {
    if len(filterTags) == 0 {
        return true // No filter = match all
    }

    switch mode {
    case TagModeAll:
        // AND: All filter tags must be present
        for _, filterTag := range filterTags {
            if !contains(tags, filterTag) {
                return false
            }
        }
        return true

    case TagModeAny:
        fallthrough
    default:
        // OR: At least one filter tag must be present
        for _, filterTag := range filterTags {
            if contains(tags, filterTag) {
                return true
            }
        }
        return false
    }
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}

// FilterIdentitiesByTags filters identities by tags.
func FilterIdentitiesByTags(
    identities map[string]*schema.Identity,
    filterTags []string,
    mode TagMode,
) map[string]*schema.Identity {
    if len(filterTags) == 0 {
        return identities // No filter
    }

    filtered := make(map[string]*schema.Identity)
    for name, identity := range identities {
        if MatchesTags(identity.Tags, filterTags, mode) {
            filtered[name] = identity
        }
    }
    return filtered
}

// FilterProvidersByTags filters providers by tags.
func FilterProvidersByTags(
    providers map[string]*schema.Provider,
    filterTags []string,
    mode TagMode,
) map[string]*schema.Provider {
    if len(filterTags) == 0 {
        return providers // No filter
    }

    filtered := make(map[string]*schema.Provider)
    for name, provider := range providers {
        if MatchesTags(provider.Tags, filterTags, mode) {
            filtered[name] = provider
        }
    }
    return filtered
}
```

### Phase 3: CLI Filtering - `atmos auth login`

**Files to modify**:
- `cmd/auth_login/command.go` - Add `--tags` and `--tag-mode` flags
- `internal/exec/auth_login.go` - Implement tag-based identity selection

**Interactive Selection**:
```go
// pkg/auth/selector.go (NEW)
package auth

import (
    "fmt"

    "github.com/charmbracelet/bubbles/list"
    tea "github.com/charmbracelet/bubbletea"

    "github.com/cloudposse/atmos/pkg/schema"
)

// SelectIdentityByTags shows an interactive picker for identities matching tags.
func SelectIdentityByTags(
    identities map[string]*schema.Identity,
    filterTags []string,
    mode TagMode,
) (string, error) {
    // Filter identities
    filtered := FilterIdentitiesByTags(identities, filterTags, mode)

    switch len(filtered) {
    case 0:
        return "", fmt.Errorf("no identities found matching tags %v", filterTags)
    case 1:
        // Single match - return automatically
        for name := range filtered {
            return name, nil
        }
    default:
        // Multiple matches - show interactive picker
        return showIdentityPicker(filtered)
    }

    return "", fmt.Errorf("unexpected state in identity selection")
}

// showIdentityPicker shows an interactive TUI picker for identity selection.
func showIdentityPicker(identities map[string]*schema.Identity) (string, error) {
    // Build list items
    items := make([]list.Item, 0, len(identities))
    for name, identity := range identities {
        items = append(items, identityItem{
            name:     name,
            identity: identity,
        })
    }

    // Create list model
    delegate := list.NewDefaultDelegate()
    l := list.New(items, delegate, 80, 20)
    l.Title = "Select Identity"

    // Run TUI
    m := identityPickerModel{list: l}
    p := tea.NewProgram(m)
    result, err := p.Run()
    if err != nil {
        return "", fmt.Errorf("failed to show picker: %w", err)
    }

    if finalModel, ok := result.(identityPickerModel); ok {
        if finalModel.selected != "" {
            return finalModel.selected, nil
        }
    }

    return "", fmt.Errorf("no identity selected")
}

type identityItem struct {
    name     string
    identity *schema.Identity
}

func (i identityItem) Title() string       { return i.name }
func (i identityItem) Description() string {
    tags := ""
    if len(i.identity.Tags) > 0 {
        tags = fmt.Sprintf(" [tags: %s]", strings.Join(i.identity.Tags, ", "))
    }
    return fmt.Sprintf("%s%s", i.identity.Kind, tags)
}
func (i identityItem) FilterValue() string { return i.name }

type identityPickerModel struct {
    list     list.Model
    selected string
}

func (m identityPickerModel) Init() tea.Cmd {
    return nil
}

func (m identityPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "enter":
            if item, ok := m.list.SelectedItem().(identityItem); ok {
                m.selected = item.name
                return m, tea.Quit
            }
        case "esc", "ctrl+c":
            return m, tea.Quit
        }
    }

    var cmd tea.Cmd
    m.list, cmd = m.list.Update(msg)
    return m, cmd
}

func (m identityPickerModel) View() string {
    return m.list.View()
}
```

### Phase 4: CLI Filtering - `atmos auth logout`

**Files to modify**:
- `cmd/auth_logout/command.go` - Add `--tags` and `--all` flags
- `internal/exec/auth_logout.go` - Implement multi-provider logout with confirmation

### Phase 5: Auto-Discovery Tag Support

**Files to modify**:
- `pkg/auth/providers/aws/sso_discovery.go` - Add tag discovery logic
- `pkg/auth/providers/aws/sso.go` - Cache instance ARN during authentication
- `pkg/auth/types/discovery.go` - Add `Tags` field to `DiscoveredIdentity`
- `pkg/auth/config_writer.go` - Write tags to discovered config file

**AWS IAM Permissions Required**: See [IAM Permissions for Auto-Discovery](#iam-permissions-for-auto-discovery) section below for detailed breakdown.

### Phase 6: Tag-Based Discovery Filtering

**Discovery Config**:
```yaml
spec:
  discovery:
    include_tags: true
    filters:
      tags:
        - admin
        - cost-center:engineering
      tag_mode: any  # Only discover identities with these tags
```

**Implementation**: Filter discovered identities by tags before writing config.

## IAM Permissions for Auto-Discovery

### Overview

AWS IAM Identity Center uses **two different APIs** for auto-discovery:

1. **SSO Portal API** - User-facing API for discovering assigned accounts/roles
2. **SSO Admin API** - Administrative API for managing permission sets and retrieving tags

These APIs have **different authentication and permission models**.

### Authentication Models

| API | Authentication | Authorization | Typical Users |
|-----|----------------|---------------|---------------|
| **SSO Portal API** | Bearer token (from device flow) | Implicit (based on SSO assignments) | End users |
| **SSO Admin API** | AWS credentials (IAM) | IAM policies | Administrators |

**Key Insight**: SSO Portal API does NOT require IAM permissions - it uses bearer tokens issued during SSO authentication. Access is implicitly granted based on SSO permission set assignments.

### Level 1: Basic Auto-Discovery (Without Tags)

**Required APIs**: SSO Portal API only

**Permissions**: **NONE** (no IAM policy required)

**How it works**:
- User authenticates via AWS SSO device flow (`atmos auth login --provider sso-prod`)
- Receives bearer token from AWS SSO OIDC service
- Bearer token grants implicit access to `ListAccounts` and `ListAccountRoles` for assigned resources
- User can only see accounts/roles they've been assigned by SSO administrator

**Example Flow**:
```bash
# User authenticates
$ atmos auth login --provider sso-prod
🔐 AWS SSO Authentication Required
Verification Code: ABCD-EFGH
Opening browser to: https://device.sso.us-east-1.amazonaws.com/...

✓ Authentication successful
✓ Discovering available roles... (found 23 permission sets across 5 accounts)
✓ Generated ~/.cache/atmos/aws/sso-prod/discovered-identities.yaml (23 identities)
```

**APIs called** (using bearer token):
- `sso:ListAccounts` - Lists accounts assigned to user
- `sso:ListAccountRoles` - Lists roles/permission sets for each account
- `sso:GetRoleCredentials` - Gets temporary credentials for selected role

**No IAM policy required** because:
- These are user-facing Portal APIs
- Authorization is based on SSO permission set assignments, not IAM policies
- Bearer token inherently grants access to user's assigned resources
- User cannot see resources they haven't been assigned

### Level 2: Auto-Discovery WITH Tags

**Required APIs**: SSO Portal API + SSO Admin API

**Permissions**: IAM policy required for SSO Admin API

**IAM Policy Required**:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSOAdminReadOnlyForTagDiscovery",
      "Effect": "Allow",
      "Action": [
        "sso:ListInstances",
        "sso:ListPermissionSets",
        "sso:DescribePermissionSet",
        "sso:ListTagsForResource"
      ],
      "Resource": "*"
    }
  ]
}
```

**How it works**:
1. User authenticates via SSO (bearer token) - discovers accounts/roles (Level 1)
2. For each discovered role, Atmos calls SSO Admin API (using AWS credentials from role):
   - `sso:ListInstances` - Get SSO instance ARN
   - `sso:ListPermissionSets` - List permission sets for the instance
   - `sso:DescribePermissionSet` - Get permission set details (match role name)
   - `sso:ListTagsForResource` - Get tags for the permission set
3. Tags are included in discovered identity config

**Example Flow**:
```bash
$ atmos auth login --provider sso-prod --identity production/AdministratorAccess
✓ Authenticated as production/AdministratorAccess
✓ Discovering available roles with tags...
  → Found 23 permission sets across 5 accounts
  → Retrieving tags for 23 permission sets... (requires sso:ListTagsForResource)
  ✓ Retrieved tags for 18 permission sets
  ⚠ Failed to retrieve tags for 5 permission sets (insufficient permissions)
✓ Generated ~/.cache/atmos/aws/sso-prod/discovered-identities.yaml (23 identities, 18 with tags)
```

**APIs called**:
- SSO Portal API (bearer token): `ListAccounts`, `ListAccountRoles`
- SSO Admin API (IAM credentials): `ListInstances`, `ListPermissionSets`, `DescribePermissionSet`, `ListTagsForResource`

**Why IAM policy is required**:
- SSO Admin API requires AWS IAM credentials (not bearer tokens)
- These are administrative APIs for managing Identity Center configuration
- Typically requires elevated permissions (administrator or read-only admin role)

### Permission Boundaries and Least Privilege

**Option 1: User Has SSO Admin Read Permissions**

User's identity has IAM policy attached (directly or via permission set):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSOAdminReadOnlyForTagDiscovery",
      "Effect": "Allow",
      "Action": [
        "sso:ListInstances",
        "sso:ListPermissionSets",
        "sso:DescribePermissionSet",
        "sso:ListTagsForResource"
      ],
      "Resource": "*"
    }
  ]
}
```

**Result**: Full tag discovery works for all discovered permission sets.

**Option 2: User Lacks SSO Admin Permissions**

User only has SSO Portal API access (bearer token), no IAM policy for Admin API.

**Result**: Graceful degradation
- Basic auto-discovery works (Level 1)
- Tag discovery fails with `AccessDeniedException`
- Atmos logs warning and continues without tags
- User sees identities without tags in discovered config

**Example Output**:
```bash
$ atmos auth login --provider sso-prod
✓ Authenticated successfully
✓ Discovering available roles...
⚠ Warning: Unable to retrieve permission set tags (AccessDeniedException)
  → sso:DescribePermissionSet permission required
  → Continuing without tags...
✓ Generated ~/.cache/atmos/aws/sso-prod/discovered-identities.yaml (23 identities, no tags)
```

**Option 3: Separate Admin Role for Discovery**

Organization uses dedicated admin identity for tag discovery:

```yaml
providers:
  sso-prod:
    kind: aws/iam-identity-center
    start_url: https://my-org.awsapps.com/start
    region: us-east-1
    auto_provision_identities: true
    spec:
      discovery:
        include_tags: true
        # Use separate admin identity for tag discovery
        admin_identity: sso-admin-readonly
```

**Flow**:
1. User authenticates with `sso-prod` (gets bearer token)
2. Discovers accounts/roles via Portal API (bearer token)
3. For tag discovery, Atmos authenticates with `sso-admin-readonly` identity
4. Uses `sso-admin-readonly` credentials for SSO Admin API calls
5. Returns tags to original user's discovery result

**Benefit**: Separates user permissions from admin permissions, follows least privilege.

### Recommended IAM Policies

#### For End Users (No Tag Discovery)

**No IAM policy required** - SSO bearer token is sufficient.

#### For End Users (With Tag Discovery)

**Option A: Use AWS Managed Policy**

```yaml
# Attach to permission set or IAM role
AttachedManagedPolicyArns:
  - arn:aws:iam::aws:policy/AWSSSOReadOnly
```

**Option B: Custom Read-Only Policy (Least Privilege)**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSOAdminReadOnlyForTagDiscovery",
      "Effect": "Allow",
      "Action": [
        "sso:ListInstances",
        "sso:ListPermissionSets",
        "sso:DescribePermissionSet",
        "sso:ListTagsForResource"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:RequestedRegion": "us-east-1"
        }
      }
    }
  ]
}
```

**Option C: Instance-Scoped Policy (Most Restrictive)**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSOAdminReadOnlyForSpecificInstance",
      "Effect": "Allow",
      "Action": [
        "sso:ListPermissionSets",
        "sso:DescribePermissionSet",
        "sso:ListTagsForResource"
      ],
      "Resource": [
        "arn:aws:sso:::instance/ssoins-1234567890abcdef",
        "arn:aws:sso:::permissionSet/ssoins-1234567890abcdef/*"
      ]
    },
    {
      "Sid": "ListInstancesForDiscovery",
      "Effect": "Allow",
      "Action": "sso:ListInstances",
      "Resource": "*"
    }
  ]
}
```

#### For Administrators (Full SSO Admin Access)

Use AWS managed policy:
```yaml
AttachedManagedPolicyArns:
  - arn:aws:iam::aws:policy/AWSSSODirectoryAdministrator
```

### Permission Set Configuration Example

**PermissionSet with Tag Discovery Support**:

```yaml
# In AWS SSO, create a permission set
PermissionSetName: AtmosUserWithTagDiscovery
Description: Atmos user with SSO tag discovery support
SessionDuration: PT8H
ManagedPolicies:
  - arn:aws:iam::aws:policy/ReadOnlyAccess  # General read access
InlinePolicy:
  Version: "2012-10-17"
  Statement:
    - Sid: SSOAdminReadOnlyForTagDiscovery
      Effect: Allow
      Action:
        - sso:ListInstances
        - sso:ListPermissionSets
        - sso:DescribePermissionSet
        - sso:ListTagsForResource
      Resource: "*"
Tags:
  - Key: atmos-enabled
    Value: "true"
  - Key: cost-center
    Value: engineering
  - Key: compliance
    Value: sox
```

### Error Handling and Graceful Degradation

**Scenario 1: No SSO Admin Permissions**
```
⚠ Warning: Unable to retrieve permission set tags (AccessDeniedException)
  → Required permissions: sso:DescribePermissionSet, sso:ListTagsForResource
  → Continuing without tags...
✓ Generated discovered-identities.yaml (23 identities, no tags)
```

**Scenario 2: Partial Permissions (ListInstances denied)**
```
⚠ Warning: Unable to list SSO instances (AccessDeniedException)
  → Required permission: sso:ListInstances
  → Cannot retrieve tags without instance ARN
  → Continuing without tags...
✓ Generated discovered-identities.yaml (23 identities, no tags)
```

**Scenario 3: Permission Set Not Found**
```
⚠ Warning: Permission set 'AdministratorAccess' not found in SSO instance
  → Role may be externally managed or not configured in Identity Center
  → Continuing without tags for this identity...
✓ Generated discovered-identities.yaml (23 identities, 22 with tags)
```

### Summary Table

| Feature | SSO Portal API | SSO Admin API | IAM Policy Required | Falls Back Gracefully |
|---------|---------------|---------------|---------------------|----------------------|
| **Basic Discovery** | ✅ `ListAccounts`, `ListAccountRoles` | ❌ | ❌ No | N/A |
| **Tag Discovery** | ✅ (for accounts/roles) | ✅ `ListInstances`, `DescribePermissionSet`, `ListTagsForResource` | ✅ Yes | ✅ Yes (continues without tags) |
| **Authentication** | Bearer token (device flow) | AWS IAM credentials | N/A | N/A |
| **Authorization** | Implicit (SSO assignments) | Explicit (IAM policies) | N/A | N/A |
| **Typical Users** | All end users | Administrators or users with elevated read permissions | N/A | N/A |

### Key Takeaways

1. **Basic auto-discovery requires NO IAM permissions** - works for all SSO users
2. **Tag discovery requires SSO Admin read permissions** - typically admin-level access
3. **Graceful degradation** - tag discovery failures don't block authentication or basic discovery
4. **Separate concerns** - Portal API for user access, Admin API for configuration/metadata
5. **Least privilege** - Use read-only SSO Admin permissions, not full admin rights

## Benefits

### For Users

1. **Organized Identities**: Group identities by environment, access level, cost center, compliance requirements
2. **Faster Selection**: Filter and select identities by category instead of exact name
3. **Discoverability**: `atmos auth list --tags admin` shows all admin-level identities
4. **Reduced Errors**: Interactive selection prevents typos in identity names
5. **Cloud-Native**: Auto-discovered tags from cloud resources (PermissionSets)

### For Organizations

1. **Centralized Tagging**: Define tags in cloud provider (AWS SSO), propagate to Atmos automatically
2. **Compliance**: Filter by compliance tags (e.g., `compliance:sox`, `compliance:pci`)
3. **Cost Attribution**: Filter by cost center tags (e.g., `cost-center:engineering`)
4. **Access Control**: Filter by access level tags (e.g., `access-level:readonly`, `access-level:admin`)
5. **Multi-Environment**: Consistent tagging across production, staging, development

## Tag Naming Conventions (Recommended)

Based on Atmos vendor and Spacelift label patterns:

| Pattern | Example | Use Case |
|---------|---------|----------|
| Simple tag | `production`, `admin`, `readonly` | Basic categorization |
| Prefixed tag | `env:production`, `role:admin` | Namespaced categorization |
| Multi-level | `cost-center:engineering`, `compliance:sox` | Hierarchical organization |

**Best Practices**:
- Use lowercase kebab-case for simple tags: `production`, `read-only`
- Use colon separator for key-value tags: `env:production`, `cost-center:engineering`
- Keep tag keys consistent: `env:prod`, `env:staging`, `env:dev`
- Avoid spaces in tags (use dashes or underscores)
- Keep tags concise (prefer `env:prod` over `environment:production`)

## Backward Compatibility

All changes are **backward compatible**:
- Tags are optional fields (default: empty array)
- Existing configs without tags work unchanged
- New flags (`--tags`, `--tag-mode`) are optional
- Auto-discovery continues to work without `include_tags: true`

## Open Questions

1. **Should we support tag wildcards/patterns?**
   - Example: `atmos auth list --tags "env:*"` (match all env tags)
   - Decision: Defer to Phase 4 (future enhancement)

2. **Should we deduplicate tags automatically?**
   - Decision: Yes, dedupe in schema validation (consistent with Spacelift labels)

3. **Should we validate tag format?**
   - Decision: No strict validation initially, warn on non-standard formats

4. **Should metadata also be filterable?**
   - Decision: No, metadata is system/internal. Tags are user-facing for filtering.

5. **Should we cache SSO instance ARN?**
   - Decision: Yes, cache in provider spec during first authentication to avoid repeated lookups

## Future Enhancements

1. **Tag Autocomplete**: Shell completion for `--tags` flag based on available tags
2. **Tag Stats**: Show tag usage counts in `atmos auth list`
3. **Tag Validation**: Enforce organization-specific tag schemas
4. **Tag Inheritance**: Identities inherit provider tags automatically
5. **Tag-Based Policies**: OPA policies that reference identity/provider tags
6. **Tag Export**: Export tag inventory for reporting/compliance

## References

- **Vendor Pull Tags**: `pkg/schema/schema.go` (AtmosVendorSource.Tags)
- **Spacelift Labels**: `pkg/spacelift/spacelift_stack_processor.go`
- **AWS SSO API**: [ListAccountRoles](https://docs.aws.amazon.com/singlesignon/latest/PortalAPIReference/API_ListAccountRoles.html)
- **AWS SSO Admin API**: [DescribePermissionSet](https://docs.aws.amazon.com/singlesignon/latest/APIReference/API_DescribePermissionSet.html), [ListTagsForResource](https://docs.aws.amazon.com/singlesignon/latest/APIReference/API_ListTagsForResource.html)
- **AWS SDK Go v2 SSO Admin**: [ssoadmin package](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/ssoadmin)
