# Fix: Wait for runner security initialization before starting containers

**Date:** 2026-10-24

## Summary

Wait for StepSecurity to finish initializing before repairing DNS and allowing
subsequent workflow steps to start.

## Context

The upstream action can return after its readiness timeout while its agent is
still restarting Docker. This interrupted a secret-scanning container and raced
Docker registry-mirror configuration. An active service and DNS listener did not
prove that initialization had finished.

## Changes

The runtime waits up to 60 seconds for the agent's `Initialized` status. It checks
service health and block policy while waiting, then rechecks them and the DNS
listener before writing resolver configuration. Unexpected status, read errors,
and deadline expiry fail without DNS writes. A readiness log confirms completion
before DNS verification and following workflow steps. Cleanup and disabled repair retain
their existing behavior.

Readiness also belongs to the current service generation. The helper compares the
marker modification time with systemd's raw main-process start timestamp and
checks that the generation stays unchanged while accepting readiness. Old markers
cannot authorize a restarted agent. Exact appended `Initialized` tokens remain
valid only after a current-generation write, preserving repeated helper calls.

## Validation

New delayed-readiness and failure regressions failed against the original runtime.
The full action test suite passes with injected time, covering status creation,
deadline expiry, service and policy changes, read failures, and unchanged cleanup.
The generation follow-up passes all 134 CI-helper tests, including 46 runtime
cases covering stale/fresh generations, append-only markers, restarts, timestamp
precision, unavailable systemd data, and deadline exhaustion during acceptance.
Runtime coverage is 96.18% of lines. Entrypoint syntax checks, the unchanged
bundled parser rebuild, fix-log validation, and `git diff --check` pass. The original
readiness fix also passed `go build -p 2 ./...`; this follow-up changes no Go code.

## Follow-ups

The existing upstream TLS lookup can select an incompatible backend on a timeout.
This fix retains the strict listener check; it does not change backend selection
or recover agent startup failures caused by upstream DNS/SERVFAIL responses.
The original readiness change passed hosted container and DNS/egress validation.
The generation follow-up introduces `busctl` and requires fresh hosted validation;
earlier hosted results do not validate that dependency or the new head.
