# Fix: `--labels` is now a repeatable flag, matching `--tags`

**Date:** 2026-08-25

## Summary

`--labels` was a single comma-separated string flag (`flags.WithStringFlag`) while its sibling
`--tags` was already a repeatable slice flag (`flags.WithStringSliceFlag`). Repeating `--labels`
on the command line silently overwrote the previous value instead of accumulating, unlike
`--tags`. `--labels` now uses `flags.WithStringSliceFlag` everywhere it's registered, so
`--labels a=1 --labels b=2` accumulates both pairs (matching `--tags`), while
`--labels a=1,b=2` (comma-separated within one occurrence) continues to work exactly as before —
pflag's `StringSlice` flag type provides both behaviors natively.

## Context

`pkg/tags.ParseLabelsFlag` used to take a single `string` and do its own comma-splitting. Every
command that exposed `--labels` (`aws cloudformation` on other branches, `kubernetes`, `helm`,
`container`, `workflow`, `vendor pull/update/diff/verify/clean`, `terraform` bulk commands, and
every `list` subcommand with a `--labels` filter) registered it with `flags.WithStringFlag`,
so a second `--labels` occurrence on the CLI replaced the first rather than adding to it — an
inconsistency with `--tags`, which already used `flags.WithStringSliceFlag` and comma-splits
plus accumulates for free via pflag.

This work package is scoped to the code present on this branch (based on `main`, commit
`68f0fe398d`); `cmd/aws/cloudformation` does not exist here — it lives only on the unmerged
`osterman/cfn-phase4-migration-graduation` branch and was out of reach from this isolated
worktree. Every other call site enumerated in the fix plan was present and updated.

## Changes

**Core parsing (`pkg/tags/flags.go`):**
- `ParseLabelsFlag(input string) (map[string]string, error)` → `ParseLabelsFlag(input []string) (map[string]string, error)`.
  pflag's `StringSlice` type already comma-splits within one occurrence and accumulates across
  repeated occurrences, so `ParseLabelsFlag` no longer does its own comma-splitting — it just
  trims and parses each already-split element with the unchanged `splitLabelPair` helper.

**Flag registration (`WithStringFlag("labels", ...)` → `WithStringSliceFlag("labels", ..., nil, ...)`):**
- `cmd/kubernetes/kubernetes.go`, `cmd/helm/helm.go` (both also gained a `flagLabels = "labels"`
  constant to satisfy the `revive add-constant` linter once the literal's use count crossed the
  threshold from the extra `GetStringSlice` call sites)
- `cmd/container/verbs.go`
- `cmd/workflow/workflow.go`
- `cmd/terraform/flags.go` (terraform's own `--labels`/`ATMOS_LABELS` registration, via the
  `flags.FlagRegistry`/`StringSliceFlag` struct form used by `--tags` right above it)
- `cmd/vendor/vendor.go` (shared `vendorLabelsFlagHelp` const wording updated), `cmd/vendor/clean.go`,
  `cmd/vendor/diff.go`, `cmd/vendor/verify.go`, `cmd/vendor/update.go`
