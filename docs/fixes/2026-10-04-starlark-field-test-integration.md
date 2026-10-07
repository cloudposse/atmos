# Fix: Starlark integration defects found by final-build field testing

**Date:** 2026-10-04

## Summary

Fresh-binary field tests exposed incorrect subprocess environment precedence, lost hook execution context, lost workflow script provenance, and missing registration of configured output masks. Fix these integration boundaries and repair the native Helm lifecycle fixture and script reference examples.

## Context

The final Starlark runtime has several callers: standalone files, workflow manifests, custom commands, and component hooks. Unit mocks did not expose duplicate inherited environment entries, YAML composition, or the CLI's distinct initialization and resolved configuration objects. Real subprocesses, composed manifests, PTYs, and an isolated Kubernetes emulator reproduced the defects.

The following exact commands ran in a disposable copy of `tests/fixtures/scenarios/starlark-steps`; the added regression tests preserve their assertions in the repository. `atmos-final` refers to the fresh workspace build; the final Helm run uses the identical build under the canonical regular filename `atmos`.

| Severity | Field command or committed regression | Expected | Observed before fix |
| --- | --- | --- | --- |
| High | `atmos-final workflow streaming-log -f field-final`; `TestReconcileMaskingRegistersResolvedConfig` | Configured literals/patterns masked | Split and multiline configured literals printed verbatim |
| Medium | `atmos-final terraform plan mock -s field-path` / `-s field-env`; `TestHookControl` regressions | Nested hook cwd/env match declared context | Missing component-relative file and missing subprocess env |
| Medium | `atmos-final workflow merged-step -f field-final` / `included-step` / `included-steps`; `TestManifest` composition regressions | Included scripts import beside their source | Import failure or include text evaluated as program |
| Medium | `atmos-final field-component-exec api -s dev`; `TestProcessDirectoryAndEnvironmentIsolation` | Per-call env wins in direct/parallel subprocesses | Later inherited duplicate overwrote the per-call value |
| Medium | `atmos-final --chdir=tests/fixtures/scenarios/helm-lifecycle test`; `TestHelmLifecycle` regressions | Release-record assertion reaches Kubernetes | `kubectl --all-namespaces get` rejected the flag |

## Changes

- Apply default component directories recursively to hook control children, retaining Atmos-command directory behavior. Merge inherited and child process environments at the registry bridge.
- Expand local YAML includes while retaining tagged nodes and their declaring-file scopes. Decode aliases and merge keys before recording script provenance, including whole-step and whole-list includes.
- Remove every inherited occurrence of an environment key before applying a Starlark per-call override. Parallel subprocess calls remain isolated.
- Register masking literals, patterns, and replacement text from the resolved invocation configuration while preserving the effective `--mask` preference.
- Update the existing secrets-masking CLI cases and regenerate their snapshots:
  configured literals and patterns must be redacted in `describe stacks` and
  `describe config`. Add explicit `--mask=false` cases that retain the original
  values. CI exposed these older expectations for unmasked output after the fix.
- Put the Kubernetes `--all-namespaces` option after `get secrets`.
- Split mutually exclusive `output` examples into separate Starlark blocks and document project-relative traceback display.

- CI fixture correction (2026-10-06): decode script-provenance test inputs as full
  `components.terraform.mock.hooks.check` manifests with the appropriate hook
  `kind` and `type`. The former bare `with` fragments discarded the type before
  the loader ran, even though the test supplied it later to the hook engine.
  Retain every source-path, load-resolution, templating, and inheritance assertion.
- Add a negative check that hook-shaped `with` data under `vars`, `settings`,
  `env`, and `metadata` keeps exactly its declared fields. The production loader
  and its plain-data protection are unchanged.

## Validation

- Reproduced hook environment/directory, composed include, and per-call environment failures twice before fixing them. Real CLI repetitions pass after the changes, including component subprocesses in direct and parallel custom-command steps.
- Fresh binary: standalone shebang and symlink invocation preserve script arguments, physical import paths, and caller cwd; `atmos.version()` uses the live command catalog.
- Fresh binary: sequential, parallel, matrix, and test workflows perform no marker writes under dry-run and create their expected markers when executed.
- Real PTY log/raw/viewport/none output masks split and multiline configured literals while captured process results retain their values. Log/raw output arrives incrementally and retains a trailing partial line. Forced-color output contains ANSI escapes.
- Custom-component release-plan and cast validation pass. The intentionally failing test report counts seven leaves correctly; included-script diagnostics report the source path.
- `go test -race -count=1 ./pkg/workflow ./pkg/hooks ./pkg/yaml/includescope` passes. Focused hook, subprocess, masking, and Helm fixture regression tests pass. Changed-package lint passes.
- Website `npm run build` succeeds.
- `go test -count=1 -timeout=10m ./tests -run 'TestCLICommands/secrets-masking_describe'`
  passes after regenerating the two snapshots through the CLI test harness.
  The test-case schema validation also passes.
- The full native Helm lifecycle fixture passes against an isolated local k3s emulator in 277.8 seconds, using one regular fresh binary named `atmos` for the outer command, nested shell commands, and `ATMOS_CLI_PATH`. Verified local/public charts, hook ordering and cleanup, CRDs, job/readiness modes, timeout cleanup, failed-install cleanup, failed-upgrade rollback, dependency ordering, deployed secrets, console/GitHub-summary masking, ingress upgrade, delete dry-run, deletion, and emulator teardown. The default Helm apply policy is unchanged.

- Reproduced the missing-provenance and empty-source-path failures locally before
  correcting the fixture context. The existing hook provenance tests then passed
  (2.051 seconds), as did the focused utils provenance and tag-walker tests
  (0.991 seconds). The new ordinary-data regression passed (1.434 seconds).
- `GOMAXPROCS=4 go test -race -shuffle=on -p 2 ./pkg/hooks ./pkg/utils -count=1`
  passed for both complete packages: hooks in 26.893 seconds and utils in
  8.925 seconds.

## Follow-ups

None.
