# Teardown of Deleted Components

**Status: Proposal — not implemented.** This document describes a possible explicit teardown workflow for components removed from Atmos configuration. It does not change the meaning of any existing command. CLI syntax, the first supported component types, and rollout scope require a separate implementation decision.

## Problem

Removing a component or stack from YAML is a configuration change, not an instruction to delete deployed infrastructure. `atmos describe affected` already reports deletions, but completing teardown requires the configuration and code that existed before removal.

Today, `atmos terraform destroy --affected` uses the shared Terraform affected-selection path, which filters out deleted components. It selects affected Terraform components still present in the current configuration. Reinterpreting that command as “destroy only removed components” would change existing behavior and could select a different set of resources than users expect.

Users currently need their own orchestration to retain comparison revisions, recover the old configuration, order destruction, and review the resulting operations. A first-class deletion workflow could make those steps explicit and reproducible.

## Proposed User Experience

Introduce an **explicit deletion selection mode**, separate from the existing affected selection semantics. The eventual syntax could be a dedicated command or an option; this proposal does not reserve a command or flag.

The workflow has two phases:

1. **Prepare:** compare immutable base and head commits, select deletion candidates, recover their configuration, resolve deployed identities, check dependencies, and produce a reviewable teardown manifest plus engine-specific previews.
2. **Execute:** consume the approved manifest and previews, verify that the reviewed inputs and target identities still match, then perform teardown in dependency order.

A deletion record is a candidate, not approval. The default selection includes only components deleted between the two revisions. Modified and newly added components are excluded. Do not implicitly add surviving dependents or shared prerequisites to the destruction set.

## Revisions and Configuration Recovery

- Resolve both comparison refs to commit SHAs before discovery. Record those exact SHAs and the repository identity in the teardown manifest. An event's original base commit remains the recovery source after merge; the latest `main` may already lack the deleted files.
- Materialize the base snapshot in an isolated checkout. Load the old stack configuration, imports, backend/workspace settings, component code, and applicable tool versions there, without changing the user's current checkout.
- Recover external sources at the versions or digests required by the base configuration. Floating refs and mutable artifacts must be resolved and recorded during preparation. If the prior deployment's source cannot be identified, report that uncertainty and require explicit resolution; a base commit alone does not prove which remote content was deployed.
- Compare resolved deployed identities across base and head, including Terraform backend/workspace and CloudFormation account, region, and stack name. A deleted Atmos name and an added name may identify the same deployment. Block teardown when there is an identity collision or unresolved rename ambiguity.
- Retain the manifest, source provenance, and engine previews until the teardown completes or is explicitly abandoned. Treat recovered hooks and scripts as executable repository code, subject to the same review and credential boundaries as deployment code.

## Ordering and Eligibility

Build the dependency graph from the base configuration, since deleted nodes and their edges may no longer exist in head. Reverse the relevant edges for teardown: dependents precede prerequisites.

Evaluate incoming edges from both snapshots. If a surviving component depends on a deletion candidate, block that candidate and identify the consumer. Do not automatically destroy the consumer. The user must first migrate or remove the dependency and prepare a new reviewable operation. Declared dependencies cannot detect every external consumer; state this limit in the preview.

Respect `metadata.locked` from the base snapshot and any surviving identity-equivalent component in head. Locked or unsupported candidates should be visible with a blocked reason rather than silently omitted. Abstract components are configuration blueprints, not teardown targets. Disabled definitions need an explicit policy: disabling execution does not prove that deployed resources are absent.

Keep approval and dependency ordering separate. An approved candidate still waits for selected dependents to complete successfully. Independent branches may run concurrently only when the adapter supports it. A failed dependent prevents teardown of its prerequisite; there must be no fallback to unordered execution.

## Component Capability Matrix

This matrix describes integration points and proposed requirements, not support already delivered by this workflow.

