---
title: AWS CloudFormation
tags: [Components, Emulators]
description: >-
  Deploy a native aws/cloudformation component — no external binary, no AWS
  account or credentials required — against a local Floci AWS emulator.
cast:
  file: /casts/examples/cloudformation/lifecycle.cast
  title: atmos aws cloudformation lifecycle
related_docs:
  - label: CloudFormation component configuration
    url: /stacks/components/aws-cloudformation
  - label: Emulator commands
    url: /cli/commands/emulator/usage
---

## Notes

This example deploys one `AWS::SSM::Parameter` resource through a CloudFormation
component against a local [Floci](https://github.com/floci-io/floci) emulator.
It requires Docker or Podman; no AWS account is needed.

The `local` stack declares the emulator. An `aws/emulator` identity in
`atmos.yaml` connects the CloudFormation component to it, including the endpoint
configuration. See [Atmos emulators](https://atmos.tools/cli/commands/emulator/usage).

The example mirrors [the Terraform emulator example](../emulator-aws) using
CloudFormation for the same resource lifecycle.

## Usage

Before running the lifecycle, install Docker or Podman and Python 3 (`python3` on `PATH`).
The demo lifecycle hooks use Python 3 to record their events.

Start the sandbox, deploy, inspect outputs, then tear everything down:

```shell
atmos emulator up aws -s local                # start the shared local sandbox
atmos aws cfn deploy demo -s local            # create/update the stack (changeset-driven apply + auto-approve)
atmos aws cfn output demo -s local            # inspect the stack's Outputs

atmos aws cfn delete demo -s local            # delete the stack
atmos emulator down aws -s local              # stop and remove the sandbox container
```

The `cfn` alias works in place of `cloudformation`. To preview changes, use
`plan` or `diff`; to print the template, use `render`; to validate it through
CloudFormation, use `validate`.

Use `atmos emulator list` to see configured instances and `atmos emulator ps`
to see running instances. Add `-s local` to filter either command to this stack.

The `atmos test` custom command runs the full deploy/delete lifecycle end to end.

## Learn More

See the [`atmos aws cloudformation`](https://atmos.tools/cli/commands/aws/cloudformation) docs.
