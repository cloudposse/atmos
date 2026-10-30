# Fix: Resolve absolute component validation schema paths

**Date:** 2026-10-22

## Summary

Component validation accepts absolute `schemas.jsonschema.base_path` and `schemas.opa.base_path` values.

## Context

Joining these paths with the project root using `filepath.Join` nested an absolute schema directory underneath the project, making existing schema files appear missing. OPA module directories had the same problem.

## Changes

Use the existing absolute-aware path helper for schema base directories, schema filenames, and OPA module-path bases.

## Validation

A regression validates JSON Schema and OPA policies stored outside a disposable project, including an OPA module import and a failing JSON input. Live component validation with both absolute schema bases succeeds twice.

## Follow-ups

Schema file existence and policy validation remain unchanged.
