---
title: Stack Validation
sidebar_position: 2
sidebar_label: Validation
description: Validate all Stack configurations and YAML syntax.
id: validation
---

To validate all Stack configurations and YAML syntax, execute the `validate stacks` command:

```shell
atmos validate stacks
```

<br/>

The command checks and validates the following:

- All YAML manifest files for YAML errors and inconsistencies

- All imports: if they are configured correctly, have valid data types, and point to existing manifest files

- Schema: if all sections in all YAML manifest files are correctly configured and have valid data types

- Misconfiguration and duplication of components in stacks. If the same Atmos component in the same Atmos stack is
  defined in more than one stack manifest file, an error message will be displayed similar to the following:

  ```console
  the Atmos component 'vpc' in the stack 'plat-ue2-dev' is defined in more than one top-level stack
  manifest file: orgs/acme/plat/dev/us-east-2, orgs/acme/plat/dev/us-east-2-extras.
  Atmos can't decide which stack manifest to use to get configuration for the component
  in the stack. This is a stack misconfiguration.
  ```

<br/>

:::tip
For more details, refer to [`atmos validate stacks`](/cli/commands/validate/stacks) CLI command
:::
