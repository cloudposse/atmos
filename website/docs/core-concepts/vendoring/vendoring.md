---
title: Vendoring
description: Use Atmos vendoring to make copies of 3rd-party components, stacks, and other artifacts in your own repo.
sidebar_position: 14
sidebar_label: Vendoring
id: vendoring
---

Atmos natively supports the concept of "vendoring", which is making copies of the 3rd party components, stacks, and other artifacts in your own repo.

The vendoring configuration is described in the `vendor.yaml` manifest, which should be placed in the directory from which the `atmos vendor pull`
command is executed, usually in the root of the infrastructure repo.

After defining the `vendor.yaml` manifest, all the remote artifacts can be downloaded by running the following command:

```bash
atmos vendor pull
```

To vendor a particular component or other artifact, execute the following command:

```bash
atmos vendor pull -c <component>
```

<br/>

:::tip
Refer to [`atmos vendor pull`](/cli/commands/vendor/pull) CLI command for more details
:::

## Vendoring Manifest

To vendor remote artifacts, create a `vendor.yaml` file similar to the example below:

```yaml
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: atmos-vendor-config
  description: Atmos vendoring configuration
spec:
  # Either `imports` or `sources` (or both) must be defined in a vendor config file
  imports:
    - "vendor/vendor2.yaml"
    - "vendor/vendor3.yaml"

  sources:
    # `source` supports the following protocols: OCI (https://opencontainers.org), Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP,
    # and all URL and archive formats as described in https://github.com/hashicorp/go-getter.
    # In 'source', Golang templates are supported  https://pkg.go.dev/text/template.
    # If 'version' is provided, '{{.Version}}' will be replaced with the 'version' value before pulling the files from 'source'.
    # Download the component from the AWS public ECR registry (https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html).
    - component: "vpc"
      source: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"
      version: "latest"
      targets:
        - "components/terraform/infra/vpc3"
      # Only include the files that match the 'included_paths' patterns.
      # If 'included_paths' is not specified, all files will be matched except those that match the patterns from 'excluded_paths'.
      # 'included_paths' support POSIX-style Globs for file names/paths (double-star `**` is supported).
      # https://en.wikipedia.org/wiki/Glob_(programming)
      # https://github.com/bmatcuk/doublestar#patterns
      included_paths:
        - "**/*.tf"
        - "**/*.tfvars"
        - "**/*.md"
    - component: "vpc-flow-logs-bucket"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}"
      version: "1.323.0"
      targets:
        - "components/terraform/infra/vpc-flow-logs-bucket/{{.Version}}"
      excluded_paths:
        - "**/*.yaml"
        - "**/*.yml"
```

<br/>

- The `vendor.yaml` vendoring manifest supports Kubernetes-style YAML config to describe vendoring configuration for components, stacks,
  and other artifacts

- The `sources` in `vendor.yaml` support all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP), and all URL and
  archive formats as described in [go-getter](https://github.com/hashicorp/go-getter), and also the `oci://` scheme to download artifacts from
  [OCI registries](https://opencontainers.org).

- The `targets` in the sources support absolute and relative paths (relative to the directory where the command is executed)

- `included_paths` and `excluded_paths` support [POSIX-style greedy Globs](https://en.wikipedia.org/wiki/Glob_(programming)) for filenames/paths
  (double-star/globstar `**` is supported as well)

- The `component` attribute in each source is optional. It's used in the `atmos vendor pull -- component <component` command if the component is
  passed in. In this case, Atmos will vendor only the specified component instead of vendoring all the artifacts configured in the `vendor.yaml`
  manifest

- The `source` and `targets` attributes support [Go templates](https://pkg.go.dev/text/template)
  and [Sprig Functions](http://masterminds.github.io/sprig/). This can be used to templatise the `source` and `targets` paths with the artifact
  version defined in the `version` attribute

- The `imports` section defines the additional vendoring manifests that are merged into the main manifest. Hierarchical imports are supported
  at many levels (one vendoring manifest can import another, which in turn can import other manifests, etc.). Atmos processes all imports and all 
  sources in the imported manifests in the order they are defined. Use `imports` to split the main `vendor.yaml` manifest into smaller files for 
  maintainability, or by their roles in the infrastructure (e.g. import separate manifest for networking, security, data management, etc.)

## Vendoring from OCI Registries

Atmos supports vendoring from [OCI registries](https://opencontainers.org).

To specify a repository in an OCI registry, use the `oci://<registry>/<repository>:tag` scheme.

Artifacts from OCI repositories are downloaded as Docker image tarballs, then all the layers are processed, un-tarred and un-compressed,
and the files are written into the directories specified by the `targets` attribute of each `source`.

For example, to vendor the `vpc` component from the `public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc`
[AWS public ECR registry](https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html), use the following `uri`:

```yaml
uri: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:latest"
```

The schema of a `vendor.yaml` manifest is as follows:

```yaml
# This is an example of how to download a Terraform component from an OCI registry (https://opencontainers.org), e.g. AWS Public ECR

apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: atmos-vendor-config
  description: Atmos vendoring configuration
spec:
  sources:
    - component: "vpc"
      source: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"
      version: "latest"
      targets:
        - "components/terraform/infra/vpc3"
      # Only include the files that match the 'included_paths' patterns.
      # If 'included_paths' is not specified, all files will be matched except those that match the patterns from 'excluded_paths'.
      # 'included_paths' and 'excluded_paths' support POSIX-style Globs for file names/paths (double-star `**` is supported).
      included_paths:
        - "**/*.tf"
        - "**/*.tfvars"
        - "**/*.md"
      excluded_paths: [ ]
```

To vendor the `vpc` component, execute the following command:

```bash
atmos vendor pull -c vpc
```
