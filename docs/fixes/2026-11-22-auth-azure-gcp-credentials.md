# Fix: Azure and GCP auth credential, subscription and cloud handling (#3720)

**Date:** 2026-11-22

## Summary

This change fixes the Azure and GCP items from the Atmos Auth pre-release review (#3720):

- The `azure/cli` provider failed whenever `subscription_id` was set.
- Atmos could reset the user's own `az` MSAL token cache and the per-realm Atmos MSAL cache, and its
  writes could race with `az`.
- `azure/pim-role` exported the parent's subscription instead of the `principal.scope` subscription.
- Logging in without a subscription cleared the user's `az account set` default.
- ACR rejected sovereign-cloud and mixed-case login servers, and logged in to them with a public-cloud
  token.
- `spec.registry.tenant_id` on `azure/acr` integrations was documented but never read.
- `gcp/service-account` passed the caller's own GCP credentials through to the subprocess.
- gcloud never read the project and region properties Atmos wrote.
- A Google STS response without an access token became an empty "authenticated" credential.

Every fix has a test that fails without it. Other #3720 and regression-test fixes land in separate PRs.

## Context

The review findings were re-checked against `main` at `2d7822f214`, and each item below still
reproduced there. Review item numbers are given in brackets, for example (#3720 2.3).

None of these items is a regression found by comparing with v1.234.1. The v1.234.1 regression test
also did not exercise live ACR exchanges or Azure MSAL caches.

## Changes

### `az` CLI provider and profile

- **`azure/cli` passes only one of `--subscription` or `--tenant`** (#3720 2.3,
  `pkg/auth/providers/azure/cli.go`).
  - Problem: `az account get-access-token` rejects both flags together ("Please specify only one of
    subscription and tenant, not both"). Atmos always passed `--tenant` and added `--subscription`
    when set, so the documented example with `subscription_id` always failed.
  - Fix: pass `--subscription` when one is configured (the token is issued in that subscription's
    tenant), otherwise `--tenant`.
  - Test: `TestCLIProvider_ExecuteAzCommand_TenantSubscriptionArgs`.
- **An empty subscription ID leaves the `az` default alone** (#3720 4.7,
  `pkg/auth/cloud/azure/setup.go`).
  - Problem: `UpdateSubscriptionsInProfile` called without a subscription (for example a device-code
    or OIDC provider with only a tenant) set `isDefault: false` on every `azureProfile.json` entry,
    clearing the user's `az account set` default. It also treated an entry with a blank `id` as the
    Atmos subscription.
  - Fix: with no subscription ID there is nothing to make default, so the entries are returned
    unchanged.
  - Tests: `TestUpdateSubscriptionsInProfile_EmptySubscriptionKeepsDefault`,
    `TestUpdateSubscriptionsInProfile_EmptySubscriptionDoesNotClaimBlankEntry`,
    `TestUpdateSubscriptionsInProfile_SetSubscriptionBecomesDefault`.

### MSAL token caches

- **The shared `az` MSAL cache is never reset** (#3720 3.1, `pkg/auth/cloud/azure/cli_msal_cache.go`).
  - Problem: the device-code and interactive providers reset an unparseable
    `~/.azure/msal_token_cache.json` (a UTF-8 BOM or a partial write) to `{}` and wrote it back,
    wiping the user's own `az login`. The read ran outside the lock, and the lock file
    (`msal_token_cache.json.lock`) was not the one msal-extensions and `az` use
    (`msal_token_cache.json.lockfile`), so a concurrent `az` write could be lost.
  - Fix: the new `UpdateAzureCLIMSALCache` does the read, update and atomic write under one lock. It
    strips a BOM. Only a missing, empty or whitespace-only file (or a JSON `null`) starts an empty
    cache. Anything else returns `ErrAzureCLIMSALCacheUnparseable` and leaves the file untouched.
    Authentication still succeeds.
  - Both writers use it: the device-code and interactive providers
    (`pkg/auth/providers/azure/device_code_cache.go`, which logs a damaged cache as a warning) and the
    shared `UpdateAzureCLIFiles` path (`setup.go`, which logs the skipped update at debug level).
  - Tests: `TestUpdateMSALCache_PreservesBOMPrefixedCache`,
    `TestUpdateMSALCache_ReadsUnderMSALExtensionsLock`, `TestUpdateMSALCache_LeavesUnparseableCacheUntouched`,
    `TestUpdateAzureCLIMSALCache`, `TestAzureCLIMSALCacheLockPath`,
    `TestUpdateAzureCLICache_PreservesBOMPrefixedCache`,
    `TestUpdateAzureCLICache_LeavesUnparseableCacheUntouched`,
    `TestUpdateAzureCLICache_WriteFailureIsNonFatal`, `TestUpdateAzureCLICache_ReadsUnderMSALExtensionsLock`.
- **Atmos's per-realm MSAL cache is never reset and merges under one lock** (#3720 follow-up,
  `pkg/auth/cloud/azure/msal_cache.go`, `msal_cache_merge.go`).
  - Problem: the cache Atmos hands to MSAL (per realm under `~/.azure/atmos/<realm>`, and the shared
    `az` cache for the empty realm) locked only `<cache>.lock`, passed a BOM-prefixed file to MSAL,
    and on unparseable data started fresh and then overwrote the file. Export rewrote the whole file
    from MSAL's memory, dropping entries other processes wrote after the load.
  - Fix: `Replace` strips a BOM. A file that is not a JSON object, or that MSAL cannot load, is not
    loaded; authentication continues without cached tokens, with a warning.
  - Fix: `Export` is a read-modify-write under the lock. It re-reads the file, applies only the entries
    MSAL changed since `Replace`, and writes atomically, so entries that `az` or another Atmos process
    added are kept. Entries are never removed. Numbers round-trip verbatim. A file on disk that is not
    valid JSON is never overwritten (`ErrAzureMSALCacheUnparseable`, logged as a warning).
  - Fix: this cache and `UpdateAzureCLIMSALCache` both take the msal-extensions `<cache>.lockfile` and
    then the legacy `<cache>.lock`, so Atmos excludes `az` and older Atmos releases. The fixed order
    rules out a deadlock.
  - Tests: `TestMSALCache_UnparseableCacheIsNeverOverwritten`, `TestMSALCache_ReplaceStripsBOM`,
    `TestMSALCache_ExportKeepsEntriesWrittenSinceReplace`,
    `TestMSALCache_ExportAfterFailedMSALLoadKeepsDiskEntries`,
    `TestMSALCache_LocksMSALExtensionsAndLegacyLockFiles`, `TestMSALCache_ConcurrentExportsKeepEveryEntry`,
    `TestMSALCache_ExportPreservesNumbersAndTopLevelFields`, `TestDecodeMSALCacheState`,
    `TestReadMSALCacheState`, `TestMSALCache_EmptyFileStartsEmptyCache`, `TestMSALCache_ExportOursUnparseable`,
    `TestMSALCache_ReplaceUnmarshalErrorKeepsAuthWorking`, `TestMSALCache_ExportReadErrorIsADirectory`,
    `TestMSALCache_ExportWriteError`.

### `azure/pim-role` subscription

- **`azure/pim-role` exports the scope's subscription** (#3720 3.3,
  `pkg/auth/identities/azure/pim_role.go`).
  - Problem: `PostAuthenticate` passed the parent credentials, with the parent's `SubscriptionID`, to
    `SetupFiles`, `SetAuthContext` and `SetEnvironmentVariables`, while `Environment()` and the `az`
    files used the `principal.scope` subscription. Terraform could run against a different subscription
    than `az`, or get no `ARM_SUBSCRIPTION_ID` when the parent had none.
  - Fix: `scopedCredentials` makes a struct copy carrying the scope's subscription (the shared parent
    credentials are not mutated and no field is dropped), and every post-authenticate step uses it.
    Non-Azure credentials and non-subscription scopes pass through unchanged.
  - Tests: `TestPIMRole_PostAuthenticate_UsesScopeSubscription`, `TestPIMRole_ScopedCredentials`.

### Azure Container Registry

- **ACR accepts `*.azurecr.us`, `*.azurecr.cn` and mixed-case names** (#3720 4.8,
  `pkg/auth/cloud/azure/acr.go`).
  - Problem: the login-server pattern matched only lowercase `*.azurecr.io`, so US Government and China
    registries and names like `MyRegistry.azurecr.io` were rejected. `--registry` also rebuilt the login
    server from the name, always as `*.azurecr.io`.
  - Fix: the input is lowercased and trimmed of whitespace, an `https://` scheme and a trailing slash,
    then matched against all three suffixes. The new `NormalizeRegistryURL` keeps the login server's own
    suffix, and `atmos azure acr login --registry` uses it.
  - Tests: `TestNormalizeRegistryURL`, `TestNormalizeRegistryURL_Invalid`,
    `TestExecuteExplicitRegistries_SovereignAndUppercase`.
- **Sovereign-cloud ACR logs in with its own cloud** (#3720 follow-up, `pkg/auth/cloud/azure/acr.go`,
  `cloud_environments.go`, `cmd/azure/acr/login.go`, `pkg/auth/integrations/azure/acr.go`,
  `pkg/auth/registryauth`).
  - Problem: `--registry x.azurecr.us` requested its ambient token from the public-cloud authority for
    the public ARM scope, which a US Government or China registry refuses. Integrations configured by
    `spec.registry.name` always built `<name>.azurecr.io`.
  - Fix: `CloudEnvironment.ACRLoginServerSuffix` maps each suffix to its cloud (`azurecr.io` public,
    `azurecr.us` US Government, `azurecr.cn` China), and `CloudEnvironmentForRegistry` reads it.
  - Fix: `LoadDefaultAzureCredentials` takes the cloud. A sovereign cloud passes its azcore cloud
    configuration (AAD authority) and requests that cloud's management scope. The public cloud keeps
    nil options, so `AZURE_AUTHORITY_HOST` still works.
  - Fix: `--registry` validates every registry before any login, so a typo never leaves a partial
    login. It then loads ambient credentials once per cloud and handles registries in mixed clouds.
  - Fix: integrations build the login server from the provider's `cloud_environment` (carried on the
    credentials), defaulting to public. Logout removes all three suffixes, because it runs without
    credentials and cannot tell the cloud. Plugin registry auth matches an `azure/acr` integration on
    its login server in any cloud, and the minted host is still checked against the requested host.
  - Tests: `TestExecuteExplicitRegistries_AmbientTokenPerRegistryCloud`,
    `TestExecuteExplicitRegistries_InvalidRegistryFailsBeforeAnyLogin`,
    `TestLoadDefaultAzureCredentials_SDKUsesSovereignAuthority`, `TestLoadDefaultAzureCredentials_Failures`,
    `TestLoadDefaultAzureCredentials_UsesRegistryCloud`, `TestLoadDefaultAzureCredentials_NilCloudIsPublic`,
    `TestGetAuthorizationToken_SovereignExchangeHost`, `TestCloudEnvironmentForRegistry`,
    `TestBuildRegistryURLForCloud`, `TestRegistryURLsForAllClouds`, `TestACRLoginServerSuffixes_RoundTrip`,
    `TestACRIntegration_LoginServerFollowsProviderCloud`,
    `TestACRIntegration_CleanupRemovesSovereignLoginServer`,
    `TestMatchingIntegrationACRSovereignHosts`.
