---
title: AWS CDK with CloudFormation
tags: [Components, Hooks, Emulators]
description: >-
  Deploy an AWS CDK application through the native aws/cloudformation component.
  A before-hook synthesizes the template, Atmos previews and deploys it, and a
  local Floci AWS emulator stands in for an AWS account.
cast:
  file: /casts/examples/cdk-cloudformation/lifecycle.cast
  title: atmos aws cloudformation lifecycle with AWS CDK
related_docs:
  - label: Deploy AWS CDK with lifecycle hooks
    url: /tutorials/deploy-cdk-with-hooks
  - label: CloudFormation component configuration
    url: /stacks/components/aws-cloudformation
  - label: Lifecycle hooks
    url: /stacks/hooks
  - label: Emulator commands
    url: /cli/commands/emulator/usage
---

## Notes

This example is the native CloudFormation track of the
[Deploy AWS CDK with lifecycle hooks](https://atmos.tools/tutorials/deploy-cdk-with-hooks)
tutorial. A TypeScript AWS CDK app in `components/cloudformation/greeting/` creates one
`AWS::SSM::Parameter`. The component's `synth` hook installs dependencies when `package-lock.json` changed or the CLI is missing, then runs
`npm run synth` before `diff`, `apply`, and `changeset create`, so Atmos always reads a freshly
synthesized `cdk.out/Demo.template.json`. Deployment state stays in CloudFormation.

The `local` stack declares a [Floci](https://github.com/floci-io/floci) AWS emulator and
an `aws/emulator` identity in `atmos.yaml` points the component at it, so no AWS account is
needed. Remove that identity and the emulator import to deploy to a real account.

Requirements: Node.js 22 or later, npm, and Docker or Podman for the emulator.

## Usage

```shell
atmos validate stacks
atmos emulator up aws -s local
atmos aws cloudformation diff greeting -s local
atmos aws cloudformation deploy greeting -s local
atmos aws cloudformation output greeting -s local
atmos aws cloudformation delete greeting -s local --auto-approve
atmos emulator down aws -s local
```

Change `parameters.Message` in `stacks/deploy/local.yaml` and run `diff` again to see the
synthesized template re-read. `render` and `validate` do not fire the hook; run
`npm --prefix components/cloudformation/greeting run synth` (after an initial `npm ci`) before
those commands.

Run the whole lifecycle with `atmos test`.
