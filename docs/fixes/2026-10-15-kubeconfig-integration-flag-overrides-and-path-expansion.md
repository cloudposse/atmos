# Fix: `update-kubeconfig --integration` honors `--kubeconfig`/`--alias`, and kubeconfig paths expand `~`/env vars

**Date:** 2026-10-15

## Summary

`atmos azure aks update-kubeconfig` and `atmos aws eks update-kubeconfig` advertise `--kubeconfig` and
`--alias` flags, but silently discarded them when run with `--integration`. The kubeconfig was always
written to the integration's configured path (or the default `~/.config/atmos/kube/config`), so running
the integration path for several clusters with distinct `--kubeconfig` values overwrote the same file and
only the last-written cluster remained reachable. The `KUBECONFIG`/`ATMOS_KUBECONFIG` env vars bound to
`--kubeconfig` were likewise ignored on that path.

Separately, a configured `spec.cluster.kubeconfig.path` (and the `--kubeconfig` flag) was used verbatim:
`~` and `${HOME}` were never expanded, so a committed, multi-developer integration config could not carry
a portable per-user path.

Both are fixed. The flag/env values now flow through the integration path as per-invocation overrides, and
kubeconfig paths are expanded (`~`, `$VAR`, `${VAR}`) at a single chokepoint.

## Context

- Reported in issue [#3386](https://github.com/cloudposse/atmos/issues/3386).
- The `--integration` branch of each command's `RunE` forwarded **only** the integration name to
  `AuthManager.ExecuteIntegration`, which built the integration from `auth.integrations.<name>` config and
  never saw the CLI flags. The direct (`--cluster-name`/`--identity`) path read the flags and worked.
- `gcp/gke` has no `update-kubeconfig` CLI command (only `gcp gke token`), so the flag-override half does
  not apply to GCP. The path-expansion half applies to all three cluster integrations (aws/eks, azure/aks,
  gcp/gke) because they share `kube.NewKubeconfigManager`.

## Changes

- `pkg/auth/types/interfaces.go`: added `IntegrationOverrides`, the `IntegrationOption` functional-option
  type, and `WithKubeconfigPath`/`WithAlias` builders (no-ops when empty). `ExecuteIntegration` now takes
  `opts ...IntegrationOption`. Regenerated `mock_interfaces.go` via `go generate`.
- `pkg/auth/manager_integrations.go`: `ExecuteIntegration` collects overrides and applies them through the
  new `applyIntegrationOverrides`, which clones only the `Spec`/`Cluster`/`Kubeconfig` branch it mutates so
  the shared `m.config` is never changed. Overrides apply only to cluster integrations; other kinds pass
  through untouched.
- `pkg/auth/deferred/manager.go`: delegates the variadic `opts...`.
- `pkg/auth/cloud/kube/config.go`: `NewKubeconfigManager` now expands a non-empty custom path via the new
  `expandKubeconfigPath` (env-var substitution with `os.ExpandEnv`, then a leading `~`/`~/` via
  `os.UserHomeDir`). Covers the integration config path, the `--kubeconfig` flag, and the
  `KUBECONFIG`/`ATMOS_KUBECONFIG` env vars, since all flow through here. The empty-path XDG default is
  unchanged.
- `cmd/azure/aks/update_kubeconfig.go` and `cmd/aws/eks/update_kubeconfig.go`: the `--integration` branch
  reads `--kubeconfig` from Viper (honoring its env bindings) and `--alias` from the flag, and forwards both
  as overrides.
- `cmd/azure/aks/update_kubeconfig_sdk.go` and `cmd/aws/eks/update_kubeconfig_sdk.go`: thread the overrides
  into `ExecuteIntegration`. Added the AKS-style `initCliConfig`/`newAuthManager` seams to the EKS SDK file
  so the forwarding is unit-testable without a live AWS account.
- Docs: `website/docs/cli/commands/azure/aks/update-kubeconfig.mdx` and
  `website/docs/cli/commands/aws/eks/update-kubeconfig.mdx` document the override behavior and path
  expansion.

## Validation

- `go build ./...` - clean.
- `go vet ./pkg/auth/... ./cmd/aws/eks/... ./cmd/azure/aks/... ./internal/exec/...` - clean.
- New tests:
  - `pkg/auth/cloud/kube/config_expand_test.go` - `~`, `$VAR`, `${VAR}`, unset-var, and default-path cases.
  - `pkg/auth/manager_integrations_overrides_test.go` - override application, kubeconfig-nil synthesis, and
    the CLAUDE.md both-directions source-isolation guard (result carries overrides; shared config untouched).
  - `cmd/aws/eks/update_kubeconfig_overrides_test.go` and an added case in
    `cmd/azure/aks/update_kubeconfig_sdk_test.go` - the `--integration` path forwards `--kubeconfig`/`--alias`.
- Updated the generic variadic-unaware contract test in `pkg/auth/deferred/manager_test.go` to invoke
  variadic methods with zero variadic args.
- `go test ./pkg/auth/... ./cmd/aws/... ./cmd/azure/... ./internal/exec/` - passing (see PR CI).
- `cd website && npm run build` - pending (run before pushing).

## Follow-ups

- Full Go-template support (e.g. `{{ .Env.HOME }}`) in `auth.integrations.*` config, suggested in the issue
  comment, is intentionally out of scope - `~`/`${HOME}` expansion already covers the portable-path need.
  Tracked in issue [#3386](https://github.com/cloudposse/atmos/issues/3386) for a future `minor` if demand
  warrants.
