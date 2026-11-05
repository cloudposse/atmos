# Fix: `atmos doctor` second field-test findings (dispatch, dependency repairs, write safety, check accuracy)

**Date:** 2026-10-15

## Summary

A second hands-on field test of `atmos doctor` (PR #3364) found problems in five areas. Root flags
such as `--cast` and `--pager` sent `atmos <flag> doctor` through normal configuration loading, which
fails on the malformed `atmos.yaml` that doctor exists to diagnose. `--fix` could silently change a
project's dependency graph, overwrote read-only files, and printed nothing when interrupted after
writing. The EditorConfig check reported tens of thousands of errors in gitignored files. The
`depends_on` migration never ran on projects that use `import:`. This change fixes those problems, the
smaller accuracy and wording problems listed below, and two related bugs that predate doctor.

## Context

The field test ran a fresh binary against copies of the doctor fixtures, against live GitHub for the
`component-monorepo` repair, and as a read-only `--fix --dry-run` against a large infrastructure
repository. The reproductions with lasting value are committed as fixtures under
`tests/fixtures/scenarios/doctor/` (`malformed`, `dependencies-inherited`, `dependencies-imports`,
`config-functions`, `monorepo-shapes`).

**Silent changes and data loss.**

- `--fix` migrated an abstract component's list-form `settings.depends_on` to `dependencies.components`.
  A concrete component that inherited it kept its own legacy map. The runtime prefers the inherited
  `dependencies.components`, so `app` stopped depending on `db` and started depending on `vpc`. Doctor
  reported `Fixed 1 finding` and exited 0.
- An atomic rename replaced files with mode `0444`.
- Ctrl-C after writes began left files changed with no report, no backup path, and no JSON. An earlier
  interrupt printed hundreds of `Unable to inspect Git status` warnings.

**False positives and silent passes.**

- Doctor passed EditorConfig-checker the project root, so the checker walked the directory instead of
  using `git ls-files`. On the test repository doctor reported 24,707 errors, compared with 131 from
  `atmos validate editorconfig`. Most of the extra errors were in gitignored generated and vendored
  directories.
- `!terraform.state` and `!store` in `atmos.yaml` passed the `yaml-functions` check. The runtime rejects
  them, because `atmos.yaml` supports a smaller set of functions.
- Quoted text such as `"!git.branches are protected"` was an error, although Atmos keeps it as text.
  `'!terraform.state[0]'` was called unknown, although Atmos evaluates it.
- Mixed-case monorepo URLs (`GitHub.com/CloudPosse/...`) were not detected.

**Repairs that could not run.**

- Any `import:` in any stack, or a `stacks.name_template`, made every stack check and the dependency
  repair incomplete. The reason named no file or line.
- Only the first dependency-graph error was reported, without a location, as raw `key=value` text.
- A list-form `depends_on` produced five cascading `oneOf` schema errors at ancestor paths.

**Invocation, output, and discovery.**

- `atmos --chdir=<missing> doctor`, positional arguments, and unknown flags did not write the JSON
  `operational_error` report, and the hint pointed at `atmos --help`.
- Doctor ignored `--logs-level` and `ATMOS_LOGS_LEVEL`.
- `atmos doctor --help` printed `WARN Could not load config` on a malformed configuration.
- The `Run: atmos doctor --fix` hint dropped `--only` and the other scoping flags, so it applied more
  repairs than were reviewed.
- A successful `--fix` on a dirty tree emitted one warning per file and counted them as remaining
  findings. A no-op re-run printed `Fixed 0 findings in 0 files`. The backup path wrapped mid-word.
- `ATMOS_CLI_CONFIG_PATH` pointing at a missing directory was reported as "outside the project".
- `--exclude` patterns that matched nothing were ignored silently.
- `--affected` outside Git failed with `exit status 128`.
- Malformed `atmos.yaml` below the Git root: its `base_path` could not be read, so doctor anchored at
  the Git root. It then inspected the whole repository and showed the syntax error against the Git
  root's own `atmos.yaml`.
- Template imports produced one identical notice per importing file.

**Bugs that predate doctor.**

- `atmos --pager terraform plan` folded `terraform` into the `--pager` value. The same happened for
  `--identity` and `--profile` followed by any command.
- The scheduler runs list-form `settings.depends_on`, but `describe dependents`,
  `describe affected --include-dependents`, and `list dependencies` ignored it, and the manifest schema
  rejected it.

## Changes

**Dispatch and invocation** (`cmd/doctor/dispatch.go`, `cmd/root.go`, `pkg/flags/preprocess/`)

- `InvocationIndex` treats `doctor` after an optional-value root flag as the command. A flag registered
  with no space-separated value, such as `--cast`, never consumes the next argument. The arguments before
  and after `doctor` are folded separately.
- `NoOptDefValPreprocessor` accepts a command matcher. The root wires in the registered command names and
  aliases, so a following command name is never folded into a flag value.
- `FailBeforeDispatch` reports `--chdir` failures for doctor invocations. Positional arguments and
  unknown flags go through the same invocation-error path, which writes the JSON or SARIF report and
  exits 2.

**Startup and output** (`pkg/startup/`, `cmd/doctor/render.go`, `cmd/doctor/doctor.go`)

- `SetupLogger` moved to `pkg/startup`, and doctor builds its logging configuration from flags and
  environment variables.
- Help for config-free commands skips the telemetry disclosure and the update check.
- The fix hint is rebuilt from the flags that were changed.
- `Nothing to fix.` replaces the zero summary. The backup path is printed without wrapping.
- An interrupted run renders the report with the changed files, which are summarized in the message, and
  the backup.

**Imports, stacks, and dependencies** (`internal/exec/stack_processor_utils.go`,
`internal/exec/validate_static_stacks.go`, `pkg/scheduler/adapters/terraform.go`, `pkg/doctor/`)

- The stack loader accepts an injected source reader through `ProcessYAMLConfigFileWithOptions`.
  Doctor's static stack and dependency validation loads each top-level stack through the real loader,
  reading doctor's in-memory documents. Local static imports, including globs, are followed.
- These still limit verification, and the reason names the file and line: templated import paths,
  imports with `context`, remote imports, unresolved imports, and dynamic values in the import closure.
- A pure `stacks.name_template` is evaluated instead of blocking.
- `adapters.TerraformDependencyTargets` exposes the runtime's dependency selection. `--fix` compares every
  concrete component's effective targets before and after the candidate edits. It rejects only the
  migrations that change them, with a reason such as `changes the effective dependencies of app in dev:
  db → vpc`. Other repairs in the same run proceed.
- New typed errors in `pkg/dependency/errors.go`. `ValidateTerraformGraph` collects every graph error
  while execution stays fail-fast, and doctor locates each error at its declaration.
- `pkg/validator/cascade.go` removes `anyOf`/`oneOf` cascades for doctor and the language server.
- `schema.ParseLegacyDependsOn` is the single legacy parser for the scheduler, `describe dependents`,
  `list dependencies`, and `pkg/component`. The manifest schema accepts the list form (deprecated).
- If `atmos.yaml` does not parse, doctor inspects only its own directory. Findings recorded before the
  project root moves are rebased to the new root.
- Template-import notices are reported once per imported file.

**Check accuracy** (`pkg/validation/editorconfig.go`, `pkg/doctor/yaml_functions.go`,
`pkg/function/tag/match.go`, `pkg/doctor/monorepo/`)

- `EditorConfigFiles` is the shared, Git-aware file selection for `atmos validate editorconfig` and
  doctor. Doctor also honors the `validate.editorconfig` settings.
- Configuration documents are checked against the `atmos.yaml` function set.
- `MatchesTag` is the runtime's own string-function rule, and doctor applies it to untagged strings.
  Look-alike strings that Atmos keeps as text are warnings.
- Monorepo URLs match case-insensitively.
- A subdirectory written inside the query string is explained, with the corrected source.
- A module subdirectory such as `//modules/eks/cluster/modules/foo` moves to
  `aws-eks-cluster//src/modules/foo`. The move is verified by the parent module's tree.

**Write path and discovery** (`pkg/doctor/fix.go`, `pkg/doctor/discovery*.go`, `pkg/doctor/exclude.go`,
`pkg/validation/affected.go`)

- Read-only files are reported for a manual change, at staging and again before the write.
- Git-state warnings become at most two `recovery` notices. Notices do not count as remaining findings or
  toward `--fail-on`. Cancellation stops the Git status checks.
- A missing configured path is an error finding. When it is the only candidate, doctor reports the
  no-project error and names the path.
- Each `--exclude` pattern that matches nothing produces a notice naming the project root.
- `--affected` errors use `ErrNotInGitRepository` or the new `ErrAffectedBaseUnresolved`, with hints.

**Docs.** `website/docs/cli/commands/doctor.mdx` documents the new behavior and lists the
`dependencies` and `component-dependencies` checks. `website/docs/stacks/settings/depends_on.mdx`
describes the list form, and `docs/prd/atmos-doctor.md` lists the rules.

## Validation

Commands that ran on this branch:

```shell
go build ./...
go test -count=1 ./pkg/doctor/... ./cmd/doctor/... ./pkg/flags/... ./pkg/startup/... ./pkg/validation/... \
  ./pkg/function/... ./pkg/scheduler/... ./pkg/list/... ./pkg/lsp/... ./pkg/validator/... ./pkg/dependency/... \
  ./pkg/schema/... ./pkg/template/... ./pkg/datafetcher/... ./pkg/component/... ./errors/... \
  ./cmd/config/... ./cmd/ci/... ./cmd/stack/...
go test -count=1 ./cmd/ ./internal/exec/
go test ./tests -run 'TestCLICommands/atmos_doctor' -regenerate-snapshots
go test ./tests -run 'TestCLICommands' -count=1
./custom-gcl run --new-from-rev=origin/osterman/fbzp2 ./cmd/... ./pkg/... ./internal/exec/... ./errors/...
cd website && npm run build
```

Each fix has a regression test that failed first. Examples:

- `cmd/root_doctor_dispatch_test.go` and `cmd/doctor/dispatch_test.go` (root flags before `doctor`).
- `pkg/flags/preprocess/nooptdefval_test.go` (command names).
- `pkg/doctor/dependencies_test.go` and `dependencies_verify.go` tests (graph equivalence, imports,
  every graph error).
- `pkg/doctor/fix_symlink_test.go` (read-only files).
- `pkg/doctor/malformed_root_test.go` (malformed configuration root).
- `pkg/validation/editorconfig_test.go` and `pkg/doctor/validators_test.go` (gitignored files).
- `pkg/doctor/yaml_functions_test.go` and `pkg/function/tag/match_test.go`.
- `pkg/doctor/monorepo/rewrite_test.go`.
- `cmd/doctor/invocation_test.go` (JSON for argument errors).

`tests/test-cases/doctor.yaml` runs the built binary in these cases:

- `--cast doctor` and `--pager doctor` on the `malformed` fixture.
- `doctor extra --format=json`.
- `--chdir` to a missing directory.
- The rejected inherited dependency migration.
- The dependency repair across a catalog import.

The field-test reproductions were re-run with the new binary:

- The graph-changing repair is rejected and the file is unchanged.
- A read-only file is left unchanged.
- Interrupting at 14 and 17 seconds wrote JSON listing the 42 and 142 changed files and the backup.
- 400 dirty files produce one notice.
- All three missing targets are reported with their lines.

A read-only `--fix --dry-run` on the large repository left `git status` unchanged. Doctor's
EditorConfig findings now cover the same 117 files as `atmos validate editorconfig`. The counts differ
(254 findings against 131 errors) only because the checker's summary groups consecutive lines.

Not verified: Windows runs.

## Follow-ups

- A repair rewrites the whole file through the YAML editor, which can drop blank lines and reflow folded
  scalars. This is a documented limitation and Phase 3 of `docs/prd/yaml-editing-get-set.md`.
- Imports with `context`, templated import paths, and remote imports still limit stack verification and
  the dependency repair. They are reported with their file and line. Supporting them requires evaluating
  templates, which doctor does not do. Keep this in `docs/prd/atmos-doctor.md`.
- In text output, the `--affected` error repeats its guidance as a hint. The guidance stays in the
  message so that JSON `operational_error` carries it.
