# Fix: CloudFormation field-test findings at the stack tip

**Date:** 2026-10-01

## Summary

A second real-AWS field test of the cumulative `atmos aws cloudformation` feature ran at the stack tip, after the earlier remediation in `c6afb03238`. It confirmed that those fixes hold, and it found four high-severity defects, eight medium ones and a long tail of lower-severity issues. This change fixes all of them except the items listed under Follow-ups.

The four high-severity defects:
- List-form `parameters:` were silently dropped.
- `!aws.cloudformation.output` returned null instead of an error.
- `diff`/`plan` left an empty `REVIEW_IN_PROGRESS` stack behind.
- Publish-only `apply` produced no output.

## Context

The test ran three parallel lanes (lifecycle, delivery, and bulk/hooks/CI/outputs) against the dev and sandbox accounts in us-east-2. A scratch git repository covered `--affected`. Each finding below names what a user would have seen.

- **List-form parameters (high).** `normalizeParameters` returned `nil, nil` for any value that was not a map. An AWS-CLI or Rain style `[{ParameterKey, ParameterValue}]` list, or an `!include` of a JSON array, deployed the template defaults with exit code 0. `diff` then reported "no changes".
- **Missing outputs resolved to null (high).** `!aws.cloudformation.output` returned `outputs[key], nil`, so a misspelled key resolved to null. A stack that was never deployed did too. Apply then sent an empty string to CloudFormation.
- **Preview stubs left behind (high).** Deleting a CREATE-type preview changeset leaves an empty `REVIEW_IN_PROGRESS` stack. One `diff --all` left nine. `list` reported them as managed, and `output` printed an empty table for them.
- **Silent publish-only apply (high).** When the selected target was an `aws/s3` publish target, apply exited 0 with no output and never said where the template went.
- **Prompt before preview (medium).** Apply asked for confirmation before it created a changeset, and it never showed the predicted changes.
- **Bucket created by read-only verbs (medium).** `validate`, `diff` and `changeset create` provisioned the artifact bucket. Every run also re-uploaded an identical template as a new object version.
- **`ROLLBACK_COMPLETE` (medium).** A stack in `ROLLBACK_COMPLETE` failed with a raw AWS error and no hint.
- **Leftover changesets (medium).** A no-op apply left a `FAILED` changeset behind every time, and changeset names doubled the `atmos-` prefix.
- **Bulk output (medium).** `output --all --format=json` printed concatenated documents that do not parse.
- **Backend list (medium).** The errors named `atmos terraform`.
- **StackSet target on apply (medium).** `apply --target <aws/stackset>` reported the kind as "not registered".
- **Packaging docs (medium).** The `packaging:` disambiguator was undocumented.

The full report and per-lane repros live in the field-test working notes; they were not committed.

## Changes

- **Stack spec and outputs**
  - `parameters:` accepts the map form and the AWS-CLI/Rain list form, including `UsePreviousValue`. Any other shape is a typed error.
  - Capabilities are validated against the SDK enum.
  - `!aws.cloudformation.output` returns an error that lists the available keys when the requested key is missing.
  - A stack with no deployed resources is reported as not deployed. This covers `REVIEW_IN_PROGRESS`, the rollback states, `CREATE_FAILED` and the delete states, and it applies to the YAML function, to `atmos.Component(...).outputs` and to `output`.
  - `output <component> <key>` is implemented.
  - JSON output no longer HTML-escapes values.
  - `get template --original` prints the stored body byte for byte.
  - A component `env` region that differs from the resolved region produces a warning.
- **Lifecycle**
  - `deployDirect` (`apply_flow.go`) creates the changeset, renders a preview on stderr, handles a no-op, and only then confirms and executes. A declined apply deletes the changeset.
  - The non-TTY gate fails before any packaging or changeset when `--auto-approve` is missing. It uses its own sentinel instead of "user aborted".
  - `discardChangeSet` removes an empty stub stack that Atmos itself created during the same invocation. It never removes a stack that existed before.
  - `ROLLBACK_COMPLETE` fails with a hint to run `delete` and then re-apply.
  - No-op changesets are deleted, and the doubled name prefix is fixed.
- **Publish-only and external delivery**
  - Publish-only apply reports the `s3://` location and the TemplateURL. External delivery reports what it delivered.
  - An `aws/stackset` target on apply fails with a hint to use the StackSet verbs, and dry-run rejects it too.
