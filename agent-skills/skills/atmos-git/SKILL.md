---
name: atmos-git
description: "Atmos Git and GitOps: git.repositories, clone/pull/status/diff/commit/push/clean, local Git hook shims and hook env vars, pre-commit framework migration, Starlark git.sandbox checks and unstaged fixes, signed commits, managed workdirs, fork-PR trust gate (clone gate and posting gate), and auth via identities or github/sts"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.1.0"
  category: ci-automation
---

# Atmos Git

Use this skill for native Git repository management, GitOps automation, managed workdirs, local Git
hook shims, signed commits, and GitHub auth through Atmos Auth or Atmos Pro STS.

## Related Skills

| Need | Load |
|---|---|
| Atmos Pro GitHub App commits | [atmos-pro](../atmos-pro/SKILL.md) |
| `github/sts` credentials for private repos | [atmos-auth](../atmos-auth/SKILL.md) |
| Lifecycle `git` hooks | [atmos-hooks](../atmos-hooks/SKILL.md) |
| Modernizing old GitHub Actions GitOps patterns | [atmos-modernization](../atmos-modernization/SKILL.md) |

## Configuration

Configure managed repositories in `atmos.yaml`:

```yaml
git:
  repositories:
    deployment:
      uri: https://github.com/acme/deployment.git
      branch: main
      auth:
        identity: atmos-pro
      commit:
        author:
          name: Atmos Bot
          email: atmos@example.com
        signing: auto               # auto | always | never
```

Use identities or `github/sts` for private GitHub access. Do not put tokens in repository URIs.

## Commands

| Command | Purpose |
|---|---|
| `atmos git list` | List configured repositories |
| `atmos git clone <name-or-uri>` | Clone or reconcile a managed repository |
| `atmos git init <name-or-path>` | Initialize a managed repository |
| `atmos git pull <name-or-path>` | Fast-forward pull |
| `atmos git status <name-or-path>` | Show working tree status |
| `atmos git diff <name-or-path>` | Show changes |
| `atmos git commit <name-or-path> --message "msg"` | Stage managed paths and commit |
| `atmos git push <name-or-path>` | Push commits |
| `atmos git clean <name>` | Remove managed workdirs |

Use `atmos git hooks install`, `run`, and `uninstall` for local Git hook shims in the current
repository. Shims pin the absolute `atmos` path at install time and fall back to `PATH`.
The `atmos git hooks run` command exits with the failing step's code (`fail()` exits 1,
`errors.build().with_exit_code(5)` exits 5).

### Pre-commit checks and migration

Use `git.hooks.pre-commit.steps` for native commit checks. A `type: script` step
with `interpreter: starlark` can load a shared suite, then call `steps.test(...)`
with named checks, `output="failures"`, and `fail={"mode": "wait_all"}`. Keep
formatters sequential. A YAML catalog loaded with `yaml.decode(fs.read_file(...))`
can declare each check's handler, file patterns, exclusions, and fix eligibility;
the catalog schema and handlers belong to the project, not Atmos's hook schema.

For staged checks, run the suite inside `git.sandbox(run=callback, scope="staged")`.
This materializes the index into a private temporary Git checkout without
registering a worktree. The default `fixes="none"` reports differences without
changing source files. For commit-time fixes use `fixes="unstaged"`; the default
`fail_on_changes=True` blocks the commit so changes can be reviewed and restaged.
The live index is never changed. Nonconflicting unstaged edits are retained;
overlapping or concurrent edits keep the original file and produce a recovery
patch. Cancellation discards fixes. See the [Starlark API](../atmos-starlark/references/api-reference.md).

Prepare binaries and pinned tools before installing hooks; commit hooks should
not build or install dependencies. To replace a recognized pre-commit-framework
shim, use `atmos git hooks install pre-commit --force`. It backs up
the old shim before replacement. This also permits replacing custom hooks, so
inspect the existing hook first and resolve `core.hooksPath` conflicts explicitly.
Linked worktrees may share the hook directory; prepare each worktree's tools and
preserve existing hook configuration unless replacement was requested.

Commands such as `atmos check staged` and `atmos format pr` are project-defined
custom commands, not built-in Atmos Git commands. Consult the project's catalog
and setup instructions before assuming they exist.

### Git Hook Environment

While a hook runs, shell steps get the hook arguments as `"$@"` and in `ATMOS_GIT_HOOK_ARGS`; Starlark
steps get `ctx.args`. For `pre-push`, `pre-receive`, `post-receive`, `post-rewrite`,
`reference-transaction`, and `proc-receive`, `ATMOS_GIT_HOOK_STDIN` is the path of a temp file holding the
data Git piped in (removed when the hook ends; the first shell step still receives it on stdin).
The `ATMOS_HOOK_DEPTH` variable counts nested hooks; a hook that reaches depth 8 fails with
`hook recursion limit exceeded`. A later config source that switches a hook between `steps:` and `command:`
replaces the earlier form. A prompt step without a `default:` fails under a non-TTY hook. Hook `print()`
reaches stdout, so a hook on a data command must use `ui.*`.

## GitOps Guidance

- Use managed repositories for deployment repos, generated config repos, and promotion workflows.
- Use signed commits where repository policy requires them; prefer `commit.signing: auto` unless the
  workflow requires `always` or `never`.
- Use `github/sts` in CI so Git subprocesses receive short-lived GitHub App credentials.
- Use `atmos pro commit` when CI-generated commits must trigger follow-on GitHub Actions workflows.
- Keep generated commits traceable with clear messages and commit trailers when the project uses
  provenance conventions.

### Fork-PR Trust Gate

`atmos git clone` is Atmos's native replacement for `actions/checkout`, including a no-arg mode
that checks out the current CI repository directly from CI environment variables. In
`pull_request_target` and `workflow_run` contexts the job holds base-repository secrets and
`GITHUB_TOKEN` — the same privilege GitHub hardened `actions/checkout@v7` against by refusing to
fetch fork PR code by default (the "pwn request" class of risk). Atmos guards the equivalent gap:
when it detects an elevated event (`pull_request_target`/`workflow_run`) combined with a
fork-targeting clone request — an explicit `--branch refs/pull/<N>/merge`/`refs/pull/<N>/head`
override, or an ad-hoc clone URI whose host or `owner/repo` differs from the base repository — it
refuses to clone and exits non-zero. The safe no-arg default (base repository at its base ref) is
never gated, and `pull_request`/`push` events are not gated since they don't hold elevated
credentials against untrusted code. Opt in explicitly and only with a documented reason via
`--allow-unsafe-fork`, `ATMOS_ALLOW_UNSAFE_FORK_EXECUTION`, or
`ci.allow_unsafe_fork_execution: true`; the bypass logs a prominent warning so it stays visible in
CI logs and easy to grep for in review. Prefer `pull_request` (not `pull_request_target`) for
workflows that clone and plan fork contributions, since `pull_request` withholds fork secrets.

### Fork Posting Gate

The clone gate has a second half for CI reporting. Under `pull_request_target` and `workflow_run`, the
posting gate holds comments, commit statuses, `ci.env`/`ci.path` exports, and SARIF uploads for a fork
pull request, from the `ci` script module and from native Terraform plan comments alike. Summaries,
outputs, annotations, groups, and masks are never held. A fork means the head and base repository
`full_name` differ; a deleted fork or an unreadable event payload counts as a fork, a repository that is
itself a fork is not penalized, and a plain `pull_request` event is never gated.
`ci.allow_unsafe_fork_execution` and `ATMOS_ALLOW_UNSAFE_FORK_EXECUTION` set the same key and release both
the clone gate and the posting gate.
