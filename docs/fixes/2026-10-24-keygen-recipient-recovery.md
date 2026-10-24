# Fix: Recover public recipients from existing age keys

**Date:** 2026-10-24

## Summary

Key generation can restore a missing SOPS recipient rule without creating another private key.

## Context

On main at `cb23b3f701`, a second vault sharing a key file, or a checkout missing `.sops.yaml`, reported existing key material and did not write a recipient. `--force` unnecessarily added a private key.

## Changes

Add an optional provider recovery interface and derive a public recipient from an existing age identity. Preserve matching rules and explicit recipients. Reject ambiguous multiple identities with remediation rather than choosing one. Cover file, inline, and store key sources. This change builds on [PR #3516](https://github.com/cloudposse/atmos/pull/3516).

## Validation

Regressions failed before the fix. Ran `go test ./cmd/secret ./pkg/secrets/providers/sops -count=1` and package lint on the isolated PR, including private-key preservation and negative recovery cases. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation.

## Follow-ups

None.
