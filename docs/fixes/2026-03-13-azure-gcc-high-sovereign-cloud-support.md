# Azure GCC High / Sovereign Cloud Support

**Related Issue:** [#2006](https://github.com/cloudposse/atmos/issues/2006) — `!terraform.state` fails in
Azure Government (GCC High) because blob storage endpoint is hardcoded to commercial Azure

**Affected Atmos Version:** v1.200.0+

**Severity:** Critical for Azure Government users — `!terraform.state` YAML function cannot resolve
Terraform state from Azure Blob Storage in sovereign cloud environments

## Background

Azure operates multiple sovereign cloud environments with distinct endpoints:

| Cloud Environment    | Login Endpoint              | Blob Storage Suffix           | Portal URL         | Management API                 |
|----------------------|-----------------------------|-------------------------------|--------------------|--------------------------------|
| **Commercial**       | `login.microsoftonline.com` | `blob.core.windows.net`       | `portal.azure.com` | `management.azure.com`         |
| **US Government**    | `login.microsoftonline.us`  | `blob.core.usgovcloudapi.net` | `portal.azure.us`  | `management.usgovcloudapi.net` |
| **China (Mooncake)** | `login.chinacloudapi.cn`    | `blob.core.chinacloudapi.cn`  | `portal.azure.cn`  | `management.chinacloudapi.cn`  |

The Atmos codebase currently hardcodes **Azure Commercial** endpoints in multiple locations, making it
impossible to use Atmos auth, `!terraform.state`, or Azure console features with sovereign clouds.

## Issue Description

A user reports that `!terraform.state` returns:

```text
failed to get blob from Azure Blob Storage: Get "https://redacted.blob.core.windows.net/redacted/tfstate"
dial tcp: lookup redacted.blob.core.windows.net: no such host.
```

The storage account is in Azure Government, where the correct endpoint suffix is
`blob.core.usgovcloudapi.net`, not `blob.core.windows.net`.

## Root Cause Analysis

### 1. `!terraform.state` — Hardcoded blob storage endpoint

**File:** `internal/terraform_backend/terraform_backend_azurerm.go` (line 103)

```go
serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", storageAccountName)
```

This hardcodes the Azure Commercial blob storage suffix. Azure Government uses
`blob.core.usgovcloudapi.net` and Azure China uses `blob.core.chinacloudapi.cn`.

### 2. Azure OIDC Provider — Hardcoded token endpoint

**File:** `pkg/auth/providers/azure/oidc.go` (lines 29-38)

```go
azureADTokenEndpoint = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
azureManagementScope = "https://management.azure.com/.default"
azureGraphAPIScope   = "https://graph.microsoft.com/.default"
azureKeyVaultScope   = "https://vault.azure.net/.default"
```

All four constants are hardcoded to Azure Commercial endpoints.

### 3. Azure Device Code Provider — Hardcoded authority

**File:** `pkg/auth/providers/azure/device_code.go` (line 140)

```go
public.WithAuthority(fmt.Sprintf("https://login.microsoftonline.com/%s", p.tenantID)),
```

### 4. Azure Device Code Cache — Hardcoded scopes and environment

**File:** `pkg/auth/providers/azure/device_code_cache.go` (lines 291, 314, 342, 350)

```go
environment:   "login.microsoftonline.com",
scope := "https://management.azure.com/.default"
// ... graph and keyvault scopes also hardcoded
```

### 5. Azure MSAL Cache Setup — Hardcoded environment and scopes

**File:** `pkg/auth/cloud/azure/setup.go` (lines 377, 399, 418, 445, 464, 480)

```go
environment:   "login.microsoftonline.com",
Scope:         "https://management.azure.com/.default",
Scope:         "https://graph.microsoft.com/.default",
Scope:         "https://vault.azure.net/.default",
```

### 6. Azure Console — Hardcoded portal URL

**File:** `pkg/auth/cloud/azure/console.go` (line 17)

```go
AzurePortalURL = "https://portal.azure.com/"
```

Azure Government uses `portal.azure.us`, Azure China uses `portal.azure.cn`.

## Fix

### Approach: Cloud environment configuration in provider spec

Add a `cloud_environment` field to Azure provider configuration that selects the appropriate
endpoint set. Default to `"public"` (Azure Commercial) for backward compatibility.

### 1. Define cloud environment endpoint sets

Create a new file `pkg/auth/cloud/azure/cloud_environments.go`:

```go
// CloudEnvironment defines the endpoints for a specific Azure cloud.
type CloudEnvironment struct {
    Name              string // "public", "usgovernment", "china"
    LoginEndpoint     string // Azure AD / Entra ID authority host
    ManagementScope   string // ARM management API scope
    GraphAPIScope     string // Microsoft Graph scope
    KeyVaultScope     string // KeyVault scope
    BlobStorageSuffix string // Blob storage URL suffix (e.g., "blob.core.windows.net")
    PortalURL         string // Azure Portal base URL
}

var cloudEnvironments = map[string]*CloudEnvironment{
    "public": {
        Name:              "public",
        LoginEndpoint:     "login.microsoftonline.com",
        ManagementScope:   "https://management.azure.com/.default",
        GraphAPIScope:     "https://graph.microsoft.com/.default",
        KeyVaultScope:     "https://vault.azure.net/.default",
        BlobStorageSuffix: "blob.core.windows.net",
        PortalURL:         "https://portal.azure.com/",
    },
    "usgovernment": {
        Name:              "usgovernment",
        LoginEndpoint:     "login.microsoftonline.us",
        ManagementScope:   "https://management.usgovcloudapi.net/.default",
        GraphAPIScope:     "https://graph.microsoft.us/.default",
        KeyVaultScope:     "https://vault.usgovcloudapi.net/.default",
        BlobStorageSuffix: "blob.core.usgovcloudapi.net",
        PortalURL:         "https://portal.azure.us/",
    },
    "china": {
        Name:              "china",
        LoginEndpoint:     "login.chinacloudapi.cn",
        ManagementScope:   "https://management.chinacloudapi.cn/.default",
        GraphAPIScope:     "https://microsoftgraph.chinacloudapi.cn/.default",
        KeyVaultScope:     "https://vault.azure.cn/.default",
        BlobStorageSuffix: "blob.core.chinacloudapi.cn",
        PortalURL:         "https://portal.azure.cn/",
    },
}

// GetCloudEnvironment returns the endpoint set for the given cloud name.
// Returns the "public" environment if name is empty or unknown.
func GetCloudEnvironment(name string) *CloudEnvironment {
    if env, ok := cloudEnvironments[name]; ok {
        return env
    }
    return cloudEnvironments["public"]
}
```

### 2. Add `cloud_environment` to provider spec schema

**File:** `pkg/schema/schema.go`

Add `CloudEnvironment` to the Azure provider spec fields so it can be configured in `atmos.yaml`:

```yaml
auth:
  providers:
    azure-gov:
      kind: azure/oidc
      spec:
        tenant_id: "..."
        client_id: "..."
        subscription_id: "..."
        cloud_environment: usgovernment  # <-- NEW
```

### 3. Thread cloud environment through Azure providers

Each Azure provider (`oidc`, `device_code`, `cli`) needs to:
1. Read `cloud_environment` from its spec config
2. Look up the `CloudEnvironment` endpoint set
3. Use endpoints from the set instead of hardcoded constants

**OIDC provider** (`pkg/auth/providers/azure/oidc.go`):
- Replace `azureADTokenEndpoint` constant with `env.LoginEndpoint`-based URL
- Replace `azureManagementScope`, `azureGraphAPIScope`, `azureKeyVaultScope` with `env.*` fields

**Device code provider** (`pkg/auth/providers/azure/device_code.go`):
- Replace `login.microsoftonline.com` authority with `env.LoginEndpoint`
- Replace hardcoded scopes in `acquireManagementToken`, `acquireGraphToken`, `acquireKeyVaultToken`

### 4. Thread cloud environment to `!terraform.state` backend

**File:** `internal/terraform_backend/terraform_backend_azurerm.go`

The `getCachedAzureBlobClient` function constructs the blob service URL. It needs the cloud
environment's `BlobStorageSuffix`.

Two approaches:

**Option A: Read from backend config** — The Terraform `azurerm` backend already supports an
`environment` field. If set, use it to determine the blob suffix:

```go
// Read cloud environment from backend config (matches Terraform's azurerm backend "environment" field).
cloudEnv := GetBackendAttribute(backend, "environment")
env := GetCloudEnvironment(cloudEnv)
serviceURL := fmt.Sprintf("https://%s.%s/", storageAccountName, env.BlobStorageSuffix)
```

This is the preferred approach because:
- Terraform's `azurerm` backend already uses `environment = "usgovernment"` for sovereign clouds
- Users already have this configured in their backend config
- No new Atmos-specific configuration needed for `!terraform.state`

**Option B: Pass via auth context** — Thread the cloud environment from the auth provider's
config down through `AuthContext` to the state reader.

### 5. Thread cloud environment to console URL generation

**File:** `pkg/auth/cloud/azure/console.go`

Replace `AzurePortalURL` constant with a cloud-environment-aware lookup.

### 6. Thread cloud environment to MSAL cache setup

**File:** `pkg/auth/cloud/azure/setup.go`

Replace hardcoded `login.microsoftonline.com` and scope strings with cloud-environment-aware values.

## Hardcoded Endpoints Inventory

| File                                                      | Line(s)                      | Hardcoded Value             | Fix                                    |
|-----------------------------------------------------------|------------------------------|-----------------------------|----------------------------------------|
| `internal/terraform_backend/terraform_backend_azurerm.go` | 103                          | `blob.core.windows.net`     | Read `environment` from backend config |
| `pkg/auth/providers/azure/oidc.go`                        | 29                           | `login.microsoftonline.com` | Use `CloudEnvironment.LoginEndpoint`   |
| `pkg/auth/providers/azure/oidc.go`                        | 32                           | `management.azure.com`      | Use `CloudEnvironment.ManagementScope` |
| `pkg/auth/providers/azure/oidc.go`                        | 35                           | `graph.microsoft.com`       | Use `CloudEnvironment.GraphAPIScope`   |
| `pkg/auth/providers/azure/oidc.go`                        | 38                           | `vault.azure.net`           | Use `CloudEnvironment.KeyVaultScope`   |
| `pkg/auth/providers/azure/device_code.go`                 | 140                          | `login.microsoftonline.com` | Use `CloudEnvironment.LoginEndpoint`   |
| `pkg/auth/providers/azure/device_code.go`                 | 284, 298, 311, 342, 378, 394 | Scopes                      | Use `CloudEnvironment.*Scope`          |
| `pkg/auth/providers/azure/device_code_cache.go`           | 291                          | `login.microsoftonline.com` | Use `CloudEnvironment.LoginEndpoint`   |
| `pkg/auth/providers/azure/device_code_cache.go`           | 314, 342, 350                | Scopes                      | Use `CloudEnvironment.*Scope`          |
| `pkg/auth/cloud/azure/setup.go`                           | 377, 418                     | `login.microsoftonline.com` | Use `CloudEnvironment.LoginEndpoint`   |
| `pkg/auth/cloud/azure/setup.go`                           | 399, 445, 464, 480           | Scopes                      | Use `CloudEnvironment.*Scope`          |
| `pkg/auth/cloud/azure/console.go`                         | 17                           | `portal.azure.com`          | Use `CloudEnvironment.PortalURL`       |

## Configuration Example

```yaml
# atmos.yaml
auth:
  providers:
    azure-gov:
      kind: azure/oidc
      spec:
        tenant_id: !env AZURE_TENANT_ID
        client_id: !env AZURE_CLIENT_ID
        subscription_id: !env AZURE_SUBSCRIPTION_ID
        cloud_environment: usgovernment  # "public" (default), "usgovernment", or "china"
  identities:
    gov-sub:
      kind: azure/subscription
      via:
        provider: azure-gov
      principal:
        subscription_id: !env AZURE_SUBSCRIPTION_ID

# Terraform backend config (already supported by Terraform's azurerm backend)
terraform:
  backend_type: azurerm
  backend:
    azurerm:
      storage_account_name: "mystorageaccount"
      container_name: "tfstate"
      environment: "usgovernment"  # Terraform's own field — Atmos should read this
```

## Backward Compatibility

- `cloud_environment` defaults to `"public"` — all existing Azure Commercial users are unaffected
- The Terraform `azurerm` backend `environment` field already exists in user configs for sovereign
  cloud deployments — no new backend configuration needed
- Unknown `cloud_environment` values fall back to `"public"` with a warning

## Files Changed

| File                                                           | Change                                                 |
|----------------------------------------------------------------|--------------------------------------------------------|
| `pkg/auth/cloud/azure/cloud_environments.go`                   | **NEW** — Cloud environment endpoint registry          |
| `pkg/auth/cloud/azure/cloud_environments_test.go`              | **NEW** — Tests for endpoint lookup                    |
| `internal/terraform_backend/terraform_backend_azurerm.go`      | Read `environment` from backend config for blob suffix |
| `internal/terraform_backend/terraform_backend_azurerm_test.go` | Add sovereign cloud test cases                         |
| `pkg/auth/providers/azure/oidc.go`                             | Replace hardcoded constants with cloud environment     |
| `pkg/auth/providers/azure/device_code.go`                      | Replace hardcoded authority and scopes                 |
| `pkg/auth/providers/azure/device_code_cache.go`                | Replace hardcoded environment and scopes               |
| `pkg/auth/cloud/azure/setup.go`                                | Replace hardcoded environment and scopes               |
| `pkg/auth/cloud/azure/console.go`                              | Replace hardcoded portal URL                           |
| `pkg/schema/schema.go`                                         | Add `CloudEnvironment` to Azure provider spec          |