- **Packaging and backend**
  - Only `apply` and `deploy` provision the artifact bucket. The other verbs fail with a hint to run `backend create` or `apply` first.
  - An object that already exists with the same digest is reused rather than uploaded again.
  - Leading and trailing slashes are trimmed from `prefix`.
  - Dry-run now applies the static checks that the real run enforces and names the verb, component and stack. The checks cover a missing region, an unknown target, an oversized template, the `SERVICE_MANAGED` account shape and the permission model. Backend dry-runs report what they would do.
  - A real run also validates `permission_model` locally.
  - The backend non-TTY error carries an `--auto-approve` hint.
  - `backend list` errors name the right command.
  - Backend tables have headers, and backend JSON keys are snake_case.
  - The shared S3 backend delete no longer prints a double warning icon or a redundant success line.
- **Delete and idempotency**
  - A successful delete prints a success line.
  - Deleting a stack that does not exist reports "nothing to delete" consistently, with or without a TTY.
  - `DELETE_FAILED` suggests `--retain-resources` with the failed logical IDs.
  - The termination-protection and retain-resources rejections have their own sentinels.
  - `changeset delete` for an unknown name reports not found.
  - `stackset delete` for a missing StackSet succeeds as a no-op.
  - The final stack-level event is drained after a terminal status, which closes a race that dropped `CREATE_COMPLETE`.
- **Bulk**
  - JSON and YAML `output` produce one document keyed by stack, then component. Table output titles each component.
  - The `--auto-approve` guidance now survives the bulk error wrapper.
  - Bulk `fmt --check` reports every unformatted file and dedupes shared templates.
- **CLI**
  - `--skip-hooks` is registered on the hook-firing verbs.
  - Flag-combination errors read "invalid flag combination".
  - `--format` errors name the valid formats.
  - `list` resolves the default identity and labels managed stacks across all stacks. It rejects an unknown `-s` and an unknown `--status`, and prints a header row.
- **CI summaries**
  - Summaries render outputs and resource changes, say "No changes" when there are none, and title themselves with the verb that ran.
  - The reproduce command includes `--fail-on-drift` when it was set.
  - The plugin writes `GITHUB_OUTPUT` variables.
  - `drift describe` shows property differences.
- **Docs and skills**
  - The verb pages document the git and selection flags through shared partials, plus `--skip-hooks`, `logs --follow` and the `--auto-approve` flags.
  - `stacks/hooks.mdx` has a CloudFormation row.
  - The Rain migration guide and skill now describe the parameter list form and `!include file .Parameters`.
  - The delivery-target docs cover `packaging:` and an explicit deploy target.
  - The new apply, diff and delete behavior is documented.
  - The CloudFormation skill was split into references so it stays under the size limit.

## Validation

- Unit tests were added for every behavior change and assert sentinels with `errors.Is`.
  - Each workstream checked fail-before/pass-after by temporarily reverting its fix. That check was done for the parameter list form, stub cleanup, the final-event drain, changeset naming, changeset-delete lookup, the rollback guard, missing-stack and missing-StackSet delete, the `DELETE_FAILED` hint, packaging reuse, provision policy, prefix trimming, the dry-run validators and the dry-run StackSet target.
  - The apply preview ordering, decline path and non-TTY gate tests were not checked that way.
- `go build ./...` passed.
- `go vet` passed on the changed packages.
- `go test -count=1` passed on these packages:
  - `./pkg/component/aws/cloudformation/...`
  - `./cmd/aws/cloudformation/...`
  - `./pkg/ci/plugins/cloudformation/...`
  - `./pkg/aws/cloudformation/...`
  - `./pkg/provisioner/...`
  - `./pkg/ci/artifact/...`
  - `./pkg/schema/...`
  - `./pkg/datafetcher/...`
  - `./errors/...`
  - `./pkg/hooks/...`
  - `./pkg/flags/...`
  - `./pkg/list/...`
- Patch-scoped `custom-gcl run --new-from-rev=f9df6c0887` reported 0 issues on the changed packages.
- `pnpm run build` in `website/` passed.
- `go test -count=1 -v ./internal/exec/` passed: 2,270 tests passed and none failed. Go appended a spurious `[no tests to run]` to the package summary line; the verbose counts are authoritative.
- The pre-commit hooks passed on the code commit, including golangci-lint through a freshly built `custom-gcl`.
- Not run: the live re-verification on real AWS. The SSO session expired before it could start, so the fixes are validated by unit tests and the static checks above, not against live stacks. Re-run the field-test repros once credentials are refreshed.

## Follow-ups

None.
