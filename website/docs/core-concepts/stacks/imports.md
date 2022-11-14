---
title: Stack Imports
sidebar_position: 7
sidebar_label: Imports
---

Imports are how we reduce duplication of configurations by creating reusable baselines. The imports should be thought of almost like blueprints. Once a reusable catalog of Stacks is exists, robust architectures can be easily created simply by importing those blueprints.

Imports may be used in Stack configuratinos together with [inheritance](/core-concepts/components/component-inheritance) and [mixins](/core-concepts/stacks/mixins) to produce an exceptionally DRY configuration in a way that is logically organized and easier to maintain for your team.

:::info
The mechanics of mixins and inheritance apply only to the [Stack](/core-concepts/stacks) configurations. Atmos knows nothing about the underlying components (e.g. terraform), and does not magically implement inheritance for HCL. However, by designing highly reusable components that do one thing well, we're able to achieve many of the same benefits.
:::

## Configuration

To import any stack configuration from the `catalog/`, simply define an `imports` section at the top of any [Stack](/core-concepts/stacks) configuration. Technically, it can be placed anywhere in the file, but by convention we recommend putting it at the top.

```yaml
imports:
- catalog/file1
- catalog/file2
- catalog/file2
```

The `base_path` for imports is specified in the [`atmos.yaml`](/cli/configuration).

If no file extension is used, a `.yaml` extension is automatically appended.

It's also possible to specify file extensions, although we do not recommend it.

```yaml
imports:
- catalog/file1.yml
- catalog/file2.yaml
- catalog/file2.YAML
```

## Conventions

We recommend placing all baseline "imports" in the `stacks/catalog` folder, however, they can exist anywhere.

Use [mixins](/core-concepts/stacks/mixins) for reusable snippets of configuration that alter the behavior of Stacks in some way.

