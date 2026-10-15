# Fix: `plan_diff.ignore_changes` warnings, unknown-value display, and validation hint

**Date:** 2026-10-13

## Summary

A field test of `components.terraform.plan_diff.ignore_changes` (PR #3378) against real Terraform found
three developer-experience gaps. None of them caused a false pass or changed a verdict, but each one made
the comparison harder to trust:

- An entry that is well formed but can never match, such as a resource address like `terraform_data.db` or
  a typo like `terraform_data.outptu`, was accepted silently. The user saw the same diff with no explanation.
- Values that become unknown in the fresh plan were rendered as removals (`- content_md5: 5d41…`). The
  `2026-10-13` edition behavior stops hiding content hashes, so every content change printed several
  misleading lines.
- The validation error for malformed entries had no hint explaining that resource addresses, module paths,
  and wildcards are unsupported.

## Context

Entries match an attribute on every resource of a type, not a resource address. Validation cannot tell an
attribute name from a resource name without provider schemas, so a mistaken address passes validation.
Terraform's JSON plan reports every schema attribute of a managed resource, so if a resource of the
configured type is present but the attribute never appears, the entry cannot match.

Terraform omits unknown values from `change.after` and records them as `true` in `change.after_unknown`.
The formatter merged only `values` and `change.after`, so an attribute that moved from known to unknown
looked absent and was printed as removed.

## Changes

- `pkg/terraform/plandiff/unmatched.go`: added `UnmatchedIgnoreChanges`, which reports entries whose
  resource type is present in either plan but whose attribute appears in none of the filtered sections.
  Entries for absent types are not reported, because the configuration is global across components.
  `WarnUnmatchedIgnoreChanges` emits one `ui.Warningf` per entry.
- `pkg/terraform/plandiff/options.go`: added `(known after apply)` line rendering on the comparison options.
  The comparison engine, formatter, and exclusions moved out of `internal/exec` into this package, which
  never runs Terraform; `internal/exec` keeps only the call sites.
- `internal/exec/terraform_plan_diff.go` and `internal/exec/terraform_verify_plan.go`: both `plan-diff` and
  deployment plan verification call `plandiff.WarnUnmatchedIgnoreChanges` before comparing with
  `plandiff.Compare`. The verdict is unchanged.
- `pkg/terraform/plandiff/compare.go` and `pkg/terraform/plandiff/resources.go`: the formatter reads
  top-level `change.after_unknown` and renders `~ attr: old => (known after apply)` or
  `~ attr: (known after apply) => new`, including for the priority attributes `id`, `url`, and `content`.
  Legacy-edition hiding still applies. Display only.
- `pkg/config/load.go`: the validation error now carries one hint describing the accepted entry shape.
- `website/docs/cli/commands/terraform/terraform-plan-diff.mdx`: documented the warning and the
  `(known after apply)` display.
- Tests: `pkg/terraform/plandiff/unmatched_test.go`, `pkg/terraform/plandiff/unknown_test.go`,
  `internal/exec/terraform_plan_diff_test.go` for the end-to-end policy call sites (serial), and a hint
  assertion in `pkg/config/terraform_plan_diff_test.go`.

## Validation

- `go build ./...` passed.
- `go test ./pkg/schema/ ./pkg/config/ ./pkg/edition/ -count=1` passed.
- `go test ./internal/exec/ -count=1 -run 'PlanDiff|Unmatched|KnownAfter|Verify|Attribute|Resource'` passed.
  The implementing subagent also ran the full `internal/exec` package, which passed.
- `atmos lint --changed` reported 0 issues (run by the implementing subagent).
- Real Terraform v1.15.6 with a local field-test fixture, built binary from this branch:
  - Entries `terraform_data.db` and `terraform_data.outptu` each printed a warning. The absent-type entry
    `aws_db_instance.latest_restorable_time` printed nothing. The valid entry `terraform_data.output`
    still produced exit `0`.
  - Changing `local_file` content now renders `~ id: … => (known after apply)` and the six hash attributes
    the same way, with exit `2`. Under `--edition=2026-10-12` the hashes stay hidden and `id` uses the
    new rendering.
  - `terraform_data.db.*` fails config loading with the new hint.
  - `atmos terraform deploy --verify-plan --ci` with a `local/dir` planfile store printed the warning for
    `terraform_data.db` and passed verification.
- CLI golden snapshot tests under `tests/` were not run; no snapshot covers `plan-diff` resource output.

## Follow-ups

None.
