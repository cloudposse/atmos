# Fix: CloudFormation templating replaces `generate`; identity, secrets and source field-test fixes

**Date:** 2026-10-02

## Summary

A field test of PR #3251 (Floci emulator plus real AWS dev/sandbox accounts) found that CloudFormation
`generate:` blocks and isolated workdirs caused a cluster of defects. Both were removed in favor of the
inline `template:` that Atmos templates already render, and the decision is recorded in
`docs/prd/aws-cloudformation-component.md` (Templating Instead of `generate`). The remaining findings —
wrong-account output references, describe/deploy identity drift, silent target-auth fallbacks, secret
selector gaps, source commands acting on the wrong directory, unhelpful S3 errors and missing
`describe affected` comparisons — were fixed in the same change.

## Context

The previous field test stopped at `be661873d9`; five later commits had never run against real state.
Findings, by area:

- **Generation and workdirs:** stale generated files deployed for fully generated components; changes only
  inside `generate:` were invisible to `--affected`; Terraform's root `generate:` leaked into every
  CloudFormation component and moved `path:` resolution into the workdir; an inline template with
  `provision.workdir.enabled: true` regressed to "workdir provisioning failed workdir provisioning failed";
  hooks saw the shared source directory; a `.atmos/metadata.json` key bricked the workdir; duplicate keys
  deployed nondeterministic content; `{{resolve:...}}` needed double escaping.
- **Identity:** `--identity` redirected `!aws.cloudformation.output`/`atmos.Component` into the caller's
  account (a same-named stack supplied wrong values); `describe stacks` ignored `--identity` while
  `describe component` and `deploy` honored it; identity-less stores fell back to ambient credentials in
  describe; a target `region` was accepted and ignored; target-auth typos ran as the component identity;
  the documented target-auth form failed schema validation; `backend list` hid healthy targets after one
  failure; two default identities silently used the SDK default chain without a TTY; target-auth errors
  named nothing.
- **Secrets:** SOPS collisions were missed for selector-based `sops:`; `!secret` could not consume a store
  chosen by `!aws.cloudformation.output`; one unresolvable selector blocked every secret command (and
  orphaned values); plain `secret list` reported a bare `error`; templated `scope:` was rejected early;
  non-TTY `secret set` failed with a raw TTY error.
- **Sources and S3:** `source pull --force`/`delete` targeted `<base>/<instance>` instead of the runtime
  directory `<base>/<metadata.component>` and deleted a hand-written component directory; dry-run
  skipped the real run's validation; absolute `metadata.component` wrote a junk tree; cross-region and
  403 S3 errors were raw SDK output; go-getter credential query parameters were silently ignored.
- **Other:** `describe affected` ignored CloudFormation `path`, `source` and `provision`; short-form
  intrinsics (`!Sub`) produced a misleading `.yaml.tmpl` hint; docs drift (blog example without
  `stack_name`, `render` "(no API calls)", stack-policy timing).

## Changes

- **Generation removed.** `generate` at component, type-level or `overrides` scope is rejected with
  `ErrAwsCloudFormationGenerateUnsupported` (`pkg/component/aws/cloudformation/manifest`); root-level
  `generate:` never reaches CloudFormation. Removed `auto_generate_files`, the CloudFormation workdir
  branch, the workdir component options and the source destination hook; S3 request-scoped source auth is
  kept (`component_files.go`). `provision.workdir.enabled` without `source:` is an error. Terraform
  workdir errors keep their cause. Schemas list the same CloudFormation fields in all three copies.
- **`stack_policy.body`.** Inline, templated policy (string or mapping), mutually exclusive with
  `stack_policy.file`.
- **Identity.** Output references use the producer target's declared identity (`DeclaredIdentityWins`);
  describe component/stacks bind the requested identity like deploy; `aws/cloudformation` targets accept
  only `kind`, `auth`, `packaging`; strict target-auth validation and contextual errors
  (`ErrProvisionTargetAuth*`); full target-auth schema; `backend list` renders every target; `backend
  create` authenticates once; superseded defaults are cleared during merges and multiple defaults fail in
  non-interactive runs; `--identity=false` warns per bypassed target; `-i` on all operation verbs.
- **Secrets.** Lazy per-declaration selector resolution (`secrets.WithSelectorEvaluator`) shared by the
  CLI and `!secret`; fail-closed SOPS collision checks; attributed per-component enumeration failures;
  `unresolved` status with a reason; templated `scope:` checked after rendering; `--force` hint without a
  TTY; warning when `--identity=false` bypasses a pinned store identity.
- **Sources and S3.** One `ResolveTarget` shared by runtime, pull and delete; delete refuses directories
  owned by sourceless components or lacking the new `.atmos/source.json` provenance marker; dry-run runs
  the real validation; invalid `metadata.component` rejected; S3 region-mismatch and forbidden errors
  with hints; unsupported credential query parameters rejected; archive link entries logged.
- **`describe affected`** compares CloudFormation `path`, `source` and `provision`.
- **Docs.** PRD decision records (templating, identity precedence); component, generate, render, apply,
  backend, source, secret and describe pages; CloudFormation and secrets agent skills; example; removed
  the generation blog post and roadmap entry; superseded note on
  `2026-10-02-cloudformation-generation-workdir-isolation.md`.

## Validation

- `go build ./...` passed on the integrated tree.
- Patch-scoped lint `custom-gcl run --new-from-rev=origin/osterman/cfn-field-test-hardening ./...`: 0 issues.
- `go test -short` across all 426 non-CLI packages: pass.
- `go test -short ./tests/` (CLI suite): five source-command tests initially failed because the new
  provenance guard ran before the established "`--force` required without a TTY" check; the check now
  runs first (the guard still applies with `--force`), and all `TestSource*` CLI tests pass. No other CLI
  test failed in the full run.
- Website production build (`docusaurus build`, frozen pnpm install): pass, no broken links or anchors.
- Package suites run by each implementation pass (cloudformation, workdir, provisioner, downloader,
  datafetcher, auth, secrets, store, deferred, cmd/aws, cmd/secret, internal/exec subsets) passed; the 14
  config snapshots were regenerated with `-regenerate-snapshots` and match #3250.
- Live, integrated binary against the field-test fixtures: `generate` rejected with location and hint;
  Terraform root `generate:` no longer affects CloudFormation and `path: ../demo/template.yaml` renders;
  inline template plus workdir errors with a hint; target `region` and target-auth typo rejected;
  `stack_policy.body` renders and passes dry-run.
- Floci emulator: deploy, output and delete of `demo` and an inline-templated component with
  `stack_policy.body`; apply hooks fired. Secrets repros (lane B) re-run on the emulator: unresolved
  status, literal secret usable beside an unresolvable selector, `!secret` through a selected store, SOPS
  collision detected.
- Not run: real-AWS re-validation of the identity fixes (SSO session expired; covered by unit tests with
  fakes); cast regeneration (stale casts listed below).

## Follow-ups

Issues are not yet opened (pending maintainer approval); each needs a number before merge:

- Regenerate `atmos-aws-cloudformation--help`, `-render--help` and `-apply--help` casts.
- `detectComponentType` (`internal/exec/describe_component.go`) treats a nested `ErrInvalidComponent` as
  "component not found", misreporting a missing producer as a missing consumer.
- `describe affected` skips selector-backed SOPS files (`Service.FileDependencies()` has no evaluator).
- `-i` is not registered on `source list/describe/delete`.
- Pre-existing, outside CloudFormation: `locals:` plus `atmos.Component` in one file recurses forever;
  YAML tags inside `!include`d files are dropped; `--query` renders account IDs as floats.
