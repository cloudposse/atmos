# Fix: native Helm UX fixes (repo isolation, status output, default identity, namespace)

**Date:** 2026-08-14

## Summary

Four related usability/correctness fixes in the experimental native Helm implementation
(`pkg/component/helm`; `atmos helm template/diff/apply/delete`), all surfaced while deploying real charts
to an AKS cluster:

1. **Repository config/cache isolation** - stop inheriting (and mutating) the user's global Helm config.
2. **Status output** - `apply`/`delete` print a status line instead of succeeding silently.
3. **Default identity** - cluster operations resolve the stack's default identity, so `--identity` is no
   longer required.
4. **Namespace** - namespace-less charts install into the configured namespace, not the kubeconfig default.

Each fix is small and backward compatible. Fixes 1 and 4 share a root cause (Atmos set the Helm *action*
fields but not the Helm `EnvSettings`); fix 2 was missing output; fix 3 is in Atmos auth setup.

## Context

**1. Repo config inherited the global Helm config.** `newSettings` was `cli.New`, so
`EnvSettings.RepositoryConfig`/`RepositoryCache` defaulted to the user's global Helm config. Resolving a
declared `repo/name` chart sets `ChartPathOptions.RepoURL`, which sends Helm down
`downloader.(*ChartDownloader).scanReposForURL` - it iterates **every** repo in the user's global
`repositories.yaml` and fails on the first one whose index is not cached (e.g. a stale `bitnami` entry:
`no cached repo found ... bitnami-index.yaml`). `setupHelmRepositories` also wrote the declared repos into
that global config.

**2. Apply/delete were silent.** `runOperation` built a summary map for apply/delete but only returned it
(it feeds the CI job summary); nothing reached the terminal, so a successful `apply`/`delete` printed
nothing (`template`/`diff` print their own output).

**3. `--identity` was always required.** `processStacksWithAuth` set up component auth only when an
explicit identity was given, so without `--identity` no auth manager was created, no `KUBECONFIG` was
injected, and the command could not reach the cluster - unlike `atmos terraform`/`helmfile`, which resolve
the stack's `default: true` identity.

**4. Namespace-less charts landed in `default`.** Helm derives the namespace for namespace-less objects
from the settings/`RESTClientGetter`, which Atmos left at the kubeconfig context default. `newActionContext`
passed the namespace to `cfg.Init` and `installRelease` set `action.Install.Namespace`, but the
`EnvSettings` namespace was never set.

## Changes

All in `pkg/component/helm`:

1. **Isolation.** `newSettings` is now `defaultSettings` = `cli.New()` + `isolateRepositoryConfig`, which
   points `RepositoryConfig`/`RepositoryCache` at an Atmos-managed XDG location (`<xdg-config>/atmos/helm`,
   `<xdg-cache>/atmos/helm/repository`) unless `HELM_REPOSITORY_CONFIG`/`HELM_REPOSITORY_CACHE` is set.
   Mirrors the existing kubeconfig isolation; explicit env vars still win.
2. **Status output.** `runOperation` calls `emitOperationStatus` after a successful apply/delete, which
   writes a one-line status via `ui.Success` - the UI channel (stderr), per `docs/io-and-ui-output.md`; the
   data channel (stdout) is reserved for pipeable output. `ui.Success` renders markdown inline, so the
   release/namespace/chart are backticked and the chart note uses `((muted))` styling.
   `formatOperationStatus` returns empty for template/diff, so nothing is emitted there or on error.
3. **Default identity.** `processStacksWithAuth` takes the operation and calls `setupComponentAuthForCLI`
   when `shouldSetupComponentAuth` is true: for an explicit identity, or any cluster operation
   (`operationRequiresCluster`, an explicit allowlist of apply/diff/delete so an unsupported operation
   surfaces `ErrHelmUnsupportedOperation` instead of an auth error). `setupTerraformAuth` auto-detects the
   stack default identity. Template stays offline; a component with no auth keeps the ambient `KUBECONFIG`.
4. **Namespace.** `newActionContext` calls `settings.SetNamespace(namespace)` before building the
   `RESTClientGetter`, so namespace-less manifests install into the component's configured namespace. An
   empty namespace leaves Helm's own default untouched; charts with an explicit namespace are unaffected.

## Validation

- Automated (new tests in `pkg/component/helm`; new functions covered 100%, package total ~89.6%):
  `repo_isolation_test.go`, `status_output_test.go`, `default_identity_test.go` (incl. an unsupported-op
  case), `namespace_test.go`, plus an `executor_extra_test.go` case asserting a cluster op resolves auth
  with no explicit identity. Status output goes through `ui.Success`, which degrades gracefully when the UI
  formatter is uninitialized; `TestMain` initializes the `data` writer for the `diff` path.
- `go test ./pkg/component/helm/... -count=1` passes; `gofmt` and `go vet` clean; `go build ./...` OK.
- Manual: built a binary and deployed a local chart and a public-repo chart to a live AKS cluster -
  isolation removed the unrelated-global-repo failure and left the global config untouched; apply and
  delete printed their status line; both charts applied and deleted with no `--identity`; and the public
  chart (whose manifests set no namespace) installed into the configured namespace rather than `default`.

## Follow-ups

None.
