# Harden Runner `none` policy used an IP literal

**Date:** 2026-11-14

## Summary

The `none` endpoint group listed `127.0.0.1:9` so that no-network jobs would select allowlist
enforcement. Harden Runner resolves every allowed endpoint as a DNS name when it starts. An IP
literal cannot be resolved that way, so the agent reverted and the local composite's DNS repair
step failed with "StepSecurity agent is not active". The `[k3s] demo-helmfile` gate job, which only
checks the k3s matrix result, failed on that step even though the matrix itself succeeded.

## Evidence

Job 132460617508 (run 43597492161, head `facc595a9f`) logged
`unable to resolve domain 127.0.0.1., status 3`, `agent.service: Failed with result 'exit-code'`,
and then `DNS repair failed: StepSecurity agent is not active`.

## Fix

`none` now lists `github.com:9`. The name resolves, port 9 (discard) is not served by GitHub, and
`github.com` is already reachable through the implicit upstream hosts, so no destination is widened.
The policy test asserts the new value.

## Verification

The policy unit tests cover the group value. Whether the agent stays active with this value is only
observable on a hosted runner, so confirmation is the next CI run on the pushed head.
