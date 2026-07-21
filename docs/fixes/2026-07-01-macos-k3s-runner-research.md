# Fix: macOS k3s CI runner selection for local emulator tests

**Date:** 2026-07-01
**Status:** Documented decision

## Problem

The Helm and Helmfile example smoke tests exercise the native Kubernetes emulator:

```text
atmos emulator up kubernetes -s dev
atmos helm diff/apply ...
```

That emulator starts a k3s container through the local container runtime. This is
important product coverage because local emulator development is primarily a macOS
developer workflow, especially on Apple Silicon.

The failing CI runs showed that GitHub-hosted Apple Silicon macOS runners cannot
reliably start the container runtime VM needed by Colima, Docker, or Podman. The
question was whether we should use Apple Silicon hosted runners, Podman, QEMU,
self-hosted Macs, AWS EC2 Mac, or keep a limited Intel macOS CI leg.

## Decision

Keep the dedicated k3s macOS example job on `macos-15-intel` for now, and keep
all ordinary macOS build and acceptance coverage on Apple Silicon (`macos-15`).
Start Colima with the VZ backend on the Intel runner. The QEMU backend reached
Lima's `usernet` SSH forwarding path and failed on hosted `macos-15-intel` with
`usernet unable to resolve IP for SSH forwarding` before any Atmos test ran.

This is not a long-term preference for Intel. It is a narrow compatibility choice
for the Docker/Colima-backed k3s emulator smoke test because the current CI
infrastructure does not provide a supported Apple Silicon macOS runner with
nested virtualization or a dedicated Mac host.

The k3s workflow remains covered on Linux as well, so the Intel macOS job is only
for validating the macOS-local runtime path.

## Findings

### GitHub-hosted Apple Silicon macOS

GitHub's standard hosted macOS arm64 runners are M1-based (`macos-14`,
`macos-15`, `macos-latest`, `macos-26`). GitHub documents a hard limitation:
nested virtualization is not supported on arm64 macOS runners due to Apple's
Virtualization Framework.

This also applies to GitHub larger macOS arm64 runners. Larger runners provide
more CPU and memory, but they do not remove the nested virtualization limitation.

References:

- GitHub hosted runner reference: https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- GitHub larger runner reference: https://docs.github.com/en/actions/reference/runners/larger-runners

### Why Docker, Colima, and Podman all hit the same class of failure

macOS cannot run Linux containers directly. Each of these tools starts a Linux VM:

- Docker Desktop: Linux VM managed by Docker Desktop.
- Colima: Linux VM via Lima, exposing Docker/containerd.
- Podman: `podman machine`, also a Linux VM.

Podman is useful as a supported local runtime option, but it does not avoid the
VM requirement. Podman's docs state that on macOS each Podman machine is backed
by a VM. On GitHub-hosted Apple Silicon macOS, that becomes VM-inside-VM nested
virtualization.

References:

- Podman macOS installation docs: https://podman.io/docs/installation
- Podman maintainer discussion about GitHub macOS runners: https://github.com/containers/podman/discussions/26859

### Observed hosted-runner failures

The known hosted Apple Silicon failure mode is `HV_UNSUPPORTED` or a VM process
exiting during startup:

- Colima on GitHub macOS arm64 fails when QEMU attempts `-accel hvf`.
- Podman can initialize the machine image, then fail when `vfkit` starts the VM.
- The `douglascamata/setup-docker-macos-action` documentation explicitly warns
  that M-series `macos-15` runners cannot start Colima because nested
  virtualization is unavailable.

References:

- GitHub runner-images issue: https://github.com/actions/runner-images/issues/9460
- Colima issue: https://github.com/abiosoft/colima/issues/970
- Setup Docker on macOS action: https://github.com/marketplace/actions/setup-docker-on-macos

### QEMU

QEMU only helps if it can run without hardware acceleration. The current failure
is not "QEMU is missing"; it is that QEMU/Lima/Colima try to use HVF on hosted
Apple Silicon macOS, and HVF is not available inside the GitHub-hosted runner.

Pure QEMU TCG software emulation may be theoretically possible, and Lima has
recent work around falling back from HVF to TCG. That is still not a dependable
path for this k3s smoke test:

- it is likely much slower than the existing 20-minute k3s job budget;
- the Colima/Lima fallback behavior is still changing;
- k3s is a relatively heavy workload for software-emulated Linux;
- using it as a required CI check would likely introduce flakiness.

We should revisit this only after Lima/Colima ship stable HVF-to-TCG fallback and
we have a measured workflow proving `atmos emulator up kubernetes` completes
reliably within CI timeouts.

References:

- Lima fallback discussion: https://github.com/lima-vm/lima/issues/5133
- Lima multi-arch/QEMU docs: https://lima-vm.io/docs/config/multi-arch/

### Self-hosted Apple Silicon Macs

A physical Apple Silicon Mac can run Podman, Colima, or Docker Desktop because
the Linux container VM is first-level virtualization on the host. This is the
best technical match for validating the local macOS developer workflow.

We are not using this path now because the current `runs-on` setup does not
provide a supported managed/self-hosted macOS runner pool for this repository.
If that changes, the preferred durable replacement for the Intel job is:

```yaml
runs-on: [self-hosted, macOS, ARM64, k3s]
```

with a pre-provisioned or job-started Podman/Colima runtime.

Security note: GitHub warns against using self-hosted runners for untrusted
public fork PRs. Any self-hosted macOS k3s runner should be restricted to trusted
events, protected branches, or an approval-gated workflow.

Reference:

- GitHub self-hosted runner warning:
  https://docs.github.com/en/actions/how-tos/manage-runners/self-hosted-runners/add-runners

### AWS EC2 Mac

AWS EC2 Mac is not a fit for this CI path right now. EC2 Mac instances are
Dedicated Hosts with a 24-hour minimum allocation period. Billing is per second
after allocation, but the minimum charge is still effectively one full day per
host allocation. Apple Silicon instance families have the same constraint.

Because we are not using dedicated-host pricing for this repository, EC2 Mac is
not a practical replacement for the hosted CI k3s job.

References:

- EC2 Mac overview and billing note: https://aws.amazon.com/ec2/instance-types/mac/
- EC2 Mac user guide: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-mac-instances.html
- EC2 Dedicated Host pricing note: https://aws.amazon.com/ec2/dedicated-hosts/pricing/

## Follow-up Criteria

Revisit the `macos-15-intel` k3s job when one of these becomes true:

1. GitHub-hosted Apple Silicon macOS runners support nested virtualization for
   Linux container runtimes.
2. The repository has a supported Apple Silicon macOS runner pool in its
   `runs-on` configuration.
3. Lima/Colima/Podman provide a measured, reliable QEMU TCG fallback that can run
   the k3s emulator workflow inside CI timeouts.

Until then, `macos-15-intel` is the least bad hosted option for this specific
k3s emulator smoke test, while normal macOS CI continues to run on Apple Silicon.
