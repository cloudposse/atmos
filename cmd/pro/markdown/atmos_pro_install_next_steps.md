## Next Steps

Your Atmos Pro workflows, auth profiles, and stack configuration have been created.

### 1. Configure Auth Profiles

Edit the auth profiles and add one identity block per AWS account:

- `profiles/github-plan/atmos.yaml` — planner (read-only) roles for `terraform plan`
- `profiles/github-apply/atmos.yaml` — terraform (admin) roles for `terraform apply`

Replace the placeholder values (`<region>`, `<account-id>`, `<role-name>`) with your
AWS account details. See `profiles/README.md` for examples.

### 2. Set Up OIDC Authentication

Deploy a GitHub OIDC provider in your AWS account to enable keyless
authentication from GitHub Actions. Create IAM roles with trust policies
scoped to your GitHub repository.

See: https://atmos-pro.com/docs/configure/cloud-authentication

### 3. Set GitHub Repository Variables

Add these in GitHub Settings → Secrets and variables → Actions → Variables:

- `ATMOS_PRO_WORKSPACE_ID` — your Atmos Pro workspace ID (not a secret)
- `ATMOS_VERSION` — Atmos container image tag (e.g., `latest` or a specific version)

### 4. Create Your Workspace

Create a workspace for your organization. Connect this repository and
configure repository permissions.

See: https://atmos-pro.com/onboarding/create-workspace

### 5. Create GitHub Environments

Create GitHub Environments matching your `tenant-stage` naming convention
(e.g., `core-prod`, `plat-staging`). The apply workflow uses these for
deployment protection rules (required reviewers, wait timers).

### 6. Open a Test PR

Once the workflows are on your default branch, open a PR to trigger the
full Atmos Pro flow:

1. `atmos-pro-affected-stacks.yaml` determines affected stacks
2. Atmos Pro dispatches `atmos-pro-terraform-plan.yaml` for each affected stack
3. Plan results appear as PR status checks
4. On merge, Atmos Pro dispatches `atmos-pro-terraform-apply.yaml`
