# PRD: Ambient Identity Support (IRSA / IMDS / ECS Task Roles)

## Executive Summary

Add two new authentication identity kinds to Atmos: a cloud-agnostic `ambient` passthrough and an AWS-specific `aws/ambient` that resolves credentials from the AWS SDK's default credential provider chain. This enables Atmos to run natively on EKS pods (IRSA), EC2 instances (instance profiles), and ECS tasks (task roles) without additional auth configuration.

## Problem Statement

### Background

Atmos auth manages credentials for cloud providers by writing isolated credential files, setting AWS_PROFILE, and configuring the environment for subprocesses (Terraform, Helmfile, etc.). To prevent accidental credential leakage, every AWS identity's `PrepareEnvironment()` method calls `awsCloud.PrepareEnvironment()` which:

1. **Clears credential environment variables**: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_SECURITY_TOKEN`, `AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_ARN`, `AWS_ROLE_SESSION_NAME`
2. **Disables IMDS fallback**: Sets `AWS_EC2_METADATA_DISABLED=true`
3. **Overrides credential file paths**: Sets `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE`, `AWS_PROFILE` to Atmos-managed paths

This design is correct for SSO, SAML, OIDC, and user credential flows where Atmos actively manages the authentication lifecycle. However, it completely blocks infrastructure-provided credentials:

- **IRSA (IAM Roles for Service Accounts)**: Kubernetes injects `AWS_WEB_IDENTITY_TOKEN_FILE` and `AWS_ROLE_ARN` into pod environments. Atmos clears both.
- **EC2 Instance Profiles / IMDS**: The AWS SDK falls back to IMDS for credentials. Atmos sets `AWS_EC2_METADATA_DISABLED=true`.
- **ECS Task Roles**: Similar to IMDS, resolved via the container credentials endpoint. Disabled by the same IMDS flag.

### User Impact

Teams running Atmos in:
- **EKS-based CI/CD**: Pods with IRSA cannot use their service account credentials for Terraform.
- **EC2-hosted automation**: Instances with IAM roles attached cannot use those roles through Atmos.
- **ECS task-based workflows**: Task role credentials are blocked.

These teams must either bypass Atmos auth entirely (losing identity management, audit logging, and credential isolation) or maintain parallel credential management outside of Atmos.

### Current Workaround

The only workaround is to not configure auth at all, which means losing all benefits of Atmos auth (identity selection, chaining, integrations like ECR/EKS, audit trail).

## Design Goals

1. **Passthrough semantics**: The `ambient` identity must not modify, clear, or override any environment variables.
2. **AWS SDK default chain**: The `aws/ambient` identity must resolve credentials through the full AWS SDK credential provider chain (environment variables → shared config → IRSA web identity → IMDS → ECS container credentials).
3. **Chaining support**: `aws/ambient` must return real `AWSCredentials` so chained identities like `aws/assume-role` can use them as base credentials for cross-account role assumption.
4. **Cloud-agnostic base**: The generic `ambient` identity works for any cloud provider by simply passing the environment through.
5. **Standalone operation**: Neither identity requires a provider or `via` configuration.
6. **Zero config for simple cases**: Just `kind: ambient` or `kind: aws/ambient` with no additional fields required.
7. **No credential storage**: Ambient identities do not write to keyring or credential files. Credentials are resolved fresh at runtime.

## Technical Specification

### Two-Tier Architecture

| | `ambient` | `aws/ambient` |
|---|---|---|
| **Cloud** | Agnostic | AWS-specific |
| **Authenticate()** | Returns `nil, nil` | Resolves via AWS SDK default chain |
| **PrepareEnvironment()** | Pure passthrough | Passthrough + optional region |
| **Chaining** | Cannot chain (no credentials) | Supports chaining via `aws/assume-role` |
| **Use case** | Simple env passthrough | IRSA, IMDS, ECS task roles |

### Identity Interface Implementation

Both identities implement `types.Identity` (defined in `pkg/auth/types/interfaces.go`).

#### Generic `ambient` Identity

```go
// Package: pkg/auth/identities/ambient/

type ambientIdentity struct {
    name   string
    config *schema.Identity
}

