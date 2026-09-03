---
title: AWS CloudFormation
tags: [Emulators, Components]
description: >-
  Deploy a native aws/cloudformation component — no external binary, no AWS
  account or credentials required — against a local Floci AWS emulator.
cast:
  file: /casts/examples/cloudformation/lifecycle.cast
  title: atmos aws cloudformation lifecycle
---

## Notes

This example deploys a real, minimal CloudFormation stack (a single `AWS::SSM::Parameter`
resource) through the native **`aws/cloudformation`** component type — SDK-native, no
`aws`/`rain` binary shell-out — against a **local AWS sandbox**: no AWS account or credentials
required. The sandbox is an [Atmos emulator component](https://atmos.tools/cli/commands/emulator/usage):
a container declared in the `local` stack that Atmos starts and stops for you. By default it runs
[Floci](https://github.com/floci-io/floci), a free, MIT-licensed AWS emulator and a drop-in
replacement for LocalStack Community Edition (which was EOL'd in March 2026).

The emulator is declared once in the `local` stack (`components.emulator.aws`, driver `floci/aws`)
and a single `aws/emulator` identity in `atmos.yaml` binds the `aws/cloudformation` component to the
`local/aws` instance. `pkg/aws/identity.LoadConfigWithAuth` — the same in-process AWS SDK config
seam every `atmos aws cloudformation` operation goes through — honors that identity's endpoint
override automatically, so there is **no endpoint configuration** to maintain anywhere in this
example.

This example mirrors [`examples/emulator-aws`](../emulator-aws) (same emulator, same
"hello from `<stage>`" SSM parameter marker), demonstrating the equivalent lifecycle through the
`aws/cloudformation` component type instead of Terraform.

## Usage

Start the sandbox, deploy, inspect outputs, then tear everything down (a container runtime —
Docker or Podman — is the only prerequisite):

```shell
atmos emulator up aws -s local                # start the shared local sandbox
atmos aws cfn deploy demo -s local            # create/update the stack (changeset-driven apply + auto-approve)
atmos aws cfn output demo -s local            # inspect the stack's Outputs

atmos aws cfn delete demo -s local            # delete the stack
atmos emulator down aws -s local              # stop and remove the sandbox container
```

`atmos aws cfn` is a Cobra alias for `atmos aws cloudformation` — both work everywhere. Other
lifecycle verbs include `plan`/`diff` (preview a changeset without executing it), `render`
(client-side template render, no API calls), and `validate` (server-side `ValidateTemplate`).

`atmos emulator list` inventories every configured emulator instance, including `local/aws` when
it has not been started; `atmos emulator ps` shows the running subset. The `INSTANCE` column is
the value an `aws/emulator` identity targets. Add `-s local` to scope either command.

The `atmos test` custom command runs the full deploy/delete lifecycle end to end.

## Learn More

See the [`atmos aws cloudformation`](https://atmos.tools/cli/commands/aws/cloudformation/usage) docs.
