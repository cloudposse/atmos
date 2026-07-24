# PRD: Atmos Git (GitOps Enablement)

**Status:** Proposed (Component Updater PR publishing is implemented separately)
**Version:** 0.3
**Last Updated:** 2026-06-10
**Author:** Atmos Team

**Related PRDs:**
- [Atmos Toolchain for Third-Party Tool Management](./toolchain-implementation.md)
- [Provisioner System](./provisioner-system.md)
- [Source Provisioner](./source-provisioner.md)
- [Custom Hooks](./custom-hooks.md)
- [Native CI Integration](./native-ci-integration.md)
- [Native Component Updater PR Workflow](./component-updater.md)
- [Atmos Pro STS](./atmos-pro-sts.md)

---

## Executive Summary

Atmos Git makes Git a reusable, foundational Atmos platform capability, similar in scope to Toolchain, Auth, and Hooks. Atmos should be able to clone, pull, inspect, diff, commit, and push Git repositories as part of provisioners, hooks, workflows, custom commands, CI workflows, and component lifecycle events.

This is the enablement layer for GitOps workflows: Atmos publishes desired state and generated artifacts to source-of-truth repositories; reconciliation is performed by consumers such as Argo CD, Flux, or CI. Atmos is the producer side of a GitOps pipeline, not the reconciler.

The core idea is that Git repositories can be artifact repositories. Atmos can render or generate artifacts, place them into a repository worktree, and perform Git operations through a shared service with consistent authentication, safety rules, and command behavior.

There are two distinct repository targets, and the architecture keeps them separate:

1. **Managed repositories** (`provision.git`): a *separate, configured* repository that Atmos clones and reconciles into a managed workdir. This is the Flux/Argo CD deployment repository case — the deployment repo is not the repository the user is working in.
2. **The current repository** (`git` hook kind): the *already-cloned* repository the component lives in. This is the Terraform case — commit generated files back to the working repository after `after.terraform.apply`.

Both targets are served by the same shared Git service.

The first concrete consumers are:

1. Kubernetes components rendering manifests into deployment repositories for Argo CD and Flux.
2. Terraform components committing generated artifacts in the current repository after lifecycle events such as `after.terraform.apply`, via a `git` hook kind.
3. Native CI workflows using `atmos git clone` as an Atmos-aware replacement for GitHub clone/checkout action patterns.
4. Local Git hooks using Atmos-managed `.git/hooks/*` shims that delegate to workflows or custom commands.

V1 ships a single `cli` provider that shells out to the Git CLI. The Component Updater additionally registers a focused GitHub API pull-request publisher; its behavior is defined in the [Component Updater PRD](./component-updater.md), not as general provisioner PR support.

---

## Problem Statement

Atmos already provisions tools, component sources, Terraform backends, CI metadata, and credentials. Git operations are still scattered across use cases or left to external scripts and CI actions. This creates several problems:

1. Kubernetes deployment repository workflows require ad hoc scripts to render manifests, copy files, commit, and push.
2. Terraform post-apply artifact publishing requires custom glue, even when Atmos owns the lifecycle event and can render the artifact.
3. Native CI workflows still depend on external checkout/clone steps even when Atmos Auth can provide Git credentials through GitHub STS.
4. Local Git hook setup is left to tools like Husky or project-specific shell scripts, which do not naturally use Atmos workflows, custom commands, Toolchain, or Auth.
5. Git safety policies such as no force push, fast-forward-only pull, push-rejection retry, signed commit control, and path-scoped dirty-tree checks are not centralized.

Atmos needs a reusable Git foundation that can be used consistently by CLI commands, provisioners, hooks, and lifecycle integrations.

---

## Goals

1. **Reusable Git Service:** Provide a `pkg/git` service with clone, pull, status, diff, commit, and push operations behind a provider registry.
2. **CLI Provider First:** Implement Git operations using the Git CLI in v1.
3. **Provider Abstraction:** Keep a provider boundary so a future GitHub API backend can be added without redesigning the public surface.
4. **Atmos Auth Integration:** Run Git subprocesses with current process environment plus optional Atmos Auth identity environment from `EnsureIdentityEnvironment`.
5. **Git Artifact Repositories:** Treat Git repositories as destinations for rendered and generated artifacts.
6. **Kubernetes Deployment Repos:** Render Kubernetes manifests into managed deployment repositories for Argo CD and Flux.
7. **Lifecycle Git Operations:** Allow Atmos lifecycle events, such as `after.terraform.apply`, to trigger Git operations through a `git` hook kind that reuses the existing hooks engine.
8. **Local Git Hook Shims:** Install local `.git/hooks/*` scripts that delegate to `atmos git hooks run`.
9. **Toolchain Compatibility:** Ensure local Git hook behavior can run Atmos workflows or custom commands so existing Toolchain resolution and PATH setup are reused.
10. **Native CI Support:** Allow `atmos git clone` to replace CI checkout/clone glue when `ci.enabled: true` and supported CI metadata is present.
11. **CI Cache Compatibility:** Place managed repository workdirs under the Atmos XDG cache root so the native CI cache captures them automatically.

---

## Non-Goals

1. **General GitHub API Provider in v1:** Component Updater PR publishing is implemented separately. Managed repositories and lifecycle hooks do not gain API PR publishing from it.
2. **Force Push:** Atmos will not perform force pushes in v1.
3. **Provisioner/Hook Pull Request Creation in v1:** This remains out of scope. The Component Updater is the only PR-publishing consumer.
4. **Managing `core.hooksPath`:** V1 writes local `.git/hooks/<hook>` shims and does not manage `core.hooksPath`.
5. **Replacing Internal Lifecycle Hooks:** Local Git hooks are separate from Atmos lifecycle events such as `after.terraform.apply`.
6. **General Git Porcelain Replacement:** Atmos Git is not intended to expose every Git command.
7. **Implicit Arbitrary Shell Automation:** Git operations should be explicit, configured, and bounded by safety rules.
8. **A Competing Clone Cache:** `atmos git clone` does not implement its own cross-run caching; it relies on XDG workdir placement plus the native CI cache, and reconciles restored workdirs instead.
9. **Continuous Reconciliation / Drift Detection:** Atmos does not implement a pull-based reconciler (Flux/Argo CD style). Atmos is the publisher; reconcilers consume what it publishes.

---

## Concepts

### Capability vs Provider

