---
title: Vendoring
description: Use Atmos vendoring to make copies of 3rd-party components, stacks, and other artifacts in your own repo.
sidebar_position: 14
sidebar_label: Vendoring
id: vendoring
---

Atmos natively supports the concept of "vendoring", which is making copies of the 3rd party components, stacks, and other artifacts in your own repo.

The vendoring configuration is defined in the `vendor.yaml` manifest. 
Atmos looks for the `vendor.yaml` file in two different places, and uses the first one found:

- In the directory from which the `atmos vendor pull` command is executed, usually in the root of the infrastructure repo

- In the directory pointed to by the [`base_path`](/cli/configuration#base-path) setting in the [`atmos.yaml`](/cli/configuration) CLI config file

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
  name: example-vendor-config
  description: Atmos vendoring manifest
spec:
  # `imports` or `sources` (or both) must be defined in a vendoring manifest
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
  and other artifacts. The file is placed into the directory from which the `atmos vendor pull` command is executed (usually the root of the repo)

- The `source` attribute supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP), and all URL and
  archive formats as described in [go-getter](https://github.com/hashicorp/go-getter), and also the `oci://` scheme to download artifacts from
  [OCI registries](https://opencontainers.org).

- The `targets` in the `sources` support absolute paths and relative paths (relative to the `vendor.yaml` file). Note: if the `targets` paths
  are set as relative, and if the `vendor.yaml` file is detected by Atmos using the `base_path` setting in `atmos.yaml`, the `targets` paths
  will be considered relative to the `base_path`. Multiple targets can be specified

- `included_paths` and `excluded_paths` support [POSIX-style greedy Globs](https://en.wikipedia.org/wiki/Glob_(programming)) for filenames/paths
  (double-star/globstar `**` is supported as well)

- The `component` attribute in each source is optional. It's used in the `atmos vendor pull -- component <component` command if the component is
  passed in. In this case, Atmos will vendor only the specified component instead of vendoring all the artifacts configured in the `vendor.yaml`
  manifest

- The `source` and `targets` attributes support [Go templates](https://pkg.go.dev/text/template)
  and [Sprig Functions](http://masterminds.github.io/sprig/). This can be used to templatise the `source` and `targets` paths with the artifact
  versions specified in the `version` attribute

- The `imports` section defines the additional vendoring manifests that are merged into the main manifest. Hierarchical imports are supported
  at many levels (one vendoring manifest can import another, which in turn can import other manifests, etc.). Atmos processes all imports and all
  sources in the imported manifests in the order they are defined

## Hierarchical Imports in Vendoring Manifests

Use `imports` to split the main `vendor.yaml` manifest into smaller files for maintainability, or by their roles in the infrastructure.

For example, import separate manifests for networking, security, data management, CI/CD, and other layers:

```yaml
imports:
  - "layers/networking.yaml"
  - "layers/security.yaml"
  - "layers/data.yaml"
  - "layers/analytics.yaml"
  - "layers/firewalls.yaml"
  - "layers/cicd.yaml"
```

Hierarchical imports are supported at many levels. For example, consider the following vendoring configurations:

```yaml title="vendor.yaml"
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example-vendor-config
  description: Atmos vendoring manifest
spec:
  imports:
    - "vendor/vendor2.yaml"
    - "vendor/vendor3.yaml"

  sources:
    - component: "vpc"
      source: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"
      version: "latest"
      targets:
        - "components/terraform/infra/vpc3"
    - component: "vpc-flow-logs-bucket"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}"
      version: "1.323.0"
      targets:
        - "components/terraform/infra/vpc-flow-logs-bucket/{{.Version}}"
```

```yaml title="vendor/vendor2.yaml"
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example-vendor-config-2
  description: Atmos vendoring manifest
spec:
  imports:
    - "vendor/vendor4.yaml"

  sources:
    - component: "my-vpc1"
      source: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"
      version: "1.0.2"
      targets:
        - "components/terraform/infra/my-vpc1"
```

```yaml title="vendor/vendor4.yaml"
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example-vendor-config-4
  description: Atmos vendoring manifest
spec:
  imports:
    - "vendor/vendor5.yaml"

  sources:
    - component: "my-vpc4"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}"
      version: "1.319.0"
      targets:
        - "components/terraform/infra/my-vpc4"
```

When you execute the `atmos vendor pull` command, Atmos processes the import chain and the sources in the imported manifests in the order they
are defined:

- First, the main `vendor.yaml` file is read based on search paths
- The `vendor/vendor2.yaml` and `vendor/vendor3.yaml` manifests (defined in the main `vendor.yaml` file) are imported
- The `vendor/vendor2.yaml` file is processed, and the `vendor/vendor4.yaml` manifest is imported
- The `vendor/vendor4.yaml` file is processed, and the `vendor/vendor5.yaml` manifest is imported
- Etc.
- Then all the sources from all the imported manifests are processed and the artifacts are downloaded into the paths defined by the `targets`

<br/>

```bash
> atmos vendor pull

Processing vendor config file 'vendor.yaml'
Pulling sources for the component 'my-vpc6' from 'github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.315.0' into 'components/terraform/infra/my-vpc6'
Pulling sources for the component 'my-vpc5' from 'github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.317.0' into 'components/terraform/infra/my-vpc5'
Pulling sources for the component 'my-vpc4' from 'github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.319.0' into 'components/terraform/infra/my-vpc4'
Pulling sources for the component 'my-vpc1' from 'public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:1.0.2' into 'components/terraform/infra/my-vpc1'
Pulling sources for the component 'my-vpc2' from 'github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.320.0' into 'components/terraform/infra/my-vpc2'
Pulling sources for the component 'vpc' from 'public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:latest' into 'components/terraform/infra/vpc3'
Pulling sources for the component 'vpc-flow-logs-bucket' from 'github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref=1.323.0' into 'components/terraform/infra/vpc-flow-logs-bucket/1.323.0'

```

## Vendoring from OCI Registries

Atmos supports vendoring from [OCI registries](https://opencontainers.org).

To specify a repository in an OCI registry, use the `oci://<registry>/<repository>:tag` scheme.

Artifacts from OCI repositories are downloaded as Docker image tarballs, then all the layers are processed, un-tarred and un-compressed,
and the files are written into the directories specified by the `targets` attribute of each `source`.

For example, to vendor the `vpc` component from the `public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc`
[AWS public ECR registry](https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html), use the following `source`:

```yaml
source: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:latest"
```

The schema of a `vendor.yaml` manifest is as follows:

```yaml
# This is an example of how to download a Terraform component from an OCI registry (https://opencontainers.org), e.g. AWS Public ECR

apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example-vendor-config
  description: Atmos vendoring manifest
spec:
  sources:
    - component: "vpc"
      source: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"
      version: "latest"
      targets:
        - "components/terraform/infra/vpc3"
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