func (i *ambientIdentity) Kind() string                    { return "ambient" }
func (i *ambientIdentity) GetProviderName() (string, error) { return "ambient", nil }
func (i *ambientIdentity) Authenticate(ctx, baseCreds)      { return nil, nil }
func (i *ambientIdentity) PrepareEnvironment(ctx, environ)  { return copy(environ), nil }
```

Key behavior: `PrepareEnvironment()` creates a defensive copy of the environment map and returns it unchanged. No credential clearing, no IMDS disabling, no file path overrides.

#### AWS `aws/ambient` Identity

```go
// Package: pkg/auth/identities/aws/

type awsAmbientIdentity struct {
    name   string
    config *schema.Identity
    realm  string
}
```

##### Authenticate()

Uses `config.LoadDefaultConfig(ctx)` **directly** — NOT through `awsCloud.LoadIsolatedAWSConfig()` or `awsCloud.WithIsolatedAWSEnv()`. This is critical because those helpers clear environment variables and disable shared config loading, which would defeat the purpose of ambient credentials.

```go
func (i *awsAmbientIdentity) Authenticate(ctx context.Context, _ types.ICredentials) (types.ICredentials, error) {
    cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
    creds, err := cfg.Credentials.Retrieve(ctx)
    return &types.AWSCredentials{
        AccessKeyID:     creds.AccessKeyID,
        SecretAccessKey: creds.SecretAccessKey,
        SessionToken:    creds.SessionToken,
        Region:          region,
    }, nil
}
```

The AWS SDK's default credential chain resolves in this order:
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
2. Shared config files (`~/.aws/config`, `~/.aws/credentials`)
3. Web identity token (IRSA: `AWS_WEB_IDENTITY_TOKEN_FILE` + `AWS_ROLE_ARN`)
4. EC2 Instance Metadata Service (IMDS)
5. ECS container credentials endpoint

##### PrepareEnvironment()

Does NOT call `awsCloud.PrepareEnvironment()`. This is the critical divergence from all other AWS identities:

```go
func (i *awsAmbientIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
    result := copy(environ)
    if region != "" {
        result["AWS_REGION"] = region
        result["AWS_DEFAULT_REGION"] = region
    }
    return result, nil
}
```

What it does NOT do (that every other AWS identity does):
- Does NOT delete `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`
- Does NOT delete `AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_ARN`, `AWS_ROLE_SESSION_NAME`
- Does NOT set `AWS_EC2_METADATA_DISABLED=true`
- Does NOT set `AWS_SHARED_CREDENTIALS_FILE` or `AWS_CONFIG_FILE`
- Does NOT set `AWS_PROFILE`

### Chain Integration

#### Standalone Detection

Both identity kinds are standalone (no `via` required), like `aws/user`. This requires changes in two places:

**`buildChainRecursive()`** — Allow `ambient` and `aws/ambient` with nil `Via`:
```go
if identity.Kind == "aws/user" || identity.Kind == "aws/ambient" || identity.Kind == "ambient" {
    *chain = append(*chain, identityName)
    return nil
}
```

**`authenticateFromIndex()`** — Add standalone authentication handlers:
```go
if aws.IsStandaloneAWSAmbientChain(m.chain, m.config.Identities) {
    return aws.AuthenticateStandaloneAWSAmbient(ctx, m.chain[0], m.identities)
}
if ambient.IsStandaloneAmbientChain(m.chain, m.config.Identities) {
    return ambient.AuthenticateStandaloneAmbient(ctx, m.chain[0], m.identities)
}
```

#### Chaining with assume-role

When `aws/ambient` is the base of a chain:
```yaml
identities:
  pod-base:
    kind: aws/ambient
  deployer:
    kind: aws/assume-role
    via:
      identity: pod-base
    principal:
      assume_role: "arn:aws:iam::999999999999:role/DeployRole"
```

The chain is: `[deployer, pod-base]`. Authentication flow:
1. `pod-base.Authenticate()` resolves IRSA/IMDS credentials → returns `AWSCredentials`
2. `deployer.Authenticate(ctx, podBaseCreds)` uses those credentials to call `sts:AssumeRole`
3. `deployer.PrepareEnvironment()` writes the assumed role credentials to files (normal behavior)

Note: When chained, the final identity in the chain (assume-role) handles `PrepareEnvironment()`, so the ambient identity's passthrough behavior only matters when it's standalone.

### Schema

No new schema fields are needed. The existing `Identity` struct supports:
- `kind`: `"ambient"` or `"aws/ambient"`
- `principal.region`: Optional region override (used by `aws/ambient`)
- `via`: Not required for ambient identities (standalone)

### Factory Registration

Both kinds are registered in `pkg/auth/factory/factory.go`:
```go
case "ambient":
    return ambientIdentities.NewAmbientIdentity(name, config)
