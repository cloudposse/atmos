---
title: Helmfile
tags: [Kubernetes]
---

# Example: Demo Helmfile

Deploy Kubernetes resources using Helmfile with Atmos stack patterns, against a local Kubernetes emulator (k3s).

Learn more about [Helmfile Components](https://atmos.tools/components/helmfile).

## What You'll See

- [Helmfile component](https://atmos.tools/components/helmfile) configuration
- Stack inheritance for Kubernetes deployments
- A local k3s cluster managed by the native **emulator** feature (no Docker Compose)
- The **`!emulator` YAML function** wiring `KUBECONFIG` to the running emulator
- **Toolchain dependencies** — Atmos installs `helmfile`, `helm`, and `kubectl` for you

## Try It

```shell
cd examples/demo-helmfile

# Start the local Kubernetes emulator (k3s)
atmos emulator up kubernetes -s dev

# Deploy to dev (Atmos installs helmfile/helm/kubectl on first run)
atmos helmfile apply demo -s dev

# Check status
atmos emulator ps kubernetes -s dev

# Clean up
atmos emulator down kubernetes -s dev          # stop (ephemeral: cluster is discarded)
```

Or run the whole round trip in one step:

```shell
atmos test
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Helmfile component config + toolchain (Aqua registry, tool aliases) |
| `stacks/catalog/emulator/kubernetes.yaml` | The local Kubernetes emulator component (`driver: k3s`, ephemeral) |
| `stacks/catalog/demo.yaml` | Helmfile `demo` component + its `helmfile`/`helm`/`kubectl` tool dependencies |
| `stacks/deploy/dev/demo.yaml` | dev stack — sets `KUBECONFIG` via `!emulator kubernetes kubeconfig` |
| `components/helmfile/nginx/` | Helmfile component with manifests |

## How the cluster connection works

Helmfile does not integrate with Atmos Auth, so instead of an identity this example consumes the emulator **declaratively**. The `dev` stack sets, on the `demo` component:

```yaml
env:
  KUBECONFIG: !emulator kubernetes kubeconfig
```

The `!emulator kubernetes kubeconfig` function harvests the running emulator's admin kubeconfig to a file and returns its path; Helmfile (and the release's `kubectl` hooks) pick it up via `KUBECONFIG`. It resolves at apply time, so the emulator must be up first — `atmos emulator up kubernetes -s dev`.

> The function is scoped to the `demo` component rather than the stack-global `env`. `atmos emulator up` processes the stack with YAML functions enabled, and a stack-global `!emulator` would deadlock `up` on the very emulator it is starting.

## Toolchain

`atmos.yaml` declares the Aqua public registry, and the `demo` component declares `helmfile`, `helm`, and `kubectl` as tool dependencies. On the first `atmos helmfile *` run, Atmos installs them under `.tools/` and puts them on `PATH` for Helmfile and its hooks — no manual installation required.
