---
title: Component Vendoring
sidebar_position: 3
sidebar_label: Vendoring
description: Use Component Vendoring to make copies of 3rd-party components in your own repo.
id: vendoring
---

Atmos natively supports the concept of "vendoring", which is making a copy of 3rd-party components or other dependencies in your own repo. 
Our implementation is primarily inspired by the excellent tool by VMware Tanzu, called [`vendir`](https://github.com/vmware-tanzu/carvel-vendir).
While Atmos does not call `vendir`, it functions and supports a subset of the configuration that is very similar.

## Use-cases

Atmos vendoring streamlines component sharing and version control across an enterprise, enhancing efficiency and collaboration while offering the flexibility to customize and manage multiple versions of dependencies, ensuring best practices in DevOps environments.

- **Sharing Components Across an Enterprise**: Utilize Atmos vendoring to access a centralized component library, promoting code reuse and 
  efficiency across teams (or business units) while enabling customization and independent version control post-vendoring. This approach enhances collaboration without sacrificing the flexibility for teams to tailor components to their specific needs or update them at their preferred pace.
- **Managing Multiple Versions of Dependencies:** Use Atmos vendoring to manage multiple versions of remote dependencies, 
  effectively implementing version pinning through locally controlled artifacts. By configuring a stacks component directory (e.g., `vpc/v1` or `vpc/v2`), vendoring provides maximum flexibility while still aligning with best practices in DevOps environments.
- **Reinforce Immutable Infrastructure**: Employ Atmos vendoring to store immutable infrastructure artifacts, guaranteeing that once a committed,
  it remains unaltered throughout its lifecycle, ensuring stability and reliability in deployments.

## Usage

Atmos supports two different ways of vendoring components:

- Using `vendor.yaml` vendoring manifest file containing a list of all dependencies. See [Vendoring](/core-concepts/vendoring/).
- Using `component.yaml` manifest file inside of a component directory. See below.

The `vendor.yaml` vendoring manifest describes the vendoring config for all components, stacks and other artifacts for the entire infrastructure.
The file is placed into the directory from which the `atmos vendor pull` command is executed. It's the recommended way to describe vendoring
configurations.

:::tip
Refer to [`Atmos Vendoring`](/core-concepts/vendoring) for more details
:::

<br/>

The `component.yaml` vendoring manifest is used to vendor components from remote repositories.
A `component.yaml` file placed into a component's directory is used to describe the vendoring config for one component only.

## Examples

### Vendoring using `component.yaml` manifest

After defining the `component.yaml` vendoring manifest, the remote component can be downloaded by running the following command:

```bash
atmos vendor pull -c components/terraform/vpc
```

:::tip
Refer to [`atmos vendor pull`](/cli/commands/vendor/pull) CLI command for more details
:::

### Vendoring Components from a Monorepo

To vendor a component, create a `component.yaml` file stored inside the `components/_type_/_name_/` folder (e.g. `components/terraform/vpc/`).

The schema of a `component.yaml` file is as follows:

```yaml
apiVersion: atmos/v1
kind: ComponentVendorConfig
metadata:
  name: vpc-flow-logs-bucket-vendor-config
  description: Source and mixins config for vendoring of 'vpc-flow-logs-bucket' component
spec:
  source:
    # Source 'uri' supports the following protocols: OCI (https://opencontainers.org), Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP,
    # and all URL and archive formats as described in https://github.com/hashicorp/go-getter
    # In 'uri', Golang templates are supported  https://pkg.go.dev/text/template
    # If 'version' is provided, '{{.Version}}' will be replaced with the 'version' value before pulling the files from 'uri'
    # To vendor a module from a Git repo, use the following format: 'github.com/cloudposse/terraform-aws-ec2-instance.git///?ref={{.Version}}
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}
    version: 1.398.0

    # Only include the files that match the 'included_paths' patterns
    # If 'included_paths' is not specified, all files will be matched except those that match the patterns from 'excluded_paths'
    # 'included_paths' support POSIX-style Globs for file names/paths (double-star/globstar `**` is supported)
    # https://en.wikipedia.org/wiki/Glob_(programming)
    # https://github.com/bmatcuk/doublestar#patterns
    included_paths:
      - "**/*.tf"
      - "**/*.tfvars"
      - "**/*.md"

    # Exclude the files that match any of the 'excluded_paths' patterns
    # Note that we are excluding 'context.tf' since a newer version of it will be downloaded using 'mixins'
    # 'excluded_paths' support POSIX-style Globs for file names/paths (double-star/globstar `**` is supported)
    excluded_paths:
      - "**/context.tf"

  # Mixins override files from 'source' with the same 'filename' (e.g. 'context.tf' will override 'context.tf' from the 'source')
  # All mixins are processed in the order they are declared in the list.
  mixins:
    # https://github.com/hashicorp/go-getter/issues/98
    - uri: https://raw.githubusercontent.com/cloudposse/terraform-null-label/0.25.0/exports/context.tf
      filename: context.tf
    - uri: https://raw.githubusercontent.com/cloudposse/terraform-aws-components/{{.Version}}/modules/datadog-agent/introspection.mixin.tf
      version: 1.398.0
      filename: introspection.mixin.tf
```

<br/>

:::warning

The `glob` library that Atmos uses to download remote artifacts does not treat the double-star `**` as including sub-folders.
If the component's folder has sub-folders, and you need to vendor them, they have to be explicitly defined as in the following example.

:::

```yaml title="component.yaml"
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}
    version: 1.398.0
    included_paths:
      - "**/**"
      # If the component's folder has the `modules` sub-folder, it needs to be explicitly defined
      - "**/modules/**"
```

<br/>

### Vendoring Modules as Components

Any terraform module can also be used as a component, provided that Atmos backend
generation ([`auto_generate_backend_file` is `true`](/cli/configuration/#components)) is enabled. Use this strategy when you want to use the module
directly, without needing to wrap it in a component to add additional functionality. This is essentially treating a terraform child module as a root
module.

To vendor a module as a component, simply create a `component.yaml` file stored inside the `components/_type_/_name_/` folder
(e.g. `components/terraform/ec2-instance/component.yaml`).

The schema of a `component.yaml` file for a module is as follows.
Note the usage of the `///` in the `uri`, which is to vendor from the root of the remote repository.

```yaml
apiVersion: atmos/v1
kind: ComponentVendorConfig
metadata:
  name: ec2-instance
  description: Source for vendoring of 'ec2-instance' module as a component
spec:
  source:
    # To vendor a module from a Git repo, use the following format: 'github.com/cloudposse/terraform-aws-ec2-instance.git///?ref={{.Version}}
    uri: github.com/cloudposse/terraform-aws-ec2-instance.git///?ref={{.Version}}
    version: 0.47.1

    # Only include the files that match the 'included_paths' patterns
    # 'included_paths' support POSIX-style Globs for file names/paths (double-star/globstar `**` is supported)
    included_paths:
      - "**/*.tf"
      - "**/*.tfvars"
      - "**/*.md"
```

### Vendoring Components from OCI Registries

Atmos supports vendoring components from [OCI registries](https://opencontainers.org).

To specify a repository in an OCI registry, use the `oci://<registry>/<repository>:tag` scheme in the `sources` and `mixins`.

Components from OCI repositories are downloaded as Docker image tarballs, then all the layers are processed, un-tarred and un-compressed,
and the component's source files are written into the component's directory.

For example, to vendor the `vpc` component from the `public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc`
[AWS public ECR registry](https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html), use the following `uri`:

```yaml
uri: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:latest"
```

The schema of a `component.yaml` file is as follows:

```yaml
# This is an example of how to download a Terraform component from an OCI registry (https://opencontainers.org), e.g. AWS Public ECR

# 'component.yaml' in the component folder is processed by the 'atmos' commands:
# 'atmos vendor pull -c infra/vpc' or 'atmos vendor pull --component infra/vpc'

apiVersion: atmos/v1
kind: ComponentVendorConfig
metadata:
  name: stable/aws/vpc
  description: Config for vendoring of the 'stable/aws/vpc' component
spec:
  source:
    # Source 'uri' supports the following protocols: OCI (https://opencontainers.org), Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP,
    # and all URL and archive formats as described in https://github.com/hashicorp/go-getter
    # In 'uri', Golang templates are supported  https://pkg.go.dev/text/template
    # If 'version' is provided, '{{.Version}}' will be replaced with the 'version' value before pulling the files from 'uri'
    # Download the component from the AWS public ECR registry (https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html)
    uri: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"
    version: "latest"
    # Only include the files that match the 'included_paths' patterns
    # If 'included_paths' is not specified, all files will be matched except those that match the patterns from 'excluded_paths'
    # 'included_paths' support POSIX-style Globs for file names/paths (double-star `**` is supported)
    # https://en.wikipedia.org/wiki/Glob_(programming)
    # https://github.com/bmatcuk/doublestar#patterns
    included_paths:
      - "**/*.*"
```
