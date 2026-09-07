# Atmos Pro Exec-Metadata Upload — Developer Quickstart

**Feature**: 002-pro-exec-metadata
**Date**: 2026-08-11

This supplements the existing "Pact Contract Testing" README section
(`specs/001-pact-consumer-contracts/quickstart.md`) — same tooling, one more interaction.

---

## Manually exercising the feature end-to-end

1. Configure Atmos Pro locally (static token is simplest for manual testing):

  ```bash
  export ATMOS_PRO_TOKEN=<a-real-or-test-token>
  export ATMOS_PRO_BASE_URL=http://localhost:<port>   # point at a local stub if desired
  ```

2. Simulate CI detection (the gate requires `telemetry.IsCI()` to be true):

  ```bash
  export CI=true
  ```

3. Run any command and confirm (via `--logs-level Debug` or `ATMOS_LOGS_LEVEL=Debug`) that
  an exec-metadata upload attempt is logged:

  ```bash
  ATMOS_LOGS_LEVEL=Debug atmos version
  ```

4. Run a synchronous command and confirm it visibly waits on / reports the upload outcome,
  and that **exactly one** upload attempt is logged (not two — this is the regression
  check for the dedup fix, US2 Acceptance Scenario 4):

  ```bash
  ATMOS_LOGS_LEVEL=Debug atmos terraform plan <component> -s <stack>
  ```

5. Unset `CI` and confirm no upload attempt is logged for the same commands — this is the
  negative-path check from Acceptance Scenario US1.2/US1.3.

6. Run a multi-component `plan` and confirm exactly **one** upload attempt is logged for
  the whole invocation, not one per component (FR-006a):

  ```bash
  ATMOS_LOGS_LEVEL=Debug atmos terraform plan --affected
  ```

7. Run `atmos terraform deploy <component> -s <stack>` and confirm it now also blocks on
  / reports its upload outcome (allowlist expansion) and carries structured
  infrastructure-change data (FR-006).

8. Run `atmos terraform plan cdn -s plat-use2-dev --upload-status` and inspect the logged
  request body (`ATMOS_LOGS_LEVEL=Debug`) to confirm the `Command`/`Args`/`Flags` shape
  fix (FR-003b): `command` MUST be `"terraform plan"` (no `atmos` prefix), `args` MUST be
  `["cdn"]` (not empty), and `flags` MUST contain `"--stack"`, `"plat-use2-dev"`, and
  `"--upload-status"` (canonical long-form names — `-s` is reported as `--stack`; array
  order is not required to match invocation order, per the 2026-08-19 clarification)
  — never combined with `args`. This is the regression check for the always-empty-`Args`
  bug (`pkg/proexec/envelope.go:55`).

9. Confirm every logged request body includes a fresh `execution_id` (UUID v4) that
  differs between separate command invocations, and stays identical to the `run_id`
  correlation field only by coincidence never by design — `execution_id` identifies the
  one record, `atmos_pro_run_id` correlates records across the whole CI run (FR-003c).

10. Exercise the inline-vs-blob-URL threshold (FR-011) with a plan large enough to push the
  whole record at/over 4 MB — e.g. a component with a very large number of pending
  resource changes, or (for a quick local check without a huge real plan) temporarily lower
  `Settings.Pro.MaxPayloadBytes` via config to a small value and rerun a normal-sized plan:

  ```yaml
  # atmos.yaml
  settings:
    pro:
      max_payload_bytes: 1024   # artificially small, for local testing only
  ```

  ```bash
  ATMOS_LOGS_LEVEL=Debug atmos terraform plan <component> -s <stack>
  ```

  Confirm two requests are logged for the one invocation: first a `POST .../atmos/exec/data`
  request (`execution_id` + `data`, no `batch_id`/chunking fields anywhere), then a
  `POST .../atmos/exec` request whose `data` field is a JSON **string** (the URL returned by
  the first request), not an inline object. Confirm the small-plan case (default
  `max_payload_bytes`) instead sends exactly one `/exec` request with `data` inline.

11. **(research.md Decision 18)** Run a plain, single-component `atmos terraform plan
  <component> -s <stack>` (no `--affected`/`--all`) against a component with pending changes,
  and inspect the logged request body's `data` field: it MUST contain
  `resource_counts`/`outputs`/`warnings`/`changes` (matching `data-model.md`'s
  `TerraformExecData` shape), not `null` — this is the single-component counterpart to step
  10's multi-component coverage (`cmd/terraform/utils_exec_metadata_test.go`'s
  `TestTerraformNodeHooks_RecordExecResultAccumulates`), verifying
  `captureExecMetadataSync`'s new `WithExecMetadataParser`-supplied closure actually ran.

