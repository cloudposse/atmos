---
title: Component Vendoring
sidebar_position: 3
sidebar_label: Vendoring
description: Use Component Vendoring to make a copy of 3rd-party components in your own repo.
id: vendoring
---

Atmos natively supports the concept of "vendoring", which is making a copy of the 3rd party components in your own repo. Our implementation is primarily inspired by the excellent tool by VMware Tanzu, called [`vendir`](https://github.com/vmware-tanzu/carvel-vendir). While `atmos` does not call `vendir`, it functions and supports a configuration that is very similar.

After defining the `component.yaml` configuration, the remote component can be downloaded by running the following command:

```bash
atmos vendor pull -c components/terraform/vpc
```

## Vendoring Modules as Components

To vendor a component, create a `component.yaml` file stored inside of the `components/_type_/_name_/` folder (e.g. `components/terraform/vpc/`).

The schema of a `component.yaml` file is as follows:

```yaml
apiVersion: atmos/v1
kind: ComponentVendorConfig
metadata:
  name: vpc-flow-logs-bucket-vendor-config
  description: Source and mixins config for vendoring of 'vpc-flow-logs-bucket' component
spec:
  source:
    # 'uri' supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP),
    # and all URL and archive formats as described in https://github.com/hashicorp/go-getter
    # In 'uri', Golang templates are supported  https://pkg.go.dev/text/template
    # If 'version' is provided, '{{.Version}}' will be replaced with the 'version' value before pulling the files from 'uri'
    # To vendor a module from a Git repo, use the following format: 'github.com/cloudposse/terraform-aws-ec2-instance.git///?ref={{.Version}}
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}
    version: 0.194.0

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
      version: 0.194.0
      filename: introspection.mixin.tf
```

## Vendoring Modules as Components

Any terraform module can also be used as a component, provided that atmos backend generation (auto_generate_backend_file is true) is enabled. Use this strategy when you want to use the module directly, without needing to wrap it in a component to add additional functionality. This is essentially treating a terraform child module as a root module.

To vendor a module as a component, simply create a component.yaml file stored inside of the components/_type_/_name_/ folder (e.g. components/terraform/ec2-instance/). Note the usage of the ///, which is to vendor from the root of the remote repository.

The schema of a component.yaml file for a module is as follows:

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