- **`spec.registry.tenant_id` on `azure/acr` integrations is honored** (#3720 remaining item,
  `pkg/auth/cloud/azure/acr_tenant.go`, `pkg/auth/integrations/azure/acr.go`).
  - Problem: the field was documented as overriding the tenant of the ACR token exchange, but no code
    read it. The exchange always sent the identity's tenant and token.
  - Cause: ACR's `/oauth2/exchange` takes the AAD token together with the tenant that issued it (as
    `az acr login` does), so changing only the tenant field cannot reach a registry in another tenant.
  - Fix: `tenant_id` is validated when the integration loads. It must be a GUID or a domain name of two
    or more labels (`ErrACRInvalidTenant`).
  - Fix: when `tenant_id` is the identity's tenant (the credentials' tenant or the token's `tid`), the
    credentials are used unchanged. Otherwise Atmos mints an ARM token in that tenant silently from the
    realm's MSAL cache (same client and cached account, at the provider cloud's login endpoint), so
    sovereign clouds keep working. A domain `tenant_id` is sent to the exchange as the issuing tenant's
    ID.
  - Fix: this works for `azure/device-code` and `azure/interactive` identities. `azure/cli` and
    `azure/oidc` fail with `ErrACRTenantOverrideUnsupported` and a hint instead of silently using the
    wrong tenant. Explicit `--registry` login is unchanged.
  - Tests: `TestValidateACRTenantID`, `TestCredentialsForACRTenant_MintsTokenInRegistryTenant`,
    `TestCredentialsForACRTenant_SovereignCloud`, `TestCredentialsForACRTenant_DomainTenantUsesIssuedTenantID`,
    `TestCredentialsForACRTenant_IdentityTenantIsUnchanged`,
    `TestCredentialsForACRTenant_UnsupportedCredentials`,
    `TestCredentialsForACRTenant_InvalidInput`, `TestCredentialsForACRTenant_Failures`,
    `TestCredentialsForACRTenant_AccountByTenantWithoutHomeAccountID`, `TestTokenClientID`,
    `TestNewACRTenantTokenClient_RealmCache`, `TestNewACRTenantTokenClient_CacheError`,
    `TestACRTenantExchange_SendsRegistryTenant`, `TestACRIntegration_TenantOverrideReachesExchange`,
    `TestACRIntegration_TenantOverrideKeepsProviderCloud`,
    `TestACRIntegration_NoTenantOverrideUsesIdentityTenant`,
    `TestACRIntegration_TenantOverrideFailureStopsExchange`,
    `TestACRIntegration_TenantOverrideUnsupportedCredentials`, `TestNewACRIntegration_TenantIDValidation`.