Git is a foundational Atmos *capability* with a top-level config section and a service package, like Toolchain, Auth, and Hooks. Within the capability, *providers* are pluggable backends registered in a provider registry, following the existing Atmos pattern (`pkg/ci/providers/`, Kubernetes component `provider: kubectl|kustomize`, auth providers, store registry).

The `cli` provider is the universal, host-agnostic default: it works with GitHub, GitLab, Bitbucket, and bare repositories. The future `github` provider is not a strict peer — it adds host-specific API capabilities (pull request creation, API commits to protected branches). When such an API-only capability is requested, the provider can be auto-resolved from the repository URI host, the same way GitHub STS keys off the GitHub host today. Users should rarely need to set `provider` explicitly.

### Managed Repository

A managed repository is a named Git repository under `git.repositories`. The name is a user-defined logical key. It is not a reserved value.

For example, `flux-deploy`, `generated-terraform`, and `deployments` are all arbitrary names:

```yaml
git:
  repositories:
    flux-deploy:
      uri: https://github.com/acme/flux-deploy.git

    generated-terraform:
      uri: https://github.com/acme/generated-terraform.git

    deployments:
      uri: https://github.com/acme/deployments.git
```

Components and commands refer to repositories by these logical names. Repository configuration follows standard Atmos deep-merge.

### Git Repository as Artifact Repository

A Git repository can be a destination for generated artifacts. Atmos can render files into a repository worktree, commit them, and push them. Examples include:

1. Kubernetes manifests consumed by Argo CD or Flux.
2. Terraform-generated files published after `apply`.
3. Generated policy, inventory, documentation, or component metadata.

### Three Hook-Shaped Things, Kept Separate

Atmos Git touches three distinct mechanisms. They are intentionally separate:

| Mechanism | Config | Triggered by | Repository target |
| --- | --- | --- | --- |
| Local Git hooks | `git.hooks` | Git itself (`pre-commit`, `commit-msg`, ...) | The current repository |
| `git` hook kind | `components.<type>.<name>.hooks` with `kind: git` | Atmos lifecycle events (`after.terraform.apply`, `after.kubernetes.render`, ...) | Current repository by default; a managed repository when `repository` is set |
| Git provisioning | `components.<type>.<name>.provision.git` | Component provisioning (no events) | A managed repository |

`provision.git` is repository *lifecycle*: ensure the managed repository is cloned, reconciled, and ready, and direct rendered artifacts into it. It has no `event` field. Git *operations* bound to lifecycle events use the `git` hook kind, which reuses the existing hooks engine: event dispatch, `--skip-hooks`, `on_failure` semantics, and kind self-registration (`pkg/hooks`, alongside `store`, `command`, `infracost`, and friends).

---

## Configuration

### Top-Level Git Configuration

```yaml
git:
  repositories:
    flux-deploy:
      uri: https://github.com/acme/flux-deploy.git
      auth:
        identity: platform-admin
      commit:
        signing: auto

    generated-terraform:
      uri: https://github.com/acme/generated-terraform.git

  hooks:
    pre-commit:
      command: atmos workflow pre-commit
    commit-msg:
      command: atmos workflow commit-msg -- "$1"
```

The keys under `git.repositories.<name>` are arbitrary logical names. A user can define as many repositories as needed.

### Repository Defaults

When omitted:

| Field | Default |
| --- | --- |
| `provider` | `cli` |
| `remote` | `origin` |
| `branch` | Current branch for existing repos; Git/default branch behavior for clone |
| `workdir` | Automatic XDG cache location (see below) |
| `commit.signing` | `auto` |
| `commit.author` | Identity-derived when available; otherwise an `atmos[bot]`-style default |
| `push.retries` | `3` |
| `clone.depth` | `0` (full history) |
| `clone.submodules` | `false` |

Clone depth is a first-class requirement: it is supported both per repository (`clone.depth`) and per invocation (`--depth`), with the flag taking precedence (CLI > ENV > config > defaults). The same applies to `clone.filter` / `--filter` and `clone.single_branch` / `--single-branch`.

**Automatic workdirs:** when `workdir` is omitted, Atmos resolves a deterministic location under the Atmos XDG cache root using the existing `pkg/xdg` helpers:

```text
$XDG_CACHE_HOME/atmos/git/repositories/<name>
```

with the standard `pkg/xdg` fallbacks and `ATMOS_XDG_*` overrides. Placing workdirs under the XDG cache root means the native CI cache (which archives the XDG cache root) captures and restores managed clones across CI runs for free. Because a restored workdir may be stale, all clone behavior is defined as *reconcile* (see Workdir Reconciliation). An explicit `workdir` overrides the automatic location.

**Clone destination rules** (the two-repo split extends to destinations):

| Clone form | Destination |
| --- | --- |
| Named managed repository (`atmos git clone flux-deploy`) | Automatic XDG workdir (cache-captured) |
| No-arg current-repo checkout replacement in CI | The **working directory** (e.g., `GITHUB_WORKSPACE`), exactly like `actions/checkout` — never an XDG workdir; subsequent Atmos commands expect the repo at CWD |
| Ad hoc URI (`atmos git clone https://...`) | `<cwd>/<repo-name>`, like plain `git clone` |

`--workdir` overrides the destination in all forms.

### Repository Fields

```yaml
git:
  repositories:
    flux-deploy:
      provider: cli                  # optional; cli is the default
      uri: https://github.com/acme/flux-deploy.git
      branch: main
      remote: origin
      workdir: .workdir/git/flux-deploy   # optional; default is automatic XDG
      clone:
        depth: 1                    # shallow clone depth; 0 = full history (default)
        filter: blob:none           # optional partial-clone filter
        single_branch: true
        submodules: false           # default false
      auth:
        identity: platform-admin
      commit:
        signing: auto                # auto | always | never
        author:
          name: atmos[bot]
          email: atmos-bot@acme.com
      push:
        retries: 3
```

Credentials are never embedded in `uri` or remote URLs. Authentication flows through the process environment and Atmos Auth (`GIT_CONFIG_*` insteadOf rewrites from GitHub STS), never through URL rewriting with tokens.

### Authentication Resolution

`auth.identity` is the only named auth reference on a repository, consistent with the rest of Atmos: **integrations are never referenced by name from config objects**. Integrations attach via their `via` link — `via.identity` binds to a specific identity, and for `github/sts` the integration should also support `via.provider`, binding to a provider so the integration follows *any* identity chained through that provider (one STS integration covers every identity on the provider, instead of one per identity). `EnsureIdentityEnvironment(ctx, identity)` composes the identity environment plus all linked `auto_provision` integrations. Naming an identity on a repository therefore brings its integrations along automatically — a `github/sts` integration linked to `platform-admin` materializes `GIT_CONFIG_*` credentials for every Git subprocess targeting that repository, with zero Git-specific auth wiring:

