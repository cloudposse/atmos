---
title: Provision
sidebar_position: 7
sidebar_label: Provision
---

Having configured the Terraform components, the Atmos components catalog, all the mixins and defaults, and the Atmos parent stacks, we can now
provision the components into the stacks.

The `vpc-1` Atmos components use the remote state from the `vpc-flow-logs-bucket-1` components, therefore the `vpc-flow-logs-bucket-1` components must
be provisioned first.

## Provision Atmos Components into All Stacks

Provision the `vpc-flow-logs-bucket-1` Atmos component into the stacks:

```shell
# `core` OU, `dev` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-flow-logs-bucket-1 -s core-ue2-dev
atmos terraform apply vpc-flow-logs-bucket-1 -s core-uw2-dev

# `core` OU, `staging` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-flow-logs-bucket-1 -s core-ue2-staging
atmos terraform apply vpc-flow-logs-bucket-1 -s core-uw2-staging

# `core` OU, `prod` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-flow-logs-bucket-1 -s core-ue2-prod
atmos terraform apply vpc-flow-logs-bucket-1 -s core-uw2-prod
```

<br/>

Provision the `vpc-1` Atmos component into the stacks:

```shell
# `core` OU, `dev` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-1 -s core-ue2-dev
atmos terraform apply vpc-1 -s core-uw2-dev

# `core` OU, `staging` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-1 -s core-ue2-staging
atmos terraform apply vpc-1 -s core-uw2-staging

# `core` OU, `prod` account, `us-east-2` and `us-west-2` regions
atmos terraform apply vpc-1 -s core-ue2-prod
atmos terraform apply vpc-1 -s core-uw2-prod
```

## Stack Search Algorithm

Looking at the commands above, you might have a question "How does Atmos find the component in the stack and all the variables?"

Let's consider what Atmos does when executing the command `atmos terraform apply vpc-1 -s core-ue2-prod`:

- Atmos uses the [CLI config](/quick-start/configure-cli) `stacks.name_pattern: "{tenant}-{environment}-{stage}"` to figure out that the first part of
  the stack name is `tenant`, the second part is `environment`, and the third part is `stage`

- Atmos searches for the stack configuration file (in the `stacks` folder and all sub-folders) where `tenant: core`, `environment: ue2`
  and `stage: prod` are defined (inline or via imports). During the search, Atmos processes all parent (top-level) config files and compares the
  context variables specified in the command (`-s` flag) with the context variables defined in the stack configurations, finally finding the matching
  stack

- Atmos finds the component `vpc-1` in the stack, processing all the inline configs and all the imports

- Atmos deep-merges all the catalog imports for the `vpc-1` component and then deep-merges all the variables for the component defined in all
  sections (global `vars`, terraform `vars`, base components `vars`, component `vars`), producing the final variables for the `vpc-1` component in
  the `core-ue2-prod` stack

- And lastly, Atmos writes the final deep-merged variables into a `.tfvar` file in the component directory and then
  executes `terraform apply -var-file ...` command
