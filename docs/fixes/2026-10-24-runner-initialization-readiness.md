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

## Validation

New delayed-readiness and failure regressions failed against the original runtime.
The full action test suite passes with injected time, covering status creation,
deadline expiry, service and policy changes, read failures, and unchanged cleanup.
All 112 CI-helper tests pass. The bundled parser rebuild is unchanged, fix-log
validation and `git diff --check` pass, and `go build -p 2 ./...` succeeds.

## Follow-ups

The existing upstream TLS lookup can select an incompatible backend on a timeout.
This fix retains the strict listener check; it does not change backend selection
or recover agent startup failures caused by upstream DNS/SERVFAIL responses.
Hosted validation of this readiness fix is still required.
