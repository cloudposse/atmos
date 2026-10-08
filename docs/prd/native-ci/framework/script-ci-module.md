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
- A configured default template for scripts. Script authors pass `template=` in the call, or build their own Markdown.

## User-facing API

The `ci` module is predeclared in every script: standalone scripts and the script steps of custom commands, workflows, and hooks.

| Call | Purpose | In CI | Locally |
|------|---------|-------|---------|
| `ci.context` | Read provider, event, SHA, branch, repository, actor, run ID, run URL, whether the event is elevated, and the pull request (`number`, `head`, `base`, `url`, `fork`, or `None`). Read-only; resolves on first read. | Values from the provider | Values from `ATMOS_CI_*` variables and the local git repository; `local` is true |
| `ci.base` | Read the base that affected detection compares against. | Resolved base | Resolved base |
| `ci.summary(markdown)` | Append Markdown to the job summary. | Written to the provider's summary | Rendered as terminal Markdown on stderr |
| `ci.output(name, value)` | Set a step output for later steps and jobs. | Written to the provider's output file | Printed as `name=value` on stderr; a multiline value prints in the heredoc form |
| `ci.env(name, value)` | Export an environment variable to later steps. | Written to the provider's environment file | Printed as `export NAME=value`, shell-quoted only when the value needs it |
| `ci.path(dir)` | Prepend a directory to `PATH` for later steps. | Written to the provider's path file | Printed as `export PATH=dir:"$PATH"`, with `dir` shell-quoted only when it needs it |
| `ci.mask(value)` | Redact a value from everything printed afterwards, including runner logs. | Registered for redaction in runner logs and Atmos output | Registered for redaction in Atmos output |
| `ci.annotate(level, message, file=, line=, title=)` | Attach an error, warning, or notice to a file and line. | Inline annotation in the pull request diff | Printed as `path:line: level: message (title)` |
| `ci.comment(body, key=, behavior=, pr=, target=, template=, data=)` | Create or update a comment. The `key` makes later calls update the same comment. `target` is `auto` (default), `pr`, or `commit`: `auto` posts to the pull request when one is known and otherwise to a commit comment on the SHA. | Posted to the pull request or commit; returns its URL | Previewed (`commit comment preview (...)` when no pull request is known), never posted; returned URL is empty |
| `ci.check(name, state=, description=, url=)` | Create a named commit status; the returned handle's `update(state, description=)` changes it as work progresses. | Created and updated on the commit; `in_progress` maps to pending and `cancelled` to error on GitHub | Rendered as a status line, with a `URL:` line when a details URL exists |
| `ci.group(name, fn)` | Run a function inside a collapsible log group and return its result. | Provider group markers (honors `ci.groups.mode`) | Plain heading, output unchanged |
| `ci.sarif(path)` | Upload a SARIF report to code scanning. | Uploaded | Reported as a one-line preview that names the file and why it was not uploaded |

On a CI platform Atmos does not recognize, forced CI mode (the `--ci` flag, `ATMOS_CI=true`, or `CI=true` with no recognized provider) makes the generic provider the detected provider. The gates then apply as on a real platform, `ci.context.local` is false, and the generic provider writes to the files named by `ATMOS_CI_OUTPUT`, `ATMOS_CI_SUMMARY`, `ATMOS_CI_ENV`, and `ATMOS_CI_PATH` when they are set. The generic provider reads the run's context from `ATMOS_CI_PR`, `ATMOS_CI_PR_FORK`, `ATMOS_CI_EVENT`, `ATMOS_CI_RUN_ID`, and `ATMOS_CI_RUN_URL` in addition to the SHA, branch, and repository variables documented in [generic.md](../providers/generic.md). `ATMOS_CI_PR_FORK` only fills `ci.context.pr.fork`; it never gates anything on the generic provider.

Workflow commands (`::add-mask::`, `::group::`, annotations) are written to stderr so piped data on stdout stays clean. `::add-mask::` is emitted only when `ci.enabled` is true. An invalid `GITHUB_API_URL` or `ATMOS_CI_GITHUB_API_URL` is a hard error, in scripts and in the Terraform `--ci` flow. Repeated `ci.base()` calls do not duplicate `safe.directory` entries.

The GitHub provider honors `GITHUB_API_URL` (GitHub Enterprise Server), overridable with `ATMOS_CI_GITHUB_API_URL`.

A `ci.comment(behavior="update")` call requires a `key`. Locally, an update with no earlier comment fails the way GitHub does. `target="pr"` with no known pull request fails with the hint "Pass the pull request number or run in a pull request context".

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
| `ci.sarif` | `ci.results.enabled` | **off (opt-in)** |
| `ci.check` | `ci.checks.enabled` | **off (opt-in)** |
| `ci.comment` | `ci.comments.enabled` | **off (opt-in)** |
| `ci.group` | `ci.groups.mode` | per mode |

A disabled call never fails and never silently does nothing. It warns, naming the switch that is off, renders locally, and the script continues. The warning names `ci.enabled` when the master switch is off, and the per-feature flag only when the master switch is on. Locally the flags are irrelevant and no warning is shown.

## Fork gate

