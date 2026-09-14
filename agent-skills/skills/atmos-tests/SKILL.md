---
name: atmos-tests
description: "Author Atmos smoke tests, post-deployment stack checks, and integration tests using type: test, HTTP assertions, shell or script/interpreter checks, require gates, parallel dependencies, matrix cases, and lifecycle hooks. Use for test suites in Atmos configuration, not Go unit tests of Atmos itself."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: ci-automation
---

# Atmos Tests

Use `type: test` to run smoke tests that validate deployed stacks or other
integration tests. It groups existing typed steps into a test report: passing
logs stay hidden, failures reveal their buffered output, and independent checks
continue by default.

Read [patterns](references/patterns.md) for complete custom-command, HTTP,
parallel/dependency, matrix, script/interpreter, and post-apply hook examples.
Use [atmos-steps](../atmos-steps/SKILL.md) for shared step fields and the relevant
surface skill: [custom commands](../atmos-custom-commands/SKILL.md),
[workflows](../atmos-workflows/SKILL.md), or [hooks](../atmos-hooks/SKILL.md).

## Authoring Process

1. Inspect the project's commands, workflows, hooks, and existing test scripts.
    Discover actual stack/component names and deployment outputs before choosing
    targets. Do not invent endpoints or hard-code credentials.
2. Prefer a custom command for a directly invoked suite with its own arguments
    or flags. Use a workflow for an existing orchestration sequence, or a lifecycle
    hook to run checks after deployment. There is no built-in standalone `atmos test`
    command: defining a custom command named `test` creates that invocation.
3. Choose the smallest step type that expresses each assertion. A command that
    merely prints a response is not an assertion; it must fail when expectations
    are unmet. Name cases descriptively and use `title` for readable display labels.
4. Keep direct children sequential. Use a sibling `parallel` or `matrix` group
    when cases should run concurrently. Set a concurrency limit suitable for the
    service and isolate mutable fixtures for each concurrent case.
5. Keep default failure-only output and continuation unless the test contract
    requires otherwise. Add bounded retries for eventual consistency; do not use
    retries to conceal a reproducible assertion failure.
6. Verify a passing case and an intentionally failing case against local fixtures
    or an appropriate test environment. Check the exit status, failure details,
    continuation, and expanded leaf totals. Restore the intended expectations.

## Choose a Test Step

| Need | Step and pattern |
|---|---|
| HTTP status, health endpoint, or response text | `http` with `expect.status` and optionally `expect.response`; prefer this over `curl` and shell parsing. |
| JSON structure, numerical comparisons, or multi-part assertions | `script` with explicit `interpreter` and `script`; consume an HTTP step's response through its result value or use a language client. |
| Existing test script or external CLI | `shell` with `command`; ensure assertion failures propagate as a nonzero exit. Prefer checked-in scripts for substantial logic. |
| Required tools, files, or directories | `require` with `tools`, `files`, or `dirs`; useful as a precondition, not proof that a service is healthy. |
| Atmos validation or inspection with meaningful exit status | `atmos` with a command such as `validate stacks`; inspect-only commands need a separate assertion on their output. |
| An isolated test runner image | Foreground `container` with a finite test command; consult its step documentation for image/runtime fields. Do not detach it or request a TTY. |
| Independent named checks | `parallel` with `max_concurrency`; use child `needs` for prerequisites and result consumption. |
| The same checks across regions, endpoints, runtimes, or other axes | `matrix` with named axis lists and `max_concurrency`; use `{{ .matrix.<axis> }}` in child fields. |

`script` requires both `interpreter` and `script`; do not put `command` on it.
Python, Node.js, and other interpreters are patterns, not new step types. Declare
needed runtimes using the owning command/workflow's `dependencies.tools` and the
project's toolchain conventions. `require` verifies availability; it never installs.

## Execution and Failure Rules

- Direct children run in declaration order. `test` does not accept
  `max_concurrency`; put it on `parallel` or `matrix`.
- `needs` is supported on children of `parallel`/`matrix`, not direct children of
  `test`. Use it whenever a check consumes another concurrent check's result.
  A failed dependency skips its dependent checks.
- Parallel and matrix groups can be siblings under `test`, but cannot contain
  another parallel/matrix group. Nested `test` groups are unsupported.
- `fail.mode: wait_all` is the default: independent tests continue, then unhandled
  failures fail the group. Unset `when` permits continuation after a failure;
  explicit `when: success` would instead gate a check on success.
- `fail.mode: fail_fast` cancels remaining work; `fail.max_failures` configures the
  existing failure threshold. `best_effort` records failures but lets the group
  succeed. Use these deliberately, especially for deployment gates.
- Preserve explicit child `continue` and nested group failure policies. For
  example, `continue: always` tolerates a leaf failure, but that case still appears
  red in the report. Tolerated failures are not passing assertions.
- `retry` retries a leaf as one case; the final attempt determines its outcome.
  Use explicit expectations and bounded attempts for readiness checks.
- Use supported noninteractive registered handlers. Interactive prompts, terminal
  handoff, process replacement (`exec`, `exit`), background/async work, recording
  or emulator sessions, and `wait`/`wait-all`/`cancel` steps cannot run inside a test.
  Start services and prepare fixtures outside the group.

## Output and Results

Set `output: failures` (the default) or `output: all` on the test group. These are
scalar test-specific choices, not `raw`, `none`, `log`, or `viewport`. Both buffer
per leaf; `all` also reveals successful logs at completion. Do not redirect leaf
output to `/dev/null`: it removes the evidence needed when a test fails.

The terminal report uses a hierarchical tree and progress bar. CI/non-TTY output
is static. Counts cover expanded leaves once, excluding group nodes; retries do
not add cases. The summary includes passed, failed, skipped, canceled, and elapsed
time. Group result metadata exposes `total`, `passed`, `failed`, `skipped`, and
`canceled`.

Use scoped environment variables and step result values rather than shared files
or process-global state to pass data. Matrix cases must not overwrite the same
fixture paths. Keep secrets in existing Atmos auth/secret mechanisms; masking
still applies to captured output, but do not deliberately print credentials.

Place optional success messages after the test group with `when: success` and
content such as `Tests passed`. Message-only steps inside the group would count
as cases without validating anything.

## Canonical References

Consult the project docs for exact fields before extending a pattern:

- `website/docs/workflows/workflows/workflow/steps/type/test.mdx`
- The neighboring `http.mdx`, `script.mdx`, `shell.mdx`, `require.mdx`,
  `atmos.mdx`, `container.mdx`, `parallel.mdx`, and `matrix.mdx` files.
- `website/docs/workflows/workflows/workflow/steps/continue.mdx` and `retry.mdx`.
- `examples/tests/atmos.yaml` for runnable local examples.

If behavior is unclear, inspect `pkg/schema/test_step.go` and the registered
handlers under `pkg/runner/step/` rather than inventing test-only assertion fields.
