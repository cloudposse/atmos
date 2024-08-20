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

Alternatively, you can execute the configured [Atmos workflow](/quick-start/gcp/create-workflows) to provision all the components in all the stacks:

```shell
# Execute the workflow `apply-all-components` from the workflow manifest `networking`
atmos workflow apply-all-components -f networking
```
