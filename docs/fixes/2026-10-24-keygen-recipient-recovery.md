# Fix: Recover public recipients from existing age keys

**Date:** 2026-10-24

## Summary

Key generation can restore a missing SOPS recipient rule without creating another private key.

## Context

On main at `cb23b3f701`, a second vault sharing a key file, or a checkout missing `.sops.yaml`, reported existing key material and did not write a recipient. `--force` unnecessarily added a private key.

## Changes

Add an optional provider recovery interface and derive a public recipient from an existing age identity. Preserve matching rules and explicit recipients. For templated file paths, recognize an existing rule only when it equals the generated path regex or omits the path restriction; matching the literal template does not establish coverage of rendered paths. Concrete file paths still honor matching custom regex rules. Existing rules retain their order, and invalid regexes remain errors. Reject ambiguous multiple identities with remediation rather than choosing one. Cover file, inline, and store key sources. This change builds on [PR #3516](https://github.com/cloudposse/atmos/pull/3516).

## Validation

Regressions failed before the fix. Ran `go test ./cmd/secret ./pkg/secrets/providers/sops -count=1` and package lint on the isolated PR, including private-key preservation and negative recovery cases. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation.

A review regression reproduced recovery incorrectly skipping a rule because `^[^0-9]*$` matched the literal stack template, followed by `Set` failing for stack `dev1` with no matching creation rule. After the fix, local encrypt/decrypt, unchanged private-key bytes, original rule order and comments, repeated recovery, and explicit catch-all preservation pass. Ran `go test ./pkg/secrets/... ./cmd/secret/... -count=1` with an isolated SOPS default key path, followed by the recovery tests after adding catch-all cases. `go build ./...`, patch-scoped custom lint, and fix-log validation passed.

## Follow-ups

None.
