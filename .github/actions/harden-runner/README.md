# Harden runner with consistent DNS

Use this composite before checkout:

```yaml
- uses: $/.github/actions/harden-runner
  with:
    policy: network-diagnostics
```

`$/` loads this action and its files at the workflow's running commit, without
checkout. `endpoints.json` contains named, reviewed endpoint lists. The action
rejects missing/empty policies and malformed endpoints. Policies are selected by
name; no workspace file or arbitrary path is loaded.

The lifecycle is intentional:

1. The internal runtime's **pre** hook loads the selected policy into the job
    environment before StepSecurity's **pre** hook reads its inputs.
2. StepSecurity installs its pinned agent with `egress-policy: block`.
3. The runtime's **main** repairs host DNS through the already-active agent.
    StepSecurity's own main step then runs normally.
4. Both actions retain their post hooks. The runtime restores the original DNS
    file/symlink if the agent has stopped; while the agent remains active it keeps
    the working route for final uploads and job reporting.

DNS repair applies to standard GitHub-hosted Linux x64 runners. It requires the
community agent in block mode and a listener on `127.0.0.1:53`, then replaces the
host nameservers with that resolver. It does not widen firewall permissions or
change Docker DNS. Other runner types retain StepSecurity's normal behavior.

Set `dns-repair: 'false'` only to observe the original behavior in diagnostics.
The diagnostic workflow invokes the internal helper later to measure before and
after on the same runner. Normal workflows need only the composite above.

This is currently integrated with the diagnostic workflow. Validate recovery on
an affected runner and hook ordering before moving normal CI jobs to it. Add a
separate named policy for each workload; do not combine all jobs' destinations
into a global permissive list.

Run tests:

```shell
node --test .github/actions/harden-runner/runtime/*.test.cjs
```

The pinned actionlint version predates `$/`. `.github/actionlint.yaml` suppresses
only the unsupported-reference diagnostic for this workflow. Other lint checks
remain enabled.
