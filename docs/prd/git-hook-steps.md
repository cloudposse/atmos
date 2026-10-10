# Git hooks with inline automation steps

**Last Updated:** 2026-10-06

**Status:** Implemented in the current PR stack; this does not identify a released version.

**Related:** [Git operations](git-ops.md), [automation language](starlark-automation-and-command-testing.md),
[Atmos SDK](atmos-sdk.md), [component lifecycle hook steps](hooks-step-types.md).

## Problem

Small repository checks should be easy to find beside their Git-hook configuration.
Previously, local Atmos Git hooks accepted only a command string. A check written
in the Atmos Automation Language needed another Atmos invocation, and filesystem
checks still required shell utilities because the language only exposed file reads.

Two existing checks motivate the implementation: verify that every Git-tracked
symlink resolves, and enforce separate byte limits for CLAUDE.md and agent files.
Both fit in short inline scripts. They do not require separate source files or
another interpreter installation.

## Goals

- Run named steps directly inside `atmos git hooks run`, including embedded Starlark.
- Reuse the registered step handlers and their result, environment, timeout, and retry contracts.
- Preserve existing command hooks and the installed shim format.
- Make filesystem inspection available through an interpreter-independent Go interface.
- Keep the two repository checks inline as separate steps, with actionable errors.

## Non-goals

- Replace the repository's existing pre-commit project installation or bypass its checks.
  Its CI bootstrap binary predates these new APIs; adding configuration does not install a shim.
- Automatically stash unstaged changes, lint only staged contents, stage fixes, or install tools during commits.
- Introduce another workflow scheduler, identity lifecycle, or background-job owner.
- Provide filesystem sandboxing, writes, recursive globbing, or a native Git-index query API.
- Implement TypeScript. The shared API leaves room for another language adapter.

## Configuration

Each `git.hooks.<name>` entry selects exactly one of:

- `command`: the existing non-empty command string.
- `steps`: a non-empty ordered list using the shared task schema.

```yaml
git:
  hooks:
    pre-commit:
      steps:
        - name: check-size
          type: script
          interpreter: starlark
          script: |
            for path in fs.glob("CLAUDE.md"):
                if fs.stat(path).size > 40000:
                    errors.build("CLAUDE.md exceeds 40,000 bytes").fail()
        - name: report
          type: script
          interpreter: starlark
          script: ui.success("Checks passed")
```

The complete two-check implementation is in [atmos.yaml](../../atmos.yaml).
Git invokes the existing shim, which invokes Atmos once. Atmos dispatches each
script step through the embedded interpreter registry; it does not start a child
Atmos process to evaluate Starlark. Scripts can still deliberately run external
programs, such as Git, through `exec.run`.

## Execution contract

1. Load the configured hook; an unknown name reports `ErrGitHookNotConfigured`.
2. Reject empty or ambiguous command/step configurations before executing anything.
3. Convert tasks to the shared step representation, assign names to unnamed steps,
    and validate types, duplicate names, required fields, and unsupported policies
    before starting the sequence. Runtime templates and scripts can still fail later.
4. Run steps in declaration order. Stop on the first error and identify its step name.
5. Share named results, declared outputs, and step environment changes within the
    sequence. Invocation state and positional arguments are isolated from the host.
6. Reuse handler-specific timeout and retry behavior. The script's timeout bounds
    evaluation and subprocess work. Cancellation reaches embedded interpreter threads.
7. Resolve relative paths against the invocation directory unless the step specifies
    `working_directory`. An inline script resolves `load()` there too.

Embedded scripts receive immutable `ctx.args`, preserving spaces and flag-like
values supplied by the hook host. This also applies to file-backed included scripts.
Git's `pre-commit` hook supplies no filename arguments. The symlink check explicitly
queries `git ls-files -s -z` and parses its NUL-delimited entries.

Existing command hooks retain shell-quoted argument forwarding, inherited stdin,
and subprocess exit behavior. Step stdin and terminal access remain handler-specific;
there is no new Starlark stdin-reading function. Prompts retain their existing
TTY/default requirements, so noninteractive checks should avoid prompting.

The synchronous list rejects `needs`, `when`, `continue`, `identity`, asynchronous
background execution, freshness policies, and non-container step container overrides.
Use a workflow for those scheduler-owned features. Explicit container operations
remain available through the container handler. Background wait/cancel handlers
still require a workflow-owned job context. Script functions can use `steps.parallel`;
that does not turn the hook into a workflow scheduler.

## Filesystem API

The Go `automation.FileSystem` interface has a `LocalFileSystem` implementation.
Starlark binds it as:

| Function | Contract |
|---|---|
| `fs.glob(pattern)` | Sorted matches using filesystem glob syntax; no recursive `**`. Relative patterns return relative paths; absolute patterns return absolute paths. Returned separators are normalized to `/`. Invalid patterns fail; directory-read errors are ignored as in Go's glob implementation. |
| `fs.stat(path, follow_symlinks=True)` | Immutable metadata: byte `size`, `is_file`, `is_dir`, and `is_symlink`. Set `follow_symlinks=False` to inspect the link itself. Missing or inaccessible paths fail. |
| `fs.exists(path)` | Follow symlinks. Missing paths and dangling targets return false; other errors propagate. |
| `fs.readlink(path)` | Return the stored target without resolving it; dangling targets are allowed. Missing paths and non-links fail. |

All operations use the invocation working directory and check cancellation before
and after operating-system calls. They do not limit access to the repository or
provide an atomic filesystem snapshot. The existing `fs.read_file` stays available.
No filesystem write API is added.

## Repository checks

- **Tracked symlinks:** inspect Git index entries with mode `120000`; fail when a
  tracked path does not resolve in the working tree. Ignore untracked symlinks.
- **Instruction files:** inspect `CLAUDE.md`, `.conductor/*/CLAUDE.md`, and
  `.claude/agents/*.md`. Skip absent files and non-regular files. Accept sizes at
  the limit; reject sizes above 40,000 bytes for CLAUDE.md and 25,000 for agent files.
  Gather oversized paths before raising a structured error.

These checks inspect working-tree contents, including unstaged edits. They neither
rewrite files nor change Git's index. The hook sequence stops after the first
failed check; each check can report multiple offending files.

## Architecture

- `pkg/schema.GitHookEntry.Steps` uses `schema.Tasks` and the existing config decode hook.
- `pkg/git/hooks.Run` selects the legacy command runner or step execution.
- `step.AutomationLibrary.RunSteps` preflights and runs the sequence through the
  same validation and execution path as direct automation step calls.
- `script.Spec.Args` and cloned step variables carry positional host arguments
  independently of whether the script has a physical source file.
- `pkg/automation.FileSystem` contains no Starlark values. The language adapter
  handles argument validation, relative paths, and immutable metadata conversion.

## Validation and acceptance

- Embedded Git hooks call native step handlers without launching another Atmos interpreter.
- Positional strings, named outputs, environment changes, and cancellation survive dispatch.
- Invalid later steps prevent earlier steps from running; failures stop later steps.
- Duplicate names and command/steps conflicts fail clearly.
- Tests execute the actual inline checks from atmos.yaml against temporary fixtures.
- Symlink fixtures cover dangling, valid, missing tracked, untracked, and spaced paths.
- Size fixtures cover absent files, UTF-8 byte counts, exact limits, and agent-file limits.
- Filesystem tests cover sorted matches, metadata, symlink following, invalid input,
  missing files, cancellation, and use from parallel script functions.
- Existing command-hook tests continue to pass; schemas, docs, and examples are validated.
