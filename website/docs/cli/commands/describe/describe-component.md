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

atmos describe component infra/vpc -s tenant1-ue2-dev --format json

atmos describe component infra/vpc -s tenant1-ue2-dev -f yaml

atmos describe component infra/vpc -s tenant1-ue2-dev --file component.yaml

atmos describe component echo-server -s tenant1-ue2-staging

atmos describe component test/test-component-override -s tenant2-ue2-prod
```

## Arguments

| Argument    | Description     | Required |
|:------------|:----------------|:---------|
| `component` | Atmos component | yes      |

## Flags

| Flag       | Description                                         | Alias | Required |
|:-----------|:----------------------------------------------------|:------|:---------|
| `--stack`  | Atmos stack                                         | `-s`  | yes      |
| `--format` | Output format: `yaml` or `json` (`yaml` is default) | `-f`  | no       |
| `--file`   | If specified, write the result to the file          |       | no       |

## Output

The command outputs the final deep-merged component configuration in YAML format.

The output contains the following sections:

- `atlantis_project` - Atlantis project name (if [Atlantis Integration](/integrations/atlantis) is configured for the component in the stack)

- `atmos_cli_config` - information about Atmos CLI configuration from `atmos.yaml`

- `atmos_component` - [Atmos component](/core-concepts/components) name

- `atmos_stack` - [Atmos stack](/core-concepts/stacks) name

- `atmos_stack_file` - the stack config file name where the Atmos stack is defined

- `backend` - Terraform backend configuration

- `backend_type` - Terraform backend type

- `command` - the binary to execute when provisioning the component (e.g. `terraform`, `terraform-1`, `helmfile`)

- `component` - the Terraform component for which the Atmos component provides configuration

- `component_info` - a block describing the Terraform or Helmfile components that the Atmos component manages. The `component_info` block has the
  following sections:
  - `component_path` - the filesystem path to the Terraform or Helmfile component

  - `component_type` - the type of the component (`terraform` or `helmfile`)

  - `terraform_config` - if the component type is `terraform`, this sections describes the high-level metadata about the Terraform component from its
    source code, including variables, outputs and child Terraform modules (using a Terraform parser from HashiCorp). The file names and line numbers
    where the variables, outputs and child modules are defined are also included. Invalid Terraform configurations are also detected, and in case of
    any issues, the warnings and errors are shows in the `terraform_config.diagnostics` section

- `deps` - a list of stack dependencies (stack config files where the component settings are defined, either inline or via imports)

- `env` - a list of ENV variables defined for the Atmos component

- `inheritance` - component's [inheritance chain](/core-concepts/components/inheritance)

- `metadata` - component's metadata config

- `remote_state_backend` - Terraform backend config for remote state

- `remote_state_backend_type` - Terraform backend type for remote state

- `settings` - component settings (free-form map)

- `sources` - sources of the values from the component's sections (`vars`, `env`, `settings`)

- `spacelift_stack` - Spacelift stack name (if [Spacelift Integration](/integrations/spacelift) is configured for the component in the stack
  and `settings.spacelift.workspace_enabled` is set to `true`)

- `vars` - the final deep-merged component variables that are provided to Terraform and Helmfile when executing `atmos terraform`
  and `atmos helmfile` commands

- `workspace` - Terraform workspace for the Atmos component

<br/>

For example:

```shell
atmos describe component test/test-component-override-3 -s tenant1-ue2-dev
```

```yaml
atlantis_project: tenant1-ue2-dev-test-test-component-override-3
atmos_cli_config:
  base_path: ./examples/complete
  components:
    terraform:
      base_path: components/terraform
      apply_auto_approve: false
      deploy_run_init: true
      init_run_reconfigure: true
      auto_generate_backend_file: false
    helmfile:
      base_path: components/helmfile
      use_eks: true
      kubeconfig_path: /dev/shm
      helm_aws_profile_pattern: '{namespace}-{tenant}-gbl-{stage}-helm'
      cluster_name_pattern: '{namespace}-{tenant}-{environment}-{stage}-eks-cluster'
  stacks:
    base_path: stacks
    included_paths:
      - orgs/**/*
    excluded_paths:
      - '**/_defaults.yaml'
    name_pattern: '{tenant}-{environment}-{stage}'
  workflows:
    base_path: stacks/workflows
