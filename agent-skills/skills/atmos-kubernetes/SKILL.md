---
name: atmos-kubernetes
description: "Native Kubernetes components (experimental): render/plan/diff/apply/deploy/delete/validate via Kubernetes Go SDK server-side apply, components.kubernetes, kubectl/kustomize providers, paths/manifests, provision targets (cluster vs. GitOps repo), and auth"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Native Kubernetes Components

Use this skill for the **native Kubernetes** component type (`components.kubernetes`). It manages
plain YAML/JSON manifests and Kustomize overlays through the **Kubernetes Go SDK** with server-side
apply. No `kubectl` or `kustomize` binary is required. This is distinct from Helm chart deployment —
see [atmos-helm](../atmos-helm/SKILL.md) for charts, or [atmos-helmfile](../atmos-helmfile/SKILL.md)
for Helmfile-based releases.

This feature is **experimental** (`IsExperimental() == true` in `cmd/kubernetes/kubernetes.go`).
`kubectl`/`kustomize` names describe manifest-processing *behavior*, not the CLI binaries.

## Related Skills

| Need | Load |
|---|---|
| Helm charts (native, Helm Go SDK) | [atmos-helm](../atmos-helm/SKILL.md) |
| Helmfile-based Kubernetes deployments | [atmos-helmfile](../atmos-helmfile/SKILL.md) |
| Component dependency ordering for `--all`/`--affected` | [atmos-components](../atmos-components/SKILL.md) |
| GitOps delivery targets (`provision.targets`, `kind: git`) | [atmos-git](../atmos-git/SKILL.md) |
| EKS kubeconfig / cluster authentication | [atmos-aws-eks](../atmos-aws-eks/SKILL.md), [atmos-auth](../atmos-auth/SKILL.md) |
| Lifecycle hooks around component operations | [atmos-hooks](../atmos-hooks/SKILL.md) |
| Native CI job summaries | [atmos-ci](../atmos-ci/SKILL.md) |
| Local Kubernetes emulator (k3s) | [atmos-emulator](../atmos-emulator/SKILL.md) |

## Component Shape

Define Kubernetes objects under `components.kubernetes` in stack manifests:

```yaml
components:
  kubernetes:
    argocd:
      provider: kustomize
      paths:
        - overlays/{{ .vars.cluster }}
      vars:
        namespace: argocd
        cluster: dev
      env:
        KUBECONFIG: /tmp/kubeconfig
      manifests:
        - apiVersion: v1
          kind: Namespace
          metadata:
            name: "{{ .vars.namespace }}"
      provision:
        default: cluster
        targets:
          cluster:
            kind: kubernetes
          deployment-repo:
            kind: git
            repository: deployments
            path: "clusters/{{ .vars.cluster }}/argocd"
```

Kubernetes components use the same stack sections as other component types — `vars`, `env`, `auth`,
`metadata`, `settings`, `dependencies`, `hooks`, `generate`, `source`/`provision`, inheritance, and
overrides. Kubernetes components do **not** support `command` — provider names describe manifest
behavior, and Atmos never shells out to `kubectl`/`kustomize`.

| Field | Purpose |
|---|---|
| `provider` | `kubectl` (plain YAML/JSON) or `kustomize` (Kustomize overlays). Defaults to `atmos.yaml` `components.kubernetes.provider` (default `kubectl`). |
| `paths` | Files or directories relative to the component directory. Directories are walked recursively for `.yaml`/`.yml`/`.json`. With `provider: kustomize`, a directory containing a Kustomize file is rendered as a Kustomize root. |
| `manifests` | Inline Kubernetes objects (or YAML strings), merged with objects loaded from `paths`. |
| `render` | Default output for `atmos kubernetes render` (`output.path`, `output.split`). |
| `provision` | Delivery targets for `apply`/`deploy` — the cluster (default) or an external target such as a Git deployment repository. |

### Providers

| Provider | Behavior |
|---|---|
| `kubectl` | Loads plain YAML/JSON manifests from files, directories, and inline `manifests`. |
| `kustomize` | Renders Kustomize directories via the Kustomize Go API, then passes objects to the same Kubernetes clients. |

Both providers are Go-SDK-based; neither requires the matching CLI binary installed.

## Commands

| Command | Purpose |
|---|---|
| `atmos kubernetes render <component> -s <stack>` | Resolve stack config, run generation, load `manifests`, render provider inputs, write final YAML. Does not contact the API server. |
| `atmos kubernetes validate <component> -s <stack>` | Offline structural checks by default (`apiVersion`/`kind` present, `metadata.name` valid DNS-1123, resolvable GVK); `--server` adds a live server-side dry-run apply. Reports every invalid object, not just the first. |
| `atmos kubernetes diff <component> -s <stack>` | Server-side dry-run apply against the live object, normalizes volatile metadata, reports created/changed/no-change per object with a unified diff. `plan` is an alias. |
| `atmos kubernetes apply <component> -s <stack>` | Resolves each object GVK→GVR via discovery/RESTMapper, applies via the dynamic client with **server-side apply**, or delivers to a `--target` provision target. |
| `atmos kubernetes deploy <component> -s <stack>` | Alias for `apply`; use when automation language should read as an application deployment rather than an API operation. |
| `atmos kubernetes delete <component> -s <stack>` | Deletes the rendered objects from the cluster. |

