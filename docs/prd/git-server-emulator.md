# PRD: Git Server Emulator (Gitea) + Local GitOps Round Trip

**Status:** Proposed
**Owner:** Atmos core
**Related:** [Emulators](./emulators.md) · [GitOps](./git-ops.md) · Kubernetes-native component · [Host-runtime containers](./host-runtime-containers.md)

## Summary

Add a **Git server emulator** to the Atmos emulator suite so the GitOps delivery
path — Atmos rendering Kubernetes manifests and **committing + pushing** them to a
managed Git repository (the `git` provision target) — can be exercised
**end-to-end, locally, with no external Git host**. The emulator is [Gitea](https://about.gitea.com/),
selected as the `git` target driver (`driver: gitea`).

On top of it, ship a runnable example — `examples/local-gitops/` — that stands up
the **complete loop**: Atmos pushes to the Gitea emulator, and **Flux**, running in
the existing **k3s emulator**, watches the repository and reconciles the manifests
back into the cluster. This is the Kubernetes-native vision in miniature: render →
commit → push → self-reconcile, entirely on a laptop or in CI.

## Motivation

The `git` provision target (`pkg/provisioner/target/git`) and the reusable
`pkg/git` clone/commit/push service are implemented and merged, but there has been
**no Git server in the emulator suite to push to**, so the push path was never
covered by an automated test and the GitOps story could not be demonstrated
without a real GitHub/GitLab repository and credentials.

A GitOps controller (Flux, Argo CD) cannot consume the `git://` protocol — it
requires `http(s)://` or `ssh://`. A bare `git daemon` would therefore prove only
that Atmos can push; it could never be watched by a controller. **Gitea over HTTP**
makes one emulator serve both halves of the loop, and is exactly what controllers
expect.

## Design

### New emulator target and driver

- **Target:** `git` (`TargetGit` in `pkg/emulator/driver.go`). Targets are not
  enum-validated at config time — `spec.Target()` returns the driver's own
  `Target()` — so adding one requires no switch/enum edits.
- **Driver:** `gitea` (`pkg/emulator/driver/gitea.go`), a `builtinDriver` with the
  Gitea image, HTTP port 3000, a headless-install env block (SQLite, INSTALL_LOCK,
  ROOT_URL) so it comes up without the web wizard, a `/api/healthz` health check,
  and `/data` as its persistence dir.
- **Profile:** `GitProfile` (`pkg/emulator/target/git.go`) surfaces
  `ATMOS_GIT_EMULATOR_URL` (the live HTTP base URL) for config/templates.

### Bootstrap (mirrors the Vault bootstrap)

A fresh Gitea boots installed-but-empty. `Manager.bootstrapGitIfNeeded`
(`pkg/emulator/manager.go`), gated on the `git` target and invoked from `Up` right
after the Vault bootstrap, makes it ready:

1. Create a throwaway admin user (`atmos`/`atmos`) via the in-container Gitea CLI,
   run as the image's `git` account. Idempotent — "user already exists" is success.
2. Create an auto-initialized `deployments` repository via the Gitea API over the
   live host port. Idempotent — a 409 conflict is success.

Credentials are throwaway-local and embedded in the configured remote URL; there is
no secret to protect.

### Shared emulator network (so Flux can reach Gitea)

For a controller inside the k3s emulator to pull from the Gitea emulator, the two
containers must resolve each other by name. A new optional runtime capability,
`NetworkEnsurer` (`pkg/container/network.go`, implemented by the Docker and Podman
runtimes), idempotently creates a per-stack user network
(`atmos-emulator-<stack>`). `Manager.attachSharedNetwork` joins every emulator
container to it with `--network-alias <component>`, so peers resolve each other
(e.g. `http://gitserver:3000`). It is **best-effort**: when the runtime cannot
create a network the container falls back to the default bridge — single-emulator
use is unaffected (host port publishing still works); only cross-container name
resolution is lost. This is the most environment-sensitive piece (Docker vs
rootless Podman vs the macOS VM).

### Push vs. pull addressing

The same Gitea server is reached two ways:

| | URL | Why |
|---|---|---|
| Atmos push (host) | `http://atmos:atmos@localhost:3000/...` | Atmos runs on the host, pushes via the published port |
| Flux pull (in-cluster) | `http://gitserver:3000/...` | Flux runs in k3s, reaches Gitea over the shared network alias |

## Example: `examples/local-gitops/`

A normal Atmos project that doubles as the E2E fixture (single source of truth):

- `gitserver` (Gitea) + `kubernetes` (k3s) emulators.
- `flux` (controllers/CRDs, vendored install manifest) + `flux-sync`
  (GitRepository + Kustomization + basic-auth Secret pointing at the local Gitea).
- `demo-app` kubernetes component with a `git` provision target.
- `atmos gitops` / `atmos teardown` custom commands and a narrated README.

## Testing

- **Unit:** `GitProfile` env shape; bootstrap idempotency (admin "already exists",
  repo 201/409) with a fake execer + `httptest`; network helpers
  (`networkCreateResult`, `sanitizeNetworkToken`, `emulatorNetworkName`).
- **E2E (gated on `ATMOS_TEST_FLOCI=true`, container runtime required), driving the
  example directory:**
  - `TestLocalGitOpsPushE2E` — `emulator up gitserver`, `kubernetes apply demo-app
    --target deployments`, then clone the repo back and assert the rendered
    manifest + provenance trailers landed. (No k3s/Flux required.)
  - `TestLocalGitOpsRoundTripE2E` — adds k3s + Flux and asserts, via
    `require.Eventually`, that the pushed resource is reconciled into the cluster.

## Validation

Both E2E tests pass against real Podman (podman-machine on macOS). The full loop was
confirmed: Flux's source-controller cloned `http://gitserver:3000/...` over the shared
network and the Kustomization applied the pushed manifest — the ConfigMap
`"Delivered by Atmos -> Gitea -> Flux"` appeared in the cluster.

## Risks / Open items

- **Cross-container DNS** (Flux pod → Gitea): **validated working** on Podman
  (aardvark-dns resolves the network alias; CoreDNS forwards to it). Still the most
  environment-sensitive piece; fallback if a runtime differs is a CoreDNS/hostAliases
  override or the host-gateway IP. The push half is unaffected regardless.
- **Controller cold-start timing:** Flux's source-controller can take minutes to become
  Ready on a memory-constrained runner; the round-trip test waits on its readiness
  before asserting reconciliation.
- **`emulator exec` + kubectl:** use plain `kubectl` (the k3s image symlink), not
  `k3s kubectl`, which misparses through exec.
- **Image pinning:** the driver uses a Gitea tag; CI should pin a digest (the Floci
  pattern) for supply-chain reproducibility.
- **Gitea CLI config path** (`/data/gitea/conf/app.ini`) is image-specific; covered by
  the E2E and adjusted if the upstream image layout changes.
- **Shared network lifecycle:** the per-stack network is created on `up` and reused
  idempotently; it is not removed on `down` (at most one empty network per stack).
