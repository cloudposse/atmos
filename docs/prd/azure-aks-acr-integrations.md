# Azure AKS/ACR Integrations PRD

## Executive Summary

Atmos already integrates AWS EKS (`atmos aws eks token`, `atmos aws eks update-kubeconfig`) and
AWS ECR (`atmos aws ecr login`) into its identity/auth system, so authenticating with an AWS
identity can also configure `kubectl` and Docker in one step. Azure had the underlying auth
foundation (providers, identities, credential types) but no equivalent for Azure Kubernetes
Service (AKS) or Azure Container Registry (ACR).

This PRD documents the design and implementation that extends the same `auth.integrations`
pattern to Azure: `atmos azure aks token`, `atmos azure aks update-kubeconfig`, and
`atmos azure acr login`, mirroring the EKS/ECR architecture so Azure users get the same
SDK-only experience — no `az` CLI or `kubelogin` binary dependency.

**Key Design Decision:** As with EKS/ECR, AKS kubeconfig and ACR Docker login are
**integrations** (not identities) — client-side credential materializations derived from an
existing Azure identity, not identities themselves.

## Problem Statement

Before this work, Azure users authenticating with `atmos auth login` had no automated path to:

1. **AKS kubeconfig**: Users had to run `az aks get-credentials`, which requires the Azure CLI
   and (for AAD-integrated clusters, the modern default) the `kubelogin` binary.
2. **ACR Docker login**: Users had to run `az acr login`, again requiring the Azure CLI.
3. **Credential mismatch**: `az` CLI commands use whatever `az` is currently logged into, which
   may not match the Atmos-managed identity the user authenticated with.

### Desired Workflow

```bash
# Single command authenticates AND sets up kubeconfig + Docker credentials
$ atmos auth login azure-dev
✓ AKS kubeconfig: dev-cluster → ~/.config/atmos/kube/config
✓ ACR login: myregistry.azurecr.io (expires in 2h59m)

$ kubectl get pods
$ docker pull myregistry.azurecr.io/myimage:latest
```

## Design Goals

1. **No external binaries**: Eliminate any dependency on the `az` CLI or `kubelogin` — use the
   Azure Go SDK (`armcontainerservice`) directly, matching the EKS precedent of using
   `aws-sdk-go-v2` instead of shelling out.
2. **Shared spec shape**: `IntegrationSpec.Cluster`/`.Registry` are reused, unmodified, across
   both clouds — `kind` (`aws/eks` vs `azure/aks`, `aws/ecr` vs `azure/acr`) determines which
   fields apply, not a forked per-cloud struct. This was an explicit user requirement: "the whole
   purpose of having different kinds is so the specs can be similar."
