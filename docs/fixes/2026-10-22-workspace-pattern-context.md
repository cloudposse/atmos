# Fix: Populate workspace pattern context for every stack naming strategy

**Date:** 2026-10-22

## Summary

A component using `metadata.terraform_workspace_pattern: '{tenant}-{environment}-{stage}'` now gets its workspace from its context variables when stack names use a template or the manifest filename.

## Context

The field fixture had tenant `team`, environment `local`, and stage `metadata`, but `describe component` returned workspace `--`. Context variables were populated only in explicit-manifest-name and name-pattern branches, leaving the name-template and filename branches empty.

## Changes

Initialize context from component vars before selecting the stack naming strategy. Continue assigning component/base-component context and preserve existing workspace override precedence.

## Validation

The new full-path workspace test failed before the fix (`--` instead of `team-local-dev`) and passed afterward. Additional tests cover manifest name, name template, name pattern, and filename strategies, including the component token. Existing workspace and stack-manifest-name tests passed in the focused suite. Before/after logs are in `.context/skills-field-test/product-regressions-{before,after}.log`.

## Follow-ups

None.
