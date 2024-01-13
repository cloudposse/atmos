---
title: atmos
sidebar_label: atmos
sidebar_class_name: command
sidebar_position: 1
description: This command starts an interactive UI to select an Atmos command, component and stack. Press "Enter" to execute the command
---

:::note Purpose
Use this command to start an interactive UI to select an Atmos command, component and stack. Press `Enter` to execute the command for the selected
stack and component
:::

## Usage

Just run `atmos`:

```shell
atmos
```

<br/>

- Use the `right/left` arrow keys to navigate between the "Commands", "Stacks" and "Components" views

- Use the `up/down` arrow keys to select a command to execute, component and stack

- Use the `/` key to filter/search for the commands, components, and stacks in the corresponding views

- Use the `Tab` key to flip the "Stacks" and "Components" views. This is useful to be able to use the UI in two different modes:

  * Mode 1: Components in Stacks. Display all available stacks, select a stack, then show all the components that are defined in the selected stack

  * Mode 2: Stacks for Components. Display all available components, select a component, then show all the stacks where the selected component is
    configured

- Press `Enter` to execute the selected command for the selected stack and component

## Example

### Mode 1: Components in Stacks

![`atmos` CLI command mode 1](/img/cli/atmos-cli-command-1.png)

### Mode 2: Stacks for Components

![`atmos` CLI command mode 2](/img/cli/atmos-cli-command-2.png)
