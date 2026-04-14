# IAM Trust Model

The OIDC trust model has three moving parts that must align: the OIDC subject-claim scope on
the IAM role, the reciprocal state-backend trust, and the root-account safety rail.

## OIDC subject-claim scoping

`trusted_github_repos` controls the `token.actions.githubusercontent.com:sub` condition. The
component translates entries into IAM condition patterns:

| `trusted_github_repos` entry               | IAM subject pattern                              | Scope                              |
|--------------------------------------------|--------------------------------------------------|------------------------------------|
| `"org/repo:main"`                          | `repo:org/repo:ref:refs/heads/main`              | `main` branch only                 |
| `"org/repo"`                               | `repo:org/repo:*`                                | Any ref, branch, or PR             |
| `"org/repo:environment:production"`        | `repo:org/repo:environment:production`           | GitHub Environment `production`    |
| `"org/repo:ref:refs/tags/v*"`              | `repo:org/repo:ref:refs/tags/v*`                 | Tag matching `v*`                  |

### Default policy

The abstract base (`stacks/catalog/aws/iam-role/defaults.yaml`) uses the most restrictive scope:

```yaml
trusted_github_repos:
  - "{{ .org }}/{{ .repo }}:main"
```

Any role that inherits from this default is `main`-branch-only.

### Per-role overrides

| Role | Trust scope | Rationale |
|------|-------------|-----------|
| `aws/iam-role/gha-tf-apply` | `main` only (inherits default) | Admin access must only run post-merge |
| `aws/iam-role/gha-tf-plan`  | Any ref (`"{{ .org }}/{{ .repo }}"`) | `terraform plan` runs on PR branches before merge |

The plan role's override is the only place the restrictive default is relaxed. The plan role
only has `ReadOnlyAccess`, so the wider subject scope is safe — with one caveat below.

### Security caveat the skill must document

The plan role is not branch-scoped and has `sts:AssumeRole` on the tfstate roles (read/write
on state). Anyone who can trigger a workflow in the repo can read Terraform state through the
plan role. The skill's generated `docs/atmos-pro.md` must state this explicitly.

### Alternative: GitHub Environment scoping

Replacing branch scope with environment scope:

```yaml
trusted_github_repos:
  - "{{ .org }}/{{ .repo }}:environment:production"
```

This requires the user to create a GitHub Environment named `production` in repo settings.
Environment protection rules (required reviewers, wait timers) gate role assumption. This is
stricter than `main`-branch scoping and is the recommended upgrade for high-privilege apply
roles.

The skill offers this as an opt-in question in Step 4 of the playbook.

## Reciprocal tfstate trust

IAM roles need `sts:AssumeRole` on the tfstate backend roles. **Both sides of the trust must
be deployed together.**

### IAM role side

In `stacks/catalog/aws/iam-role/gha-tf.yaml`, the abstract `aws/iam-role/gha-tf/defaults`
includes:

```yaml
policy_statements:
  TerraformStateBackendAssumeRole:
    effect: "Allow"
    actions:
      - "sts:AssumeRole"
      - "sts:TagSession"
      - "sts:SetSourceIdentity"
    resources:
      - arn:aws:iam::{{ .root_account_id }}:role/{{ .namespace }}-core-gbl-root-tfstate
      - arn:aws:iam::{{ .root_account_id }}:role/{{ .namespace }}-core-gbl-root-tfstate-ro
```

Both plan and apply roles inherit this via `aws/iam-role/gha-tf/defaults` inheritance.

### Multi-namespace repo

If multiple namespaces (e.g., separate `dev` / `stg` / `prd` namespaces) live in the
same repo, the `resources:` list expands to include each namespace's tfstate-backend
ARNs. Group them by namespace with comments so future readers can tell which is which:

```yaml
resources:
  # dev namespace tfstate roles
  - arn:aws:iam::111111111111:role/dev-core-gbl-root-tfstate
  - arn:aws:iam::111111111111:role/dev-core-gbl-root-tfstate-ro
  # stg namespace tfstate roles
  - arn:aws:iam::222222222222:role/stg-core-gbl-root-tfstate
  - arn:aws:iam::222222222222:role/stg-core-gbl-root-tfstate-ro
  # prd namespace tfstate roles
  - arn:aws:iam::333333333333:role/prd-core-gbl-root-tfstate
  - arn:aws:iam::333333333333:role/prd-core-gbl-root-tfstate-ro
```

Over-listing is harmless: the actual `sts:AssumeRole` call still requires the trust
policy on the target tfstate role to allow the calling principal. Per-account
isolation is enforced on the tfstate side, not by the IAM role's resource list.

### tfstate-backend side

In `stacks/catalog/tfstate-backend/defaults.yaml`, `access_roles.default.allowed_principal_arns`
is extended additively with a wildcard per namespace:

```yaml
# Single-namespace repo:
allowed_principal_arns:
  - "arn:aws:iam::*:role/{{ .namespace }}-*-gbl-*-gha-tf-*"

# Multi-namespace repo (one wildcard per namespace):
allowed_principal_arns:
  - "arn:aws:iam::*:role/dev-*-gbl-*-gha-tf-*"
  - "arn:aws:iam::*:role/stg-*-gbl-*-gha-tf-*"
  - "arn:aws:iam::*:role/prd-*-gbl-*-gha-tf-*"
```

