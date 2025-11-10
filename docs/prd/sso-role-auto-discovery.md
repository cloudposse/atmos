# AWS SSO Identity Auto-Provisioning

**Status**: Draft
**Last Updated**: 2025-11-09
**Related PRDs**: [Tags and Labels Standard](./tags-and-labels-standard.md)

---

## 1. Executive Summary

### Problem
Users must manually configure every AWS SSO permission set they want to use in Atmos, creating friction during onboarding and ongoing maintenance burden. For organizations with dozens of accounts and permission sets, this results in hundreds of lines of identity configuration that quickly becomes stale as SSO permissions change.

### Solution
Automatic provisioning of AWS SSO permission sets as Atmos identities. When enabled, logging in via `atmos auth login` queries AWS Identity Center APIs to provision all available permission sets across assigned accounts and generates a dynamic configuration file that seamlessly integrates with Atmos's existing config system.

### Value Proposition
- **Zero-config onboarding**: New users run `atmos auth login` and immediately see all identities they can use
- **Always up-to-date**: Provisioning runs on each login, automatically reflecting current SSO permissions
- **Self-service access**: Users discover their available roles without manual configuration
- **Reduced support burden**: Eliminates "how do I configure this role?" support requests

### Success Criteria
- **Time to first auth**: <2 minutes from clone to authenticated (vs 10+ minutes manual config)
- **User feedback**: "This just works" vs "I'm confused how to use this"

---

## 2. Problem Statement

### Current State: Manual Identity Configuration

Today, every AWS SSO permission set requires manual YAML configuration:

```yaml
# atmos.yaml
identities:
  production-admin:
    kind: aws/permission-set
    via:
      provider: sso-prod
    principal:
      name: AdministratorAccess
      account:
        name: production
        id: "123456789012"

  production-poweruser:
    kind: aws/permission-set
    via:
      provider: sso-prod
    principal:
      name: PowerUserAccess
      account:
        name: production
        id: "123456789012"

  staging-admin:
    kind: aws/permission-set
    via:
      provider: sso-prod
    principal:
      name: AdministratorAccess
      account:
        name: staging
        id: "234567890123"

  # ... repeat for every account + permission set combination
```

**Pain Points**:
- **Scalability**: 50 permission sets across 15 accounts = 750 lines of YAML to write
- **Onboarding friction**: New team members must manually configure every role they need
- **Maintenance burden**: Configs become stale when SSO permissions change
- **Discovery problem**: Users don't know what permission sets they have access to
- **Support overhead**: Constant "how do I configure X role?" questions

### User Personas

#### 1. DevOps Engineer (Sarah)
- **Needs**: Quick access to multiple accounts (dev, staging, prod)
- **Pain**: Spends 30 minutes configuring identities before first deployment
- **Wants**: `atmos auth login` → immediately usable

#### 2. New Team Member (Alex)
- **Needs**: Discover what access they've been granted
- **Pain**: Doesn't know which roles to configure, waits for team help
- **Wants**: Self-service discovery of available access

#### 3. Platform Team (Jordan)
- **Needs**: Minimize config maintenance across 100+ engineers
- **Pain**: Updates atmos.yaml when SSO permissions change, causing drift
- **Wants**: Automatic reflection of SSO state, no manual sync

### Requirements

#### Functional Requirements

**FR1**: Auto-provision permission sets
- **Given** a user with AWS SSO access to multiple accounts
- **When** they run `atmos auth login --provider sso-prod`
- **Then** system queries AWS Identity Center APIs and discovers all available permission sets

**FR2**: Populate identities without manual configuration
- **Given** auto-provisiony completed successfully
- **When** user runs `atmos auth list`
- **Then** all discovered permission sets appear as usable identities

**FR3**: Work seamlessly with existing commands
- **Given** provisioned identities exist
- **When** user runs any Atmos command with `--identity` flag
- **Then** provisioned identities work identically to manually configured ones

**FR4**: Support manual identity overrides
- **Given** both manual and provisioned identities with same name
- **When** config loading processes identities
- **Then** manual identity takes precedence (standard import merge)

**FR5**: Clean up on logout
- **Given** user is logged in with provisioned identities
- **When** user runs `atmos auth logout`
- **Then** provisioning cache is cleaned up automatically

#### Non-Functional Requirements

**NFR1**: No breaking changes
- Existing workflows must continue working
- Opt-in via provider-level feature flag
- Manual-only configurations unaffected

**NFR2**: Graceful degradation
- Provisioning failures don't block authentication
- Manual identities still work if provisioning fails
- Clear error messages for common issues (insufficient permissions, API throttling)

**NFR3**: Performance
- Provisioning completes in <10 seconds for 100 permission sets
- Uses parallel API calls where safe
- Respects AWS API rate limits with exponential backoff

**NFR4**: Security
- Access tokens never written to provisioning cache
- Provisioning file permissions: `0600` (owner read/write only)
- No credentials stored in provisioning file

---

## 3. Proposed Solution

### Design Philosophy: Dynamic Configuration Import

Instead of runtime-only identities or new commands, **enable auto-provisiony at the provider level with a feature flag**. Discovery generates a valid Atmos configuration file that is automatically imported during config loading.

