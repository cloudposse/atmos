# Remote-State Bridge Pattern

This reference covers a pattern that solves two related but distinct problems with one
mechanism. Understand both use cases before applying it -- a single set of YAML primitives
serves both transient migration and permanent multi-repo architectures.

## What the Pattern Solves

The pattern registers an **external Terraform root module as a queryable component** in your
Atmos stacks, regardless of whether you own the actual Terraform code. Once registered, a real
Atmos component can pull outputs from it using `!terraform.state` just like any other Atmos
component.

This works whether the "external" root module is:

- A legacy vanilla-Terraform directory in the same repo that hasn't been migrated yet
- A root module owned by another repository (e.g., platform team's networking repo)
- A root module owned by another AWS account / GCP project / Azure subscription

The mechanism in all cases is the same: define a stack instance with `metadata.component`
pointing at a placeholder (or the real component as abstract), and use per-instance `backend`
overrides to point at the external state file. The deploy guard (`metadata.enabled: false` or
`metadata.type: abstract`) ensures CI/CD doesn't try to plan or apply against it.

## The Two Use Cases

### Use case 1: Progressive migration (transient)

The user is migrating a multi-module Terraform repo to Atmos one component at a time. Some
components are now Atmos components; others are still vanilla TF directories. A new Atmos
component needs to read outputs from one of the un-migrated dirs.

- **Lifetime:** Temporary. Bridge entries get deleted as each legacy directory becomes a real
  Atmos component.
- **Best variant:** [Variant A](#variant-a--dummy-component) -- a single tiny `dummy` component
  serves as the placeholder for all legacy state files.

### Use case 2: Multi-repo / cross-team state sharing (steady-state)

A workload repository needs to read outputs from components owned by a separate platform,
networking, or security repository. The dependency is permanent -- the workload repo declares
"these external components exist, here's where their state lives, and here's how I want to
reference them."

- **Lifetime:** Permanent. The bridge is the workload repo's contract with the platform repo.
- **Best variant:** [Variant B](#variant-b--abstract-real-component) -- reference the real
  component (vendored or via shared catalog) as `type: abstract`, with a cross-account
  read-only role for state access.

Both use cases share the same primitives. The variants differ in whether you need the real
component's code present at all.

## Variant A — Dummy Component

Use when the component does not yet exist in Atmos at all and you just need to expose its state
for `!terraform.state` lookups.

### Step 1: Create a stub component

Create `components/terraform/dummy/` with a single minimal `.tf` file. Atmos only needs a
directory it recognizes as a component -- no resources are ever provisioned because the dummy
is never deployed.

```hcl
# components/terraform/dummy/main.tf
# Placeholder component used by the remote-state bridge pattern.
# This component is never deployed; it exists only so Atmos can resolve
# stack instances that override `metadata.component: dummy` and use
# backend overrides to point at external Terraform state files.
terraform {
  required_version = ">= 1.0"
}
```

If the user follows Cloud Posse's `context.tf` convention, they can additionally drop in
`context.tf` from `terraform-null-label` -- it adds nothing functional to a dummy but keeps the
component file structure uniform with their other components.

### Step 2: Add a stack instance per legacy state file

For each un-migrated Terraform root module whose state you need to read, add an instance to the
appropriate stack:

```yaml
components:
  terraform:
    vpc/us-east-1-development:
      metadata:
        enabled: false                    # Blocks CI/CD plan/apply -- critical
        terraform_workspace: "default"    # Match the legacy workspace name exactly
        component: dummy                  # Point at the stub component
        name: vpc/us-east-1-development   # Stable instance name for !terraform.state lookups
      backend:
        s3:
          key: infra/terraform/123456789012/vpc/us-east-1-development/terraform.tfstate
```

The instance name (the YAML key, `vpc/us-east-1-development`) is what you'll use in
`!terraform.state` calls. Pick a name that matches how the user thinks about the legacy module.

### Step 3: Reference it from real components

From any real Atmos component in the same stack:

```yaml
components:
  terraform:
    eks-cluster:
      vars:
        vpc_id: !terraform.state vpc/us-east-1-development vpc_id
        subnet_ids: !terraform.state vpc/us-east-1-development private_subnet_ids
```

Atmos resolves these by reading the legacy state file directly via the backend config in the
dummy instance. The legacy Terraform code does not need to know Atmos exists.

### Why `metadata.enabled: false` matters

Without it, `atmos describe affected` and CI/CD pipelines will treat the dummy instance as a
deployable component. A pipeline could attempt to `terraform plan` against it, and since the
dummy has no resources, the plan would propose destroying everything in the legacy state file.
**Always set `enabled: false` on Variant A instances.**

### Cleanup as components are migrated

Once a legacy directory becomes a real Atmos component, delete the corresponding bridge
instance. The `!terraform.state` lookups can stay -- they'll resolve against the new real
component automatically (the instance name is what matters, and you can preserve it via the
new component's `metadata.name`).

## Variant B — Abstract Real Component

Use when the real component code exists (perhaps vendored, perhaps in another repo / account)
and you want to declare a permanent state-reading dependency on it.

```yaml
components:
  terraform:
    ecs/cluster:
      metadata:
        component: ecs                                          # The real component
        type: abstract                                          # Blocks direct deploy
        terraform_workspace: '{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.deps_stage }}-{{ .atmos_component | regexp.ReplaceLiteral "\\W" "-" }}'
      backend_type: s3
      backend:
        s3:
          bucket: cplive-core-ue2-root-tfstate-plat            # Platform team's state bucket
          encrypt: true
          key: terraform.tfstate
          acl: bucket-owner-full-control
          region: us-east-2
          assume_role:
            role_arn: arn:aws:iam::828744362454:role/cplive-core-gbl-root-tfstate-plat-ro  # READ-ONLY role
```

### Why `type: abstract` instead of `enabled: false`

`metadata.type: abstract` declares this instance is not directly deployable but is intended as a
base for inheritance or external reference. Other Atmos commands (like `atmos list components`)
distinguish abstract components from concrete ones. For the bridge pattern, either guard works
-- pick the one that matches the user's mental model:

- `type: abstract` -- "this is a definition, not a deployable" (best when the real component
  exists but is owned elsewhere)
- `enabled: false` -- "this is disabled for this stack" (best when the component is a true
  placeholder)

### Why the templated `terraform_workspace`

The upstream owner (platform team) writes state with their own workspace naming convention.
Your bridge instance must match that convention exactly to find the right state file.
Templating it from `vars.tenant`, `vars.environment`, etc. keeps the bridge in sync as the
upstream evolves -- you change vars in one place, not workspace names across many files.

### Why a read-only role

The bridge only needs to **read** state. Use a dedicated read-only role
(naming convention: `*-ro`) for the `assume_role.role_arn`. This:

- Prevents the workload repo from accidentally writing to platform-owned state
- Satisfies least-privilege requirements for security review
- Makes audit logs unambiguous about which side wrote the state

Have the platform team publish the read-only role ARN as part of their consumer interface
documentation.

## Decision Matrix

| Situation                                                       | Use                                                   |
|-----------------------------------------------------------------|-------------------------------------------------------|
| Legacy TF dir, no Atmos component for it yet, default workspace | Variant A (dummy + `enabled: false`)                  |
| Legacy TF dir, non-default workspace convention                 | Variant A with `terraform_workspace` set to match     |
| Real component owned by another team / repo / account           | Variant B (real component + `type: abstract`)         |
| State lives in a different cloud account/project                | Variant B with `assume_role` to a read-only role      |
| You'll delete the bridge after migration completes              | Variant A -- easier to remove cleanly                 |
| Permanent multi-repo workload→platform contract                 | Variant B -- documents the dependency in code         |

## Common Mistakes

- **Forgetting the deploy guard.** Without `enabled: false` or `type: abstract`, CI will try to
  plan/apply against the bridge instance. For Variant A this can propose destroying everything
  in the legacy state.
- **Wrong workspace name.** `terraform_workspace` must match the actual workspace the state was
  written under. A typo silently creates a new (empty) workspace and `!terraform.state` returns
  nothing useful.
- **Using a read/write role for cross-account state access.** The bridge only reads. Use a
  `*-ro` role and have the state owner create it.
- **Bridging your own active state with the same component twice.** If you have a real Atmos
  component AND a bridge instance both pointing at the same backend key, the real component's
  plan/apply will fight the bridge. Bridges are for state you don't own (or haven't migrated
  yet); never bridge a component you're actively deploying.
- **Forgetting to delete migrated bridges.** Variant A entries should be deleted as their
  legacy backing directory becomes a real Atmos component. Stale bridges add noise and can mask
  schema mismatches.

## Related Skills

- [atmos-yaml-functions](../../atmos-yaml-functions/SKILL.md) -- for `!terraform.state` syntax and YQ expressions
- [atmos-stacks](../../atmos-stacks/SKILL.md) -- for `metadata.component`, `metadata.name`, inheritance semantics
- [atmos-components](../../atmos-components/SKILL.md) -- for `type: abstract` semantics and concrete-vs-abstract patterns
- [atmos-auth](../../atmos-auth/SKILL.md) -- for cross-account role configuration patterns
