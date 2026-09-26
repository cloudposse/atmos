# Atmos Pro CLI exception reporting

## Goals

Report CLI failures to the configured Atmos Pro deployment when effective
`settings.pro.enabled` is true, independently of a separately configured Sentry
destination. Preserve masked metadata, event IDs, fingerprints, command exit codes,
and the execution ID used by execution uploads. Resolved presence tags and labels
use unprefixed Sentry keys, such as `production: "true"` and `team: "platform"`.

## Migration and editions

Previously, `settings.pro.enabled: true` selected Pro integration for a stack or
component but did not cause the CLI to send exceptions to Pro. After this change,
the same stored value also enables exception reporting in GitHub Actions when a
repository ID and GitHub OIDC credentials are available. No additional Sentry DSN
is required. CLI-wide Pro enablement remains false when omitted.

This automatic behavior is intentional for Pro-enabled projects. Set
`settings.pro.errors.enabled: false` or `ATMOS_PRO_ERRORS_ENABLED=false` to retain
the previous exception-delivery behavior without disabling other Pro features.
Component settings override invocation defaults, while the errors environment
override takes precedence over component values.

This is a reinterpretation of an existing enabled value, not a literal default
value change. It is not gated by the edition journal: `KindBehavior` resolution
is not implemented. The candidate is recorded in [the editions roadmap](editions.md#roadmap-v2).

## Delivery and validation

The transport posts Sentry envelopes to
`/api/v1/events/{github_repository_id}/exceptions`, using a fresh token with audience
`atmos-pro.com` in `X-Sentry-Auth`. Delivery has a two-second budget including token
acquisition, a queue of 32 events, a 1 MiB envelope limit, and a shared two-second
shutdown flush. Reporting failures do not change the command's exit result.

The existing Atmos consumer Pact covers successful ingestion and authentication
rejection with deterministic credentials. Local tests cover masking, context,
duplicate suppression, opt-outs, fan-out, token refresh, and cancellation.
[Provider verification and persisted-tag assertions](../../specs/001-pact-consumer-contracts/contracts/exceptions.md)
remain Atmos Pro backend acceptance work, tracked in
[issue #3219](https://github.com/cloudposse/atmos/issues/3219).
