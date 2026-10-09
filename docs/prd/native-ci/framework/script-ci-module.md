# PRD: `ci` module for Atmos Automation Language scripts

> Related: [Overview](../overview.md) | [Interfaces](./interfaces.md) | [CI Detection](./ci-detection.md) | [Fork-PR Trust Gate](./fork-pr-trust-gate.md) | [CI Log Groups](./ci-log-groups.md) | [Generic Provider](../providers/generic.md) | [GitHub Provider](../providers/github/provider.md)

## Status

Implemented. Scripts written in the Atmos Automation Language (Starlark) get a predeclared `ci` module. Remaining work is listed under [Remaining implementation work](#remaining-implementation-work).

## Problem

Teams moving build, test, and deploy pipelines out of CI YAML and shell glue into Atmos custom commands, workflows, and hooks can run the work itself with one command, but they could not report back to the pull request from the same code:

- Posting a pull request comment meant shelling out to `gh` with a token the script had to arrange for itself.
- Setting a step output, exporting an environment variable, or adding a `PATH` entry for later steps meant echoing into `$GITHUB_OUTPUT`, `$GITHUB_ENV`, and `$GITHUB_PATH` by hand.
- Annotations, log groups, masking, commit statuses, and SARIF uploads each had their own provider-specific incantation.
- None of it worked on a laptop. The reporting half of a pipeline could only be debugged by pushing a commit and waiting for a runner.

Atmos already reports natively for Terraform commands (summaries, outputs, comments, checks, groups), gated by the `ci.*` configuration. Script authors had no way to reach that same machinery from their own code.

## Goals

1. **One call per report.** From any script, a single call posts a comment, writes a summary, sets an output, exports an environment variable or `PATH` entry, masks a value, emits an annotation, creates and updates a commit status, opens a log group, or uploads a SARIF report.
2. **Local equals CI.** The same script runs unchanged on a laptop. With no CI provider detected, every call renders locally on the terminal.
3. **Same switches as native reporting.** In CI, each call honors the same `ci.*` configuration as Atmos's built-in reporting. Nothing posts when the feature is not enabled.
4. **Safe by default.** The fork gate, masking, and opt-in comments and checks apply to script calls exactly as they do to built-in reporting.
5. **Provider-neutral.** Scripts name no provider. GitHub Actions is the first real provider; the generic provider covers unknown CI and local runs.

## Non-goals

- Replacing the `atmos ci` command group or the Terraform lifecycle reporting. Those keep working unchanged.
- Posting for real from a laptop (see follow-ups).
- A provider-specific escape hatch. A script that needs something the module cannot express calls the provider's own tooling.
- New templates for summaries and comments. Script authors build their own Markdown.

## User-facing API

The `ci` module is predeclared in every script: standalone scripts and the script steps of custom commands, workflows, and hooks.

| Call | Purpose | In CI | Locally |
|------|---------|-------|---------|
| `ci.context` | Read provider, event, SHA, branch, repository, actor, run ID, run URL, whether the event is elevated, and the pull request (`number`, `head`, `base`, `url`, or `None`). Read-only; resolves on first read. | Values from the provider | Values from `ATMOS_CI_*` variables and the local git repository; `local` is true |
| `ci.base` | Read the base that affected detection compares against. | Resolved base | Resolved base |
| `ci.summary(markdown)` | Append Markdown to the job summary. | Written to the provider's summary | Rendered as terminal Markdown on stderr |
| `ci.output(name, value)` | Set a step output for later steps and jobs. | Written to the provider's output file | Printed as `name=value` on stderr |
| `ci.env(name, value)` | Export an environment variable to later steps. | Written to the provider's environment file | Printed as `export NAME='value'` |
| `ci.path(dir)` | Prepend a directory to `PATH` for later steps. | Written to the provider's path file | Printed as `export PATH='dir':"$PATH"` |
| `ci.mask(value)` | Redact a value from everything printed afterwards, including runner logs. | Registered for redaction in runner logs and Atmos output | Registered for redaction in Atmos output |
| `ci.annotate(level, message, file=, line=, title=)` | Attach an error, warning, or notice to a file and line. | Inline annotation in the pull request diff | Printed as `file:line: level: message` |
| `ci.comment(body, key=)` | Create or update a pull request comment. The `key` makes later calls update the same comment. | Posted to the pull request; returns its URL | Previewed, never posted; returned URL is empty |
| `ci.check(name, state=, description=)` | Create a named commit status; the returned handle's `update(state, description=)` changes it as work progresses. | Created and updated on the commit | Rendered as a status line |
| `ci.group(name, fn)` | Run a function inside a collapsible log group and return its result. | Provider group markers (honors `ci.groups.mode`) | Plain heading, output unchanged |
| `ci.sarif(path)` | Upload a SARIF report to code scanning. | Uploaded | Reported as a one-line preview |

On a CI platform Atmos does not recognize, the `--ci` flag (or `ATMOS_CI=true`) selects the generic provider, which writes to files named by `ATMOS_CI_OUTPUT`, `ATMOS_CI_SUMMARY`, `ATMOS_CI_ENV`, and `ATMOS_CI_PATH`. The generic provider reads the run's context from `ATMOS_CI_PR`, `ATMOS_CI_EVENT`, `ATMOS_CI_RUN_ID`, and `ATMOS_CI_RUN_URL` in addition to the SHA, branch, and repository variables documented in [generic.md](../providers/generic.md).

The GitHub provider honors `GITHUB_API_URL` (GitHub Enterprise Server), overridable with `ATMOS_CI_GITHUB_API_URL`.

### Example

```yaml
commands:
  - name: app test
    arguments: [{ name: component, required: true }]
    flags: [{ name: stack, shorthand: s, required: true }]
    steps:
      - name: test
        type: script
        interpreter: starlark
        script: |
          component, stack = ctx.arguments["component"], ctx.flags["stack"]
          tests = ci.group("test", lambda: exec.run(["go", "test", "./..."], check = False, output = "capture"))
          policy = atmos.validate("component", component, flags = {"stack": stack}, check = False, output = "capture")
          ok = tests.exit_code == 0 and policy.exit_code == 0
          body = "## Test: `" + component + "` in `" + stack + "`"
          ci.summary(body)
          ci.comment(body, key = "app-test:" + component + ":" + stack)
          ci.output("tested", "true" if ok else "false")
          if not ok:
              fail("tests or policy failed")
```

Running `atmos app test api --stack=dev` behaves the same in CI and on a laptop.

## Configuration gating

In CI, every write honors `ci.enabled` plus a flag for its feature:

| Call | Flag | Default once `ci.enabled` is true |
|------|------|-----------------------------------|
| `ci.summary` | `ci.summary.enabled` | on |
| `ci.output`, `ci.env`, `ci.path` | `ci.output.enabled` | on |
| `ci.annotate` | `ci.annotations.enabled` | on |
| `ci.sarif` | `ci.results.enabled` | on |
| `ci.check` | `ci.checks.enabled` | **off (opt-in)** |
| `ci.comment` | `ci.comments.enabled` | **off (opt-in)** |
| `ci.group` | `ci.groups.mode` | per mode |

A disabled call never fails and never silently does nothing. It warns, naming the flag, renders locally, and the script continues. Locally the flags are irrelevant and no warning is shown.

## Fork gate

Under `pull_request_target` and `workflow_run`, the job holds privileged credentials while the triggering code can come from a fork. In that situation `ci.comment` and `ci.check` do not post unless `ci.allow_unsafe_fork_execution` is true. They warn and render locally instead. This is the same opt-in and the same grep-able name as the [fork-PR trust gate](./fork-pr-trust-gate.md). `ci.context.elevated` lets a script detect the situation and skip work that only makes sense when posting is allowed.

## Masking policy

- Summaries, comment bodies, and check descriptions are masked by Atmos before they leave the process.
- Step outputs, environment exports, and SARIF bodies are written exactly as given, because later steps consume them and altering them would corrupt the data.
- `ci.mask(value)` registers a value for redaction from everything printed afterwards. Scripts mask a secret before they print or export it.

## Permissions

On GitHub Actions the job token needs only what the calls in use require:

| Call | Permission |
|------|------------|
| `ci.comment` | `pull-requests: write` |
| `ci.check` | `statuses: write` |
| `ci.sarif` | `security-events: write` |

Summaries, outputs, environment exports, annotations, masking, and groups need no API permission.

## Local parity

Local equals CI is a requirement, not a convenience. With no provider detected, the generic provider renders every call on the terminal, never contacts a remote service, and never requires a token. Failures that only exist in CI (missing token, missing permission) are the only differences, and they are reported with the same error presentation. The generic provider is never auto-detected; it is the fallback when no provider matches.

## Error behavior

- A real provider failure (API error, missing token, nonexistent pull request) fails the script with the usual script error presentation, including a hint where one applies.
- Gating, the fork gate, and the absence of a pull request never fail a script. They warn and render locally.

## `atmos.ci` versus `ci`

These are different things and the names must not be conflated in docs:

- `ci` reports into the CI provider running the script, in process, using the same machinery as built-in reporting.
- `atmos.ci` runs the `atmos ci` command group as a subprocess, like every other `atmos.*` call.

## Dependencies

- The provider-facing seam is the `ci.Reporter` described in [interfaces.md](./interfaces.md#reporter-script-and-step-facing-seam). The module is a thin binding over it and holds no provider logic.
- The local renderer is the [generic provider](../providers/generic.md).
- GitHub endpoints and permissions are in the [GitHub provider PRD](../providers/github/provider.md).

## Remaining implementation work

This implementation has not merged into `main`. Resolve required fixes in the active PR stack and keep remaining work here.

- YAML step wrappers, so non-script steps (`type: ci.comment`, and similar) reach the same seam without a script.
- Container image summary through the same path, so image build and push steps report through `ci.summary`.
- The `ci.summary.template` and `ci.comments.template` settings applied to script calls, so teams can standardize layout for script-authored reports.
- A laptop opt-in to post for real, for debugging a comment against a draft pull request.
- The `pr.fork` field populated from the event payload, so scripts can tell a fork pull request apart without relying on the event name.
- Housekeeping found during implementation: dead `providers/github/loggroup.go`, plugin-local gating copies, and the generic provider reading `ATMOS_CI_OUTPUT`/`ATMOS_CI_SUMMARY` at init instead of call time.

## Testing

- Local rendering for every call through the generic provider, asserting the exact stderr and file output.
- Gating: for each call, a disabled flag warns with the flag name and renders locally; an enabled flag reaches the provider.
- Fork gate: `ci.comment` and `ci.check` hold under `pull_request_target` and `workflow_run`, and post with `ci.allow_unsafe_fork_execution` set.
- GitHub provider against a fake API server, including `GITHUB_API_URL` and `ATMOS_CI_GITHUB_API_URL` precedence.
- Negative path: gating and the fork gate do not fail the script; a provider API error does.
