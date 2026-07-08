<!-- atmos:template -->
# [[ .Config.project_name ]]

An [Atmos](https://atmos.tools) application SDLC repository for AWS. It has
flat `dev`, `staging`, and `prod` stacks, one deployable `app` component, and
native CI configuration for GitHub Actions.

The app component uses only resources that run on the local AWS emulator: S3,
SQS, and SSM Parameter Store.

## Quick start

```shell
atmos test
```

To work with one environment:

```shell
atmos emulator up aws -s dev
atmos terraform apply app -s dev -i false
atmos terraform output app -s dev
atmos emulator down aws -s dev
```

## Layout

```
atmos.yaml
.github/workflows/deploy.yml
components/terraform/app/
stacks/_defaults.yaml
stacks/dev.yaml
stacks/staging.yaml
stacks/prod.yaml
```

Use this scaffold for an application repository that deploys into already
created environments. Use `aws/landing-zone` for the environment foundation
itself.

`stacks/_defaults.yaml` pins the Terraform/OpenTofu toolchain version via
`dependencies.tools` (https://atmos.tools/cli/commands/toolchain).
