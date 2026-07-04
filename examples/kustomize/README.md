---
title: Kustomize
tags: [Kubernetes]
cast:
  file: /casts/examples/kustomize/lifecycle.cast
  title: atmos kustomize lifecycle
---

# Example: Kustomize Components

Render and deploy Kustomize overlays with Atmos-native Kubernetes components against a local Kubernetes emulator (k3s).

This example uses `provider: kustomize`, so Atmos renders the overlay with the Kustomize Go API and then applies the resulting objects through the Kubernetes Go SDK. It does not require the `kustomize` or `kubectl` binaries.

The local cluster is managed by the native **emulator** feature — Atmos starts the k3s container, harvests its kubeconfig, and injects `KUBECONFIG` for you. There is no `docker-compose.yml` and no manual kubeconfig wiring.

## Try It

```shell
cd examples/kustomize

# Start the local Kubernetes emulator (k3s)
atmos emulator up kubernetes -s dev

# Render the dev overlay
atmos kubernetes render demo -s dev --identity local-k3s

# Preview and apply through the Kubernetes SDK
atmos kubernetes diff demo -s dev --identity local-k3s
atmos kubernetes apply demo -s dev --identity local-k3s

# Clean up
atmos kubernetes delete demo -s dev --identity local-k3s
atmos emulator down kubernetes -s dev          # stop (state persists)
# atmos emulator reset kubernetes -s dev       # stop and delete cluster state
```

Or run the whole round trip in one step:

```shell
atmos test
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Kubernetes component config and the `local-k3s` emulator identity |
| `stacks/catalog/emulator/kubernetes.yaml` | The local Kubernetes emulator component (`driver: k3s`) |
| `components/kubernetes/demo/base/` | Kustomize base |
| `components/kubernetes/demo/overlays/dev/` | Kustomize overlay used by the dev stack |
| `stacks/catalog/demo.yaml` | Abstract component with provider and path configuration |

## Auth

The `local-k3s` identity uses `kind: kubernetes/emulator`, bound to the `kubernetes` emulator component. Running with `--identity local-k3s` makes Atmos resolve the running k3s container, harvest its admin kubeconfig, and export `KUBECONFIG` into the component environment — so the Kubernetes SDK client talks to the emulator with no extra configuration.

For real EKS clusters, pair an AWS identity with an `aws/eks` integration that writes kubeconfig for the cluster.