```yaml
auth:
  identities:
    platform-admin:
      kind: github/user        # illustrative
  integrations:
    deploy-sts:
      kind: github/sts
      via:
        identity: platform-admin

git:
  repositories:
    flux-deploy:
      uri: https://github.com/acme/flux-deploy.git
      auth:
        identity: platform-admin   # pulls in deploy-sts via the identity link
```

Per-operation resolution order:

1. `--identity=<name>` flag.
2. `git.repositories.<name>.auth.identity`.
3. **Ambient credential broker:** when no identity is configured, Git subprocess spawn is a broker choke point — the lazy ambient broker (`pkg/auth/broker`) runs before the first Git subprocess, exactly as it does for other remote reads. This is the zero-config CI path: `atmos git clone` of a private GitHub repository works with no `auth` block at all when an auto-provision `github/sts` integration (or ambient `GITHUB_TOKEN`) is available.
4. Process environment passthrough: the developer's own Git credentials (credential helpers, SSH agent) always work, since the Git CLI provider inherits the process environment.

The Git service must register its subprocess spawn as a broker choke point; it must not duplicate broker or integration selection logic.

### Commit Author

CI runners typically have no `user.name`/`user.email` configured, and `git commit` fails without them. `commit.author` provides the identity used for commits, passed per invocation (`-c user.name=... -c user.email=...`) without mutating repository or global Git config. Defaults:

1. Explicit `commit.author` config.
2. Identity-derived values when an Atmos Auth identity is in play and exposes them.
3. A documented `atmos[bot]` fallback.

Local interactive use keeps the user's own Git config: when Git already resolves an author, Atmos passes nothing.

### Local Git Hook Configuration

```yaml
git:
  hooks:
    pre-commit:
      command: atmos workflow pre-commit
    commit-msg:
      command: atmos workflow commit-msg -- "$1"
```

Hook `command` strings should usually invoke an Atmos workflow or custom command. This preserves existing Toolchain, environment, and identity behavior through the existing runners.

### Templating

`commit.message`, `provision.git.path`, and other templated fields use the existing Atmos template engine (`FuncMap()` from `internal/exec/template_funcs.go`) with the standard component context: `{{ .component }}`, `{{ .stack }}`, `{{ .vars.* }}`, etc. No new template engine or context is introduced.

---

## CLI Commands

### List Command

```bash
atmos git list [--columns=<col>,...] [--format=table|json|yaml|csv|tsv] [--delimiter=<char>] [--check-status]
```

Lists configured repositories from `git.repositories`, built on the standard `pkg/list` rendering pipeline (filter → column → sort → format → output) like other `atmos list`-style commands:

| Column | Content |
| --- | --- |
| `name` | Logical repository name |
| `uri` | Repository URI |
| `provider` | Resolved provider (`cli`) |
| `branch` | Configured branch or `(default)` |
| `workdir` | Resolved workdir (automatic XDG or explicit) |
| `status` | `cloned`, `missing`, or `dirty` — only with `--check-status` |

Spec details (so implementation does not re-decide scope):

1. **Formats:** `table|json|yaml|csv|tsv` plus `--delimiter` for CSV/TSV, per `pkg/list/format`. `tree` and `matrix` are explicitly not supported for this flat list.
2. **`status` is resolved at extraction time, not in a column template** — the extractor performs the filesystem/`git status` probes and materializes the value into the row map before rendering (column templates are pure and cannot do I/O). Probes run concurrently with a bounded worker pool.
3. **`status` is opt-in via `--check-status`** so the default `atmos git list` never touches the filesystem or runs Git and is always fast.
4. **Default sort:** `name:asc`.
5. **Column configuration** in `atmos.yaml` follows the established `<section>.list.columns` pattern:

    ```yaml
    git:
      list:
        format: table
        columns:
          - name: Name
            value: "{{ .name }}"
          - name: URI
            value: "{{ .uri }}"
    ```

    This also feeds dynamic tab completion for `--columns`.
6. **Alias (decided):** `atmos list git-repositories` is registered via the command registry alias mechanism (`CommandAlias` from `GetAliases()`), pointing at `atmos git list` — same pattern as `atmos workflow list` ↔ `atmos list workflows`.
7. **Filtering:** v1 includes the standard list filter flag, bound to `ATMOS_GIT_LIST_FILTER` (not the shared `ATMOS_LIST_FILTER` key).

### Repository Commands

```bash
atmos git clone [name-or-uri] [--all]
atmos git pull <name-or-path> [--all]
atmos git status <name-or-path> [--all]
atmos git diff <name-or-path> [--path=<path>...]
atmos git commit <name-or-path> --message=<msg> [--path=<path>...] [--sign|--no-sign] [--dry-run]
atmos git push <name-or-path> [--dry-run]
```

**Argument resolution order:** configured repository name → URI (detected by scheme, `git::` prefix, or scp-style pattern) → path. To force path interpretation for an argument that collides with a repository name, use an explicit path prefix (`./deployments`). Bare arguments that match no configured name and no URI pattern are treated as paths.

### Clone Argument Forms

`atmos git clone` accepts, in addition to a configured repository name:

1. Plain Git URLs: `https://github.com/acme/repo.git`, `git@github.com:acme/repo.git` (scp-style).
2. **Go-getter style `git::` URIs**, consistent with the syntax users already write in vendoring and `source` configs:

    ```bash
    atmos git clone 'git::https://github.com/acme/repo.git?ref=main&depth=1'
    ```

    The `git::` forcing prefix is stripped; `?ref=` maps to branch/ref and `?depth=` to clone depth. Precedence: explicit flags > query params > repository config > defaults. Honored query params are `ref` and `depth`; unknown params fail with a clear error. Implementation reuses the existing go-getter URI parsing from the vendoring/downloader code path — no new parser.

**Ad hoc URI clone destination:** a URI clone with no configured name clones into `<cwd>/<repo-name>` like plain `git clone`; `--workdir` overrides. XDG workdirs are reserved for *named* managed repositories.

### Bulk Operations: `--all`

`--all` operates on every repository configured under `git.repositories`, following the established Atmos bulk-ops convention:

