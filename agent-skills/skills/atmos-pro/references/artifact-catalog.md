# Artifact Catalog

Every file the skill produces, with the rationale and when to include it.

## `stacks/mixins/atmos-pro.yaml`

**Always.** Single import point for Atmos Pro. Does three things:

1. Imports the IAM role catalog (`catalog/aws/iam-role/gha-tf`).
2. Disables Spacelift (`settings.spacelift.workspace_enabled: false`).
3. Declares `settings.pro` with the full event → workflow → inputs dispatch contract.

Source template: `templates/mixins/atmos-pro.yaml.tmpl`. See
[`settings-pro-contract.md`](settings-pro-contract.md) for the schema.

## `stacks/catalog/aws/iam-role/defaults.yaml`

**Always.** Abstract base component with OIDC trust defaults:

- `github_oidc_provider_enabled: true`
- `github_oidc_provider_arn: !terraform.state github-oidc-provider oidc_provider_arn`
- `trusted_github_org: {{ .org }}`
- `trusted_github_repos: ["{{ .org }}/{{ .repo }}:main"]`

## `stacks/catalog/aws/iam-role/gha-tf.yaml`

**Always.** Imports `defaults.yaml` and declares three concrete components:

- `aws/iam-role/gha-tf/defaults` (abstract) — includes `TerraformStateBackendAssumeRole` policy
  granting `sts:AssumeRole` on the tfstate role ARNs.
- `aws/iam-role/gha-tf-apply` — inherits defaults + gha-tf/defaults. AdministratorAccess,
  SSM write, KMS decrypt for SSM.
- `aws/iam-role/gha-tf-plan` — inherits defaults + gha-tf/defaults. ReadOnlyAccess, SSM read,
  KMS decrypt for SSM. Overrides `trusted_github_repos` to allow any ref (required for PR plans).

## `components/terraform/aws/iam-role/component.yaml`

**Always.** Vendor manifest pointing at
`cloudposse-terraform-components/aws-iam-role v1.537.0`. Excludes upstream `providers.tf` and
mixes in `provider-without-account-map.tf` and `account-verification.mixin.tf`.

The component is vendored into `components/terraform/aws/iam-role/`, keeping the legacy
`components/terraform/iam-role/` untouched for existing roles.

## `profiles/github-plan/atmos.yaml`

**Always.** One `identities:` entry per detected `{tenant}-{stage}` pair. Every identity points
at the account's `*-gha-tf-plan` role ARN. Used when `ATMOS_PROFILE=github-plan` is set in the
plan workflow.

## `profiles/README.md`

**Always.** Explainer doc for the `profiles/` directory: which profile each workflow uses,
how `ATMOS_PROFILE` deep-merge works, how to add a new account. Source template:
`templates/profiles/README.md.tmpl`. Helps future maintainers understand the directory
without reading the SKILL or the Atmos docs.

## `profiles/github-apply/atmos.yaml`

**Always.** One `identities:` entry per detected `{tenant}-{stage}` pair.

- Non-root accounts point at the `*-gha-tf-apply` role ARN.
- **Root account points at the `*-gha-tf-plan` ARN** (safety rail — automation must never apply
  to the root account). A comment in the file explains this.

Used when `ATMOS_PROFILE=github-apply` is set in the apply workflow.

## `.github/workflows/atmos-pro.yaml`

**Always.** Triggered on `pull_request` (`opened`, `synchronize`, `reopened`, `closed`).
Runs `atmos describe affected --upload --process-functions=false` to publish the affected
component/stack list to Atmos Pro. Atmos Pro then dispatches plan workflows per affected pair.

Runs in `ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}`. Uses `ATMOS_PROFILE=github-plan`.

## `.github/workflows/atmos-pro-list-instances.yaml`

**Always.** Triggered on push to `main`, a daily schedule, and `workflow_dispatch`.
Runs `atmos list instances --upload` to publish the full instance inventory for drift detection
and compliance views. Uses `ATMOS_PROFILE=github-plan`.

## `.github/workflows/atmos-terraform-plan.yaml`

**Always.** `workflow_dispatch` only — dispatched by Atmos Pro with inputs `sha`, `component`,
`stack`. Runs `atmos terraform plan <component> -s <stack> --upload-status`. Uses
`ATMOS_PROFILE=github-plan`.

## `.github/workflows/atmos-terraform-apply.yaml`

**Always.** `workflow_dispatch` only — dispatched by Atmos Pro with inputs `sha`, `component`,
`stack`, `github_environment`. Runs `atmos terraform deploy <component> -s <stack> --upload-status`.
Uses `ATMOS_PROFILE=github-apply`. Requires the GitHub Environment named in `github_environment`
to exist (created manually by the user).

## Edit: `stacks/catalog/tfstate-backend/defaults.yaml`

**Always** (additive). Adds wildcard patterns to `access_roles.default.allowed_principal_arns`:

```yaml
allowed_principal_arns:
  - "arn:aws:iam::*:role/{{ .namespace }}-*-gbl-*-gha-tf-*"
```

If the trust policy is near the IAM 2048-char size limit, additionally sets
`team_permission_sets_enabled: false` to suppress auto-generated SSO PermissionSet ARN patterns.

## Edit: `stacks/orgs/{target_org}/_defaults.yaml`

**Always.** Adds `mixins/atmos-pro` to the org's `import:` list. This is the one line that
actually enables Atmos Pro for the org.

## `docs/atmos-pro.md`

**Always.** Generated README describing:

- What was produced by the skill and why
- The rollout procedure (deploy order, verification steps)
- How to add more orgs later
- How to add more repos to the same AWS accounts
- Troubleshooting quick-reference

Variant sections included based on detection:

- Geodesic usage section (only if Geodesic was detected)
- Spacelift migration notes (only if Spacelift was previously enabled)
- Atmos Auth adoption path (only if no Atmos Auth was detected)

## Optional: `.github/workflows/oidc-test.yaml`

**Generated by default.** Manual-dispatch workflow that calls `aws sts get-caller-identity`
through the plan role. Confirms OIDC trust works end-to-end without involving Atmos Pro.
Users should run this as step 5 of the rollout checklist.
