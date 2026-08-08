# Migrating from Terragrunt

This reference covers both Terragrunt shapes: the classic pattern (`terragrunt.hcl`,
`include`, `find_in_parent_folders()`) and the newer Stacks pattern
(`terragrunt.stack.hcl`, `unit` blocks). For the full user-facing prose tutorial with
complete concept and function mapping tables, see
[atmos.tools/migration/terragrunt](https://atmos.tools/migration/terragrunt). This
reference distills that tutorial into agent-actionable recipes and adds material the
tutorial does not cover: how to translate Terragrunt Stacks.

## Identifying the User's Shape

Check the repository for these signals before proposing anything:

| Signal | Shape | Recipe |
|---|---|---|
| `terragrunt.hcl` files, no `terragrunt.stack.hcl` | Classic | This reference, sections below |
| `terragrunt.stack.hcl` present | Stacks | This reference, plus [Terragrunt Stacks](#terragrunt-stacks-map-directly-to-atmos-stacks) below |
| A Gruntwork "Runbooks" scaffold on top of Stacks | Stacks + scaffolding | Same as Stacks — the scaffolding layer does not change the mapping |

Mixed repositories exist. Treat each `terragrunt.hcl` or `terragrunt.stack.hcl` unit
independently, the same way multiple root modules get treated independently in a
native-Terraform migration.

## Core Concept Mapping

Each pair below shows the Terragrunt construct and its direct Atmos equivalent. Full
function-by-function tables live in the canonical tutorial; this section covers only
the constructs an agent needs to translate a real project.

### `include` and `find_in_parent_folders()` → stack imports

Terragrunt's DRY mechanism climbs the directory tree at run time. Atmos resolves
inheritance declaratively through `import:` and deep-merge, with no directory-walk
step:

```hcl
# terragrunt.hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}
```

```yaml
# stacks/orgs/acme/prod/us-east-1.yaml
import:
  - orgs/acme/_defaults
  - mixins/region/us-east-1
```

Atmos's import system supports deeper inheritance chains (org → tenant → stage →
region) than Terragrunt's parent-folder walk, and it works the same way whether the
imported file lives next to the stack or in a separate repository. Route deeper
organization questions to the [atmos-design-patterns](../../atmos-design-patterns/SKILL.md)
skill.

### `generate "backend"` / `generate "provider"` → automatic backend and provider generation

Terragrunt's `root.hcl` typically has a `generate "provider"` block and a
`remote_state { generate = {...} }` block that each write one file. Atmos generates
both automatically from stack configuration — no `generate` block needed for either:

```hcl
generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "aws" {
  region = "${local.aws_region}"
}
EOF
}

remote_state {
  backend = "s3"
  config = { bucket = "...", key = "...", region = "..." }
  generate = { path = "backend.tf", if_exists = "overwrite_terragrunt" }
}
```

```yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: acme-prod-tfstate
      key: terraform.tfstate
      region: us-east-1
```

Atmos writes `backend.tf.json` from this section automatically at plan/apply time. No
provider-file generation is needed either — set provider region and account
restrictions through stack `vars:` that the component's own `providers.tf` reads, or
through the `providers:` stack section (`website/docs/stacks/providers.mdx`, e.g.
`terraform.providers:` at the component-type level or `providers:` on the component)
when the provider block itself needs per-stack values Atmos does not already inject
through an identity.

If a Terragrunt unit's `generate` block writes something other than a backend or
provider file, Atmos has a direct, more general equivalent: the declarative `generate:`
stack section (`website/docs/stacks/generate.mdx`), which writes arbitrary files from
stack configuration with full templating and a 5-level merge (global, component-type,
base component, component, and override). This covers the general case Terragrunt's
`generate` block handles; backend and provider files are simply the two cases Atmos
automates without any `generate:` configuration at all.

### `dependency` blocks → `dependencies.components` and `!terraform.state`

Terragrunt's `dependency` block does two things at once: it orders execution and it
reads another unit's outputs. Atmos splits these into two primitives that compose:

```hcl
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
```

```yaml
components:
  terraform:
    eks-cluster:
      dependencies:
        components:
          - name: vpc
      vars:
        vpc_id: !terraform.state vpc vpc_id
```

`dependencies.components` orders execution across `atmos terraform apply --all` and
`--affected`, the same job `dependency` does in Terragrunt. `!terraform.state` reads
the real value once the dependency is deployed.

### `mock_outputs` → the `mocks` component field and `--use-mocks`

Atmos has a first-class, direct equivalent to `mock_outputs`, not a workaround. A
Terraform component declares literal output values under `mocks`, and any caller
resolves them explicitly with `--use-mocks`:

```hcl
dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    vpc_id             = "vpc-mock1234"
    private_subnet_ids = ["subnet-a", "subnet-b"]
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
}
```

```yaml
components:
  terraform:
    vpc:
      mocks:
        vpc_id: vpc-mock1234
        private_subnet_ids: [subnet-a, subnet-b]
```

```shell
atmos terraform plan eks-cluster -s dev --use-mocks
atmos describe component eks-cluster -s dev --use-mocks
```

With `--use-mocks`, `!terraform.state vpc vpc_id` and `!terraform.output vpc vpc_id`
resolve from `vpc`'s `mocks` map instead of real state, with no Terraform init,
authentication, or backend read at all. Without `--use-mocks`, the same expression
resolves the real value as usual. This matches Terragrunt's
`mock_outputs_allowed_terraform_commands` scoping more closely than a default-value
expression does: `--use-mocks` is rejected outright on `apply`, `deploy`, and
`destroy`, so a mock value can never reach a mutating Terraform operation, and an
undeclared `mocks` entry is a hard error rather than a silent fallback. A working,
provider-free reference lives at `examples/terraform-component-mocks` in the Atmos
repository.

Reserve the YQ-default pattern (`!terraform.state vpc '.vpc_id // "vpc-mock1234"'`)
for the narrower case of a placeholder that should also apply during a normal,
non-mock run against a dependency that genuinely has not deployed yet — for example,
while bringing up a dependency graph for the first time. `mocks` and `--use-mocks` are
the right default for anything resembling Terragrunt's `mock_outputs`.

### Terraform source pinning → vendoring

```hcl
terraform {
  source = "git::git@github.com:acme/modules.git//vpc?ref=v1.2.3"
}
```

```yaml
# vendor.yaml
spec:
  sources:
    - component: vpc
      source: github.com/acme/modules.git//vpc?ref={{.Version}}
      version: v1.2.3
      targets:
        - components/terraform/vpc
```

`atmos vendor pull` pulls the pinned module into `components/terraform/vpc` as a
one-time step, rather than Terragrunt's implicit download on every run. This is a real
workflow difference worth naming to the user directly: vendoring is explicit
(`atmos vendor pull`, typically a CI or setup step), not automatic on every
`atmos terraform plan`. Route deeper vendoring questions (include/exclude globs,
`atmos vendor diff`/`update`) to the [atmos-vendoring](../../atmos-vendoring/SKILL.md)
skill.

### `before_hook` / `after_hook` → Atmos hooks

```hcl
terraform {
  before_hook "package" {
    commands = ["apply", "plan", "destroy"]
    execute  = ["./scripts/package.sh", "./src", "./handler.zip"]
  }
}
```

```yaml
components:
  terraform:
    lambda-service:
      hooks:
        package:
          events:
            - before.terraform.plan
            - before.terraform.apply
            - before.terraform.destroy
          kind: step
          type: archive
          with:
            source: src/
            destination: handler.zip
```

For the common case of packaging a directory into a zip or tar before Terraform runs
(a Lambda deployment artifact is the most frequent example), use the native
`type: archive` step through the `kind: step` hook bridge shown above, not a
`kind: command` hook shelling out to `zip`/`tar`. The archive step runs on the Go
standard library alone, so it behaves identically on Windows, whereas a shell hook
depends on `zip`/`tar` being installed and behaving the same way across platforms.

For any other packaging or validation logic a `before_hook`/`after_hook` runs, use the
generic `kind: command` hook, or one of the named scanner kinds (`checkov`, `trivy`,
`kics`, `infracost`) if the hook wraps a security or cost tool Atmos already has a
built-in kind for. See the [atmos-hooks](../../atmos-hooks/SKILL.md) skill for the
full kind list and event lifecycle.

**Known rough edge:** if `atmos terraform plan` reports the archive source does not
exist even though the path shown above looks correct, use an absolute path for
`source`/`destination` instead of a relative one. Some Atmos versions do not resolve
a relative archive-step path against the component directory when the step runs as a
hook.

## Terragrunt Stacks Map Directly to Atmos Stacks

A common misreading is that `terragrunt.stack.hcl` needs a special Atmos feature to
translate — it does not. Terragrunt Stacks retrofits "declare a reusable, parameterized
group of units" onto Terragrunt's original one-`terragrunt.hcl`-per-directory
architecture. Atmos never had that constraint: an Atmos **stack** already is a named,
declarative, parameterized collection of components, resolved without any
materialization step. The mapping is direct, not a workaround:

```hcl
# terragrunt.stack.hcl
unit "lambda_service" {
  source = "github.com/acme/catalog//units/lambda-service"
  path   = "service"
  values = { name = "my-service", runtime = "nodejs22.x" }
  autoinclude {
    dependency "role" { config_path = unit.role.path }
  }
}

unit "role" {
  source = "github.com/acme/catalog//units/iam-role"
  path   = "role"
}
```

```yaml
# One Atmos stack manifest — no materialization, no per-unit directory
components:
  terraform:
    lambda-service:
      dependencies:
        components:
          - name: iam-role
      vars:
        name: my-service
        runtime: nodejs22.x
        iam_role_arn: !terraform.state iam-role '.arn // "pending"'
    iam-role:
      vars:
        name: my-service-role
```

Run this with `atmos terraform apply --all -s <stack>` (or `--affected` for a subset).
Do not reach for `compositions` here — that primitive groups components for local
multi-kind operation (container, compose, terraform together) and explicitly does not
define execution order. Ordering and value-passing across units is
`dependencies.components` and `!terraform.state`, exactly as shown above, whether the
source project uses classic Terragrunt or Terragrunt Stacks.

One difference worth naming to the user: Terragrunt Stacks pins each unit's catalog
source and version inline in the `unit` block. Atmos separates that concern into
`vendor.yaml` (or direct component authoring), so a stack manifest's `components:`
section only carries values, not source references. This is a cleaner separation of
concerns, not a missing capability — see
[from-terragrunt.md](#terraform-source-pinning--vendoring) above for the vendoring
side.

## Migration Workflow

1. **Inventory the source repository.** Find every `terragrunt.hcl` and
    `terragrunt.stack.hcl`, and every `include`/`dependency` reference between them, to
    build a migration order — the same reconnaissance step a native-Terraform migration
    starts with.
2. **Stand up `atmos.yaml` and one stack file** for a single unit, converting `include`
    and `inputs` per the concept mapping above.
3. **Wire `dependency` blocks** to `dependencies.components` and `!terraform.state`,
    mapping `mock_outputs` to the producer component's `mocks:` field and using
    `--use-mocks` for read-only planning/description (reserve the YQ `// "default"`
    pattern for real dependencies that simply have not deployed yet).
4. **Convert `generate` blocks.** Backend and provider generation need no
    configuration at all; anything else becomes a `generate:` stack section.
5. **Validate** with `atmos validate stacks`, then compare `atmos list affected`
    (human-readable table; commit your change first — it diffs committed trees, not
    the working tree) against the equivalent Terragrunt change-detection output. Use
    `atmos describe affected` instead when scripting/CI needs the JSON/YAML form.
6. **Make the result testable.** Wire the component's stack manifest to an
    `aws/emulator` (or the matching cloud target) identity so `atmos terraform plan`/
    `apply` runs with zero real cloud credentials before the user connects a real
    account. See the [atmos-emulator](../../atmos-emulator/SKILL.md) skill.
7. **Repeat per unit**, using [remote-state-bridge.md](remote-state-bridge.md) to keep
    not-yet-migrated units reachable via `!terraform.state` during the transition.

## When to Escalate to Other Skills

- **Stack organization (orgs, tenants, accounts, regions)** → [atmos-design-patterns](../../atmos-design-patterns/SKILL.md)
- **Vendoring converted module sources** → [atmos-vendoring](../../atmos-vendoring/SKILL.md)
- **Abstract components, inheritance, catalog patterns** → [atmos-components](../../atmos-components/SKILL.md)
- **Deep merging, imports, overrides** → [atmos-stacks](../../atmos-stacks/SKILL.md)
- **Provider credentials, identity chaining** → [atmos-auth](../../atmos-auth/SKILL.md)
- **Local emulator setup for testing the migrated stack** → [atmos-emulator](../../atmos-emulator/SKILL.md)
- **YAML function selection** (`!terraform.output`, `!terraform.state`, `!store`) → [atmos-yaml-functions](../../atmos-yaml-functions/SKILL.md)
- **Hooks beyond packaging** (scanners, cost estimation, custom commands) → [atmos-hooks](../../atmos-hooks/SKILL.md)
- **Progressive, component-by-component migration** → [remote-state-bridge.md](remote-state-bridge.md)

## Anti-Patterns

- **"Terragrunt Stacks needs the `compositions` feature."** No — an ordinary stack
  manifest with `dependencies.components` already covers it. `compositions` solves a
  different problem (grouping components of different kinds for local operation).
- **"Atmos cannot run components in parallel like `run-all --parallelism`."** No —
  `atmos terraform apply --all --max-concurrency N` runs a dependency-ordered
  concurrent scheduler. Confirm the installed Atmos version if `--max-concurrency`
  does not appear in `--help`; it shipped after a long sequential-only period.
- **"Port every `generate` block one-to-one with a `kind: command` hook."** No — check
  whether the block only writes a backend or provider file first (automatic, no
  configuration needed) before reaching for a hook.
- **"There is a `terragrunt://` import scheme that converts `terragrunt.hcl`
  automatically."** No — this is a documented future proposal, not a shipped feature.
  Translation is manual or agent-assisted.

## Additional Resources

- [remote-state-bridge.md](remote-state-bridge.md) — progressive migration technique,
  identical to the native-Terraform case
- [atmos.tools/migration/terragrunt](https://atmos.tools/migration/terragrunt) — the
  full prose tutorial, including complete concept and function mapping tables
