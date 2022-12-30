---
title: Create Atmos Stacks
sidebar_position: 6
sidebar_label: Create Stacks
---

In the previous step, we've configured the Terraform components and described how they can be copied into the repository.

Next step is to create and configure [Atmos stacks](/core-concepts/stacks).

## Create Catalog for Components

## Create Parent Stacks

When executing the [CLI commands](/cli/cheatsheet), Atmos does not use the stack file names and their filesystem locations to search for the stack
where the component is defined. Instead, Atmos uses the context variables (`namespace`, `tenant`, `environment`, `stage`) to search for the stack. The
stack config file names cam be anything, and they can be in any folders in any sub-folders in the `stacks` directory.

For example, when executing the `atmos terraform apply infra/vpc -s tenant1-ue2-dev`
command, the stack `tenant1-ue2-dev` is specified by the `-s` flag. By looking at `name_pattern: "{tenant}-{environment}-{stage}"`
(see [Configure CLI](/quick-start/configure-cli)) and processing the tokens, Atmos knows that the first part of the stack name is `tenant`, the second
part is `environment`, and the third part is `stage`. Then Atmos searches for the stack configuration file (in the `stacks` directory)
where `tenant: tenant1`, `environment: ue2` and `stage: dev` are defined (inline or via imports).
