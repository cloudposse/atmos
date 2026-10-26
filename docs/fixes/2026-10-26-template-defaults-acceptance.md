# Fix: template defaults acceptance snapshots and Helm literals

**Date:** 2026-10-26

## Summary

Update configuration snapshots for the new template defaults and preserve scalar `!literal` values until component template evaluation finishes.

## Context

The 2026-10-26 config edition enables templates, Sprig, and Gomplate when their settings are omitted. Seven acceptance snapshots still expected the old defaults, producing the same failures across Linux, macOS, and Windows shards. The Helm lifecycle fixture also exposed a loader bug: the loader removed `!literal` before component templating, so Atmos consumed `{{ .Values.tpl.source }}` and Helm received an empty string.

## Changes

Regenerate the seven affected snapshots through the acceptance harness. When stack templates are enabled, retain scalar literal identity in the existing YAML source transport, protect it during template passes, and unwrap its original contents during YAML function resolution without evaluating them. Keep scaffold handling and template-disabled loading unchanged.

Add regression coverage for raw and transported literals, repeated passes, Helm expressions, alternate delimiters, whitespace, multiline values, list entries, and function-looking strings. Exercise the component pipeline with current and legacy YAML-function editions.

## Validation

Reproduced all seven snapshot failures and the empty Helm value before the fix. Focused template, Starlark, literal, deferred-resolution, and explicit YAML-tag tests pass. The Helm lifecycle fixture now renders six objects with `rendered: from-stack`, preserving both ordered hooks. `go build ./...` passes. The repository short-test command passes for the source transport, deferred resolution, utilities, config, and native Helm packages. The full Kubernetes deployment lifecycle was not run locally; the failing pre-cluster render step was reproduced and all of its assertions now pass. All seven affected acceptance cases pass again without snapshot regeneration. Patch-scoped lint and `git diff --check` pass. Branch lint reports the existing complexity finding in unchanged `cmd/secret/set.go`.

## Follow-ups

None.
