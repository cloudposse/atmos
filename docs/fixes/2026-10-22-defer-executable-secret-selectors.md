# Fix: Defer executable secret selectors during discovery

**Date:** 2026-10-22

## Summary

Secret listing and service discovery leave executable selectors unevaluated. An authenticated operation evaluates only the declaration it actually requests.

## Context

On the unchanged baseline, `atmos secret list -s dev -c app` executed an `!exec touch <marker>` in a declaration's `sops` selector without `--verify`. Native tags and string-form functions reproduced the side effect twice. The discovery skip list deferred backend reads but omitted executable functions and cloud identity queries.

## Changes

Add exec, Starlark and cloud identity reads to the discovery skip list. Preserve local declaration templates and generated declaration shapes. Keep the existing lazy authenticated selector evaluator for explicitly requested secrets.

## Validation

Regression tests first reproduced eager selector evaluation. The complete secret-command suite now covers native/string exec selectors, template dependencies, provider fields and Starlark selectors: listing, enumeration and service loading remain inert; explicitly getting the selected secret evaluates its selector. Existing generated-declaration and lazy CloudFormation-selector tests remain covered.

## Follow-ups

None. Local declaration expressions continue to resolve during discovery; this fix defers executable and authenticated selector functions.
