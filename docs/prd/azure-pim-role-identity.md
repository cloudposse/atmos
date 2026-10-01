# Azure PIM Role Activation (`azure/pim-role`)

**Status**: Implemented **Last Updated**: 2026-09-30 **Owners**: Atmos auth subsystem

**Upstream references** (verified via Microsoft docs):

- PIM for Azure
  resources - [Activate my Azure resource roles](https://learn.microsoft.com/en-us/entra/id-governance/privileged-identity-management/pim-resource-roles-activate-your-roles)
- Azure RBAC
  REST - [Role Assignment Schedule Requests - Create](https://learn.microsoft.com/en-us/rest/api/authorization/role-assignment-schedule-requests/create)
  (`requestType: SelfActivate`, api-version `2020-10-01`)
- Azure RBAC
  REST - [Role Eligibility Schedule Instances - List For Scope](https://learn.microsoft.com/en-us/rest/api/authorization/role-eligibility-schedule-instances)
  (`$filter=asTarget()`)
- Azure RBAC
  REST - [Role Assignment Schedule Instances - List For Scope](https://learn.microsoft.com/en-us/rest/api/authorization/role-assignment-schedule-instances)
  (detect an already-active assignment)

**Related Atmos PRDs**:

- [PRD-Atmos-Auth](../../pkg/auth/docs/PRD/PRD-Atmos-Auth.md) (umbrella)
- [Azure Interactive Browser Authentication](azure-interactive-auth.md) (the provider a `pim-role` identity typically
  chains from)
- [AWS Assume Root Identity](aws-assume-root-identity.md) (the chaining "become more privileged" precedent)

---

## 1. Executive Summary

### Problem

Azure resource roles are increasingly made **PIM-eligible** (just-in-time) rather than standing, so an
operator holds a role only after activating it. Activation is a multi-step REST dance: enumerate the roles
you are eligible for at a scope, PUT a self-activation request with a justification and a time-boxed
duration, then poll until it is provisioned. There is **no native `az` command** for activating a PIM-eligible
Azure *resource* role, so operators hand-roll `az rest` calls or reach for third-party scripts. Because the
activation is time-boxed, this repeats every working session.

Atmos Auth already models the two things this needs - **identity chaining** (becoming something more
privileged, as with `aws/assume-root`) and **credential expiry/TTL** - but Azure has only `azure/subscription`.
There is no way to express "activate my eligible role, then run." Every downstream consumer (`--identity`,
`atmos auth exec`, the AKS kubeconfig exec plugin, MCP servers) therefore cannot benefit from a
single place that performs the elevation.

### Solution

A new identity kind **`azure/pim-role`** that chains from an existing Azure identity and, on authentication,
runs the ARM PIM self-activation for a role the authenticated principal is eligible for, scoped and time-boxed
by config. Because it lives in the identity chain, every consumer inherits it with no extra wiring.

Unlike `aws/assume-role` and `aws/assume-root`, it **mints no new credentials**. The activation elevates the *existing*
principal server-side - Azure RBAC evaluates roles by object id at request time, not as token
claims - so `Authenticate` returns the parent credentials unchanged, with the role now active. This is also
why it composes cleanly: downstream consumers keep using the same token, now carrying the activated role.

## 2. Design

### Identity kind

`azure/pim-role` - activates a PIM-eligible **Azure resource role** (Authorization RBAC) for the principal
established by the parent identity. Named for the mechanism (PIM role activation), consistent with the repo
convention that kinds name auth mechanisms.

```yaml
auth:
  identities:
    azure-dev:
      kind: azure/subscription
      via:
        provider: azure-interactive
      principal:
        subscription_id: "00000000-0000-0000-0000-000000000000"

    prod-contributor:
      kind: azure/pim-role           # new
      via:
        identity: azure-dev          # elevate from who I already am
      principal:
        role_definition_id: "/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"
        scope: "/subscriptions/00000000-0000-0000-0000-000000000000"
        duration: "8h"               # Go-style; converted to the ISO-8601 the API wants, capped at the PIM policy max
        justification: "planned change window"   # optional default; see care points
```

Principal fields: `role_definition_id` (required, full role definition id - the API needs the id, not a
name), `scope` (required - subscription, resource group, or resource ARM id), `duration` (optional Go-style
duration converted to the `scheduleInfo` ISO-8601 the API requires, capped at the role's PIM activation-policy
maximum), `justification` (optional default). `via.identity` chains from the identity whose token holds the
eligibility; `via.provider` is allowed but less common.

### Authentication flow

`Authenticate(ctx, baseCreds)` uses the management token from `baseCreds` and runs the ARM PIM sequence:

1. **Resolve** the principal object id from `baseCreds`.
2. **Short-circuit if already active** - `GET roleAssignmentScheduleInstances` at scope filtered to the
    principal and role; if an active instance already covers the scope, return `baseCreds` unchanged. This
    keeps the identity from filing a request on every command.
3. **Enumerate eligibility** - `GET roleEligibilityScheduleInstances` at scope (`$filter=asTarget()`); find
    the matching eligible assignment and its `roleEligibilityScheduleId`. A missing eligibility is a distinct,
    actionable error: this kind *activates* an eligibility, it does not grant one.
4. **Attach to a pending request** - if a `roleAssignmentScheduleRequest` for this principal, role, and scope
    is `PendingApproval`, attach to it rather than creating a duplicate.
5. **Self-activate** - `PUT roleAssignmentScheduleRequests/{guid}` (api-version `2020-10-01`) with
    `requestType: SelfActivate`, the principal and role-definition ids, the linked eligibility schedule id, the
    justification, and `scheduleInfo.expiration` (`AfterDuration`, the configured duration).
6. **Poll** `PendingApproval -> Provisioned`, then return `baseCreds`. Note that Azure RBAC propagation can
    lag the `Provisioned` status by a few minutes server-side; consumers that hit a transient `403` immediately
    after activation should retry rather than treat it as a failure.

### Credential model (pass-through)

`azure/pim-role` returns the parent credentials. The activated role attaches to the principal the parent
authenticated, server-side; the token itself is unchanged. The elevation is a side effect, not a new
credential, which is what lets it sit transparently in a chain ahead of `atmos terraform`, `atmos auth exec`,
the AKS exec plugin, or an MCP server.

### Care points

- **Do not re-request every command** (step 2): check for an active assignment first; activating repeatedly
  would spam PIM and hit request throttling.
- **Idempotency while pending** (step 4): a second invocation during `PendingApproval` attaches to the
  existing request instead of creating a duplicate.
- **Justification, interactive vs non-interactive**: justification is human-supplied. In an interactive
  session, prompt for it; in a non-interactive chain (CI, `atmos auth exec`, an MCP server starting up), take
  it from a flag or environment variable (for example `--justification` / `ATMOS_PIM_JUSTIFICATION`), and
  refuse clearly when elevation is required, no justification is supplied, and no prompt can be shown.
- **Long waits**: `Authenticate` is normally fast, but waiting on a human approver is not. Bound the wait,
  show visible progress, and make it resumable - a later invocation attaches to the pending request rather
  than starting over.

### Implementation shape

New `pkg/auth/identities/azure/pim_role.go` following `pkg/auth/identities/azure/subscription.go`; the constant
`IdentityKindAzurePIMRole = "azure/pim-role"` in `pkg/auth/types/constants.go`; a `case "azure/pim-role"` in
`pkg/auth/factory/factory.go`. The PIM calls hit the `Microsoft.Authorization` REST surface
(`roleEligibilityScheduleInstances`, `roleAssignmentScheduleRequests`, `roleAssignmentScheduleInstances`) with
the management token from the parent credentials. The enumerate / request / poll calls are behind injection
seams per the repo's dependency-injection convention so tests can mock ARM.

### Testability

Unit tests mock the ARM PIM responses across the paths: eligible-and-activates, already-active (no-op),
pending-then-provisioned, not-eligible error, non-interactive-without-justification refusal, and the
long-wait / resume path. No live tenant is required for unit coverage; an end-to-end activation against a real
eligible role is a manual verification step.

## 3. Non-goals

- **Entra directory roles.** Those activate through Microsoft Graph
  (`roleManagement/directory/roleAssignmentScheduleRequests`), a different API, and would be a separate kind (for
  example `azure/pim-directory-role`). This kind covers Azure *resource* roles only.
- **Managing eligibility.** This activates an existing eligibility; it does not create PIM-eligible
  assignments.
- **PIM for Groups.** Activating eligible group membership is a different API and a later kind.
- **Approval beyond waiting.** Configuring approvers or approving on someone's behalf is out of scope;
  this kind self-activates and waits.

## 4. Acceptance

- An `azure/pim-role` identity chaining from an Azure identity activates its configured eligible role on
  authentication; `atmos auth login --identity <pim-identity>` and
  `atmos auth exec --identity <pim-identity> -- <cmd>` run with the role active.
- A second run within the activation window is a no-op (no duplicate request); `atmos auth whoami` reflects the
  chain.
- An ineligible principal gets an error that distinguishes "not eligible" from "activation failed".
- A non-interactive invocation without a justification fails fast with guidance to supply the flag or
  environment variable.
- A role requiring approval produces a bounded wait with visible progress, and a later invocation attaches to
  the pending request rather than restarting it.

## 5. Implementation status

Implemented 2026-09-30. Blog: `website/blog/2026-09-30-azure-pim-role-activation.mdx`
(slug `azure-pim-role-activation`). Roadmap: "Just-in-time PIM role activation (azure/pim-role)" under the
Unified Authentication initiative.

### What shipped

- **Identity kind constant** `IdentityKindAzurePIMRole = "azure/pim-role"` in
  `pkg/auth/types/constants.go` (alongside a new `IdentityKindAzureSubscription`), registered in
  `pkg/auth/factory/factory.go`.
- **Identity** `pkg/auth/identities/azure/pim_role.go` implements the full `Identity` interface.
  `Authenticate` runs the PRD flow (resolve object id -> short-circuit if active -> require
  eligibility -> resume-or-create request -> poll to `Provisioned`) and returns the parent
  credentials unchanged (pass-through). Chains from `via.identity` or `via.provider`.
- **PIM client** `pkg/auth/identities/azure/pim_client.go` is the ARM REST surface behind a
  `PIMClient` interface (the dependency-injection seam). The live `armPIMClient` targets the
  Resource Manager endpoint for the credential's cloud environment (public/usgov/china) with
  api-version `2020-10-01`, filters list calls with `$filter=asTarget()`, and PUTs a
  `SelfActivate` `roleAssignmentScheduleRequest`. The HTTP transport is itself injectable
  (`httpDoer`) so request building is unit-tested without a network.
- **Principal object id** resolved from the parent management token's `oid` claim via a new
  exported `azureCloud.ExtractObjectIDFromToken` (`pkg/auth/cloud/azure/token_oid.go`).
- **Cloud endpoint helper** `CloudEnvironment.ResourceManagerEndpoint()` in
  `pkg/auth/cloud/azure/cloud_environments.go`.
- **Duration** Go-style `duration` is converted to ISO-8601 (`8h` -> `PT8H`); an empty/invalid
  duration is omitted so ARM applies the policy default.
- **Justification** resolved from `principal.justification` -> `ATMOS_PIM_JUSTIFICATION` (read via
  viper's ATMOS_ automatic-env binding) -> interactive prompt (TTY) -> a fail-fast error.
- **Error sentinels** in `errors/errors.go`: `ErrAzurePIMNotEligible`,
  `ErrAzurePIMJustificationRequired`, `ErrAzurePIMActivationFailed`,
  `ErrAzurePIMActivationTimeout`, `ErrAzurePIMRequestFailed`.

### Acceptance criteria mapping

- Activates on authentication; inherited by every consumer -> `Authenticate` pass-through + chain
  wiring. Covered by `TestPIMRole_EligibleAndActivates`.
- Second run within the window is a no-op -> step-1 active-assignment short-circuit.
  `TestPIMRole_AlreadyActive_NoOp`.
- Ineligible principal gets a distinct "not eligible" error -> `ErrAzurePIMNotEligible`.
  `TestPIMRole_NotEligible`.
- Non-interactive without justification fails fast with guidance ->
  `ErrAzurePIMJustificationRequired`. `TestPIMRole_NonInteractiveWithoutJustification`.
- Approval-gated role: bounded wait, resumable -> `waitForActivation` + pending-request resume.
  `TestPIMRole_PendingThenProvisioned`, `TestPIMRole_ResumePendingRequest`,
  `TestPIMRole_ActivationTimeout`.

### Tests and coverage

`pkg/auth/identities/azure/pim_role_test.go`, `pim_client_test.go`,
`pkg/auth/cloud/azure/token_oid_test.go`, a `ResourceManagerEndpoint` test, and a factory case.
Package `pkg/auth/identities/azure` coverage is 87.0%. All ARM interaction is mocked; a live
end-to-end activation against a real eligible role remains a manual verification step.

### Deferred (non-goals or follow-ups)

- Capping `duration` at the role's PIM activation-policy maximum pre-flight (would require the
  `roleManagementPolicyAssignments` API) - tracked in
  [#3238](https://github.com/cloudposse/atmos/issues/3238). Today an over-long duration is rejected
  by ARM server-side; the create-failure path surfaces ARM's message plus an actionable hint to
  lower `duration`, so the gap is only the pre-flight clamp.
- Entra directory roles, PIM for Groups, and configuring approvers remain out of scope (Section 3).