3. **Cloud-agnostic kubeconfig writer**: Generalize `pkg/auth/cloud/kube.KubeconfigManager`
   (previously typed to AWS's `EKSClusterInfo`) to a cloud-agnostic `ClusterInfo` struct, so both
   EKS and AKS share one merge/diff/write implementation instead of forking a parallel writer.
4. **Non-blocking failures**: Integration failures during `atmos auth login` warn, not fail
   authentication (same as EKS/ECR).
5. **Testability**: Mockable SDK client interfaces (`AKSClient`), HTTP client injection for the
   ACR OAuth2 exchange, and function-var test seams throughout, matching the EKS/ECR pattern.

## Technical Specification

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│ atmos auth login azure-dev                                       │
└─────────────────────────────────┬───────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│ AuthManager.Authenticate()                                       │
│  1. Authenticate with provider (device-code/OIDC/CLI)            │
│     → also acquires an AKS-scoped AAD token (see below)          │
│  2. Get identity credentials (azure/subscription)                │
│  3. triggerIntegrations() → Run linked integrations (non-fatal)  │
└─────────────────────────────────┬───────────────────────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    │                           │
                    ▼                           ▼
          ┌─────────────────┐         ┌──────────────────┐
          │ ACR Integration │         │ AKS Integration  │
          │ (azure/acr)     │         │ (azure/aks)      │
          │                 │         │                  │
          │ OAuth2 exchange │         │ ManagedClusters  │
          │ Write Docker    │         │  .Get + .ListCluster│
          │ config.json     │         │  UserCredentials │
          └─────────────────┘         │ Write Kubeconfig │
                                       │ (exec: atmos)    │
                                       └──────────────────┘
                                               │
                                      (later, at kubectl time)
                                               │
                                               ▼
                                      ┌──────────────────┐
                                      │ kubectl exec     │
                                      │ → atmos azure    │
                                      │   aks token      │
                                      │                  │
                                      │ Reads cached     │
                                      │ AKS-scoped token │
                                      └──────────────────┘
```

### Shared Configuration Schema

`pkg/schema/schema_auth.go` widens the existing `EKSCluster`/`ECRRegistry` structs (renamed to
`Cluster`/`Registry`) rather than introducing Azure-specific siblings — their YAML keys
(`spec.cluster`, `spec.registry`) are unchanged:

```go
// Cluster is shared by aws/eks and azure/aks integrations.
type Cluster struct {
    Name           string // required by both
    Region         string // required for aws/eks
    ResourceGroup  string // required for azure/aks
    SubscriptionID string // optional for azure/aks — defaults to identity's subscription
    Alias          string
    Kubeconfig     *KubeconfigSettings // unchanged, already cloud-agnostic
}

// Registry is shared by aws/ecr and azure/acr integrations.
type Registry struct {
    AccountID string // required for aws/ecr
    Region    string // required for aws/ecr
    Name      string // required for azure/acr (login server = name + ".azurecr.io")
    TenantID  string // optional for azure/acr — defaults to identity's tenant
}
```

Each integration's constructor (`NewEKSIntegration`, `NewAKSIntegration`, `NewECRIntegration`,
`NewACRIntegration`) validates only the fields relevant to its own `kind`.

```yaml
auth:
  integrations:
    dev/aks:
      kind: azure/aks
      via:
        identity: azure-dev
      spec:
        cluster:
          name: dev-cluster
          resource_group: dev-rg
          alias: dev-aks

    dev/acr:
      kind: azure/acr
      via:
        identity: azure-dev
      spec:
        registry:
          name: myregistry
```

### Generalized Kubeconfig Writer

`pkg/auth/cloud/kube/config.go` previously took `*awsCloud.EKSClusterInfo` directly. It now takes
a cloud-agnostic `ClusterInfo`:

```go
type ClusterInfo struct {
    Name                     string
    Endpoint                 string
    CertificateAuthorityData string   // base64, decoded the same way for every cloud
    ID                       string   // ARN (AWS) or ARM resource ID (Azure)
    Region                   string   // AWS region or Azure resource group (username uniqueness)
    UserPrefix               string   // "eks" or "aks"
    ExecArgs                 []string // fully-built exec-plugin args, built by the caller
    ExecEnv                  []clientcmdapi.ExecEnvVar
}
```

`BuildClusterConfig`/`WriteClusterConfig` no longer hardcode the `aws eks token` exec command —
each cloud package builds its own adapter:

- `pkg/auth/cloud/aws.BuildKubeClusterInfo(info *EKSClusterInfo, identity string) *kube.ClusterInfo`
- `pkg/auth/cloud/azure.BuildKubeClusterInfo(info *AKSClusterInfo, identity string) *kube.ClusterInfo`

`ListClusterARNs` was renamed to `ListClusterIDs` since it now returns AWS ARNs *or* Azure ARM
resource IDs. A regression test suite in `pkg/auth/cloud/kube/config_test.go` locks in that the
AWS-shaped output is byte-identical to pre-refactor behavior.

### AKS: Cluster Description Without `kubelogin`

`az aks get-credentials --format exec` returns a kubeconfig whose exec plugin invokes the
`kubelogin` binary — exactly the dependency this PRD avoids. Instead,
`pkg/auth/cloud/azure/aks.go`'s `DescribeCluster`:

1. Calls `ManagedClustersClient.Get` for the cluster's ARM resource ID.
2. Calls `ManagedClustersClient.ListClusterUserCredentials` with `Format: exec` — the response
   embeds a ready-made kubeconfig (server endpoint, CA data, and a `kubelogin` exec block) that
   Atmos **parses but does not use verbatim**.
3. Extracts `Server`/`CertificateAuthorityData` from the embedded kubeconfig's single cluster
   entry, and scans the embedded `kubelogin` exec args for `--server-id`/`--tenant-id` — the AAD
   server application ID and tenant *that specific cluster* expects. This is preferred over
   hardcoding the well-known AKS server app ID (`6dae42f8-4368-4678-94ff-3960e28e3630`), since it
   is robust to both AKS-managed AAD (the modern default) and legacy BYO-server-app clusters.
4. If the embedded kubeconfig has no exec block (the cluster isn't AAD-enabled — `Format: exec`
   silently falls back to a local-account kubeconfig), `DescribeCluster` fails with
   `ErrAKSClusterNotAADEnabled`. Only AAD-integrated clusters are supported.
5. Atmos then builds its **own** kubeconfig exec entry pointing at `atmos azure aks token`
   instead of `kubelogin`.

### AKS: Token Generation and Scope-Bound AAD Tokens

Unlike AWS SigV4 (one long-lived credential can sign a request for any service at call time),
Azure AAD access tokens are scope-bound at issuance. EKS's `atmos aws eks token` can call STS at
token-generation time with the target cluster baked into the presigned URL; AKS has no equivalent
call-time trick — the identity's *provider* must acquire an AKS-scoped token during
authentication, mirroring the existing Microsoft Graph / KeyVault "additional token" pattern:

- `pkg/auth/types/azure_credentials.go`: `AzureCredentials` gains `AKSToken`/`AKSTokenExpiration`
  fields (same shape as `GraphAPIToken`/`GraphAPIExpiration`).
- `pkg/auth/cloud/azure/constants.go`: `AKSServerAppID` (well-known GUID, same across
  public/government/china clouds) and `AKSServerScope = AKSServerAppID + "/.default"`.
- All three Azure identity providers acquire this token, non-fatally, alongside their existing
  Graph/KeyVault acquisitions:
  - `pkg/auth/providers/azure/device_code.go` — a third `AcquireTokenSilent` call in both the
    silent-reauth and post-device-code paths.
  - `pkg/auth/providers/azure/oidc.go` — a third `exchangeToken` goroutine in
    `acquireAdditionalTokens`.
  - `pkg/auth/providers/azure/cli.go` — a **new** capability (this provider didn't do
    Graph/KeyVault acquisition before): a second `az account get-access-token --resource
    <AKSServerAppID>` shellout.
- `pkg/auth/identities/azure/subscription.go` propagates `AKSToken`/`AKSTokenExpiration` through
  its manual `AzureCredentials` field copy.
- `pkg/auth/cloud/azure.GetToken(creds) (token string, expiresAt time.Time, err error)` simply
  reads the pre-acquired token; if empty, it errors pointing at the identity's provider.

**Known limitation:** the login-time token is scoped to the well-known `AKSServerAppID`. If
`DescribeCluster` discovers a cluster expecting a *different* AAD server application (a legacy
BYO-server-app cluster), `AKSIntegration.Execute` logs a warning — Atmos cannot re-acquire a
differently-scoped token after the fact without a larger provider redesign (see Future
Enhancements).

### ACR: OAuth2 Token Exchange (No ARM SDK Needed)

Unlike AKS, ACR login needs no Azure Resource Manager SDK call — `az acr login`'s underlying
mechanism is a plain HTTPS POST:

```
POST https://{login-server}/oauth2/exchange
Content-Type: application/x-www-form-urlencoded

grant_type=access_token&service={login-server}&tenant={tenant-id}&access_token={AAD-token}
```

The response's `refresh_token` becomes the Docker password, with the fixed username
`00000000-0000-0000-0000-000000000000`. `pkg/auth/cloud/azure/acr.go`'s
`GetAuthorizationToken` performs this exchange via the shared `pkg/http.Client` interface
(mockable in tests), and decodes the refresh token's own JWT `exp` claim for the expiration
(the exchange response carries no `expires_in`).

### CLI Commands

| Command | Mirrors | Notes |
|---|---|---|
| `atmos azure aks token` | `atmos aws eks token` | Same `ExecCredential` JSON contract (`client.authentication.k8s.io/v1beta1`). No `exportAWSCredsToEnv` equivalent — Azure SDK calls take credentials explicitly, no ambient env-var chain. |
| `atmos azure aks update-kubeconfig` | `atmos aws eks update-kubeconfig` | Only `--integration` and direct-SDK (`--cluster-name`/`--resource-group`/`--identity`) modes — no legacy CLI-shellout branch, unlike EKS's legacy AWS-CLI path. |
| `atmos azure acr login [integration]` | `atmos aws ecr login` | No `--public` mode — ACR has no equivalent to ECR Public; every registry is private. |

### Package Structure (as implemented)

```
pkg/auth/
  cloud/
    aws/
      eks.go                 # MODIFIED: added BuildKubeClusterInfo adapter
    azure/
      config.go               # NEW: BuildAzureCredentialFromCreds, BuildARMClientOptions
      aks.go                  # NEW: AKSClient, DescribeCluster, GetToken, BuildKubeClusterInfo
      acr.go                  # NEW: GetAuthorizationToken, BuildRegistryURL, ParseRegistryURL
      constants.go             # MODIFIED: AKSServerAppID, AKSServerScope
    kube/
      config.go                # MODIFIED: EKSClusterInfo → cloud-agnostic ClusterInfo
  integrations/
    types.go                   # MODIFIED: KindAzureAKS, KindAzureACR
    azure/
      aks.go                   # NEW: azure/aks Integration implementation
      acr.go                   # NEW: azure/acr Integration implementation
  providers/azure/
    device_code.go, oidc.go, cli.go  # MODIFIED: AKS-scope token acquisition
  identities/azure/
    subscription.go            # MODIFIED: propagate AKSToken/AKSTokenExpiration
  types/
    azure_credentials.go       # MODIFIED: AKSToken/AKSTokenExpiration fields, exported StaticTokenCredential

cmd/azure/
  azure.go                     # NEW: parent CommandProvider
  aks/
    aks.go, token.go, update_kubeconfig.go, update_kubeconfig_sdk.go
  acr/
    acr.go, login.go

pkg/schema/schema_auth.go      # MODIFIED: EKSCluster → Cluster, ECRRegistry → Registry
errors/errors.go               # MODIFIED: ErrAKS*, ErrACR* sentinels
```

### Dependency

`go.mod` gains `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6`
for `ManagedClustersClient`. No `armcontainerregistry` dependency — ACR login is a plain HTTP
call, not an ARM SDK call.

## Testing Strategy

- **`pkg/auth/cloud/kube`**: regression suite asserting the generalized `BuildClusterConfig`
  produces byte-identical output for AWS-shaped `ClusterInfo` inputs as before the refactor, plus
  new assertions for Azure-shaped inputs.
- **`pkg/auth/cloud/aws`**: `BuildKubeClusterInfo` unit tests (with/without identity).
- **`pkg/auth/cloud/azure`**: `aks_test.go` builds real exec-format kubeconfig fixtures via
  `clientcmd.Write` (both AAD-enabled and local-account shapes) and round-trips them through
  `DescribeCluster` via a mockgen'd `AKSClient`; `acr_test.go` mocks the HTTP exchange via
  `pkg/http.MockClient`; `config_test.go` covers the ARM client options mapping.
  All get their own `BuildKubeClusterInfo` tests.
- **`pkg/auth/integrations/azure`**: `aks_test.go`/`acr_test.go` mirror the AWS integration test
  suites — validation, `Execute`/`Cleanup`/`Environment`, function-var test seams.
- **`pkg/auth/providers/azure`**: `oidc_test.go`'s existing `TestOIDCProvider_Authenticate` test
  extended to assert `AKSToken`/`AKSTokenExpiration` are populated end-to-end (the same httptest
  server serves all scope-exchange requests, since the handler doesn't distinguish by scope).
  `device_code.go`/`cli.go`'s AKS-scope paths are covered by build/vet correctness and code
  review — MSAL's `public.Client` and `az` CLI shellouts aren't practical to unit-test without a
  live/mocked MSAL server or `az` binary, matching the pre-existing gap for their Graph/KeyVault
  paths.
- **`cmd/azure/aks`, `cmd/azure/acr`**: mirror `cmd/aws/eks`/`cmd/aws/ecr`'s command-level test
  suites (flag registration, dispatch, DI-based `RunE` overrides).

## Security Considerations

Same posture as EKS/ECR:

1. **Credential isolation**: kubeconfig uses an exec plugin — no long-lived tokens stored on disk.
2. **File permissions**: kubeconfig written with `0600`; ACR/ECR credentials go through the
   existing `pkg/auth/cloud/docker` writer, unchanged.
3. **Token scope**: the AKS-scoped token is acquired specifically for the AKS-managed server
   application — it is not the identity's primary ARM-scoped token, limiting blast radius if
   leaked.
4. **No secrets in kubeconfig**: only cluster endpoint and CA cert are stored; auth is via exec.

## Success Metrics

1. AKS kubeconfig and ACR Docker login generated correctly with valid output.
2. `kubectl`/`docker` can authenticate using the generated credentials against a real AAD-enabled
   AKS cluster / ACR registry (not exercised in this sandbox — no live Azure subscription
   available; verified via unit tests exercising the real `clientcmd`/JWT/kubeconfig-merge code
   paths against synthetic fixtures).
3. Auto-provisioning triggers both integrations on `atmos auth login`, non-blocking on failure.
4. Full repository test suite (`go test ./...`) passes with the AWS EKS/ECR regression suite
   unchanged in behavior.

## Future Enhancements

1. **Non-default AAD server application support**: re-acquire an AKS token scoped to a
   cluster-specific `--server-id` discovered during `DescribeCluster`, rather than only warning
   when it differs from the well-known default. Requires threading a scope parameter back into
   the provider's token-acquisition call, which today only runs once at `Authenticate()` time.
2. **ACR admin/service-principal auth modes**: today only AAD-token-based login is supported,
   matching `az acr login`'s primary mode; ACR admin-user credentials are out of scope.
3. **CI/CD workflow for `atmos azure aks token`**: define behavior when Azure credentials come
   from federated/managed identity in CI rather than `atmos auth login` (mirrors the same open
   question already tracked for `atmos aws eks token`).

## References

- [EKS Kubeconfig Authentication PRD](./eks-kubeconfig.md) — the AWS precedent this design mirrors
- [ECR Authentication PRD](./ecr-authentication.md) — the AWS precedent this design mirrors
- [Azure AKS `ListClusterUserCredentials` REST API](https://learn.microsoft.com/en-us/rest/api/aks/managed-clusters/list-cluster-user-credentials)
- [ACR AAD OAuth2 token exchange](https://github.com/Azure/acr/blob/main/docs/AAD-OAuth.md)
- [Kubernetes client-go exec credentials](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#client-go-credential-plugins)

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-07-23 | AI Assistant | Initial PRD, documenting the as-built implementation |
