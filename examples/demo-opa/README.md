---
title: OPA Policy Validation
tags: [Validation, OPA]
description: >-
  Enforce component configuration with Open Policy Agent (OPA) policies - block
  provisioning when a stack violates your rules, before any Terraform runs.
related_docs:
  - label: "Open Policy Agent (OPA) validation"
    url: /validation/opa
  - label: "Validate component command"
    url: /cli/commands/validate/component
  - label: "JSON Schema validation"
    url: /validation/json-schema
---

# Example: OPA Policy Validation

Validate the `vpc` component against [OPA (Rego) policies](https://atmos.tools/validation/opa)
before it is ever provisioned. The component is a credential-free mock, so the
only moving part is the policy itself.

## What You'll See

The `vpc` component carries a `settings.validation` section (in
`stacks/catalog/vpc/defaults.yaml`) that runs an OPA policy. The policy and its
shared constants live under `stacks/schemas/opa`:

- `stacks/schemas/opa/vpc/validate-vpc-component.rego` - the policy.
- `stacks/schemas/opa/catalog/constants/constants.rego` - reusable constants and
  error messages imported by the policy.

Two stacks are wired to fail on purpose so you can see each rule fire:

- `dev` sets three Availability Zones and an invalid VPC name.
- `prod` maps public IPs on launch.

## Try It

```shell
# `dev` violates the Availability-Zone-count and VPC-name rules
atmos validate component vpc -s dev

# `prod` forbids mapping public IPs on launch
atmos validate component vpc -s prod

# The same policy can be run explicitly with command-line flags
atmos validate component vpc -s dev \
  --schema-path vpc/validate-vpc-component.rego \
  --schema-type opa \
  --module-paths catalog/constants
```

Each command exits non-zero and prints the policy error messages, which is also
what blocks `atmos terraform apply` for a component that fails validation.
