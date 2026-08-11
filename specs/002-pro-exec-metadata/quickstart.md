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

4. Run a synchronous command and confirm it visibly waits on / reports the upload outcome:

  ```bash
  ATMOS_LOGS_LEVEL=Debug atmos terraform plan <component> -s <stack>
  ```

5. Unset `CI` and confirm no upload attempt is logged for the same commands — this is the
  negative-path check from Acceptance Scenario US1.2/US1.3.

## Regenerating the Pact contract

```bash
go test -tags pact ./pkg/pro/... -v -run TestPact/UploadExecMetadata
git diff pacts/atmos-AtmosPro.json
```

Hand `pacts/atmos-AtmosPro.json` to the Atmos Pro team as the source of truth for
implementing the `POST /v1/atmos/exec` provider endpoint — see
`contracts/interactions.md` in this feature directory for the human-readable version of
the same contract.

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
