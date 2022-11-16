---
title: Describe Stacks
sidebar_position: 7
sidebar_label: Describing
id: describing
description: Describe stacks to view the fully deep-merged configuration
---

Describing stacks is helpful to understand what the final, fully computed and deep-merged configuration of a stack will look like. Use this to slice and dice the Stack configuration to show different information about stacks and component.

For example, if we wanted to understand what the final configuration looks like for the "production" stack, we could do that by calling the [`atmos describe stacks`](/cli/commands/describe/stacks) command to view the YAML output.

The output can be written to a file by passing the `--file` command-line flag to `atmos` or even formatted as YAML or JSON by using `--format` command-line flag.

Since the output of a Stack might be overwhelming and we're only interested in some particular section of the configuration, the output can be filtered using flats to narrow the output by `stack`, `component-types`, `components`, and `sections`. The component sections can be further filtered by `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`.

:::tip PRO TIP
If the filtering options built-in to atmos are not sufficient, redirect the output to [`jq`](https://stedolan.github.io/jq/) for very powerful filtering options.
:::
