## Next Steps

Your Atmos Pro workflows, auth profile, and stack configuration have been created.

### 1. Configure the Auth Profile

Edit `profiles/github/atmos.yaml` and replace the placeholder values:
- `<region>` with your AWS region (e.g., `us-east-2`)
- `<role-arn>` with the IAM role ARN for Terraform operations

See: https://atmos-pro.com/docs/howto/aws

### 2. Set Up OIDC Authentication

Deploy a GitHub OIDC provider in your AWS account to enable keyless
authentication from GitHub Actions.

### 3. Create IAM Roles

Create IAM roles with trust policies that allow your GitHub repository
to assume them via OIDC. Scope roles to specific branches or GitHub
Environments for production safety.

### 4. Create Your Workspace

Visit https://app.atmos.tools and create a workspace for your organization.

### 5. Add Your Repository

Connect this repository to your Atmos Pro workspace to enable
automated plan/apply orchestration.

### 6. Set ATMOS_VERSION Repository Variable

Add a repository variable `ATMOS_VERSION` in GitHub Settings → Variables
with the desired Atmos version (e.g., `latest` or a specific version).
