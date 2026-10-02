# Fix: CloudFormation integration failures found before merge

**Date:** 2026-10-02

## Summary

Fix the seven integration failures reproduced while field-testing CloudFormation
before merge: delivery identities, secret evaluation, S3 sources, failed-operation
CI evidence, independent CI outputs, and null dependency parameters.

## Context

Live tests based on PR #3100's published tip exposed failures beyond generation
and workdir isolation. Each unexpected behavior was reproduced twice. These fixes
are included in this change at the user's request; no follow-up issue is needed.

## Changes

- Resolve delivery-target authentication in an independent scope, merging global,
  component and target configuration while preserving explicit CLI identity
  precedence. Apply the scope to direct deployments, StackSet creation/update,
  S3 packaging, backend commands and Git delivery. Default direct-target output
  references use the same scope, including identity-sensitive template caching.
- Bind secret-store authentication before CloudFormation stack evaluation.
  Secret discovery evaluates declarations and their local template dependencies
  without resolving unrelated deployed outputs.
- Restore S3 source downloads through AWS SDK v2 with request-scoped credentials,
  lazy authentication for cold local-template operations, versioned object and
  prefix support, archive extraction, cancellation and destination path guards.
  Warm sources and deployed-stack-only operations retain their existing boundaries.
- Preserve changeset identifiers, resource changes and final stack status when an
  apply fails. Attempt CI output independently if writing the summary fails.
- Reject null CloudFormation parameter values with an actionable producer/fallback
  error before mutation. Preserve Terraform's documented missing-state null
  behavior, explicit empty strings and `UsePreviousValue`.
- Update the CloudFormation documentation and delivery-target skill reference.

## Validation

- Focused CloudFormation, backend, secret, downloader, source and output-reference
  regression tests passed. CloudFormation/CI, downloader and secret/source race
  suites exercise credential isolation, independent error channels and sources.
- Live configured-identity secret validation passed twice. Secret-backed
  CloudFormation apply and Secrets Manager rotation/update passed with masking.
- Live S3 cold/warm render, validation, generated deployment, force refresh and
  source-independent output/delete passed.
- A dev S3 delivery target with a sandbox component default uploaded successfully
  twice. Explicit sandbox CLI override was denied twice, as expected.
- Two real WaitCondition failures rolled back while retaining changeset names,
  resource changes, `has_changes=true` and `ROLLBACK_COMPLETE` in CI output.
  Two successful applies wrote outputs despite an unwritable summary destination.
- Cold Terraform output consumers failed clearly on null parameters twice.
- Default direct-target deployment and both output-reference forms passed live
  twice in dev; an explicit CLI override selected sandbox twice, and returning
  to implicit selection restored dev. Describe now carries the original identity
  request from its authenticated manager into nested evaluation.
- Final independent cleanup verified 20 empty inventories across dev and sandbox
  in us-east-2 and us-west-2, including StackSet children, secret deletion state,
  versioned objects, parameters and IAM roles.
- After stacking on the latest published baseline, `atmos test --full` passed
  for CloudFormation, workdir, source, config-schema and datafetcher packages.
  Focused race tests passed for the describe identity and cache regressions.
- `go build ./...`, patch-scoped `atmos lint --changed`, the generated-schema check
  and the website production build passed. Exact fixtures, commands and AWS
  evidence are retained under the originating workspace's
  `.context/field-test-cfn-gaps-20261001-2314/`.

### Review validation

- Preserve the original CLI identity request when deferred stores resolve credentials
  after component authentication. Six cases cover explicit, implicit, legacy,
  store-specific, interactive-selection and disabled-selection values.
- Normalize S3 object-key separators before inspecting every destination ancestor,
  including intermediate links followed by missing directories. Regression tests
  assert rejection before object retrieval and no writes outside the destination.
- Full S3 downloader and deferred-store tests, race tests and affected builds passed.
  Package statement coverage is 93.9% and 100%, respectively. The broader
  patch-scoped package test run and changed-line lint also passed.
- Direct shared-auth tests cover all target-auth functions, including identity
  precedence, independent contexts and error propagation (100% function statement
  coverage). Workdir tests cover all component-option helpers and verify real
  Terraform state migration, lock restoration and CloudFormation source isolation.

## Follow-ups

None.