For multi-namespace setups, also set `team_permission_sets_enabled: false` on the
tfstate-backend component to prevent SSO PermissionSet auto-generation from pushing
the trust policy past the IAM 2048-char size limit. Three namespace wildcards plus
auto-generated PermissionSet ARNs (one per team per namespace) routinely exceed the
limit; the explicit `allowed_permission_sets` entries below the wildcards still take
effect.

### `team_permission_sets_enabled` requires module-level support

The variable is **silently ignored** unless the underlying Terraform code declares
and plumbs it through. Older vendored copies of `tfstate-backend` and
`account-map/modules/team-assume-role-policy` do not declare it. Before relying on
the stack-level setting, verify support — and patch if missing:

```bash
# Detect module support
grep -q "team_permission_sets_enabled" \
  components/terraform/tfstate-backend/variables.tf \
  components/terraform/account-map/modules/team-assume-role-policy/variables.tf \
  2>/dev/null && echo "OK" || echo "MISSING — patch required"
```

When the check reports MISSING, the skill must emit four patches:

1. **`components/terraform/account-map/modules/team-assume-role-policy/variables.tf`**
   — add the variable declaration:

   ```hcl
   variable "team_permission_sets_enabled" {
     type        = bool
     description = <<-EOT
       When true, team roles in the identity account will also generate corresponding
       AWS SSO PermissionSet ARN patterns in the trust policy. Set to false when trust
       policies are exceeding IAM size limits and SSO access is not needed.
       EOT
     default     = true
   }
   ```

2. **`components/terraform/account-map/modules/team-assume-role-policy/main.tf`**
   — pass the variable to both `roles-to-principals` module calls:

   ```hcl
   module "allowed_role_map" {
     # ... existing inputs ...
     overridable_team_permission_sets_enabled = var.team_permission_sets_enabled
   }
   module "denied_role_map" {
     # ... existing inputs ...
     overridable_team_permission_sets_enabled = var.team_permission_sets_enabled
   }
   ```

3. **`components/terraform/tfstate-backend/variables.tf`** — same variable declaration
   as in step 1.

4. **`components/terraform/tfstate-backend/iam.tf`** — pass the variable to the
   `assume_role` module:

   ```hcl
   module "assume_role" {
     # ... existing inputs ...
     team_permission_sets_enabled = var.team_permission_sets_enabled
   }
   ```

Without these four patches, the trust policy will silently exceed 2048 chars on
multi-namespace tfstate-backend deployments and the apply will fail with
`InvalidParameter: LimitExceeded: Cannot exceed quota for PolicySize: 2048`.

### Why both sides

The IAM role says "I'm allowed to assume the tfstate role." The tfstate role says "I allow
these principals to assume me." Both must agree. If only one side is deployed, the assumption
returns `AccessDenied` with no useful error message.

The skill enforces this by generating both edits in the same PR and refusing to proceed if
the user declines one.

## Root-account safety rail

Automation must never apply to the root AWS account. The root account holds organization-level
IAM and AWS Organizations — an automated apply there could disable guardrails for the entire
infrastructure.

### How the rail is implemented

In `profiles/github-apply/atmos.yaml`, the root account's identity points at the **plan**
role's ARN, not the apply role's:

```yaml
identities:
  core-root/gha-tf-apply:
    kind: aws/assume-role
    via:
      provider: github-oidc
    principal:
      # Safety: assumes gha-tf-plan (read-only) to prevent automation from applying to root account.
      assume_role: arn:aws:iam::{{ .root_account_id }}:role/{{ .namespace }}-core-gbl-root-gha-tf-plan
```

The skill must emit this comment verbatim so future contributors understand why the apply
profile references a plan role ARN.

### Detection

The skill identifies the root account stack by:

1. Inspecting `atmos.yaml` `auth.identity_accounts` or equivalent config for a `root` entry.
2. Falling back to the stack whose name matches `*-gbl-root` or `*-root-gbl-*`.
3. Asking the user if neither heuristic resolves.

## Why `gha-tf-plan` for the plan role name

The `gha-tf-` prefix namespaces Atmos Pro roles separately from human IAM roles and legacy
automation roles. The suffix (`plan` or `apply`) distinguishes privilege level. The
wildcard `arn:aws:iam::*:role/{{ .namespace }}-*-gbl-*-gha-tf-*` in the tfstate trust relies
on this prefix.

Changing the role names means updating:

1. The role `name` in `stacks/catalog/aws/iam-role/gha-tf.yaml`
2. The identity ARNs in both `profiles/github-plan/atmos.yaml` and `profiles/github-apply/atmos.yaml`
3. The wildcard in `stacks/catalog/tfstate-backend/defaults.yaml`
4. All references in `docs/atmos-pro.md`

The skill does **not** support customizing the role names in v1 — consistency with the
reference implementation is more important than flexibility.
