# CloudFormation field-test remediation plan

Date: 2026-10-01
Status: implemented and locally/field validated; latest-head CI and review gates pending.

See the [fix record](../fixes/2026-10-01-cloudformation-field-test-remediation-plan.md) for implementation and observed validation.

## Delivery decision

Apply every remediation as additive commits on the existing top branch, `osterman/cfn-phase5-native-ci`, in [PR #3100](https://github.com/cloudposse/atmos/pull/3100). Keep its current base, `osterman/cfn-phase4-code-rebuild`. Do not redistribute fixes into earlier PRs, rebase the stack, rename branches, or open another PR.

Verified planning baseline: `6a8791b4dcaf7ababa00756acb585e947c28798d`. Snapshot all stack PR heads before implementation and check again at delivery: only #3100 should change. Work in the isolated repair checkout; preserve the original workspace's staged skill edits.

The top PR already contains the cumulative implementation, so it can fix code introduced by any earlier phase. The tradeoff is that lower PRs remain individually defective; the field-test fixes become available when #3100 lands. The complete stack through #3100 is the release/sign-off unit. No merging is part of this work.

Proposed final title: **feat(cloudformation): native CI and field-tested lifecycle hardening**. Rewrite its description around the final behavior and validation, with a finding checklist and coverage evidence.

## Scope and approach

Close all eleven confirmed implementation/contract findings plus the output-by-key documentation mismatch from the 2026-10-01 field report. The local evidence is in `../field-test-cfn-20261001/` beside this checkout.

Implement fixes in Atmos's Go/config paths. Do not add shell wrappers, CI workaround actions, Helm-specific detours, or change CI shard count/parallelism. Preserve existing APIs and CLI syntax except implementing the already-documented `--ci` flag. Correct the unsupported output-key claim in documentation instead of adding a new positional API.

Use focused commits and regression tests per batch. Consolidate validated batches into one initial push to #3100, then handle CodeRabbit findings in validated batches. No cloud work or PR mutation is needed to prepare this plan.

## Batch 1 — lifecycle safety (findings 1–3 and 7)

### 1. Reverse bulk deletion dependencies

Location: `pkg/component/aws/cloudformation/executor_bulk.go` and graph regression fixtures.

Set reverse dependency order for stack deletion, while keeping normal dependency order for apply. Confirm graph discovery does not re-resolve deleted producers before consumers finish. Do not change unrelated operations or implicitly broaden the selected set.

Tests: a real graph with producer/consumer and a YAML output reference; assert consumer-before-producer deletion and no remaining resources. Exercise all, tags, labels, combined selectors, and affected selection through a local baseline repository. Assert apply remains producer-first and selectors are cleared before per-node dispatch. Validate selection/cycle errors before any deletion.

Acceptance: the exact field-test bulk-delete command succeeds twice without editing away the output reference.

### 2. Align named changeset post-execution safety with apply

Location: `changeset_verbs.go`, shared apply finalization/protection helpers, and policy execution tests.

After successful named CREATE/UPDATE completion, enable configured termination protection using the same helper as apply. Preserve pre-update and post-create policy ordering, and the rule that a false setting never silently disables existing protection. Return an actionable failure if protection could not be enabled after deployment; do not report full success.

Tests: named CREATE/UPDATE, protection true/false, already protected, execution failure, rollback, policy failure, protection API failure, and declined confirmation. Update the test that currently expects protection to be omitted.

Acceptance: independent AWS inspection reports protection true after each named execution; delete still requires the explicit disable flag.

### 3. Mask standalone output before rendering, without source provisioning

Location: `output.go`, `parameters.go`, output execution/formatter boundary and tests.

Preserve deployed reads when the local template is missing. Do not fix masking by making output vendor sources or require local template files. Retrieve deployed template metadata via the existing CloudFormation client interface, identify NoEcho parameters, and register available resolved parameter values before rendering.

Also redact outputs whose deployed template expressions reference NoEcho parameters, including when configured values have changed since deployment or AWS returns redacted parameter values. Prefer a conservative redaction when an expression's sensitivity cannot be safely resolved. Do not invent a guarantee that arbitrary transformed secrets can be reconstructed or recognized by value.

Keep masking at the presentation boundary: internal output lookups used to build other components must receive real values, never `<MASKED>`. Respect the existing explicit masking controls. When masking is enabled and required sensitivity metadata cannot be obtained/parsed, return an actionable error before emitting raw output. Document the deployed-template read permission and precise masking limits.

Tests: fresh CLI processes, JSON/YAML/other format adapters, direct Ref and intrinsic expressions, safe outputs, defaults, stale/missing configured values, absent local/source template, template API/parse failures, and explicit masking opt-out. Assert no canary appears on stdout, stderr, logs or enabled summary output, and no source fetch occurs. Cross-component output resolution must still return the real value internally.

Acceptance: deploy and fresh output both mask the synthetic canary; harmless outputs remain usable. This is the most design-sensitive change and gets its own review checkpoint before the publication batch.

### 7. Make dry-run safe before stack evaluation

Location: single and bulk executors, affected discovery, and existing deferred stack-processing facilities.

Choose a non-executing stack-resolution path before evaluating YAML functions, external template functions, authentication, source provisioning or hooks. Apply it consistently to single, bulk, affected and render-plus-dry-run paths. Perform static validation only; report unresolved dynamic values as deferred instead of evaluating them to fabricate a complete preview.

Tests must use the real processing pipeline and harmless markers/request counters, not stub out ProcessStacks. Cover !exec, output/store lookups, template-driven external reads, hooks and sources; assert zero executions/downloads/cloud calls/auth prompts. Include malformed static config and dynamic fields that cannot be fully validated without execution.

Acceptance: repeated dry-runs leave marker files, request counts and AWS inventories unchanged.

## Batch 2 — delivery and source contracts (findings 4, 5 and 8)

### 4. Assign the S3 prefix once

Location: `packaging.go`, S3 backend integration tests.

Pass an unprefixed relative artifact key to the backend; construct the public TemplateURL with the effective prefix exactly once. Leave shared artifact-store prefix behavior intact for other callers. Keep metadata sidecar naming, checksum and content addressing consistent with the actual object key.

Tests: exercise the real backend/key-building composition against a local SDK HTTP endpoint, rather than replacing the entire backend with a mock. Empty, nonempty and nested prefixes; URL escaping; sidecar key; repeat upload; publish-only and automatic large-template delivery.

Acceptance: requested URL key equals uploaded key; large-template apply with `prefix: templates` succeeds twice.

### 5. Reject invalid StackSet target types before mutation

Location: `stackset.go` target decoding/validation.

Require account IDs to be quoted 12-digit strings; reject numeric items and mixed-type lists with the exact config path and a quoted example. Do not silently drop items or guess leading zeroes. Retain valid scalar/list shorthand where already supported, and distinguish intentionally omitted targeting dimensions from malformed supplied values. Apply validation before CreateStackSet/UpdateStackSet and in static dry-run.

Tests: numeric/scalar/mixed input, leading-zero quoted ID, wrong length/type, malformed regions, valid quoted accounts, omitted targets and both create/update. Assert zero SDK mutations on invalid config.

Acceptance: the previous numeric fixture errors clearly and creates no empty StackSet; the valid cross-account control still creates/updates/deletes its instance.

### 8. Implement single-file source inference

Location: component/spec validation, source provisioning/result handling and their regression tests.

Allow a source-only component through preliminary validation; after provisioning, infer the template path from a confirmed single-file result. Preserve explicit path and inline-template precedence/conflict rules. Reuse existing provisioner metadata rather than guessing every source URI is a file or selecting an arbitrary directory entry. Ambiguous directory/archive contents must request an explicit path.

For deployed read/delete operations, source-only configuration must still identify the stack without fetching its source. Dry-run must defer source-dependent inference without making a request.

Tests: loopback file URI with query string, basename/escaped characters, cached/forced retrieval, explicit path, directory and ambiguous result, failed download, and source-only deployed reads with zero requests. Preserve fixture contents and use workdir-aware paths.

Acceptance: the exact documented source-only example renders correctly; cache, explicit paths and dry-run remain safe.

## Batch 3 — hooks and native CI (findings 6 and 9–11)

### 6. Preserve meaningful hyphens in hook events

Location: `pkg/hooks/hook.go`, event normalization and shared hook tests.

Match canonical event names before legacy hyphenated aliases, or normalize event structure without rewriting hyphens inside command names. Retain existing Terraform/Helm/Kubernetes aliases. Verify actual configured hook execution, not just the executor's event table.

Tests: before/after drift-detect and drift-describe marker hooks; legacy event spelling; plan/deploy aliases; skip-hooks and success/failure outcomes. Exercise shared hook users to catch regressions.

Acceptance: each expected drift marker appears exactly once per invocation, and read-only verbs without configured events remain silent.

### 9. Wire --ci through the actual parser

Location: CFN command registration, operation flag extraction and CI dispatch.

Register the documented flag for the supported lifecycle/CI commands using the standard parser and forward it through single and bulk execution. Preserve ci.enabled, summary.enabled, environment and provider behavior. Do not duplicate inherited global flags.

Tests: real Cobra/CLI parsing for canonical commands and aliases; --ci versus ATMOS_CI/CI, disabled gates, generic and GitHub file destinations, and dry-run suppression. No real PR publication is needed for these tests.

### 10. Count only drifted resources

Location: `drift.go`, `ci.go`, plugin templates/tests.

Compute an explicit typed drift count using actual drift statuses; do not use the length of the unfiltered API response. Preserve a known zero. Keep no detection results, IN_SYNC, MODIFIED, DELETED and NOT_CHECKED distinct.

Tests: clean, mixed, deleted-only, empty and paginated results, with rendered summary assertions and fail-on-drift behavior.

Acceptance: a clean stack produces zero drift in both commands and summary; mixed results count only drifted resources.

### 11. Correct every summary reproduction command

Location: all five CloudFormation CI templates and their golden expectations.

Use `atmos aws cloudformation`. Correct canonical command/alias rendering consistently. Test that a generated command's command/flag structure parses through the actual CLI, using safe dry-run/test seams.

Regenerate goldens using the repository's supported mechanism; do not manually edit snapshot output. Confirm generic and GitHub summaries show usable commands.

## Batch 4 — documentation, durable tests and field verification (including finding 12)

Correct the skill's unsupported `output <component> [key]` syntax; retain the existing one-component CLI contract. Update the CFN skill, delivery reference, CLI/config pages and example hook comments for the fixed behavior: five event pairs, supported CI flag, source inference, dry-run limits, masking guarantees, quoted account IDs, preview cleanup and termination protection.

Inspect Helm/Helmfile skill guidance only where shared hook or masking behavior is affected; avoid unrelated rewrites. Preserve the original workspace's staged skill edits.

Move reusable regression fixtures into established repository test/example locations with synthetic canaries and dynamic names/ports. Retain the field report and cleanup proof locally. Update the accompanying fix log from planned to implemented with exact checks and results.

## Validation and publication gates

1. Establish a failing regression for every behavioral finding before its fix. Use CLI/process, real stack-processing and backend composition tests where unit seams hid the defect. Documentation-only corrections need targeted consistency checks.
2. Run affected Go package tests, race detection and shuffled repeated regression runs. Run relevant shared hooks, output/masking, provisioner and command/parser suites, then repository-required lint/build/acceptance checks. Never weaken assertions or add coverage exclusions to reach a percentage.
3. Require **at least 85% Codecov patch coverage on #3100**. Also inspect coverage on the remediation-only diff from 6a8791b4dc so previously covered Phase 5 lines cannot conceal uncovered new fixes.
4. Rebuild and repeat every field repro in dev, sandbox only for the cross-account case. Include happy-path controls, real PTY behavior and independent AWS state checks. Restore a pristine reproduction manifest first.
5. Keep a fresh ownership ledger and unique names. Clean up on success and failure: consumers before producers; StackSet instances before IAM roles; nested stacks before the artifact bucket; all bucket versions/markers. Independently verify no owned stacks, StackSets, SSM parameters, roles or buckets remain in either account.
6. Commit the validated changes using the repository's signing conventions. Push only the existing top branch; update #3100's title/body and finding checklist.
7. Require Linux, macOS and Windows checks to pass on the exact latest head. Obtain CodeRabbit approval and resolve actionable feedback after validated fixes. Respect its reported retry-after/rate-limit window; avoid repeated review triggers or unchanged CI reruns.
8. Verify lower PR heads stayed unchanged. Report the final SHA, CI, actual patch coverage, review state, all 12 finding dispositions and cleanup proof. No merging.

## Completion criteria

All 12 report items have a tested fix or the explicitly planned documentation correction; no known finding is deferred. The top PR alone carries the remediation, required checks pass, patch coverage meets the threshold, CodeRabbit has no unresolved actionable findings, and cloud cleanup is verified.

If implementation uncovers additional work that must be deferred, explicitly track it under the repository's follow-up rules rather than silently declaring this plan complete.