- `atmos git clone --all` — clone/reconcile all managed workdirs. One CI warm-up step materializes every deployment repo (especially effective right after a CI cache restore).
- `atmos git pull --all` — fast-forward-only pull across all cloned workdirs.
- `atmos git status --all` — status across all workdirs (shares extraction logic with `atmos git list --check-status`).
- `commit --all` and `push --all` are **not** supported in v1: commit requires per-repo message/path intent, and bulk publishing is the hooks/provisioner layer's job (see Open Questions).

Shared `--all` semantics:

1. Mutually exclusive with a positional name/URI/path argument and with `--repo-uri`.
2. Runs concurrently with a bounded worker pool.
3. Per-repo identity resolution — each repository's `auth.identity` applies independently.
4. Attempt-all error semantics: every repository is attempted, per-repo results are reported, and the command exits non-zero with a combined error (`errors.Join`) if any failed. Not fail-fast — one bad repo must not block warming the rest.
5. Env bindings per command (`ATMOS_GIT_CLONE_ALL`, `ATMOS_GIT_PULL_ALL`, ...) following the flag conventions.

Shared flags:

```bash
--identity=<name>      # the EXISTING global persistent flag; not re-registered by cmd/git
--repo-uri=<uri>
--branch=<branch>
--remote=<remote>
--workdir=<path>
```

Clone flags (CI checkout replacement):

```bash
--depth=<n>            # shallow clone; 0 = full history
--filter=<spec>        # e.g. blob:none for partial clone (matches git's own flag name)
--single-branch
--submodules           # off by default
```

> **Interplay with `atmos describe affected`:** affected detection requires merge-base history. Shallow defaults must not break it. Documentation must give fetch-depth guidance equivalent to `actions/checkout` (e.g., `--depth=0` or a sufficient depth when `describe affected` runs against the clone).

Commit flags:

```bash
--message=<message>
--path=<path>          # string-slice flag (repeatable / comma-separated)
--sign
--no-sign
--dry-run
```

`--dry-run` for `commit` and `push` reports exactly what would be staged, committed, or pushed without performing the operation.

### Flag Architecture

All `cmd/git` commands register via the command registry (`CommandProvider`) under a new **Git** command group (Git is a foundational capability; it does not belong in "Other Commands"). All command-specific flags use `flags.NewStandardParser()` with the two-step binding from the reference implementation (`cmd/version/version.go`): `BindToViper` in `init()`, `BindFlagsToViper` in `RunE`. Never `viper.BindEnv()`/`viper.BindPFlag()` directly (Forbidigo-enforced).

Specific requirements discovered against the existing flag registry:

1. **`--identity` is already a global persistent flag** (bound to `ATMOS_IDENTITY`, with select support). `cmd/git` must NOT re-register it — Cobra panics on redefinition. Read it from the inherited global.
2. **All git-specific env vars use the `ATMOS_GIT_` prefix** to avoid Viper flat-keyspace collisions with existing bindings (`ATMOS_REPO_PATH` is taken by terraform/list-affected; `ATMOS_WORKDIR_*` is taken by `terraform workdir clean`; `ATMOS_DRY_RUN` is the generic dry-run key; `ATMOS_LIST_FILTER` is the list filter):

    | Flag | Env var |
    | --- | --- |
    | `--repo-uri` | `ATMOS_GIT_REPO_URI` |
    | `--branch` | `ATMOS_GIT_BRANCH` |
    | `--remote` | `ATMOS_GIT_REMOTE` |
    | `--workdir` | `ATMOS_GIT_WORKDIR` |
    | `--depth` | `ATMOS_GIT_DEPTH` |
    | `--filter` (clone) | `ATMOS_GIT_FILTER` |
    | `--dry-run` | `ATMOS_GIT_DRY_RUN` |
    | `--columns` (list) | `ATMOS_GIT_LIST_COLUMNS` |
    | `--format` (list) | `ATMOS_GIT_LIST_FORMAT` |

3. **`--sign`/`--no-sign` mutual exclusion** uses the established pattern: register both via `WithBoolFlag`, then `cmd.MarkFlagsMutuallyExclusive("sign", "no-sign")` after `parser.RegisterFlags(cmd)` (precedent: `cmd/devcontainer/shell.go`).
4. **`--path` is a string-slice flag** (`WithStringSliceFlag`), following existing repeatable-flag plumbing.
5. **`<name-or-path>` resolution is business logic** in `RunE` (or a dedicated resolver), not encoded in `PositionalArgsBuilder`.

### Diff

`atmos git diff` is the read-before-write step for GitOps publishing: it shows the difference between the rendered/working state of a managed path and the repository's committed state, without committing. In CI, this enables pull request previews of what a change would do to a deployment repository — the GitOps analog of `terraform plan`.

### Hook Commands

```bash
atmos git hooks install [hook-name...] [--force]
atmos git hooks uninstall [hook-name...]
atmos git hooks run <hook-name> [args...]
```

`install` writes local `.git/hooks/<hook>` shim scripts:

```sh
#!/bin/sh
exec atmos git hooks run <hook> "$@"
```

Install behavior:

1. Install all configured hooks when no hook names are provided.
2. Install only the requested hooks when hook names are provided.
3. Refuse to overwrite an existing hook unless `--force` is passed.
4. Mark generated hook scripts executable.
5. Do not manage `core.hooksPath`, but **warn** when `core.hooksPath` is set (e.g., by Husky), because Git will ignore `.git/hooks/*` shims in that case.
6. Resolve the hooks directory through Git (`git rev-parse --git-path hooks`) so linked worktrees (where `.git` is a file and hooks live in the common dir) work correctly.

Uninstall behavior:

1. Remove only shims that Atmos generated (identified by shim content), never user-authored hooks.

Run behavior:

1. Load Atmos config for the current repository.
2. Resolve `git.hooks.<hook-name>.command`.
3. Execute the configured command via the shared workflow/command dispatch (`workflow.CommandRunner`), inheriting ToolchainPATH, env, and identity behavior.
4. Forward hook args **and stdin** (hooks such as `pre-push` and `pre-receive` receive their input on stdin, not argv).
5. Propagate the command's exit code.
6. Fail with `ErrGitHookNotConfigured` when the hook is not configured (see Error Handling).

Because hook args are arbitrary strings that may look like flags (`commit-msg "$1"`), `hooks run` must disable Cobra flag parsing for trailing args (`DisableFlagParsing` with manual hook-name extraction, or `FParseErrWhitelist{UnknownFlags: true}`), following the existing passthrough-command precedent.

