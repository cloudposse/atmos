# Atmos Quick Start

[`Atmos`](https://github.com/cloudposse/atmos) is a universal tool for DevOps and cloud automation. It allows
deploying and destroying Terraform and helmfile components, as well as running workflows to bootstrap or teardown all
resources in an account.

Noticeable Atmos commands:

```bash
atmos version
atmos validate stacks
atmos describe stacks

atmos terraform shell vpc -s plat-ue2-dev
atmos terraform shell vpc-flow-logs-bucket -s plat-ue2-dev

atmos describe component vpc -s plat-ue2-dev
atmos describe component vpc -s plat-ue2-staging
atmos describe component vpc -s plat-ue2-dev

atmos describe component vpc -s plat-uw2-dev
atmos describe component vpc -s plat-uw2-staging
atmos describe component vpc -s plat-uw2-dev

atmos describe component vpc-flow-logs-bucket -s plat-ue2-dev
atmos describe component vpc-flow-logs-bucket -s plat-ue2-staging
atmos describe component vpc-flow-logs-bucket -s plat-ue2-dev

atmos terraform plan vpc -s plat-ue2-dev
atmos terraform plan vpc -s plat-ue2-staging
atmos terraform plan vpc -s plat-ue2-prod

atmos terraform apply vpc -s plat-ue2-dev
atmos terraform apply vpc -s plat-ue2-staging
atmos terraform apply vpc -s plat-ue2-prod

atmos terraform apply vpc -s plat-uw2-dev
atmos terraform apply vpc -s plat-uw2-staging
atmos terraform apply vpc -s plat-uw2-prod

atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-dev
atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-staging
atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-prod

atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-dev
atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-staging
atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-prod
```
