<!-- atmos:template -->
# {{ .Config.project_name }}

A full-featured [Atmos](https://atmos.tools) landing zone for AWS, runnable end
to end on the [Floci](https://floci.io) emulator — no real AWS credentials
required.

## Capabilities

- **Native backend provisioning** — Atmos creates the S3 state bucket
  automatically before `terraform init` (`provision.backend.enabled` on the
  `bucket` component); no separate backend component is needed.
- **Secrets** — AWS SSM Parameter Store and Secrets Manager stores, consumed via
  `!secret` in the `secret-consumer` component.
- **Custom commands** — `atmos test` (full provision/destroy) and an
  `atmos floci up|down|reset` group.
- **Workflows** — `provision` and `validation` workflows under `stacks/workflows/`.
- **Validation** — JSON Schema + OPA policies guard the `bucket` component.
- **Environments** — `dev`, `staging`, and `prod` stacks built from stage mixins.

## Layout

```
atmos.yaml                       Atmos config: stores, schemas, workflows, commands
components/terraform/            bucket (native backend), secret-consumer
stacks/mixins/floci.yaml         Points the AWS provider at the emulator
stacks/mixins/stage/             dev / staging / prod mixins
stacks/catalog/                  Abstract component definitions
stacks/deploy/<stage>/           Concrete per-stage stacks
stacks/workflows/                provision + validation workflows
stacks/schemas/                  JSON Schema + OPA policies
```

## Try it

```sh
# 1. Start the Floci emulator.
atmos floci up      # or: docker compose up -d

# 2. (optional) Isolate secret paths per run.
export FLOCI_SECRETS_SSM_PREFIX="/atmos/{{ .Config.namespace }}/ssm"
export FLOCI_SECRETS_ASM_PREFIX="atmos/{{ .Config.namespace }}/asm"

# 3. Validate, provision, and tear down everything against Floci.
atmos test
```

When you are ready to target real AWS, remove the `mixins/floci` imports and the
`floci-superuser` identity, drop the custom `endpoints`/`use_path_style`/`access_key`
settings from the `bucket` backend, and configure real provider/auth settings.
