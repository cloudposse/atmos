# Terraform Module Source Interpolation Test Fixture

This test fixture reproduces and validates the fix for [issue #2913](https://github.com/cloudposse/atmos/issues/2913),
which addresses Terraform 1.15+ `const`-variable module source interpolation support in Atmos.

## Problem Statement

Terraform 1.15+ introduced the ability to use variable interpolation in module sources, provided the
variable is declared `const = true`:

```hcl
variable "org" {
  const   = true
  type    = string
  default = "acme"
}

module "greeting" {
  source = "./mods/${var.org}"
}
```

Atmos's `terraform-config-inspect`-based validation phase rejects this syntax before any Terraform
command is ever invoked, because that library decodes `module.source` with a `nil` `hcl.EvalContext`
and cannot evaluate any variable reference -- regardless of whether the configured tool/version
actually supports it.

Atmos already tolerated this exact diagnostic for OpenTofu 1.8+ (see
`tests/fixtures/scenarios/opentofu-module-source-interpolation/`), but only when the configured
command was detected as OpenTofu. Since this fixture uses plain `terraform` (no `command:` override
in `atmos.yaml`), it previously failed with:

```
failed to load terraform component
The Terraform component 'test-component' contains invalid HCL code at .../main.tf:14.
Variables not allowed: Variables may not be used here.
```

## Solution

The skip is no longer gated on OpenTofu detection -- it applies to this known diagnostic pattern
regardless of which tool (`terraform` or `tofu`) is configured, since the diagnostic is a static-parser
limitation, not a tool-specific feature gate.

## Expected Behavior

```bash
atmos describe component test-component -s test
```

Should return the component configuration with `vars.org: acme`, and no validation error, even though
no `command:` override is set (i.e. this exercises plain Terraform, not OpenTofu).

## References

- Issue: https://github.com/cloudposse/atmos/issues/2913
- Terraform 1.15 release: https://www.hashicorp.com/en/blog/new-in-terraform-115-dynamic-sources-variable-deprecation-and-more
- PRD: `docs/prd/opentofu-module-source-interpolation.md`
