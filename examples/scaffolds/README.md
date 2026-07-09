---
title: Scaffolds
tags: [DX]
description: Concrete, ready-to-run scaffold templates — the same catalog atmos init pulls from.
cast:
  file: /casts/examples/scaffolds/aws-landing-zone-lifecycle.cast
  title: atmos scaffold generate ./aws/landing-zone
---

# Example: Scaffolds

Concrete, ready-to-run scaffold templates, organized by cloud. These are the
same templates `atmos init`'s catalog pulls from (`aws/app`, `aws/landing-zone`,
`gcp/landing-zone`, `azure/landing-zone`) — this directory is their source of
truth.

Learn more in the [Init Command Documentation](https://atmos.tools/cli/commands/init)
and the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate).

## What You'll See

- A real, emulator-provable landing zone generated from a local template
  directory with `atmos scaffold generate`
- How the generated project's `!terraform.state` dependency (`kms` first,
  then everything that reads its key) drives apply order

## Try It

```shell
cd examples/scaffolds

# See the templates this directory provides
atmos scaffold generate ./aws/landing-zone ./my-landing-zone --set project_name=my-landing-zone --dry-run

# Or generate for real
atmos scaffold generate ./aws/landing-zone ./my-landing-zone --set project_name=my-landing-zone
cd my-landing-zone
atmos emulator up aws -s dev
atmos terraform apply kms -s dev -auto-approve
atmos terraform apply audit-trail -s dev -auto-approve
atmos terraform apply baseline -s dev -auto-approve
atmos terraform apply monitoring -s dev -auto-approve
atmos terraform output monitoring -s dev
atmos emulator down aws -s dev
```

The same templates are also reachable by name through the catalog once
published (`atmos init aws/landing-zone ./my-landing-zone` or
`atmos scaffold generate aws/landing-zone ./my-landing-zone`).

## Templates

| Directory | Purpose |
|-----------|---------|
| [`aws/app`](./aws/app) | Application SDLC repository for AWS |
| [`aws/landing-zone`](./aws/landing-zone) | AWS landing zone environments (audit trail, KMS, SSM, monitoring, IAM) |
| [`gcp/landing-zone`](./gcp/landing-zone) | GCP landing zone environments (GCS, Secret Manager, service accounts) |
| [`azure/landing-zone`](./azure/landing-zone) | Azure landing zone environments (resource group, VNet, subnet, NSG) |

## Creating Custom Templates

For a minimal walkthrough of scaffold-template mechanics (prompts, Go
templating, `atmos.yaml`-registered templates), see
[`examples/scaffolding`](../scaffolding) instead — these templates are meant
to be used, not edited as a tutorial.
