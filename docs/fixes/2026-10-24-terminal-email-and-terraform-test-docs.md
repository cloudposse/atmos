# Fix: Render terminal email addresses once and clarify Terraform tests

**Date:** 2026-10-24

## Summary

Email autolinks no longer duplicate their `mailto:` destination in terminal messages. Terraform test documentation now explains inherited flags, stack variables, and retained files.

## Context

On main at `cb23b3f701`, `ui.info("hello a@b.com")` printed the address and its destination. The reported Terraform test behaviors were existing behavior that needed documentation.

## Changes

Render email autolinks as text in terminal Markdown while preserving explicit Markdown link destinations and HTML behavior. Document `--help=all`, stack-variable injection and test variable precedence, and generated-file retention. No variable-injection opt-out or automatic cleanup policy is introduced.

## Validation

The email regression failed before the fix. Ran `go test ./pkg/ui ./pkg/ui/markdown/... -count=1`, package lint, and the CLI reproduction, which printed the address once. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation. Terraform help and file behavior were checked and documented.

## Follow-ups

None. The Terraform behaviors are documented where users look for test guidance; they are not deferred bug fixes.