**PATH caveat (documented):** shims invoke `atmos` from PATH. GUI Git clients (IDEs, Sourcetree) may run hooks with a different PATH than the user's shell; documentation must cover this, as it is the most common Husky-class failure mode.

---

## Git Service Design

### Relationship to the Existing `pkg/git`

`pkg/git` already exists: a go-git-based package (`RepositoryOperations`, `GetLocalRepo()`, `GetRepoInfo()`) consumed by `describe affected`, the CI providers, and Atmos Pro. That package is read-oriented (open, inspect, merge-base). The new service adds *write* operations (clone, pull, commit, push) behind a provider registry and coexists with — and extends — the existing package rather than replacing it. Exact packaging (subpackage vs merged files) is an implementation detail; the public surface is the service plus the provider registry.

**Why the CLI provider, not go-git, for v1:** GitHub STS materializes credentials as `GIT_CONFIG_KEY_n`/`GIT_CONFIG_VALUE_n` environment variables (`pkg/auth/integrations/github/sts.go`). Subprocess `git` honors these; go-git ignores them. The CLI provider is therefore the only backend that gets Atmos Auth/STS credential injection for free, which is the load-bearing requirement for native CI.

### Provider Interface

The Git service exposes a small provider abstraction, registered via the standard Atmos registry pattern:

```go
type Provider interface {
    Clone(ctx context.Context, opts CloneOptions) error
    Pull(ctx context.Context, opts PullOptions) error
    Status(ctx context.Context, opts StatusOptions) (*StatusResult, error)
    Diff(ctx context.Context, opts DiffOptions) (*DiffResult, error)
    Commit(ctx context.Context, opts CommitOptions) (*CommitResult, error)
    Push(ctx context.Context, opts PushOptions) error
}
```

V1 implements:

```text
provider: cli
```

Future:

```text
provider: github
```

The future GitHub provider is not part of v1. Its concrete motivation is pull-request-based publishing: pushing to a branch and opening a PR against protected deployment-repo branches, where direct pushes are rejected by branch protection.

### Runner

The CLI provider uses a fakeable runner so command construction can be unit tested without invoking Git:

```go
type Runner interface {
    Run(ctx context.Context, command string, args []string, opts RunOptions) (RunResult, error)
}
```

Production runner uses `exec.CommandContext`.

### Environment

Every Git subprocess receives:

1. Current process environment.
2. Atmos config global env where appropriate for command context.
3. Optional identity environment from `auth.Manager.EnsureIdentityEnvironment(ctx, identity)`.

Atmos Auth identity env is required for GitHub STS support in native CI because the STS integration returns `GIT_CONFIG_*` environment variables for Git subprocesses.

### Workdir Reconciliation

Clone semantics are defined as **reconcile**, because workdirs persist across runs (and are restored, possibly stale, by the native CI cache):

1. Workdir absent → clone.
2. Workdir present and clean → fetch, checkout configured branch, fast-forward to the remote ref.
3. Workdir present and dirty from a crashed or interrupted run → defined recovery: discard uncommitted changes *inside managed paths only* and re-reconcile; fail with a hint when unmanaged dirty files are present.

Concurrent access to the same workdir within one process (parallel component execution) is serialized with a file lock (reuse `pkg/cache` `FileLock`).

---

## Safety Rules

### Pull

`pull` must be fast-forward-only by default:

```bash
git pull --ff-only <remote> <branch>
```

### Push

Atmos must not perform force push in v1.

**Push contention:** a rejected non-fast-forward push (another component, job, or human pushed first) is the most common failure mode in GitOps publishing and must be handled, not surfaced raw:

1. On rejection, run a bounded retry loop: `pull --ff-only` (or fetch + replay of the managed-path commit), then re-push.
2. Retry count is configurable via `push.retries` (default `3`).
3. After exhaustion, fail with a clear error and hint (e.g., the remote branch is receiving concurrent pushes; consider serializing publishers or batching).

### Commit

Commit behavior:

1. Stage only managed paths when paths are provided.
2. Fail when path-scoped commits detect unrelated dirty files outside managed paths.
3. No-op cleanly when there are no staged or managed changes.
4. Return a structured result that indicates whether a commit was created.
5. Append provenance trailers to generated commits:

```text
Atmos-Stack: <stack>
Atmos-Component: <component>
Atmos-Source-SHA: <sha of the source repository, when available>
```

Trailers make deployment-repo commits traceable back to exactly what produced them, and provide the substrate for deployment provenance (e.g., Atmos Pro).

### Path Validation

Any rendered or configured repository-relative path (`provision.git.path`, `--path` values) must resolve inside the repository worktree after template rendering and path cleaning. Path traversal out of the worktree is an error.

### Signing

Signing modes:

| Mode | Behavior |
| --- | --- |
| `auto` | Pass no signing flag; Git config decides |
| `always` | Pass `-S` to `git commit` |
| `never` | Pass `--no-gpg-sign` to `git commit` |

CLI overrides:

| Flag | Git flag |
| --- | --- |
| `--sign` | `-S` |
| `--no-sign` | `--no-gpg-sign` |

`--sign` and `--no-sign` are mutually exclusive.

---

## Error Handling

All errors follow the Atmos error system: static sentinel errors in `errors/errors.go`, wrapped with `%w`, checked with `errors.Is()`, and surfaced through the error builder with hints, context, and exit codes. Git subprocess failures must never surface as raw `exit status 128` — the service classifies common Git failures into named sentinels:

| Failure | Sentinel | Exit code | Hints (each self-contained, separate `WithHint()`) |
| --- | --- | --- | --- |
| Unknown repository name | `ErrGitRepositoryNotFound` | 2 | List configured names; suggest `atmos git list` |
| Authentication failure (clone/pull/push) | `ErrGitAuthFailed` | 1 | Three separate hints: set `auth.identity` in `atmos.yaml`; configure a `github/sts` integration for CI; override with `--identity=<name>` |
| Non-fast-forward push after retries exhausted | `ErrGitPushRejected` | 1 | Two separate hints: remote branch is receiving concurrent pushes (serialize publishers); batching is planned future work |
| Dirty files outside managed paths on commit | `ErrGitDirtyUnmanagedFiles` | 2 | Commit/stash the listed files or adjust `--path` |
| Path escapes worktree | `ErrGitPathEscapesWorktree` | 2 | Show the rendered path in context |
| Hook not configured for `hooks run` | `ErrGitHookNotConfigured` | 2 | List hooks configured under `git.hooks` |
| No-arg clone outside CI | `ErrGitRepositoryRequired` | 2 | Name a repository, or enable `ci.enabled: true` in CI |
| Unclassified git subprocess failure | existing `ErrGitCommandExited` (fallback; do not duplicate) | 1 | — |

