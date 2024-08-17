---
title: Provision
sidebar_position: 11
sidebar_label: Provision
---

Having configured the Terraform components, the Atmos components catalog, all the mixins and defaults, and the Atmos top-level stacks, we can now
provision the components in the stacks.

The `vpc` Atmos components use the remote state from the `vpc-flow-logs-bucket` components, therefore the `vpc-flow-logs-bucket` components must
be provisioned first.

## Provision Atmos Components into all Stacks

Provision the `vpc-flow-logs-bucket` Atmos component into the stacks:

```shell
# `plat` OU, `dev` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-dev
atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-dev

# `plat` OU, `staging` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-staging
atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-staging

# `plat` OU, `prod` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-prod
atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-prod
```

<br/>

Provision the `vpc` Atmos component into the stacks:

```shell
# `plat` OU, `dev` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc -s plat-ue2-dev
atmos terraform apply vpc -s plat-uw2-dev

# `plat` OU, `staging` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc -s plat-ue2-staging
atmos terraform apply vpc -s plat-uw2-staging

# `plat` OU, `prod` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc -s plat-ue2-prod
atmos terraform apply vpc -s plat-uw2-prod
```

<br/>

Alternatively, you can execute the configured [Atmos workflow](/quick-start/advanced/create-workflows) to provision all the components in all the stacks:

```shell
# Execute the workflow `apply-all-components` from the workflow manifest `networking`
atmos workflow apply-all-components -f networking
```

## Stack Search Algorithm

Looking at the commands above, you might have a question "How does Atmos find the component in the stack and all the variables?"

Let's consider what Atmos does when executing the command `atmos terraform apply vpc -s plat-ue2-prod`:

- Atmos uses the [CLI config](/quick-start/advanced/configure-cli) `stacks.name_pattern: "{tenant}-{environment}-{stage}"` to figure out that the first part of
  the stack name is `tenant`, the second part is `environment`, and the third part is `stage`

- Atmos searches for the stack configuration file (in the `stacks` folder and all sub-folders) where `tenant: plat`, `environment: ue2`
  and `stage: prod` are defined (inline or via imports). During the search, Atmos processes all parent (top-level) config files and compares the
  context variables specified in the command (`-s` flag) with the context variables defined in the stack configurations, finally finding the matching
  stack

- Atmos finds the component `vpc` in the stack, processing all the inline configs and all the configs from the imports

- Atmos deep-merges all the catalog imports for the `vpc` component and then deep-merges all the variables for the component defined in all
  sections (global `vars`, terraform `vars`, base components `vars`, component `vars`), producing the final variables for the `vpc` component in
  the `plat-ue2-prod` stack

- And lastly, Atmos writes the final deep-merged variables into a `.tfvar` file in the component directory and then
  executes `terraform apply -var-file ...` command
