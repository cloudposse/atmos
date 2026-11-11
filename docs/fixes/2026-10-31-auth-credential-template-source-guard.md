# Fix: Preserve ordinary templates while protecting every credential path

**Date:** 2026-10-31

**Updated:** 2026-11-01

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
- Skip YAML decoding only when an ASCII credential-path key is absent and the source contains neither backslashes nor candidate templated mapping keys. Escaped keys, tagged keys, and aliases used as keys continue through full decoding; ordinary value tags and aliases retain the fast path.
- Keep application data under `vars` and `settings` outside the auth walk, while preserving root and component credential protection.
- Give each restoration map a fresh 128-bit random nonce, preventing ordinary rendered values from colliding with predictable placeholders.
- Protect newly generated credentials after every datasource evaluation pass, then restore all generated layers after evaluation finishes.
- Recognize only complete standalone control actions. Reject inline wrappers around auth mappings with a standalone-controls hint; preserve ordinary conditional text inside application data and scalar values.
- Project standalone Go comments, including trimmed and multiline forms, without losing source positions or alias-detection markers.
- Recognize a leading auth key independently of children on later lines. Malformed-source detection also recognizes literal keys after a Go action terminator, preventing comment or empty-action prefixes from concealing credentials.
- Build source line offsets once per input and splice protected spans with one builder pass instead of repeatedly splitting or copying the full manifest.
- Reject templated structural auth keys before general rendering, including partial and multiple expressions, aliases, tagged keys, and malformed double quoting. Keep dynamic component and identity names supported. Memoize fixed credential-shape probes by node and path position so shared merge graphs stay bounded; cycles fail closed.
- Distinguish mapping-key colons from ordinary expressions and assignments. After a YAML decoding failure, inspect an inert action projection for auth ownership before rejecting dynamic application data; never evaluate helpers to identify a declaration.
- Recognize explicit YAML keys such as `? auth` during malformed-source checks.
- Follow configuration aliases and merge maps through the same ownership visitor as literal configuration, including component auth anchored beneath `vars` and `settings`. Bound cycles and deduplicate physical auth declarations so repeated or direct-plus-merged ownership cannot produce overlapping replacements; leave application data sections renderable.
- Inspect original bounded quoted/block scalar source through aliases and properties, including comments between tags, anchors, and values. Retain projected controls with an input-local marker so prefixed plain credential values cannot lose their markers; preserve decoder rune columns and the no-auth fast path.
- Ignore separated YAML comments in the raw key scan while retaining quoted and adjacent hash characters. Test an actual datasource call in a structural key as well as credential-value calls.
- Let parsed inert ownership decide malformed-source handling, so matching auth-shaped keys beneath root/component `vars` and `settings` remain application data. Consult coarse raw key evidence only when the inert projection cannot decode.
- Limit inline-auth wrapper detection to a leading auth key. Conditional ordinary configuration containing auth-shaped application data retains normal rendering.
- Explain the deliberately conservative ambiguity policy in the error and public plugin documentation, including the valid-raw-YAML rewrite that restores dynamic-key rendering.

## Validation

Regression tests reproduce the ordinary-inline and duplicate-path failures. They exercise both conditional branches, exact source restoration, quoted and block aliases, escaped keys, and all three general renderer entry points. HTTP spies assert zero requests for invalid credential forms; positive controls verify that ordinary helpers remain callable.

The unchanged curated `TemplateProcessingComplex` benchmark measures the ordinary-template path. Ten-sample local replays used the same benchmark source, Linux AMD64, Ryzen 7 7800X3D, Go `1.27.3-X:nodwarf5`, CGO disabled, `GOMAXPROCS=2`, and one-second calibration. Local builds and tests finished before measurements. Pinned benchstat compared rebuilt main `3d2d6b5a6191`, pre-fix PR `ddd373f36c20`, and fixed source `44f25f69e492`:

| Median per operation | Main | Before | Fixed |
| --- | ---: | ---: | ---: |
| Microseconds | 177.445 | 180.673 | 168.635 |
| Bytes | 176,347 | 215,486 | 175,947.5 |
| Allocations | 554 | 821 | 554 |

The allocation regression reproduced locally and returned to baseline after the fix. Fixed bytes were 0.23% below main; allocation counts matched (`p=1.000`, ten samples). Local latency before the fix was only 1.82% above main, so these runs did not reproduce the CI latency magnitude. Fixed latency was 4.96% below local main (`p=0.002`), with wider sample variability. This is rebuilt local evidence, not a historical CI comparison; fresh upstream CI is required to assess CI latency.