**Key Principles**:
- **Treat provisioned identities like any other config**: Process through normal Atmos import chain
- **Require authentication**: Discovery only happens when logged in (no credentials, no discovery)
- **Support mixed manual/auto**: Manual identities coexist with discovered ones
- **No command pollution**: Works across all Atmos commands without new flags
- **Preserve existing behavior**: `atmos auth list` still works without login (shows manual identities)

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ atmos auth login --provider sso-prod                        │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
         ┌───────────────────────┐
         │ SSO Provider Auth     │
         │ (Device Flow)         │
         └───────────┬───────────┘
                     │
                     ▼
         ┌───────────────────────┐
         │ Check Feature Flag:   │
         │ auto_discover?        │
         └───────────┬───────────┘
                     │ yes
                     ▼
         ┌───────────────────────┐
         │ AWS Identity Center:  │
         │ - ListAccounts        │
         │ - ListAccountRoles    │
         └───────────┬───────────┘
                     │
                     ▼
         ┌───────────────────────────────────────┐
         │ Generate Dynamic Config:              │
         │ ~/.cache/atmos/aws/sso-prod/          │
         │   discovered-identities.yaml          │
         │                                       │
         │ auth:                                 │
         │   identities:                         │
         │     production/AdministratorAccess:   │
         │       kind: aws/permission-set        │
         │       ...                             │
         └───────────┬───────────────────────────┘
                     │
                     ▼
         ┌───────────────────────┐
         │ atmos terraform plan  │
         │   --identity prod/...  │
         └───────────┬───────────┘
                     │
                     ▼
         ┌───────────────────────┐
         │ Config Loading:       │
         │ 1. Load atmos.yaml    │
         │ 2. Process imports    │
         │ 3. Inject dynamic     │
         │    import (if exists) │
         └───────────┬───────────┘
                     │
                     ▼
         ┌───────────────────────┐
         │ Identity Available    │
         │ (no special handling) │
         └───────────────────────┘
```

### User Experience

#### Enable Auto-Provisioning

```yaml
# atmos.yaml
providers:
  sso-prod:
    kind: aws/iam-identity-center
    start_url: https://my-org.awsapps.com/start
    region: us-east-1
    spec:
      # Enable automatic identity provisioning
      auto_provision_identities: true

      # Optional: Configure provisioning behavior
      provisioning:
        cache_duration: 1h
        filters:
          accounts: ["production", "staging"]
          roles: ["AdministratorAccess", "PowerUserAccess"]
```

#### Login Triggers Provisioning

```bash
$ atmos auth login --provider sso-prod

🔐 AWS SSO Authentication Required
Verification Code: ABCD-EFGH
Opening browser to: https://device.sso.us-east-1.amazonaws.com/...

✓ Authentication successful
✓ Provisioning available roles... (found 47 permission sets across 15 accounts)
✓ Generated ~/.cache/atmos/aws/sso-prod/provisioned-identities.yaml (47 identities)
```

#### Use Provisioned Identities

```bash
# List all identities (manual + provisioned)
$ atmos auth list
 Authentication Configuration
└──sso-prod (aws/iam-identity-center)
   └──Identities
      ├──● production/AdministratorAccess (sso-prod) 11h58m
      │  └──Principal
      │     ├──name: AdministratorAccess
      │     └──account
      │        ├──name: production
      │        └──id: 123456789012
      ├──● production/PowerUserAccess (sso-prod) 11h58m
      │  └──Principal
      │     ├──name: PowerUserAccess
      │     └──account
      │        ├──name: production
      │        └──id: 123456789012
      ├──● staging/AdministratorAccess (sso-prod) 11h58m
      │  └──Principal
      │     ├──name: AdministratorAccess
      │     └──account
      │        ├──name: staging
      │        └──id: 234567890123
      └──... (44 more identities)

# Use any provisioned identity
$ atmos terraform plan component -s stack --identity production/AdministratorAccess
✓ Using identity: production/AdministratorAccess

# Works with all commands - no special handling
$ atmos helmfile sync component -s stack --identity staging/ReadOnlyAccess
$ atmos workflow run deploy -f workflow.yaml --identity production/PowerUserAccess
```

#### Logout Cleans Up

```bash
$ atmos auth logout --provider sso-prod
✓ Logged out from sso-prod
✓ Cleaned up provisioned identities cache
```

### Generated Provisioning File

```yaml
# ~/.cache/atmos/aws/sso-prod/provisioned-identities.yaml
# Auto-generated by Atmos identity provisioning
# Provider: sso-prod
# Provisioned: 2025-11-09T14:30:00Z
# DO NOT EDIT - This file is regenerated on each login
# Manual overrides should be placed in atmos.yaml

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

      # Tags: Auto-generated list for simple filtering
      tags:
        - production
        - engineering
        - admin

      # Labels: Key-value pairs from AWS PermissionSet tags
      labels:
        environment: production
        cost-center: engineering
        access-level: admin

      metadata:
        provisioned: true
        provisioned_at: "2025-11-09T14:30:00Z"
        permission_set_arn: "arn:aws:sso:::permissionSet/ssoins-1234/ps-abcd"

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
        - production
        - poweruser
      labels:
        environment: production
        access-level: poweruser
      metadata:
        provisioned: true
        provisioned_at: "2025-11-09T14:30:00Z"

    # ... 45 more identities ...
