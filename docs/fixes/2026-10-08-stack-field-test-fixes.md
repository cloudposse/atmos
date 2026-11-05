# Fix: Field-test fixes across the Automation Language and CI reporting stack

**Date:** 2026-10-08

## Summary

A field test of GitHub stack #3267 (the 16 pull requests that add the Atmos Automation Language and
the `ci` reporting module, #3263 through #3308) researched every layer against its docs and tests and
produced a findings register. The fixes land as four stacked pull requests on top of #3308: the CI
reporter and providers, the `ci` module and script inputs, the step library and Git hooks, and the
documentation. Each code fix started from a test that reproduced the finding.

## Context

The findings that mattered most:

- The fork posting gate read the pull request number from `GITHUB_REF`, which GitHub sets to the base
  branch under `pull_request_target`, so the headline same-repository case never posted. A deleted fork
  made the gate fail open, a repository that is itself a fork held every pull request, and the native
  Terraform comment and status paths skipped the gate entirely.
- Parallel and matrix script children in custom commands and workflows ran with a nil-config reporter,
  so every `ci.*` gate read as off under GitHub Actions and the warnings named flags the user had on.
- A custom-command `type: shell` step with a timeout and a backgrounded grandchild hung Atmos forever,
  because the shell runner outside the step handler used the interpreter's default exec handler and
  `os/exec` waited on the pipe the orphan held.
- A component containing `!starlark` made selective commands (`list … --columns`, `describe stacks
  --sections`, `describe component --query`) evaluate the whole component, running sibling `!exec`
  values that those commands otherwise never touched.
- `steps.<type>` accepted any misspelled field, `retry.conditions` was ignored for every type but
  `http`, step timeouts surfaced as a bare `context deadline exceeded`, Ctrl-C skipped `defer`,
  `atmos.tf` diverged from `atmos.terraform`, custom-command int flags parsed base-0, wrong-typed flag
  defaults were dropped silently, and `!literal` was ignored on half the fields its page listed.

User decisions taken during review: `ci.enabled` stays explicit (never auto-set in CI); script report
templates have no configuration default; `ci.comment` gains `target=auto|pr|commit` with commit
comments on the SHA when no pull request is known; hook `print()` keeps reaching stdout and is
documented rather than redirected; `exec.run` keeps `stream|capture` and gains `viewport`.

## Changes

Stacked pull requests, bottom to top:

1. `osterman/ci-fork-gate-commit-comments` — fork classification by head and base `full_name` with a
    deleted or unreadable payload treated as a fork; pull request number from the event payload;
    native Terraform comments and statuses through `ci.Reporter`; env, path, and SARIF held for fork
    pull requests under elevated events; `ATMOS_ALLOW_UNSAFE_FORK_EXECUTION` bound to
    `ci.allow_unsafe_fork_execution`; commit comments through a `CommitCommenter` capability with
    `ghtest` routes; gate receipts name `ci.enabled`; workflow commands on stderr; `safe.directory`
    deduplicated; nested log groups guarded; `behavior=update` requires a key; check updates send the
    id; an invalid API URL is a hard error; container image comments keyed by registry/repository,
    truncated at 65,000 characters, and posted only with `ci.comments.enabled`, with one implementation
    in `pkg/component/container/imagesummary`.
2. `osterman/starlark-ci-module-inputs-fixes` — the configured reporter reaches parallel and matrix
    children; `ci.comment(target=)`; templates require `template=` with `data=`, render with
    `missingkey=error`, and name the resolved path when missing; annotate argument validation; check
    handle id and url; Ctrl-C cancels the script and runs `defer` with a 30 s grace; `env -S` shebang
    flags, case-insensitive `.star`, a stdin hint on a TTY; `cli.flag` declaration errors for a
    default outside `choices`; `--help` ends the script; script paths resolve against the pre-`--chdir`
    directory; `atmos.tf` shares the terraform binding; `output="viewport"`; base-10 int flags;
    wrong-typed defaults stub the command; an argument without `required:` or `default:` is required;
    `!literal` on plain-string steps, command-level env, parallel and matrix env, and `timeout`;
    unsupported `!literal` fields fail at load; `!include.raw` used as written; a shared retry
    predicate.
