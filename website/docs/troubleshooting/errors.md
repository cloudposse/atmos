---
title: Errors
sidebar_position: 1
sidebar_label: Errors
---

## Common Mistakes

:::note
TODO
:::

## Common Errors


### Error: `stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable`

This means that you are probably missing a section like this in your `atmos.yml`. See the instructions on CLI Configuration for more details.

```yaml
stacks:
  name_pattern: "{tenant}-{environment}-{stage}"
```

### Error: `The stack name pattern '{tenant}-{environment}-{stage}' specifies 'tenant', but the stack ue1-prod does not have a tenant defined`

This means that your `name_pattern` declares a `tenant` is required, but not specified in the Stack configurations. Either specify a `tenant` in your `vars` for the Stack configuration, or remove the `{tenant}` from the `name_pattern`.

```yaml
stacks:
  name_pattern: "{tenant}-{environment}-{stage}"
```
