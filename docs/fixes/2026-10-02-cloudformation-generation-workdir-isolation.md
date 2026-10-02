# Fix: CloudFormation generation with isolated workdirs

**Date:** 2026-10-02

## Summary

CloudFormation components can opt into `generate:` with
`components.aws/cloudformation.auto_generate_files: true` (default false).
Generated templates and policies are prepared in isolated stack/component
workdirs before local-template operations consume them.

## Context

PR #3100's published tip, `a19b8df12387026ba9c14e21fdc57ac2ca7fae7b`, did not
preserve or execute CloudFormation generation blocks. Shared component files
would also be unsafe for concurrent instances with different generated values.
The implementation was developed in a separate checkout to preserve existing work.

## Changes

- Preserve global, component-type, inherited, component and override generation
  configuration; extend configuration and manifest schemas.
- Reuse the generation engine and inspect both aggregate and per-file errors.
  Require an explicit template `path`, or an inline template with auxiliary files.
- Enable canonical CloudFormation workdirs automatically for nonempty enabled
  generation. Reject explicit disable, shared working-directory overrides and
  generated paths escaping the isolated directory.
- Resolve local/source files and nested source paths before generation. Keep source
  pull, forced refresh and delete on the same isolated destination.
- Generalize local workdir provisioning while retaining Terraform defaults,
  lockfile restoration and legacy migration behavior.
- Preserve dry-run and deployed-stack operation boundaries; named changeset
  execution prepares a configured generated policy without reading its template.
- Update documentation, example and CloudFormation agent skill. Isolation applies
  to distinct stack/component instances; it does not serialize deployments of
  the same AWS stack.

## Validation

- CloudFormation, local workdir, component, source-command, stack-processing,
  generation, schema/datafetcher and CloudFormation CLI suites passed.
- Race tests passed for CloudFormation, workdir, component and source-command
  packages, including concurrent subprocesses sharing an unchanged source tree.
- Acceptance coverage includes exact generated template/policy bytes at the AWS
  client boundary, YAML/JSON/string serialization, inheritance/overrides, entirely
  generated and nested source components, reruns, failure paths, dry-run and
  deployed-operation isolation, and named changeset policy generation.
- `go build ./...` and changed-code custom lint passed. Website dependency
  installation and production build passed. The generated example rendered using
  the freshly built CLI.
- Live disposable deployments in dev and sandbox verified concurrent generated
  templates/policies, authenticated HTTP sources, cross-account packaging,
  self-managed sparse StackSet teardown, policy-denied rollback, and two PTY
  interruptions followed by AWS rollback and cleanup.
- SSM and Secrets Manager set/get/rotation/validation and `!secret` deployment
  passed with ambient credentials; configured store identities exposed failures
  recorded separately. CLI secret output was masked.
- Explicit dependency `kind` enabled Terraform/CloudFormation dependency discovery,
  affected propagation and output consumption. Type-specific bulk commands stayed
  within their own component type. Graph and CloudFormation output references also
  passed for Helmfile, Packer, Ansible, Helm, Kubernetes, Container and Emulator
  manifests; these other runtimes were not deployed.
- Independent inventories verified no run-owned stacks, StackSet children,
  parameters, secrets (including scheduled deletion), buckets, object versions or
  IAM roles remained across both accounts and us-east-2/us-west-2.
- Exact commands, fixtures, outcomes and cleanup evidence are retained under the
  originating workspace's `.context/field-test-cfn-gaps-20261001-2314/`. S3 source
  success is blocked by the downloader's unsupported scheme. Organizations and
  service-managed StackSet changes were excluded.

## Follow-ups

[#3247](https://github.com/cloudposse/atmos/issues/3247) tracks reproduced findings
in delivery/store identities, secret validation credentials, advertised S3 sources,
failed-operation CI evidence, independent CI output emission and cold Terraform
output behavior. These were investigated without expanding this implementation's
scope to fix them.
