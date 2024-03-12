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

- All YAML config files for any YAML errors and inconsistencies

- All imports - if they are configured correctly, have valid data types, and point to existing files

- Schema - if all sections in all YAML files are correctly configured and have valid data types
