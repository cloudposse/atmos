# Fix: demonstrate conditional imports across environments

**Date:** 2026-10-26

## Summary

Show one shared import condition selecting a retention setting that the app consumes, with a fallback for dev.

### conditional-import validation across YAML tags and native metadata (2026-10-25)

Align conditional-import/deferred tests with explicit YAML-tag handling, and correct metadata edge cases found while field-testing native tag support.

## Context

The previous example repeated the condition beside a literal `pci: true` or `pci: false` in each stack. That obscured why conditional imports are useful: an environment's classification should select behavior defined once in shared configuration. Review also requested log retention instead of an audit-logging toggle, then a shared setting consumed by the app to demonstrate how classification selects reusable configuration.

### conditional-import validation across YAML tags and native metadata (2026-10-25)

CI tested the PR merged with a newer `main`, whose YAML-function policy distinguishes explicit tags from quoted strings. Branch-only tests had constructed raw function strings and therefore missed that integration. The live metadata pass then found unavailable tag lists being treated as empty, literal exclamation-prefixed tags being rejected, and invalid custom-column syntax in the draft example.

## Changes

Both environments import `catalog/environment`, which imports the app and conditionally includes the PCI mixin. The mixin supplies `settings.default.log_retention_days: 366`; the app uses `!template` with `.settings | dig "default" "log_retention_days" 7` to read the merged setting with a numeric fallback. Use the current edition's enabled template defaults. Keep the example focused on labels, with no duplicate native tag or tag-selection command. Cite PCI DSS 10.5.1: at least 12 months of audit history and immediate access to the latest three months; 366 days covers a leap year, while the example does not implement log storage or access. The cast shows both the settings producer and consumer before describing the results. Dev declares `pci: false` and retains 7 days; prod declares `pci: true` and selects 366 days. Update the Terraform input, blog, import reference, README, custom test command, and generated cast together. Frame the blog as implementing feature flags using tags and labels: inheritance already handles environment differences, while the PCI classification selects behavior across environments. Keep the runnable example focused on the label. Keep the PCI explanation brief: state the requirement, explain the leap-year allowance, and omit the redundant compliance caveat.

The post-merge review also found that import schemas accepted boolean conditions and `{cel: ...}` objects that the runtime decoder rejects. Both schemas now recursively validate the supported string/list/all/any/not forms, including normalized operator names, and reject missing negation operands. The Starlark skill now scopes component settings and execution to bound hooks and explains guards for nullable hook context in shared scripts.

### conditional-import validation across YAML tags and native metadata (2026-10-25)

- Merge `main` and construct explicit YAML source values in deferred/CEL fixtures, retaining tests that reject quoted `!cel` strings.
- Keep unknown tag lists distinct from known empty/null lists. Conditions needing unknown tags fail evaluation instead of enabling negative membership checks.
- Preserve unknown state across append/merge operations; a known replacement or null can clear it.
- Respect literal YAML strings beginning with `!`; actual YAML functions remain unavailable during early import resolution.
- Use default `list instances` columns in the example, cast, and blog.
- Rewrite the blog around the same native metadata selecting configuration and component instances, with scope/type limitations stated explicitly.

## Validation

The example's `atmos test` passed: stacks validate, the label selector identifies `prod/app`, and descriptions report 7 days for dev and 366 for prod. JSON output checks confirm both values are numbers. The earlier tag variant was also checked before simplifying the example to labels. Terraform formatting and the regenerated cast validator passed; the rendered cast was visually checked. EditorConfig on files changed since HEAD, `git diff --check`, all 113 website prebuild tests, and the production website build passed. A repository-wide EditorConfig scan reported existing violations outside this change; the affected-file check passed.

The schema/decoder comparison reproduced unsupported boolean, CEL-object, and malformed nested conditions before the review fix and passes afterward for both embedded schemas. The datafetcher, condition, and import package tests pass, as do focused hook, Starlark, config-import, and stack-import regressions. All agent-skill workflow validation steps pass. Required branch lint reports four findings in unchanged `internal/exec/stack_utils.go` against an advanced `origin/main`; patch-scoped lint reports zero issues. Fix-log validation and `git diff --check` pass.

### conditional-import validation across YAML tags and native metadata (2026-10-25)

The original failing cases passed against merged `main`, including under the race detector. Condition, deferred, merge, import, and config package suites passed. The CI compatibility commit cleared the original test failures; two later jobs failed before tests because the StepSecurity agent exited on DNS lookup failures.

The native-metadata field test reproduced each defect twice, then verified both runtime fixes twice with the rebuilt CLI. It also verified the happy path, combined selectors, case sensitivity, list strategies, structural append, skipped imports, and documented scope/type boundaries. See `docs/field-tests/2026-10-25-native-metadata-imports.md` for exact repro commands and observed results.

## Follow-ups

None.

### conditional-import validation across YAML tags and native metadata (2026-10-25)

None for the implemented fixes. Final cast, lint, test, and website results are recorded in the PR validation summary.
