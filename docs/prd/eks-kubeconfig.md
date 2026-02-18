# EKS Kubeconfig Authentication Integration PRD

## Executive Summary

This document defines the design for integrating EKS kubeconfig generation into Atmos's authentication system. The integration enables automatic kubeconfig setup when users authenticate with AWS identities, eliminating manual `aws eks update-kubeconfig` invocations and providing seamless Kubernetes cluster access via Atmos-managed credentials.

**Key Design Decision:** EKS kubeconfig is an **integration** (not an identity) because it represents client-side credential materialization derived from an existing AWS identity—the kubeconfig itself is not an identity, but a credential file enabling `kubectl` to authenticate against EKS clusters.

## Problem Statement

### Background

Atmos introduced an authentication system (`atmos auth`) that manages AWS credentials through SSO, SAML, and other identity providers. Users can authenticate with a single command (`atmos auth login <identity>`) and have their AWS credentials automatically configured. However, EKS cluster access requires additional manual steps.

### Current Challenges

1. **Manual kubeconfig setup**: After authenticating with Atmos, users must manually run `aws eks update-kubeconfig --name <cluster> --region <region>` for each EKS cluster they need to access.

2. **No integration with Atmos credentials**: The `aws eks update-kubeconfig` command uses ambient AWS credentials, which may not match the Atmos-managed identity the user authenticated with.

3. **Repetitive workflow**: Users working with multiple clusters must repeat the kubeconfig update process for each cluster, often multiple times per day as credentials expire.

4. **Token refresh complexity**: EKS exec credential plugins have short token lifetimes (~15 minutes), and users must ensure their AWS credentials are valid when kubectl makes API calls.

5. **Inconsistent credential sources**: The existing `atmos aws eks update-kubeconfig` command shells out to the AWS CLI, creating dependency on CLI installation and ambient credentials.

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
                                      │ STS GetCallerID  │
                                      │ → Bearer token   │
                                      └──────────────────┘
```

### Integration Interface

EKS implements the same `Integration` interface as ECR:

```go
// Integration interface (defined in pkg/auth/integrations/types.go).
type Integration interface {
    // Kind returns the integration type (e.g., "aws/ecr", "aws/eks").
    Kind() string

    // Execute performs the integration using the provided AWS credentials.
    // Returns nil on success, error on failure.
    Execute(ctx context.Context, creds types.ICredentials) error
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
    // Parsed via strconv.ParseUint(mode, 8, 32) at config-load time.
    // Invalid values (e.g., "abc", "999") are rejected with a validation error
    // referencing KubeconfigSettings.Mode.
    Mode string `yaml:"mode,omitempty" json:"mode,omitempty" mapstructure:"mode"`

    // Update determines how to handle existing kubeconfig: "merge" (default), "replace", or "error".
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

The EKS integration appends the Atmos kubeconfig path to the existing `KUBECONFIG` environment
variable using colon-separated notation (standard kubectl behavior). The append is **idempotent** —
if the Atmos path is already present, it will not be duplicated.

```bash
# Before: KUBECONFIG=~/.kube/config
# After:  KUBECONFIG=~/.kube/config:~/.config/atmos/kube/config
```

Users can export all auth environment variables (including the updated `KUBECONFIG`) at once:

```bash
eval $(atmos auth env --format=export)
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

Flags:
  -s, --stack string         Stack name (with component)
      --integration string   Named integration from auth.integrations
      --name string          EKS cluster name (explicit mode)
      --region string        AWS region
      --profile string       AWS profile
      --role-arn string      IAM role ARN to assume
      --alias string         Context alias in kubeconfig
      --kubeconfig string    Custom kubeconfig path
      --dry-run              Print kubeconfig to stdout instead of writing
      --verbose              Print detailed output

Examples:
  # Using a component (existing behavior, now with Go SDK)
  atmos aws eks update-kubeconfig eks-cluster -s dev-use2

  # Using a named integration
  atmos aws eks update-kubeconfig --integration dev/eks

  # Explicit cluster
  atmos aws eks update-kubeconfig --name dev-cluster --region us-east-2 --alias dev
```

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
- Follows the existing auth subcommand pattern (see `cmd/auth_ecr_login.go` for reference)
- Outputs `ExecCredential` JSON (`client.authentication.k8s.io/v1beta1`)
- Generates EKS bearer token using STS pre-signed `GetCallerIdentity` URL
- Accepts `--identity`, `--cluster-name`, `--region` flags
- Uses Atmos-managed AWS credentials (no AWS CLI dependency)

#### 4. EKS Integration (`pkg/auth/integrations/aws/eks.go`)

- Implements `Integration` interface
- `Execute()` - DescribeCluster + write kubeconfig
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
5. **Credential source**: Exec plugin uses ambient AWS credentials from Atmos-managed files
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
- **AWS SDK v2 EKS**: `github.com/aws/aws-sdk-go-v2/service/eks` (new dependency, not yet in go.mod)
- **XDG package**: `pkg/xdg/` for path resolution (already available)

## Future Enhancements

1. **Token caching**: Cache EKS tokens to reduce API calls
2. **Namespace support**: Set default namespace in kubeconfig context
3. **Role ARN support**: Configure assume-role for kubectl authentication
4. **Kubeconfig cleanup**: Command to remove stale cluster entries

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
