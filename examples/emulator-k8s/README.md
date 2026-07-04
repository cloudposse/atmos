---
title: Kubernetes Emulator
tags: [Emulators, Kubernetes]
---

## Notes

This example deploys a Helm release (nginx) to a **local Kubernetes sandbox** — no EKS, no
cloud, no `aws eks update-kubeconfig`. The sandbox is an
[Atmos emulator component](https://atmos.tools/cli/commands/emulator/usage) running
[k3s](https://k3s.io): a stack-scoped container Atmos starts and stops for you.

A single `kubernetes/emulator` identity in `atmos.yaml` binds every Helmfile component to the
sandbox. When the identity is active, Atmos harvests the k3s kubeconfig and exports `KUBECONFIG`
automatically — so Helmfile runs against the local cluster with **no kubeconfig to manage**. The
`nginx` Helmfile component is reused verbatim from [`../demo-helmfile`](../demo-helmfile); only the
cluster source changed (emulator instead of a hand-rolled `docker-compose.yml`).

## Usage

A container runtime (Docker or Podman) is required. The Helmfile component declares its
`helmfile`, `helm`, `kubectl`, and `kustomize` dependencies, so Atmos installs and adds those
tools to `PATH` through the project toolchain before running Helmfile:

```shell
atmos emulator up kubernetes -s local   # start the local k3s sandbox
atmos helmfile apply demo -s local       # deploy the nginx release to it

atmos helmfile destroy demo -s local     # remove the release
atmos emulator down kubernetes -s local  # stop and remove the sandbox container
```

The `atmos test` custom command runs the full apply/destroy lifecycle.
