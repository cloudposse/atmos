# Fix: Emulator endpoint unreachable from a socket-mounted job container

**Date:** 2026-08-19

## Summary

`atmos terraform test --ci`, run inside a CI job container that talks to Docker only through a
mounted host socket, started the AWS emulator successfully but handed Terraform/OpenTofu an
endpoint (`http://172.17.0.1:<port>`) that was not reachable from that same job container. The
AWS provider's `GetCallerIdentity` call failed with `connect: connection refused` after nine
retries. Atmos now actively joins its own container to the emulator's dedicated network instead
of guessing a reachable IP, so the emulator is addressed by DNS alias, not by IP.

## Context

`pkg/emulator/manager.go`'s `endpoint()` prefers a DNS-alias endpoint when Atmos's own container
can reuse its *existing* network (`container.CurrentContainerNetwork`). A job container started
with a plain `docker run` (no `--network`) sits on Docker's default bridge network, which is
correctly excluded from reuse because it doesn't support embedded DNS/aliases. When reuse failed,
`pkg/emulator/endpoint_host.go`'s `reachableHostForPublishedPorts()` fell back to parsing the
container's own default gateway out of `/proc/net/route` -- Docker's classic bridge-gateway IP.
That heuristic assumes the caller and the emulator container share one Linux host's `docker0`
bridge. Under Docker Desktop for macOS the daemon runs inside a VM; the bridge-gateway address
there is not where the host's published-port forwarding actually listens for sibling containers,
so nothing answered -- "connection refused," exactly as reported.

This closes a gap left open by PR #2942 ("Shared per-stack networking for containers, emulators &
run steps"), which introduced the current-container-network-reuse mechanism but only reused an
*existing* usable network -- it never made an unusable one (the default bridge) usable.

Reported and reproduced via a disposable copy of an internal application repository's
`bugs.md` item 8 ("Emulator identity loopback endpoints fail inside GitHub job containers"),
using a socket-mounted `docker:cli` job container with no `--network` flag -- the same shape as a
real CI job container that doesn't explicitly join a custom Docker network.

## Changes

- `pkg/container/network.go`: new `NetworkConnector` interface (`ConnectNetwork`) and
  `networkConnectResult` idempotency helper, alongside the existing `NetworkEnsurer`.
- `pkg/container/docker.go`, `pkg/container/podman.go`: `ConnectNetwork` implementations via
  `docker network connect` / `podman network connect`, using a new shared
  `buildNetworkConnectArgs` helper in `pkg/container/common.go`.
- `pkg/container/stack_network.go`: `AttachSharedNetwork` now calls
  `joinCurrentContainerToNetwork` when reuse fails -- best-effort connecting Atmos's own
  container to the dedicated per-stack network it just ensured for the new container, gated on
  `PreferCurrentContainerNetwork()` (so host-native/opted-out runs are unaffected). Once that
  real network attachment exists, every subsequent `CurrentContainerNetwork` call (in this or any
  later Atmos process in the same container) picks it up automatically -- no changes were needed
  in `pkg/emulator/manager.go`.
- `pkg/emulator/endpoint_host.go`: `reachableHostForPublishedPorts()` now prefers
  `host.docker.internal` (only when it actually resolves, via a 2s-bounded
  `net.Resolver.LookupHost`) before the raw default-gateway guess, as a secondary hardening for
  the last-resort fallback path.
- New tests: `pkg/container/sibling_network_test.go` (`TestSiblingContainerNetworking_Real`, an
  unstubbed regression test that exercises the real self-detection/join mechanism when actually
  run inside a container) and `pkg/container/sibling_network_docker_test.go`
  (`TestSiblingContainerNetworking_Docker`, opt-in via `ATMOS_TEST_SIBLING_CONTAINER=1`, which
  drives the above by running `go test` itself inside a nested container with no `--network` and
  the host socket mounted -- the exact reported topology).
- Unit test coverage added/extended in `pkg/container/{network,common,stack_network}_test.go` and
  `pkg/container/{docker,podman}_unit_test.go`, and `pkg/emulator/endpoint_host_test.go`.

## Validation

- `go build ./...` -- clean.
- `go test ./pkg/container/... ./pkg/emulator/...` -- all passing, including the new real-Docker
  integration test (`TestDockerRuntime_SharedNetworkAliasReachable_Integration`) and the
  unstubbed self-detection test.
- `ATMOS_TEST_SIBLING_CONTAINER=1 go test ./pkg/container/ -run TestSiblingContainerNetworking_Docker`
  -- passes with the fix in place. Verified it's a genuine regression test by temporarily
  reverting the `joinCurrentContainerToNetwork` call and re-running: fails with
  `dial tcp: lookup siblingtest-probe on ...: no such host`, confirming the test actually catches
  the bug rather than passing vacuously.
- `atmos lint --changed` -- 0 issues.
- Manual reproduction: disposable copy of the affected application repository, run as a
  socket-mounted `docker:cli` job container (`docker run --rm -v /var/run/docker.sock:... -v
  "$APP_COPY:/workspace" -e CI=true docker:cli sh -c 'atmos terraform test app -s fixtures
  --ci'`) with a locally built `atmos` binary. Before the fix: `emulator aws is up at
  http://172.17.0.1:<port>` followed by `connection refused`. After the fix: `emulator aws is up
  at http://fixtures-aws:4566` (DNS alias), and the run completes fully --
  `Success! 1 passed, 0 failed, 0 skipped.` -- with zero `connection refused`/`dial tcp` occurrences
  across the full ~800-line log, including two real `terraform apply`/`destroy` cycles against
  the emulator.
- Addressed CodeRabbit review feedback on cloudposse/atmos#2960: removed an overly broad
  `"already in"` idempotency substring match (would have masked genuine failures like "port
  already in use"), bounded the `host.docker.internal` DNS lookup with a timeout, and tightened
  the corresponding test's timing assertion against the actual configured constant instead of a
  loose bound.

## Follow-ups

None.
