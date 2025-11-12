# Azure Refresh Token Investigation

## Problem Statement

PR review comment (https://github.com/cloudposse/atmos/pull/1768#pullrequestreview-3437651727):

> **MSAL Cache Refresh Token Issue**
> Location: `pkg/auth/cloud/azure/setup.go`
> Critical Problem: Refresh tokens are missing from the MSAL cache
> Impact: Tokens will only be valid for ~1 hour, breaking the goal of being a drop-in replacement for `az login`
> Suggested Fix: Extend `updateMSALCache` to persist refresh token entries

## Investigation Results

### Current Implementation

The current Azure authentication implementation uses:
- **azidentity.DeviceCodeCredential** - Azure SDK for Go high-level wrapper
- **azidentity.GetToken()** - Returns `azcore.AccessToken` (access token + expiration only)
- **Manual MSAL cache updates** - `updateMSALCache()` manually writes access tokens to `~/.azure/msal_token_cache.json`

### Root Cause

**The Azure SDK for Go (`azidentity` package) does not expose refresh tokens through its public API.**

Evidence:
```go
// azcore.AccessToken struct
type AccessToken struct {
    Token     string
    ExpiresOn time.Time
    // No RefreshToken field
}
```

The `GetToken()` method only returns access tokens. Refresh tokens are managed internally by the SDK but not exposed.

### MSAL Cache Structure

The Azure CLI (`az login`) writes 5 sections to the MSAL cache:
```json
{
  "AccessToken": { ... },
  "RefreshToken": { ... },  // ← Missing in Atmos
  "IdToken": { ... },       // ← Missing in Atmos
  "Account": { ... },
  "AppMetadata": { ... }    // ← Missing in Atmos
}
```

**RefreshToken entry format:**
```json
{
  "credential_type": "RefreshToken",
  "secret": "1.ARgA...",  // Long base64-encoded refresh token
  "home_account_id": "objectId.tenantId",
  "environment": "login.microsoftonline.com",
  "client_id": "04b07795-8ddb-461a-bbee-02f9e1bf7b46",
  "target": "https://management.core.windows.net//.default ...",
  "last_modification_time": "1762825938",
  "family_id": "1"
}
```

## Proposed Solutions

### Option 1: Migrate to MSAL Library (Recommended)

Replace `azidentity` with Microsoft Authentication Library (MSAL) for Go.

**Advantages:**
- MSAL automatically persists refresh tokens to cache
- Full control over cache location (`~/.azure/msal_token_cache.json`)
- True drop-in replacement for `az login`
- Supports silent token renewal

**Implementation:**
```go
import (
    "github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
    "github.com/AzureAD/microsoft-authentication-extensions-for-go/cache"
    "github.com/AzureAD/microsoft-authentication-extensions-for-go/cache/accessor"
)

// Create cache accessor
accessor, err := accessor.New("atmos-azure-auth")

// Create cache pointing to Azure CLI location
msalCache, err := cache.New(accessor, "~/.azure/msal_token_cache.json")

// Create MSAL public client
client, err := public.New(
    "04b07795-8ddb-461a-bbee-02f9e1bf7b46",  // Azure CLI client ID
    public.WithAuthority(fmt.Sprintf("https://login.microsoftonline.com/%s", tenantID)),
    public.WithCache(msalCache),
)

// Device code flow
deviceCode, err := client.AcquireTokenByDeviceCode(ctx, scopes)
// User authenticates...
result, err := deviceCode.AuthenticationResult(ctx)
// MSAL automatically writes refresh tokens to cache!
```

**Scope of Change:**
- Modify `pkg/auth/providers/azure/device_code.go` (~200 lines)
- Potentially modify OIDC and service principal providers
- Add MSAL cache helper in `pkg/auth/cloud/azure/`
- Update tests
- Estimated effort: 2-4 hours

**Risks:**
- Breaking change to authentication flow
- Need extensive testing across all Azure providers
- Different error handling patterns between azidentity and MSAL

### Option 2: Use Azure SDK Persistent Cache

Use `azidentity/cache` package for automatic token management.

**Advantages:**
- Smaller code change
- Stays within Azure SDK ecosystem

**Disadvantages:**
- Writes to different location than Azure CLI (`~/.IdentityService/msal.cache` not `~/.azure/msal_token_cache.json`)
- Not a true drop-in replacement for `az login`
- Still doesn't solve the review comment's requirement

**Implementation:**
```go
import "github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"

azCache, err := cache.New(&cache.Options{Name: "atmos"})

cred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
    TenantID: tenantID,
    ClientID: clientID,
    Cache:    azCache,  // SDK manages refresh tokens internally
})
```

### Option 3: Document as Known Limitation

Accept that Atmos tokens expire after 1 hour and document this behavior.

**Advantages:**
- No code changes
- Honest about current capabilities

**Disadvantages:**
- Doesn't solve the problem
- Not a drop-in replacement for `az login`
- Poor user experience for long-running Terraform operations

## Recommendation

**Proceed with Option 1 (MSAL Migration)** because:

1. **Solves the root problem** - Refresh tokens will be persisted
2. **True `az login` compatibility** - Uses same cache file and structure
3. **Better long-term architecture** - Direct access to MSAL features
4. **Enables future enhancements** - Silent token renewal, token caching across processes

The migration is a significant change but necessary to achieve the goal of being a drop-in replacement for Azure CLI.

## Alternative: Hybrid Approach

For minimal risk, we could:
1. Keep current `azidentity` implementation for device code, OIDC, service principal
2. Add MSAL cache population AFTER getting tokens from `azidentity`
3. Manually construct and persist RefreshToken entries (even if we don't have real refresh tokens)

But this is hacky and doesn't actually enable token renewal - it just makes the cache look correct without providing the functionality.

## Next Steps

1. Get approval for MSAL migration approach
2. Create feature branch for MSAL integration
3. Implement MSAL-based device code provider
4. Add comprehensive tests
5. Update documentation
6. Test with real Azure subscriptions

## References

- Azure SDK for Go: https://github.com/Azure/azure-sdk-for-go
- MSAL for Go: https://github.com/AzureAD/microsoft-authentication-library-for-go
- MSAL Extensions for Go: https://github.com/AzureAD/microsoft-authentication-extensions-for-go
- MSAL Token Cache Specification: https://aka.ms/msal-net-token-cache-serialization
