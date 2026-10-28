# Fix: Manual backend environment values and accurate command contracts

**Date:** 2026-10-28

## Summary

Manual backend create, update, and delete resolve explicit `!env` values while keeping unrelated configuration functions deferred. Unsupported backend inspection now returns its intended error without authenticating.

## Context

The field test reproduced environment-backed backend creation failure on both current main and the reconciled extraction. Literal configuration worked. List attempted to describe an empty component before reaching its unsupported result; help also incorrectly said forced deletion required an empty backend.

## Changes

The callback loads the component without evaluating functions, then evaluates only backend fields with the shared YAML evaluator and all non-env tags skipped. Component environment precedence, defaults, explicit-tag policy and disabled/named identity handling are retained. List/describe keep argument validation but skip unnecessary auth/config work. Help and docs say list/describe are not implemented and deletion requires `--force` even when empty; forced deletion removes stored state. No deletion semantics, flags, configuration defaults, or edition interpretation changed.

## Validation

`GOMAXPROCS=4 go test -p 2 ./cmd/terraform/backend -count=1` passed. Portable tests cover backend scope, explicit versus literal tags, malformed tags, component/process/default environment precedence, disabled identity and production callback stack loading. Fresh isolated Floci cases passed twice for literal and environment-backed CRUD, named/disabled identities, unknown-identity refusal, nonempty deletion refusal and forced removal. Empty deletion without force also refuses as designed.

Live evidence and exact command records are retained under `.context/terraform-repair-20261028/contracts/`; `*-isolated-*` cases explicitly use per-fixture encrypted file keyrings.

## Follow-ups

Final integrated validation, website build and affected help-cast regeneration are coordinated in the approved Terraform field-findings repair plan. Real cloud IAM and destructive production state operations were not tested.
