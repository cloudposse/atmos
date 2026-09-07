# Terraform Component Mocks

## Problem

Atmos YAML functions such as `!terraform.state` and `!terraform.output` normally require the referenced component to have real Terraform output or remote state. This makes local configuration inspection and planning difficult when a dependency has not been deployed or credentials are unavailable.

## Goals

- Let a Terraform component declare literal output values under `mocks`.
- Resolve those values only when the caller explicitly passes `--use-mocks`.
- Support the existing `!terraform.state` and `!terraform.output` argument and YQ-expression syntax.
- Keep normal commands unchanged and prevent mock values from reaching mutating Terraform operations.

## Non-goals

- Mocking Terraform providers, resources, data sources, stores, secrets, or other Atmos YAML functions.
- Named mock profiles, wildcard matching, consumer-local overrides, or synthetic output generation.
- Evaluating templates or YAML functions inside mock values.

## Configuration and CLI contract

Declare mocks on the Terraform component that normally produces the outputs:

```yaml
components:
  terraform:
    vpc:
      mocks:
        vpc_id: vpc-local
        private_subnet_ids: [subnet-a, subnet-b]
```

Use them explicitly:

```shell
atmos terraform plan app -s dev --use-mocks
atmos describe component app -s dev --use-mocks
```

With `--use-mocks`, both `!terraform.state vpc vpc_id` and `!terraform.output vpc vpc_id` resolve from `components.terraform.vpc.mocks`. The map participates in normal component inheritance and deep merging; the most-specific value wins.

`--use-mocks` requires YAML function processing and is supported only by `terraform plan` and `describe component`. Terraform apply, deploy, destroy, and passthrough commands reject the flag before stack resolution.

## Resolution and errors

When mock mode is active, Atmos loads the referenced component with template and YAML-function processing disabled, evaluates the requested output expression against its literal `mocks` map, and returns the result. It does not initialize Terraform, authenticate, read a backend, or use the Terraform state/output caches.

Mock mode is fail-closed: an absent `mocks` map or an undeclared direct output is an error. Atmos does not fall back to real state, because doing so would make an explicit mock invocation non-hermetic. Explicit `null` mock values remain valid. Existing YQ defaults continue to work against the mock map.

## Security and rollout

The feature is opt-in and excluded from mutating Terraform commands, so production apply behavior is unchanged. Literal mock values are still configuration data: users must not put secrets in them unless their normal repository controls permit it.

Roll out with `describe component --use-mocks` and local `terraform plan --use-mocks` first. The provider-free example in `examples/terraform-component-mocks` demonstrates the intended workflow.
