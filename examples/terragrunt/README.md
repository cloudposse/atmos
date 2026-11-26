---
title: Terragrunt
tags: [Terraform, Components, Stacks]
cast:
  file: /casts/demo/fixtures/terragrunt-native/plan.cast
  title: Discover and plan a Terragrunt unit with Atmos
related_docs:
  - label: Terragrunt with Terraform components
    url: /components/terraform/terragrunt
  - label: Terragrunt migration
    url: /migration/terragrunt
  - label: Plugin usage
    url: /cli/commands/plugin/usage
---

# Terragrunt

Run an existing Terragrunt unit as an Atmos Terraform component. This example uses
OpenTofu, local state, and a built-in `terraform_data` resource. It requires no cloud credentials.

## Discover and plan

With Atmos and OpenTofu installed, run these commands from the example directory:

```shell
atmos list instances
atmos terraform plan demo -s live
```

On first use, `atmos list instances` installs the plugin declared in `atmos.yaml`,
writes `.atmos.plugins.lock.yaml`, and lists component `demo` in stack `live`.
You can also run `atmos plugin install` to install it up front. Commit the lockfile
before running in CI so Atmos can restore the same release.

The plan shows one resource to add. Its input is `HELLO FROM TERRAGRUNT`,
which the unit computes with the HCL `upper` function.

## Apply and clean up

```shell
atmos terraform apply demo -s live
atmos terraform output demo -s live -- -raw greeting
atmos terraform destroy demo -s live
```

## Use in your own project

To add the plugin to your project's `atmos.yaml` and install it, run:

```shell
atmos plugin install oci://ghcr.io/cloudposse/atmos/plugins/terragrunt \
  --as=terragrunt --version='^0.1.0'
```

See [Enable Terragrunt support](https://atmos.tools/components/terraform/terragrunt#enable-terragrunt-support)
to configure discovery for your project's units.
