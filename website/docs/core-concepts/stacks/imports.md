---
title: Stack Imports
sidebar_position: 7
sidebar_label: Imports
id: imports
---

Imports are how we reduce duplication of configurations by creating reusable baselines. The imports should be thought of almost like blueprints. Once
a reusable catalog of Stacks exists, robust architectures can be easily created simply by importing those blueprints.

Imports may be used in Stack configuratinos together with [inheritance](/core-concepts/components/inheritance)
and [mixins](/core-concepts/stacks/mixins) to produce an exceptionally DRY configuration in a way that is logically organized and easier to maintain
for your team.

:::info
The mechanics of mixins and inheritance apply only to the [Stack](/core-concepts/stacks) configurations. Atmos knows nothing about the underlying
components (e.g. terraform), and does not magically implement inheritance for HCL. However, by designing highly reusable components that do one thing
well, we're able to achieve many of the same benefits.
:::

## Configuration

To import any stack configuration from the `catalog/`, simply define an `import` section at the top of any [Stack](/core-concepts/stacks)
configuration. Technically, it can be placed anywhere in the file, but by convention we recommend putting it at the top.

```yaml
import:
  - catalog/file1
  - catalog/file2
  - catalog/file2
```

s
The base path for imports is specified in the [`atmos.yaml`](/cli/configuration) in the `stacks.base_path` section.

If no file extension is used, a `.yaml` extension is automatically appended.

It's also possible to specify file extensions, although we do not recommend it.

```yaml
import:
  - catalog/file1.yml
  - catalog/file2.yaml
  - catalog/file2.YAML
```

## Conventions

We recommend placing all baseline "imports" in the `stacks/catalog` folder, however, they can exist anywhere.

Use [mixins](/core-concepts/stacks/mixins) for reusable snippets of configurations that alter the behavior of Stacks in some way.

## Imports Schema

The `import` section supports the following two formats:

- a list of paths to the imported files, for example:

  ```yaml title=stacks/orgs/cp/tenant1/test1/us-east-2.yaml
  import:
    - mixins/region/us-east-2
    - orgs/cp/tenant1/test1/_defaults
    - catalog/terraform/top-level-component1
    - catalog/terraform/test-component
    - catalog/terraform/vpc
    - catalog/helmfile/echo-server
  ```

- a list of objects with the following schema:

  ```yaml
  import:
    - path: "<path_to_imported_file>"
      context: {}
    - path: "<path_to_imported_file>"
      context: {}
  ```

## Related

- [Configure CLI](/quick-start/configure-cli)
- [Create Atmos Stacks](/quick-start/create-atmos-stacks)
