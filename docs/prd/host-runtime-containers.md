# Host-Runtime Containers (`container.runtime.host`) — Design

> Status: **approved, implementing.** Names a single intent flag so a container Atmos
> launches can drive the **host's** container runtime (Docker-out-of-Docker), which is
> what lets the MiniStack AWS emulator spawn its backing k3s cluster for EKS.

## Problem

Some containers Atmos launches need to **drive the host container runtime themselves** —
they spawn and manage *sibling* containers (Docker-out-of-Docker / DooD). Concrete cases:

- The **MiniStack** AWS emulator's EKS service spawns a real `rancher/k3s` container per
  `eks:CreateCluster`. Without runtime access it logs `EKS: Docker unavailable — cluster
  created without k3s backend` and returns an empty CA / unreachable endpoint.
- General DooD workloads (Testcontainers-style suites, build tools that shell out to
  `docker`) running as container steps or container components.

Atmos already adapts to a rootless runtime (e.g. the k3s emulator's cgroup-nesting shim),
but there is **no way to ask Atmos to give a container access to the host runtime**, which
in practice requires: bind-mounting the host runtime **socket**, running the container with
enough privilege to use it (root), and on **SELinux** hosts relabeling the socket mount.
(Verified by hand: MiniStack's k3s backend only came up with a rootful socket +
`-v …/podman.sock:/var/run/docker.sock` + `--user root` + `--security-opt label=disable`.
See memory `project_ministack_eks_emulation_findings`.)

## Naming: why `container.runtime.host` (not `rootful` / `privileged`)

The capability is **relational** — it's about the container's relationship to the host
runtime — which is why a single adjective never fits. Industry vocabulary: **DinD**
(docker-in-docker, a nested daemon) vs **DooD** (docker-outside-of-docker, drive the host
daemon via its socket). Ours is DooD.

- `rootful` describes the runtime *privilege model* (Podman's `--rootful`/rootless). It's
  only the *enabler* on rootless hosts, not the capability; on Docker you need nothing
  "rootful," just the socket.
- `privileged` is a different, already-used field (kernel caps / device access; the k3s
  emulator sets it). A privileged container still can't reach the host daemon without the
  socket, and MiniStack worked **without** `--privileged`. Orthogonal.

The disambiguation belongs in the **namespace**, not an underscore: Atmos already nests
`container.runtime.provider`. So the flag is `container.runtime.host` — one natural word per
segment, scoped by `runtime.`, expressing intent ("use the host runtime"). Atmos handles the
mechanics (socket, root, SELinux, going rootful where required), the same way it
auto-applies the k3s rootless shim.

## Configuration

`host` is added to the existing `ContainerRuntimeConfig` (`pkg/schema/container_config.go`),
which already backs the global `container.runtime` block. `ContainerRunStep`
(`pkg/schema/workflow.go`) — the per-instance struct shared by emulator components, container
components, and container steps — gains a nested `runtime` block reusing that struct, so the
flag reads identically everywhere:

```yaml
# Emulator component (MiniStack with EKS)
components:
  emulator:
    aws:
      driver: ministack/aws
      container:
        runtime:
          host: true       # ministack spawns k3s siblings for EKS

# Container component
components:
  container:
    integration-tests:
      run:
        runtime:
          host: true

# Container step (workflow / custom command)
steps:
  - name: e2e
    type: container
    runtime:
      host: true
```

A global default is also available: `container.runtime.host: true`.

## What `host: true` does (semantics)

A single `Host bool` threads from `ContainerRunStep.Runtime.Host` (and the global
`container.runtime.host`) onto `container.CreateConfig.Host`, and is applied **once** in the
shared `runtime.Create` chokepoint that every surface reaches (`UpWithRuntime` and
`RunEphemeralContainer` both call `runtime.Create`). When set, Atmos:

1. **Mounts the host runtime socket** into the container at `/var/run/docker.sock` (the
   de-facto path tools, including MiniStack, expect), resolving the host socket from the
   active runtime.
2. **Runs as root** — sets `--user 0` unless the config already pins a `User`.
3. **Relabels for SELinux** — adds `--security-opt label=disable` (no-op off SELinux) so the
   root-owned socket is reachable from inside the container.
4. **Sets `DOCKER_HOST`** in the container env to the mounted socket so SDKs/CLIs inside find it.

### Host socket resolution (`pkg/container`)

A new `HostRuntimeSocket(ctx, runtime)` (next to `RuntimeIsRootless`) returns the socket for
the active runtime:

- **Docker**: `DOCKER_HOST` if a unix socket, else `/var/run/docker.sock`.
- **Podman**: `podman info --format '{{.Host.RemoteSocket.Path}}'` (authoritative for the
  active rootless/rootful socket).

## Known limitation (documented, not hidden)

Under **rootless podman**, a bind-mounted socket is `Permission denied` inside the container
even as root (user-namespace boundary). True DooD there needs the **rootful** podman service
(`podman machine set --rootful`). So `host: true`:

- **Works** on Docker (Linux, Docker Desktop) and rootful podman.
- **Degrades** on rootless podman — Atmos emits a clear `log.Warn` ("`container.runtime.host`
  requested but the active runtime is rootless podman; the container cannot reach the host
  runtime — use Docker or `podman machine set --rootful`") rather than failing silently.

## Security

Host-runtime access is effectively host root. The flag is **opt-in**, off by default,
documented with a warning, and never implied by a driver default without the user opting in.
It is independent of `privileged` (kernel caps), which remains a separate field.

## Plan

1. Schema: `Host` on `ContainerRuntimeConfig`; nested `Runtime *ContainerRuntimeConfig` on
   `ContainerRunStep`. Update JSON schemas under `pkg/datafetcher/schema/`.
2. `pkg/container`: `Host` on `CreateConfig` / `NamedConfig` / `EphemeralConfig`; pass through
   `buildNamedCreateConfig` / `buildEphemeralCreateConfig`; `HostRuntimeSocket`; apply in
   `podman.Create` / `docker.Create` before `buildCreateArgs`; rootless-podman warn.
3. Wire each surface to set `Host`: emulator (`Spec` → `namedConfig`), container component
   (`run`), container step (`buildRunConfig`).
4. Tests: arg assembly (socket mount + `--user 0` + `label=disable` + `DOCKER_HOST`),
   socket resolution, rootless warn, schema decode of the nested `runtime` block.
5. Docs: Docusaurus pages for emulators / container components / container steps.

## Non-goals

- Emulating EKS IAM auth (MiniStack's k3s has no webhook; `aws eks get-token` → 401). Out of
  scope; see `project_ministack_eks_emulation_findings`.
- Rootless-podman socket passthrough (an upstream podman constraint).
