---
title: Component Overrides
sidebar_position: 9
sidebar_label: Overrides
description: Use the 'overrides' pattern to modify component(s) configuration and behavior.
id: overrides
---

Atmos supports the __overrides__ pattern to modify component(s) configuration and behavior using the `overrides` section in Atmos stack manifests.

You can override the following sections in the component(s) configuration:

- `vars`
- `settings`
- `env`
- `command`

The `overrides` section can be used at the global level or at the Terraform and Helmfile levels.

## Overrides Schema

The `overrides` section schema at the global level is as follows:

```yaml
overrides:
  # Override the ENV variables for the components in the current stack manifest and all its imports
  env: { }
  # Override the settings for the components in the current stack manifest and all its imports
  settings: { }
  # Override the variables for the components in the current stack manifest and all its imports
  vars: { }
  # Override the command to execute for the components in the current stack manifest and all its imports
  command: "<command to execute>"
```

The `overrides` section schemas at the Terraform and Helmfile levels are as follows:

```yaml
terraform:
  overrides:
    env: { }
    settings: { }
    vars: { }
    command: "<command to execute>"

helmfile:
  overrides:
    env: { }
    settings: { }
    vars: { }
    command: "<command to execute>"
```

You can include the `overrides`, `terraform.overrides` and `helmfile.overrides` sections in any Atmos stack manifest at any level of inheritance
to override the configuration of all the Atmos components defined inline in the manifest and in all its imports.

<br/>

:::tip
Refer to [Atmos Component Inheritance](/core-concepts/components/inheritance) for more information on all types of component inheritance
supported by Atmos
:::

## Advanced Example

The __overrides__ pattern is used to override the components only in a particular Atmos stack manifest and all the imported
manifests. This is different from the other configuration sections (e.g. `vars`, `settings`). If we define a `vars` or `settings` section at the 
global, Terraform or Helmfile levels, all the components in the top-level stack will get the `vars` and `settings` configurations. On the other hand,
If we define an `overrides` section in a stack manifest, only the components directly defined in the manifest and its imports will get the overridden 
values, not all the components in a top-level Atmos stack.

This is especially useful when you have Atmos stack manifests split per Teams, each Team manages a set of components, and you need to define a common
configuration (or override the existing one) for the components that only a particular Team manages. 

For example, we have two Teams: `devops` and `testing`.

The `devops` Team manifest is defined in `stacks/teams/devops.yaml` and the Team manages the following components:

```yaml title="stacks/teams/devops.yaml"
# The `devops` Team manages the following components:
import:
  - catalog/terraform/top-level-component1
```

The `testing` Team manifest is defined in `stacks/teams/testing.yaml` and the Team manages the following components:

```yaml title="stacks/teams/testing.yaml"
# The `testing` Team manages the following components:
import:
  - catalog/terraform/test-component
  - catalog/terraform/test-component-override
```

We can import the two manifests into a top-level stack manifest, e.g. `tenant1/dev/us-west-2.yaml`:

```yaml title="stacks/orgs/cp/tenant1/dev/us-west-2.yaml"
import:
  - mixins/region/us-west-2
  - orgs/cp/tenant1/dev/_defaults
  # Import all components that the `devops` Team manages
  - teams/devops
  # Import all components that the `testing` Team manages
  - teams/testing
```

Now suppose that we want to change some variables in the `vars` section and some config in the `settings` section for all the components that the 
`testing` Team manges, but we don't want to affect any component that the `devops` Team manages. If we add a global or Terraform level `vars` and
`settings` section to the top-level manifest `stacks/orgs/cp/tenant1/dev/us-west-2.yaml` or to the Team manifest `stacks/teams/testing.yaml`, then
all the components in the `tenant1/dev/us-west-2` top-level stack will be modified, including those managed by the `devops` Team.

We could individually modify the `vars` and `settings` sections in all the components that the `testing` Team manages, but the entire configuration
would not be DRY and reusable. To make it DRY and configured only in one place, use the __overrides__ pattern and the `overrides` section.

For example:

```yaml title="stacks/teams/testing.yaml"
# The `testing` Team manages the following components:
import:
  - catalog/terraform/test-component
  - catalog/terraform/test-component-override

# Global overrides
# Override the variables, env, command and settings ONLY in the components that the `testing` team manages
overrides:
  env:
    # This variable will be added or overridden in all the components that the `testing` Team manages
    TEST_ENV_VAR1: "test-env-var1-overridden"
  settings: { }
  vars: { }

# Terraform overrides
# Override the variables, env, command and settings ONLY in the Terraform components that the `testing` team manages
# The Terraform `overrides` are deep-merged with the global overrides
terraform:
  overrides:
    settings:
      spacelift:
        # All the components that the `testing` Team manages will have the Spacelift stacks auto-applied
        # if the planning phase was successful and there are no plan policy warnings
        # https://docs.spacelift.io/concepts/stack/stack-settings#autodeploy
        autodeploy: true
    vars:
      # This variable will be added or overridden in all the Terraform components that the `testing` Team manages
      test_1: 1
    # The `testing` Team uses `tofu` instead of `terraform`
    # https://opentofu.org
    # The commands `atmos terraform ...` will execute the `tofu` binary
    command: tofu

# Helmfile overrides
# Override the variables, env, command and settings ONLY in the Helmfile components that the `testing` team manages
# The Helmfile `overrides` are deep-merged with the global overrides
helmfile:
  overrides: { }
```
