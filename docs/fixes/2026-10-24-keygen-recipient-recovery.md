# Fix: Recover public recipients from existing age keys

**Date:** 2026-10-24

## Summary

Key generation can restore a missing SOPS recipient rule without creating another private key.

## Context

On main at `cb23b3f701`, a second vault sharing a key file, or a checkout missing `.sops.yaml`, reported existing key material and did not write a recipient. `--force` unnecessarily added a private key.

## Changes

Add an optional provider recovery interface and derive a public recipient from an existing age identity. Preserve matching rules and explicit recipients. For templated file paths, recognize the generated cross-platform regex, its equivalent native-separator form, and rules that omit the path restriction. The generated and native forms may have an explicit start anchor. Recognition is deliberately conservative: it does not infer arbitrary regex containment from a match against template syntax or sample rendered paths. Concrete file paths still honor matching custom regex rules. Existing rules retain their order, and invalid regexes remain errors. Reject ambiguous multiple identities with remediation rather than choosing one. Cover file, inline, and store key sources. This change builds on [PR #3516](https://github.com/cloudposse/atmos/pull/3516).

## Validation

Regressions failed before the fix. Ran `go test ./cmd/secret ./pkg/secrets/providers/sops -count=1` and package lint on the isolated PR, including private-key preservation and negative recovery cases. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation.

A review regression reproduced recovery incorrectly skipping a rule because `^[^0-9]*$` matched the literal stack template, followed by `Set` failing for stack `dev1` with no matching creation rule. After the fix, local encrypt/decrypt, unchanged private-key bytes, original rule order and comments, repeated recovery, and explicit catch-all preservation pass. Ran `go test ./pkg/secrets/... ./cmd/secret/... -count=1` with an isolated SOPS default key path, followed by the recovery tests after adding catch-all cases. `go build ./...`, patch-scoped custom lint, and fix-log validation passed.

An additional review regression reproduced a valid authored native-path rule being rejected with an ambiguous two-identity keyring. Native and cross-platform rules, with and without a start anchor, now preserve the recipient file and keyring byte-for-byte and encrypt/decrypt stack `dev1` without recovery. Foreign-separator rules remain unrecognized, and the earlier literal-template false-positive regression still passes. Validation for this follow-up used the full secrets and secret CLI tests with an isolated default SOPS key path, affected-package builds, patch-scoped custom lint, and fix-log validation.

## Follow-ups

None.