### GCP

- **`gcp/service-account` scrubs the caller's GCP credential variables** (#3720 4.6,
  `pkg/auth/cloud/gcp/env.go`, `pkg/auth/identities/gcp_service_account/identity.go`).
  - Problem: `PrepareEnvironment` copied the caller's environment and only added the Atmos variables,
    so a personal `GOOGLE_APPLICATION_CREDENTIALS`, `CLOUDSDK_COMPUTE_REGION`/`ZONE`, or an override
    such as `GOOGLE_CREDENTIALS` or `GOOGLE_IMPERSONATE_SERVICE_ACCOUNT` reached the subprocess next to
    the Atmos token.
  - Fix: the new `ScrubAmbientEnvironment` removes the Atmos-managed GCP variables and the credential
    overrides (`GOOGLE_CREDENTIALS`, `GOOGLE_CLOUD_KEYFILE_JSON`, `GCLOUD_KEYFILE_JSON`,
    `GOOGLE_IMPERSONATE_SERVICE_ACCOUNT`, `CLOUDSDK_CORE_ACCOUNT`, `CLOUDSDK_AUTH_ACCESS_TOKEN_FILE`,
    `CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT`, `CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE`) before the
    identity's own variables are applied.
  - Tests: `TestScrubAmbientEnvironment`, `TestScrubAmbientEnvironment_RemovesEveryManagedVariable`,
    `TestPrepareEnvironment_ScrubsCallerGCPEnvironment`.
