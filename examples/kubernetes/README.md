---
title: Kubernetes Components
tags: [Kubernetes]
cast:
  file: /casts/examples/kubernetes/lifecycle.cast
  title: atmos kubernetes lifecycle
---

# Example: Kubernetes Components

Deploy Kubernetes manifests with Atmos-native Kubernetes components against a local Kubernetes emulator (k3s).

This example uses `provider: kubectl`, which means kubectl-compatible manifest behavior through the Kubernetes Go SDK. It does not require the `kubectl` binary.

The local cluster is managed by the native **emulator** feature — Atmos starts the k3s container, harvests its kubeconfig, and injects `KUBECONFIG` for you. There is no `docker-compose.yml` and no manual kubeconfig wiring.

## Try It

```shell
cd examples/kubernetes

# Start the local Kubernetes emulator (k3s)
atmos emulator up kubernetes -s dev

# Render manifests without contacting the cluster
atmos kubernetes render demo -s dev --identity local-k3s

# Preview and apply through the Kubernetes SDK
atmos kubernetes diff demo -s dev --identity local-k3s
atmos kubernetes apply demo -s dev --identity local-k3s

# Process every Kubernetes component in DAG order
atmos kubernetes apply --all -s dev --identity local-k3s

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
| `components/kubernetes/demo/files/` | File-backed Kubernetes manifests loaded through `paths` |
| `stacks/catalog/demo.yaml` | Abstract Kubernetes component with inline `manifests` |

## Auth

The `local-k3s` identity uses `kind: kubernetes/emulator`, bound to the `kubernetes` emulator component. When a command runs with `--identity local-k3s`, Atmos resolves the running k3s container, harvests its admin kubeconfig to a realm-scoped file, and exports `KUBECONFIG` into the component environment — so the Kubernetes SDK client talks to the emulator with no extra configuration.

For real EKS clusters, pair an AWS identity with an `aws/eks` integration that writes kubeconfig for the cluster (see the commented block in `atmos.yaml`).
