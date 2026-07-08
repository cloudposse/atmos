<!-- atmos:template -->
# [[ .Config.project_name ]]

An [Atmos](https://atmos.tools) landing zone for GCP: flat `dev`, `staging`,
and `prod` environments with a small emulator-proven baseline.

The scaffold provisions only resources verified against `floci/gcp`: GCS,
Secret Manager, service accounts, and bucket IAM. Pub/Sub, KMS, and Cloud
Logging are intentionally omitted because the current emulator does not expose
Terraform-compatible REST APIs for them.

## Quick start

```shell
atmos test
```

To work with one environment:

```shell
atmos emulator up gcp -s dev
atmos terraform apply --all -s dev -i false
atmos terraform output foundation -s dev
atmos emulator down gcp -s dev
```

## Layout

```
atmos.yaml
components/terraform/foundation/
stacks/_defaults.yaml
stacks/dev.yaml
stacks/staging.yaml
stacks/prod.yaml
```

Each environment is a single stack file that imports `_defaults.yaml` and
overrides only the values that differ for that stage.

`stacks/_defaults.yaml` pins the Terraform/OpenTofu toolchain version via
`dependencies.tools` (https://atmos.tools/cli/commands/toolchain).
