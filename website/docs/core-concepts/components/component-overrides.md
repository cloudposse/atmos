---
title: Component Overrides
sidebar_position: 9
sidebar_label: Overrides
description: Use the 'overrides' pattern to modify component(s) configuration and behavior.
id: overrides
---

Atmos supports the 'overrides' pattern to modify component(s) configuration and behavior using the `overrides` section in Atmos stack manifests.

You can override the following sections in the component(s) configuration:

- `vars`
- `settings`
- `env`
- `command`

The `overrides` section can be used at the global level or at the Terraform and Helmfile levels.

The `overrides` section schema at the global level is as follows:

```yaml
overrides:
  # Override the ENV variables for the components in the current stack manifest and all its imports
  env: {}
  # Override the settings for the components in the current stack manifest and all its imports
  settings: {}
  # Override the variables for the components in the current stack manifest and all its imports
  vars: {}
  # Override the command to execute for the components in the current stack manifest and all its imports
  command: "<command to execute>"
```

The `overrides` section schemas at the Terraform and Helmfile levels are as follows:

```yaml
terraform:
  overrides:
    env: {}
    settings: {}
    vars: {}
    command: "<command to execute>"

helmfile:
  overrides:
    env: {}
    settings: {}
    vars: {}
    command: "<command to execute>"
```

```yaml title="stacks/orgs/cp/tenant1/dev/us-west-2.yaml"
import:
  - mixins/region/us-west-2
  - orgs/cp/tenant1/dev/_defaults
  # Import all components that the `devops` Team manages
  - teams/devops
  # Import all components that the `testing` Team manages
  - teams/testing
```

```yaml title="stacks/teams/devops.yaml"
# The `devops` Team manages the following components:
import:
  - catalog/terraform/top-level-component1
```

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
