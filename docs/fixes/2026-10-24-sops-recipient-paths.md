# Fix: Resolve nested SOPS recipient files and relative rules

**Date:** 2026-10-24

## Summary

Key generation and encryption agree on paths when the recipient configuration is in a nested `.sops.yaml`.

## Context

On main at `cb23b3f701`, a recipient file at `secrets/.sops.yaml` received a repository-relative regex that could not match. Discovery also skipped the file adjacent to the encrypted data.

## Changes

Generate creation-rule patterns relative to the recipient file’s directory. When `spec.file` is omitted, reuse the scope-derived path with a wildcard covering stack and instance filenames under `spec.path` (default `secrets`). Emit separator classes matching both forward slashes and backslashes, so committed rules work on Unix and Windows. Start SOPS configuration discovery from the encrypted file path, normalized to forward slashes for SOPS’s POSIX-path discovery API. Keep the original native path for creation-rule matching. Add local encrypt/decrypt regressions for both relative and absolute encrypted file paths and update the secrets documentation.

## Validation

Regressions failed before the fix. Ran `go test ./pkg/secrets/providers/sops -count=1` and package lint on the isolated PR. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation. Encryption tests run locally without external services. Review regressions reproduced the omitted-file failure and Windows-separator mismatch before the follow-up fix. The full SOPS package tests passed afterward, including default, relative, and absolute scope directories with root and nested recipient files; both stack and instance secrets were encrypted and decrypted. Regex tests assert exact patterns, Unix and Windows matches, and rejection of unrelated paths and extensions. Follow-up `go build ./...`, patch-scoped custom lint, and fix-log validation passed. Full package lint still reports existing findings on unchanged lines: two G703 path-traversal warnings and the `sops.go` file-length limit.

Windows CI exposed five failing nested-recipient encryption cases because the discovery API interpreted backslashes as filename characters. The boundary fix keeps those regression cases and adds a test selecting the nearest recipient file over a different root recipient, with an anchored relative-path rule. Local macOS validation passed for the full secrets packages and secret CLI tests with an isolated `SOPS_AGE_KEY_FILE`, `go build ./...`, patch-scoped custom lint, and fix-log validation. Without the isolated key-file environment, an existing CLI keygen test sees the developer’s default SOPS key and skips generation; existing private-key files were left unchanged. Windows execution remains to be confirmed by CI.

## Follow-ups

Pre-existing limitation: arbitrary `recipients_file` names outside `.sops.yaml` discovery are not explicitly loaded during encryption. Per the task’s scope boundary, this is recorded in [PR #3516](https://github.com/cloudposse/atmos/pull/3516), without expanding the reported nested `.sops.yaml` fix or opening a new issue.
