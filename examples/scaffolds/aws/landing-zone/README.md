<!-- atmos:template -->
# [[ .Config.project_name ]]

An [Atmos](https://atmos.tools) landing zone for AWS: `dev`, `staging`, and
`prod` environments, each with a small, conventional baseline — encrypted audit
log bucket, encryption key, environment metadata, monitoring, and a deployment
role — all provisionable end to end on a local emulator with no AWS credentials.

## Quick start

```shell
atmos test
```

That validates the stacks, then for each environment starts the local
emulator, applies every component, destroys them, and shuts the emulator down.

To work with one environment interactively:

```shell
atmos emulator up aws -s dev
atmos terraform apply --all -s dev -i false
atmos terraform output monitoring -s dev
atmos emulator down aws -s dev
```

## Layout

```
atmos.yaml                    Atmos config: emulator identity, test command
components/terraform/
  kms/                        Baseline encryption key + alias
  audit-trail/                Encrypted, private audit log bucket
  baseline/                   Environment metadata in SSM Parameter Store
  monitoring/                 Log group, alerts topic, log-volume alarm
  iam-baseline/               Environment deployment role
stacks/
  _defaults.yaml              Shared foundation: emulator, state backend
  dev.yaml                    Development: disposable, short retention
  staging.yaml                Staging: production-like rehearsal
  prod.yaml                   Production: protected log bucket, long retention
```

Every environment is a single flat stack file that imports `_defaults.yaml`
and carries its own real configuration — no deep directory hierarchy.

## How it works

- **State backend** — Atmos natively provisions the S3 state bucket before
  `terraform init` (`terraform.provision.backend.enabled` in
  `stacks/_defaults.yaml`); there is no hand-rolled tfstate component.
- **Emulator** — the `local` identity (`kind: aws/emulator`) binds every
  component to the stack-scoped emulator component. Atmos injects the
  provider endpoint and credentials automatically; the components contain no
  `providers.tf` and no endpoint configuration.
- **Environments** — per-stage substance (retention, alarm thresholds, bucket
  protection) lives visibly in `dev.yaml` / `staging.yaml` / `prod.yaml`.

## Moving to real AWS

1. In `stacks/_defaults.yaml`, delete the emulator component and everything
   from `access_key` down in the backend block; pick a globally unique state
   bucket name.
2. In `atmos.yaml`, replace the `local` emulator identity with your real
   authentication (see [Atmos Auth](https://atmos.tools/cli/commands/auth)).
3. `atmos terraform plan --all -s dev` and review.

Note: this scaffold intentionally excludes CloudTrail because Floci does not
currently emulate that API. Add CloudTrail only when targeting real AWS or an
emulator version that supports it.

## Next steps

- Add your first workload component under `components/terraform/` and wire it
  into the stage stacks.
- For an application repository that deploys *into* these environments, see
  the `aws/app` scaffold and the
  [Application SDLC Environments](https://atmos.tools/design-patterns/stack-organization/application-sdlc)
  design pattern.
