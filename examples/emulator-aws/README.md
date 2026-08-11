---
title: AWS Emulator
tags: [Emulators]
description: >-
  Learn how to use the AWS emulator with Atmos — declare it as a stack
  component and Atmos starts, binds, and stops it around your Terraform
  runs.
cast:
  file: /casts/examples/emulator-aws/lifecycle.cast
  title: atmos emulator aws lifecycle
---

## Notes

This example provisions an S3 bucket against a **local AWS sandbox** — no AWS account or
credentials required. The sandbox is an [Atmos emulator component](https://atmos.tools/cli/commands/emulator/usage):
a container declared in the normal `local` stack that Atmos starts and stops for you. By default it runs
[Floci](https://github.com/floci-io/floci), a free, MIT-licensed AWS emulator and a drop-in
replacement for LocalStack Community Edition (which was EOL'd in March 2026).

The emulator is declared once in the `local` stack (`components.emulator.aws`, driver `floci/aws`)
and a single `aws/emulator` identity in `atmos.yaml` binds every Terraform component to the
`local/aws` instance. The
provider-config contributor injects the AWS provider settings (dummy credentials, path-style
S3, skip-flags) automatically, so there is **no `providers.tf` and no endpoint configuration**
to maintain, and no hand-rolled `docker-compose.yml`.

## Usage

Start the sandbox, apply, then tear everything down (a container runtime — Docker or Podman —
is the only prerequisite):

```shell
atmos emulator up aws -s local         # start the shared local sandbox
atmos terraform apply demo -s dev     # provision the S3 bucket against the emulator
atmos terraform output demo -s dev    # inspect outputs

atmos terraform destroy demo -s dev   # remove the resources
atmos emulator down aws -s local      # stop and remove the sandbox container
```

`atmos emulator list` inventories every configured emulator instance, including
`local/aws` when it has not been started; `atmos emulator ps` shows the running subset.
The `INSTANCE` column is the value an `aws/emulator` identity targets. Add `-s local`
to scope either command. For raw container-runtime
diagnostics, including stale containers from other projects, use `--runtime`.

Other lifecycle verbs include `atmos emulator logs aws -s local`.

The `atmos test` custom command runs the full apply/destroy lifecycle across the `dev`,
`staging`, and `prod` stacks.