12. **(research.md Decision 19)** Run `atmos terraform plan <component> -s <stack>` against a
  component whose outputs include at least one `sensitive = true` output (e.g. add a test
  output block marked sensitive to a scratch component), and inspect the logged request
  body's `data.outputs` field: the sensitive output's `value` MUST be the literal string
  `"<MASKED>"` while its `sensitive` field still reads `true` and `type` is still reported —
  never the real value, even if that value would not match any Gitleaks secret pattern (e.g.
  a plain internal ID). Confirm a non-sensitive output in the same run still reports its real
  `value` unmasked (unless it separately happens to match a Gitleaks pattern, in which case
  layer 2 masks it instead — either way, no sensitive-flagged output's real value should ever
  appear in the logged body).

13. **(research.md Decision 20)** Run `atmos terraform plan <component> -s <stack>` twice —
  once against a component with no pending changes, once against one with pending changes or
  a deliberate error — and confirm the logged request body's `data.has_changes`/
  `data.has_errors`/`data.errors` accurately reflect each run's outcome, matching what the
  command's own terminal output already showed.

14. **(research.md Decision 21)** Run `atmos terraform plan <component> -s <stack>` and
  confirm the logged request body's `data.component`/`data.stack` match the invocation's
  actual component/stack — this is the single-component structured-data counterpart to the
  base envelope's `Args`/`Flags` (step 8), giving Atmos Pro a direct identity field instead of
  requiring it to parse `args[0]`/the `--stack` value out of `flags`.

15. **(research.md Decision 22)** Run `atmos describe affected` (no `--upload`) in CI with
  Atmos Pro configured, and confirm the logged request body's `data` field is present and
  shaped `{"version": 1, "stacks": [...]}` — not `null` — even though `--upload` was not
  passed, since the affected-stacks list is already computed for every invocation regardless.