atmos_component: test/test-component-override-3
atmos_stack: tenant1-ue2-dev
atmos_stack_file: orgs/cp/tenant1/dev/us-east-2
backend:
  bucket: cp-ue2-root-tfstate
  dynamodb_table: cp-ue2-root-tfstate-lock
  key: terraform.tfstate
  region: us-east-2
  workspace_key_prefix: test-test-component
backend_type: s3
command: terraform
component: test/test-component
component_info:
  component_path: examples/complete/components/terraform/test/test-component
  component_type: terraform
  terraform_config:
    path: examples/complete/components/terraform/test/test-component
    variables:
      enabled:
        name: enabled
        type: bool
        description: Set to false to prevent the module from creating any resources
        default: null
        required: false
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/context.tf
          line: 97
      environment:
        name: environment
        type: string
        description: ID element. Usually used for region e.g. 'uw2', 'us-west-2',
          OR role 'prod', 'staging', 'dev', 'UAT'
        default: null
        required: false
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/context.tf
          line: 115
      name:
        name: name
        type: string
        description: |
          ID element. Usually the component or solution name, e.g. 'app' or 'jenkins'.
          This is the only ID element not also included as a `tag`.
          The "name" tag is set to the full `id` string. There is no tag with the value of the `name` input.
        default: null
        required: false
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/context.tf
          line: 127
      namespace:
        name: namespace
        type: string
        description: ID element. Usually an abbreviation of your organization name,
          e.g. 'eg' or 'cp', to help ensure generated IDs are globally unique
        default: null
        required: false
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/context.tf
          line: 103
      region:
        name: region
        type: string
        description: Region
        default: null
        required: true
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/variables.tf
          line: 1
      service_1_name:
        name: service_1_name
        type: string
        description: Service 1 name
        default: null
        required: true
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/variables.tf
          line: 6
      stage:
        name: stage
        type: string
        description: ID element. Usually used to indicate role, e.g. 'prod', 'staging',
          'source', 'build', 'test', 'deploy', 'release'
        default: null
        required: false
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/context.tf
          line: 121
      tenant:
        name: tenant
        type: string
        description: ID element _(Rarely used, not included by default)_. A customer
          identifier, indicating who this instance of a resource is for
        default: null
        required: false
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/context.tf
          line: 109
    outputs:
      service_1_id:
        name: service_1_id
        description: Service 1 ID
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/outputs.tf
          line: 1
      service_2_id:
        name: service_2_id
        description: Service 2 ID
        sensitive: false
        pos:
          filename: examples/complete/components/terraform/test/test-component/outputs.tf
          line: 6
    requiredcore:
      - '>= 1.0.0'
    modulecalls:
      service_1_label:
        name: service_1_label
        source: cloudposse/label/null
        version: 0.25.0
        pos:
          filename: examples/complete/components/terraform/test/test-component/main.tf
          line: 1
      service_2_label:
        name: service_2_label
        source: cloudposse/label/null
        version: 0.25.0
        pos:
          filename: examples/complete/components/terraform/test/test-component/main.tf
          line: 10
      this:
        name: this
        source: cloudposse/label/null
        version: 0.25.0
        pos:
          filename: examples/complete/components/terraform/test/test-component/context.tf
          line: 23
    diagnostics: []
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
  TEST_ENV_VAR4: null
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
    protect_from_deletion: true
    stack_destructor_enabled: false
    stack_name_pattern: '{tenant}-{environment}-{stage}-new-component'
    workspace_enabled: false
sources:
  env:
    TEST_ENV_VAR1:
      final_value: val1-override-3
      name: TEST_ENV_VAR1
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-3
          stack_file_section: components.terraform.env
          variable_value: val1-override-3
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-2
          stack_file_section: components.terraform.env
          variable_value: val1-override-2
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override
          stack_file_section: components.terraform.env
          variable_value: val1-override
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.env
          variable_value: val1
  settings:
    spacelift:
      final_value:
        protect_from_deletion: true
        stack_destructor_enabled: false
        stack_name_pattern: '{tenant}-{environment}-{stage}-new-component'
        workspace_enabled: false
      name: spacelift
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-3
          stack_file_section: components.terraform.settings
          variable_value:
            workspace_enabled: false
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-2
          stack_file_section: components.terraform.settings
          variable_value:
            stack_name_pattern: '{tenant}-{environment}-{stage}-new-component'
            workspace_enabled: true
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.settings
          variable_value:
            workspace_enabled: true
        - dependency_type: import
          stack_file: catalog/terraform/spacelift-and-backend-override-1
          stack_file_section: settings
          variable_value:
            protect_from_deletion: true
            stack_destructor_enabled: false
            workspace_enabled: true
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

