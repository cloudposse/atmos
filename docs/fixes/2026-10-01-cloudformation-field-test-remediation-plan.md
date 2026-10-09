# Fix: CloudFormation field-test remediation

**Date:** 2026-10-01

## Summary

Implemented the eleven field-test fixes and corrected the unsupported output-key syntax, as additive changes on the existing top PR #3100. The lower stack branches remain unchanged. The [remediation plan](../proposals/cloudformation-field-test-remediation.md) records the original findings and scope.

## Context

Real AWS testing at 6a8791b4dcaf7ababa00756acb585e947c28798d exposed failures that mock-only tests missed: bulk teardown broke output dependencies, named changesets omitted protection, output leaked a synthetic NoEcho canary, S3 prefixes doubled, and several documented source/hook/CI contracts did not work.

## Changes

- Reverse bulk deletion and defer YAML-function evaluation until per-component execution. This also permits initial bulk apply before a producer exists.
- Enable configured termination protection after successful named changeset execution.
- Mask outputs using deployed NoEcho metadata, including intrinsic, resource and condition dependencies, at the presentation boundary. Fail closed on unreadable/malformed metadata; redact unknown output definitions. Internal dependency values remain unchanged; explicit masking opt-out remains supported.
- Validate dry-runs before dynamic execution, authentication, downloads and hooks. Defer dynamic template/target values while rejecting malformed static values.
- Apply S3 prefixes once, reject numeric/malformed StackSet targets before mutation, and infer an unambiguous provisioned template file.
- Preserve query-sensitive source cache identity independently of redacted provenance metadata.
- Match canonical drift hook names, forward `--ci` through the parser, count only modified/deleted resources, distinguish unknown/unchecked/clean drift, and render valid reproduction commands.
- Isolate startup-banner tests from the process-tree notice sentinel inherited by `atmos test`, preserving the assertions and runtime behavior.
- Correct CLI/skill documentation, update the release notes/roadmap, and regenerate affected CloudFormation help recordings. Move detailed operational examples into a skill reference to satisfy the 20 KB entrypoint limit.
- Document changed function and regression-test contracts to address CodeRabbit's documentation warning, without changing non-comment Go tokens.

## Validation

Failing regressions were reproduced before their fixes. Tests exercise real stack processing, fresh subprocess masking, the real S3 artifact backend against loopback, source retrieval/cache reuse, graph execution, Cobra dispatch, hook subprocesses, and the CI summary pipeline.

Passed locally:

- Affected CloudFormation, source provisioner, hooks, CI plugin, schema and command package suites.
- Startup-banner package under the inherited sentinel/FIPS environment with five shuffled repetitions, and through the actual Atmos test command.
- Repeated behavioral regressions with `-race -shuffle=on` (three or five repetitions depending on the suite).
- `go build ./...`, `atmos build`, `atmos lint --changed` (zero issues), and `npm run build`.
- CloudFormation help recording generation/validation through the repository's casts command.
- Local coverage-profile comparison: remediation 250/250 measured added lines; top PR 434/435. This is touched-package coverage, not full-suite Codecov. CI's Codecov patch report remains authoritative.

A fresh binary repeated the AWS repros in dev and sandbox using an isolated fixture and unique ownership prefix. Consumer-before-producer deletion succeeded twice with the live output reference intact. The second bulk apply started from both stacks absent. Named CREATE/UPDATE enabled protection; deletion without its explicit disable flag was rejected. Fresh output processes masked the canary even with stale local parameters and a missing template. Large-template apply with a nonempty prefix succeeded twice; actual S3 keys matched the URL. Numeric StackSet accounts failed before mutation; quoted accounts created/updated a sandbox instance. All four drift hooks fired; final real CI output showed `IN_SYNC`, zero drifted resources, and a valid command. Source-only render/cache reuse and non-executing dry-runs passed.

All owned resources were destroyed through Atmos. Independent inventories confirmed no owned active stacks (including StackSet children), StackSets, SSM parameters, IAM roles or S3 buckets remained in either account. The source cache was deleted and loopback server terminated. Local fixture, ownership ledger and cleanup proof are retained under `.context/field-test-cfn-remediation-20261001/` beside the repair checkout.

The repository-wide short runner exceeded its fixed five-minute acceptance timeout; broader local validation uses the configurable acceptance runner with short-test selection and its standard 40-minute budget. A fresh shared clone initially failed go-git object lookups; copying Git objects locally and setting a fixed terminal width resolved the targeted checkout-dependent failures without code changes. Latest-head cross-platform CI, authoritative Codecov coverage and CodeRabbit review results are tracked on [PR #3100](https://github.com/cloudposse/atmos/pull/3100).

## Follow-ups

None.
