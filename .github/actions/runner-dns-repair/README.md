# Runner DNS repair experiment

This Linux-only action tests the westus2 DNS-routing workaround. Use it **after**
StepSecurity initialization and the diagnostic's original-configuration probes:

```yaml
- uses: $/.github/actions/runner-dns-repair
```

`$/` loads the action at the workflow's running commit without checkout. The action
requires the community agent to be active, configured in block mode, and listening
on UDP `127.0.0.1:53`. It replaces the host nameserver list with that resolver while
preserving other resolver options. It does not modify firewall rules or Docker DNS.
Unexpected DNS configurations are rejected.

The original file or symlink is saved before replacement. The post step restores
it if the agent has stopped. If the agent is still active, it retains the repaired
route: restoring the faulty route would disrupt final uploads and job reporting.
This is intended only for disposable GitHub-hosted diagnostic runners.

The workflow must independently verify allowed host/container traffic and blocked
traffic after repair. A successful file write alone does not prove the workaround.
This experiment does not yet wrap StepSecurity or load its endpoint policy from a
file; those changes depend on validating the repair first.

Run regression tests with:

```shell
node --test .github/actions/runner-dns-repair/index.test.cjs
```
