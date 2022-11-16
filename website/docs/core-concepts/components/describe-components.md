---
title: Describe Components
sidebar_position: 6
sidebar_label: Describing
description: Describe components to view the fully deep-merged configuration
id: describing
---

Describing components is helpful to understand what the final, fully computed and deep-merged configuration of a component will look like across any number of Stacks. The more [DRY a configuration is due to imports](core-concepts/stacks/imports), the more [derviced the configuration is due to inheritance](/core-concepts/component/inheritance), the harder it may be to understasnd what the final stack configuration will be. 


For example, if we wanted to understand what the final configuration looks like for a "vpc" running in the "production" stack, we could do that by calling the [`atmos describe components`](/cli/commands/describe/component) command to view the YAML output.

For more powerful filtering options, consider [describing stacks](/core-concepts/stacks/describing) instead.
