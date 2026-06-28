# Atmos Quick Start (Advanced)

[Atmos](https://atmos.tools/) is a universal tool for DevOps and cloud automation. This advanced
example provisions a small AWS application stack — **entirely offline** — against a local AWS
emulator ([Floci](https://floci.io/)), so you can learn the patterns end to end without a real AWS
account or any credentials.

Follow the [Quick Start: Advanced](https://atmos.tools/quick-start/advanced/) guide for a step-by-step
walkthrough of this repository.

## It's just plain Terraform

The components in [`components/terraform/`](components/terraform/) are **vanilla Terraform** — raw
`aws_*` resources using only the official `hashicorp/aws` provider. There are **no Cloud Posse
modules and no special wrappers**. Atmos is a **bring-your-own-Terraform** orchestrator: it never
requires you to rewrite your Terraform or adopt proprietary components. Each component carries only a
**stock `providers.tf`** (`provider "aws" { region = var.region }`) with **no endpoint or credentials
configuration** — the `local-aws` identity (`kind: aws/emulator`) generates a
`providers_override.tf.json` that points the provider at the emulator at runtime, so the exact same
code deploys unchanged against real AWS.

| Component | What it is (plain Terraform) |
|-----------|------------------------------|
| `kms-key` | `aws_kms_key` + alias |
| `s3-bucket` | `aws_s3_bucket` (+ versioning, SSE) |
| `dynamodb-table` | `aws_dynamodb_table` |
| `sns-topic` | `aws_sns_topic` |
| `sqs-queue` | `aws_sqs_queue` (+ policy) |
| `app-config` | publishes resolved config + secrets to SSM Parameter Store |

The components are wired together with Atmos features — not Terraform `remote_state` data sources:
stack templates build predictable coordinates, dependency metadata controls graph order, store hooks
publish applied outputs, and `!secret` resolves declared secrets from the SSM/Secrets Manager stores.

## Run it end to end

Everything runs against the local emulator. You need [Atmos](https://atmos.tools/install) and a
container runtime (Docker or Podman).

The `test` custom command brings up the emulator, seeds secrets, applies the full component DAG in
dependency order, confirms the cross-component wiring resolves, then tears everything down:

```shell
atmos test -s plat-ue2-dev
```

Or run the same steps by hand:

```shell
# Start the local AWS emulator for the stack.
atmos emulator up aws -s plat-ue2-dev

# Lint every stack manifest.
atmos validate stacks

# Seed the required app-config secrets.
atmos secret set API_KEY=sk-quickstart-example -s plat-ue2-dev -c app-config
atmos secret set 'DB_CONFIG={"username":"app","password":"s3cr3t"}' -s plat-ue2-dev -c app-config

# Apply every component in dependency order (the S3 state backend is auto-provisioned
# inside the emulator via `provision.backend.enabled`).
atmos terraform deploy --all -s plat-ue2-dev

# Inspect a component and see where every value came from.
atmos describe component app-config -s plat-ue2-dev --provenance
atmos list instances --format tree --provenance

# Tear down.
atmos terraform destroy --all -s plat-ue2-dev -auto-approve
atmos emulator down aws -s plat-ue2-dev
```

Available stacks: `plat-ue2-{dev,staging,prod}` (us-east-2) and `plat-uw2-{dev,staging,prod}`
(us-west-2).

## How the environments differ

The three environments deploy the **same services with different settings** — that's the whole point of the layered configuration. The
[catalog](stacks/catalog/) defines each component's defaults once; each account's `_defaults.yaml` overrides only what differs. Open
[`stacks/orgs/acme/plat/prod/_defaults.yaml`](stacks/orgs/acme/plat/prod/_defaults.yaml) and you can see, in one file, exactly what makes `prod`
different.

| Setting | `dev` (ephemeral) | `staging` (middle) | `prod` (hardened) |
|---|---|---|---|
| `s3-bucket` `force_destroy` | `true` | `true` | `false` |
| `s3-bucket` `versioning_enabled` | `false` | `true` | `true` |
| `kms-key` `deletion_window_in_days` | `7` | `14` | `30` |
| `kms-key` `enable_key_rotation` | `false` | `true` | `true` |
| `sqs-queue` `message_retention_seconds` | `86400` (1d) | `345600` (4d) | `1209600` (14d) |

See it resolved per stage (no deploy required):

```shell
atmos describe component s3-bucket -s plat-ue2-dev  --process-functions=false --process-templates=false
atmos describe component s3-bucket -s plat-ue2-prod --process-functions=false --process-templates=false
```

## Operator commands

This example also registers operator-focused [custom commands](https://atmos.tools/core-concepts/custom-commands)
in `atmos.yaml`:

```shell
atmos operator status -s <stack>                  # show stack tree, instances, catalog, and next commands
atmos operator inspect <component> -s <stack>     # describe component config with provenance
```

For the full CLI configuration and command reference, see [Atmos CLI](https://atmos.tools/cli/configuration).
