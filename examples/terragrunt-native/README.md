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

## Install the plugin

The Terragrunt configuration provider is a plugin package that Atmos installs from the registry.
`atmos.yaml` declares it with a source and a version constraint:

```yaml
plugins:
  packages:
    terragrunt:
      source: oci://ghcr.io/cloudposse/atmos/plugins/terragrunt
      version: "^0.1.0"
```

Install Atmos and OpenTofu, copy the example into a new directory, and install the plugin:

```shell
atmos init ./examples/terragrunt-native ./terragrunt-native \
  --copy --no-git
cd terragrunt-native
atmos plugin install
```

`atmos plugin install` downloads the release, verifies its digest, and writes it to
`.atmos.plugins.lock.yaml`. Commit that file: the lock pins the exact release, so every machine
and CI run restores the same bytes, and CI refuses to create a new lock entry on its own.
Without this step the first command that needs the plugin installs it and creates the lock entry;
`atmos plugin install --frozen` restores exactly what the lock records and never changes it.
If the package cannot be fetched (for example because the registry is unreachable or the release is
not yet published), Atmos reports which source and version constraint it could not prepare and
changes nothing.

To enable the same behavior in an existing project, run these commands from that project's root:

```shell
atmos plugin install oci://ghcr.io/cloudposse/atmos/plugins/terragrunt \
  --as=terragrunt
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

Adjust `included_paths` to match your Terragrunt directory layout;
see [discovery paths](https://atmos.tools/migration/terragrunt#discovery-paths) for the
pattern rules. `components.terraform.base_path` is required by Atmos even though
Terragrunt units do not live there; the directory does not need to exist.
If you use Terraform instead, set `components.terraform.command` to `terraform`
and install Terraform 1.4 or newer.

## Develop against a local build

Only plugin developers need this. The `dev` profile replaces the registry package with a local
build, which is never fetched, locked, or signature-checked. From the root of the Atmos checkout
containing this example:

```shell
go build -o build/atmos .
export PATH="$PWD/build:$PATH"
atmos plugin build plugins/terragrunt
export ATMOS_TERRAGRUNT_PLUGIN_PATH="$PWD/plugins/terragrunt"
export ATMOS_PROFILE=dev
```

Run the commands below from the copied example directory with `ATMOS_PROFILE=dev` set.
`profiles/dev/atmos.yaml` reads the plugin path from `ATMOS_TERRAGRUNT_PLUGIN_PATH`.

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