16. **(research.md Decision 23, gating updated by spec.md's 2026-08-22 Clarifications session)**
  Run `atmos list instances --upload` (with the CI event/repo preconditions `--upload`
  requires) and confirm the logged request body's `data` field is `{"version": 1, "instances":
  [...]}`. Then run `atmos list instances` **without** `--upload`, still with Atmos Pro
  integration active (CI detected, Pro credentials configured — `proexec.GateOpen` true), and
  confirm `data` is still `{"version": 1, "instances": [...]}` — Pro-integration-active alone
  now also populates it, per FR-006c. Finally, run `atmos list instances` with **neither**
  `--upload` nor Pro integration active (e.g. not in CI, or Pro not configured) and confirm
  `data` is absent/`null` — this is the regression check for FR-006c's "MUST NOT compute the
  instance list solely to populate this field" requirement, now scoped to the case where
  neither condition holds.

17. **(research.md Decision 24)** Inspect the logged request bodies from steps 11 (terraform),
  15 (`describe affected`), and 16 (`list instances`) together and confirm each `data.version`
  reads `1` — independently present on all three shapes, never on the outer envelope itself
  (no top-level `version` field alongside `execution_id`/`atmos_version`/etc.).

18. **(research.md Decision 26)** Run `atmos terraform plan <component> -s <stack>` against a
  component with no warnings/errors/changes, and inspect the logged request body's
  `data.changes`/`data.warnings`/`data.errors`: each MUST be `[]`, never `null` — this is the
  regression check for the real CI payload (`atmos-pro-qa-3` run 32412509172) that surfaced
  this gap.

19. **(research.md Decisions 27-28)** Run `atmos terraform plan <component> -s <stack>` and
  confirm the logged request body's `data.exit_code` matches the terraform subprocess's own
  exit code (0 for a clean plan; non-zero for a deliberately-failing one), and that it is
  distinct from the top-level request body's own `exit_code` field. Then run a multi-component
  `plan --affected` where one component's terraform subprocess fails while another succeeds,
  and confirm each component's own entry in the folded aggregate carries its own `exit_code` —
  no single aggregate `data.exit_code`.

20. **(research.md Decision 29)** Run `atmos terraform plan <component> -s <stack>` against a
  component engineered to produce terraform output the parser cannot recognize at all (or
  simulate via the unit test fixture, since reproducing this manually is awkward), and confirm
  `data` is still present — not omitted — with `version`/`exit_code`/`component`/`stack`
  populated and every other field at its empty/zero/false default.

21. **(research.md Decision 30r, correcting the retracted Decision 30)** Run `atmos terraform
  deploy <component> -s <stack>` against a component with pending changes, and inspect the
  logged request body's `data` field: it MUST be the identical single `TerraformExecData`
  shape `terraform plan`/`apply` use directly (including the new `exit_code` field) — NOT a
  two-phase `{plan, apply}` wrapper. A prior design proposed the two-phase wrapper on the
  premise that `deploy` runs plan and apply as two separate subprocesses; that premise was
  found false during implementation (`deploy` runs exactly one subprocess), so the single
  shape is correct, not a regression.

### Automated-test coverage of steps 11-21 (Phase 7/T025)

Steps 11-21 above require a live/stubbed Atmos Pro endpoint and CI-mode simulation, so they
remain manual/exploratory end-to-end checks. The table below maps each to the automated test(s)
that already cover its underlying behavior at the unit/contract level, so a contributor can
confirm the logic is exercised by `go test` without needing a live Pro backend:

| Step | Behavior | Automated coverage |
|------|----------|---------------------|
| 11 | Single-component `data` shape populated | `TestBuildTerraformExecData_ApplySuccess`, `TestTerraformNodeHooks_RecordExecResultAccumulates` (`cmd/terraform/utils_exec_metadata_test.go`) |
| 12 | Sensitive outputs masked to `<MASKED>` | `TestMaskSensitiveOutputs` (`cmd/terraform/utils_exec_metadata_test.go`), `TestBuildRecord_SecretMaskingAppliedToData`-family (`pkg/proexec/envelope_test.go`), `TestPact_UploadExecMetadata` (`pkg/pro/consumer_pact_test.go`) |
| 13 | `has_changes`/`has_errors`/`errors` accurate | `TestBuildTerraformExecData_ApplySuccess`, `TestBuildTerraformExecData_ApplyFailure` (`cmd/terraform/utils_exec_metadata_test.go`), `TestPact_UploadExecMetadata` |
| 14 | `component`/`stack` present/omitted correctly | `TestBuildTerraformExecData_EmptyComponentStackOmitted`, `TestTerraformCaptureShellOpts_AlwaysWiresCaptureAndParser`, `TestTerraformExecMetadataParserFunc_ReadsBuffersAtCallTime` (`cmd/terraform/utils_exec_metadata_test.go`), `TestPact_UploadExecMetadata` |
| 15 | `describe affected` `data` unconditional | `TestExecuteInner_ReturnsAffected`, `TestExecute_AttachesAffectedAsStructuredData` (`internal/exec/describe_affected_upload_test.go`), `TestPact_UploadExecMetadata_DescribeAffected`(`_BlobURL`) |
| 16 | `list instances` `data` gated on `--upload` OR Pro-integration-active | `TestUploadInstancesWithDeps_SetsPendingAsyncDataForExecMetadata` (`pkg/list/list_instances_upload_test.go`), `TestCaptureAsync_UsesAndClearsPendingAsyncData` (`pkg/proexec/async_test.go`), `TestPact_UploadExecMetadata_ListInstances`(`_BlobURL`), `TestExecuteListInstancesCmd_ProGateWithoutUpload`/`TestExecuteListInstancesCmd_NoUploadNoProGate_NoPendingData` (`pkg/list/list_instances_coverage_test.go`, spec.md 2026-08-22) |
| 17 | `version: 1` present on every shape, absent from envelope | `TestVersionedData_*` (`pkg/proexec/envelope_test.go`); every `TestPact_UploadExecMetadata*` case asserts `version` as an exact-literal `1` |
| 18 | `changes`/`warnings`/`errors` are `[]`, never `null`, when empty | `TestBuildTerraformExecData_EmptyListsAreNotNull` (`cmd/terraform/utils_exec_metadata_test.go`), `TestPact_UploadExecMetadata` |
| 19 | `exit_code` present, distinct from envelope `exit_code`, per-component in multi-component runs | `TestBuildTerraformExecData_ExitCode`, `TestTerraformNodeHooks_RecordExecResultAccumulates` (per-node `ExitCode` already covered), `TestPact_UploadExecMetadata` |
| 20 | Minimal `Data` still attached when parsing fails entirely | `TestBuildTerraformExecData_UnparseableOutputStillAttachesMinimalData` (`cmd/terraform/utils_exec_metadata_test.go`) |
| 21 | `terraform deploy` uses the identical `TerraformExecData` shape, not a two-phase split | `TestBuildTerraformExecData_DeployParsedAsApply` (`cmd/terraform/utils_exec_metadata_test.go`), `TestCaptureExecMetadataSync_DeployReportedAsDeploy` (`internal/exec/`) |

Step 10 (inline-vs-blob-URL threshold) has no automated Pact equivalent by design — per
research.md Decision 25, the Pact suite constructs the inline and blob-URL cases directly
rather than routing through the real size-threshold code, so the threshold decision itself
stays covered only by `pkg/proexec/envelope_test.go`'s own threshold-focused unit tests, not
by the six shape-coverage Pact interactions.

## Regenerating the Pact contract

```bash
go test -tags pact ./pkg/pro/... -v -run 'TestPact_UploadExecMetadata|TestPact_UploadExecData'
git diff pacts/atmos-AtmosPro.json
```

Hand `pacts/atmos-AtmosPro.json` to the Atmos Pro team as the source of truth for
implementing the `POST /v1/atmos/exec` and `POST /v1/atmos/exec/data` provider endpoints —
see `contracts/interactions.md` in this feature directory for the human-readable version of
the same contract, covering both `Data` shapes (inline and blob-URL) plus the new
`/exec/data` interaction.

## New configuration surface

```yaml
# atmos.yaml
settings:
  pro:
    exec:
      sync_timeout: 10s   # default; only increasing this value has any effect
```

No opt-out flag exists — delivery is always attempted whenever CI is detected and Atmos
Pro is configured (FR-001/FR-002).
