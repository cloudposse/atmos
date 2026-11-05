# Fix: `atmos doctor` field-test findings (crashes, false positives, silent passes, repair applicability, DX)

**Date:** 2026-10-12

## Summary

A hands-on field test of `atmos doctor` (PR #3364) found 20 problems in five groups: crashes on
valid-but-unusual projects, false positives that contradicted `atmos validate stacks`, silent passes that
hid real problems, repairs that were refused for the wrong reason, and rough output and exit-code
behavior. This PR addresses each of them. Two root causes sat outside doctor, in code that already shipped on
`main`: `atmos` runtime ignored staged changes when computing affected files, and the legacy
`init_run_reconfigure` environment variable and flag no longer overrode an explicit `init.reconfigure`.
Both are fixed here because doctor's repair writes `init.reconfigure` and exposed them. Re-checking the
demo also exposed one more bug in verification, fixed in the same pass.

## Context

Findings came from field-testing the built binary against a real infrastructure repository and
small reproduction projects; the reproductions that close coverage gaps are committed as fixtures
under `tests/fixtures/scenarios/doctor/`. They fall into these groups.

**Crashes.**

- A project with only `.editorconfig` (no `.editorconfig-checker.json` or `.ecrc`) exited 2 with
  `no such file`, because `editorConfigPaths` always passed both default checker paths. The failure also
  aborted every later check, so `source-repository` vanished from the JSON `checks`.
- `init_run_reconfigure: "false"` (quoted) exited 2 with a raw `yaml: unmarshal errors` message, plus three
  `anyOf` schema findings that had no path or line. The runtime accepts the quoted value.

**False positives.**

- `./_defaults` (relative to the importing file) and `catalog/vpc` (resolving to `catalog/vpc.yml`) were
  reported as unresolved imports. Doctor had its own resolver, and `resolveAuthImportPaths` in
  `pkg/config` was a third, so there were three disagreeing import resolvers.
- Imported `*.yaml.tmpl` files were parsed as plain YAML and produced parse errors.
- `skipDirectory` skipped `build`, `dist`, and similar names anywhere, including inside the stacks base
  path, so `stacks/orgs/acme/build/broken.yaml` was silently ignored.
- `--only=sources,modernization` and `--affected` still exited 1 because of an unrelated discovery-time
  schema or parse error.

**Silent passes.**

- Config discovery ignored parent directories, `ATMOS_CLI_CONFIG_PATH`, and the active profile, merged
  inactive profiles, and resolved `atmos.d` against the current directory. Running from a subdirectory
  reported a warning and exited 0.
- `component.yaml` `spec.source.uri` was never matched by the source or modernization rules, so an archived
  upstream was reported as passed. A check with zero inputs also reported `passed` instead of `skipped`.
- `pkg/validation` `AffectedFiles` missed staged-only changes, and treated `git ls-files --others` paths as
  repository-root relative although they are working-directory relative.

**Repair applicability and wording.**

- Any comment on a migrated key made verification fail with `comments changed`, including the PR's own
  `eligible` fixture. A document that ended with two comment blocks separated by a blank line failed the
  comment check even when the edit was elsewhere.
- Any `{{` anywhere in the project (custom command `steps`, or `stacks.name_template`, the recommended
  replacement for `name_pattern`) marked the whole project dynamic and blocked every repair.
- Remediation text told users to run `atmos doctor --fix` for findings that `--fix` cannot resolve, claimed
  "the replacement is already declared" when only `init.pass_vars` existed, said "Use provides" for an
  argument `type: string`, and gave two identical findings for two hook events on one line.
- `--fix` exited 0 when a requested repair ended `verification-failed`.

**Output and invocation.**

- Invalid `--format` and `--fail-on` values gave `invalid doctor options` without naming the option or the
  valid values, and several invocation errors wrote nothing to stdout in JSON mode.
- Findings printed twice in text and rich output, the summary said `Fixed 0 findings in 0 files` without
  `--fix`, per-file rows said `(passed)` for files with findings, and the imports notice printed `:0`.
- actionlint did not honor `.github/actionlint.yaml`, and unknown-label findings dumped a 1 KB label list.
- `--list-checks` wrote to stderr and accepted conflicting flags.
- `--offline` notices were warnings, so `--offline --fail-on=warning` always exited 1.
- The backup manifest `mode` was decimal (`420`) instead of an octal string.
- `atmos --skip-hooks doctor` (any root flag missing from a hand-written allowlist) fell into normal
  configuration loading and crashed on a malformed `atmos.yaml`.

**Root causes outside doctor.**

- The legacy `ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE` variable and `--init-run-reconfigure` flag set
  only the legacy boolean. Once `init.reconfigure` is present, `EffectiveInitReconfigure` prefers it, so the
  override silently lost to configuration. Doctor writes `init.reconfigure`, so a user who kept the legacy
  override would have seen it stop working after `--fix`.

**Found while re-recording the demo.**

- When `atmos.yaml` sits below the Git root and declares no `base_path`, Atmos anchors the project at the
  Git root, so doctor moves its project root there. The later verification and recheck passes reused the
  moved root as the starting directory, so they failed with `no project Atmos configuration found` (or
  `source closure changed` with a relative `--config`). Every repair was rejected in that layout.

## Changes

**Configuration discovery (`pkg/config`).** New `ResolveConfigCandidates` in `pkg/config/candidates.go`
and `candidates_paths.go` returns the ordered configuration files without parsing them, sharing path
computation with `LoadConfig`. It honors `--config`, `--config-path`, `ATMOS_CLI_CONFIG_PATH`, parent and
Git-root discovery, `atmos.d` relative to each config file, and only the active profiles (including
`profiles.default`). The loader (`load.go`) and `stack_auth_loader.go` now call the same helpers. Doctor
uses it (`pkg/doctor/discovery_config.go`), and finding no project configuration is an operational failure
that exits 2.

**Shared import resolution (`pkg/stack/imports`).** `local.go` holds one local-import resolver used by
doctor and the auth import loader: relative-to-importer first, then the extension order the stack
processor uses. Template imports are reported as not evaluated and never parsed as YAML
(`discovery_imports.go`).

**Discovery (`pkg/doctor`).** Only directories outside the stacks base path are skipped by name. Discovery
findings respect `--only` and `--affected`. `component.yaml` `spec.source.uri` is matched by the source and
modernization rules, and a check with no inputs reports `skipped`.

**Crash fixes.** `editorConfigPaths` returns only existing checker files. A failing check is recorded on
its own `Check`, the remaining checks still run, and the run exits 2 at the end. Quoted legacy booleans
are accepted, and every schema finding carries a path and line.

**Relevance-scoped repair blocking (`limits.go`, `locate.go`, `edit_apply.go`).** A repair is blocked only
by inputs that can change what it means: imports, `atmos.d` files, active profiles, and ambient
configuration for configuration repairs, and dynamic values at or under the edited key. Templates
elsewhere in a file no longer block anything. Every blocked finding names the blocking `file:line` or
layer.

**Comments (`verify_comments.go`, `edit_apply.go`).** The legacy key's head, inline, and foot comments move
to the replacement key. Verification requires comments on untouched nodes to be identical and compares
trailing document comments as a text multiset instead of by attachment.

**Remediation text (`remediation.go`).** Findings that `--fix` cannot resolve say what blocked them and
what to change by hand, never "Run atmos doctor --fix". A conflict is reported only when `init.reconfigure`
exists. Hook-event messages name the event, and the argument `type: string` message says to remove it.

**Command (`cmd/doctor`).** `options.go` validates options before reading any project file, names the
option, and lists valid values. `failInvocation` writes the common report shape with `operational_error`
in JSON and SARIF. `render.go` prints each finding once, uses `N findings (M fixable)` without `--fix`, and
sends `--list-checks` to stdout. `dispatch.go` derives root-flag arity from `RootCmd.PersistentFlags()`,
and `cmd/root.go` calls it, so `atmos --skip-hooks doctor` no longer enters normal config loading.
`--fix` exits 1 when a requested repair fails verification.

**Reporting details.** Offline repository notices have notice severity and never count toward a
threshold. The manifest `mode` is an octal string. actionlint receives `.github/actionlint.yaml`
(`pkg/ci/validate`), and its unknown-label remediation points to that file.

**Rediscovery (`types.go`, `engine.go`, `fix.go`).** `Engine.run` keeps a copy of the options as the caller
gave them, and the unchanged-source check, the project verification, and the post-write recheck rediscover
from that copy instead of from the project root that discovery moved.

**Fixes to code that already shipped on `main`.**

- `pkg/validation/affected.go` also lists `git diff --cached` and runs `git ls-files --others` with
  `--full-name -- :/`, so staged-only changes are included and untracked paths are repository-root
  relative from any subdirectory.
- `pkg/schema/schema.go` adds `OverrideInitRunReconfigure`, which records the legacy boolean and also sets
  the effective `init.reconfigure`. The environment variable and flag use it, and the newer
  `init.reconfigure` variable and flag are applied after it, so CLI over ENV over config holds and the
  newer setting wins at the same level.

**Docs, fixtures, tests, and demo.** `website/docs/cli/commands/doctor.mdx`, `docs/prd/atmos-doctor.md`, the
`atmos-modernization` skill, and the blog post describe the behavior above, including the known limitation
that a repair rewrites the file through the YAML editor and can reformat blank lines and folded scalars.
Fixture headers under `tests/fixtures/scenarios/doctor/` describe their own scenario, and the `layered`
fixture now imports a configuration file. The demo fixture declares `base_path: "./"` and
`website/static/casts/demo/fixtures/doctor/doctor.cast` was re-recorded.

## Validation

Commands that ran in this worktree:

```shell
go build ./...
go test -short ./pkg/doctor/... ./cmd/doctor/... ./pkg/config/... ./pkg/stack/imports/... ./pkg/validation/... ./pkg/schema/...
go test ./tests -run 'TestCLICommands/atmos_doctor'
./custom-gcl run --new-from-rev=origin/main ./pkg/doctor/...
cd website && npm run build
atmos --chdir=demo/casts casts generate demo fixtures doctor doctor
```

New and changed tests:

- `pkg/doctor/fixture_scenarios_test.go` pins the outcome of each `tests/fixtures/scenarios/doctor/` scenario
  offline: `eligible` (all repairs available, applied, and idempotent), `layered` (configuration repairs
  blocked with a reason that names `atmos.yaml:5`, the stack file still repaired), `templated-step` (repair
  survives a templated step), `sources` (offline findings are notices that do not trip `--fail-on=warning`),
  and `edge` (multi-document workflow reported and untouched, template-looking text preserved, empty stack
  file untouched).
- `pkg/doctor/gitroot_base_path_test.go` reproduced the rediscovery bug first (four subtests failed before
  the fix) and covers discovered and `--config` selection, with and without `--dry-run`.
- `pkg/doctor/fieldtest_discovery_test.go`, `fieldtest_inputs_test.go`, `relevance_test.go`,
  `remediation_test.go`, `verify_comments_test.go`, and `fixture_comments_test.go` cover the discovery,
  crash, relevance, wording, and comment findings. `cmd/doctor/dispatch_test.go`, `invocation_test.go`,
  and `cmd/root_doctor_dispatch_test.go` cover the invocation and dispatch findings.
- `pkg/config/candidates_test.go`, `candidates_workdir_test.go`, and `init_reconfigure_precedence_test.go`,
  `pkg/schema/terraform_init_test.go`, `pkg/stack/imports/local_test.go`, and
  `pkg/validation/affected_git_test.go` cover the code shared with the rest of Atmos.
- `tests/test-cases/doctor.yaml` runs the real binary: `--list-checks` on stdout and as JSON, a rejected
  `--list-checks --dry-run`, `operational_error` JSON for `--fail-on=bogus`, and `--offline` on the
  `eligible` fixture with and without `--fail-on=warning`. Snapshots were generated with
  `-regenerate-snapshots`. The project runs select the fixture with `--config=atmos.yaml` so the
  repository's own `.atmos.d` and the developer's home configuration cannot change the report.

Not verified: Windows runs, and snapshots on any machine other than the author's.

## Follow-ups

- `atmos doctor --fix` re-indents repaired files and can drop blank lines or reflow folded scalars,
  because the YAML editor re-emits the document. This is a known limitation, documented in the command
  reference, and planned as Phase 3 of `docs/prd/yaml-editing-get-set.md`. No GitHub issue is opened yet;
  open one and link it here by number before merging if the maintainers want it tracked outside the PRD.
- An empty stack file is reported as a schema error (`Invalid type. Expected: object, given: null`), while
  `atmos validate stacks` accepts it. The `edge` fixture test deliberately does not pin this behavior.
- A multi-document workflow file is reported with the internal-sounding message
  `doctor semantic verification failed: multiple YAML documents are not supported`, while the
  `github-actions` check reports the same file as passed.
- A `--fix --dry-run` summary read `Fixed 0 findings in 0 files. N findings remain.`, which sounded like a
  failed fix. Resolved in review: a dry run now reports `N repairs verified.`, and every count uses the
  singular or plural form.