```

---

## 4. Use Cases

### Use Case 1: Zero-Config Onboarding

**Scenario**: New engineer joins team, needs to deploy to production

**Today (Manual)**:
1. Clone repo
2. Ask: "What identity should I configure?"
3. Wait for team response
4. Manually write identity YAML (10 minutes)
5. Run deployment

**With Auto-Provisioning**:
1. Clone repo
2. `atmos auth login --provider sso-prod` (2 minutes)
3. `atmos terraform plan component -s stack --identity production/AdministratorAccess`
4. Deploy immediately

**Time saved**: 30+ minutes, eliminates support request

### Use Case 2: Mixed Manual and Provisioned Identities

**Scenario**: Team wants auto-provisioning but needs custom env vars for specific identities

```yaml
# atmos.yaml - manual identity with customization
identities:
  prod-admin:  # Overrides provisioned "production/AdministratorAccess"
    kind: aws/permission-set
    via:
      provider: sso-prod
    principal:
      name: AdministratorAccess
      account:
        name: production
    env:
      - key: TF_VAR_environment
        value: production
      - key: CUSTOM_VAR
        value: special-value
```

**Result**:
- 46 identities auto-provisioned (low-touch)
- 1 identity manually configured (high-touch customization)
- Manual identity overrides discovered one (standard import precedence)

### Use Case 3: Selective Discovery with Filters

**Scenario**: Large org with 200+ permission sets, only need subset

```yaml
# atmos.yaml
providers:
  sso-prod:
    kind: aws/iam-identity-center
    start_url: https://my-org.awsapps.com/start
    region: us-east-1
    spec:
      auto_discover_identities: true
      discovery:
        filters:
          accounts: ["production", "staging"]  # Only these accounts
          roles: ["*Admin*", "*PowerUser*"]    # Only admin/poweruser roles
```

**Result**:
- Discovers 8 identities instead of 200
- Reduces API calls and file size
- Focuses on relevant permissions

### Use Case 4: Auth List Without Login

**Scenario**: User wants to see configured identities before logging in

```bash
# Before login - shows manual identities only
$ atmos auth list
 Authentication Configuration
└──sso-prod (aws/iam-identity-center)
   └──Identities
      └──⚠ prod-admin (sso-prod) NOT AUTHENTICATED
         └──Principal
            ├──name: AdministratorAccess
            └──account
               ├──name: production
               └──id: 123456789012

# After login - shows all identities
$ atmos auth login --provider sso-prod
✓ Provisioned 46 identities

$ atmos auth list
 Authentication Configuration
└──sso-prod (aws/iam-identity-center)
   └──Identities
      ├──● prod-admin (sso-prod) 11h58m
      │  └──Principal
      │     └──... (manual config with custom env vars)
      ├──● production/AdministratorAccess (sso-prod) 11h58m
      │  └──Principal
      │     └──... (auto-provisioned)
      └──... (45 more provisioned identities)
```

**Benefit**: Preserves existing behavior, provides helpful guidance

---

## 5. Technical Design

### 5.1 Provisioner Interface

```go
// pkg/auth/provisioning/provisioner.go
package provisioning

// Provisioner provisions identities from external sources.
// Different providers may implement this to auto-provision identities
// from their respective identity systems (AWS SSO, Azure AD, Okta, etc).
type Provisioner interface {
    // ProvisionIdentities queries the provider for available identities.
    // Returns provisioned identities and metadata about the provisioning operation.
    ProvisionIdentities(ctx context.Context, creds ICredentials) (*Result, error)
}

// Result contains the outcome of an identity provisioning operation.
type Result struct {
    Identities    map[string]*Identity  // Keyed by identity name
    Provider      string                // Provider name that performed provisioning
    ProvisionedAt time.Time            // When provisioning completed
    ExpiresAt     time.Time            // Optional expiration for cached results
    Metadata      Metadata             // Statistics about the operation
}

// Identity represents a single provisioned identity.
type Identity struct {
    Kind      string                 // Identity kind (e.g., "aws/permission-set")
    Via       *IdentityVia           // Provider reference
    Principal map[string]interface{} // Account + role info
    Tags      []string               // Simple list for filtering
    Labels    map[string]string      // Key-value pairs from cloud provider tags
    Metadata  map[string]interface{} // Provisioning metadata (timestamp, ARN, etc.)
}

// Metadata contains statistics about a provisioning operation.
type Metadata struct {
    AccountCount       int           // Number of accounts queried
    PermissionSetCount int           // Number of permission sets found
    APICallCount       int           // Number of API calls made
    Duration           time.Duration // Time taken to complete
}
```

### 5.2 AWS SSO Implementation

```go
// pkg/auth/providers/aws/sso_provisioning.go
package aws

import (
    "context"
    "fmt"
    "time"

    "github.com/cloudposse/atmos/pkg/auth/provisioning"
)

// ProvisionIdentities implements the Provisioner interface for AWS SSO.
// It queries AWS Identity Center APIs to discover all permission sets the user can access.
func (p *ssoProvider) ProvisionIdentities(ctx context.Context, creds ICredentials) (*provisioning.Result, error) {
    // 1. Create AWS SSO client using authenticated credentials
    ssoClient := sso.NewFromConfig(awsConfig)
    accessToken := creds.GetAccessToken()

    // 2. List all AWS accounts the user can access
    accounts, err := p.listAccounts(ctx, ssoClient, accessToken)
    if err != nil {
        return nil, fmt.Errorf("failed to list accounts: %w", err)
    }

    // 3. For each account, list available permission sets (roles)
    identities := make(map[string]*provisioning.Identity)
    apiCalls := 0
    startTime := time.Now()

    for _, account := range accounts {
        roles, err := p.listAccountRoles(ctx, ssoClient, accessToken, account.AccountID)
        apiCalls++
        if err != nil {
            log.Warn("Failed to list roles for account", "account", account.Name, "error", err)
            continue
        }

        // Create an identity for each permission set
        for _, role := range roles {
            identityName := p.generateIdentityName(account.Name, role.RoleName)
            identities[identityName] = &provisioning.Identity{
                Kind: "aws/permission-set",
                Via: &IdentityVia{Provider: p.name},
                Principal: map[string]interface{}{
                    "name": role.RoleName,
                    "account": map[string]interface{}{
                        "name": account.Name,
                        "id":   account.AccountID,
                    },
                },
                Metadata: map[string]interface{}{
                    "provisioned":    true,
                    "provisioned_at": time.Now(),
                },
            }
        }
    }

    return &provisioning.Result{
        Identities:    identities,
        Provider:      p.name,
        ProvisionedAt: time.Now(),
        Metadata: provisioning.Metadata{
            AccountCount:       len(accounts),
            PermissionSetCount: len(identities),
            APICallCount:       apiCalls,
            Duration:           time.Since(startTime),
        },
    }, nil
}