| Component type | Existing lifecycle operation | Proposed preview | Requirements and limits |
| --- | --- | --- | --- |
| Terraform/OpenTofu | `terraform destroy`; destroy planning with the engine | Saved destroy plan when supported by the backend/execution mode | Recover root module, inputs, providers, backend, and workspace. Bind execution to the reviewed plan; reject unsupported saved-plan modes or design a separately reviewed alternative. |
| Native CloudFormation | `aws cloudformation delete` | Resolved target, deployed resources, retention policies, and termination-protection status | CloudFormation deletion does not provide the same saved-plan contract as Terraform. Respect termination protection; any override must be an explicit reviewed choice. |
| Helmfile | Underlying Helmfile destroy operation | Selected releases, namespaces, cluster identity, and supported preview information | Recover release selection and environment values. Do not describe a release list as a Terraform-style destruction plan. |
| Custom components | Only an explicitly supplied teardown command or adapter | Adapter-defined target and preview contract | No inferred command names and no arbitrary script execution merely because a component disappeared. Without an explicit contract, report unsupported. |
| Other types | No universal destruction lifecycle | None by default | Image builders, artifacts, and other tools require their own lifecycle design; absence from YAML does not imply deletion of their products. |

Use a common candidate/manifest model while allowing each adapter to describe its preview guarantees. A generic component type is not evidence that the same destructive operation is valid for every tool.

## Review, Credentials, and Execution

The manifest should contain the candidate's component type/name/stack, reason for deletion, base/head SHAs, deployed identity, source and tool provenance, dependency ordering, eligibility findings, preview references, and operation status. Exact serialization and versioning are implementation decisions.

Resolve credentials at execution time through the supported authentication system; never serialize credentials or resolved secrets into the manifest. Separate the engine identity from the Atmos stack name so reviewers can see the account, region, cluster, or backend actually targeted. Preview and execution must resolve to the same deployed identity even if credentials rotate.

Require an explicit execution action after preparation. CI can connect that boundary to its existing approval mechanism. Before each destructive step, validate the manifest, recovered input digests, target identity, current protection/lock settings, and the engine's plan validity. If these checks fail, require a new preparation or review rather than regenerating and applying a different plan silently.

Deletion previews and saved plans may contain sensitive values. Store them with appropriate access controls and retention, and redact human-readable logs using the existing secret-handling facilities.

## Failures, Retries, and Recovery

- Persist per-candidate outcomes and engine operation identifiers so a retry can distinguish completed, failed, and in-progress operations.
- On retry, query the engine to determine whether the original target is absent, still deleting, or failed. Treat absence as completion only when the recorded target identity has been verified. Do not retarget a newly created deployment that happens to reuse an Atmos name.
- Preserve successful outcomes; do not blindly repeat destructive steps or lifecycle hooks. Document hooks that cannot safely be retried.
- Reuse reviewed previews only while their engine-specific validity checks pass. Stale Terraform plans require a new plan and review.
- Surface partial completion and blocked prerequisites. Cancellation must stop scheduling additional work and report any remote operation that may still be running.

## Acceptance Scenarios for a Future Implementation

- Deleting one component or an entire stack produces deletion-only candidates; editing a component does not.
- Existing `destroy --affected` selection remains unchanged.
- A post-merge run recovers the original base SHA even after the target branch advances.
- Missing source artifacts or mismatched source digests stop preparation/execution with actionable diagnostics.
- A base dependency chain tears down in reverse order, including cross-stack edges; a failed dependent blocks its prerequisite.
- A surviving dependent, locked target, unsupported adapter, or unresolved rename blocks the affected candidate without hiding it.
- Renaming Atmos configuration while preserving deployed identity does not produce an executable destruction candidate.
- Approval binds to the prepared target and preview. Changes to inputs, identity, or plan validity require renewed preparation/review.
- Credentials can rotate without being stored in artifacts; changes in the actual deployed target are detected.
- Partial failures, retries, cancellations, already-deleted targets, and protection/retention policies produce explicit outcomes.
- Mixed component types report their distinct preview and execution guarantees; unsupported types never fall back to an inferred teardown command.

## Decisions Before Implementation

Choose the command/flag surface, manifest schema and retention, initial adapters, approval integration, disabled-component policy, handling of mutable sources, and any narrowly scoped overrides for blocked candidates. Terraform/OpenTofu is a practical first adapter because saved destruction plans provide a concrete review artifact; this is a recommendation, not a committed rollout scope.

The documentation update accompanying this proposal describes only current behavior. Publish instructions for the proposed workflow only after implementation and acceptance testing.