Exit codes follow the established mapping: `2` for config/usage errors, `1` (default) for runtime/external failures.

Additional rules:

1. **Retry internals stay internal:** each push-retry step wraps its own low-level errors, but exhaustion surfaces `ErrGitPushRejected` so callers can `errors.Is()` it without walking retry internals.
2. **No-op is not an error:** a commit with nothing to commit returns `(CommitResult{Committed: false}, nil)` — no `ErrNoChanges`-style sentinel. Callers (the `git` hook kind, Kubernetes flow) branch on the structured result.
3. **`hooks run` exit codes:** the delegated command's exit code is extracted from the wrapped `exec.ExitError` via the standard `GetExitCode()` path; the implementation must not stack `WithExitCode()` on top, which would override the subprocess code.
4. **`core.hooksPath` during `hooks install`** is a `ui.Warning()` from `pkg/ui` — no error is built, no exit code is set, install succeeds.
5. **Secret masking of Git stderr:** the masking pipeline wraps the I/O writers; it does NOT retroactively scan strings embedded in error messages. The CLI provider therefore streams Git subprocess stderr to a masked `pkg/io` writer at write time, and returns only the exit status plus a classified sentinel — raw stderr is not embedded in the error chain. (If any captured output must be included in an error, it is explicitly masked before wrapping.)
6. New sentinels live in the existing `// Git-related errors` block in `errors/errors.go`; `ErrGitAuthFailed` is intentionally distinct from the general `ErrAuthenticationFailed` so Git auth failures are distinguishable under `errors.Is()`.

---

## Native CI Behavior

When `ci.enabled: true` and a CI provider is detected, `atmos git clone` may infer clone inputs from CI metadata.

CI detection and metadata use the **existing provider-agnostic `pkg/ci` interface** (`Provider.Detect()` / `Provider.Context()`), which already exposes `Repository`, `Ref`, `SHA`, and pull request metadata, with a `generic` fallback provider. No GitHub-specific detection path is added; GitHub Actions is simply the first provider with full metadata.

This enables workflows where Atmos replaces a GitHub clone/checkout action pattern:

```yaml
ci:
  enabled: true

git:
  repositories:
    flux-deploy:
      uri: https://github.com/acme/flux-deploy.git
      auth:
        identity: platform-admin
```

Expected CI usage:

```bash
atmos git clone flux-deploy
```

For current-repository checkout replacement, no-arg clone can infer repository metadata only when:

1. `ci.enabled: true`.
2. The CI provider is detected.
3. Required provider metadata is present.

No-arg clone targets the **working directory** (e.g., `GITHUB_WORKSPACE`), exactly like `actions/checkout` — subsequent Atmos commands expect the repository at CWD.

If `ci.enabled` is false, no-arg clone fails with `ErrGitRepositoryRequired`.

GitHub STS should be documented as the preferred native CI credential path for private GitHub repositories.

### Native CI Clone Lifecycle

The end-to-end job flow, composing native CI, the CI cache, Auth/STS, and Git (cross-reference the native-ci and CI cache PRDs):

1. **Job starts.** CI cache auto-restore (PersistentPreRun) restores the Atmos XDG cache root — toolchain binaries *and* managed-repo Git workdirs come back from the previous run.
2. **`atmos git clone`** (no-arg) puts the current repository in the workspace. Fetch-depth guidance applies when `atmos describe affected` will run against this clone.
3. **`atmos git clone --all`** (or per-name clone / `provision.git`) **reconciles** restored XDG workdirs: a cache hit means fetching the delta instead of a full clone; a cache miss means a fresh clone. Correctness is identical either way — reconcile makes clone behavior independent of cache freshness, which is exactly what makes Git workdirs safe under `restore_keys` fallback hits.
4. **First Git subprocess** triggers the ambient broker / `EnsureIdentityEnvironment` — GitHub STS materializes `GIT_CONFIG_*` credentials.
5. **Operations run:** render, publish, commit, push (with retry).
6. **Command end:** CI cache pending-save archives the XDG root for the next run.

A minimal GitHub Actions job replacing `actions/checkout` + `actions/cache` glue:

```yaml
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: cloudposse/github-action-setup-atmos@v2
      - run: atmos git clone            # checkout replacement (workspace)
      - run: atmos git clone --all      # reconcile all deployment repos (XDG, cache-warm)
      - run: atmos kubernetes deploy argocd -s prod   # render + publish via hooks
```

(Cache restore/save and STS credential minting happen inside the Atmos commands; no `actions/checkout`, `actions/cache`, or token-wiring steps.)

**Cache churn:** Git workdirs change on every publish, so they churn cache archives. This is safe — workdirs are regenerable and always reconciled. Users who want lean archives can scope `ci.cache.paths` to exclude `git/repositories/`, trading cache hits for full clones. `atmos git list --check-status` shows what is currently materialized under the cache root.

---

## Kubernetes Deployment Repository Provisioning

The Kubernetes component already has a render pipeline (`render.output.path`, `render.output.split`) and lifecycle events (`before/after.kubernetes.render|diff|apply|delete`). Git deployment-repo publishing **composes with** that pipeline rather than defining a parallel one: `provision.git` directs render output into a managed repository worktree, and a `git` hook publishes it.

```yaml
components:
  kubernetes:
    argocd:
      provision:
        git:
          enabled: true
          repository: flux-deploy
          path: clusters/{{ .vars.cluster }}/argocd
          mode: sync   # sync | additive; sync is the default for rendered manifests
      hooks:
        publish:
          events:
            - after.kubernetes.render
          kind: git
          repository: flux-deploy
          commit:
            message: "Render {{ .component }} for {{ .stack }}"
          push: true
```

`repository` references a top-level `git.repositories.<name>` entry. V1 requires named repositories; inline repository configuration is not supported.

The Kubernetes flow:

1. Resolve component and stack configuration.
2. Provision component source if needed.
3. Reconcile the managed repository workdir (`provision.git`).
4. Generate files if configured.
5. Render Kubernetes manifests into `<workdir>/<path>` via the existing render output pipeline.
6. Run Git operations (via hooks or explicit commands) only after manifest generation succeeds.