// AWS SDK API calls
func (p *ssoProvider) listAccounts(ctx context.Context, client *sso.Client, token string) ([]Account, error) {
    var accounts []Account
    input := &sso.ListAccountsInput{AccessToken: aws.String(token)}

    paginator := sso.NewListAccountsPaginator(client, input)
    for paginator.HasMorePages() {
        resp, err := paginator.NextPage(ctx)
        if err != nil {
            return nil, err
        }
        for _, acct := range resp.AccountList {
            accounts = append(accounts, Account{
                AccountID: aws.ToString(acct.AccountId),
                Name:      aws.ToString(acct.AccountName),
            })
        }
    }

    return accounts, nil
}

func (p *ssoProvider) listAccountRoles(ctx context.Context, client *sso.Client, token, accountID string) ([]Role, error) {
    var roles []Role
    input := &sso.ListAccountRolesInput{
        AccessToken: aws.String(token),
        AccountId:   aws.String(accountID),
    }

    paginator := sso.NewListAccountRolesPaginator(client, input)
    for paginator.HasMorePages() {
        resp, err := paginator.NextPage(ctx)
        if err != nil {
            return nil, err
        }
        for _, role := range resp.RoleList {
            roles = append(roles, Role{
                RoleName: aws.ToString(role.RoleName),
            })
        }
    }

    return roles, nil
}
```

### 5.3 Config Writer

```go
// pkg/auth/provisioning/writer.go
package provisioning

import (
    "fmt"
    "os"
    "path/filepath"
    "time"

    "gopkg.in/yaml.v3"
    "github.com/cloudposse/atmos/pkg/schema"
)

// WriteConfig writes provisioned identities to a YAML config file.
// The output file is a valid Atmos configuration that can be imported.
func WriteConfig(outputPath string, result *Result) error {
    // Build Atmos config structure from provisioned identities
    config := schema.AuthConfig{
        Identities: make(map[string]schema.Identity),
    }

    for name, provisioned := range result.Identities {
        config.Identities[name] = schema.Identity{
            Kind:      provisioned.Kind,
            Via:       provisioned.Via,
            Principal: provisioned.Principal,
            Tags:      provisioned.Tags,
            Labels:    provisioned.Labels,
            Metadata:  provisioned.Metadata,
        }
    }

    // Marshal to YAML
    data, err := yaml.Marshal(map[string]interface{}{"auth": config})
    if err != nil {
        return fmt.Errorf("failed to marshal config: %w", err)
    }

    // Add header comment with metadata
    header := fmt.Sprintf(`# Auto-generated by Atmos identity provisioning
# Provider: %s
# Provisioned: %s
# DO NOT EDIT - This file is regenerated on each login
# Manual overrides should be placed in atmos.yaml

`, result.Provider, result.ProvisionedAt.Format(time.RFC3339))

    content := header + string(data)

    // Ensure directory exists with secure permissions
    if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
        return fmt.Errorf("failed to create directory: %w", err)
    }

    // Write file with owner-only permissions (no group/world access)
    if err := os.WriteFile(outputPath, []byte(content), 0600); err != nil {
        return fmt.Errorf("failed to write file: %w", err)
    }

    return nil
}
```

### 5.4 Auth Manager Integration

```go
// pkg/auth/manager.go (additions)

func (m *manager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
    // ... existing authentication logic ...

    // After successful provider authentication, check if provisioning is enabled
    if provisioner, ok := providerInterface.(provisioning.Provisioner); ok {
        if m.shouldAutoProvision(providerName) {
            if err := m.provisionAndWriteConfig(ctx, providerName, provisioner, providerCreds); err != nil {
                // Non-fatal: warn but don't fail authentication
                // User can still authenticate with manually configured identities
                log.Warn("Failed to provision identities", "provider", providerName, "error", err)
            }
        }
    }

    // ... continue with identity authentication ...
}

func (m *manager) shouldAutoProvision(providerName string) bool {
    provider, exists := m.config.Providers[providerName]
    if !exists {
        return false
    }

    if spec, ok := provider.Spec["auto_provision_identities"].(bool); ok {
        return spec
    }

    return false
}

func (m *manager) provisionAndWriteConfig(ctx context.Context, providerName string, provisioner provisioning.Provisioner, creds types.ICredentials) error {
    // Provision identities from the provider
    result, err := provisioner.ProvisionIdentities(ctx, creds)
    if err != nil {
        return fmt.Errorf("failed to provision identities: %w", err)
    }

    // Get output path from provider config or use default
    outputPath := m.getProvisioningOutputPath(providerName)

    // Write provisioned identities to config file
    if err := provisioning.WriteConfig(outputPath, result); err != nil {
        return fmt.Errorf("failed to write config: %w", err)
    }

    log.Info("Identity provisioning complete",
        "provider", providerName,
        "identities", len(result.Identities),
        "accounts", result.Metadata.AccountCount,
        "duration", result.Metadata.Duration,
        "path", outputPath)
    return nil
}

