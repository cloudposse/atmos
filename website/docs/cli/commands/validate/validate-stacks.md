---
title: atmos validate stacks
sidebar_label: stacks
sidebar_class_name: command
id: stacks
description: Use this command to validate all Stack configurations.
---

:::note Purpose
Use this command to validate Atmos stack manifest configurations.
:::

## Usage

Execute the `validate stacks` command like this:

```shell
atmos validate stacks
```

<br/>

This command validates Atmos stack manifests and checks the following:

- All YAML manifest files for any YAML errors and inconsistencies

- All imports: if they are configured correctly, have valid data types, and point to existing files

- Schema: if all sections in all YAML manifest files are correctly configured and have valid data types

<br/>

:::tip
Run `atmos validate stacks --help` to see all the available options
:::

## Examples

```shell
atmos validate stacks
atmos validate stacks --schemas-atmos-manifest schemas/atmos-manifest/1.0/atmos-manifest.json
```

## Flags

| Flag                       | Description                                                                                                                                           | Alias | Required |
|:---------------------------|:------------------------------------------------------------------------------------------------------------------------------------------------------|:------|:---------|
| `--schemas-atmos-manifest` | Path to JSON Schema to validate Atmos stack manifests.<br/>Can be an absolute path or <br/>a path relative to the `base_path` setting in `atmos.yaml` |       | no       |

## Validate Atmos Manifests using JSON Schema

Atmos can use the [Atmos Manifest JSON Schema](pathname://../../../schemas/atmos-manifest/1.0/atmos-manifest.json) to validate Atmos stack manifests
on the command line by executing the command `atmos validate stacks`.

For this to work, configure the following:

- Add the [Atmos Manifest JSON Schema](pathname://../../../schemas/atmos-manifest/1.0/atmos-manifest.json) to your repository, for example
  in  `schemas/atmos-manifest/1.0/atmos-manifest.json`

- Configure the following section in the `atmos.yaml` [CLI config file](/cli/configuration)

  ```yaml title="atmos.yaml"
  # Validation schemas (for validating atmos stacks and components)
  schemas:
    # JSON Schema to validate Atmos manifests
    atmos:
      # Can also be set using 'ATMOS_SCHEMAS_ATMOS_MANIFEST' ENV var, or '--schemas-atmos-manifest' command-line arguments
      # Supports both absolute and relative paths (relative to the `base_path` setting in `atmos.yaml`)
      manifest: "schemas/atmos-manifest/1.0/atmos-manifest.json"
  ```

- Execute the command `atmos validate stacks`

- Instead of configuring the `schemas.atmos.manifest` section in `atmos.yaml`, you can provide the path to
  the [Atmos Manifest JSON Schema](pathname://../../../schemas/atmos-manifest/1.0/atmos-manifest.json) file by using the ENV variable `ATMOS_SCHEMAS_ATMOS_MANIFEST`
  or the
  `--schemas-atmos-manifest` command line argument:

  ```shell
  ATMOS_SCHEMAS_ATMOS_MANIFEST=schemas/atmos-manifest/1.0/atmos-manifest.json atmos validate stacks
  atmos validate stacks --schemas-atmos-manifest schemas/atmos-manifest/1.0/atmos-manifest.json
  ```
