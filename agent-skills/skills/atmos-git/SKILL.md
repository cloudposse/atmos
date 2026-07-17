---
name: atmos-git
description: "Atmos Git and GitOps: git.repositories, clone/pull/status/diff/commit/push/clean, local Git hook shims, signed commits, managed workdirs, fork-PR trust gate, and auth via identities or github/sts"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
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
        signing:
          mode: auto
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
repository.

## GitOps Guidance

- Use managed repositories for deployment repos, generated config repos, and promotion workflows.
- Use signed commits where repository policy requires them; prefer `signing.mode: auto` unless the
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
