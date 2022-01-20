# atmos examples

The [`atmos`](https://github.com/cloudposse/atmos) CLI is a universal tool for DevOps and cloud automation. It allows deploying and destroying
Terraform and helmfile components, as well as running workflows to bootstrap or teardown all resources in an account.

For local development inside a Docker container, start the Docker container and execute the following commands:

```bash
cd /localhost/
cd <path to the repo on localhost>
# Set the base path to the `stacks` and `components` folders
export ATMOS_BASE_PATH=$(pwd)
```

Execute `atmos` commands from the container:

```bash
atmos version
atmos terraform plan infra/vpc -s tenant1-ue2-dev
atmos terraform plan test/test-component-override -s tenant1-ue2-dev
atmos terraform plan test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform validate test/test-component-override -s tenant1-ue2-dev
atmos terraform output test/test-component-override -s tenant1-ue2-dev
atmos terraform graph test/test-component-override -s tenant1-ue2-dev
atmos terraform show test/test-component-override -s tenant1-ue2-dev
atmos terraform shell test/test-component-override -s tenant1-ue2-dev
```
