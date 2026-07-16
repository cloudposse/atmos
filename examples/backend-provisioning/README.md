---
title: Backend Provisioning
tags: [Emulators, Terraform]
description: >-
  Manage a Terraform state backend directly with `atmos terraform backend` —
  create, update, and delete the S3 bucket that stores a component's state,
  independent of any `terraform apply`.
cast:
  file: /casts/examples/backend-provisioning/lifecycle.cast
  title: atmos terraform backend lifecycle
---

## Notes

This example provisions and manages a Terraform state **backend** — the S3 bucket that
stores a component's state — using the [`atmos terraform backend`](https://atmos.tools/cli/commands/terraform/terraform-backend)
CRUD subcommands, against a **local AWS sandbox** (no AWS account or credentials required).
The sandbox is an [Atmos emulator component](https://atmos.tools/cli/commands/emulator/usage),
the same one used by the [`emulator-aws`](/examples/emulator-aws) example.

The emulator is declared as a component (`components.emulator.aws`, driver `floci/aws`) and a
single `aws/emulator` identity in `atmos.yaml` binds every component to it. Both the Terraform
provider-config contributor (used by `atmos terraform apply`) *and* the `atmos terraform
backend` provisioner independently resolve that same identity's live endpoint, so `atmos
terraform backend create/update/delete --stack <stage>` works against the sandbox with no
manual endpoint configuration.

`provision.backend.enabled: true` must be set on the component (see `stacks/catalog/demo.yaml`)
for the manual CRUD commands to operate — the backend section itself must nest its
type-specific settings under the backend type key (`backend.s3.*`, not a flat `backend.*`).

## Usage

Start the sandbox, then exercise the backend lifecycle for a stack:

```shell
atmos emulator up aws -s dev                        # start the local sandbox for the `dev` stack

atmos terraform backend create demo -s dev           # provision the S3 bucket backing `demo`'s state
atmos terraform backend update demo -s dev           # re-apply secure defaults (idempotent)
atmos terraform backend delete demo -s dev --force   # remove the bucket

atmos emulator down aws -s dev                       # stop and remove the sandbox container
```

Every subcommand takes `--stack`/`-s` to target a specific stack — `atmos terraform backend
create demo -s staging` and `atmos terraform backend create demo -s prod` provision independent
buckets.

`list` and `describe` are also wired up on the CLI (they parse `--stack` correctly), but their
underlying provisioner logic is still a stub as of this writing (`atmos terraform backend list`
and `atmos terraform backend describe` both return "not yet implemented") — this example does
not exercise them.

The `atmos test` custom command runs the full create/update/delete cycle against `dev`, then
repeats create/delete against `staging` to prove multi-stack targeting works.

## Related

- [`atmos terraform backend`](https://atmos.tools/cli/commands/terraform/terraform-backend) — CLI reference for these subcommands
- [Backend Provisioning](https://atmos.tools/stacks/components/provision/backend) — the related `provision.backend.enabled` automatic-provisioning flow
- [`emulator-aws`](/examples/emulator-aws) — the same local AWS sandbox, used here for backend infrastructure instead of a component resource
