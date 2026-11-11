# Fix: `!ci.labels` sequence arguments in `locals` fail with explicit YAML tags

**Date:** 2026-11-05

## Summary

A `!ci.labels [label, ...]` sequence inside `locals:` failed every stack-loading command with
`!ci.labels requires no arguments or a nonempty list of strings, got the scalar "[\"preview\"]"` when
explicit YAML tags were active (unpinned projects and edition pins on or after `2026-10-20`). The same tag
under `vars:` worked, and earlier pins were unaffected. Sequence arguments now survive YAML template passes
under every edition.

## Context

The v1.233.2 → v1.234.0-rc.1 regression test found the failure with the baseline fixture
`ci-labels-adversarial` (`validate stacks` exited 0 on v1.233.2 and 1 on the release candidate) and reduced it
to `locals: {labels: !ci.labels [preview]}`.

With explicit tags, the stack loader transports a deferred function as an encoded `yamlsource.Source`
string. `handleCILabelsTag` encoded the sequence argument as a JSON flow sequence (`["preview"]`). Locals are
re-serialized for a YAML template pass, and `yamlsource.RestoreTags` restored every source as a double-quoted
tagged scalar (`!ci.labels "[\"preview\"]"`). Re-parsing that YAML produced a scalar `!ci.labels` node, which
`parser.EncodeCILabels` deliberately rejects so users cannot write a quoted list. The legacy path stores a
plain `!ci.labels ["preview"]` string and never goes through `RestoreTags`, which is why pins up to
`2026-10-19` worked. This was an accidental defect of the explicit-tag path, not an intended behavior change,
so it is fixed for every edition rather than gated.

## Changes

- `yamlsource.Source` records `Sequence` when a tag was written with a YAML sequence argument.
- `yamlsource.RestoreTags` restores such sources as tagged flow sequences (`!ci.labels ["preview"]`), falling
  back to the existing quoted-scalar form when the argument is not a flow sequence.
- `handleCILabelsTag` sets `Sequence` for sequence arguments before rewriting the node into a transported scalar.
- `!ci.labels` validation is unchanged: an authored quoted scalar argument is still rejected.
- Other collection-argument tags were checked. `!literal` only transports scalars, `!append` resolves
  immediately, and the remaining deferred encoders (`!cel`, `atmos.yaml` tags, generic function deferral) carry
  scalar arguments, so `!ci.labels` was the only deferred tag affected.

## Validation

- New `internal/exec/stack_locals_ci_labels_test.go` runs global, section-specific and nested locals with block
  and flow sequences, the bare tag, and `!literal` sequence/mapping arguments through both locals extraction and
  `ProcessYAMLConfigFileWithOptions` for unpinned, `2026-10-20` and `2026-10-19` editions, asserting resolved
  values. Before the fix, 8 sequence cases failed under the explicit-tag editions; after it, all pass. A negative
  test confirms a quoted scalar `!ci.labels "[preview]"` is still rejected under both behaviors.
- New `pkg/function/yamlsource/yaml_test.go` covers sequence restoration and re-parse, the scalar fallback for
  invalid or unparsable sequence arguments, and transport of the `Sequence` flag.
- `go test -short` passed for `./pkg/utils/...`, `./pkg/function/...`, `./pkg/yaml/...`, `./pkg/config/...`
  and the full `./internal/exec/` package (8,068 passing tests, no failures); `go build ./...` passed.
- `gofumpt -l` reported no changes; `atmos lint --changed` reported 0 issues.

## Follow-ups

None.
