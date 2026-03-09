# EKS Kubeconfig Authentication Integration PRD

## Executive Summary

In cloud environments, obtaining Kubernetes credentials is tied to a cloud identity. On AWS, authenticating to EKS requires calling the AWS API to generate a kubeconfig for the current session. Since Atmos already manages authentication with cloud providers, a natural extension is for Atmos to provision the Kubernetes authentication configuration as part of that same workflow — allowing a developer to be fully authenticated with everything they need in a single step.

This PRD defines the design for that extension: integrating EKS kubeconfig generation into the Atmos auth system. When a user runs `atmos auth login`, Atmos can automatically configure kubeconfig for linked EKS clusters alongside the AWS credentials it already manages. This follows the same integration pattern established by ECR authentication (PR #1859).

To be clear, Atmos does not provision identities. This is strictly about configuring the local environment to support working with Kubernetes once the appropriate cloud identity is in place.

**Key Design Decision:** EKS kubeconfig is an **integration** (not an identity) because it represents client-side credential configuration derived from an existing AWS identity — the kubeconfig itself is not an identity, but a file that enables `kubectl` to authenticate against EKS clusters using the identity Atmos already manages.

## Problem Statement

Today, after authenticating with Atmos, users must still perform manual steps to access EKS clusters:

1. **Manual kubeconfig setup**: Users must run `aws eks update-kubeconfig --name <cluster> --region <region>` for each cluster, even though Atmos already holds the credentials needed to do this.

2. **Credential mismatch**: The `aws eks update-kubeconfig` command uses ambient AWS credentials, which may not match the Atmos-managed identity the user authenticated with.

3. **Repetitive across clusters**: Users working with multiple clusters repeat this process for each one, often multiple times per day as credentials expire.

4. **Token refresh complexity**: EKS exec credential plugins have short token lifetimes (~15 minutes), and users must ensure their AWS credentials remain valid when kubectl makes API calls.

5. **AWS CLI dependency**: The existing `atmos aws eks update-kubeconfig` command shells out to the AWS CLI rather than using the Go SDK directly.

### User Impact

**Current Workflow (Manual):**
```bash
# Step 1: Authenticate with Atmos
$ atmos auth login dev-admin

# Step 2: Manually update kubeconfig for each cluster
$ aws eks update-kubeconfig --name dev-cluster --region us-east-2
$ aws eks update-kubeconfig --name staging-cluster --region us-east-1
$ aws eks update-kubeconfig --name prod-cluster --region us-west-2

# Step 3: Use kubectl
$ kubectl get pods
```

**Desired Workflow (Automated):**
```bash
# Single command authenticates AND sets up kubeconfig
$ atmos auth login dev-admin
# Kubeconfig automatically configured for all linked EKS clusters

# Explicit identity via flag (equivalent to positional arg)
$ atmos auth login --identity dev-admin

# Interactive identity selection (when multiple identities exist)
$ atmos auth login --identity

# With configuration profile overlay
$ atmos auth login dev-admin --profile production

# Or explicit cluster setup using existing atmos aws command
$ atmos aws eks update-kubeconfig dev/eks

# Ready to use kubectl
$ kubectl get pods
```

## Design Goals

1. **Seamless integration**: EKS kubeconfig setup should happen automatically during `atmos auth login` when configured
2. **Use AWS Go SDK**: Eliminate dependency on AWS CLI entirely — use `eks.DescribeCluster()` for kubeconfig generation and `atmos auth eks-token` as the exec credential plugin
3. **XDG compliance**: Store kubeconfig in XDG-compliant locations (`~/.config/atmos/kube/config`)
4. **Merge support**: Append cluster configurations to existing kubeconfig without overwriting
5. **Multiple clusters**: Support multiple EKS integrations linking to the same identity
6. **Enhance existing command**: Update `atmos aws eks update-kubeconfig` to use Go SDK and support integrations
7. **Non-blocking failures**: Integration failures during login should warn, not fail authentication
8. **Testability**: Design for unit testing with mocked AWS clients

## Technical Specification

### Architecture Overview

EKS follows the **integration pattern** established by ECR authentication (PR #1859):

```
┌─────────────────────────────────────────────────────────────────┐
│ atmos auth login dev-admin                                      │
└─────────────────────────────────┬───────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│ AuthManager.Authenticate()                                       │
│  1. Authenticate with provider (SSO/SAML)                       │
│  2. Get identity credentials (permission-set/assume-role)       │
│  3. PostAuthenticate() → Setup AWS files                        │
│  4. triggerIntegrations() → Run linked integrations (non-fatal)  │
└─────────────────────────────────┬───────────────────────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    │                           │
                    ▼                           ▼
          ┌─────────────────┐         ┌──────────────────┐
          │ ECR Integration │         │ EKS Integration  │
          │ (aws/ecr)       │         │ (aws/eks)        │
          │                 │         │                  │
          │ GetAuthToken()  │         │ DescribeCluster()│
          │ Write Docker    │         │ Write Kubeconfig │
          │ config.json     │         │ (exec: atmos)    │
          └─────────────────┘         └──────────────────┘
                                               │
                                      (later, at kubectl time)
                                               │
                                               ▼
                                      ┌──────────────────┐
                                      │ kubectl exec     │
                                      │ → atmos auth     │
                                      │   eks-token      │
                                      │                  │
                                      │ GetCallerIdentity│
                                      │ → Bearer token   │
                                      └──────────────────┘
```

### Integration Interface

EKS implements the `Integration` interface. This PRD proposes extending the current interface
(which has only `Kind()` and `Execute()`) with `Cleanup()` and `Environment()` methods to
support logout cleanup and environment variable composition:

```go
// Integration interface (defined in pkg/auth/integrations/types.go).
// Current interface has Kind() and Execute(). This PRD adds Cleanup() and Environment().
type Integration interface {
    // Kind returns the integration type (e.g., "aws/ecr", "aws/eks").
    Kind() string

    // Execute performs the integration using the provided credentials.
    // Returns nil on success, error on failure.
    Execute(ctx context.Context, creds types.ICredentials) error

    // Cleanup reverses the effects of Execute (e.g., removes kubeconfig entries, docker logout).
    // Called during identity/provider logout to clean up integration artifacts.
    // Idempotent — returns nil if nothing to clean up.
    // Errors are non-fatal during logout (logged as warnings, do not block logout).
    Cleanup(ctx context.Context) error

    // Environment returns environment variables contributed by this integration.
    // Returns vars based on configuration (deterministic), not Execute() output.
    // Called by the manager when composing env vars for atmos auth env / auth shell.
    Environment() (map[string]string, error)
}

// IntegrationConfig wraps the schema.Integration with the integration name.
type IntegrationConfig struct {
    Name   string
    Config *schema.Integration
}

// IntegrationFactory creates integrations from configuration.
type IntegrationFactory func(config *IntegrationConfig) (Integration, error)
```

**Note:** The `Integration` interface intentionally does not include an `Identity()` method.
Identity linkage is handled via `IntegrationConfig.Config.Via.Identity` at construction time,
and individual integrations may expose it via struct methods (e.g., `GetIdentity()`) if needed.

### Configuration Schema

#### Schema Types

Add to `pkg/schema/schema_auth.go`:

```go
// EKSCluster represents an EKS cluster configuration for aws/eks integrations.
type EKSCluster struct {
    // Name is the EKS cluster name (required).
    Name string `yaml:"name" json:"name" mapstructure:"name"`

    // Region is the AWS region where the cluster is located (required).
    Region string `yaml:"region" json:"region" mapstructure:"region"`

    // Alias is the context name in kubeconfig (optional, defaults to cluster ARN).
    Alias string `yaml:"alias,omitempty" json:"alias,omitempty" mapstructure:"alias"`

    // Kubeconfig contains kubeconfig file settings (optional).
    Kubeconfig *KubeconfigSettings `yaml:"kubeconfig,omitempty" json:"kubeconfig,omitempty" mapstructure:"kubeconfig"`
}

// KubeconfigSettings configures kubeconfig file behavior.
type KubeconfigSettings struct {
    // Path is a custom kubeconfig file path (optional, defaults to XDG path).
    Path string `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`

    // Mode is the file permission mode as an octal string (optional, defaults to "0600").
    // Parsed via strconv.ParseUint(mode, 8, 32) at config-load time (e.g., "0600" → 0o600).
    // Invalid values (e.g., "abc", "999") are rejected with a validation error
    // referencing KubeconfigSettings.Mode.
    //
    // Design decision: Mode is a string (not os.FileMode/int) because YAML octal
    // parsing is ambiguous — YAML 1.1 treats 0600 as octal (384), but YAML 1.2
    // (used by Go's yaml.v3) treats it as decimal 600. A quoted string "0600"
    // with explicit octal parsing avoids this ambiguity entirely.
    Mode string `yaml:"mode,omitempty" json:"mode,omitempty" mapstructure:"mode"`

    // Update determines how to handle existing kubeconfig: "merge" (default), "replace", or "error".
    // Validated at config-load time; invalid values (e.g., "invalid") are rejected with a
    // validation error referencing KubeconfigSettings.Update. Defaults to "merge" when empty.
    Update string `yaml:"update,omitempty" json:"update,omitempty" mapstructure:"update"`
}
```

Update `IntegrationSpec`:

```go
type IntegrationSpec struct {
    AutoProvision *bool        `yaml:"auto_provision,omitempty" json:"auto_provision,omitempty" mapstructure:"auto_provision"`
    Registry      *ECRRegistry `yaml:"registry,omitempty" json:"registry,omitempty" mapstructure:"registry"`
    Cluster       *EKSCluster  `yaml:"cluster,omitempty" json:"cluster,omitempty" mapstructure:"cluster"`  // NEW
}
```

#### Configuration Example

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      start_url: https://company.awsapps.com/start
      region: us-east-1

  identities:
    dev-admin:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: AdministratorAccess
        account:
          id: "123456789012"

  integrations:
    # EKS integration - auto-provisions kubeconfig on identity login
    dev/eks:
      kind: aws/eks
      via:
        identity: dev-admin
      spec:
        auto_provision: true
        cluster:
          name: dev-cluster
          region: us-east-2
          alias: dev-eks

    # Multiple clusters can link to the same identity
    staging/eks:
      kind: aws/eks
      via:
        identity: dev-admin
      spec:
        auto_provision: true
        cluster:
          name: staging-cluster
          region: us-east-1
          alias: staging-eks
```

### AWS SDK Integration

#### Required Dependency

Add to `go.mod`:

```
github.com/aws/aws-sdk-go-v2/service/eks
```

#### EKS API Usage

```go
// DescribeCluster retrieves cluster information needed for kubeconfig.
// Uses the existing buildAWSConfigFromCreds helper from pkg/auth/cloud/aws/ecr.go
// (should be extracted to a shared location, e.g., pkg/auth/cloud/aws/config.go).
func DescribeCluster(ctx context.Context, creds types.ICredentials, clusterName, region string) (*EKSClusterInfo, error) {
    // Reuse the shared AWS config builder (currently in ecr.go, extract to config.go).
    cfg, err := buildAWSConfigFromCreds(ctx, creds, region)
    if err != nil {
        return nil, fmt.Errorf("%w: %w", errUtils.ErrEKSDescribeCluster, err)
    }

    client := eks.NewFromConfig(cfg)

    output, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{
        Name: aws.String(clusterName),
    })
    if err != nil {
        return nil, fmt.Errorf("%w: %w", errUtils.ErrEKSDescribeCluster, err)
    }

    return &EKSClusterInfo{
        Name:                     clusterName,
        Endpoint:                 aws.ToString(output.Cluster.Endpoint),
        CertificateAuthorityData: aws.ToString(output.Cluster.CertificateAuthority.Data),
        ARN:                      aws.ToString(output.Cluster.Arn),
        Region:                   region,
    }, nil
}
```

**Implementation Note:** The `buildAWSConfigFromCreds` helper currently lives in
`pkg/auth/cloud/aws/ecr.go` (unexported). Before implementing EKS, extract it to a
shared file (e.g., `pkg/auth/cloud/aws/config.go`) and export it as `BuildAWSConfigFromCreds`
so both ECR and EKS can reuse it.

### Kubeconfig Generation

#### Output Format

The generated kubeconfig uses `atmos` as the exec credential plugin, eliminating the AWS CLI dependency:

```yaml
apiVersion: v1
kind: Config
current-context: dev-eks
clusters:
- name: arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster
  cluster:
    server: https://XXXXXXXXXXXX.gr7.us-east-2.eks.amazonaws.com
    certificate-authority-data: LS0tLS1CRUdJTi...
contexts:
- name: dev-eks
  context:
    cluster: arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster
    user: user-dev-cluster
users:
- name: user-dev-cluster
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: atmos
      args:
      - auth
      - eks-token
      - --cluster-name
      - dev-cluster
      - --region
      - us-east-2
      - --identity
      - dev-admin
      env:
      - name: ATMOS_IDENTITY
        value: dev-admin
      interactiveMode: Never
```

#### Exec Credential Plugin

The kubeconfig uses `atmos auth eks-token` as the exec plugin for authentication:

- **Command**: `atmos auth eks-token --cluster-name <name> --region <region> --identity <name>`
- **API Version**: `client.authentication.k8s.io/v1beta1`
- **Token Lifetime**: ~15 minutes (refreshed automatically by kubectl on expiration)
- **Credential Source**: Uses Atmos-managed AWS credentials via the specified identity
- **interactiveMode**: `Never` — token generation does not require a TTY

**Identity resolution at exec time (precedence order):**
1. `--identity` flag in exec args (embedded in kubeconfig at generation time)
2. `ATMOS_IDENTITY` env var (set in exec env array as fallback)
3. Default identity from `atmos.yaml` auth config

The `--identity` flag ensures deterministic credential selection when multiple Atmos
identities exist. Both the flag and the `env` array are populated at kubeconfig generation
time from the integration's `via.identity` field.

**Why `atmos` instead of `aws`:**
1. No dependency on the AWS CLI being installed
2. Uses Atmos-managed credentials directly (identity-aware)
3. Consistent with the Atmos auth system — credentials flow through `atmos auth`

**Implementation:** The `atmos auth eks-token` command needs to be implemented as a new
subcommand that outputs an `ExecCredential` JSON object compatible with
`client.authentication.k8s.io/v1beta1`. It should use the AWS STS `GetCallerIdentity`
pre-signed URL approach (same as `aws eks get-token`) to generate a bearer token.

**Flags for `atmos auth eks-token`:**
- `--cluster-name` (required) — EKS cluster name
- `--region` (required) — AWS region
- `--identity` (optional) — Atmos identity name for credential resolution

### File Path Management

#### XDG-Compliant Default Path

The `pkg/xdg` package handles subdirectory creation via its `subpath` parameter directly:

```go
// Default: ~/.config/atmos/kube/config
// GetXDGConfigDir("kube", 0o700) returns ~/.config/atmos/kube (creates directory)
// The kubeconfig file "config" is written inside that directory.
kubeDir, err := xdg.GetXDGConfigDir("kube", 0o700)
// kubeDir = ~/.config/atmos/kube
// Write kubeconfig to: kubeDir + "/config"
```

#### Environment Variable Integration

The EKS integration contributes `KUBECONFIG` to the environment via the `Environment()` method
on the `Integration` interface (see [Integration Interface](#integration-interface) and
[Integration Environment Variables](#integration-environment-variables)). The manager appends the
Atmos kubeconfig path to the existing `KUBECONFIG` using colon-separated notation (standard
kubectl behavior). The append is **idempotent** — if the Atmos path is already present, it
will not be duplicated.

```bash
# Before: KUBECONFIG=~/.kube/config
# After:  KUBECONFIG=~/.kube/config:~/.config/atmos/kube/config
```

**Multiple EKS integrations under one identity:** When an identity has multiple EKS integrations
(e.g., `dev/eks-blue` and `dev/eks-green`), all clusters are written to the same kubeconfig file
(as separate contexts). Both integrations return the same `KUBECONFIG` path from `Environment()`,
so the manager deduplicates before composing the colon-separated value.

Users can export all auth environment variables (including integration-contributed variables
like `KUBECONFIG`) at once:

```bash
eval $(atmos auth env)
# KUBECONFIG now includes the Atmos kubeconfig path
kubectl get pods
```

Or use `atmos auth shell` which sets all auth environment variables automatically:

```bash
atmos auth shell
# All auth environment variables set, including KUBECONFIG with Atmos path appended
kubectl get pods
```

### CLI Command

#### Command: `atmos aws eks update-kubeconfig`

The existing `atmos aws eks update-kubeconfig` command will be enhanced to:
1. Use AWS Go SDK instead of shelling out to AWS CLI
2. Support named integrations from `auth.integrations`
3. Use Atmos-managed AWS credentials when available

```
atmos aws eks update-kubeconfig [component] [flags]

Generate or update kubeconfig for AWS EKS clusters.

Usage:
  atmos aws eks update-kubeconfig [component]           Use component's EKS config
  atmos aws eks update-kubeconfig --integration <name>  Use named integration
  atmos aws eks update-kubeconfig --name <cluster>      Explicit cluster name

Flags (existing):
  -s, --stack string         Stack name (env: ATMOS_STACK)
      --name string          EKS cluster name (explicit mode)
      --region string        AWS region (env: ATMOS_AWS_REGION, AWS_REGION)
      --profile string       AWS CLI profile (env: ATMOS_AWS_PROFILE, AWS_PROFILE)
      --role-arn string      IAM role ARN to assume
      --alias string         Context alias in kubeconfig
      --kubeconfig string    Custom kubeconfig path (env: ATMOS_KUBECONFIG, KUBECONFIG)
      --dry-run              Print generated kubeconfig to stdout without writing
      --verbose              Enable verbose output

Flags (new):
      --integration string   Named integration from auth.integrations

Examples:
  # Using a component (existing behavior, now with Go SDK)
  atmos aws eks update-kubeconfig eks-cluster -s dev-use2

  # Using a named integration
  atmos aws eks update-kubeconfig --integration dev/eks

  # Explicit cluster
  atmos aws eks update-kubeconfig --name dev-cluster --region us-east-2 --alias dev
```

**Flag disambiguation:**

The `--profile` flag on `atmos aws eks update-kubeconfig` maps to `AWS_PROFILE` (the AWS CLI
profile to use for authentication). This is distinct from the global `--profile` flag which maps
to `ATMOS_PROFILE` (Atmos configuration profile overlays). The EKS command uses
`flags.WithViperPrefix("eks")` to namespace its Viper keys under `eks.*`, preventing collision
(see PR #2077).

The `--identity` flag is available on all `atmos auth` subcommands (including `atmos auth eks-token`)
as a PersistentFlag inherited from `authCmd`. It is **not** available on
`atmos aws eks update-kubeconfig` because this command lives under the `aws` command tree, not
`auth`. When using the `--integration` flag, identity resolution happens automatically from the
integration's `via.identity` field.

### Error Handling

#### Error Types

Add to `errors/errors.go`:

```go
// EKS integration errors.
var (
    ErrEKSDescribeCluster   = errors.New("failed to describe EKS cluster")
    ErrEKSClusterNotFound   = errors.New("EKS cluster not found")
    ErrEKSIntegrationFailed = errors.New("EKS integration failed")
    ErrKubeconfigPath       = errors.New("failed to determine kubeconfig path")
    ErrKubeconfigWrite      = errors.New("failed to write kubeconfig")
    ErrKubeconfigMerge      = errors.New("failed to merge kubeconfig")
)
```

#### Error Behavior by Context

| Context | Behavior |
|---------|----------|
| `atmos auth login` (auto-provision) | Warn and continue; don't fail authentication |
| `atmos aws eks update-kubeconfig` (explicit) | Return error to user |
| Invalid configuration | Validation error during config load |

### Integration Cleanup on Logout

Today, `manager.Logout()` cleans up identity credentials (keyring entries and identity-specific
files) but does not touch integration artifacts. This means logging out leaves stale kubeconfig
entries and docker sessions. Since login triggers integrations, logout should undo their effects.

**Updated logout flow for `manager.Logout(identityName)`:**

1. Find all integrations linked to the identity (`findIntegrationsForIdentity(name, false)`)
2. Create each integration instance via the registry
3. Call `Cleanup()` on each (non-fatal — log warnings, do not block logout)
4. Proceed with existing keyring deletion and `identity.Logout()` (unchanged)

The same pattern applies to `LogoutProvider()` (cleanup integrations for all identities using
that provider) and `LogoutAll()`.

**EKS `Cleanup()` implementation:**
- Remove the cluster, context, and user entries from the kubeconfig file (via `clientcmd`)
- If the kubeconfig file is empty after removal, delete the file
- Remove the Atmos kubeconfig path from `KUBECONFIG` if it was appended

**ECR `Cleanup()` implementation:**
- Call `docker.ConfigManager.RemoveAuth()` with the registry URL
- This method already exists at `pkg/auth/cloud/docker/config.go`

### Integration Environment Variables

Providers and identities declare environment variables via `Environment()` and
`PrepareEnvironment()` methods (see `pkg/auth/types/interfaces.go`). Integrations currently
do not participate in this system. For EKS, `KUBECONFIG` must be set; for ECR, `DOCKER_CONFIG`
may need to be set. This section specifies how integrations contribute to the env var system.

**Design decision:** The `Environment()` method on the `Integration` interface (see
[Integration Interface](#integration-interface)) returns env vars based on configuration
(deterministic from config), not from `Execute()` output. For EKS, the kubeconfig path is
known from the XDG default or `spec.cluster.kubeconfig.path`, so `Environment()` works without
calling `DescribeCluster`.

**Manager composition flow (updated `GetEnvironmentVariables` and `PrepareShellEnvironment`):**

1. Get identity env vars (existing behavior — calls `identity.Environment()`)
2. Find linked integrations for the identity (`findIntegrationsForIdentity(name, false)`)
3. Call `Environment()` on each integration instance
4. Merge integration env vars into identity env vars using composition rules

**Composition strategy:**

| Variable | Strategy | Rationale |
|----------|----------|-----------|
| `KUBECONFIG` | Colon-separated concatenation (idempotent, deduplicated) | Standard kubectl behavior for multiple kubeconfig files |
| All others | Last integration in config order wins | Simple default; avoids overcomplicating for single-value vars |

The manager maintains a small set of known composable variables and their delimiters. This set
can be extended as new integration types are added.

**EKS `Environment()` implementation:**

Returns `{"KUBECONFIG": "<path>"}` where `<path>` is the XDG default
(`~/.config/atmos/kube/config`) or the custom path from `spec.cluster.kubeconfig.path`.

**ECR `Environment()` implementation:**

Returns `{"DOCKER_CONFIG": "<docker-config-dir>"}`, matching what `cmd/auth_ecr_login.go`
already sets via `os.Setenv`.

**Multi-integration scenarios:**

| Scenario | Behavior |
|----------|----------|
| Blue/green clusters (`dev/eks-blue`, `dev/eks-green`) under same identity | Both write different contexts to the same kubeconfig file. Both return the same `KUBECONFIG` path. Manager deduplicates. |
| Mixed integrations (`dev/eks` + `dev/ecr`) under same identity | EKS returns `KUBECONFIG`, ECR returns `DOCKER_CONFIG`. No overlap — simple merge. |
| Multiple identities with EKS integrations | Each identity's env vars are composed independently when queried via `atmos auth env --identity <name>`. |

**Alternatives considered:**

- **Side-effect during Execute():** Integrations call `os.Setenv()` during `Execute()`. Simpler,
  and ECR already does this in `cmd/auth_ecr_login.go`. However, this is not declarative, not
  composable (last write wins), and breaks `atmos auth env` which needs env vars without executing
  integrations.

- **File-based contract:** Integrations write to well-known XDG paths; identity's `Environment()`
  discovers and includes them. Simpler, but creates tight coupling between identity and integration
  implementations.

## Implementation Details

### Package Structure

```
pkg/auth/
  cloud/
    aws/
      config.go           # NEW: Extract buildAWSConfigFromCreds from ecr.go (shared)
      ecr.go              # MODIFY: Remove buildAWSConfigFromCreds (moved to config.go)
      eks.go              # NEW: EKS SDK wrapper (DescribeCluster, GetToken)
      eks_test.go         # NEW: Unit tests
    kube/
      config.go           # NEW: Kubeconfig manager
      config_test.go      # NEW: Unit tests
  integrations/
    types.go              # EXISTS: KindAWSEKS = "aws/eks" already defined (remove "Future" comment)
    aws/
      ecr.go              # EXISTS: ECR integration (reference implementation)
      eks.go              # NEW: EKS integration
      eks_test.go         # NEW: Unit tests

cmd/
  auth_eks_token.go         # NEW: `atmos auth eks-token` exec credential plugin subcommand
  auth_eks_token_test.go    # NEW: Unit tests (follows existing auth subcommand pattern)

cmd/aws/eks/
  update_kubeconfig.go       # MODIFY: Use Go SDK, add --integration flag
  update_kubeconfig_test.go  # MODIFY: Update tests

internal/exec/
  aws_eks_update_kubeconfig.go  # MODIFY: Use Go SDK instead of shelling out
```

### Core Components

#### 1. EKS SDK Wrapper (`pkg/auth/cloud/aws/eks.go`)

- `DescribeCluster()` - Get cluster endpoint and CA certificate
- `GetToken()` - Generate EKS bearer token via STS pre-signed URL (for `atmos auth eks-token`)
- `EKSClusterInfo` struct - Cluster data needed for kubeconfig

#### 2. Kubeconfig Manager (`pkg/auth/cloud/kube/config.go`)

- `NewKubeconfigManager(customPath)` - Create manager with XDG default or custom path
- `WriteClusterConfig(info, alias, update)` - Generate and write kubeconfig
- Merge via `k8s.io/client-go/tools/clientcmd` (`ClientConfigLoadingRules` with precedence) instead of custom merge logic — `k8s.io/client-go` is already an indirect dependency

#### 3. EKS Token Command (`cmd/auth_eks_token.go`)

- Implements `atmos auth eks-token` as a subcommand of `authCmd`
- Follows the existing auth subcommand pattern (see `cmd/auth_ecr_login.go` for reference) — added via `authCmd.AddCommand()`, not `CommandProvider` (which is for top-level commands only; no auth subcommand uses it)
- Outputs `ExecCredential` JSON (`client.authentication.k8s.io/v1beta1`)
- Generates EKS bearer token using STS pre-signed `GetCallerIdentity` URL
- Accepts `--identity`, `--cluster-name`, `--region` flags
- Uses Atmos-managed AWS credentials (no AWS CLI dependency)

#### 4. EKS Integration (`pkg/auth/integrations/aws/eks.go`)

- Implements `Integration` interface (all four methods)
- `Execute()` - DescribeCluster + write kubeconfig
- `Cleanup()` - Remove cluster/context/user entries from kubeconfig file
- `Environment()` - Return `{"KUBECONFIG": "<path>"}` (deterministic from config)
- Validates configuration during construction

#### 5. Enhanced CLI Command (`cmd/aws/eks/update_kubeconfig.go`)

- Existing component/stack mode (backward compatible)
- New integration mode via `--integration` flag
- Explicit cluster mode via `--name` flag
- Uses Go SDK instead of shelling to AWS CLI
- Leverages Atmos-managed credentials when available

## Testing Strategy

### Unit Tests

**EKS SDK Wrapper (`pkg/auth/cloud/aws/eks_test.go`):**
- Mock EKS client using `go.uber.org/mock/mockgen`
- Test successful cluster description
- Test cluster not found error
- Test access denied error
- Test invalid region error

**Kubeconfig Manager (`pkg/auth/cloud/kube/config_test.go`):**
- Test kubeconfig YAML generation
- Test XDG path resolution
- Test clientcmd merge with empty existing config
- Test clientcmd merge with existing clusters (update)
- Test clientcmd merge with existing clusters (add new)
- Test file permissions (0600 default, custom mode via octal string)
- Test directory creation
- Test Mode parsing (valid octal, invalid string, empty defaults to 0600)

**EKS Token Command (`cmd/auth_eks_token_test.go`):**
- Test ExecCredential JSON output format
- Test token generation with mock STS client
- Test missing credentials error
- Test flag parsing (--cluster-name, --region)

**EKS Integration (`pkg/auth/integrations/aws/eks_test.go`):**
- Test valid configuration
- Test missing cluster config
- Test missing via.identity
- Test Execute with mock credentials
- Test Cleanup removes cluster/context/user entries from kubeconfig
- Test Cleanup with non-existent kubeconfig file (idempotent)
- Test Environment returns correct KUBECONFIG path (XDG default and custom)

**Integration Logout (`pkg/auth/manager_logout_test.go`):**
- Test Logout calls Cleanup on linked integrations
- Test Cleanup errors are non-fatal (logout still succeeds)
- Test LogoutProvider cleans up integrations for all affected identities

**Integration Environment Composition (`pkg/auth/manager_environment_test.go`):**
- Test GetEnvironmentVariables includes integration env vars
- Test KUBECONFIG colon-separated composition with deduplication
- Test multiple integrations under one identity (EKS + ECR)
- Test blue/green clusters produce deduplicated KUBECONFIG path

**CLI Command (`cmd/aws/eks/update_kubeconfig_test.go`):**
- Use `cmd.NewTestKit(t)` for command isolation
- Test backward compatibility with component/stack mode
- Test new `--integration` flag
- Test flag parsing and argument validation
- Test help output

### Integration Tests

- End-to-end test with LocalStack or mock server
- Verify generated kubeconfig is valid
- Test kubectl can parse the output

### Coverage Target

- Minimum 80% coverage (CodeCov enforced)
- Focus on error paths and edge cases

## Security Considerations

1. **Credential isolation**: Kubeconfig uses exec plugin, so no long-lived tokens stored
2. **File permissions**: Kubeconfig written with 0600 permissions
3. **Directory permissions**: XDG kube directory created with 0700
4. **Token security**: EKS tokens are short-lived (~15 minutes)
5. **Credential source**: Exec plugin resolves credentials deterministically via explicit `--identity` flag and `ATMOS_IDENTITY` env var (embedded at generation time), following the credential resolution order documented in the Exec Credential Plugin section
6. **No secrets in kubeconfig**: Only cluster endpoint and CA cert stored; auth is via exec

## Configuration Examples

### Single Cluster Setup

```yaml
auth:
  identities:
    dev-admin:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: DevAccess
        account: { id: "123456789012" }

  integrations:
    dev/eks:
      kind: aws/eks
      via:
        identity: dev-admin
      spec:
        auto_provision: true
        cluster:
          name: dev-cluster
          region: us-east-2
          alias: dev
```

### Multi-Cluster Setup

```yaml
auth:
  identities:
    platform-admin:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: PlatformAdmin
        account: { id: "123456789012" }

  integrations:
    dev/eks:
      kind: aws/eks
      via:
        identity: platform-admin
      spec:
        auto_provision: true
        cluster:
          name: dev-cluster
          region: us-east-2
          alias: dev

    staging/eks:
      kind: aws/eks
      via:
        identity: platform-admin
      spec:
        auto_provision: true
        cluster:
          name: staging-cluster
          region: us-east-1
          alias: staging

    prod/eks:
      kind: aws/eks
      via:
        identity: platform-admin
      spec:
        auto_provision: false  # Manual only for prod
        cluster:
          name: prod-cluster
          region: us-west-2
          alias: prod
```

### Custom Kubeconfig Path

```yaml
auth:
  integrations:
    dev/eks:
      kind: aws/eks
      via:
        identity: dev-admin
      spec:
        cluster:
          name: dev-cluster
          region: us-east-2
          kubeconfig:
            path: /home/user/.kube/atmos-config
            mode: "0600"
            update: replace  # merge | replace | error
```

### Terraform Kubernetes Provider

Once Atmos provisions the kubeconfig, Terraform components that use the `kubernetes`, `helm`, or `kubectl` providers can authenticate to EKS without any additional configuration. The generated kubeconfig contains an exec credential plugin spec with `command: atmos` and `args: [auth, eks-token, ...]` (see [Output Format](#output-format)). When the Terraform provider reads this kubeconfig, it invokes `atmos auth eks-token` on demand to obtain short-lived tokens — the same mechanism kubectl uses. This means token refresh is handled automatically, even during long Terraform runs, as long as `atmos` is on PATH and the underlying AWS credentials are valid.

**Provider configuration using kubeconfig (recommended):**

```hcl
# The Kubernetes provider reads KUBECONFIG automatically.
# Since atmos auth login appends the Atmos-managed kubeconfig path
# to the KUBECONFIG environment variable, this works out of the box.
provider "kubernetes" {
  config_path    = "~/.config/atmos/kube/config"
  config_context = "dev-eks"
}

provider "helm" {
  kubernetes {
    config_path    = "~/.config/atmos/kube/config"
    config_context = "dev-eks"
  }
}
```

**Provider configuration using exec (alternative):**

Components can also call `atmos auth eks-token` directly, bypassing kubeconfig entirely. This is useful when the component needs to target a specific cluster without relying on kubeconfig context:

```hcl
provider "kubernetes" {
  host                   = data.aws_eks_cluster.cluster.endpoint
  cluster_ca_certificate = base64decode(data.aws_eks_cluster.cluster.certificate_authority[0].data)

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "atmos"
    args        = ["auth", "eks-token", "--cluster-name", var.cluster_name, "--region", var.region]
  }
}
```

**Atmos component configuration:**

To wire the kubeconfig context into a Terraform component, reference the integration's alias in the stack config:

```yaml
components:
  terraform:
    my-k8s-app:
      vars:
        eks_context: dev-eks  # Matches the alias from the integration spec
