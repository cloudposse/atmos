# Fix: Preserve ordinary templates while protecting every credential path

**Date:** 2026-10-31

## Summary

Ordinary inline conditionals render in manifests containing auth configuration. Every occurrence of an identity's `spec.env` is inspected before general template rendering, including duplicate keys and alternate conditional branches.

## Context

The credential source guard decoded raw YAML before rendering. An ordinary value such as `stage: {{ if .enabled }}prod{{ else }}dev{{ end }}` was not valid raw YAML, so the presence of auth-shaped keys caused rejection even when the credential values were literal. Path lookup also stopped at the first matching key, leaving later duplicate credential expressions exposed to general helpers.

The [PR benchmark report](https://github.com/cloudposse/atmos/actions/runs/40971188406) identified overhead in the existing `TemplateProcessingComplex` workload. Against main `3d2d6b5a6191`, the pre-fix PR measured 286.96 microseconds versus 210.55 microseconds, 212,980 versus 173,990 bytes, and 822 versus 554 allocations per operation. These are ten-sample CI measurements on Linux ARM64 with Go 1.26.10; they are not directly comparable to measurements from another machine or toolchain.

## Changes

- Project YAML-breaking control syntax only for locating scalars; preserve original source when restoring protected values and retain template markers for alias checks.
- Visit every matching auth path, preserving valid conditional branches while shielding duplicate credential expressions.
- Match a scalar's own decoded template content before using its raw span, preventing an earlier flow scalar from consuming a later credential expression.
- Reject unquoted control prefixes, including control-only values decoded as implicit null, before helpers can execute.
- Skip YAML decoding only when an ASCII credential-path key is absent and the source contains no backslash. Escaped keys continue through full decoding.
- Keep application data under `vars` and `settings` outside the auth walk, while preserving root and component credential protection.
- Give each restoration map a fresh 128-bit random nonce, preventing ordinary rendered values from colliding with predictable placeholders.
- Protect newly generated credentials after every datasource evaluation pass, then restore all generated layers after evaluation finishes.
- Recognize only complete standalone control actions. Reject inline wrappers around auth mappings with a standalone-controls hint; preserve ordinary conditional text inside application data and scalar values.
- Project standalone Go comments, including trimmed and multiline forms, without losing source positions or alias-detection markers.
- Recognize a leading auth key independently of children on later lines. Malformed-source detection also recognizes literal keys after a Go action terminator, preventing comment or empty-action prefixes from concealing credentials.
- Build source line offsets once per input and splice protected spans with one builder pass instead of repeatedly splitting or copying the full manifest.

## Validation

Regression tests reproduce the ordinary-inline and duplicate-path failures. They exercise both conditional branches, exact source restoration, quoted and block aliases, escaped keys, and all three general renderer entry points. HTTP spies assert zero requests for invalid credential forms; positive controls verify that ordinary helpers remain callable.

The unchanged curated `TemplateProcessingComplex` benchmark measures the ordinary-template path. Ten-sample local replays used the same benchmark source, Linux AMD64, Ryzen 7 7800X3D, Go `1.27.3-X:nodwarf5`, CGO disabled, `GOMAXPROCS=2`, and one-second calibration. Local builds and tests finished before measurements. Pinned benchstat compared rebuilt main `3d2d6b5a6191`, pre-fix PR `ddd373f36c20`, and fixed source `44f25f69e492`:

| Median per operation | Main | Before | Fixed |
| --- | ---: | ---: | ---: |
| Microseconds | 177.445 | 180.673 | 168.635 |
| Bytes | 176,347 | 215,486 | 175,947.5 |
| Allocations | 554 | 821 | 554 |

The allocation regression reproduced locally and returned to baseline after the fix. Fixed bytes were 0.23% below main; allocation counts matched (`p=1.000`, ten samples). Local latency before the fix was only 1.82% above main, so these runs did not reproduce the CI latency magnitude. Fixed latency was 4.96% below local main (`p=0.002`), with wider sample variability. This is rebuilt local evidence, not a historical CI comparison; fresh upstream CI is required to assess CI latency.

All 11 affected root packages, the standalone plugin suite, complete custom lint, and Go compilation passed on the final production changes. A final test-only application-data extension passed the complete source package and affected lint again. Additional public tests verify that an earlier non-mapping path does not prevent later credential protection. The source package reached 95.8% statement coverage; the root-only added-instrumented-line estimate reached 476/487 (97.74%). These local estimates exclude the standalone plugin and do not replace authoritative CI Codecov.

The final review regressions reproduce premature datasource calls, application-data shielding, and predictable-token restoration on the earlier code. After the fixes, inline, block, and whole-document generation across three passes make zero credential-helper requests while ordinary values render. All three renderer entry points preserve application variables and rendered placeholder lookalikes. Root and nested inline auth wrappers fail before helper calls; scalar application text remains renderable. A fresh ten-sample benchmark replay after these changes retained medians of 554 allocations and 175,947 bytes per operation. Shared-workstation timing is not an upstream CI latency result.

The complete maintained Mage collection at `700b5f710c3c` passed all 57 metrics with ten samples each and a complete provenance manifest. Repeated-store CLI workloads used 22 reads for affected stacks and 11 for stack descriptions; the caller-stack workload used 22. Subsequent comment, split-key, settings, and indexing fixes were validated with the affected suites and another ten-sample complex-template replay: medians remained 554 allocations and 175,948 bytes. The complete snapshot belongs to `700b5f710c3c`; it is not a full-suite benchmark claim for the later source revision.

Further regressions reproduce and reject helper execution behind split auth keys, including escaped/component forms and leading comment or empty actions. Supported standalone comments render normally, structured settings and ordinary conditional application data render normally, and mixed Unicode quoted/block/flow scalars retain exact source through protection and restoration.

## Follow-ups

None.
