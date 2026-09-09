# Fix: Helm runtime value override docs — missing `values` command and `--all` fan-out warning

**Date:** 2026-09-09

## Summary

A `/field-test` pass on PR #3094 (native Helm runtime value overrides and `atmos helm values`)
found two documentation gaps in `website/docs/cli/commands/helm/usage.mdx`'s "Runtime value
overrides" section: it omitted `atmos helm values` from the list of commands that accept
overrides, and it never mentioned that combining an override flag with `--all`/`--affected`/
`--tags`/`--labels` applies the same override to every matched component.

## Context

The field test built `./build/atmos` from the branch and ran real commands against the
`examples/helm` fixture. Two findings came out of that pass:

1. `usage.mdx` line 58 read "`template`/`render`, `diff`/`plan`, and `apply`/`deploy` accept Helm-compatible value overrides," leaving out `values` even though `helm-values.mdx`, the code (`operationSupportsValueOverrides` in `cmd/helm/helm.go`), and `agent-skills/skills/atmos-helm/SKILL.md` all correctly include it. A reader of only that paragraph could wrongly conclude `values` doesn't take overrides.
2. Live testing (`atmos helm template --all -s dev --set image.tag=blast-all`) confirmed the same override is broadcast identically to every component matched by `--all`. This is consistent with how `--all` already works for every other flag, but it was undocumented anywhere near the override or `--all`/`--affected` flag descriptions in `usage.mdx`, `helm-diff.mdx`, or `helm-template.mdx` — a real hazard for `apply --all --set ...`, which can push an unintended value onto every release in a batch.

This is a documentation-only fix; no runtime behavior changed.

## Changes

- `website/docs/cli/commands/helm/usage.mdx`: added `values` to the "Runtime value overrides"
  intro sentence (and to the "use the same arguments with ... to inspect and preview" follow-up
  sentence), and added a new paragraph after the precedence rules explaining that `values` accepts
  exactly one component while `template`/`diff`/`plan`/`apply`/`deploy` compose with
  `--all`/`--affected`/`--tags`/`--labels`, and that doing so applies the override identically to
  every matched component with no per-component targeting.

`helm-diff.mdx`, `helm-template.mdx`, and `helm-apply.mdx` all link to
`/cli/commands/helm/usage#runtime-value-overrides` for the full override reference, so the new
warning is visible from every command page without duplicating it four times.

## Validation

- `cd website && npm run build` — succeeded. The only broken-anchor warnings reported are
  pre-existing and unrelated to this change (`/changelog/mcp-for-ai-coding-assistants`,
  `/functions/yaml/terraform.output`, `/functions/yaml/terraform.state`).
- Re-read the edited section rendered in the build output to confirm the added sentences read
  correctly in context.
- No code changed, so no `go build`/`atmos test` run was needed.

## Follow-ups

None.
