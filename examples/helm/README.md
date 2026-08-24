---
title: Native Helm
tags: [Kubernetes]
cast:
  file: /casts/examples/helm/lifecycle.cast
  title: atmos native helm lifecycle
---

# Native Helm Component Example

A minimal, credential-free example of a native Helm component. The chart lives in
`components/helm/demo` and is configured by `stacks/deploy/dev.yaml`.

## Try It

Run the local chart workflow end to end:

```shell
atmos validate stacks
atmos helm template demo -s dev
atmos emulator up kubernetes -s dev
atmos helm diff demo -s dev --identity local-k3s
atmos helm apply demo -s dev --identity local-k3s
atmos emulator exec kubernetes -s dev -- kubectl -n demo get deployment demo
atmos emulator exec kubernetes -s dev -- kubectl -n demo get service demo
atmos helm delete demo -s dev --identity local-k3s
atmos emulator down kubernetes -s dev
```

The `atmos test` custom command remains available as the CI smoke wrapper for
this same lifecycle.

## Render (no cluster, no credentials)

```shell
atmos helm template demo -s dev

# Render the same chart through a declarative Helm repository.
HELM_DEMO_REPO_URL=http://127.0.0.1:8080 atmos helm template demo-repo -s dev
```

## Diff

`atmos helm diff` (alias `plan`) shows a real unified diff (via the embedded
[helm-diff](https://github.com/databus23/helm-diff) library — no plugin to install).
Secret values are redacted.

```shell
# Offline: capture a baseline render, change a value, then diff against the baseline.
atmos helm template demo -s dev --output=baseline.yaml
# (edit stacks/deploy/dev.yaml, e.g. set replicaCount: 3)
atmos helm diff demo -s dev --from-manifest=baseline.yaml

# Against the deployed release (requires a reachable cluster):
atmos helm diff demo -s dev

# Against a GitOps deployment repository configured under `provision`:
atmos helm diff demo -s dev --against=target
```

## Apply

```shell
# Install/upgrade the release on the current kubecontext.
atmos helm apply demo -s dev

# Or deploy directly to the local K3s emulator used by the CI smoke test.
atmos emulator up kubernetes -s dev
atmos helm apply demo -s dev --identity local-k3s
```

## Helm Repositories

The `demo-repo` component shows the declarative Helm repository path:

```yaml
components:
  helm:
    demo-repo:
      repositories:
        - name: local
          url: !env HELM_DEMO_REPO_URL
      chart: local/demo
```

Use `atmos helm repo list` to inspect chart repository associations:

```shell
atmos helm repo list demo-repo -s dev
```

See the [`atmos helm`](https://atmos.tools/cli/commands/helm/usage) docs for the full
command and flag reference.