### Prune Semantics

Rendering must handle removals: when a manifest disappears from the render output, additive copying leaves the stale file in the deployment repo and Argo CD/Flux keep applying it indefinitely.

| Mode | Behavior |
| --- | --- |
| `sync` (default for rendered manifests) | The managed `path` is made to exactly mirror the render output: new files added, changed files updated, removed files **deleted** |
| `additive` | Files are only added or updated; nothing is deleted |

Sync deletions are confined to the configured `path`; nothing outside the managed path is ever touched.

### Explicit Git Operations

Explicit per-component Git operations remain available for imperative use:

```bash
atmos kubernetes git pull argocd -s prod
atmos kubernetes git diff argocd -s prod
atmos kubernetes git commit argocd -s prod
atmos kubernetes git push argocd -s prod
```

`atmos kubernetes git diff` renders manifests and diffs them against the deployment repository without committing — the pull-request preview workflow.

**Ownership:** the `git` subcommand group under `kubernetes` is owned by `cmd/kubernetes` (the Kubernetes component branch), not injected from `cmd/git` — the registry alias mechanism resolves parent commands at registration time, so `cmd/git` cannot attach subcommands to a command group that may not exist. `cmd/kubernetes` depends on the shared `pkg/git` service only. This makes the Kubernetes component branch an explicit prerequisite for Phase 5.

> **Design note:** an earlier draft proposed `--git-op pull|commit|push|commit-push` flags on `render`/`apply`/`deploy`. This is dropped: event-bound publishing is the `git` hook kind's job (less new surface, inherits `--skip-hooks` and `on_failure`), and imperative publishing is the explicit subcommands' job.

---

## Terraform Lifecycle Git Publishing

Terraform components publish generated artifacts via the `git` hook kind on existing lifecycle events. The default target is the **current repository** — the repo the component already lives in — matching the common case of committing generated files back to the working repo:

```yaml
components:
  terraform:
    app:
      hooks:
        publish-artifacts:
          events:
            - after.terraform.apply
          kind: git
          # repository omitted -> the current repository
          commit:
            message: "Update generated artifacts for {{ .component }} in {{ .stack }}"
            paths:
              - generated/{{ .stack }}/{{ .component }}
          push: true
```

Setting `repository: <name>` targets a managed repository instead (with `provision.git` ensuring its workdir), for cases where generated artifacts belong in a separate artifact repo:

```yaml
components:
  terraform:
    app:
      provision:
        git:
          enabled: true
          repository: generated-terraform
      hooks:
        publish-artifacts:
          events:
            - after.terraform.apply
          kind: git
          repository: generated-terraform
          commit:
            message: "Update generated artifacts for {{ .component }} in {{ .stack }}"
          push: true
```

The `git` hook kind is a standard `pkg/hooks` kind:

1. It self-registers in the hooks kind registry alongside `store`, `command`, and the tool kinds.
2. It binds to existing events (`after.terraform.apply`, `after.kubernetes.render`, ...); no new event mechanism is introduced.
3. It inherits hooks semantics: `--skip-hooks` / `ATMOS_SKIP_HOOKS`, `on_failure: warn|fail|ignore`, toolchain/env setup.
4. All Git operations go through the shared Git service (safety rules, auth env, push retry included).

This replaces the earlier draft's `provision.git.event` field, which duplicated the hooks event mechanism. `provision.git` has no events; hooks have no clone/reconcile responsibility.

Future lifecycle events can be supported with zero Git-side changes — any event the hooks system dispatches can carry a `git` hook.

### Batch Publishing (Future)

A DAG run across many components publishing to the same repository should not produce N commits and N racing pushes. A future batch mode stages per-component changes and performs a single commit/push at the end of the run. V1 relies on push retry for correctness; batching is an optimization noted here so the v1 design does not preclude it.

---

## Artifact Paths

`provision.git.path` is optional, but it is the explicit opt-in for writing into a repository.

When `path` is set, Atmos writes artifacts into that path inside the managed repository worktree.

When `path` is omitted, artifacts go **purely to XDG**: a deterministic location under the Atmos XDG cache root via the existing `pkg/xdg` helpers:

```text
$XDG_CACHE_HOME/atmos/git/artifacts/<component-type>/<stack>/<component>
```

with the standard `pkg/xdg` fallbacks and overrides. No repository-relative default is invented, and nothing is written into a repository worktree — repository publishing always requires an explicit `path`. Commit operations against a repository with no configured `path` and no changes inside the worktree no-op cleanly.

Atmos must never attempt to commit files outside a Git worktree; artifacts staged in XDG are only ever copied into a worktree when an explicit `path` directs them there.

Note for documentation: Flux and Argo CD installations have established path conventions (`clusters/<cluster>/...`); `path` is always explicit for deployment-repo use cases.

---

## Documentation Requirements

Add documentation for:

1. `atmos git`
2. `atmos git hooks`
3. The `git` hook kind
4. Kubernetes deployment repository provisioning
5. GitHub STS and Atmos Auth integration for cloning deployment repositories in native CI

Update Kubernetes docs to explain how Git deployment repositories relate to Atmos Auth integrations such as EKS. Do not position "no kubectl/kustomize binary" as the entire Kubernetes value proposition. The main value is integrated rendering, GitOps artifact publishing, Auth, CI, and Atmos component lifecycle orchestration.

Command docs and config docs should cross-link following existing documentation style.

---

## Implementation Roadmap

### Phase 1: PRD and Schema Design

1. Write this PRD.
2. Add Go schema structs for top-level `git`.
3. Add component `provision.git` schema support and `git` hook kind schema support.
4. Update JSON schemas.
5. Update LSP completion, hover, and diagnostics where existing schema patterns require it.

### Phase 2: Git Service and CLI Commands

1. Extend `pkg/git` with the service and provider registry (coexisting with the existing go-git-based package).
2. Implement the CLI provider with a fakeable runner.
3. Add config resolution for named repositories, automatic XDG workdirs, commit author resolution, and push retry.
4. Add `cmd/git` via the command registry pattern under a new Git command group; all command-specific flags use `flags.NewStandardParser()` with `BindToViper` in `init()` and `BindFlagsToViper` in `RunE`, env vars under the `ATMOS_GIT_` prefix.
5. Add `atmos git list` on the `pkg/list` rendering pipeline, with the `atmos list git-repositories` alias.
6. Add command examples and embedded markdown usage.

### Phase 3: Local Git Hook Shims

