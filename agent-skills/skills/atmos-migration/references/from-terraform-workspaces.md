# Migrating from Terraform Workspaces

This reference is the agent's decision guide for users coming from a `terraform.workspace`-driven
setup. For the full user-facing prose tutorial, see
[atmos.tools/migration/terraform-workspaces](https://atmos.tools/migration/terraform-workspaces).

## The Two Migration Paths

There are two paths, and choosing wrong costs the user state. Identify which one fits their
situation before proposing anything:

| Situation                                                              | Path                                |
|------------------------------------------------------------------------|-------------------------------------|
| User wants to keep existing workspace state intact, migrate gradually  | [Path 1: Keep workspaces](#path-1-keep-the-workspace-state-easiest) |
| User wants per-environment backend isolation (recommended steady state) | [Path 2: Separate backends](#path-2-migrate-to-separate-backends-recommended) |

Default recommendation: **Path 1 first** to get the user running on Atmos with zero state risk,
then **Path 2 later** when they have time for the state migration. Do not force Path 2 on day one.

## Mapping `terraform.workspace` → Atmos Stack

The cleanest mapping is: **one workspace → one stack file**. The workspace name typically becomes
the stack name (`dev`, `staging`, `prod`).

If the user has a workspace naming convention you need to preserve (e.g., the upstream state was
written with workspaces named `tenant-environment-stage-component`), use
`metadata.terraform_workspace` on the component to override the workspace name Atmos derives:

```yaml
components:
  terraform:
    vpc:
      metadata:
        terraform_workspace: '{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.stage }}-{{ .atmos_component | regexp.ReplaceLiteral "\\W" "-" }}'
```

This is critical when the legacy workspace name does **not** match Atmos's default workspace
derivation. Without it, Atmos will create a new (empty) workspace and the user will think their
state vanished.

## Replacing `terraform.workspace` Ternaries

The single largest source of code in workspace-based repos is conditional logic keyed on
`terraform.workspace`. Replace it with stack-level `vars`.

**Before (workspace logic in `.tf`):**
```hcl
locals {
  instance_type     = terraform.workspace == "prod" ? "m5.large" : "t3.small"
  enable_monitoring = terraform.workspace == "prod" ? true : false
  backup_retention  = terraform.workspace == "prod" ? 30 : 7
}
```

**After (config moves to stack YAML, code stays generic):**

```hcl
variable "instance_type"     { type = string }
variable "enable_monitoring" { type = bool }
variable "backup_retention"  { type = number }
```

```yaml
# stacks/prod.yaml
components:
  terraform:
    app:
      vars:
        instance_type: m5.large
        enable_monitoring: true
        backup_retention: 30

# stacks/dev.yaml
components:
  terraform:
    app:
      vars:
        instance_type: t3.small
        enable_monitoring: false
        backup_retention: 7
```

Once the conditionals are gone, the Terraform code is generic and reusable across any number of
stacks without further changes.

## Path 1: Keep the Workspace State (Easiest)

For users who already have valuable state in workspaces and don't want to migrate it, Atmos can
read and write to the existing workspace structure unchanged.

```yaml
# stacks/prod.yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: terraform-state           # Same bucket as before
      key: vpc/terraform.tfstate        # Same key
      region: us-east-1
      workspace_key_prefix: env         # Matches existing workspace convention

components:
  terraform:
    vpc:
      metadata:
        terraform_workspace: prod       # Selects the existing workspace
      vars:
        cidr_block: "10.100.0.0/16"
        environment: prod
```

**Critical:** The user's `metadata.terraform_workspace` value must exactly match the existing
workspace name in their state backend. If their workspaces are named `env:prod` vs `prod` vs
`production`, the value here must match exactly or Atmos will operate against an empty workspace.

After this works for one stack, repeat for `dev`, `staging`, etc. The user has zero state risk
and can now use `atmos terraform plan/apply` against their existing infrastructure.

## Path 2: Migrate to Separate Backends (Recommended)

This is the steady-state target for most users -- per-environment backend isolation prevents the
"prod state corrupted by dev misconfig" failure mode that workspaces are vulnerable to.

This requires a one-time state migration per workspace. Walk the user through it carefully -- it
is the highest-risk step in the whole migration.

1. **Export state from each workspace:**
   ```bash
   terraform workspace select prod
   terraform state pull > prod.tfstate
   ```
2. **Configure the new per-stack backend in Atmos:**
   ```yaml
   # stacks/prod.yaml
   terraform:
     backend_type: s3
     backend:
       s3:
         bucket: terraform-state-prod   # Dedicated bucket
         key: vpc.tfstate
         region: us-east-1
   ```
3. **Initialize the new backend and push state:**
   ```bash
   atmos terraform init vpc -s prod
   terraform state push prod.tfstate
   ```
4. **Verify with a plan -- it must show zero changes:**
   ```bash
   atmos terraform plan vpc -s prod
   ```
5. Repeat for each workspace.

After all environments are on isolated backends, the user can delete the workspaces from the
original backend (after confirming no other tooling references them).

## Real Environments vs Per-Developer Sandboxes

Users sometimes conflate two distinct uses of workspaces:

- **Environment workspaces** (`dev`, `staging`, `prod`) -- these become Atmos stacks. Use either
  migration path above.
- **Per-developer sandbox workspaces** (`alice-test`, `bob-experiment`) -- these are usually
  short-lived and should not become long-lived stacks. Convert them to ephemeral stacks created
  on demand (e.g., via `atmos terraform apply <comp> -s sandbox --var "owner=alice"`) or have
  developers create per-feature stack files (`stacks/sandbox-alice.yaml`).

If the user has hundreds of sandbox workspaces from drift over years, treat them as state to
audit-and-delete, not state to migrate.

## Reading State from Un-migrated Workspaces

If the user is migrating component-by-component, they will often need a new Atmos component to
read outputs from a TF root module that still uses workspaces. The
[remote-state-bridge.md](remote-state-bridge.md) pattern handles this -- specifically Variant A
with `metadata.terraform_workspace` set to the legacy workspace name and `backend.s3` pointing
at the legacy state file.

## CI/CD Update

Replace workspace selection in CI with stack arguments:

**Before:**
```bash
terraform workspace select $ENV
terraform plan
terraform apply -auto-approve
```

**After:**
```bash
atmos terraform plan $COMPONENT -s $STACK
atmos terraform apply $COMPONENT -s $STACK -auto-approve
```

For the broader CI/CD setup (affected detection, native Atmos CI container, GitHub Actions
patterns), route to the [atmos-ci](../../atmos-ci/SKILL.md) skill.

## Common Mistakes

- **Wrong workspace name in `metadata.terraform_workspace`** -- silently operates against an
  empty workspace. Always cross-check against `terraform workspace list` output.
- **Migrating state without a backup** -- always `terraform state pull > backup.tfstate` before
  any push or backend change.
- **Removing workspace logic from `.tf` before stacks are ready** -- the user's existing
  `terraform apply` will break. Add the variable declarations alongside the locals, then remove
  locals only after stacks are populated.
- **Forcing Path 2 on day one** -- Path 1 unblocks Atmos adoption; Path 2 can come later as a
  steady-state cleanup. Don't make state migration a prerequisite for trying Atmos.
