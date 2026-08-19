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

## Regenerating the Pact contract

```bash
go test -tags pact ./pkg/pro/... -v -run 'TestPact/UploadExecMetadata|TestPact/UploadExecData'
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
      sync_timeout_seconds: 10   # default; only increasing this value has any effect
```

No opt-out flag exists — delivery is always attempted whenever CI is detected and Atmos
Pro is configured (FR-001/FR-002).