func (m *manager) LogoutProvider(ctx context.Context, providerName string) error {
    // ... existing logout logic ...

    // Cleanup provisioning file if auto-provisioning enabled
    if m.shouldAutoProvision(providerName) {
        if err := m.cleanupProvisioningFile(providerName); err != nil {
            log.Warn("Failed to cleanup provisioning file", "provider", providerName, "error", err)
        }
    }

    return nil
}
```

### 5.5 Config Loading Integration

```go
// pkg/config/load.go (additions)

func LoadConfig(configAndStacksInfo *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
    // ... existing config loading ...

    // After loading base config and processing imports, inject provisioned identity configs
    if err := injectProvisionedIdentityImports(&atmosConfig); err != nil {
        log.Warn("Failed to inject provisioned identity imports", "error", err)
        // Non-fatal: continue without dynamic imports
    }

    // ... continue with rest of config loading ...
}

func injectProvisionedIdentityImports(atmosConfig *schema.AtmosConfiguration) error {
    for providerName, provider := range atmosConfig.Auth.Providers {
        if shouldAutoProvisionIdentities(&provider) {
            outputPath := getProvisioningOutputPath(providerName, &provider)

            // Check if provisioning file exists
            if _, err := os.Stat(outputPath); err == nil {
                // File exists, add to imports (prepend so manual config overrides)
                atmosConfig.Imports = append([]string{outputPath}, atmosConfig.Imports...)
                log.Debug("Injected provisioned identity import", "provider", providerName, "path", outputPath)
            }
        }
    }

    return nil
}

func shouldAutoProvisionIdentities(provider *schema.Provider) bool {
    if provider.Spec == nil {
        return false
    }

    if autoProvision, ok := provider.Spec["auto_provision_identities"].(bool); ok {
        return autoProvision
    }

    return false
}

func getProvisioningOutputPath(providerName string, provider *schema.Provider) string {
    // Check for custom path in provider.Spec["provisioning"]["output_path"]
    if provider.Spec != nil {
        if provisioning, ok := provider.Spec["provisioning"].(map[string]interface{}); ok {
            if outputPath, ok := provisioning["output_path"].(string); ok {
                return expandPath(outputPath)
            }
        }
    }

    // Default: XDG cache directory
    subpath := filepath.Join("aws", providerName)
    cacheDir, _ := xdg.GetXDGCacheDir(subpath, 0700)
    return filepath.Join(cacheDir, "provisioned-identities.yaml")
}
```

### 5.6 Data Flow

#### Login Flow (Provisioning)

```
1. User: atmos auth login --provider sso-prod

2. SSO Provider Authenticates
   └─> Device flow → access token
   └─> Cache token in keyring

3. Auth Manager: Check auto_provision_identities
   └─> provider.Spec["auto_provision_identities"] == true?

4. Auth Manager: Provision Identities
   └─> ssoProvider.ProvisionIdentities(ctx, accessToken)
       └─> AWS ListAccounts API (paginated)
       └─> AWS ListAccountRoles API (per account, paginated)
       └─> Apply filters (if configured)
       └─> Build provisioning.Result

5. Auth Manager: Write Config
   └─> provisioning.WriteConfig(outputPath, result)
       └─> Generate YAML with header
       └─> Write to ~/.cache/atmos/aws/sso-prod/provisioned-identities.yaml
       └─> Permissions: 0600

6. Complete: Display summary
   ✓ Provisioned 47 identities across 15 accounts (2.3s)
```

#### Config Loading Flow (Any Command)

```
1. User: atmos terraform plan --identity production/Admin

2. Load atmos.yaml
   └─> Parse base configuration

