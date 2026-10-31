---
title: Native Terragrunt
tags: [Components, Stacks]
related_docs:
  - label: Terragrunt migration
    url: /migration/terragrunt
  - label: Plugin usage
    url: /cli/commands/plugin/usage
---

# Native Terragrunt

Use an existing Terragrunt HCL unit as an Atmos [Terraform component](https://atmos.tools/components/terraform) without adding
Atmos stack YAML. The example discovers component `demo` in stack `live`, evaluates
its HCL with Terragrunt Go modules, and executes OpenTofu through Atmos. It does not
run the Terragrunt executable.

The module uses the built-in `terraform_data` resource and local state. No cloud
credentials or provider downloads are needed.

## Build the development package

This example requires the experimental Atmos branch and its local Terragrunt
plugin. See [plugin usage](https://atmos.tools/cli/commands/plugin/usage) for package configuration. The plugin is not yet published. Install Go and OpenTofu, then run these
commands from the root of the Atmos checkout containing this example:

```shell
go build -o build/atmos .
export PATH="$PWD/build:$PATH"
atmos plugin build plugins/terragrunt
export ATMOS_TERRAGRUNT_PLUGIN_PATH="$PWD/plugins/terragrunt"
```

Copy the example into a new directory and select its `dev` profile, which points the
plugin package at your build:

```shell
atmos init ./examples/terragrunt-native ./terragrunt-native \
  --copy --no-git
cd terragrunt-native
export ATMOS_PROFILE=dev
```

The example already enables native discovery, and `profiles/dev/atmos.yaml` reads the
plugin path from `ATMOS_TERRAGRUNT_PLUGIN_PATH`. To enable the same behavior in an
existing project, run these commands from that project's root:

```shell
atmos config set plugins.packages.terragrunt.path \
  "$ATMOS_TERRAGRUNT_PLUGIN_PATH"
atmos config set components.terraform.base_path \
  components/terraform
atmos config set components.terraform.command tofu
atmos config set components.terraform.terragrunt.enabled true
atmos config set components.terraform.terragrunt.mode native
atmos config set --type=yaml \
  components.terraform.terragrunt.discovery.included_paths \
  '["live/**"]'
atmos config set \
  components.terraform.terragrunt.discovery.enabled true
```

In an existing project the plugin path is stored in `atmos.yaml`, so Atmos does not
need the environment variable on subsequent runs. Adjust `included_paths` to match your Terragrunt directory layout;
see [discovery paths](https://atmos.tools/migration/terragrunt#discovery-paths) for the
pattern rules. `components.terraform.base_path` is required by Atmos even though
Terragrunt units do not live there; the directory does not need to exist.
If you use Terraform instead, set `components.terraform.command` to `terraform`
and install Terraform 1.4 or newer.

## Discover and plan

```shell
atmos list stacks
atmos list instances
atmos terraform plan demo -s live
```

The plan contains one `terraform_data.demo` resource with the input
`HELLO FROM TERRAGRUNT`, evaluated by HCL's `upper` function. Atmos prepares an isolated workdir; the source HCL and
module stay in place. The local backend retains the unit's state path at
`live/demo/state/terraform.tfstate`.

## Apply and clean up

```shell
atmos terraform apply demo -s live -- -auto-approve
atmos terraform output demo -s live -- -raw greeting
atmos terraform plan demo -s live -- -detailed-exitcode
atmos terraform destroy demo -s live -- -auto-approve
```

After apply, output prints the greeting and the unchanged plan exits with code
zero. Destroy removes the local test resource. This example demonstrates one local
unit; it does not establish compatibility with all Terragrunt constructs.
