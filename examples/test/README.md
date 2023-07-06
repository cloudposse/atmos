# Atmos configuration for testing

__NOTE:__ This folder is used to test different Atmos functionality (e.g. components and stacks config) and to run all Atmos tests.
It's not intended to be provisioned. For a complete working example, see [examples/complete](../complete).

The [`atmos`](https://github.com/cloudposse/atmos) CLI is a universal tool for DevOps and cloud automation. It allows
deploying and destroying Terraform and helmfile components, as well as running workflows to bootstrap or teardown all
resources in an account.

Noticeable `atmos` commands:

```bash
atmos version
atmos validate stacks
atmos describe stacks
atmos describe component infra/vpc -s tenant1-ue2-dev
atmos terraform plan infra/vpc -s tenant1-ue2-dev
atmos terraform plan test/test-component-override -s tenant1-ue2-dev
atmos terraform plan test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform validate test/test-component-override -s tenant1-ue2-dev
atmos terraform output test/test-component-override -s tenant1-ue2-dev
atmos terraform graph test/test-component-override -s tenant1-ue2-dev
atmos terraform show test/test-component-override -s tenant1-ue2-dev
atmos terraform shell test/test-component-override -s tenant1-ue2-dev
```