All operation commands accept `--all`, `--affected` (with `--base`/`--ref`/`--sha`/`--repo-path`/
`--clone-target-ref`/`--ssh-key`/`--ssh-key-password`), and `--include-dependents`, matching
`atmos describe affected` semantics. `--all`/`--affected` are mutually exclusive with a positional
component argument. `kubernetes` aliases to `k8s`.

### render output

`render` writes multi-document YAML to stdout by default. `--output <file>` writes a single file;
`--output-dir <dir>` (with optional `--split`) writes one file per object (or `manifest.yaml` without
`--split`). These flags only work rendering a single component — set component-level `render.output`
for `--all`/`--affected` runs.

### validate

Runs the same offline structural checks automatically before `apply`/`deploy`, so malformed manifests
are rejected before anything reaches the cluster or a provision target. `--server` additionally
requires a reachable cluster/kubeconfig and surfaces schema errors and missing CRDs authoritatively.

### diff

`atmos kubernetes diff` does **not** shell out to `kubectl diff`. `Secret` objects are omitted from
diff output (and CI summaries) so their data is never printed or written anywhere.

## Provision Targets (GitOps delivery)

By default `apply`/`deploy` applies to the cluster (`kind: kubernetes`). A component can instead
publish rendered manifests to a Git deployment repository reconciled by Argo CD/Flux (`kind: git`):

```yaml
git:
  repositories:
    deployments:
      uri: https://github.com/acme/deployments.git
      branch: main

components:
  kubernetes:
    argocd:
      provision:
        default: cluster                 # used when --target is omitted
        targets:
          cluster:
            kind: kubernetes
          deployment-repo:
            kind: git
            repository: deployments      # references git.repositories.<name>
            path: "clusters/{{ .vars.cluster }}/argocd"
            auth:
              identity: platform-admin   # optional; else the repository default
            commit:
              message: "Render {{ .vars.app_name }} for {{ .vars.stage }}"
              signing: auto              # auto | always | never
```

```shell
atmos kubernetes deploy argocd -s plat-ue2-dev --target=deployment-repo
```

The git target clones (or fast-forwards), replaces the managed `path` with the rendered manifests,
commits with provenance trailers (`Atmos-Stack`, `Atmos-Component`), and pushes. Re-delivering
identical manifests is a clean no-op. Credentials come from Atmos Auth (GitHub STS) — never written
into the manifests. Pull-request publishing is not yet supported. See
[atmos-git](../atmos-git/SKILL.md) for the underlying mechanics.

## Auth

Kubernetes operations use whatever kubeconfig/client configuration is visible to the Kubernetes Go
client; Atmos Auth prepares that environment before the SDK client is created.

Local/ambient clusters:

```yaml
auth:
  identities:
    local-k3s:
      kind: ambient

components:
  kubernetes:
    app:
      env:
        KUBECONFIG: /path/to/kubeconfig
```

EKS clusters — chain an `aws/eks` integration off an AWS identity to resolve the cluster and write
kubeconfig before the Kubernetes command runs:

```yaml
auth:
  identities:
    platform-admin:
      kind: aws
  integrations:
    prod/eks:
      kind: aws/eks
      via:
        identity: platform-admin
      spec:
        cluster:
          name: acme-prod-eks
          region: us-east-1
          kubeconfig:
            path: /tmp/acme-prod-kubeconfig
            update: replace
```

See [atmos-aws-eks](../atmos-aws-eks/SKILL.md) for the full `aws/eks` integration contract.

## atmos.yaml Configuration

```yaml
components:
  kubernetes:
    base_path: components/kubernetes    # default: components/kubernetes
    provider: kubectl                   # default provider: kubectl | kustomize
    auto_generate_files: false          # render component `generate:` before operations
```

## Native CI Summaries

When `ci.enabled: true` and Atmos runs in a supported CI provider (e.g. GitHub Actions), Kubernetes
commands write a compact Markdown job summary (`$GITHUB_STEP_SUMMARY`) — summaries only (no
`$GITHUB_OUTPUT`, commit statuses, PR comments, or artifacts):

| Command | Summary content |
|---|---|
| `render` | Rendered objects. |
| `plan`/`diff` | Created/changed/no-change counts, plus a collapsible **Kubernetes Diff** block (per-object unified diff; `Secret` objects omitted). |
| `apply`/`deploy` | Applied or delivered objects. |
| `delete` | Deleted and not-found objects. |
| `validate` | Valid and invalid objects. |

## Hooks

Dotted lifecycle events: `before`/`after` × `kubernetes.{render,plan,diff,apply,deploy,delete,validate}`.
`plan` normalizes to the `diff` lifecycle and `deploy` normalizes to the `apply` lifecycle for hook
matching, so hooks written for either spelling match the equivalent operation.

```yaml
components:
  kubernetes:
    argocd:
      hooks:
        notify:
          events:
            - after.kubernetes.apply
          command: echo "applied argocd"
```

## Guidance

- Use `validate` (offline by default) before wiring `--server` dry-run checks that need a reachable
  cluster — it catches structural errors (bad `apiVersion`/`kind`, invalid `metadata.name`) for free.
- Use `dependencies.components` so `--all`/`--affected` apply objects across components in the right
  order.
- Use `provision.targets` with `kind: git` for GitOps delivery instead of ad hoc scripts that render
  and commit manifests manually.
- Prefer `kustomize` provider when component paths are Kustomize roots; use `kubectl` provider for
  plain manifest files/directories — both avoid needing the matching CLI binary installed.
- Remember `Secret` objects never appear in diff output or CI summaries — don't rely on `diff`/CI logs
  to review secret contents.
