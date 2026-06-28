# Example: Local GitOps Round Trip

Stand up an **entire GitOps loop on your laptop** — no GitHub account, no real
cluster, no cloud credentials — and watch a change travel all the way around:

```
atmos render  ─push─▶  Gitea (Git server emulator)  ─watch─▶  Flux  ─apply─▶  k3s (Kubernetes emulator)
     ▲                                                                              │
     └──────────────────────────  you observe it running  ◀───────────────────────┘
```

Two local emulators do the heavy lifting:

- **`gitserver`** — a [Gitea](https://about.gitea.com/) Git server (the `gitea`
  emulator driver). Atmos auto-bootstraps a throwaway admin (`atmos`/`atmos`) and a
  `deployments` repository. Gitea serves Git over **HTTP**, which is what Flux needs
  to watch a repository (a bare `git daemon` speaking `git://` cannot be watched).
- **`kubernetes`** — a [k3s](https://k3s.io/) cluster (the `k3s` emulator driver).
  **Flux** runs inside it and reconciles the `deployments` repository.

Both emulators join a shared per-stack network, so Flux (in k3s) reaches Gitea by
name at `http://gitserver:3000`, while Atmos pushes from the host to the same
server on `http://localhost:3000`.

## Try It

Prerequisites: a container runtime (Docker or Podman) and Terraform/OpenTofu are
not needed here — only the container runtime. Then:

```shell
cd examples/local-gitops

# The whole loop in one command:
atmos gitops
```

`atmos gitops` runs these steps (run them by hand to watch each stage):

```shell
# 0. Vendor the pinned Flux install manifest into the flux component's files/
atmos vendor pull

# 1. Start the Git server emulator (Gitea bootstraps admin + `deployments` repo)
atmos emulator up gitserver -s local

# 2. Start the Kubernetes emulator (k3s)
atmos emulator up kubernetes -s local

# 3. Install Flux into the cluster, then point it at the local Gitea repository
atmos kubernetes apply flux -s local --identity local-k3s
atmos kubernetes apply flux-sync -s local --identity local-k3s

# 4. Render the demo app's manifests and PUSH them to Gitea (no cluster contact)
atmos kubernetes apply demo-app -s local --target deployments
```

Now watch the round trip complete — Flux pulls the commit Atmos just pushed and
applies it:

```shell
# Browse the pushed commit in Gitea's UI (login atmos / atmos)
open http://localhost:3000/atmos/deployments

# Confirm Flux reconciled it INTO the cluster (it appears within ~30s once Flux is
# ready). `emulator exec` runs a command inside the k3s container, which bundles kubectl:
atmos emulator exec kubernetes -s local -- kubectl get ns gitops-demo
atmos emulator exec kubernetes -s local -- kubectl -n gitops-demo get configmap demo-app -o yaml
atmos emulator exec kubernetes -s local -- kubectl -n flux-system get gitrepository,kustomization
```

Make a change and push again to see it flow around:

```shell
# Edit vars.message in stacks/catalog/demo-app.yaml, then:
atmos kubernetes apply demo-app -s local --target deployments
# Flux reconciles the new commit; the ConfigMap updates in-cluster.
```

Tear it all down:

```shell
atmos teardown          # stop + remove both emulators AND the managed clone
# atmos emulator reset kubernetes -s local   # also wipe cluster state
```

> Re-running `atmos gitops` after a teardown starts from a fresh `gitserver` (it is
> ephemeral) and a fresh managed clone, so they always line up. Within one session
> the `gitserver` stays up, so editing and re-applying `demo-app` simply
> fast-forwards the repository.

## How the Push and the Pull Differ

| | URL | Why |
|---|---|---|
| **Atmos push** (host) | `http://atmos:atmos@localhost:3000/...` | Atmos runs on the host and pushes through Gitea's published port |
| **Flux pull** (in-cluster) | `http://gitserver:3000/...` | Flux runs in k3s and reaches Gitea over the shared emulator network by alias |

Both point at the same Gitea server — one from outside, one from inside.

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | `git.repositories.deployments` (local Gitea), the `local-k3s` identity, and the `atmos gitops` / `atmos teardown` commands |
| `stacks/catalog/emulator/gitea.yaml` | The Git server emulator (`driver: gitea`) |
| `stacks/catalog/emulator/kubernetes.yaml` | The Kubernetes emulator (`driver: k3s`) |
| `stacks/catalog/flux.yaml` | `flux` (controllers) + `flux-sync` (GitRepository/Kustomization/Secret pointing at Gitea) |
| `stacks/catalog/demo-app.yaml` | The app delivered through the loop, with a `git` provision target |
| `vendor.yaml` | Pulls the pinned upstream Flux install manifest |

## The Bigger Picture

This is the Kubernetes-native vision in miniature: a component is **rendered**,
**committed**, and **pushed** to Git, and the cluster **self-reconciles** from
there. Swap the Gitea emulator for a real Git host and the k3s emulator for a real
cluster, and the exact same Atmos components drive production GitOps.
