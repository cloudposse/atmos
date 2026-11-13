# Request and provision

1. Open **Create → Request a service with an S3 bucket**.
2. Enter a unique name, such as `orders`.
3. Follow the task's pull-request link to Gitea. Sign in with `atmos` / `atmos`.
4. Review the generated Atmos stack and catalog entry. Wait for the **plan** job.
5. Merge the PR. The **deploy** job provisions the local bucket and registers the
    service in Backstage. Follow its logs from the repository's Actions page.
6. Open **Catalog → orders**. The service depends on the `orders-bucket` Resource.

## What the template does

An Atmos scaffold creates `stacks/deploy/services/orders.yaml` and
`catalog/orders.yaml`. A reusable Starlark module publishes these files to a
request branch through a Git target. Backstage opens the review request.

The catalog YAML describes ownership, source links, and infrastructure dependencies.
Terraform creates `atmos-backstage-dev-orders` in the local AWS emulator.

## State and cleanup

CI keeps Terraform state in the runner's mounted local state directory, separately
from the manual host runner. Use one execution path for a service; do not alternate
CI and manual apply against different state files.

`atmos portal down` resets the entire demo, including the repository, buckets,
runner registration, and Terraform state. It is not a per-service destroy command.
