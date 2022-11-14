---
title: Spacelift Integration
sidebar_position: 3
sidebar_label: Spacelift
---

Atmos natively supports [Spacelift](https://spacelift.io). This is accomplished using
the [`cloudposse/terraform-spacelift-cloud-infrastructure-automation`](https://github.com/cloudposse/terraform-spacelift-cloud-infrastructure-automation)
terraform module that reads the YAML Stack configurations and produces the Spacelift resources.

Cloud Posse provides two terraform components that implement Spacelift support.

- [Terraform Component](/core-concepts/components/) for provising
  a [Spacelift Worker Pool](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/spacelift-worker-pool)
- [Terraform Component](/core-concepts/components/) for provisioning
  the [Spacelift Stacks](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/spacelift)

## Stack Configuration

The Atmos Spacelift Terraform Component supports some `spacelift` specific settings.

```yaml
components:
  terraform:
    example:
      settings:
        spacelift:
          # enable the stack in spacelift
          workspace_enabled: true 

          administrative: true

          # auto deploy this stack
          autodeploy: true   

          # commands to run before init
          before_init: []

          # Specify which component directory to use
          component_root: components/terraform/example

          description: Example component

          # whether or not to auto destroy resources if the stack is deleted
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
