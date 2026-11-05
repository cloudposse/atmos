# Fix: Rain migration field test — parity gaps closed in `aws/cloudformation` and `!include`

**Date:** 2026-10-08

## Summary

A hands-on field test of the Rain migration path (agent skill reference plus the user guide) against a
realistic Rain repository, a local Floci emulator, and a real AWS sandbox found two silent-corruption
bugs, two silent-data-loss bugs, and several parity gaps. This change closes them in code, corrects the
skill references and PRD, and rewrites the user guide as `website/docs/migration/rain.mdx`.

## Context

The goal was parity with Rain through Atmos conventions, not Rain compatibility: for every Rain directive,
config file, and command, there must be an Atmos-native way to accomplish the same thing, and where there
was none the gap is closed in the product rather than documented around. Findings were reproduced twice
each; the scratch fixture and evidence live under `.context/field-test-rain-20261008/` (not committed).

## Changes

- `!include` of a YAML file now splices the document as YAML nodes instead of decoding it to Go values, so
  the file's own tags survive. A new, opt-in `| eval` option (parsed by the shared option grammar in
  `pkg/function/parser`, allowed only for `!include`) evaluates Atmos YAML functions inside the included
  file; without it, included content stays data exactly as documented (tag dropped, argument kept).
- The YAML tag walker gained a foreign-tag registry (`pkg/utils/yaml_tag_foreign.go`): a subsystem can
  register in-place rewriters and hint providers for tags that are not Atmos functions. The
  `aws/cloudformation` manifest package registers all 18 CloudFormation short-form intrinsics
  (`!Ref`, `!Sub`, `!GetAtt`, ...) as rewriters to their long form, so inline `template:` maps and
  `template: !include <file>` accept templates verbatim. Previously `template: !include` silently turned
  `Role: !GetAtt Role.Arn` into the string `Role.Arn` and a nested `ZipFile: !include handler.py` into the
  literal path, and both deployed. The now-dead short-form hint in `internal/exec/stack_processor_utils.go`
  was removed.
- A `path:` template that still contains Rain directives (`!Rain::X`, `${Rain::X}`) fails locally with
  `ErrAwsCloudFormationRainDirective`, listing the directives and one hint per directive naming the
  Atmos-native replacement. A `!Rain::` tag inside an inline or included template gets the same hint from
  the walker. Before, only CloudFormation rejected it ("YAML not well-formed (line 20, column 14)").
- `tags:` must be a map. An AWS CLI `[{Key,Value}]` list or `!tags` (a string list) is now rejected with
  a hint (map form, the `ireduce` yq reshape for an existing CLI file, or `tags: !labels`) instead of
  deploying a stack with no tags. The list interface was deliberately not added: parity, not a third syntax.
- A missing packaging bucket now fails with the hinted `ErrAwsCloudFormationBackendMissing` on every
  packaging verb when `provision.backend.enabled` is off (previously a raw `NoSuchBucket` SDK error wrapped
  three deep); an S3 `NoSuchBucket` from the upload itself maps to the same error.
- `changeset create` and `changeset execute` fire new canonical hook events
  (`before/after.aws/cloudformation.changeset-create` and `changeset-execute`) and accept `--skip-hooks`,
  so the `rain deploy --no-exec` workflow can keep its archive and publish packaging hook.
- `UsePreviousValue` on a first deployment (CREATE changeset) fails locally with a hint instead of the raw
  AWS validation error.
- `delete` checks that the stack exists before the termination-protection gate, so a missing stack with
  `termination_protection: true` configured reports "nothing to delete".
- `!env` of an unset variable with no default logs a warning naming the variable (it still resolves to an
  empty string); the function doc said `null`, corrected to empty string.
- Agent skills (`atmos-migration` reference and description, `atmos-aws-cloudformation` and its
  references) and the PRD verb table corrected: `rain logs` not `rain log`, Rain had `stackset`, `rain diff`
  compares templates, `rain tree` graphs a local template, `!Rain::S3Http` exists, Rain is SDK-native,
  packaging guidance updated to the hook pattern, no `rain build` to `scaffold` mapping.
- User guide renamed from `from-rain.mdx` to `rain.mdx` (`/migration/rain`, matching the sibling guides)
  and rewritten on the tool-to-tool template with the AI-skill tip, directive and command mappings,
  before/after examples taken from what actually ran, and the YAML-functions-never-mutate-state rule for
  uploads. Sidebar, component doc, blog post, and skill links updated.

## Validation

- Live, twice each, against the Floci emulator (basic `examples/cloudformation` wiring) with the rebuilt
  binary: Rain template rejected locally with per-directive hints; `!include`d short-form template renders
  with long-form intrinsics; `| eval` evaluates nested `!env` and `!include` (tags fragment and Lambda
  source land in the template) while the default keeps content as data; tags list rejected with the hint;
  unset `!env` warns; missing bucket hint on `plan` without auto-provisioning; a packaging hook subscribed to
  `changeset-create` publishes before the changeset; `UsePreviousValue` on create fails with the hint;
  `delete` of a missing stack with protection configured says nothing to delete. Every emulator stack and
  bucket was deleted through Atmos (`list` reports none; S3 shows no fixture buckets).
- Live against the real AWS sandbox (`plat-sandbox/terraform`, us-east-2, prefixed stack names): backend
  create, plan (real 5-resource changeset table), deploy with `tags: !labels` landing on the stack, no-op
  plan, drift detect IN_SYNC, drift describe, get template `--original`, tree, logs `--chart`, a
  `UsePreviousValue` update, the termination-protection delete gate, and `backend delete --force`; the
  account inventory after the run shows zero stacks and buckets from the test.
- `go build ./...`; `go test` for `pkg/utils`, `pkg/function`, `pkg/component/aws/cloudformation/...`,
  `pkg/hooks`, `cmd/aws/cloudformation`, `pkg/ci/plugins/cloudformation`, `errors` passes; new tests cover
  the registry, tag-preserving and data-mode includes, every intrinsic rewrite, Rain detection, the
  template guard, tags rejection (unit and dry run), the bucket pre-check and `NoSuchBucket` mapping,
  changeset hook dispatch, `UsePreviousValue` on create versus update, delete ordering, and the `!env`
  warning.
- `atmos lint --changed`: no findings in code written this session. Eight findings remain in files this
  session did not touch (`backend_create.go`, `internal/exec/yaml_func_aws.go`, `observability.go`,
  `validate.go`, `executor.go` `runApply`, `dryrun_static_test.go` `s3TargetBlock`, `pkg/generator`),
  all present on the stack base.
- `cd website && npm run build`, `examples/cloudformation-advanced` `atmos test` against Floci, and the
  repository's short `atmos test` were run after the code settled; results are reported in the PR.

## Follow-ups

None tracked yet. Three minor findings are pre-existing and await a per-item decision (fix now or open an
issue): `changeset delete` leaves the `REVIEW_IN_PROGRESS` stub stack that `changeset create` made for a
stack that did not exist yet; `--dry-run` never reads a `path:` template body, so it reports "validated" for
a template full of Rain directives; `fmt` removes every blank line between top-level sections, so any
Rain-formatted template shows as unformatted once. The eight pre-existing lint findings above are in the
same category.
