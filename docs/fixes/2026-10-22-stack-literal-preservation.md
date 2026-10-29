# Fix: Preserve stack scalar `!literal` values through evaluation

**Date:** 2026-10-22

## Summary

Scalar values tagged with `!literal` retain their exact text through stack templates, YAML-function evaluation, and output generation.

## Context

Issue #3478 reported Go-template failures and silent value replacement despite `!literal`. The loader cleared the tag before the component template pass. The same loss of identity also allowed literal text beginning with a YAML function, such as `!env`, to execute later.

## Changes

- Stack loading carries scalar literals in a versioned string envelope through imports, merges, and cached configuration. Cache keys distinguish stack preservation from generic loading.
- Template rendering shields raw tagged scalar bodies and envelopes, including repeated evaluation and custom delimiters. Template and Starlark context reads see original text.
- Component processing, describe-stacks, and Terraform varfile/backend generation restore ordinary values after evaluation, including when evaluation is disabled. Shared cached trees remain unmodified.
- Generic config/workflow loaders retain their plain-string and step-metadata behavior. Literal protection applies to the tagged scalar; an untagged expression copying its text retains ordinary template-output semantics.

## Validation

- Reproduced the original undefined-template-function error with the baseline binary.
- Added a regression that failed before the fix for enabled templates and for literal `!env` text with templates disabled.
- Tested all combinations of template/function processing flags, nested lists, exact whitespace, imports and overrides, evaluated includes, template and Starlark references, multiple template passes, custom delimiters, raw template files with inline control actions, dynamic keys and unquoted expressions, stack identity, describe-stacks, and actual generated JSON varfiles/backends.
- Tested cache isolation and existing config/workflow literal behavior with focused Go tests.

## Follow-ups

None.
