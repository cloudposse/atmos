# Fix: Isolate the ECR ambient credential failure test

**Date:** 2026-10-02

## Summary

Make the ECR credential-retrieval error test deterministic without contacting a
real EC2 metadata service or using ambient developer credentials.

## Context

Linux acceptance shard 8 failed because the test expected `context.Canceled`
from a pre-canceled request, but the AWS SDK credential cache returned an IMDS
HTTP 400 failure instead. Cancellation does not guarantee which credential
retrieval error the SDK returns. The production wrapper preserved that cause.

## Changes

Clear inherited AWS configuration for the test, use empty temporary shared
configuration files, and point IMDS at an `httptest` server that returns HTTP 400.
Assert that retrieval returns nil credentials, the ECR authentication sentinel,
the original typed SDK response error with status 400, and the Atmos identity
hint. Verify exactly one local token request. Production behavior is unchanged.

## Validation

The original test passed 20 local repetitions with real IMDS disabled; the CI
log records the environment-dependent failure. The deterministic replacement
passed 20 repetitions. The full `pkg/auth/cloud/aws` suite passed both normally
and with `-race`. Lint of the changes passed with zero issues.

## Follow-ups

None.
