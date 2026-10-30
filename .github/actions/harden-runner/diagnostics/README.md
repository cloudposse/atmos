# Runner network diagnostics

A dependency-free Node action for testing DNS and StepSecurity enforcement on
GitHub-hosted Linux runners. Use it through `$/` before checkout:

```yaml
- uses: $/.github/actions/harden-runner/diagnostics
  with:
    operation: probe
    phase: hardened
```

The workflow controls ordering, repair, timeouts, and artifact upload. This
action performs four operations:

| Operation | Behavior |
| --- | --- |
| `prepare` | Downloads the pinned curl container image; a failure still permits host probes. |
| `probe` | Tests five allowed destinations and a unique denied hostname from the host and container. `phase` must be `hardened` or `repaired`; repaired probes wait for Docker's resolver cache and retry the image download, and any phase retries it when the image is still missing. |
| `capture` | Records final host DNS and agent activity, including after an earlier failure. |
| `report` | Verifies required phases and writes a final pass/fail summary. |

Set `expect-repair: true` when reporting to require repaired probes. Also set
`repair-first: true` when the runner was repaired before its initial probes;
then both phases must pass. With both inputs false, initial probes must pass.
A reachable denied hostname always fails, even when later probes recover.
Missing or interrupted required probes, missing container evidence, and an
inactive agent also fail reporting.

The GitHub step summary shows destination results by phase and client, fresh
canary rejection evidence, and the final outcome. Failed initial connections
can be reported as recovered when the repaired phase passes.

Artifacts live in `$RUNNER_TEMP/network-diagnostics`. Each phase has a JSON
result and a text snapshot; `enforcement.log` contains only generated probe
fields. Raw agent logs, status files, and configuration are never uploaded or
printed. Rejection matching reads only fresh log lines and requires an exact
record for the probe's unique hostname, with bounded polling for delayed writes.
It recognizes both the community proxy's `domain not allowed` records and eBPF
`dns-request-dropped` events with `matchedPolicy=NONE`. Ordinary DNS requests,
different hostnames, and command audit lines do not confirm enforcement.

Commands use argument arrays with no shell and bounded execution times. Tests
inject command results and use temporary files; they do not change local DNS,
start containers, or contact external services.

```shell
node --test .github/actions/harden-runner/diagnostics/*.test.cjs
```
