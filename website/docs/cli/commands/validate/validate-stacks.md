---
title: atmos validate stacks
sidebar_label: stacks
sidebar_class_name: command
id: validate-stacks
description: Use this command to validate Stack configurations.
---

:::note Purpose
Use this command to validate Stack configurations.
:::

## Usage

Execute the `validate stacks` command like this:

```shell
atmos validate stacks
```

<br/>

This command validates stacks configurations. The command checks and validates the following:

- All YAML config files for any YAML errors and inconsistencies

- All imports - if they are configured correctly, have valid data types, and point to existing files

- Schema - if all sections in all YAML files are correctly configured and have valid data types

<br/>

:::tip
Run `atmos validate stacks --help` to see all the available options
:::