At `b417cf66dbfc`, all 11 affected root packages, the standalone plugin suite, complete custom lint, and Go compilation passed on the production changes. A subsequent test-only application-data extension passed the complete source package and affected lint again. Additional public tests verify that an earlier non-mapping path does not prevent later credential protection. The source package reached 95.8% statement coverage; the root-only added-instrumented-line estimate reached 476/487 (97.74%). These local estimates exclude the standalone plugin and do not replace authoritative CI Codecov.

The final review regressions reproduce premature datasource calls, application-data shielding, and predictable-token restoration on the earlier code. After the fixes, inline, block, and whole-document generation across three passes make zero credential-helper requests while ordinary values render. All three renderer entry points preserve application variables and rendered placeholder lookalikes. Root and nested inline auth wrappers fail before helper calls; scalar application text remains renderable. A fresh ten-sample benchmark replay after these changes retained medians of 554 allocations and 175,947 bytes per operation. Shared-workstation timing is not an upstream CI latency result.

The complete maintained Mage collection at `700b5f710c3c` passed all 57 metrics with ten samples each and a complete provenance manifest. Repeated-store CLI workloads used 22 reads for affected stacks and 11 for stack descriptions; the caller-stack workload used 22. Subsequent comment, split-key, settings, and indexing fixes were validated with the affected suites and another ten-sample complex-template replay: medians remained 554 allocations and 175,948 bytes. The complete snapshot belongs to `700b5f710c3c`; it is not a full-suite benchmark claim for the later source revision.

Further regressions reproduce and reject helper execution behind split auth keys, including escaped/component forms and leading comment or empty actions. Supported standalone comments render normally, structured settings and ordinary conditional application data render normally, and mixed Unicode quoted/block/flow scalars retain exact source through protection and restoration.

Structural-key tests cover root, identities, spec, and env keys with partial/multiple expressions, escaping, flow/unquoted forms, aliases, merges, cycles, URI tags, and malformed double quotes. All three host renderers assert zero HTTP helper requests on rejected inputs, with callable ordinary-helper positive controls. Dynamic logical names and application data still render; the multiline-control case rejects before credential execution. Compatibility regressions also cover bare expressions, assignments, colons in action arguments, legacy datasource declarations, and inline generated YAML. The complete source package and all 36 compatibility tests caught by the broader run passed after narrowing the malformed-source check.

At `e0b926f43239`, after merging upstream `0f878a270969`, all 11 affected root packages, the standalone plugin suite, full custom lint, root Go compilation, and production website build passed. Two public tests distinguish generated identities from block-style structural keys and preserve ordinary rendering beside non-environment identities. The complete affected suite passed again with those tests; affected lint passed and source coverage reached 357/372 statements (96.0%).

The remaining inline findings were reproduced before fixing them: explicit-key malformed input, promoted merge auth, and projected control-only credential values executed source or HTTP helpers before authentication. Regression tests now cover all three host renderers, merge chains/repeated ownership/cycles, split scalar properties and comments, and prefixed plain controls. Balanced quoted/block templates, ordinary application controls, Unicode source positions, flow neighbors, and quoted/adjacent hashes remain supported. Three source-only fixtures verify merge-sequence error propagation, a block control closing outside its value, and a shape-probe cycle through application data.

Additional public regressions reproduce component auth inherited through root merges and component-map aliases, and malformed raw application templates whose auth-shaped data must render normally. The shared visitor now handles references consistently; all three host renderers defer promoted credentials, preserve exact restoration, and render unowned application data. Conditional data wrappers pass for both branches. The ambiguity tests verify the static ownership explanation, actionable hint, zero helper calls before rejection, and a successful valid-raw-YAML rewrite. The complete 11-package root suite and standalone plugin, full custom lint, root compilation, and production website build passed with the combined changes. Source coverage reached 440/463 statements (95.0%); these are scoped own-package measurements, with CI Codecov authoritative.

If the inert projection still cannot decode, ownership remains ambiguous and raw auth-key evidence can still cause rejection. Such inputs need valid raw YAML structure and quoted or block credential templates; the fallback does not execute helpers to guess their output.

## Follow-ups

Upstream fork-workflow approval and current-head CI are required before merge. A maintainer must approve the pending runs.