The `sources.vars` section of the output shows the final deep-merged component's variables and their inheritance chain.

Each variable descriptor has the following schema:

- `final_value` - the final value of the variable after Atmos processes and deep-merges all values from all stack config files
- `name` - the variable name
- `stack_dependencies` - the variable's inheritance chain (stack config files where the values for the variable were provided). It has the following
  schema:

  - `stack_file` - the stack config file where the value for the variable was provided
  - `stack_file_section` - the section of the stack config file where the value for the variable was provided
  - `variable_value` - the variable's value
  - `dependency_type` - how the variable was defined (`inline` or `import`). `inline` means the variable was defined in one of the sections
    in the stack config file. `import` means the stack config file where the variable is defined was imported into the parent Atmos stack

<br/>

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

<br/>

:::info

The `stack_dependencies` inheritance chain shows the variable sources in the reverse order the sources were processed.
The first item in the list was processed the last and its `variable_value` overrode all the previous values of the variable.

:::

<br/>

For example, the component's `enabled` variable has the following inheritance chain:

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
```

<br/>

Which we can interpret as follows (reading from the last to the first item in the `stack_dependencies` list):

- In the `orgs/cp/tenant1/dev/us-east-2` stack config file (the last item in the list), the value for `enabled` was set to `true` in the global `vars`
  section (inline)

- Then in the same `orgs/cp/tenant1/dev/us-east-2` stack config file, the value for `enabled` was set to `false` in the `terraform.vars`
  section (inline). This value overrode the value set in the global `vars` section

- Finally, in the `catalog/terraform/test-component` stack config file (which was imported into the parent Atmos stack
  via [`import`](/core-concepts/stacks/imports)), the value for `enabled` was set to `true` in the `components.terraform.vars` section of
  the `test/test-component-override-3` Atmos component. This value overrode all the previous values arriving at the `final_value: true` for the
  variable. This final value is then set for the `enabled` variable of the Terraform component `test/test-component` when Atmos
  executes `atmos terraform apply test/test-component-override-3 -s <stack>` command

## Sources of Component ENV Variables

The `sources.env` section of the output shows the final deep-merged component's environment variables and their inheritance chain.

Each variable descriptor has the following schema:

- `final_value` - the final value of the variable after Atmos processes and deep-merges all values from all stack config files
- `name` - the variable name
- `stack_dependencies` - the variable's inheritance chain (stack config files where the values for the variable were provided). It has the following
  schema:

  - `stack_file` - the stack config file where the value for the variable was provided
  - `stack_file_section` - the section of the stack config file where the value for the variable was provided
  - `variable_value` - the variable's value
  - `dependency_type` - how the variable was defined (`inline` or `import`). `inline` means the variable was defined in one of the sections
    in the stack config file. `import` means the stack config file where the variable is defined was imported into the parent Atmos stack

<br/>

For example:

```shell
atmos describe component test/test-component-override-3 -s tenant1-ue2-dev
```

```yaml
sources:
  env:
    TEST_ENV_VAR1:
      final_value: val1-override-3
      name: TEST_ENV_VAR1
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-3
          stack_file_section: components.terraform.env
          variable_value: val1-override-3
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-2
          stack_file_section: components.terraform.env
          variable_value: val1-override-2
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override
          stack_file_section: components.terraform.env
          variable_value: val1-override
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.env
          variable_value: val1
    TEST_ENV_VAR2:
      final_value: val2-override-3
      name: TEST_ENV_VAR2
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-3
          stack_file_section: components.terraform.env
          variable_value: val2-override-3
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-2
          stack_file_section: components.terraform.env
          variable_value: val2-override-2
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.env
          variable_value: val2
    TEST_ENV_VAR3:
      final_value: val3-override-3
      name: TEST_ENV_VAR3
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-3
          stack_file_section: components.terraform.env
          variable_value: val3-override-3
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override
          stack_file_section: components.terraform.env
          variable_value: val3-override
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.env
          variable_value: val3
