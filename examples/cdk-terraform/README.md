---
title: CDK Terrain with Terraform
tags: [Components, Hooks, Emulators]
description: >-
  Deploy a CDK Terrain application through a Terraform component. A before-hook
  synthesizes Terraform JSON, Terraform applies it, and a local Floci AWS emulator
  stands in for an AWS account.
cast:
  file: /casts/examples/cdk-terraform/lifecycle.cast
  title: atmos terraform lifecycle with CDK Terrain
related_docs:
  - label: Deploy AWS CDK and CDK Terrain with lifecycle hooks
    url: /tutorials/deploy-cdk-with-hooks
  - label: Lifecycle hooks
    url: /stacks/hooks
  - label: Terraform component configuration
    url: /components/terraform
  - label: Emulator commands
    url: /cli/commands/emulator/usage
---

## Notes

This example is the Terraform track of the
[Deploy AWS CDK and CDK Terrain with lifecycle hooks](https://atmos.tools/tutorials/deploy-cdk-with-hooks)
tutorial. [CDK Terrain](https://github.com/open-constructs/cdk-terrain) is the community fork of
CDK for Terraform (CDKTF), which HashiCorp deprecated; it keeps the same construct model and
lets you define Terraform configuration in a general-purpose language. A TypeScript Terrain app
in `components/terraform/greeting/` declares an AWS provider and one `aws_ssm_parameter`.
The project file is still named `cdktf.json` in this Terrain release, a holdover from CDKTF.
The component's `synth` hook installs dependencies when `package-lock.json` changed or the CLI is missing, then runs
`npm run synth` before `init`, `plan`, and `apply`, so
Terraform always reads a freshly generated `cdk.tf.json`, a Terraform configuration in JSON
syntax. No CloudFormation is involved; Terraform state tracks the resource directly.

The `local` stack declares a [Floci](https://github.com/floci-io/floci) AWS emulator and an
`aws/emulator` identity in `atmos.yaml` wires the AWS provider to it, so no AWS account is
needed. Remove that identity and the emulator import to deploy to a real account.

The companion [`cdk-cloudformation` example](../cdk-cloudformation) creates the same kind of
resource from an AWS CDK app through Atmos's native CloudFormation component.

Requirements: Node.js 22.19.0 or later (CDK Terrain's minimum), npm, Terraform or OpenTofu, and Docker or Podman for the
emulator.

## Usage

```shell
atmos validate stacks
atmos emulator up aws -s local
atmos terraform plan greeting -s local
atmos terraform apply greeting -s local
atmos terraform output greeting -s local
atmos terraform destroy greeting -s local
atmos emulator down aws -s local
```

Change `vars.message` and plan again to see the regenerated configuration. To apply a saved
plan with its original generated files, run
`atmos terraform apply greeting -s local --from-plan --skip-hooks=synth`.

Run the whole lifecycle with `atmos test`.
