---
name: atmos-helm
description: "Native Helm components (experimental): Helm Go SDK rendering/apply/delete, components.helm, chart sources (local/repo/OCI), values, repositories, helm plugin, provision targets, and how this differs from Helmfile"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Native Helm Components

Use this skill for the **native Helm** component type (`components.helm`). It deploys Helm charts —
local, remote-repository, or OCI — through the **Helm Go SDK**, in-process. No `helm` or `helmfile`
binary is required. This is a different component type than `components.helmfile`; see
[Native Helm vs. Helmfile](#native-helm-vs-helmfile) below before choosing one.

This feature is **experimental**.

## Related Skills

| Need | Load |
|---|---|
| Helmfile-based deployments (shells out to `helmfile`/`helm`) | [atmos-helmfile](../atmos-helmfile/SKILL.md) |
| Native Kubernetes manifests/Kustomize (no Helm charts) | [atmos-kubernetes](../atmos-kubernetes/SKILL.md) |
| Component dependency ordering for `--all`/`--affected` | [atmos-components](../atmos-components/SKILL.md) |
| Secret values in `values:` (`!secret`) | [atmos-secrets](../atmos-secrets/SKILL.md) |
| GitOps delivery targets (`provision.targets`, `kind: git`) | [atmos-git](../atmos-git/SKILL.md) |
| EKS/cluster authentication | [atmos-aws-eks](../atmos-aws-eks/SKILL.md), [atmos-auth](../atmos-auth/SKILL.md) |
| Native CI job summaries | [atmos-ci](../atmos-ci/SKILL.md) |

## Native Helm vs. Helmfile

| | Native Helm (`components.helm`) | Helmfile (`components.helmfile`) |
|---|---|---|
| Execution | Helm Go SDK, in-process | Shells out to the `helmfile` and `helm` binaries |
| Binaries required | None | `helmfile`, `helm` (plus any declared `helm plugin`s) |
| Multiple releases per component | One chart/release per component | One or more releases per `helmfile.yaml` |
| Diff engine | Embedded `helm-diff` library (no plugin install) | `helmfile diff` (needs `helm-diff` plugin, installed via `atmos helm plugin`) |
| Values | `values:`/`values_files:` merged through Atmos inheritance | Varfile generated from stack `vars:` |
| Status | Experimental | Stable |

Use **native Helm** for straightforward chart deployments where you want no external binaries and
first-class GitOps delivery targets. Use **Helmfile** ([atmos-helmfile](../atmos-helmfile/SKILL.md))
for existing `helmfile.yaml` projects, multi-release releases files, or `helm-secrets`/other Helm CLI
plugins. `atmos helm plugin` manages plugins **for Helmfile components** (native Helm does not run Helm
CLI subcommand plugins).

## Component Shape

Define Helm releases under `components.helm` in stack manifests:

```yaml
components:
  helm:
    monitoring:
      chart: prometheus-community/kube-prometheus-stack
      version: "65.1.1"
      repositories:
        - name: prometheus-community
          url: https://prometheus-community.github.io/helm-charts
      namespace: monitoring
      values:
        grafana:
          adminPassword: !secret grafana_admin_password
      dependencies:
        components:
          - cert-manager
      provision:
        default: cluster
        targets:
          cluster:
            kind: kubernetes
          deployment-repo:
            kind: git
            repository: deployments
            path: "clusters/{{ .vars.stage }}/monitoring"
```

Helm components use the same stack sections as other component types — `vars`, `env`, `auth`,
`metadata`, `settings`, `dependencies`, `hooks`, inheritance, and overrides — plus Helm-specific
fields:

| Field | Purpose |
|---|---|
| `chart` *(required)* | Local path (`.`, `./charts/app`), `repo/name` reference, bare name with `repository`, or `oci://` reference. |
| `version` | Chart version constraint (repository/OCI charts). |
| `repository` | Explicit HTTP chart repository URL for a bare `chart` name. |
| `repositories` | List of chart repositories used to resolve `repo/name` references (`name`, `url`, basic auth, TLS files, `pass_credentials_all`, `insecure_skip_tls_verify`). Merges with global `atmos.yaml` `components.helm.repositories`; component-level entries with the same `name` win. |
| `namespace` | Target Kubernetes namespace. Defaults to `default`. |
| `name` | Release name. Defaults to the component's last path segment. |
| `values` | The chart's values, merged through Atmos inheritance. This map **is** the values passed to the chart. |
| `values_files` | Value files layered *underneath* inline `values` (templated, in listed order). |
| `render` | Default output for `atmos helm template` (`output.path`, `output.split`). |
| `provision` | Delivery targets for `apply`/`deploy` — the cluster (default) or an external target such as a Git deployment repository. |

### Chart sources

- **Local chart** — path relative to the component directory (`chart: .`, `chart: ./charts/app`), or
  absolute.
- **Remote repository chart** — `repository: https://...` + `chart: <name>`, or a `repo/name`
  reference resolved against merged global/component `repositories:`. Atmos adds/updates these
  repositories in Helm's local repository config before chart operations.
- **OCI chart** — an `oci://` reference (e.g. `chart: oci://ghcr.io/acme/charts/app`).

### Values and secrets

The component `values:` map **is** the Helm values, merged through the normal Atmos import/inheritance
chain. `values_files:` overlay templated value files underneath the inline `values`. Helm has no
native secrets concept — Atmos provides it: secret values flow in through `!secret` and are masked
automatically wherever they'd otherwise be printed (e.g. in `atmos helm diff` output).

## Commands

| Command | Purpose |
|---|---|
| `atmos helm template <component> -s <stack>` | Render the chart to manifests via the Helm Go SDK (equivalent to `helm template`). No cluster or credentials needed. `render` is an alias. |
| `atmos helm diff <component> -s <stack>` | Real unified diff (embedded [helm-diff](https://github.com/databus23/helm-diff) library — no plugin install) against a baseline. `plan` is an alias. |
| `atmos helm apply <component> -s <stack>` | Install or upgrade the release (`helm upgrade --install`), or deliver to a `--target` provision target. |
| `atmos helm deploy <component> -s <stack>` | Alias for `apply`. |
| `atmos helm delete <component> -s <stack>` | Uninstall the release (`helm uninstall`). No-op if the release does not exist. |
| `atmos helm repo list [component] -s <stack>` | List declarative repository associations (global, component, or direct) and whether each is used by the resolved `chart`. |
| `atmos helm plugin list` / `atmos helm plugin install <plugin>...` | Manage Helm CLI plugins in the Atmos-managed `HELM_PLUGINS` directory — **for Helmfile components**, not native Helm. |

All operation commands (`template`, `diff`, `plan`, `apply`, `deploy`, `delete`) accept `--all`,
`--affected` (with `--base`/`--ref`/`--sha`/`--repo-path`/`--clone-target-ref`/`--ssh-key`/
`--ssh-key-password`), and `--include-dependents`, matching `atmos describe affected` semantics.
`--all`/`--affected` are mutually exclusive with a positional component argument.

### diff baselines

`atmos helm diff` (alias `plan`) compares the freshly rendered chart against one baseline, selected by
flag precedence `--from-manifest` → `--against` → deployed release:

| Baseline | Flag | Notes |
|---|---|---|
| Deployed release *(default)* | *(none)* | Reads the cluster; a nonexistent release shows every object as added. Only mode needing cluster access. |
| Local manifest | `--from-manifest=<path>` | Fully offline. |
| Provision target | `--against=target[:<name>]` | The manifests currently published in a non-cluster provision target (e.g. Git deployment repo) — offline, git access only. Without `:<name>` uses `provision.default`. |

`--context=<n>` controls unified-diff context lines (default `3`).

### template output

`atmos helm template` writes multi-document YAML to stdout by default. Use `--output <file>` for a
single file, or `--output-dir <dir>` (with optional `--split` for one file per object). `--output`/
`--output-dir` only work rendering a single component — configure `render.output` on the component for
`--all`/`--affected` runs.

## Provision Targets (GitOps delivery)

Like native Kubernetes components, `apply`/`deploy` can deliver rendered manifests to a **provision
target** instead of the cluster — e.g. committing them to a Git deployment repository reconciled by
Argo CD/Flux:

```yaml
components:
  helm:
    monitoring:
      provision:
        default: cluster
        targets:
          cluster:
            kind: kubernetes
          deployment-repo:
            kind: git
            repository: deployments
            path: "clusters/{{ .vars.stage }}/monitoring"
```

```shell
atmos helm deploy monitoring -s plat-ue2-dev --target deployment-repo
```

`--target` defaults to `provision.default`, otherwise the cluster. See
[atmos-git](../atmos-git/SKILL.md) for the underlying git target mechanics (clone/fast-forward,
provenance trailers, credentials from Atmos Auth).

## atmos.yaml Configuration

```yaml
components:
  helm:
    base_path: components/helm          # default: components/helm
    auto_generate_files: false          # render component `generate:` before operations
    repositories:                       # reusable chart repositories, referenced as repo-name/chart-name
      - name: prometheus-community
        url: https://prometheus-community.github.io/helm-charts
      - name: internal
        url: https://charts.example.com
        username: !env HELM_REPO_USERNAME
        password: !env HELM_REPO_PASSWORD
        pass_credentials_all: true
```

Repository fields: `name`/`url` *(required)*, `username`/`password` (basic auth),
`pass_credentials_all`, `cert_file`/`key_file`/`ca_file` (TLS), `insecure_skip_tls_verify`.
Component-level `repositories` entries override global entries with the same `name`.

## Native CI Summaries

When `ci.enabled: true` and CI is detected (or `--ci`/`ATMOS_CI` forces it), Helm commands write a
Markdown step summary through Atmos native CI — summaries only (no `$GITHUB_OUTPUT`, commit statuses,
PR comments, or artifacts):

| Command | Summary template |
|---|---|
| `template`, `render` | `ci.templates.helm.template` |
| `diff`, `plan` | `ci.templates.helm.diff` |
| `apply`, `deploy` | `ci.templates.helm.apply` |
| `delete`, `destroy` | `ci.templates.helm.delete` |

## Guidance

- Prefer native Helm for new chart deployments that don't need Helm CLI subcommand plugins; use
  Helmfile for existing `helmfile.yaml` projects or plugin-dependent workflows (`helm-secrets`, etc.).
- Use `atmos helm diff` before `apply`/`deploy` to review the real unified diff, not just a dry-run
  dump; secret values are redacted automatically.
- Use `dependencies.components` so `--all`/`--affected` runs install/upgrade releases in the right
  order — Helm itself has no cross-release dependency ordering.
- Use `!secret` for chart values that are sensitive (e.g. `adminPassword`) instead of plaintext in
  stack manifests.
- Use `provision.targets` with `kind: git` to publish rendered manifests to a GitOps deployment
  repository instead of applying directly to a cluster.
- Run `atmos helm repo list` to confirm which repository a component's `chart` resolves against
  before debugging chart-not-found errors.
