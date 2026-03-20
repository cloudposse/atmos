# ECR Public Authentication PRD

## Executive Summary

Add ECR Public authentication support to Atmos via a new **`aws/ecr-public`** integration kind. This enables authenticated pulls from `public.ecr.aws`, eliminating rate limits that affect CI workflows.

**Companion to:** [`ecr-authentication.md`](./ecr-authentication.md) (private ECR).

## Problem Statement

Docker image pulls from `public.ecr.aws` are rate-limited when unauthenticated. The `cloudposse/github-action-docker-build-push` action pulls BuildKit and binfmt images from public ECR, so every Docker build hits these limits. Currently there is no native Atmos way to authenticate to ECR Public — users must add manual `docker/login-action` steps to their workflows.

### User Impact

**Current Experience:**
```bash
# Must manually authenticate to ECR Public
$ aws ecr-public get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin public.ecr.aws

# Or add docker/login-action steps to every CI workflow
```

**Desired Experience:**
```bash
# Automatic via integration auto_provision
$ atmos auth login dev-admin
✓ Authenticated as arn:aws:sts::123456789012:assumed-role/DevRole/user
✓ ECR Public login: public.ecr.aws (expires in 12h)

# Or explicit
$ atmos auth ecr-login ecr-public
✓ ECR Public login: public.ecr.aws (expires in 12h)
```

## How ECR Public Differs from Private ECR

| Aspect | Private ECR (`aws/ecr`) | Public ECR (`aws/ecr-public`) |
|--------|------------------------|-------------------------------|
| AWS SDK service | `ecr` | `ecrpublic` |
| API call | `ecr:GetAuthorizationToken` | `ecr-public:GetAuthorizationToken` |
| Auth mechanism | SigV4 | Bearer token (`sts:GetServiceBearerToken`) |
| Auth region | Any region where registry exists | **us-east-1 only** |
| Service regions | All commercial AWS regions | us-east-1, us-west-2 only |
| Registry URL | `{account_id}.dkr.ecr.{region}.amazonaws.com` | `public.ecr.aws` (always) |
| IAM permissions | `ecr:GetAuthorizationToken` | `ecr-public:GetAuthorizationToken` + `sts:GetServiceBearerToken` |
| Config needs | `account_id` + `region` required | No config needed (fixed endpoint) |
| China/GovCloud | Private ECR available | **Not available** |

### Regional Availability

ECR Public is only available in two regions:

| Region | Service endpoints | Auth (`GetAuthorizationToken`) |
|--------|------------------|-------------------------------|
| us-east-1 (N. Virginia) | Yes | **Yes (only region)** |
| us-west-2 (Oregon) | Yes | No |

**Not available in:** EU, Asia Pacific, China (cn-north-1, cn-northwest-1), GovCloud, or any other region.

**Source:** [AWS ECR Public endpoints and quotas](https://docs.aws.amazon.com/general/latest/gr/ecr-public.html).

### Region Validation

The implementation must validate any user-specified region against the supported set `{us-east-1, us-west-2}`. Auth calls are always forced to us-east-1 regardless of any user configuration.

## Configuration Schema

### Minimal Configuration (Recommended)

```yaml
auth:
  integrations:
    ecr-public:
      kind: aws/ecr-public
      via:
        identity: plat-dev/terraform
      spec:
        auto_provision: true
```

No `registry` block is needed since ECR Public is always `public.ecr.aws` in `us-east-1`.

### Configuration Options

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `kind` | Yes | — | Must be `aws/ecr-public` |
| `via.identity` | No | — | Identity providing AWS credentials |
| `spec.auto_provision` | No | `true` | Auto-trigger on identity login |

## Technical Specification

### New Integration Kind: `aws/ecr-public`

Registered via the existing integration registry pattern (same as `aws/ecr`).

### Authentication Flow

1. Build AWS config with credentials from the linked identity.
2. Force region to `us-east-1` (the only supported auth region).
3. Call `ecrpublic.GetAuthorizationToken()` via AWS SDK v2.
4. Decode base64 authorization token to `username:password`.
5. Write credentials to Docker config for `public.ecr.aws`.
6. Log success with token expiration time.

### Package Structure (New Files)

```text
pkg/auth/
├── cloud/aws/
│   ├── ecr.go              # Existing private ECR
│   ├── ecr_public.go       # NEW: ECR Public token fetcher
│   └── ecr_public_test.go  # NEW: Tests
├── integrations/
│   ├── types.go            # Add KindAWSECRPublic constant
│   └── aws/
│       ├── ecr.go          # Existing private ECR integration
│       ├── ecr_public.go   # NEW: ECR Public integration
│       └── ecr_public_test.go  # NEW: Tests
```

### Error Handling

New sentinel errors in `errors/errors.go`:
- `ErrECRPublicAuthFailed` — "ECR Public authentication failed"
- `ErrECRPublicInvalidRegion` — "invalid ECR Public region"

Integration failures during auto-provision are non-fatal (logged as warnings, don't block identity auth). Explicit `ecr-login` command failures are fatal.

### CLI Integration

No new CLI command needed. The existing `atmos auth ecr-login` command routes through the integration registry and handles `aws/ecr-public` automatically:

```bash
# Named integration
atmos auth ecr-login ecr-public

# Via identity (triggers all auto_provision integrations)
atmos auth ecr-login --identity plat-dev/terraform
```

## Implementation Checklist

- [ ] PRD document (`docs/prd/ecr-public-authentication.md`)
- [ ] SDK dependency (`github.com/aws/aws-sdk-go-v2/service/ecrpublic`)
- [ ] Error sentinels (`errors/errors.go`)
- [ ] Kind constant (`pkg/auth/integrations/types.go`)
- [ ] Cloud layer (`pkg/auth/cloud/aws/ecr_public.go`)
- [ ] Cloud layer tests (`pkg/auth/cloud/aws/ecr_public_test.go`)
- [ ] Integration (`pkg/auth/integrations/aws/ecr_public.go`)
- [ ] Integration tests (`pkg/auth/integrations/aws/ecr_public_test.go`)

## Security Considerations

1. **Token lifetime:** ECR Public tokens expire after 12 hours (AWS-enforced).
2. **Docker config:** Credentials written to standard Docker config location with `0600` permissions.
3. **No secrets in logs:** Authorization tokens are never logged.
4. **Secret masking:** ECR Public tokens follow Atmos secret masking patterns via Gitleaks integration.
5. **Region pinning:** Auth is always pinned to us-east-1, preventing misconfiguration.

## References

- [Amazon ECR Public endpoints and quotas](https://docs.aws.amazon.com/general/latest/gr/ecr-public.html)
- [ECR Public registry authentication](https://docs.aws.amazon.com/AmazonECR/latest/public/public-registry-auth.html)
- [ECR Public GetAuthorizationToken API](https://docs.aws.amazon.com/AmazonECRPublic/latest/APIReference/API_GetAuthorizationToken.html)
- [aws ecr-public is region specific — Issue #5917](https://github.com/aws/aws-cli/issues/5917)
- [Companion PRD: ECR Authentication](./ecr-authentication.md)
