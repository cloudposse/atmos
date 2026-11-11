# Fix: Review follow-ups for the v1.234.0-rc.1 regression fixes

**Date:** 2026-11-06

## Summary

CodeRabbit's review of #3637 (regressions found in v1.234.0-rc.1) found five problems in those fixes. #3637
merged before they were committed, so they land here. The fixes cover `!ci.labels` restoration,
include-not-found hints, Gomplate experimental contexts, and the `!include` path-resolution docs.

## Context

- `yamlsource.RestoreTags` restored a `!ci.labels` sequence and then decoded its items again. A label equal to a
  transported source became a tagged node, which `EncodeCILabels` rejected.
- When a `./` include was missing, the "Did you mean" hint suggested only the path. It dropped the YQ query and
  `| eval`, so copying the suggestion changed what was included.
- For template (`.yaml.tmpl`) stack manifests, the final parse wrapped the include error with `%v`. The error
  renderer could not reach the path-specific hints.
- `pkg/template/gomplate.CreateFuncs` recognized only this package's experimental marker. A context enabled
  with upstream `gomplate.SetExperimental` got the non-experimental function map, so calls such as
  `net.CIDRHost` failed.
- The `!include` and `!include.raw` docs said each path kind resolves in exactly one place. Bare paths in stack
  manifests also fall back to the directory Atmos runs from.

## Changes

- `pkg/function/yamlsource/yaml.go`: `RestoreTags` stops after restoring a node, so restored sequence items stay
  literal strings.
- `pkg/utils/yaml_include_not_found.go`, `pkg/utils/yaml_include_by_extension.go`: the suggestion replaces only
  the path in the tag's argument text. It keeps the path's quoting, the query, and `| eval`.
- `internal/exec/stack_processor_utils.go`: the merge-context wrapper uses `%w`, so cause hints stay reachable.
- `pkg/template/gomplate/funcs.go`: Gomplate's unexported experimental context key is recovered once with
  reflection, without `unsafe`. `isExperimental` accepts either marker. If the key cannot be located, only this
  package's marker is recognized, and a test fails.
- `website/docs/functions/yaml/include.mdx`, `include.raw.mdx`: each path kind has one primary location, and the
  docs name the bare-path fallback for stack manifests.
- Tests:
  - restored sequence items stay literal;
  - include hints keep the query and `| eval`, a quoted path stays quoted, and a missing absolute path names the
    requested path;
  - the include hint reaches `errUtils.AllHints` through stack processing of a `.yaml.tmpl` manifest;
  - an upstream-enabled Gomplate context selects the experimental map;
  - `!ci.labels` in locals resolves against a GitHub provider backed by a local API server;
  - each registered legacy `stack get/set/delete/format` wrapper writes its deprecation notice to stderr only;
  - replacing the edition journal with one of the same length resets the behavior index.

## Validation

- `go build ./...`: passed.
- `go test` passed in these packages: `./pkg/function/yamlsource/...`, `./pkg/function/parser/...`,
  `./pkg/template/gomplate/...`, `./pkg/utils/...`, `./pkg/edition/...`, `./cmd/stack/...`.
- `go test ./internal/exec/ -run 'Stack|Locals|Include|YAMLConfig|CILabels|Template'`: passed. A plain
  `-short` run of the package reported no tests to run, so the relevant tests were selected by name.
- The new `RestoreTags` test failed before the fix and passed after it. The template-manifest hint test failed
  with `%v` and passed with `%w`.
- `atmos lint --changed`: 0 issues.
- `git diff --check`: clean.
- Website build (`cd website && npm run build`): see the PR.

## Follow-ups

None. The review also suggested a test that isolates `GCP_META_ENDPOINT` per function map. It was not added:
Gomplate creates a new metadata client and reads that variable on every `gcp.Meta` call, so the test could not
tell per-map namespaces from shared ones.
