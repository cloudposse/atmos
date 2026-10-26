# Fix: demonstrate conditional imports across environments

**Date:** 2026-10-26

## Summary

Show one shared import condition selecting a retention setting that the app consumes, with a fallback for dev.

## Context

The previous example repeated the condition beside a literal `pci: true` or `pci: false` in each stack. That obscured why conditional imports are useful: an environment's classification should select behavior defined once in shared configuration. Review also requested log retention instead of an audit-logging toggle, then a shared setting consumed by the app to demonstrate how classification selects reusable configuration.

## Changes

Both environments import `catalog/environment`, which imports the app and conditionally includes the PCI mixin. The mixin supplies `settings.default.log_retention_days: 366`; the app uses `!template` with `.settings | dig "default" "log_retention_days" 7` to read the merged setting with a numeric fallback. Use the current edition's enabled template defaults. Keep the example focused on labels, with no duplicate native tag or tag-selection command. Cite PCI DSS 10.5.1: at least 12 months of audit history and immediate access to the latest three months; 366 days covers a leap year, while the example does not implement log storage or access. The cast shows both the settings producer and consumer before describing the results. Dev declares `pci: false` and retains 7 days; prod declares `pci: true` and selects 366 days. Update the Terraform input, blog, import reference, README, custom test command, and generated cast together. Frame the blog as implementing feature flags using tags and labels: inheritance already handles environment differences, while the PCI classification selects behavior across environments. Keep the runnable example focused on the label. Keep the PCI explanation brief: state the requirement, explain the leap-year allowance, and omit the redundant compliance caveat.

The post-merge review also found that import schemas accepted boolean conditions and `{cel: ...}` objects that the runtime decoder rejects. Both schemas now recursively validate the supported string/list/all/any/not forms, including normalized operator names, and reject missing negation operands. The Starlark skill now scopes component settings and execution to bound hooks and explains guards for nullable hook context in shared scripts.

## Validation

The example's `atmos test` passed: stacks validate, the label selector identifies `prod/app`, and descriptions report 7 days for dev and 366 for prod. JSON output checks confirm both values are numbers. The earlier tag variant was also checked before simplifying the example to labels. Terraform formatting and the regenerated cast validator passed; the rendered cast was visually checked. EditorConfig on files changed since HEAD, `git diff --check`, all 113 website prebuild tests, and the production website build passed. A repository-wide EditorConfig scan reported existing violations outside this change; the affected-file check passed.

The schema/decoder comparison reproduced unsupported boolean, CEL-object, and malformed nested conditions before the review fix and passes afterward for both embedded schemas. The datafetcher, condition, and import package tests pass, as do focused hook, Starlark, config-import, and stack-import regressions. All agent-skill workflow validation steps pass. Required branch lint reports four findings in unchanged `internal/exec/stack_utils.go` against an advanced `origin/main`; patch-scoped lint reports zero issues. Fix-log validation and `git diff --check` pass.

## Follow-ups

None.
