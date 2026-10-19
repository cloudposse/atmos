# Fix: Colima VM DNS on macOS k3s jobs uses the runner's resolver, not public ones

**Date:** 2026-10-18

## Summary

The `[k3s-macos] helm` and `[k3s-macos] demo-helmfile` jobs failed in `Start Docker-compatible runtime on macOS` because the Colima VM could not resolve `registry-1.docker.io`. The previous fix had pointed the VM at `8.8.8.8` and `1.1.1.1`, which harden-runner drops on these runners. The VM now uses the runner's own IPv4 resolvers, and the step checks VM DNS before it spends the image-pull budget.

## Context

Run 39128535689 failed both macOS legs the same way: Colima started, `docker info` passed, and `docker pull rancher/k3s:latest` failed twice with `lookup registry-1.docker.io on 192.168.5.1:53: i/o timeout`. The harden-runner telemetry in those jobs shows why: `exe=.../limactl remoteIP=8.8.8.8 remotePort=53 proto=UDP verdict=Dropped`, and the same for `1.1.1.1`. Both macOS jobs run under harden-runner with `egress-policy: block`, and neither public resolver is allowed. Harden-runner also enforces the endpoint allowlist on DNS by domain, so allowlisting public resolvers would have weakened the policy instead of fixing the job. The `[k3s] demo-helmfile` failure in the same run is only the matrix result gate reporting the macOS failures.

The DNS path inside the VM, from Colima 0.10.3 and Lima 2.2.0 source, is: `/etc/resolv.conf` points at a dnsmasq Colima runs on `192.168.5.1`; dnsmasq forwards to the `--dns` list, or to the Lima gateway `192.168.5.2` when the list is empty; the gateway is gvisor-tap-vsock inside `limactl`, which resolves A/AAAA on the host through the macOS system resolver with no timeout. Queries to an explicit `--dns` address leave `limactl` as plain UDP to that address, which is what harden-runner dropped.

The default gateway path is not reliable either. In the pre-fix run 39099284771, `[k3s-macos] demo-helmfile` failed with the same `i/o timeout` while `[k3s-macos] helm` passed. The telemetry shows the host-side lookup of `registry-1.docker.io` was permitted at 06:15:09 but answered at 06:16:05, a 56-second stall, after both pull attempts had already given up at 06:15:19 and 06:15:59. In the passing job the same lookup answered within the same second. That stall is what motivated the public-resolver change.

## Changes

Single file, `.github/actions/setup-colima/action.yml`:

- Read the runner's IPv4 nameservers from `scutil --dns` before `colima start` and pass each as `--dns`. Only default resolver blocks in the unscoped section qualify; supplemental blocks (those with a `domain :` line, which the host uses for one domain only) and the scoped section are skipped, as are loopback addresses, and the list is deduplicated. The runner's resolver is already permitted by harden-runner, and an explicit `--dns` address bypasses the host system-resolver path where the stall occurred. When `scutil` reports nothing the list is empty and Colima keeps its default; the expansion is guarded for `set -u` on bash 3.2.
- Add a DNS readiness gate after `docker version`: up to eight `colima ssh -- getent ahostsv4 registry-1.docker.io` attempts, ten seconds each with a five-second pause, so a resolver fault fails the step in about two minutes with diagnostics instead of after two 300-second pull timeouts. The retries also absorb a transient stall of the size observed.
- Factor the `Colima DNS diagnostics` group into a helper used by both the readiness gate and the pull-failure path, and extend it with `scutil --dns`, the resolvers passed to Colima, and the VM's dnsmasq config.
- Replace the comment citing the Colima FAQ with the actual constraint: harden-runner drops direct UDP/53 to public resolvers on these runners.

Also updated the budget comment on the `Start Docker-compatible runtime on macOS` step in `.github/workflows/test.yml` so its arithmetic includes the readiness gate; the 55-minute step cap and all other budgets are unchanged.

## Validation

- `bash -n` on the extracted `run:` script under the macOS system bash 3.2.57 passed, and `shellcheck -s bash -S warning` reported nothing.
- The resolver-discovery pipeline and the `set -u` array idioms were exercised locally under the macOS system bash 3.2.57 (the strictest target; no Homebrew bash is installed here) with both a populated `scutil --dns` and a simulated empty one: the populated case produced one `--dns <ip>` pair per unique IPv4 nameserver in order, and the empty case produced no `--dns` arguments and the fallback message. The default-resolver filter was also checked against a crafted `scutil --dns` sample containing a default block, an mdns block, a VPN split-zone block with IPv4 and IPv6 servers, a loopback block, and a scoped section: only the default block's address survived.
- `actionlint` was run but does not apply to composite action files (it reports missing `on`/`jobs` sections), so it validated nothing here.
- The harden-runner behavior cannot be reproduced locally. The fix is confirmed by re-running `[k3s-macos] helm` and `[k3s-macos] demo-helmfile` on PR #3411: the readiness gate should pass on the first attempt, `docker pull rancher/k3s:latest` should complete, and the harden-runner log should show no `limactl ... remotePort=53 ... verdict=Dropped` entries.

## Follow-ups

If the host-resolver path also stalls, the readiness gate now shows it within about two minutes together with `scutil --dns` output from the runner, which is the data needed to decide the next step (for example a Docker Hub mirror such as `mirror.gcr.io`, which is already allowlisted and used on the Linux leg but still depends on working DNS). No change is planned until that evidence exists.
