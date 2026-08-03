# Fix: Document schema deprecation warnings in website docs

**Date:** 2026-07-24

## Summary

Added website documentation for the schema deprecation-warning behavior (`atmos validate
schema`/`config`, `atmos validate stacks`, and the Atmos LSP server), addressing CodeRabbit
review thread `PRRT_kwDOEW4XoM6TceLm` on PR #2793: the warnings were user-facing but
undocumented, only isolated deprecated fields had guidance.

## Context

`docs/fixes/2026-07-23-schema-deprecation-warnings-and-compatibility.md` shipped non-failing
deprecation warnings across CLI validation and the LSP, but no `website/docs` page explained
that a deprecated field is still valid input, only surfaces a warning, and never fails
validation. CodeRabbit flagged this gap against
`docs/fixes/2026-07-23-schema-deprecation-warnings-and-compatibility.md` lines 42-57 as a
still-open, unresolved review thread (the four other threads on this PR were already resolved
in commit `dd033dcbc1`, verified still intact in current code before this fix).

## Changes

- `website/docs/cli/commands/validate/validate-schema.mdx`: added a "Deprecation Warnings"
  section with example `warning:`-line output, clarifying it applies to `atmos validate
  config`/`atmos config validate` too (same underlying command) and to every schema, not just
  the built-in `config` entry.
- `website/docs/cli/commands/validate/validate-stacks.mdx`: added the equivalent section for
  stack manifests, with a `depends_on` example.
- `website/docs/lsp/lsp-server.mdx`: added a bullet under "Syntax Validation" noting the LSP
  server surfaces the same deprecation findings as warning-severity diagnostics.
- `website/docs/stacks/settings/depends_on.mdx`: noted that the direct component-level
  `depends_on` form (without the `settings` wrapper, restored for compatibility alongside
  `settings.depends_on`) is also accepted and now triggers the same `atmos validate stacks`
  warning.

## Validation

- `cd website && npm run build` — succeeded (`[SUCCESS] Generated static files in "build"`).
  The one broken-anchor warning Docusaurus reported is pre-existing, on an unrelated changelog
  page (`/changelog/mcp-for-ai-coding-assistants`), not touched by this change.
- Manually re-verified the other four CodeRabbit threads on PR #2793
  (`internal/exec/dependency_parser.go`, `internal/exec/validate_schema.go`,
  `internal/exec/validate_stacks.go`, `pkg/validator/deprecation.go`) are still fixed in
  current code — all marked resolved, addressed in commit `dd033dcbc1`, unaffected by
  subsequent changes.

## Follow-ups

None.
