# Fix: `!include` not-found errors and docs misdescribed where paths resolve

**Date:** 2026-11-05

## Summary

A missing local `!include` or `!include.raw` file always reported "tried relative to manifest … and base path …",
which was wrong for both relative path kinds. The error now names the exact location for the path's kind and, for
`./` paths, suggests the base_path form when that file exists. The function docs now say the three path kinds do not
overlap instead of describing a "first match" search.

## Context

#3426 made `./` and `../` include paths resolve relative to the including file only; v1.233.2 had silently fallen back
to `base_path`. The v1.233.2 → v1.234.0-rc.1 regression test confirmed the new behavior matches
`docs/prd/base-path-resolution-semantics.md` (FR3/FR4) and the existing docs, and that `./` paths next to a manifest or
an imported catalog file resolve identically in both versions. Projects that relied on the undocumented fallback now
fail, and the error claimed base_path had been tried. For bare paths it claimed the manifest directory had been tried,
although bare paths resolve against `base_path` and then the working directory. The docs introduced resolution as
strategies "in order, finding the first match", implying the removed fall-through.

## Changes

- `pkg/utils/yaml_include_not_found.go`: `includeNotFoundError` mirrors `findLocalFile`. A `./` or `../` path reports
  the resolved path and the including file, with a hint explaining the path kinds; it adds
  "Did you mean `!include <path>`?" (or `!include.raw`) only when the prefix-free path exists under `base_path`.
  A bare path reports `base_path` and the working directory with a hint about `./`. Absolute paths report the path.
  The specific hints also replace the misleading generic ".yaml.tmpl" hint added by locals extraction.
- `pkg/utils/yaml_include_by_extension.go`: uses the helper with the tag being processed.
- `website/docs/functions/yaml/include.mdx`, `include.raw.mdx`: a table of the three path kinds, a statement that
  kinds do not fall back to each other, the not-found behavior, imported catalog files as including files, and the
  working-directory last resort for bare paths in stack manifests.

## Validation

- New `pkg/utils/yaml_include_not_found_test.go`, through `processCustomTags`: suggestion present only when the
  base_path file exists, none for `../`, the `!include.raw` tag in its suggestion, bare-path wording and hint, the
  exact resolved path in the message, `errors.Is(ErrIncludeYamlFunctionInvalidFile)`, and a `./` path resolving next
  to the including file even when base_path has a same-named file.
- `go test ./pkg/utils/` and the `Include|Locals` tests in `./internal/exec/` passed.
- A locally built binary on the regression fixture (`./config/data.yaml` present only under base_path) printed the
  resolved path and both hints, including "Did you mean !include config/data.yaml?".

## Follow-ups

`include.mdx` states that workflow manifests and custom commands never resolve bare include paths against the working
directory, while `findLocalFile` still uses it as a last resort. That claim was not verified in this change.
