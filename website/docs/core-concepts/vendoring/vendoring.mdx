---
title: Vendoring
description: Use Atmos vendoring to make copies of 3rd-party components, stacks, and other artifacts in your own repo.
sidebar_position: 14
sidebar_label: Vendoring
id: vendoring
---

Atmos natively supports "vendoring," a practice that involves replicating 3rd-party components, stacks, and artifacts within your own repository.
This feature is particularly beneficial for managing dependencies in software like Terraform, which do not support pulling root modules remotely
by configuration. Vendoring standardizes dependency management, encourages enterprise component reuse, and ensures compliance standards adherence. Furthermore, it allows teams to customize and independently manage their vendored components according to their specific requirements.

## Use-cases

Use vendoring to maintain a local copy of external dependencies critical for managing your infrastructure. Organize the dependencies in the manner that best suits your project's structure. Even create multiple vendor manifests, for example, tailored to specific layers, products, or teams. Then easily
update those dependencies by bumping the versions in the vendor manifest.

- **Managing Third-Party Dependencies**: Use vendoring in Atmos to efficiently manage and version-control third-party Terraform components, modules or other infrastructure dependencies. This approach is crucial for teams relying on external libraries, root modules, and configurations from sources such as Git repositories. Vendoring these dependencies into your project repository ensures that every team member and CI/CD pipeline works with the same dependency versions, enhancing consistency and reliability across development, testing, and production environments.
- **Sharing Components Across an Enterprise**: Utilize Atmos vendoring to access a centralized component library, promoting code reuse and efficiency across teams while enabling customization and independent version control post-vendoring. This approach enhances collaboration without sacrificing the flexibility for teams to tailor components to their specific needs or update them at their preferred pace.
- **Maintaining Immutable Artifacts for Compliance**: Employ vendoring through Atmos to maintain local, immutable copies of remote dependencies, essential for meeting compliance and regulatory requirements. Keeping a local version-controlled artifact of dependencies ensures that your infrastructure complies with policies that mandate a record of all external components used within the system. This practice supports auditability and traceability, key aspects of maintaining a secure and compliant infrastructure.
- **Overcoming Tooling Limitations with Remote Dependencies**: Utilize Atmos vendoring as a practical solution when your tooling lacks native support for managing remote dependencies. By copying these dependencies into your project repository, you can work around these limitations, ensuring that your infrastructure can still leverage essential external modules and configurations. This approach allows for greater flexibility in infrastructure management, adapting to tooling constraints while still benefiting from the broad ecosystem of available infrastructure modules and configurations.
- **Optimize Boilerplate Code Reusability with Vendoring** Developers can utilize Atmos vendoring together with components to consolidate code by sourcing mixins (e.g. `providers.tf`, `context.tf`, etc) and boilerplate from centralized locations, streamlining development with DRY principles and immutable infrastructure.

:::tip Pro Tip! Use GitOps
Vendoring plays nicely with GitOps practices, especially when leveraging [GitHub Actions](/integrations/github-actions/).
Use a workflow that automatically updates the vendor manifest and opens a pull request (PR) with all the changes.
This allows you to inspect and precisely assess the impact of any upgrades before merging by reviewing the job summary of the PR.
:::

## Features

With Atmos vendoring, you can copy components and other artifacts from the following sources:

