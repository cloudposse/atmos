# PRD: Atmos Pro Security Findings Upload

## Executive Summary

Ship an experimental `--upload` flag on `atmos aws security analyze` that POSTs a SARIF 2.1.0 report of mapped findings to the Atmos Pro `/repos/{owner}/{repo}/security-findings` endpoint. Authentication piggybacks on the existing Atmos Pro client (static `ATMOS_PRO_TOKEN` or GitHub OIDC exchange). The flag is independent of `--format`: local output (markdown, JSON, …) is unchanged; the upload always serializes SARIF internally and ships it with repo metadata + commit SHA.

This PRD captures the design and code shape that was implemented on the `osterman/aws-security-sarif-upload` branch and then **stripped before merge** because the Atmos Pro endpoint is not yet GA. Revive this PRD when the endpoint is locked in.

## Problem Statement

`atmos aws security analyze` already maps AWS Security Hub / Config / Inspector / GuardDuty findings to Atmos components and stacks, but each invocation produces only a local report. Centralizing findings — with consistent stack/component/repo attribution — requires teams to write their own collector. We already track drift, deployment provenance, and instance status in Atmos Pro; security findings belong on the same surface so that ownership, history, and remediation queues live in one place.

### Why this didn't ship now

The receiving endpoint at Atmos Pro is not generally available. Shipping the client without a stable receiver would create a public API surface we'd then have to keep working through contract changes. Holding both sides until the endpoint stabilizes is cheaper than emitting a release-note retraction.

## Goals

- One-flag upload from `atmos aws security analyze` to Atmos Pro.
- SARIF 2.1.0 as the on-wire format (canonical, dedup-friendly, already produced for `--format=sarif`).
- Reuse the existing `pro.AtmosProAPIClient` auth pipeline (token + OIDC, retry, error normalization).
- Repo + commit metadata attached server-side so findings land on the right project without extra config.
- Mockable end-to-end for offline tests (no AWS calls, no real Atmos Pro calls).

## Non-Goals

- Per-finding ack/dismiss workflow (server-side concern).
- Bidirectional sync (Atmos Pro state → local report).
- Multi-repo or workspace-wide aggregation in a single upload (one repo per call).
- A long-running daemon / scheduled push (CI invocation only).

## Design

### CLI surface

```shell
atmos aws security analyze --upload                              # upload + default markdown locally
atmos aws security analyze --format=sarif --file=findings.sarif --upload   # local SARIF + upload
ATMOS_AWS_SECURITY_UPLOAD=true atmos aws security analyze        # env-var form
```

- Flag: `--upload` (bool, default `false`), registered via `flags.NewStandardParser` with env var `ATMOS_AWS_SECURITY_UPLOAD`.
- Independent of `--format`: the upload always carries SARIF; local rendering is whatever the user asked for.
- Pre-flight warning printed via `ui.Warning(...)` on every invocation: "--upload is experimental; the Atmos Pro security findings endpoint is not yet generally available and may change."

### Request contract

DTO `pkg/pro/dtos/security_findings.go`:

```go
type SecurityFindingsUploadRequest struct {
    RepoURL   string          `json:"repo_url"`
    RepoName  string          `json:"repo_name"`
    RepoOwner string          `json:"repo_owner"`
    RepoHost  string          `json:"repo_host"`
    GitSHA    string          `json:"git_sha,omitempty"`   // best-effort; empty when HEAD is detached
    Stack     string          `json:"stack,omitempty"`
    Component string          `json:"component,omitempty"`
    Format    string          `json:"format"`              // always "sarif"
    SARIF     json.RawMessage `json:"sarif"`               // pre-serialized SARIF 2.1.0 document
}
```

Endpoint: `POST {BaseURL}/{BaseAPIEndpoint}/security-findings` (i.e., `…/api/v1/security-findings`). Authentication via the existing `getAuthenticatedRequest` helper, retried via `doWithRetry` with `defaultRetryConfig()` so transient 401s trigger OIDC refresh.

### Code shape (as implemented)

| Layer | File | Responsibility |
|---|---|---|
| CLI flag + orchestration | `cmd/aws/security/security.go` | `--upload` flag binding; `uploadReportToAtmosPro(atmosConfig, report, stack, component)` orchestrator; package-level `gitRepoFactory` and `proClientFactory` (returns narrow `pro.APIClient`) for test injection. |
| HTTP method | `pkg/pro/api_client_security.go` | `(c *AtmosProAPIClient) UploadSecurityFindings(dto)`: nil-check, marshal, sha256 hash for debug log (never log raw SARIF), `doWithRetry` wrapping `getAuthenticatedRequest`+`handleAPIResponse`. `defer perf.Track(nil, "pro.AtmosProAPIClient.UploadSecurityFindings")()` as first stmt. |
| Interface | `pkg/pro/interface.go` (`APIClient`) | Adds `UploadSecurityFindings`. Narrow interface — what tests/factories need. |
| Interface | `pkg/pro/api_client.go` (`AtmosProAPIClientInterface`) | Mirror entry for the wider real-client interface. |
| Mock | `pkg/pro/mock_interface.go` | mockgen-regenerated to expose `UploadSecurityFindings` on `MockAPIClient`. |
| DTO | `pkg/pro/dtos/security_findings.go` | Request struct (above). |
| Errors | `errors/errors.go` | `ErrAWSSecurityUploadFailed` sentinel; `ErrAWSSecurityInvalidFormat` message extended to list `sarif`. |
| Tests | `cmd/aws/security/security_upload_test.go` | gomock-based: success, client-creation failure, server error, repo-info failure, missing-SHA non-fatal. Uses `MockGitRepoInterface` (generated for `pkg/git`) + `MockAPIClient`. |
| Tests | `pkg/pro/api_client_security_test.go` | Success, nil DTO, server 4xx (non-retried), transport error. Uses the existing `MockRoundTripper` from the pro package. |

