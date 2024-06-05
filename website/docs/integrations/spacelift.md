---
title: Spacelift Integration
sidebar_position: 6
sidebar_label: Spacelift
---

Atmos natively supports [Spacelift](https://spacelift.io). This is accomplished using
the [`cloudposse/terraform-spacelift-cloud-infrastructure-automation`](https://github.com/cloudposse/terraform-spacelift-cloud-infrastructure-automation)
terraform module that reads the YAML Stack configurations and produces the Spacelift resources.

Cloud Posse provides two terraform components for Spacelift support:

- [Terraform Component](/core-concepts/components/) for provisioning a
  [Spacelift Worker Pool](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/spacelift/worker-pool)

- [Terraform Component](/core-concepts/components/) for
  provisioning [Spacelift Stacks](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/spacelift/admin-stack)

## Stack Configuration

Atmos components support the following `spacelift` specific settings:

```yaml
components:
  terraform:
    example:
      settings:
        spacelift:
          # enable the stack in Spacelift
          workspace_enabled: true

          administrative: true

          # auto deploy this stack
          autodeploy: true

          # commands to run before init
          before_init: []

          # Specify which component directory to use
          component_root: components/terraform/example

          description: Example component

          # whether to auto destroy resources if the stack is deleted
          stack_destructor_enabled: false

          worker_pool_name: null

          # Do not add normal set of child policies to admin stacks
          policies_enabled: []

          # set explicitly below
          administrative_trigger_policy_enabled: false

          # policies to enable
          policies_by_id_enabled:
            - trigger-administrative-policy
```

<br/>


## OpenTofu Support

Spacelift is compatible with [OpenTofu](https://opentofu.org) and configurable on a global and per stack or component basis.

To make OpenTofu the default, add the following to your top-level stack manifest:

```yaml
settings:
  spacelift:
    # Use OpenTofu    
    terraform_workflow_tool: OPEN_TOFU
```

Similarly, to override this behavior, or to only configure it on specific components, add the following to the component 
configuration:

```yaml
components:
  terraform:
    my-component:
      settings:
        spacelift:
          # Use OpenTofu
          terraform_workflow_tool: OPEN_TOFU
```

For more details on [Atmos support for OpenTofu](/integrations/opentofu) see our integration page.

## Spacelift Stack Dependencies

Atmos supports [Spacelift Stack Dependencies](https://docs.spacelift.io/concepts/stack/stack-dependencies) in component configurations.

You can define component dependencies by using the `settings.depends_on` section. The section used to define all the Atmos components (in
the same or different stacks) that the current component depends on.

The `settings.depends_on` section is a map of objects. The map keys are just the descriptions of dependencies and can be strings or numbers.
Provide meaningful descriptions or numbering so that people can understand what the dependencies are about.

Each object in the `settings.depends_on` section has the following schema:

- `component` (required) - an Atmos component that the current component depends on
- `namespace` (optional) - the `namespace` where the Atmos component is provisioned
- `tenant` (optional) - the `tenant` where the Atmos component is provisioned
- `environment` (optional) - the `environment` where the Atmos component is provisioned
- `stage` (optional) - the `stage` where the Atmos component is provisioned

<br/>

The `component` attribute is required. The rest are the context variables and are used to define Atmos stacks other than the current stack.
For example, you can specify:

- `namespace` if the `component` is from a different Organization
- `tenant` if the `component` is from a different Organizational Unit
- `environment` if the `component` is from a different region
- `stage` if the `component` is from a different account
- `tenant`, `environment` and `stage` if the component is from a different Atmos stack (e.g. `tenant1-ue2-dev`)

<br/>

In the following example, we specify that the `top-level-component1` component depends on the following:

- The `test/test-component-override` component in the same Atmos stack
- The `test/test-component` component in Atmos stacks in the `dev` stage
- The `my-component` component from the `tenant1-ue2-staging` Atmos stack

```yaml
components:
  terraform:
    top-level-component1:
      settings:
        depends_on:
          1:
            # If the `context` (namespace, tenant, environment, stage) is not provided,
            # the `component` is from the same Atmos stack as this component
            component: "test/test-component-override"
          2:
            # This component (in any stage) depends on `test/test-component`
            # from the `dev` stage (in any `environment` and any `tenant`)
            component: "test/test-component"
            stage: "dev"
          3:
            # This component depends on `my-component`
            # from the `tenant1-ue2-staging` Atmos stack
            component: "my-component"
            tenant: "tenant1"
            environment: "ue2"
            stage: "staging"
      vars:
        enabled: true
```

<br/>

:::tip

Refer to [`atmos describe dependents` CLI command](/cli/commands/describe/dependents) for more information.

:::