- `cmd/list/flag_wrappers.go`'s `WithLabelsFlag()` (shared by `list metadata/stacks/dependencies/
  sources/components`; `list instances` reuses the same options struct pattern)

**Call sites updated from `GetString("labels")`/`.Value.String()` to `GetStringSlice("labels")`:**
- `cmd/container/container.go`, `cmd/kubernetes/kubernetes.go` (`hasSelectionFlags`,
  `validateOperationArgs`, `buildConfigAndStacksInfo`), `cmd/helm/helm.go` (same three sites)
- `cmd/terraform/shared/run_options.go`, `cmd/terraform/utils.go` (`isMultiComponentInvocation`)
- `cmd/vendor/diff.go`, `verify.go`, `update.go`, `clean.go`; `internal/exec/vendor.go`
  (`ExecuteVendorPullCommand`'s presence check, `parseOptionalLabelsFlag`)
- `internal/exec/workflow.go`: `atmos workflow`'s own `--labels` stays a pass-through value
  forwarded verbatim to nested `atmos ...` step invocations (never parsed into a map in this
  file), so the fix here is reading it via `GetStringSlice` and re-joining with `,` before
  storing it in the unchanged `workflowCommandFilters.labels string` field — the receiving
  `--labels` flag downstream is also now a `StringSlice`, so it comma-splits the rejoined value
  correctly.
- Every `cmd/list/*.go` command (`metadata.go`, `sources.go`, `instances.go`, `stacks.go`,
  `dependencies.go`, `components.go`): each command's options struct changed `LabelsRaw string` →
  `LabelsRaw []string`, sourced via `v.GetStringSlice("labels")`.
- `pkg/list/list_instances.go` (`InstancesCommandOptions.LabelsRaw`, `buildInstanceFilters`) and
  `pkg/list/list_metadata.go` (`MetadataOptions.LabelsRaw`, `buildMetadataFilters`): same
  `string` → `[]string` change, plus three `opts.LabelsRaw != ""` guards changed to
  `len(opts.LabelsRaw) > 0`.

**Tests updated for the new signature/type** (converted literal comma-separated strings to
already-split `[]string` literals, matching what pflag hands `ParseLabelsFlag` after parsing —
see the `TestParseLabelsFlag` conversion note below):
`pkg/tags/flags_test.go`, `cmd/terraform/shared/execution_coverage_test.go`,
`cmd/list/sources_test.go`, `cmd/list/cmd_executor_integration_test.go`,
`cmd/list/components_test.go`, `cmd/list/instances_test.go`, `cmd/list/parse_options_test.go`,
`cmd/vendor/verify_test.go`, `cmd/vendor/clean_test.go`, `cmd/workflow/workflow_test.go`,
`internal/exec/vendor_test.go`, `internal/exec/vendor_pull_sweep_test.go`,
`internal/exec/workflow_test.go`, `internal/exec/workflow_no_stacks_test.go`,
`pkg/list/list_instances_test.go`, `pkg/list/list_instances_coverage_test.go`,
`pkg/list/list_metadata_test.go`.

`TestParseLabelsFlag`'s subtests were converted case by case rather than mechanically wrapping
each old string in a one-element `[]string`: where a subtest's intent was "multiple pairs in one
`--labels` occurrence, comma-separated" (e.g. `"cost-center=platform, compliance = sox"` or
`"team:platform,env:dev"` elsewhere), it now passes an already-split `[]string` with one pair per
element (`[]string{"cost-center=platform", " compliance = sox"}`), because that comma-splitting
now happens in pflag before `ParseLabelsFlag` ever runs, not inside the function anymore. Where a
subtest's intent was genuinely "one raw value" (a single pair, or a deliberately malformed
value), it became a one-element slice unchanged in meaning.

**New tests proving repeat-accumulation end-to-end:**
- `pkg/tags/flags_test.go`: `TestParseLabelsFlag_PflagStringSliceRepeatAccumulates` builds a real
  `pflag.FlagSet` with a `StringSlice` `--labels` flag and proves (a) two repeated `--labels`
  occurrences both survive into the parsed map, (b) a single comma-separated occurrence still
  works, and (c) the two forms compose.
- `cmd/kubernetes/kubernetes_test.go`: `TestBuildConfigAndStacksInfoLabelsRepeatAccumulates`
  drives a real `*cobra.Command` through `cmd.ParseFlags([]string{"--labels", "a=1", "--labels",
  "b=2"})` and asserts `buildConfigAndStacksInfo` sees both pairs.

**Docs (`website/docs/cli/commands/**/*.mdx`):** 32 files updated (delegated to a background
docs subagent) to document the repeatable form alongside the existing comma-separated form, and
to add a "repeat `--labels`" example wherever `--labels` was the subject of a dedicated example
block. `website/docs/cli/commands/terraform/terraform-init.mdx` and
`.../terraform-destroy.mdx` were intentionally left untouched — `--labels` only appears there
inside an unrelated flag's own composability list, not as a `--labels`-specific example.
`website/docs/cli/commands/list/list-vendor.mdx` was also left untouched: it documents that
`--labels` is *not* available on `list vendor`.

## Validation

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./pkg/tags/... ./cmd/kubernetes/... ./cmd/helm/... ./cmd/container/... ./cmd/workflow/... ./cmd/vendor/... ./cmd/terraform/... ./cmd/list/... ./pkg/list/...` — all pass.
- `go test ./internal/exec/...` — passes (225.9s).
- `./build/atmos lint --changed` (patch-scoped `custom-gcl` against `origin/main`, the same gate
  CI runs) — found and fixed two `revive add-constant` findings introduced by this patch
  (`cmd/helm/helm.go`, `cmd/kubernetes/kubernetes.go`: the `"labels"` string literal crossed the
  4-use threshold once `GetStringSlice("labels")` calls were added) by introducing a
  `flagLabels = "labels"` constant in each file; re-run reports 0 issues.
- `cd website && npm run build` — succeeds (`[SUCCESS] Generated static files in "build"`); the
  one pre-existing broken-anchor warning it reports is unrelated to this change (an
  `/changelog/mcp-for-ai-coding-assistants` anchor).
- Manual verification: `./build/atmos list components --help`, `./build/atmos workflow --help`,
  `./build/atmos vendor pull --help`, and `./build/atmos terraform apply --help=all` all show
  `--labels strings` (the repeatable pflag type), and the new automated tests above exercise
  `--labels a=1 --labels b=2` through both the raw pflag layer and a real cobra command's
  `ParseFlags`, proving both occurrences are retained rather than the second overwriting the
  first.

## Follow-ups

`cmd/aws/cloudformation` was out of scope for this branch (it doesn't exist here; it lives on the
unmerged `osterman/cfn-phase4-migration-graduation` branch). The equivalent `--labels` →
`WithStringSliceFlag` change, plus its `ParseLabelsFlag` call-site update, still needs to be
applied there before or during that branch's merge — no issue opened yet; flag for the cfn
branch owner to confirm before merging, per this repo's `project_cfn_stays_experimental`
convention of re-confirming CFN-affecting changes.