```

<br/>

:::info

The `stack_dependencies` inheritance chain shows the ENV variable sources in the reverse order the sources were processed.
The first item in the list was processed the last and its `variable_value` overrode all the previous values of the variable.

:::

<br/>

For example, the component's `TEST_ENV_VAR1` ENV variable has the following inheritance chain:

```yaml
sources:
  env:
    TEST_ENV_VAR1:
      final_value: val1-override-3
      name: TEST_ENV_VAR1
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-3
          stack_file_section: components.terraform.env
          variable_value: val1-override-3
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-2
          stack_file_section: components.terraform.env
          variable_value: val1-override-2
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override
          stack_file_section: components.terraform.env
          variable_value: val1-override
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.env
          variable_value: val1
```

<br/>

Which we can interpret as follows (reading from the last to the first item in the `stack_dependencies` list):

- In the `catalog/terraform/test-component` stack config file (the last item in the list), the value for the `TEST_ENV_VAR1` ENV variable was set
  to `val1` in the `components.terraform.env` section

- Then the value was set to `val1-override` in the `catalog/terraform/test-component-override` stack config file. This value overrides the value set
  in the `catalog/terraform/test-component` stack config file

- Then the value was set to `val1-override-2` in the `catalog/terraform/test-component-override-2` stack config file. This value overrides the values
  set in the `catalog/terraform/test-component` and `catalog/terraform/test-component-override` stack config files

- Finally, in the `catalog/terraform/test-component-override-3` stack config file (which was imported into the parent Atmos stack
  via [`import`](/core-concepts/stacks/imports)), the value was set to `val1-override-3` in the `components.terraform.env` section of
  the `test/test-component-override-3` Atmos component. This value overrode all the previous values arriving at the `final_value: val1-override-3` for
  the ENV variable

## Sources of Component Settings

The `sources.settings` section of the output shows the final deep-merged component's settings and their inheritance chain.

Each setting descriptor has the following schema:

- `final_value` - the final value of the setting after Atmos processes and deep-merges all values from all stack config files
- `name` - the setting name
- `stack_dependencies` - the setting's inheritance chain (stack config files where the values for the variable were provided). It has the following
  schema:

  - `stack_file` - the stack config file where the value for the setting was provided
  - `stack_file_section` - the section of the stack config file where the value for the setting was provided
  - `variable_value` - the setting's value
  - `dependency_type` - how the setting was defined (`inline` or `import`). `inline` means the setting was defined in one of the sections
    in the stack config file. `import` means the stack config file where the setting is defined was imported into the parent Atmos stack

<br/>

For example:

```shell
atmos describe component test/test-component-override-3 -s tenant1-ue2-dev
```

```yaml
sources:
  settings:
    spacelift:
      final_value:
        protect_from_deletion: true
        stack_destructor_enabled: false
        stack_name_pattern: '{tenant}-{environment}-{stage}-new-component'
        workspace_enabled: false
      name: spacelift
      stack_dependencies:
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-3
          stack_file_section: components.terraform.settings
          variable_value:
            workspace_enabled: false
        - dependency_type: import
          stack_file: catalog/terraform/test-component-override-2
          stack_file_section: components.terraform.settings
          variable_value:
            stack_name_pattern: '{tenant}-{environment}-{stage}-new-component'
            workspace_enabled: true
        - dependency_type: import
          stack_file: catalog/terraform/test-component
          stack_file_section: components.terraform.settings
          variable_value:
            workspace_enabled: true
        - dependency_type: import
          stack_file: catalog/terraform/spacelift-and-backend-override-1
          stack_file_section: settings
          variable_value:
            protect_from_deletion: true
            stack_destructor_enabled: false
            workspace_enabled: true
```

<br/>

:::info

The `stack_dependencies` inheritance chain shows the sources of the setting in the reverse order the sources were processed.
The first item in the list was processed the last and its `variable_value` overrode all the previous values of the setting.

:::
