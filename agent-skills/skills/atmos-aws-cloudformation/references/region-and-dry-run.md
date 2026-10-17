# CloudFormation Region Resolution and Static Dry-Run

Detail for [the CloudFormation skill](../SKILL.md).

## Region resolution

CloudFormation API calls need an AWS region, resolved most-specific-wins:

1. `settings.aws_cloudformation.region` on the component
2. The active identity's region
3. The AWS SDK's default credential/region chain (`AWS_REGION`, shared config)

A component `env` that sets `AWS_REGION`/`AWS_DEFAULT_REGION` does not change the region; Atmos
warns when it differs from the resolved region and points to `settings.aws_cloudformation.region`.

There is no per-component account override outside StackSets — the account is always the active
identity's account.

## Static dry-run

`--dry-run` defers YAML functions, Go templates, source downloads, authentication and hooks,
including with `render` and bulk/affected selection. It validates known static fields and reports
execution-dependent checks as deferred. Use a normal `render` to inspect a provisioned template.
Bulk deletion reverses dependency order so consumers can still resolve producer outputs.
Named changeset execution enables configured termination protection after successful completion,
just as apply does; false never disables existing protection.
