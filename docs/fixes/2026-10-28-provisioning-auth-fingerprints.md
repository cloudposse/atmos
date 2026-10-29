# Fix: Use keyed fingerprints for provisioning auth scopes

**Date:** 2026-10-28

## Summary

Use HMAC-SHA256 with a random process-local secret for source and output provisioning cache identities that include authentication material.

## Context

CodeQL flagged unkeyed SHA-256 fingerprints of serialized authentication configuration in both provisioning caches. These are in-memory reuse keys, not password-verification records, but an unkeyed digest can still reveal whether a guessed credential matches a retained key.

## Changes

Share a fingerprint helper with a secret initialized once per process. Never persist the secret or fingerprints. Preserve all identity inputs, including checkout, component, source version, and auth scope, so unchanged inputs reuse provisioning and changed credentials cause another attempt.

Add an HMAC test vector, concurrent key-stability coverage, and source/output regressions for unchanged versus rotated credentials.

## Validation

Full `pkg/auth/fingerprint`, `pkg/provisioner/source`, and `pkg/terraform/output` suites pass with the race detector, including the credential-rotation regressions.

Hosted CodeQL must confirm the two reported flows are eliminated. No suppression or password-hashing exception was added.

## Follow-ups

Confirm the updated lifecycle PR's CodeQL analysis and native Windows acceptance tests.