Under `pull_request_target` and `workflow_run`, the job holds privileged credentials while the triggering code can come from a fork. For a fork pull request the posting gate holds comments, commit statuses, `ci.env` exports, `ci.path` exports, and SARIF uploads, unless `ci.allow_unsafe_fork_execution` (or `ATMOS_ALLOW_UNSAFE_FORK_EXECUTION`, which sets the same key) is set. Summaries, outputs, annotations, groups, and masks only affect the current job and are never held. A held call warns and renders locally instead. A pull request from the same repository posts normally under these events, and a plain `pull_request` event from a fork is never gated. This is the same opt-in and the same grep-able name as the [fork-PR trust gate](./fork-pr-trust-gate.md), and one setting releases both the clone gate and the posting gate. `ci.context.elevated` and `ci.context.pr.fork` let a script detect the situation and skip work that only makes sense when posting is allowed.

The native Terraform plan comments and commit statuses go through the same reporter and gate as scripts.

The provider decides `fork` from the event payload. The GitHub provider reports a fork when `head.repo.full_name` differs from `base.repo.full_name`; it does not consult `head.repo.fork`, so a repository that is itself a fork is not penalized. A deleted fork (`head.repo` is null) and an unreadable event payload are treated as a fork, so the gate holds rather than fails open. For a `workflow_run` event, which carries no `pull_request` object, it uses `workflow_run.head_repository` the same way. Under `pull_request_target` and `workflow_run`, the pull request number comes from the event payload (`pull_request.number`, or `workflow_run.pull_requests[0].number`) and falls back to `GITHUB_REF`. The generic provider reads `ATMOS_CI_PR_FORK`.

## Report templates

A script keeps the layout of a report in a template file and passes the values:

- `ci.summary(template=, data=)` and `ci.comment(template=, data=)` render a file under `ci.templates.base_path`, or an absolute path, with `data` as the context.
- Scripts have no configured default template. A call that passes `data` without `template=` is an argument error. `ci.summary.template` and `ci.comments.template` are not read by scripts; `ci.summary.template` keeps its meaning for the native plugins only.
- A relative `ci.templates.base_path` resolves against the Atmos `base_path`. There is no default base path.
- A key that the template reads but `data` does not contain fails the render and names the key. A template that cannot be found fails with the resolved path and the value of `ci.templates.base_path`. A configured `ci.templates.container.image` that does not exist is an error.

## Container image comments

The `atmos container build` and `atmos container push` commands report through the same seam. One comment is kept per `registry/repository`, so pushes of new tags update it. Bodies are truncated at 65,000 characters with a note. A comment is attempted only when `ci.comments.enabled` is on; with comments off nothing is previewed in the log. A failed post is logged at Warn and does not fail the build. See the [container provider PRD](../providers/container.md).

## Masking policy

- Summaries, comment bodies, and check descriptions are masked by Atmos before they leave the process.
- Step outputs, environment exports, and SARIF bodies are written exactly as given, because later steps consume them and altering them would corrupt the data.
- `ci.mask(value)` registers a value for redaction from everything printed afterwards. Scripts mask a secret before they print or export it.

## Permissions

On GitHub Actions the job token needs only what the calls in use require:

| Call | Permission |
|------|------------|
| `ci.comment` on a pull request | `pull-requests: write` |
| `ci.comment` on a commit | `contents: write` |
| `ci.check` | `statuses: write` |
| `ci.sarif` | `security-events: write` |

Summaries, outputs, environment exports, annotations, masking, and groups need no API permission.

## Local parity

Local equals CI is a requirement, not a convenience. With no provider detected, the generic provider renders every call on the terminal, never contacts a remote service, and never requires a token. Failures that only exist in CI (missing token, missing permission) are the only differences, and they are reported with the same error presentation. Without forced CI mode the generic provider is the local fallback when no provider matches, and `ci.context.local` is true. With forced CI mode it stands in as the detected provider and `ci.context.local` is false.

## Error behavior

- A real provider failure (API error, missing token, nonexistent pull request) fails the script with the usual script error presentation, including a hint where one applies.
- Gating and the fork gate never fail a script. They warn and render locally.
- The absence of a pull request does not fail `ci.comment` with the default `target="auto"`, which falls back to a commit comment on the SHA. Only `target="pr"` with no known pull request fails.

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
- A laptop opt-in to post for real, for debugging a comment against a draft pull request.

## Testing

- Local rendering for every call through the generic provider, asserting the exact stderr and file output.
- Gating: for each call, a disabled flag warns with the flag name and renders locally; an enabled flag reaches the provider.
- Fork gate: `ci.comment`, `ci.check`, `ci.env`, `ci.path`, and `ci.sarif` hold for a fork pull request under `pull_request_target` and `workflow_run`; the same calls post for a same-repository pull request (with the pull request number read from the event payload when `GITHUB_REF` names the base branch), and post for a fork with `ci.allow_unsafe_fork_execution` set. A deleted fork and an unreadable payload hold; a repository that is itself a fork posts for same-repository pull requests.
- Commit comments: `target="auto"` falls back to a commit comment when no pull request is known, `target="pr"` fails without one, and a keyed commit comment is updated on the next run.
- GitHub provider against a fake API server, including `GITHUB_API_URL` and `ATMOS_CI_GITHUB_API_URL` precedence.
- Negative path: gating and the fork gate do not fail the script; a provider API error does.