3. `osterman/starlark-steps-hooks-cleanup` — every step handler declares its known fields;
    `retry.conditions` for every step type; shell and atmos steps and every `ShellRunnerWithWriters`
    caller run children in a process group torn down on timeout or cancel (the held-pipe hang is
    covered by a regression test); step timeouts name the step, type, and duration; `!starlark`
    selective evaluation through static `ctx` reference analysis with a full-evaluation fallback;
    `!starlark` rejected in `atmos.yaml`; Git hook arguments as `"$@"` and `ATMOS_GIT_HOOK_ARGS`,
    captured stdin in `ATMOS_GIT_HOOK_STDIN`, config sources can switch a hook between `steps:` and
    `command:`, shims pin the `atmos` path, prompts without a default fail with a hint, exit codes
    propagate; `ATMOS_HOOK_DEPTH` recursion guard; profile and identity forwarded to script steps;
    `--format` on the three getters through the standard flag parser.
4. `osterman/stack-field-test-docs` — the CI configuration pages, environment variable table, the
    `ci` module and automation function pages, step pages, Git hook pages, the `!starlark` and
    `!literal` pages, the agent skills, the native CI and Starlark PRDs, the in-stack changelog post,
    and this record.

## Validation

- `go build ./...` and `go vet ./...` on each stacked branch in a clean worktree: pass for the first
  branch alone, the first two, and all three code branches together.
- The staged lint gate (`go tool mage lint:precommit`, the pre-commit hook) on every commit: pass.
  Twenty findings surfaced on the first branch and four on the second during staging and were fixed
  without suppressions.
- Unit tests of every touched package on the combined code stack (`pkg/ci/...`,
  `pkg/component/container/...`, `pkg/script/...`, `pkg/config/...`, `pkg/customcommand/...`,
  `pkg/process/...`, `pkg/git/...`, `pkg/hooks/...`, `pkg/deferred/...`, `pkg/utils/...`,
  `pkg/automation/...`, `pkg/workflow/...`, `pkg/yaml/...`, `cmd/config/...`, `cmd/stack/...`,
  `cmd/terraform/...`): pass. The six PTY cast-session tests in `pkg/runner/step` hang in a headless
  session, as noted in the 2026-10-07 records, and were skipped.
- CLI acceptance cases: the new `starlark-ci.yaml` (12 cases), `native-ci-reporter.yaml` (3 cases),
  about 40 new standalone, step-field, and example cases, about 46 new step cases, 5 Git hook cases,
  and 8 `!starlark` cases; the combined Starlark and native-CI suites were run on the full code stack
  (see the pull request checks for the authoritative result).
- `cd website && npm run build`: pass with no broken links.
- Findings not reproduced and left unchanged: `exec.run(check=True)` already exits with the child's
  code; duplicate `cli.flag` names were already rejected; the `defer`, `digest`, `exec.which`, and
  `fs.resolve` extras behaved as documented and gained acceptance cases.

## Follow-ups

Remaining work for the stack stays in `docs/prd/native-ci/framework/script-ci-module.md` under
"Remaining implementation work" (YAML step wrappers for CI reporting; a workstation opt-in to post
for real). Not changed in this pass, by decision or scope: hook `print()` reaching stdout
(documented); `describe stacks` rejecting a computed stack identity while the Terraform generators
accept it (documented); `ci.comments.template` was declared but read by nothing and is removed from the schema;
`pkg/terminal/pty`, `pkg/auth`, and some `internal/exec` callers still spawn children outside the
process-group runner; a Starlark `ctx.stdin_path` for Git hooks (scripts read
`ATMOS_GIT_HOOK_STDIN`).
