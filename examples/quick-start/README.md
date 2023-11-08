# Atmos Quick Start

The [`atmos`](https://github.com/cloudposse/atmos) CLI is a universal tool for DevOps and cloud automation. It allows
deploying and destroying Terraform and helmfile components, as well as running workflows to bootstrap or teardown all
resources in an account.

Noticeable Atmos commands:

```bash
atmos version
atmos validate stacks
atmos describe stacks
atmos describe component vpc -s plat-ue2-dev
atmos terraform plan vpc -s plat-ue2-dev
atmos terraform shell vpc -s plat-ue2-dev
```
