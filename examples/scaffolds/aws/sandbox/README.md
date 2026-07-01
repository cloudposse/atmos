<!-- atmos:template -->
# {{ .Config.project_name }}

A bare-bones [Atmos](https://atmos.tools) project for AWS, wired to the
[Floci](https://floci.io) emulator so you can plan and apply real Terraform
without real AWS credentials.

## What's inside

- `atmos.yaml` — Atmos configuration with a default Floci identity.
- `stacks/mixins/floci.yaml` — points the AWS provider at the local emulator.
- `stacks/catalog/bucket.yaml` — an abstract `bucket` component.
- `stacks/deploy/dev/bucket.yaml` — the `dev` stack that provisions it.
- `components/terraform/bucket/` — a minimal S3 bucket component.

## Try it

```sh
# 1. Start the Floci emulator (listens on http://localhost:4566).
docker compose up -d

# 2. Validate, plan, apply, and destroy the bucket — all against Floci.
atmos test

# Or run a single step:
atmos terraform apply bucket -s dev
```

When you are ready to target real AWS, replace `stacks/mixins/floci.yaml` with
real provider/auth settings and remove its import from the deploy stacks.
