# Fix: YAML function values resolve inside template strings

**Date:** 2026-10-30

## Summary

A value like `name: "app-{{ .vars.region }}"` with `region: !env AWS_REGION` now resolves to `app-us-east-1`. Atmos previously sent an internal placeholder to Terraform, which changed on every run and contained the manifest's absolute path. In some shapes it silently produced an empty or wrong value.

## Context

Templates render before YAML functions, so explicit function tags travel through rendering as an internal transport string. That string had no end marker and was only recognized as the entire value:

- Text before it left the placeholder in the output.
- Text after it was read as part of the argument: `"{{ .vars.a }}-post"` read the variable `SYNTH-post` and returned an empty string.
- Two values in one string returned a wrong value with exit 0: `"{{ .vars.a }}/{{ .vars.c }}"` returned only the second function's result. v1.233.2 failed this shape with an error.

The regression test for the upcoming release found this. v1.233.2 also got these shapes wrong, but its results were stable.

## Changes

- `pkg/function/yamlsource`: transport strings now have a process-local terminator and nested sources are matched as balanced pairs. `Decode` accepts exactly one complete source. New `Replace`, `Embedded`, and `Transformed` helpers handle source inside larger text, and `Expression` and `Display` show it as readable function text.
- `pkg/function/embedded`: new package that substitutes embedded sources with their values. Structured results and sources rewritten by a template function now fail with `ErrYamlFuncTemplateEmbedding` rather than leaking a placeholder. YAML function processing in `internal/exec` calls it inline and supplies the evaluator.
- `internal/exec/describe_affected_utils_2.go`: embedded sources compare by function text, because their metadata contains checkout paths.
- `pkg/hooks/hooks.go`: hook values use the same evaluation.
- `website/docs/functions/yaml/index.mdx`: documents using a function value in a template, including what can't work.

No edition gate: this corrects defects in the explicit-tag path. Pins before `2026-10-20` don't use transport strings for these values and keep their earlier results.

## Validation

- Added tests first in `pkg/function/yamlsource`, `pkg/function/embedded`, `internal/exec`, and `pkg/hooks`, covering whole, prefix, suffix, middle, repeated, and multiple sources; `!literal`; `!template`; hook commands; structured results; rewritten sources; skipped functions; and authored strings staying literal.
- `go test -short` passes for `internal/exec`, `pkg/function` (including `embedded`), `pkg/hooks`, `pkg/utils`, `pkg/merge`, `pkg/config`, `pkg/secrets`, `pkg/doctor`, `errors`, and `cmd`. An earlier run found two problems, now fixed: a test helper relying on the old format, and the new error firing on interim `atmos_stack` values.
- Golden CLI tests whose names cover YAML functions, templates, hooks, and locals: 64 passed, 0 failed. The full `tests/` suite was not run locally.
- End to end with real binaries: embedded values reach the Terraform varfile correctly and stay identical across runs, under default config and explicit templates, and in hook commands. Pin `2026-10-19` keeps its earlier result.
- Sentinels against the unfixed candidate: 70 of 70 steps matched on exit status and stdout, across 14 fixtures that don't use this shape.
- Patch-scoped `custom-gcl` lint reports 0 issues.

## Follow-ups

Conditions and template functions still run before the function value exists: `{{ if eq .vars.a "x" }}` compares against the unevaluated function. v1.233.2 behaves the same way. This is documented; changing it needs a decision on evaluation order.