case "aws/ambient":
    return awsIdentities.NewAWSAmbientIdentity(name, config)
```

## Security Considerations

### Trust Model

Ambient identities trust the execution environment to provide correct credentials. This is appropriate for:
- **EKS pods with IRSA**: Credentials are injected by the Kubernetes service account controller and scoped to the pod's service account.
- **EC2 instances with instance profiles**: Credentials are managed by the EC2 metadata service and scoped to the IAM role attached to the instance.
- **ECS tasks with task roles**: Credentials are provided by the ECS agent and scoped to the task's IAM role.

This is NOT appropriate for:
- **Developer laptops**: Where stale or wrong credentials might be in the environment, leading to accidental operations in the wrong account.
- **Shared CI runners without isolation**: Where one job's credentials might leak to another.

### Comparison with Explicit Auth

| Aspect | Explicit (SSO/OIDC) | Ambient |
|---|---|---|
| Credential source | Atmos-managed | Environment/IMDS |
| Credential isolation | Realm-scoped files | None (trusts environment) |
| IMDS protection | Disabled | Enabled (required) |
| Audit trail | Full | Limited to CloudTrail |
| Session management | Atmos-managed | Platform-managed |

### Recommendation

Use `aws/ambient` only in controlled environments where the credential source is trusted and well-scoped. For developer workflows, continue using SSO/OIDC identities.

## Use Cases

### 1. EKS Pod with IRSA (Standalone)
```yaml
auth:
  identities:
    eks-deployer:
      kind: aws/ambient
      principal:
        region: us-east-1
```

### 2. IRSA → Cross-Account Assume Role
```yaml
auth:
  identities:
    pod-base:
      kind: aws/ambient
      principal:
        region: us-east-1
    cross-account:
      kind: aws/assume-role
      via:
        identity: pod-base
      principal:
        assume_role: "arn:aws:iam::999999999999:role/TerraformDeployRole"
```

### 3. EC2 Instance Profile
```yaml
auth:
  identities:
    instance-creds:
      kind: aws/ambient
```

### 4. Generic Passthrough
```yaml
auth:
  identities:
    passthrough:
      kind: ambient
```

### 5. CI Runner with Pre-Configured Credentials
```yaml
auth:
  identities:
    ci:
      kind: aws/ambient
      principal:
        region: us-west-2
```

## Testing Strategy

### Unit Tests

- **Constructor validation**: Kind check, nil config handling.
- **PrepareEnvironment**: Verify no credential clearing, no IMDS disabling, optional region setting, input non-mutation.
- **IsStandaloneChain**: True for single-element chain with correct kind, false otherwise.
- **Kind/GetProviderName**: Return correct values.

### Integration Tests (Manual)

- Deploy Atmos to an EKS pod with IRSA and verify `aws/ambient` resolves credentials.
- Run on an EC2 instance with an instance profile and verify IMDS credentials work.
- Chain `aws/ambient` → `aws/assume-role` and verify cross-account access.

## Files Changed

### New Files
- `pkg/auth/identities/ambient/ambient.go` — Generic ambient identity implementation.
- `pkg/auth/identities/ambient/ambient_test.go` — Tests for generic ambient.
- `pkg/auth/identities/aws/ambient.go` — AWS ambient identity implementation.
- `pkg/auth/identities/aws/ambient_test.go` — Tests for AWS ambient.
- `examples/config-profiles/profiles/eks/auth.yaml` — EKS/IRSA example config.

### Modified Files
- `pkg/auth/factory/factory.go` — Register both identity kinds.
- `pkg/auth/manager_chain.go` — Standalone chain detection and authentication.
- `website/docs/stacks/auth.mdx` — Documentation for ambient identities.
- `website/src/data/roadmap.js` — Roadmap milestone.
- `website/blog/2026-03-25-ambient-credential-support.mdx` — Feature blog post.