- Copy all files from an [OCI Registry](https://opencontainers.org) into a local folder
- Copy all files from Git, Mercurial, Amazon S3, Google GCP into a local folder
- Copy all files from an HTTP/HTTPS endpoint into a local folder
- Copy a single file from an HTTP/HTTPS endpoint to a local file
- Copy a local file into a local folder (keeping the same file name)
- Copy a local file to a local file with a different file name
- Copy a local folder (all files) into a local folder

The vendoring configuration is defined in the `vendor.yaml` manifest (vendor config file).

## How it works

Atmos searches for the vendoring manifest in the following locations, and uses the first one found:

- In the directory from which the [`atmos vendor pull`](/cli/commands/vendor/pull) command is executed, usually in the root of the infrastructure repo

- In the directory pointed to by the [`base_path`](/cli/configuration#base-path) setting in the [`atmos.yaml`](/cli/configuration) CLI config file

After defining the `vendor.yaml` manifest, all the remote artifacts can be downloaded by running the following command:

```bash
atmos vendor pull
```

To vendor a particular component or other artifact, execute the following command:

```bash
atmos vendor pull -c <component>
```

To vendor components and artifacts tagged with specific tags, execute the following command:

```bash
atmos vendor pull --tags <tag1>,<tag2>
```

<br/>

:::tip
Refer to [`atmos vendor pull`](/cli/commands/vendor/pull) CLI command for more details
:::

## Vendoring Manifest

To vendor remote artifacts, create a `vendor.yaml` file similar to the example below:

```yaml title="vendor.yaml"
# atmos vendor pull
# atmos vendor pull --component vpc-mixin-1
# atmos vendor pull -c vpc-mixin-2
# atmos vendor pull -c vpc-mixin-3
# atmos vendor pull -c vpc-mixin-4
# atmos vendor pull --tags test
# atmos vendor pull --tags networking,storage

apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example-vendor-config
  description: Atmos vendoring manifest
spec:
  # `imports` or `sources` (or both) must be defined in a vendoring manifest
  imports:
    - "vendor/vendor2"
    - "vendor/vendor3.yaml"

  sources:
    # `source` supports the following protocols: local paths (absolute and relative), OCI (https://opencontainers.org),
    # Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP,
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
      # Tags can be used to vendor component that have the specific tags
      # `atmos vendor pull --tags test`
      # Refer to https://atmos.tools/cli/commands/vendor/pull
      tags:
        - test
        - networking
    - component: "vpc-flow-logs-bucket"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}"
      version: "1.323.0"
      targets:
        - "components/terraform/infra/vpc-flow-logs-bucket/{{.Version}}"
      excluded_paths:
        - "**/*.yaml"
        - "**/*.yml"
      # Tags can be used to vendor component that have the specific tags
      # `atmos vendor pull --tags networking,storage`
      # Refer to https://atmos.tools/cli/commands/vendor/pull
      tags:
        - test
        - storage
    - component: "vpc-mixin-1"
      source: "https://raw.githubusercontent.com/cloudposse/terraform-null-label/0.25.0/exports/context.tf"
      targets:
        - "components/terraform/infra/vpc3"
      # Tags can be used to vendor component that have the specific tags
      # `atmos vendor pull --tags test`
      # Refer to https://atmos.tools/cli/commands/vendor/pull
      tags:
        - test
    - component: "vpc-mixin-2"
      # Copy a local file into a local folder (keeping the same file name)
      # This `source` is relative to the current folder
      source: "components/terraform/mixins/context.tf"
      targets:
        - "components/terraform/infra/vpc3"
      # Tags can be used to vendor component that have the specific tags
      # `atmos vendor pull --tags test`
      # Refer to https://atmos.tools/cli/commands/vendor/pull
      tags:
        - test
    - component: "vpc-mixin-3"
      # Copy a local folder into a local folder
      # This `source` is relative to the current folder
      source: "components/terraform/mixins"
      targets:
        - "components/terraform/infra/vpc3"
      # Tags can be used to vendor component that have the specific tags
      # `atmos vendor pull --tags test`
      # Refer to https://atmos.tools/cli/commands/vendor/pull
      tags:
        - test
    - component: "vpc-mixin-4"
      # Copy a local file into a local file with a different file name
      # This `source` is relative to the current folder
      source: "components/terraform/mixins/context.tf"
      targets:
        - "components/terraform/infra/vpc3/context-copy.tf"
      # Tags can be used to vendor component that have the specific tags
      # `atmos vendor pull --tags test`
      # Refer to https://atmos.tools/cli/commands/vendor/pull
      tags:
        - test
```

<br/>

- The `vendor.yaml` vendoring manifest supports Kubernetes-style YAML config to describe vendoring configuration for components, stacks,
  and other artifacts. The file is placed into the directory from which the `atmos vendor pull` command is executed (usually the root of the repo).

- The `source` attribute supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP), and all the URL and
  archive formats as described in [go-getter](https://github.com/hashicorp/go-getter), and also the `oci://` scheme to download artifacts from
  [OCI registries](https://opencontainers.org).

  **IMPORTANT:** Include the `{{ .Version }}` parameter in your `source` URI to ensure the correct version of the artifact is downloaded.
  For example:

  ```yaml
  source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}"
  ```

- The `targets` in each source supports absolute paths and relative paths (relative to the `vendor.yaml` file). Note: if the `targets` paths
  are set as relative, and if the `vendor.yaml` file is detected by Atmos using the `base_path` setting in `atmos.yaml`, the `targets` paths
  will be considered relative to the `base_path`. Multiple targets can be specified.

- `included_paths` and `excluded_paths` support [POSIX-style greedy Globs](https://en.wikipedia.org/wiki/Glob_(programming)) for filenames/paths
  (double-star/globstar `**` is supported as well).

- The `component` attribute in each source is optional. It's used in the `atmos vendor pull -- component <component>` command if the component is
  passed in. In this case, Atmos will vendor only the specified component instead of vendoring all the artifacts configured in the `vendor.yaml`
  manifest.

- The `source` and `targets` attributes support [Go templates](https://pkg.go.dev/text/template)
  and [Sprig Functions](http://masterminds.github.io/sprig/). This can be used to templatise the `source` and `targets` paths with the artifact
  versions specified in the `version` attribute.

  Here's an advanced example showcasing how templates and Sprig functions can be used together with `targets`:

  ```yaml
  targets:
    # Vendor a component into a major-minor versioned folder like 1.2
    - "components/terraform/infra/vpc-flow-logs-bucket/{{ (first 2 (splitList \".\" .Version)) | join \".\" }}"
  ```

- The `tags` in each source specifies a list of tags to apply to the component. This allows you to only vendor the components that have the
  specified tags by executing a command `atmos vendor pull --tags <tag1>,<tag2>`

- The `imports` section defines the additional vendoring manifests that are merged into the main manifest. Hierarchical imports are supported
  at many levels (one vendoring manifest can import another, which in turn can import other manifests, etc.). Atmos processes all imports and all
  sources in the imported manifests in the order they are defined.

  **NOTE:** The imported file extensions are optional. If an import is defined without an extension, the `.yaml` extension is assumed and used
  by default.

<br/>

:::warning

The `glob` library that Atmos uses to download remote artifacts does not treat the double-star `**` as including sub-folders.
If the component's folder has sub-folders, and you need to vendor them, they have to be explicitly defined as in the following example.

:::

```yaml title="vendor.yaml"
spec:
  sources:
    - component: "vpc-flow-logs-bucket"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}"
      version: "1.323.0"
      targets:
        - "components/terraform/vpc-flow-logs-bucket"
      included_paths:
        - "**/**"
        # If the component's folder has the `modules` sub-folder, it needs to be explicitly defined
        - "**/modules/**"
```

<br/>

## Hierarchical Imports in Vendoring Manifests

Use `imports` to split the main `vendor.yaml` manifest into smaller files for maintainability, or by their roles in the infrastructure.

For example, import separate manifests for networking, security, data management, CI/CD, and other layers:

```yaml
imports:
  - "layers/networking"
  - "layers/security"
  - "layers/data"
  - "layers/analytics"
  - "layers/firewalls"
  - "layers/cicd"
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
    - "vendor/vendor2"
    - "vendor/vendor3"

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
    - "vendor/vendor4"

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
    - "vendor/vendor5"

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
- The `vendor/vendor2` and `vendor/vendor3` manifests (defined in the main `vendor.yaml` file) are imported
- The `vendor/vendor2` file is processed, and the `vendor/vendor4` manifest is imported
- The `vendor/vendor4` file is processed, and the `vendor/vendor5` manifest is imported
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
      excluded_paths: []
```

To vendor the `vpc` component, execute the following command:

```bash
atmos vendor pull -c vpc
```
