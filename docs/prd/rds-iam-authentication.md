# RDS IAM Authentication PRD

**Status**: Draft
**Last Updated**: 2026-07-24
**Owners**: Atmos auth subsystem

## Executive Summary

Add native **RDS IAM database authentication** to Atmos via a new `atmos aws rds token` command — the
RDS analog of `atmos aws eks token`. It mints a short-lived (~15-minute) SigV4 "connect" token from an
Atmos identity and prints it to stdout for use as a database password, with **no AWS CLI required** and
no long-lived DB credentials.

Companion to [`eks-kubeconfig.md`](./eks-kubeconfig.md) and [`ecr-public-authentication.md`](./ecr-public-authentication.md);
it follows the same aws-command + on-demand-token pattern as `atmos aws eks token` (PR #2149), and — like
ECR Public (PR #2231) — exposes an AWS auth capability through a first-class Atmos command.

**Key design decision:** token generation is a standalone command, **not** an `auth.integrations` kind. An
RDS auth token is a short-lived credential valid ~15 minutes (it authenticates each new connection within
that window, not single-use), so (like `eks token`) it is
minted on demand and never provisioned at `atmos auth login`.

## Problem Statement

RDS/Aurora IAM authentication replaces static database passwords with short-lived, IAM-signed tokens. But
generating one today requires either the AWS CLI (`aws rds generate-db-auth-token`) or hand-rolled SigV4
signing, plus separately-resolved AWS credentials — none of which compose with the Atmos identity system
that already governs every other `atmos aws *` command.

### User Impact

**Current experience** — separate tool, separate credential resolution:

```shell
# Requires the AWS CLI and an already-resolved AWS profile/session:
export PGPASSWORD="$(aws rds generate-db-auth-token \
  --hostname mydb.abc123.us-east-2.rds.amazonaws.com --port 5432 \
  --username app --region us-east-2)"
psql "host=mydb.abc123.us-east-2.rds.amazonaws.com sslmode=require dbname=app user=app"
```

**Desired experience** — one Atmos identity, no AWS CLI:

```shell
export PGPASSWORD="$(atmos aws rds token \
  --host mydb.abc123.us-east-2.rds.amazonaws.com --port 5432 \
  --username app --region us-east-2 --identity dev-admin)"
psql "host=mydb.abc123.us-east-2.rds.amazonaws.com sslmode=require dbname=app user=app"
```

> **Reducing the arguments further** is under active discussion (see [cloudposse/discussions#121](https://github.com/orgs/cloudposse/discussions/121)): referencing an `aws/rds` integration by name — as `atmos aws rds connect <name>` already does — or a no-argument instance picker. The flags above are the Phase-1 explicit form.

## Design Goals

- Mint an RDS IAM auth token from an Atmos identity (assumed-role / SSO chain via `pkg/auth`), no AWS CLI.
- Mirror `atmos aws eks token` in structure, flags-style, and testability.
- Engine-agnostic: the AWS primitive signs a `host:port` endpoint; no Postgres-vs-MySQL special-casing.
- Emit a clean, pipeable token (drop-in for `PGPASSWORD=$(…)`), byte-for-byte intact.
- 100% offline unit-testable; clear ≥85% patch coverage without a live RDS instance.

## Non-Goals (Phase 1)

- No stack/component endpoint auto-discovery (endpoints are post-apply state — deferred to a follow-up command).
- No `aws/rds` auth **integration** kind (login-time provisioning) — deferred.
- No database user / `rds_iam` grant management, and no RDS provisioning.
- No `connect`/`shell` launcher that execs `psql`/`mysql` — deferred.

## Technical Specification

### Architecture Overview

Two layers, mirroring `atmos aws eks token`; **no** `auth.integrations` subsystem involvement. Token
generation runs under `auth.ContextWithSkipIntegrations(ctx)` so it has no side effects.

1. **Cloud layer** — `pkg/auth/cloud/aws/rds.go`: `GetRDSToken`, a sibling of `GetToken` (`eks.go`).
2. **Command layer** — `cmd/aws/rds/{rds.go,token.go}`: the `rds` group + `token` leaf.

### Configuration Schema

| Flag | Env | Required | Default | Description |
|---|---|---|---|---|
| `--host` | `ATMOS_AWS_RDS_HOST` | ✅ | – | DB / cluster / proxy / RDS endpoint hostname. |
| `--port` | `ATMOS_AWS_RDS_PORT` | ✅ | – | DB port (required, engine-agnostic — matches the AWS CLI). |
| `--username` (`-u`) | `ATMOS_AWS_RDS_USERNAME` | ✅ | – | DB account name; must match the IAM `dbuser` ARN (case-sensitive). |
| `--region` | `ATMOS_AWS_RDS_REGION` | – | identity's credential region | Region of the DB endpoint. **Optional** — when empty, `GetRDSToken` falls back to the authenticated identity's credential region. The command errors only when neither the flag nor the credentials supply a region. |
| `--identity` (`-i`) | `ATMOS_IDENTITY` | – | – | Atmos identity to authenticate. If omitted, uses `ATMOS_IDENTITY`, or the single configured identity when exactly one exists; otherwise errors. |

Precedence: CLI > env > config > default (Viper via `flags.StandardParser`). All env binding goes through
`flags.WithEnvVars` — **never** `viper.BindEnv`/`viper.BindPFlag`/`os.Getenv` (forbidigo-enforced). This is
a deliberate refinement over `eks token`'s legacy `os.Getenv("ATMOS_IDENTITY")` fallback.

### CLI Command

```shell
atmos aws rds token --host <endpoint> --port <port> --username <user> [--region <region>] [--identity <id>]
```

`endpoint = host:port` is assembled before signing (RDS rejects a bare host at connect time). The command
sets `Args: cobra.NoArgs`, `SilenceUsage: true`.

### AWS SDK Integration

`GetRDSToken` builds an **isolated** static-credential `aws.Config` via `LoadIsolatedAWSConfig` (existing
`env.go`) — isolation stops an ambient `AWS_PROFILE` or shared config from breaking signing or corrupting the
region — then calls `github.com/aws/aws-sdk-go-v2/feature/rds/auth`:

```go
const rdsTokenExpiry = 15 * time.Minute // AWS-fixed; BuildAuthToken returns no expiry.

func GetRDSToken(ctx context.Context, creds types.ICredentials, endpoint, region, dbUser string) (string, time.Time, error) {
    defer perf.Track(nil, "aws.GetRDSToken")()

    // Isolated config: an explicit region wins over the credentials' region; ambient AWS_PROFILE
    // and shared config are excluded so they cannot break signing.
    cfg, err := LoadIsolatedAWSConfig(ctx, config.WithRegion(effectiveRegion),
        config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(/* from creds */)))
    if err != nil {
        return "", time.Time{}, fmt.Errorf("%w: %w", errUtils.ErrRDSTokenGeneration, err)
    }
    token, err := rdsauth.BuildAuthToken(ctx, endpoint, cfg.Region, dbUser, cfg.Credentials)
    if err != nil {
        return "", time.Time{}, fmt.Errorf("%w: %w", errUtils.ErrRDSTokenGeneration, err)
    }
    log.Debug("Generated RDS auth token", "endpoint", endpoint, "region", cfg.Region, "token_length", len(token))
    return token, time.Now().Add(rdsTokenExpiry), nil
}
```

Signing is **offline** (no network call), so the cloud layer is fully unit-testable with static fake creds.

### Output & Token Masking

The RDS token is a plaintext presigned URL containing `X-Amz-Credential=AKIA…`. Atmos's central output
choke-point masks the regex `AKIA[0-9A-Z]{16}` and the value of `AWS_ACCESS_KEY_ID`, so a default
`data.Write(token)` would rewrite the embedded access-key-id to `***` and **silently corrupt the password**.
(EKS is unaffected only because its token is base64-wrapped.)

The token is therefore emitted via **`data.WriteUnmasked(token)`** — the same escape hatch `cmd/auth/env.go`
uses to emit real AWS credentials — with the required justification:

```go
// The token IS the requested credential; masking it (AKIA... -> ***) would corrupt the DB password.
if err := data.WriteUnmasked(token); err != nil {
    return fmt.Errorf("%w: %w", errUtils.ErrRDSTokenGeneration, err)
}
```

The expiry is written to **stderr** via `ui.*` (not sensitive), keeping stdout a clean, pipeable token.

### Error Handling

The static sentinel `ErrRDSTokenGeneration` (in `errors/errors.go`, beside `ErrEKSTokenGeneration`) wraps
validation and token-generation failures; identity and config failures wrap their own sentinels
(`ErrIdentityAuthFailed`, `ErrFailedToInitConfig`). All are wrapped via `fmt.Errorf` and matchable with
`errors.Is`, as detailed in the table below.

| Context | Behavior |
|---|---|
| Missing `--host`/`--port`/`--username` | `ErrRDSTokenGeneration` (validation), usage suppressed |
| `--region` unresolved (neither flag nor credentials) | Hard error — never silently default to `us-east-1` |
| Identity auth failure | Wrapped `ErrIdentityAuthFailed` |
| SDK `BuildAuthToken` error | Wrapped `ErrRDSTokenGeneration` |

## Implementation Details

### Package Structure

```
NEW:
  pkg/auth/cloud/aws/rds.go            GetRDSToken (+ rdsTokenExpiry const)
  pkg/auth/cloud/aws/rds_test.go       offline unit tests
  cmd/aws/rds/rds.go                   bare RdsCmd cobra group (copy eks/eks.go)
  cmd/aws/rds/token.go                 token leaf (StandardParser + flow + WriteUnmasked + DI seams)
  cmd/aws/rds/token_test.go            DI-var swap + os.Pipe capture tests
MODIFY:
  errors/errors.go                     add ErrRDSTokenGeneration (after the EKS block)
  cmd/aws/aws.go                       awsCmd.AddCommand(rds.RdsCmd) (beside eks.EksCmd)
  go.mod / go.sum                      add feature/rds/auth (go mod tidy)
  NOTICE                               Apache-2.0 stanza (alphabetical, mirror service/eks)
```

## Testing Strategy

### Unit Tests

- **`rds_test.go`** (offline, no mocks): `TestGetRDSToken_Success` — static `AKIAIOSFODNN7EXAMPLE` creds;
  assert the token **contains** `Action=connect`, `DBUser=<user>`, `X-Amz-Signature=`, and `expiresAt.After(now)`
  (never exact-match — the signature embeds a live `X-Amz-Date`). `TestGetRDSToken_InvalidCredentials(nil)` →
  `errors.Is(err, ErrRDSTokenGeneration)`.
- **`token_test.go`**: DI-var swaps (`initCliConfigFn`/`authenticateForTokenFn`/`getRDSTokenFn`) with
  `t.Cleanup`; `os.Pipe` stdout capture. Table-driven flag validation, env/flag precedence
  (`ATMOS_AWS_RDS_REGION` vs `--region`), endpoint assembly, and raw-token output (no JSON, no decoration).
  Follows the eks **subpackage** precedent (local cobra builders + DI), not `cmd.NewTestKit` — noted in the PR.

### Coverage Target

Minimum **85%** patch coverage (CodeCov-enforced in the PR context). The offline cloud test + DI command
tests are sufficient; **no integration tests** — Phase 1 is fully offline.

## Security Considerations

- Token lifetime is AWS-fixed at ~15 minutes; it authenticates each new connection within that window (not single-use).
- The token is a credential: never logged (only `token_length` at debug), emitted only on the data channel
  via `data.WriteUnmasked` with a `#nosec G104` + CodeQL `clear-text-logging` justification.
- Credentials always come from the Atmos identity chain, never the ambient default chain.
- Prerequisites (documented, not enforced by the command): `EnableIAMDatabaseAuthentication` on the
  instance/cluster; IAM `rds-db:connect` on `arn:aws:rds-db:<region>:<acct>:dbuser:<DbiResourceId>/<dbUser>`;
  the DB-side grant (Postgres `GRANT rds_iam`; MySQL `IDENTIFIED WITH AWSAuthenticationPlugin`); and a TLS
  connection.

## Implementation Checklist

**Phase 1 — core (cloud)**
- [ ] Add `feature/rds/auth` to `go.mod`; `go mod tidy`; `go build ./...` to confirm `BuildAuthToken` signature; add `NOTICE` stanza.
- [ ] Add `ErrRDSTokenGeneration` sentinel to `errors/errors.go`.
- [ ] TDD `GetRDSToken` (`rds.go` + `rds_test.go`) — red, then green.

**Phase 2 — command**
- [ ] `cmd/aws/rds/rds.go` (group) + `cmd/aws/rds/token.go` (StandardParser, flow, `data.WriteUnmasked`, DI seams).
- [ ] Register `rds.RdsCmd` in `cmd/aws/aws.go`; `atmos aws rds token --help` exits 0.
- [ ] `token_test.go`; `make testacc-cover` ≥85%.
- [ ] `gofumpt` + `goimports`; `make lint` clean (cyclop ≤15, funlen 60/40, godot, forbidigo).

**Phase 3 — docs & release (minor label)**
- [ ] `website/docs/cli/commands/aws/aws-rds-token.mdx` (model on `aws-eks-token.mdx`); website build passes.
- [ ] Problem-first changelog blog post (tags from `tags.yml`, author from `authors.yml`; no line opens with a backtick).
- [ ] `roadmap.js` shipped milestone on the Unified Authentication initiative (`featured[]` untouched).
- [ ] This PRD committed at `docs/prd/rds-iam-authentication.md`.
- [ ] Open the follow-up GitHub issue (connect/discovery + `aws/rds` integration); link its `#number` in blog/roadmap/PR.

## Success Metrics

- A minted token is usable directly as `PGPASSWORD` against an IAM-auth-enabled RDS instance.
- Zero AWS CLI dependency; works in CI with only an Atmos identity.
- ≥85% patch coverage; fully offline-testable.

## Dependencies

- New: `github.com/aws/aws-sdk-go-v2/feature/rds/auth` (core `aws-sdk-go-v2` already present).
- Prior art: PR #2149 (EKS kubeconfig / `eks token`), PR #2231 (ECR Public).

## Future Enhancements

- `atmos aws rds connect` / `shell`: resolve endpoint/port/user from `<component> -s <stack>` (terraform
  outputs + stack vars) and exec `psql`/`mysql` with TLS + the RDS CA bundle.
- `aws/rds` auth **integration** kind for login-time provisioning (`~/.pgpass` / `PGPASSWORD`).
- Token caching within the ~15-minute validity window.

## References

- AWS: IAM database authentication for RDS and Aurora.
- [`eks-kubeconfig.md`](./eks-kubeconfig.md), [`ecr-public-authentication.md`](./ecr-public-authentication.md).
- Implementing PR: #<pr>. Follow-up issue: #<issue>.

## Changelog

| Version | Date | Change |
|---|---|---|
| 0.1 | 2026-07-24 | Initial draft (Phase 1: `atmos aws rds token`). |