3. Process manual imports
   └─> imports: [path/to/catalogs/*.yaml]
   └─> Merge all manual imports

4. Inject provisioned identity imports
   └─> For each provider with auto_provision_identities:
       └─> Check if ~/.cache/atmos/aws/{provider}/provisioned-identities.yaml exists
       └─> If exists: Prepend to imports list (so manual config overrides)
       └─> If not: Skip (user not logged in)

5. Process dynamic imports
   └─> Load provisioned-identities.yaml
   └─> Merge identities (manual precedence over provisioned)

6. Identity available
   └─> "production/Admin" resolved from provisioned config
   └─> Use normally (no special handling)
```

#### Logout Flow (Cleanup)

```
1. User: atmos auth logout --provider sso-prod

2. Auth Manager: Call provider.Logout()
   └─> Clean AWS files (credentials, config)
   └─> Remove cached credentials from keyring

3. Auth Manager: Check auto_discover_identities
   └─> If true: Clean provisioning file

4. Auth Manager: cleanupDiscoveryFile()
   └─> Get path: ~/.cache/atmos/aws/sso-prod/discovered-identities.yaml
   └─> Delete file if exists
   └─> Log warning if fails (non-fatal)

5. Complete: Display summary
   ✓ Logged out from sso-prod
```

---

## 6. Configuration Reference

### Provider Configuration

```yaml
providers:
  sso-prod:
    kind: aws/iam-identity-center
    start_url: https://my-org.awsapps.com/start
    region: us-east-1

    spec:
      # Enable auto-provisiony (default: false)
      auto_discover_identities: true

      # Optional: Discovery configuration
      discovery:
        # Cache duration (default: 1h)
        cache_duration: 1h

        # Custom output path (default: XDG cache directory)
        output_path: ~/.cache/atmos/aws/sso-prod/discovered-identities.yaml

        # Filters: Limit which identities are discovered
        filters:
          # Account filters (OR logic)
          accounts:
            - production
            - staging
          # Or pattern-based (Phase 2)
          account_pattern: "prod-.*|staging-.*"

          # Role filters (OR logic)
          roles:
            - AdministratorAccess
            - PowerUserAccess
          # Or pattern-based (Phase 2)
          role_pattern: ".*Admin.*"

        # Identity naming template (default: "{account-name}/{PermissionSetName}")
        identity_name_template: "{{ .AccountName }}/{{ .RoleName }}"
        # Available variables:
        #   .AccountName  - AWS account name
        #   .AccountID    - AWS account ID
        #   .RoleName     - Permission set name

        # Include AWS PermissionSet tags as labels (default: false, Phase 3)
        include_tags: true
```

### Schema Changes

#### Provider Spec

```json
{
  "providers": {
    "properties": {
      "spec": {
        "type": "object",
        "properties": {
          "auto_discover_identities": {
            "type": "boolean",
            "description": "Automatically discover and populate identities from AWS SSO",
            "default": false
          },
          "discovery": {
            "type": "object",
            "properties": {
              "cache_duration": {
                "type": "string",
                "description": "How long to cache discovery results (e.g., '1h', '30m')",
                "default": "1h"
              },
              "output_path": {
                "type": "string",
                "description": "Custom path for provisioned identities file (XDG-compliant by default)"
              },
              "filters": {
                "type": "object",
                "properties": {
                  "accounts": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Only discover identities for these accounts"
                  },
                  "account_pattern": {
                    "type": "string",
                    "description": "Regex pattern for account filtering"
                  },
                  "roles": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Only discover these permission sets"
                  },
                  "role_pattern": {
                    "type": "string",
                    "description": "Regex pattern for role filtering"
                  }
                }
              },
              "identity_name_template": {
                "type": "string",
                "description": "Go template for identity naming",
                "default": "{{ .AccountName }}/{{ .RoleName }}"
              },
              "include_tags": {
                "type": "boolean",
                "description": "Include AWS PermissionSet tags as labels",
                "default": false
              }
            }
          }
        }
      }
    }
  }
}
```

#### Identity Extensions (Phase 3)

```json
{
  "identities": {
    "properties": {
      "tags": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Simple list of tags for filtering"
      },
      "labels": {
        "type": "object",
        "additionalProperties": {"type": "string"},
        "description": "Key-value labels from AWS PermissionSet tags"
      },
      "metadata": {
        "type": "object",
        "description": "Discovery metadata (discovered, provisioned_at, permission_set_arn, etc.)"
      }
    }
  }
}
```

---

## 7. Implementation Plan

### Phase 1: Core Discovery (MVP) - 5-7 days

**Goal**: Basic auto-provisiony working end-to-end

**Deliverables**:
1. `pkg/auth/types/discovery.go` - Interface definitions
2. `pkg/auth/providers/aws/sso_discovery.go` - AWS SSO implementation
3. `pkg/auth/config_writer.go` - Config file writer
4. `pkg/auth/manager.go` - Discovery orchestration
5. `pkg/config/load.go` - Dynamic import injection
6. Schema updates for `auto_discover_identities` flag
7. Unit tests for all components

**APIs Used**:
- `sso.ListAccounts()` - Enumerate accounts
- `sso.ListAccountRoles()` - Enumerate permission sets per account

**Success Criteria**:
- `atmos auth login` discovers and writes config file
- `atmos auth list` shows provisioned identities
- Any `--identity` flag works with provisioned identities
- `atmos auth logout` cleans up provisioning file

### Phase 2: Filtering & Customization - 2-3 days

**Goal**: Allow selective discovery and custom naming

**Deliverables**:
1. Account/role filter implementation
2. Identity name template support
3. Pattern-based filtering (regex)
4. Discovery configuration validation

**Success Criteria**:
- Filters reduce provisioned identities as expected
- Custom templates generate correct identity names
- Invalid configs produce clear error messages

### Phase 3: Tags & Labels (AWS PermissionSet Tags) - 3-4 days

**Goal**: Enrich provisioned identities with AWS tags

**Deliverables**:
1. `sso_discovery.go` - PermissionSet tag discovery
2. Tag → label mapping logic
3. Auto-generated tags from label values
4. Schema extensions for tags/labels

**APIs Used**:
- `ssoadmin.ListPermissionSets()` - Find permission set ARN
- `ssoadmin.ListTagsForResource()` - Get AWS tags from permission set

**Success Criteria**:
- Provisioned identities include `tags` and `labels` fields
- AWS PermissionSet tags correctly mapped to labels
- Tags auto-generated from label values

### Phase 4: Advanced Features (Future)

**Potential additions**:
- Discovery metadata tracking (first seen, last used)
- Discovery stats in `atmos auth list`
- Export provisioned identities to YAML for manual customization
- Multiple SSO provider support with conflict resolution

---

## 8. Key Design Decisions

### 1. Dynamic Config File vs Runtime Identities

**Decision**: Write provisioned identities to a dynamic YAML file, auto-import during config loading

**Rationale**:
- Treats provisioned identities like any other config (processed through normal Atmos import chain)
- Works across all Atmos commands without flag pollution
- Preserves `atmos auth list` working without login (shows manual identities)
- Leverages existing import/merge logic for override behavior
- No changes required to individual command implementations

**Trade-off**: Requires login to see auto-provisioned identities in `atmos auth list`

**Alternatives Considered**:
- **Runtime-only identities**: Would require every command to check for provisioned identities at runtime
- **New command structure**: Would require `atmos auth discover` separate from `atmos auth login`

### 2. XDG Cache Directory vs Project Directory

**Decision**: Default to `{XDG_CACHE_HOME}/atmos/aws/{provider}/discovered-identities.yaml`

**Rationale**:
- **XDG-compliant**: Follows XDG Base Directory Specification
- **Ephemeral cache**: `XDG_CACHE_HOME` indicates regeneratable data (vs `XDG_CONFIG_HOME` for persistent config)
- **Mirrors existing pattern**: AWS credentials already use XDG paths via AWS file manager
- **Per-provider isolation**: Allows multiple SSO providers without conflicts
- **Platform-appropriate**: Linux `~/.cache`, macOS `~/Library/Caches`, Windows `%LOCALAPPDATA%`
- **Not backed up**: Cache directories typically excluded from system backups
- **User-writable**: No permission issues

**Trade-off**: Provisioning file not in project directory (but accessible, configurable)

**Alternatives Considered**:
- **Project directory** (`.atmos/cache/`): Would be checked into git accidentally
- **Home config** (`~/.atmos/`): Wrong semantic (not persistent config)

### 3. Identity Naming Convention

**Decision**: Default to `{account-name}/{PermissionSetName}` (e.g., `production/AdministratorAccess`)

**Rationale**:
- Hierarchical structure reflects AWS organization (account → role)
- Preserves permission set name casing (as defined in AWS)
- Slash separator is clear and commonly used for hierarchical identifiers
- Easy to parse and filter (e.g., all production: `production/*`)
- Matches common convention in infrastructure-as-code tools

**Trade-off**: May not match organization's existing naming convention

**Alternatives Considered**:
- **Lowercase kebab-case**: `production-administratoraccess` - Less readable, loses hierarchy
- **Account ID based**: `123456789012-AdministratorAccess` - Not human-readable

**Configurable**: Via `identity_name_template` for organizations with specific conventions

### 4. Manual Identity Precedence

**Decision**: Manual identities override auto-provisioned (standard Atmos import merge)

**Rationale**:
- Allows customizing specific identities while benefiting from auto-provisiony
- Consistent with Atmos import precedence rules
- Clear semantic: "I explicitly configured this, use my config"
- Enables gradual migration from manual to auto-provisiony

**Trade-off**: Manual config can silently override discovered (but this is expected behavior)

### 5. Discovery Timing

**Decision**: Discover on `atmos auth login`, write file, file persists until next login

**Rationale**:
- File persists across commands (don't re-discover on every `atmos` invocation)
- Regenerate on login ensures reasonable freshness
- Avoids API throttling from frequent discovery
- Clear lifecycle: login creates, logout deletes

**Trade-off**: Discovery not real-time (but SSO permissions don't change frequently)

**Future Enhancement**: Add cache expiration check during config loading (Phase 3)

### 6. Feature Flag Approach

**Decision**: Opt-in via `auto_discover_identities: true` at provider level

**Rationale**:
- Backward compatible (no behavior change unless enabled)
- Allows gradual adoption across teams
- Clear intent ("I want auto-provisiony for this provider")
- Per-provider granularity (can enable for prod but not dev)

**Trade-off**: Not enabled by default (but new feature, conservative rollout)

### 7. Non-Fatal Discovery Failures

**Decision**: Provisioning failures warn but don't block authentication

**Rationale**:
- User can still authenticate and use manual identities
- Avoids blocking critical workflows due to transient API failures
- Clear warning message helps debugging
- Discovery is enhancement, not requirement

**Trade-off**: Silent failures if user doesn't read warnings (but logs clearly)

---

## 9. Benefits & Impact

### For Users

1. **Zero Configuration**: Just enable the flag - identities auto-populate on login
2. **Discovery of Available Access**: `atmos auth list` shows everything you can use
3. **Faster Onboarding**: New team members just login - all identities available immediately
4. **Always Up-to-Date**: Discovery runs on each login, reflecting current SSO permissions

### For Organizations

1. **Self-Service**: Users automatically get access to all their assigned roles
2. **Reduced Support**: No "how do I configure this role?" questions
3. **Centralized Control**: SSO admin controls access, Atmos reflects it automatically
4. **No Config Sprawl**: No need to maintain large identity lists in atmos.yaml

### Metrics & Success Criteria

**Time to First Auth**:
- Target: <2 minutes from clone to authenticated
- Baseline: 10+ minutes with manual configuration
- Measure: Time from `git clone` to successful `atmos terraform plan`

**User Satisfaction**:
- Target: "This just works" feedback > "I'm confused" feedback
- Measure: Beta user interviews, community feedback

---

## 10. Open Questions & Risks

### Open Questions

1. **Should discovery happen on every login or use cache with TTL?**
   - Phase 1: Every login (simple, always fresh)
   - Phase 3: Add cache_duration check (performance optimization)

2. **How to handle large organizations (1000+ permission sets)?**
   - Filters recommended in documentation
   - Consider async discovery with progress bar
   - May need to add discovery timeout config

3. **Should we support multi-provider discovery conflicts?**
   - Phase 1: Each provider writes separate file, no conflicts
   - Future: Add conflict detection/resolution if needed

### Risks & Mitigations

#### Risk 1: AWS API Rate Limits

**Impact**: Discovery may fail for users with many accounts

**Mitigation**:
- Implement exponential backoff
- Cache results aggressively (1h default)
- Allow filters to reduce API calls
- Document rate limit handling

#### Risk 2: Permission Set ARN Resolution (Phase 3)

**Impact**: Need SSO Admin API access to get tags, may not be granted

**Mitigation**:
- Make tag discovery optional (`include_tags: false` default)
- Graceful degradation if SSO Admin access unavailable
- Document IAM permissions required in docs

#### Risk 3: Config File Conflicts

**Impact**: User manually edits discovered-identities.yaml

**Mitigation**:
- Add warning comment in generated file: "DO NOT EDIT"
- Regenerate on every login (overwrites edits)
- Document that manual edits should go in atmos.yaml

#### Risk 4: Large Discovery Files

**Impact**: 1000+ identities = large YAML file, slow parsing

**Mitigation**:
- Recommend filters for large orgs
- Consider splitting into multiple files (Phase 4)
- Monitor file sizes in telemetry

---

## 11. Appendix: References

### A. Inspiration: aws-sso-cli

The [aws-sso-cli](https://github.com/synfinatic/aws-sso-cli) tool demonstrates a successful implementation of AWS SSO role auto-provisiony. We analyzed their approach and adapted key concepts for Atmos.

#### How aws-sso-cli Implements Discovery

**Discovery Mechanism**:
1. User authenticates via SSO device flow
2. Calls `ListAccounts` API to enumerate assigned accounts
3. For each account, calls `ListAccountRoles` API for permission sets
4. Caches results locally with metadata

**APIs Used**:
```go
// 1. List all accounts
accountsResp, err := ssoClient.ListAccounts(ctx, &sso.ListAccountsInput{
    AccessToken: aws.String(accessToken),
})

// 2. List roles per account
rolesResp, err := ssoClient.ListAccountRoles(ctx, &sso.ListAccountRolesInput{
    AccessToken: aws.String(accessToken),
    AccountId:   aws.String(accountID),
})
```

**Caching Strategy**:
- Stores discovered roles in local cache
- Tracks credential expiration timestamps
- Invalidates cache on config changes or version updates

#### What We Adapted

1. **Same AWS APIs**: `ListAccounts` + `ListAccountRoles`
2. **Pagination handling**: Both APIs support `NextToken` for large result sets
3. **Parallel enumeration**: First account serial, rest parallel for performance

#### What We Changed

1. **Configuration approach**: aws-sso-cli manages `~/.aws/config`, Atmos generates dynamic `atmos.yaml` imports
2. **Integration level**: aws-sso-cli is standalone tool, Atmos integrates into workflow/stack system
3. **Discovery timing**: aws-sso-cli has aggressive background caching, Atmos discovers on-demand at login
4. **Primary use case**: aws-sso-cli focuses on credential management, Atmos focuses on infrastructure orchestration

#### Key Takeaways

- **API reliability**: `ListAccounts` + `ListAccountRoles` are stable, well-documented AWS APIs
- **Performance**: Parallel enumeration important for organizations with many accounts
- **Error handling**: Non-fatal failures keep manual identities working
- **Caching**: Some form of caching essential to avoid API throttling

**Reference**: https://github.com/synfinatic/aws-sso-cli

### B. Related Atmos PRDs

- **[Tags and Labels Standard](./tags-and-labels-standard.md)**: Defines Atmos-wide convention for tags (lists) vs labels (maps), including AWS PermissionSet tag mapping used in Phase 3

### C. AWS API Documentation

- **AWS SSO Service**: https://docs.aws.amazon.com/singlesignon/latest/PortalAPIReference/Welcome.html
  - `ListAccounts`: https://docs.aws.amazon.com/singlesignon/latest/PortalAPIReference/API_ListAccounts.html
  - `ListAccountRoles`: https://docs.aws.amazon.com/singlesignon/latest/PortalAPIReference/API_ListAccountRoles.html

- **AWS SSO Admin Service** (Phase 3 - Tags):
  - `ListPermissionSets`: https://docs.aws.amazon.com/singlesignon/latest/APIReference/API_ListPermissionSets.html
  - `ListTagsForResource`: https://docs.aws.amazon.com/singlesignon/latest/APIReference/API_ListTagsForResource.html

### D. XDG Base Directory Specification

- **Specification**: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html
- **XDG_CACHE_HOME**: Directory for user-specific non-essential data (cache)
  - Linux: `~/.cache`
  - macOS: `~/Library/Caches`
  - Windows: `%LOCALAPPDATA%`

---

## 12. Conclusion

AWS SSO role auto-provisiony addresses a major pain point in Atmos onboarding and maintenance. By leveraging AWS Identity Center APIs and Atmos's existing config import system, we can provide zero-config identity population that "just works" while preserving backward compatibility and allowing manual customization.

The phased implementation approach delivers immediate value (Phase 1 MVP) while establishing a foundation for advanced features (filtering, tags/labels). The design decisions prioritize user experience, backward compatibility, and alignment with Atmos's existing patterns (XDG paths, import precedence, provider-level configuration).

**Next Steps**:
1. Review and approve PRD
2. Begin Phase 1 implementation (5-7 days)
3. Beta testing with 3-5 early adopters
4. Documentation and examples
5. Release with feature flag (opt-in)
6. Monitor adoption and feedback for Phase 2/3 prioritization