### Orchestration flow

`uploadReportToAtmosPro` (in `cmd/aws/security/security.go`) sequences:

1. `ui.Warning("--upload is experimental; …")` — runs before any network activity.
2. `pkgsecurity.BuildSARIFLog(report)` then `json.Marshal` — produces canonical bytes.
3. `gitRepoFactory().GetLocalRepoInfo()` — required; failure aborts with `errors.Join(ErrAWSSecurityUploadFailed, err)`.
4. `gitRepoFactory().GetCurrentCommitSHA()` — best-effort; on error, log warn and continue with empty SHA.
5. `proClientFactory(atmosConfig)` — wraps `pro.NewAtmosProAPIClientFromEnv`. Failure produces a `ErrAWSSecurityUploadFailed` builder with two hints ("Set ATMOS_PRO_TOKEN…", "See …/authentication") and the underlying cause.
6. Assemble DTO; `apiClient.UploadSecurityFindings(dto)`.
7. Success → `ui.Successf("Uploaded SARIF report to Atmos Pro (%d findings).", report.TotalFindings)`.

### Why narrow `pro.APIClient` instead of the wider `AtmosProAPIClientInterface`

The upload path needs only `UploadSecurityFindings`. The narrow interface (`pkg/pro/interface.go`) is what `mockgen` already generates a mock for, so tests get a generated `MockAPIClient` with no manual fakes. The wider interface (`AtmosProAPIClientInterface` in `api_client.go`) lists 6 methods — overkill for the security upload call site and harder to mock.

### Test mocking architecture

Two factory variables in the command package, swapped via a `withFactories(t, mockRepo, mockClient, clientErr)` helper:

```go
var proClientFactory = func(c *schema.AtmosConfiguration) (pro.APIClient, error) {
    return pro.NewAtmosProAPIClientFromEnv(c)
}
var gitRepoFactory = git.NewDefaultGitRepo
```

Tests construct `gomock.NewController(t)`, build `git.NewMockGitRepoInterface(ctrl)` and `pro.NewMockAPIClient(ctrl)`, set `EXPECT()` calls, install via `withFactories`, run `uploadReportToAtmosPro`, and assert on a captured DTO via `DoAndReturn`. `t.Cleanup` restores the originals.

`pkg/git/GitRepoInterface` (in `pkg/git/git.go`) gained a `//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE -mock_names=GitRepoInterface=MockGitRepoInterface` directive so the mock regenerates with the file.

## Open questions for revival

- **Endpoint contract.** Is the path `/repos/{owner}/{repo}/security-findings` or simply `/security-findings` with repo metadata in the body? The branch implementation used the latter — confirm before reviving.
- **Response shape.** Server may want to return a finding-set ID, a dedup count, or links into Atmos Pro UI. Current client ignores the response body; revisit before GA.
- **Payload size.** SARIF can grow with grouped findings disabled. The existing chunked-upload path used by `UploadInstances` may be needed if reports exceed ~5 MiB.
- **Idempotency.** Should re-uploading the same SARIF for the same SHA be a no-op server-side, or always write a new revision? Affects retry semantics.
- **Audit story.** What identity gets logged on the Atmos Pro side — the `ATMOS_PRO_TOKEN` owner or the GitHub OIDC subject? Needs alignment with the dashboard audit log.

## Reference: original announcement copy

If reviving, the blog/docs/roadmap copy used in PR #2483 (subsequently reverted) can be retrieved from git history at commit `bd5d2b508`:

- `website/blog/2026-05-22-aws-security-sarif.mdx` — title "AWS Security Findings Now Export to SARIF and Atmos Pro"; slug `aws-security-sarif-and-upload`.
- `website/docs/cli/commands/aws/security/analyze.mdx` — `--upload` flag entry + "Atmos Pro Upload (Experimental)" Terminal block.
- `website/src/data/roadmap.js` — milestone label "SARIF output and Atmos Pro upload (experimental)".

The SARIF half of that PR shipped; this PRD describes only the upload half.

## Future Wire Formats

OCSF 1.4.0 was added as a sibling `--format=ocsf` output (Detection Finding class 2004 with cloud + vulnerability profiles) for SIEM/data-lake consumers. When the Atmos Pro upload endpoint is revived, OCSF is a candidate alternative wire format — particularly if downstream Atmos Pro features want to feed events into a security data lake (Splunk, Snowflake) rather than (or in addition to) the SARIF-shaped GitHub code-scanning surface.

If we add an OCSF upload path:

- Envelope mirrors the SARIF envelope: `Payload struct { Repo, SHA, OCSF json.RawMessage }`. Server inspects a `content_type` field (`"sarif"` vs `"ocsf"`) to route.
- Reuses `pkgsecurity.BuildOCSFEvents(report)` then `json.Marshal` — same canonicalization story as `BuildSARIFLog`.
- Same auth (`ATMOS_PRO_TOKEN` or GitHub OIDC) and retry semantics.
- `--upload-format=sarif|ocsf` (default `sarif`) is the most natural flag shape; alternatively `--upload-ocsf` as a parallel boolean to the existing `--upload`.

This is a design candidate for the endpoint revival, **not** a v1 commitment. The decision belongs to the Pro endpoint design that gates this PRD coming off ice.