```

This pattern means `atmos auth login dev-admin` authenticates to AWS *and* configures kubectl *and* enables Terraform Kubernetes provider access — all in one step.

## Success Metrics

1. **Functionality**: EKS kubeconfig generated correctly with valid YAML structure
2. **Authentication**: kubectl can authenticate to EKS using generated kubeconfig
3. **Auto-provision**: Kubeconfig updated automatically on `atmos auth login`
4. **Merge behavior**: Existing kubeconfig entries preserved when merging
5. **Error handling**: Clear error messages for common failures (cluster not found, access denied)
6. **Performance**: Kubeconfig generation completes in <5 seconds
7. **Test coverage**: >80% code coverage on new code
8. **Non-blocking**: Auth login succeeds even if EKS integration fails

## Dependencies

- **ECR integration** (merged via PR #1859): Provides integration infrastructure (`pkg/auth/integrations/`), including `Integration` interface, `IntegrationFactory`, registry pattern, and `KindAWSEKS` constant (already defined in `types.go`)
- **Helmfile EKS modernization** (merged via PR #1903): Refactors `atmos aws eks` into command registry/flag handler pattern; must be merged before implementing this PRD (already in main)
- **AWS SDK v2 EKS**: `github.com/aws/aws-sdk-go-v2/service/eks` (new dependency, not yet in go.mod)
- **k8s.io/client-go**: Currently an indirect dependency; must be promoted to direct in go.mod for `clientcmd` imports used by the kubeconfig merge logic
- **XDG package**: `pkg/xdg/` for path resolution (already available)

## Future Enhancements

1. **Token caching**: Cache EKS tokens to reduce API calls
2. **Namespace support**: Set default namespace in kubeconfig context
3. **Role ARN in exec plugin**: Embed `--role-arn` in the kubeconfig exec plugin args so kubectl token refresh assumes a role at runtime. This is distinct from the existing `--role-arn` flag on `atmos aws eks update-kubeconfig`, which applies role assumption at kubeconfig generation time only
4. **CI/CD workflow**: Define how `atmos auth eks-token` works in CI environments where AWS credentials come from OIDC/instance profiles rather than `atmos auth login`

## References

- [Linear Ticket DEV-3815](https://linear.app/cloudposse/issue/DEV-3815)
- [ECR Authentication PR #1859](https://github.com/cloudposse/atmos/pull/1859)
- [AWS EKS update-kubeconfig Documentation](https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html)
- [Kubernetes client-go exec credentials](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#client-go-credential-plugins)
- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-17 | AI Assistant | Initial PRD |
| 1.1 | 2025-12-18 | AI Assistant | Updated based on review: CLI uses `atmos aws eks` namespace, improved kubeconfig schema structure, clarified exec plugin format |
| 1.2 | 2026-02-17 | AI Assistant | Synced PRD with codebase after rebase: fixed Integration interface, corrected architecture diagram, updated file paths, noted `buildAWSConfigFromCreds` reuse, updated dependency status |
| 1.3 | 2026-02-17 | AI Assistant | Changed exec credential plugin from `aws` to `atmos auth eks-token` (eliminates AWS CLI dependency), added `GetToken()` to package structure, simplified XDG path section, moved "Atmos as exec plugin" from Future Enhancements to core design |
| 1.4 | 2026-02-18 | AI Assistant | Added identity flag and `interactiveMode: Never` to exec plugin spec, specified KUBECONFIG colon-separated append semantics (idempotent), fixed command path to `cmd/auth_eks_token.go` (follows existing auth subcommand pattern), specified `KubeconfigSettings.Mode` octal parsing with `strconv.ParseUint`, replaced custom `MergeKubeconfig` with `k8s.io/client-go/tools/clientcmd` merge |
| 1.5 | 2026-02-18 | AI Assistant | Synced with codebase review: updated KubeconfigSettings.Update validation, added k8s.io/client-go and PR #1903 to Dependencies, fixed `atmos auth env` format |
| 1.6 | 2026-03-03 | AI Assistant | Rewrote executive summary and problem statement, added Terraform Kubernetes provider section |
| 1.7 | 2026-03-03 | AI Assistant | Added `--identity`/`--profile` flags to Desired Workflow, added `Cleanup()` and `Environment()` to Integration interface for logout cleanup and env var composition, added Integration Cleanup on Logout and Integration Environment Variables sections, updated CLI command flags with env var bindings and flag disambiguation, replaced kubeconfig cleanup future enhancement with CI/CD workflow |
