# Fix: native Helm UX fixes (repo isolation, status output, default identity, namespace)

**Date:** 2026-08-14
**Area:** `pkg/component/helm` (experimental native Helm components: `atmos helm template/diff/apply/delete`)

## Summary

Four related usability/correctness issues in the experimental native Helm implementation, all
surfaced while deploying real charts to an AKS cluster. They are grouped in one change because they
share a common shape: Atmos was configuring the Helm *action* objects but not the Helm `EnvSettings`
that Helm actually reads for repository resolution, namespacing, and identity. Each fix is small and
backward compatible.

1. Repository config/cache inherited (and mutated) the user's global Helm config.
2. `atmos helm apply` / `delete` succeeded silently (no status output).
3. `atmos helm` ignored the stack's default-identity binding, forcing an explicit `--identity`.
4. Charts whose manifests omit `metadata.namespace` installed into the kubeconfig-default namespace.

---

## Issue 1: repository config/cache inherited the user's global Helm config

### Root cause

`newSettings` was `cli.New`, so Helm's `EnvSettings.RepositoryConfig` / `RepositoryCache` defaulted to
the user's global Helm config (`~/.config/helm/repositories.yaml`, `~/Library/Caches/helm/...`, etc.).
Two consequences:

- When resolving a declared `repo/name` chart, Atmos sets `ChartPathOptions.RepoURL`, which sends Helm
  down `downloader.(*ChartDownloader).scanReposForURL`. That function iterates **every** repository in
  the user's global `repositories.yaml` and `LoadIndexFile`s each one, failing on the first repo whose
  index is not cached (e.g. a stale `bitnami` entry: `no cached repo found ... bitnami-index.yaml`).
  A completely unrelated repository in the user's global config would break an Atmos chart render.
- `setupHelmRepositories` writes the components' declared repositories into that same global config,
  mutating the user's personal Helm setup.

### Fix

`newSettings` is now `defaultSettings`, which calls `cli.New()` and then `isolateRepositoryConfig`:
when the user has **not** set `HELM_REPOSITORY_CONFIG` / `HELM_REPOSITORY_CACHE`, the config and cache
are pointed at an Atmos-managed XDG location (`<xdg-config>/atmos/helm/repositories.yaml`,
`<xdg-cache>/atmos/helm/repository`) via `pkg/xdg`. This mirrors how Atmos already isolates the
kubeconfig (`pkg/auth/cloud/kube`). Explicit `HELM_REPOSITORY_*` env vars are respected unchanged.

Result: chart resolution depends only on the repositories the components declare, is reproducible on
any workstation/CI, and never touches the user's global Helm config.

## Issue 2: `apply` / `delete` succeeded silently

### Root cause

`runOperation` builds a summary map for apply/delete but only returns it (it feeds the CI job summary).
Nothing was written to the terminal, so a successful `atmos helm apply`/`delete` printed nothing and
the user had to run `kubectl` to confirm anything happened. (`template` and `diff` print their own
output.)

### Fix

`runOperation` now calls `emitOperationStatus` after a successful apply/delete, which writes a
one-line summary via `data.Writeln` (the same output path `diff` uses): `Applied Helm release "X" to
namespace "Y" (chart Z)` / `Deleted Helm release "X" from namespace "Y"`. `formatOperationStatus`
builds the message and returns empty for operations that need no status line, so nothing is emitted on
error or for template/diff.

## Issue 3: `atmos helm` ignored the stack default-identity binding

### Root cause

`processStacksWithAuth` set up component auth only `if info.Identity != ""`. With no `--identity`,
no auth manager was created and `applyAuthEnvironment` returned a no-op, so no `KUBECONFIG` was
injected and the command could not reach the cluster. Unlike `atmos terraform` /
`atmos helmfile` (which call `authManager.GetDefaultIdentity`/`storeAutoDetectedIdentity` via
`setupTerraformAuth`), `atmos helm` never resolved the stack's `default: true` identity, forcing an
explicit `--identity` on every cluster command.

### Fix

`processStacksWithAuth` now takes the `operation` and calls the shared `setupComponentAuthForCLI`
(already aliased to `internal/exec.SetupComponentAuthForCLI` -> `setupTerraformAuth`) whenever
`shouldSetupComponentAuth` is true: for an explicit identity, or for any cluster operation
(`operationRequiresCluster`: everything except `template`). `setupTerraformAuth` auto-detects the
stack default identity into `info.Identity`, so `atmos helm apply/diff/delete` honor the per-stack
`auth.identities: { <id>: { default: true } }` binding with no `--identity`, exactly like terraform.

The offline `template` render never triggers auth (stays fully offline). When no auth is configured,
`setupComponentAuthForCLI` returns a nil manager and the identity stays empty, so the ambient
`KUBECONFIG` is used, preserving prior behavior.

## Issue 4: namespace-less charts installed into the kubeconfig-default namespace

### Root cause

For a chart whose manifests omit `metadata.namespace`, Helm assigns the namespace from the kube
client's `RESTClientGetter`, which derives it from `EnvSettings.Namespace()`. `newActionContext`
passed the target namespace to `cfg.Init` (the storage driver) and `installRelease` set
`action.Install.Namespace`, but the `EnvSettings` namespace was never set, so it stayed at the
kubeconfig context default (usually `default`). Charts that hardcode `namespace: {{ .Release.Namespace }}`
worked; charts that do not (e.g. the public Ealenn `echo-server`) landed in `default`.

### Fix

`newActionContext` now calls `settings.SetNamespace(namespace)` before building the
`RESTClientGetter`, so namespace-less manifests install into the component's configured namespace.
An empty namespace leaves Helm's own default untouched.

---

## Backward compatibility

- No config or public API changes. Explicit `HELM_REPOSITORY_CONFIG`/`HELM_REPOSITORY_CACHE` and
  `--identity`/`ATMOS_IDENTITY` continue to win.
- `template` remains fully offline (no auth, no cluster).
- Components with no auth configured continue to use the ambient `KUBECONFIG`.
- Charts that already set an explicit namespace are unaffected.

## Tests

All in `pkg/component/helm` (new functions covered 100%; package total ~89.6%):

- `repo_isolation_test.go` - `TestNewSettings_IsolatesRepositoryConfigWhenEnvUnset`,
  `TestNewSettings_RespectsExplicitRepositoryEnv`.
- `status_output_test.go` - `TestEmitOperationStatus` (writes for apply/delete on success only; silent
  on error / template / diff), `TestFormatOperationStatus`.
- `default_identity_test.go` - `TestShouldSetupComponentAuth`, `TestOperationRequiresCluster`;
  plus `executor_extra_test.go` asserts a cluster op resolves auth with no explicit identity.
- `namespace_test.go` - `TestNewActionContext_SetsSettingsNamespace`,
  `TestNewActionContext_EmptyNamespaceKeepsDefault`.

`TestMain` initializes the `data` writer once for the package (apply/delete now emit via `data.Writeln`).

```shell
go test ./pkg/component/helm/... -count=1
```

## Expected behavior

- Native Helm no longer breaks on unrelated repos in the user's global Helm config, and no longer
  mutates that config.
- `atmos helm apply`/`delete` print a status line on success.
- `atmos helm apply/diff/delete <component> -s <stack>` work without `--identity` when the stack binds
  a default identity.
- Charts without an explicit namespace install into the component's configured `namespace`.
