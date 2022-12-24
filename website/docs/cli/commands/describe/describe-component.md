---
title: atmos describe component
sidebar_label: component
sidebar_class_name: command
id: component
description: Use this command to describe the complete configuration for an Atmos component in an Atmos stack.
---

:::note Purpose
Use this command to describe the complete configuration for an [Atmos component](/core-concepts/components) in
an [Atmos stack](/core-concepts/stacks).
:::

## Usage

Execute the `atmos describe component` command like this:

```shell
atmos describe component <component> -s <stack>
```

<br/>

:::tip
Run `atmos describe component --help` to see all the available options
:::

## Examples

```shell
atmos describe component infra/vpc -s tenant1-ue2-dev
atmos describe component echo-server -s tenant1-ue2-staging
atmos describe component test/test-component-override -s tenant2-ue2-prod
```

## Arguments

| Argument    | Description     | Required |
|:------------|:----------------|:---------|
| `component` | Atmos component | yes      |

## Flags

| Flag      | Description | Alias | Required |
|:----------|:------------|:------|:---------|
| `--stack` | Atmos stack | `-s`  | yes      |

## Output

The command outputs the final component configuration in YAML format.

The configuration contains the following sections:

- `atmos_component` - Atmos component name
- `atmos_stack` - Atmos stack name
- `backend` -
- `backend_type` -
- `command` -
- `component` -
- `deps` -
- `env` -
- `inheritance` -
- `metadata` -
- `remote_state_backend` -
- `remote_state_backend_type` -
- `settings` -
- `sources` -
- `vars` -
- `workspace` -

<br/>

For example:

```shell
atmos describe component test/test-component-override-3 -s tenant1-ue2-dev
```

```yaml
atmos_component: test/test-component-override-3
atmos_stack: tenant1-ue2-dev
backend:
  bucket: cp-ue2-root-tfstate
  dynamodb_table: cp-ue2-root-tfstate-lock
  key: terraform.tfstate
  region: us-east-2
  workspace_key_prefix: test-test-component
backend_type: s3
command: terraform
component: test/test-component
deps:
  - catalog/terraform/mixins/test-1
  - catalog/terraform/mixins/test-2
  - orgs/cp/_defaults
  - orgs/cp/tenant1/_defaults
  - orgs/cp/tenant1/dev/us-east-2
env:
  TEST_ENV_VAR1: val1-override-3
  TEST_ENV_VAR2: val2-override-3
  TEST_ENV_VAR3: val3-override-3
  TEST_ENV_VAR4: val4-override-3
inheritance:
  - mixin/test-2
  - mixin/test-1
  - test/test-component-override-2
  - test/test-component-override
  - test/test-component
metadata:
  component: test/test-component
  inherits:
    - test/test-component-override
    - test/test-component-override-2
    - mixin/test-1
    - mixin/test-2
  terraform_workspace: test-component-override-3-workspace
remote_state_backend:
  bucket: cp-ue2-root-tfstate
  dynamodb_table: cp-ue2-root-tfstate-lock
  region: us-east-2
  workspace_key_prefix: test-test-component
remote_state_backend_type: s3
settings:
  config:
    is_prod: false
  spacelift:
    stack_name_pattern: '{tenant}-{environment}-{stage}-new-component'
    workspace_enabled: true
sources:
  vars:
    enabled:
      final_value: true
      name: enabled
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.vars
          variable_value: true
        - dependency_type: inline
          stack_file: orgs/cp/tenant1/dev/us-east-2
          stack_file_section: terraform.vars
          variable_value: false
    # Other variables are omitted for clarity
vars:
  enabled: true
  environment: ue2
  namespace: cp
  region: us-east-2
  service_1_map:
    a: 1
    b: 6
    c: 7
    d: 8
  service_1_name: mixin-2
  stage: dev
  tenant: tenant1
workspace: test-component-override-3-workspace
```

## Sources of Component Variables

For example:

```shell
atmos describe component test/test-component-override-3 -s tenant1-ue2-dev
```

```yaml
sources:
  vars:
    enabled:
      final_value: true
      name: enabled
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.vars
          variable_value: true
        - dependency_type: inline
          stack_file: orgs/cp/tenant1/dev/us-east-2
          stack_file_section: terraform.vars
          variable_value: false
        - dependency_type: inline
          stack_file: orgs/cp/tenant1/dev/us-east-2
          stack_file_section: vars
          variable_value: true
    environment:
      final_value: ue2
      name: environment
      stack_dependencies:
        - dependency_type: import
          stack_file: mixins/region/us-east-2
          stack_file_section: vars
          variable_value: ue2
    namespace:
      final_value: cp
      name: namespace
      stack_dependencies:
        - dependency_type: import
          stack_file: orgs/cp/_defaults
          stack_file_section: vars
          variable_value: cp
    region:
      final_value: us-east-2
      name: region
      stack_dependencies:
        - dependency_type: import
          stack_file: mixins/region/us-east-2
          stack_file_section: vars
          variable_value: us-east-2
    service_1_map:
      final_value:
        a: 1
        b: 6
        c: 7
        d: 8
      name: service_1_map
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/services/service-1-override-2
          stack_file_section: components.terraform.vars
          variable_value:
            b: 6
            c: 7
            d: 8
        - dependency_type: import
          stack_file: catalog/terraform/services/service-1-override
          stack_file_section: components.terraform.vars
          variable_value:
            a: 1
            b: 2
            c: 3
    service_1_name:
      final_value: mixin-2
      name: service_1_name
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/mixins/test-2
          stack_file_section: components.terraform.vars
          variable_value: mixin-2
        - dependency_type: import
          stack_file: catalog/terraform/mixins/test-1
          stack_file_section: components.terraform.vars
          variable_value: mixin-1
        - dependency_type: import
          stack_file: catalog/terraform/services/service-1-override-2
          stack_file_section: components.terraform.vars
          variable_value: service-1-override-2
        - dependency_type: import
          stack_file: catalog/terraform/tenant1-ue2-dev
          stack_file_section: components.terraform.vars
          variable_value: service-1-override-2
        - dependency_type: import
          stack_file: catalog/terraform/services/service-1-override
          stack_file_section: components.terraform.vars
          variable_value: service-1-override
        - dependency_type: import
          stack_file: catalog/terraform/services/service-1
          stack_file_section: components.terraform.vars
          variable_value: service-1
    stage:
      final_value: dev
      name: stage
      stack_dependencies:
        - dependency_type: import
          stack_file: mixins/stage/dev
          stack_file_section: vars
          variable_value: dev
```
