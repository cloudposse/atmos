# Installing and Pinning Terraform/OpenTofu via the Atmos Toolchain

The Atmos toolchain installs and pins both Terraform and OpenTofu so the same binary version runs
on every developer machine and in CI. Binary **selection** (`command: terraform` vs `command: tofu`)
and binary **version pinning** (`dependencies.tools.terraform` vs `dependencies.tools.opentofu`) are
independent settings that compose:

- `command` says *which* binary Atmos invokes when you run `atmos terraform`.
- `dependencies.tools.<name>` says *which version* of that binary the toolchain installs and uses.

Always pin both together. Setting `command: tofu` without a corresponding `opentofu` dependency
means Atmos invokes whatever `tofu` happens to be on PATH (or fails if there's nothing).

## Project-wide default via `.tool-versions`

Use `.tool-versions` (asdf-compatible) for tools every developer/agent/shell needs by default:

```text
# .tool-versions at repo root
terraform 1.9.8
opentofu 1.10.3
```

Then `atmos toolchain install` provisions both. After install, `atmos terraform plan` uses the
version that matches the configured `command`.

`.tool-versions` is the right place for project-wide defaults. It is NOT the right place when a
specific stack or component requires a different version -- use `dependencies.tools` for that.

## Stack-wide pinning via `dependencies.tools`

Put `dependencies.tools` in a stack default to pin the version for every component in that
stack scope:

```yaml
# stacks/orgs/acme/_defaults.yaml -- applies to all components org-wide
dependencies:
  tools:
    opentofu: "1.10.3"

# stacks/orgs/acme/plat/prod/_defaults.yaml -- prod-only override
dependencies:
  tools:
    opentofu: "1.10.2"      # Keep prod on a known-good slightly older patch
```

This is the right tool when a stack is on OpenTofu but the rest of the org is on Terraform (or
vice versa). Combine with `terraform.overrides.command: tofu` in the same stack default so the
stack switches binary AND version together.

## Per-component pinning

Pin a single component to a specific version when it lags or leads the rest of the project:

```yaml
components:
  terraform:
    legacy-vpc:
      command: terraform
      dependencies:
        tools:
          terraform: "1.5.7"          # Legacy component pinned to older Terraform
    new-eks:
      command: tofu
      dependencies:
        tools:
          opentofu: "1.10.3"          # New component on current OpenTofu
```

Per-component is the right tool during a Terraform → OpenTofu migration where some components
are validated and switched while others remain on Terraform.

## Component-type defaults

Apply a default to every Terraform/OpenTofu component without touching each one:

```yaml
# stacks/.../_defaults.yaml
terraform:
  dependencies:
    tools:
      opentofu: "1.10.3"
```

## Resolution order

When Atmos invokes the configured binary, it resolves the version using standard precedence:
per-component `dependencies.tools` > component-type `terraform.dependencies.tools` > stack
`dependencies.tools` > project `.tool-versions` > whatever's on PATH (last resort).

## Installation workflow

```bash
# Install everything declared in .tool-versions + dependencies.tools across the project
atmos toolchain install

# Install a specific version ad-hoc
atmos toolchain install opentofu@1.10.3
atmos toolchain install terraform@1.9.8

# Verify what's installed
atmos toolchain list

# Pin a version into .tool-versions
atmos toolchain set opentofu@1.10.3
```

## See also

- [SKILL.md](../SKILL.md) -- binary selection (`command: terraform` vs `tofu`) at project, stack,
  component, and per-invocation scopes.
- [../atmos-toolchain/SKILL.md](../../atmos-toolchain/SKILL.md) -- registry configuration,
  custom registries, install paths, package verification, and the full toolchain command reference.
