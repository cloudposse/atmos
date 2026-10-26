# Fix: shared edition history, template defaults, and CI validation

**Date:** 2026-10-26

## Summary

### Share the edition journal between the CLI and website

Generate a complete public edition history from the same journal embedded in Atmos.

### Edition-aware template enablement defaults

Honor the documented enabled defaults for stack templates, Sprig, and Gomplate while preserving earlier editions.

### Edition website tests and merged skill size

Fix the website preview dependency error and the agent-skill size failure.

### Template defaults acceptance snapshots and Helm literals

Update configuration snapshots for the new template defaults and preserve scalar `!literal` values until component template evaluation finishes.

## Context

### Share the edition journal between the CLI and website

The configuration reference and CLI documentation contained hand-maintained sample tables that omitted newer changes. The actual journal lived in a Go literal, leaving website readers without a current index.

### Edition-aware template enablement defaults

A minimal `atmos.yaml` omitted these keys and received Go zero values, disabling templates. The documented defaults and no-config-file fallback already enabled them. A settings-driven conditional-import example exposed the mismatch.

### Edition website tests and merged skill size

The edition generator tests resolved `@docusaurus/mdx-loader` from the website root even though it was only a transitive dependency. A clean pnpm install could not resolve it. Separately, GitHub tests the PR merged with main: additional Starlark guidance made that merged skill 20,542 bytes, exceeding the 20,480-byte limit even though the branch alone passed.

### Template defaults acceptance snapshots and Helm literals

The 2026-10-26 config edition enables templates, Sprig, and Gomplate when their settings are omitted. Seven acceptance snapshots still expected the old defaults, producing the same failures across Linux, macOS, and Windows shards. The Helm lifecycle fixture also exposed a loader bug: the loader removed `!literal` before component templating, so Atmos consumed `{{ .Values.tpl.source }}` and Helm received an empty string.

## Changes

### Share the edition journal between the CLI and website

Move the canonical entries to `pkg/edition/journal.yaml`, preserving their scalar types and explanatory comments. Embed and decode that file in the CLI. Generate `/cli/editions` before Docusaurus discovers documentation; the generated Markdown is ignored by Git and regenerated for builds and previews. Place Editions in the Configuration navigation group and link the history near the top of the `atmos list editions` command page. Replace the sample tables with links to the complete history. Update contributor guidance and snapshot instructions to point to the shared source.

Introduce editions by explaining how date pins defer newer defaults until the user opts in. Include an `atmos config set` example and links to configuring, listing, and describing editions.

### Edition-aware template enablement defaults

Set the three Viper defaults to true and journal their previous false values on 2026-10-26. Earlier pins retain disabled defaults when the settings are omitted; explicit values win. Leave the existing no-config-file fallback unchanged. Regenerate the defaults snapshot, document migration, and remove redundant enablement from the example.

### Edition website tests and merged skill size

Declare `@mdx-js/mdx` as a development dependency at the version already present in the lockfile and import its compiler directly. Preserve the full hook guidance in `references/hook-context.md`, with a summary and link in the Starlark skill. The merged skill is now 19,137 bytes.

### Template defaults acceptance snapshots and Helm literals

Regenerate the seven affected snapshots through the acceptance harness. When stack templates are enabled, retain scalar literal identity in the existing YAML source transport, protect it during template passes, and unwrap its original contents during YAML function resolution without evaluating them. Keep scaffold handling and template-disabled loading unchanged.

Add regression coverage for raw and transported literals, repeated passes, Helm expressions, alternate delimiters, whitespace, multiline values, list entries, and function-looking strings. Exercise the component pipeline with current and legacy YAML-function editions.

## Validation

### Share the edition journal between the CLI and website

Compared all 24 entries against a pre-migration export: every field is unchanged. Generator tests cover complete coverage, date ordering, typed values, source links, re-reading changed data, malformed entries, and MDX escaping. Go tests cover scalar types, invalid embedded documents, edition resolution, and configuration defaults. Edition command tests, `go build ./...`, patch lint, affected EditorConfig validation, and the production website build passed (117 website tests). Browser verification confirmed all 24 transitions and the shared-source edit link; the history also appears in the generated Markdown exports.

Verified the documented `atmos config set` command against a temporary configuration: both creating and updating the edition write a quoted date string without an explicit type flag. The focused Reference navigation check passed. The broader sidebar test file has an existing Policies expectation that omits two entries already present in HEAD.

### Edition-aware template enablement defaults

Config-loading tests cover unpinned, before/on/after transition, month pins, explicit overrides, and unchanged no-config-file behavior. Snapshot regeneration, journal invariants, the full configuration and edition suites, and `go build ./...` passed. The scoped `atmos test` run passed, and a separate JSON-reporting import/template run recorded 56 passing tests. Direct CLI checks confirmed the edition boundary and numeric dev/prod values of 7 and 366 without explicit template settings. The regenerated cast passed its validator and visual review. Patch-scoped lint reported zero issues; branch-wide lint still reports the pre-existing complexity violation in `cmd/secret/set.go`. EditorConfig for affected files and `git diff --check` passed. All 113 website prebuild tests and the production website build passed; the final prose edits were also compiled directly as MDX.

### Edition website tests and merged skill size

An isolated frozen-lockfile pnpm install reproduces the missing undeclared loader, while all four updated edition tests pass. All validation shell steps from the failing skill workflow pass against its exact merged checkout with the fix applied. The production website build passed, including all 117 prebuild tests. Affected EditorConfig validation and `git diff --check` passed. The required branch lint run reports the existing complexity finding in the unchanged `cmd/secret/set.go` function `findGlobalSetContext`.

### Template defaults acceptance snapshots and Helm literals

Reproduced all seven snapshot failures and the empty Helm value before the fix. Focused template, Starlark, literal, deferred-resolution, and explicit YAML-tag tests pass. The Helm lifecycle fixture now renders six objects with `rendered: from-stack`, preserving both ordered hooks. `go build ./...` passes. The repository short-test command passes for the source transport, deferred resolution, utilities, config, and native Helm packages. The full Kubernetes deployment lifecycle was not run locally; the failing pre-cluster render step was reproduced and all of its assertions now pass. All seven affected acceptance cases pass again without snapshot regeneration. Patch-scoped lint and `git diff --check` pass. Branch lint reports the existing complexity finding in unchanged `cmd/secret/set.go`.

## Follow-ups

None.