1. Add `atmos git hooks install` / `uninstall`.
2. Add `atmos git hooks run` (dispatching through `workflow.CommandRunner`, forwarding args and stdin).
3. Implement overwrite protection, `--force`, and the `core.hooksPath` warning.
4. Keep local Git hooks separate from Atmos lifecycle events.

### Phase 4: `git` Hook Kind

1. Register the `git` kind in the hooks kind registry.
2. Implement commit/push (and pull) operations through the shared Git service, current-repo default, managed-repo by name.
3. Honor `--skip-hooks` and `on_failure` semantics.
4. Wire provenance trailers.

### Phase 5: Kubernetes Deployment Repository Integration

1. Add Kubernetes `provision.git` config support (registered via the provisioner registry with the standard `(atmosConfig, componentSections, authContext)` signature).
2. Direct render output into managed repository paths via the existing render pipeline, with `sync`/`additive` modes.
3. Add `atmos kubernetes git pull|diff|commit|push`.
4. Ensure Git operations run only after generation succeeds.

### Phase 6: Docs, Examples, and Native CI Guidance

1. Add user docs.
2. Add examples for Argo CD, Flux, and Terraform generated artifacts.
3. Add native CI GitHub STS examples and fetch-depth guidance for `describe affected`.
4. Cross-link relevant command and config docs.

---

## Testing Strategy

### Unit Tests

1. Test Git CLI command construction with a fake runner.
2. Test signing flag behavior by asserting arguments.
3. Test commit author flag injection (and absence when Git already resolves an author).
4. Test path-scoped commit dirty-tree validation and path traversal rejection.
5. Test config default resolution, including automatic XDG workdir generation.
6. Test push retry loop: rejected push → ff pull → re-push, bounded by `push.retries`.
7. Test identity env merge with a fake Auth manager returning `GIT_CONFIG_*`.
8. Test name-vs-path argument resolution.
9. Test error classification: each failure in the Error Handling table maps to its sentinel (checked with `errors.Is()`) and exit code; stderr is streamed to a masked writer, not embedded in error chains.
10. Test `atmos git list` extraction (status resolved at extraction time; `--check-status` gating; default sort `name:asc`).
11. Test clone argument parsing: `git::` prefix stripping, `?ref=`/`?depth=` mapping, flag > query param > config precedence, unknown query params rejected, scp-style URL detection.
12. Test `--all` semantics: mutual exclusion with positional args, attempt-all with `errors.Join` aggregation (one failing repo does not block the rest), per-repo identity resolution.
13. Test CI lifecycle reconcile: simulate a restored stale workdir → clone reconciles to the expected ref; snapshot test for no-arg clone outside CI (`ErrGitRepositoryRequired`).

### Integration-Style Tests

Use temporary local bare repositories for:

1. Clone (including reconcile of an existing/stale workdir)
2. Pull
3. Status
4. Diff
5. Commit (including provenance trailers)
6. Push (including non-fast-forward rejection and retry)

These tests should not require external network access.

### Hook Tests

1. `hooks install` refuses to overwrite existing hooks.
2. `hooks install --force` overwrites existing hooks.
3. `hooks install` warns when `core.hooksPath` is set.
4. `hooks uninstall` removes only Atmos-generated shims.
5. Generated hook scripts are executable and delegate to `atmos git hooks run <hook> "$@"`.
6. `hooks run` forwards args and stdin to the configured command and propagates exit codes.
7. Missing hook config fails clearly.

### `git` Hook Kind Tests

1. Kind registers and dispatches on `after.terraform.apply`.
2. Defaults to the current repository; targets a managed repository when `repository` is set.
3. Honors `--skip-hooks` and `on_failure`.
4. No-ops cleanly when there are no changes.

### Kubernetes Tests

1. Render output lands in the managed repository path before commit.
2. `sync` mode deletes removed manifests within the managed path only; `additive` mode does not delete.
3. Git operations do not run when manifest generation fails.
4. Named repository config is resolved from top-level `git.repositories`.
5. `atmos kubernetes git diff` produces a diff without committing.

### Focused Test Runs

Run focused tests for:

1. `pkg/git`
2. `cmd/git`
3. `pkg/hooks` (git kind)
4. Kubernetes command tests
5. Kubernetes provider tests
6. Schema tests
7. LSP schema/completion tests touched by the change
8. Affected/all DAG tests touched by Kubernetes work

---

## Acceptance Criteria

1. The PRD clearly establishes Git as a foundational Atmos platform capability, in scope with Toolchain, Auth, and Hooks.
2. The PRD cleanly separates the two repository targets: managed deployment repositories (`provision.git`) and the current repository (`git` hook kind), both on one shared service.
3. The PRD explains Git artifact repository use cases for Kubernetes, Terraform, CI, hooks, and future provisioners.
4. The PRD includes final config shapes and command surfaces, using provider (not driver) terminology consistent with the rest of Atmos.
5. The PRD captures all defaults and safety rules, including push contention, commit author, prune semantics, and workdir reconciliation.
6. The PRD distinguishes local Git hooks, the `git` hook kind, and Git provisioning.
7. The PRD is detailed enough for another engineer to implement without re-deciding scope.

---

## Resolved Questions

1. **Inline repository config for `provision.git`?** No. V1 requires named repositories under `git.repositories`.
2. **Default path when `provision.git.path` is omitted?** Purely XDG: artifacts stage to `$XDG_CACHE_HOME/atmos/git/artifacts/<component-type>/<stack>/<component>`. There is no repository-relative default; writing into a repository always requires an explicit `path`.
3. **GitHub-only or provider-agnostic CI detection for no-arg clone?** Provider-agnostic from day one: the existing `pkg/ci` provider interface already supplies the needed metadata with a generic fallback.
4. **How do Git hook commands execute?** Local Git hook shims dispatch through the existing `workflow.CommandRunner`; lifecycle-bound Git operations are a `pkg/hooks` kind using the existing hooks engine. No new execution mechanism is introduced.

## Open Questions

1. Exact field schema for the `git` hook kind (`commit.paths` vs reusing `provision.git.path`; whether `pull` is exposed as a hook operation in v1).
2. Batch publishing design: where per-run staging state lives and how it interacts with push retry.
3. PR-based publish flow design for the future `github` provider (branch naming, PR templates, auto-merge policy).
4. Whether `commit --all` / `push --all` ever make sense at the CLI layer, or whether bulk publishing remains exclusively the hooks/provisioner layer's job (current position: the latter).
