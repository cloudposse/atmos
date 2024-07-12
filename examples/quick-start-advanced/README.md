# Atmos Quick Start

[Atmos](https://atmos.tools/) is a universal tool for DevOps and cloud automation. It allows
deploying and destroying Terraform and helmfile components, as well as running workflows to bootstrap or teardown all resources in an account.

Refer to the Atmos [Quick Start](https://atmos.tools/category/quick-start/) guide that describes the steps to configure and provision the 
infrastructure from this repository.

## Run Atmos Docker Image

To run the Atmos Docker image, execute the following command:

```shell
make all
```

## Noticeable Atmos commands

<details>
  <summary> Noticeable Atmos commands:</summary>

```shell
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
</details>

<br/>

For the description of the Atmos CLI configuration and all CLI commands, refer to [Atmos CLI](https://atmos.tools/cli/configuration).