- **gcloud properties go where the active configuration reads them** (#3720 suspected item,
  `pkg/auth/cloud/gcp/files.go`, `env.go`, `doc.go`).
  - Problem: Atmos set `CLOUDSDK_ACTIVE_CONFIG_NAME=config_atmos` but wrote the project and region
    properties to `$CLOUDSDK_CONFIG/properties`. gcloud treats the variable as a configuration name and
    loads `configurations/config_<name>` (here `config_config_atmos`). It never reads the root
    `properties` file while a named configuration is active, so the properties were dead.
  - Fix: the active configuration is now `atmos` (`ActiveConfigName`), and `GetPropertiesFilePath`
    returns `configurations/config_atmos`, creating the directory. This is the layout `doc.go` already
    described.
  - Tests: `TestSetupFiles_PropertiesAreWhereGcloudReadsThem`,
    `TestGetPropertiesFilePath_ConfigurationsDirFailure`,
    `TestWritePropertiesFile_PathMatchesActiveConfig`, plus updated `TestGetPropertiesFilePath`,
    `TestSetupFiles`, `TestGetEnvironmentVariables` and `TestSetEnvironmentVariables`.
- **A WIF STS response without an access token is rejected** (#3720 suspected item,
  `pkg/auth/providers/gcp_wif/provider.go`).
  - Problem: `exchangeToken` accepted any 200 from Google STS, so a body with a missing or empty
    `access_token` produced federated credentials with an empty token that failed far from the cause.
  - Fix: a blank access token returns `ErrAuthenticationFailed`.
  - Test: `TestExchangeToken_EmptyAccessToken`.

### Errors, schema and docs

- New sentinels in `errors/errors_integrations.go`: `ErrAzureCLIMSALCacheUpdate`,
  `ErrAzureCLIMSALCacheUnparseable`, `ErrAzureMSALCacheUnparseable`, `ErrACRInvalidTenant`,
  `ErrACRTenantOverrideUnsupported`.
- Two mockgen mocks were generated for the new ACR interfaces
  (`mock_acr_token_credential_test.go`, `mock_acr_tenant_token_client_test.go`).
- The `Registry.Name` description in `pkg/schema/schema_auth.go` and the config schema
  (`pkg/datafetcher/schema/atmos/config/1.0.json`) now lists all three ACR suffixes.
