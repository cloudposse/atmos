# Atmos Auth Profiles

This directory contains [Atmos auth profiles](https://atmos.tools/cli/auth/) that configure
authentication for CI/CD workflows. Profiles are activated by setting the `ATMOS_PROFILE`
environment variable in GitHub Actions workflows.

## Profiles

| Profile | Purpose | IAM Role |
|---------|---------|----------|
| `github-plan` | Read-only access for `terraform plan` | `*-planner` roles |
| `github-apply` | Full access for `terraform apply` | `*-terraform` roles |

## How It Works

Each profile defines:

- **Auth provider** (`github-oidc`): Configures GitHub's OIDC token exchange with AWS STS
- **Identities**: Maps each `<tenant>-<stage>/terraform` identity to an IAM role ARN in the
  corresponding AWS account

When `ATMOS_PROFILE` is set, Atmos deep-merges the profile's configuration on top of the
base configuration. This overrides the auth settings so that identity references in stacks
resolve to the profile's identity definitions. This is how the same stack can authenticate
differently depending on context — a CI/CD workflow uses a GitHub OIDC profile while a
developer running locally uses their SSO credentials.

## Adding a New Account

Add an identity entry to both profiles:

```yaml
# profiles/github-plan/atmos.yaml
identities:
  <tenant>-<stage>/terraform:
    kind: aws/assume-role
    via:
      provider: github-oidc
    principal:
      assume_role: arn:aws:iam::<account-id>:role/<namespace>-<tenant>-gbl-<stage>-planner
```

```yaml
# profiles/github-apply/atmos.yaml
identities:
  <tenant>-<stage>/terraform:
    kind: aws/assume-role
    via:
      provider: github-oidc
    principal:
      assume_role: arn:aws:iam::<account-id>:role/<namespace>-<tenant>-gbl-<stage>-terraform
```

## References

- [Atmos Auth Profiles](https://atmos.tools/cli/auth/)
- [GitHub OIDC with AWS](https://docs.github.com/en/actions/security-for-github-actions/security-hardening-your-deployments/configuring-openid-connect-in-amazon-web-services)
