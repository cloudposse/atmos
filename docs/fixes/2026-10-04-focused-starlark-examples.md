# Fix: Focused Starlark examples and recordings

**Date:** 2026-10-04

## Summary

Add separate runnable examples for an executable Atmos shebang, a custom command,
and a lifecycle hook. Each has a real terminal recording embedded in the Atmos
Automation Language changelog post, introducing the Python-like language built
on Starlark.

## Context

The original release-plan recording combined component lookup, local imports, and
parallel execution. It did not give users a small starting point for each execution
style. This was a documentation and discoverability gap, not a runtime defect.

## Changes

- `examples/starlark-script` reads a JSON manifest with an executable `.star` file.
- `examples/starlark-commands` calculates capacity from a named flag, passed through
  the environment rather than interpolated into script source.
- `examples/starlark-hooks` checks resolved ownership before a real Terraform plan.
  Its provider-free module needs no cloud credentials. A second stack clears the
  owner and demonstrates the hook blocking execution.
- A repeatable Atmos cast command copies the actual examples into disposable
  workdirs and records real execution. Its Starlark validator reconstructs event
  text, checks results and ordering, and rejects local paths and a secret sentinel.
- CLI acceptance cases cover success, defaults, invalid inputs, rejected hooks,
  and all three recording validators. Website example metadata and the changelog
  link each recording to its corresponding example.
- The parallel deployment reference resolves the current stack before starting
  timed tasks, then passes it explicitly. Lazy invocation-context resolution no
  longer runs inside the example's task timeout.
- After syncing the stack with the first-class Steps reference on `main`, migrate
  changelog, include reference, example README, and example navigation links from
  `/workflows/steps` to `/steps`. Keep legacy redirect configuration unchanged.

## Validation

Run from the repository root unless a directory is specified:

- `go build -o .context/bin/atmos .` passed.
- From `examples/starlark-script`, with the rebuilt Atmos on `PATH`,
  `./summarize.star services.json` returned two services and five replicas.
- `atmos --chdir=examples/starlark-commands capacity --replicas 3` returned twelve
  workers. The default returns eight; zero replicas fails.
- `atmos --chdir=examples/starlark-hooks terraform plan api -s dev` ran the owner
  check before Terraform reported no changes. The same command with `-s unowned`
  exited 1 with `Set an owner before planning api`, without running a plan.
- `PATH="$PWD/.context/bin:$PATH" ATMOS_VERSION_CHECK_ENABLED=false ATMOS_EXPERIMENTAL=silence .context/bin/atmos --chdir=demo/casts casts generate examples starlark`
  recorded all three casts and passed their Starlark validator.
- `atmos --chdir=demo/casts casts validate examples starlark` passed independently.
  A disposable copy with `SECRET_` and `SENTINEL` in separate output events was
  rejected, proving validation reconstructs text before checking secrets.
- All three casts were rendered with `atmos cast render <recording> --output=<png>`
  and visually reviewed. Commands, results, and the expected hook error fit without
  clipping. Preview images remain in `.context/`; only casts are committed.
- `go test ./tests -run 'TestCLICommands/starlark_example|TestTestCaseSchemaValidation' -count=1 -timeout=15m`
  passed all eight acceptance cases and schema validation.
- `terraform fmt -check examples/starlark-hooks/components/terraform/service` and
  `git diff --check` passed.
- Executed the documented `deploy-peers` command in a disposable copy of the
  release-plan fixture with a local stub deployment executable. Both `api` and
  `worker` completed successfully using the explicitly passed stack.
- Verified all migrated `/steps` documentation paths and section anchors
  exist; example navigation JavaScript syntax and whitespace checks passed.

## Follow-ups

None.
