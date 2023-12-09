---
title: Atmos component migration in YAML config
sidebar_label: Component Migration
description: "Learn how to migrate an Atmos component to a new name or to use the `metadata.inheritance`."
---

## Migrate component from using deprecated key `component` to use `metadata.inherits`

1. Identify the component to move, we'll use `vpc` for this example.

    Here is an example of the older method of using the `component` key

    ```yaml
    # stacks/catalog/vpc.yaml
    components:
      terraform:
        vpc:
          # This cannot be enabled here in the older method due to the global
          # AZ fragment importation
          settings:
            spacelift:
              workspace_enabled: false
          vars:
            enabled: true
            # ...etc...

    # stacks/gbl-ue2.yaml
    components:
      terraform:
        vpc:
          # This is the AZ fragment importation thats imported across all stacks
          # even stacks without a vpc.
          availability_zones:
            - us-east-1a
            - us-east-1b
            - us-east-1c

    # stacks/plat-ue2-dev.yaml
    import:
      - gbl-ue2
      - catalog/vpc
    
    components:
      terraform:
        vpc:
          component: vpc
          # This is where spacelift has to be enabled
          settings:
            spacelift:
              workspace_enabled: true
          vars:
            name: vpc
            # ...etc...

        vpc1:
          component: vpc
          # This is where spacelift has to be enabled
          settings:
            spacelift:
              workspace_enabled: true
          vars:
            name: vpc1
            # ...etc...
    ```

1. Verify terraform plan for `vpc` component is a `no change`. If not, configure it and apply it until it shows `no change`.

    ```sh
    ⨠ atmos terraform plan vpc --stack plat-ue2-dev
    ```

1. Pull down the latest `vpc` component and repeat the previous step (optional)

    ```sh
    ⨠ wget https://raw.githubusercontent.com/cloudposse/atmos/master/examples/quick-start/components/terraform/vpc/component.yaml -O components/terraform/vpc/component.yaml
    ⨠ sed -i 's,infra/vpc-flow-logs-bucket,vpc,g' components/terraform/vpc/component.yaml
    ⨠ atmos vendor pull -c vpc
    ```

1. Verify the current `workspace` name

    ```sh
    ⨠ atmos describe component vpc --stack plat-ue2-dev | grep ^workspace
    workspace: plat-ue2-dev
    ```

1. Count the current root stacks where the `vpc` component is defined

    ```sh
    ⨠ atmos describe stacks --components=vpc --sections=components | grep -oP '^[a-z0-9-]+' | wc -l
    17
    ```

1. In the `vpc.yaml` stack catalog, rename `vpc` to be `vpc/defaults`

    Add the `metadata` block

    ```yaml
    components:
      terraform:
        vpc/defaults:
          metadata:
            type: abstract
          # spacelift can now be enabled in the catalog
          settings:
            spacelift:
              workspace_enabled: true
          vars:
            enabled: true
            # ...etc...
    ```

1. Create a `vpc` component that inherits from `vpc/defaults`

    ```yaml
        vpc:
          metadata:
            component: vpc
            inherits:
              - vpc/defaults
          vars:
            name: vpc
    ```

1. Verify the `workspace` is the same

    ```sh
    ⨠ atmos describe component vpc --stack plat-ue2-dev | grep ^workspace
    workspace: plat-ue2-dev
    ```

    NOTE: Since the `vpc` component name shares the same name as its base component, the `workspace` will remain the same.

    NOTE: If the `workspace` has the `component` suffixed to it e.g. `plat-ue2-dev-vpc1` then the tfstate will have to be migrated. See component renaming and subsequent tfstate rename below before continuing.

1. Add `availability_zones` to `vpc/defaults` and only add the letters and omit the region.

1. Move any `vpc` component global overrides for `availability_zones` to `vpc/defaults`.

    The `vpc` configuration may have a fragment in a global region file

    ```yaml
        vpc:
          availability_zones:
            - us-east-1a
            - us-east-1b
            - us-east-1c
    ```

    It can now be set as this in the `vpc.yaml` catalog

    ```yaml
          availability_zones:
            - a
            - b
            - c
    ```

1. Count the root stacks again and verify the number is lower than previous

    ```sh
    ⨠ atmos describe stacks --components=vpc --sections=components | grep -oP '^[a-z0-9-]+' | wc -l
    9
    ```

1. Rerun the plan and verify `no changes` again

    ```sh
    ⨠ atmos terraform plan vpc --stack plat-ue2-dev
    ```

## Renaming components

If renaming a component is desired, for example, from `vpc` to `vpc1`, the workspace will change.

Follow these steps to ensure your state is properly managed.

1. Identify all the stacks affected by the component rename

1. Select a root stack and the component. We will use `plat-ue2-dev` root stack and `vpc` component for this example.

1. Verify terraform plan for `vpc` component is a `no change`. If not, configure and apply it.

1. Verify the old and new workspaces.

    Before renaming

    ```sh
    ⨠ atmos describe component vpc --stack plat-ue2-dev | grep ^workspace
    workspace: plat-ue2-dev
    ```

    After renaming

    ```sh
    ⨠ atmos describe component vpc1 --stack plat-ue2-dev | grep ^workspace
    workspace: plat-ue2-dev-vpc1
    ```

1. Follow one of the guides below for migrating the state or overriding the workspace

1. After you have completed the previous step, you will also need to rename any `component` keys in `remote-state.tf` files since the original reference to the component has been removed from the tfstate

### Migrating state manually

This approach is recommended because this will allow the catalog to be imported without additional overrides.

1. Navigate to the component and list the workspaces

    ```sh
    ⨠ cd components/terraform/vpc
    ⨠ terraform init
    ⨠ terraform workspace list
    default
    * plat-gbl-sandbox
      plat-ue2-dev
    ```

1. Select the original component's workspace

    ```sh
    ⨠ terraform workspace select plat-ue2-dev
    ```

1. Dump the terraform state into a new file

    ```sh
    ⨠ terraform state pull > plat-ue2-dev.tfstate
    ```

1. Select or create the new workspace if it does not exist

    ```sh
    ⨠ terraform workspace select plat-ue2-dev-vpc1
    ⨠ terraform workspace new plat-ue2-dev-vpc1
    Created and switched to workspace "plat-ue2-dev-vpc1"!
    ```

1. Push up the original workspace's state to the new workspace's state

    ```sh
    ⨠ terraform state push plat-ue2-dev.tfstate
    Releasing state lock. This may take a few moments...
    ```

1. Verify terraform plan for the new component is a `no change`.

    ```sh
    ⨠ atmos terraform plan vpc1 --stack plat-ue2-dev
    ```

### Overriding the workspace name

The fastest approach would be to override the workspace name but is not recommended since this will need to be done for every root stack where the catalog is imported.

1. In the root stack affected, instantiate the component and override the `terraform_workspace` to match the original.

    ```yaml
    # root stack: plat-ue2-dev
    import:
      - catalog/vpc

    components:
      terraform:
        vpc1:
          metadata:
            terraform_workspace: plat-ue2-dev
            component: vpc
            inherits:
              - vpc/defaults
          vars:
            name: vpc
    ```

1. Verify the `workspace` is correct

    ```
    ⨠ atmos describe component vpc1 --stack plat-ue2-dev | grep ^workspace
    workspace: plat-ue2-dev
    ```

1. Repeat the above steps for all the root stacks where the component is imported