- Docs: `website/docs/cli/commands/azure/acr-login.mdx` gained "Sovereign Clouds" and "Registries in
  Another Tenant" sections and the new `--registry` formats. `website/docs/cli/configuration/auth/providers.mdx`
  lists ACR login servers among what `cloud_environment` adjusts. The ACR agent-skill reference
  (`agent-skills/skills/atmos-auth/references/azure-acr-integration.md`) and
  `docs/prd/azure-aks-acr-integrations.md` are updated.

## Validation

All of the following ran in this branch's worktree.

- `go build ./...`: passed.
- `go vet` on the changed packages (`./cmd/azure/acr`, `./errors`, `./pkg/auth/cloud/azure`,
  `./pkg/auth/cloud/gcp`, `./pkg/auth/identities/azure`, `./pkg/auth/identities/gcp_service_account`,
  `./pkg/auth/integrations/azure`, `./pkg/auth/providers/azure`, `./pkg/auth/providers/gcp_wif`,
  `./pkg/auth/registryauth`, `./pkg/schema`, `./pkg/datafetcher`): passed.
- `go test -count=1 -cover` on the same packages: all passed. This PR does not touch the `./cmd` root
  package or `./internal/exec`.

Statement coverage, `origin/main` (`2d7822f214`) versus this branch:

| Package | Before | After |
|---|---|---|
| `cmd/azure/acr` | 88.6% | 89.7% |
| `errors` | 87.3% | 87.3% |
| `pkg/auth/cloud/azure` | 95.0% | 96.7% |
| `pkg/auth/cloud/gcp` | 88.5% | 88.9% |
| `pkg/auth/identities/azure` | 89.7% | 89.9% |
| `pkg/auth/identities/gcp_service_account` | 88.1% | 88.1% |
| `pkg/auth/integrations/azure` | 93.6% | 94.1% |
| `pkg/auth/providers/azure` | 78.7% | 79.1% |
| `pkg/auth/providers/gcp_wif` | 96.6% | 96.6% |
| `pkg/auth/registryauth` | 95.5% | 95.9% |
| `pkg/schema` | 91.0% | 91.0% |
| `pkg/datafetcher` | 56.0% | 56.0% |

Not run:

- `golangci-lint` and the race detector.
- The website build (`cd website && npm run build`) for the two changed `.mdx` pages.
- Live Azure, ACR, gcloud and Google STS calls. All cloud calls in the tests are mocked or served by
  local test servers.

## Follow-ups

These choices were made along the way and can be revisited:

- **`tenant_id` override scope.** It works only for `azure/device-code` and `azure/interactive`
  identities. Supporting `azure/oidc` and `azure/cli` would need provider changes: Atmos keeps no
  credential those providers could use to get a token from another tenant.
- **ACR logout covers every cloud.** Logout removes the registry's Docker login for `azurecr.io`,
  `azurecr.us` and `azurecr.cn`, because it runs without the provider's credentials. Only entries that
  exist are touched.
- **Damaged `az` cache on the shared profile path.** On the device-code and interactive path a damaged
  `~/.azure/msal_token_cache.json` is logged as a warning. On the `UpdateAzureCLIFiles` path (used by
  the other Azure identities) the skipped update is logged only at debug level, so `az` silently does
  not see that login until the file is repaired.

Pre-existing gap noted by the regression test and not fixed here (needs a decision: fix now, or open an
issue): ambient `GOOGLE_*`, `CLOUDSDK_CORE_ACCOUNT`, `ARM_CLIENT_SECRET` and `AZURE_CLIENT_ID` still pass
through for `gcp/project` and `azure/cli` identities. This PR scrubs them only for
`gcp/service-account`.

Compatibility notes for users:

- `azure/cli` with `subscription_id` set now calls `az account get-access-token --subscription <id>`
  without `--tenant`, so the token comes from the subscription's tenant.
- gcloud now reads Atmos's project and region from `configurations/config_atmos`, with
  `CLOUDSDK_ACTIVE_CONFIG_NAME=atmos`. Scripts that read the old `properties` file or expect
  `config_atmos` as the configuration name must be updated.
- `gcp/service-account` subprocesses no longer see the caller's GCP credential, project and location
  variables.
- An ACR `spec.registry.tenant_id` that is not a GUID or domain now fails when the integration loads,
  and one naming another tenant fails for `azure/cli` and `azure/oidc` identities instead of being
  ignored.
