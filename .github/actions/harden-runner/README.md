# Harden runner with consistent DNS

Use this composite before checkout:

```yaml
- uses: $/.github/actions/harden-runner
  with:
    policies: >-
      github
      go-toolchain
      go-proxy
      go-checksums
```

`$/` loads this action and its files at the workflow's running commit, without
checkout. [`.github/allowlist.yaml`](../../allowlist.yaml) contains reusable endpoint
groups. The action reads it relative to its downloaded action directory, at the
same commit as the workflow. `$/` is action-reference syntax, not a file-path
prefix. The action
rejects missing policies and malformed endpoints. An explicit empty list adds
no destinations beyond StepSecurity’s implicit endpoints. Select one or more groups
with the whitespace-separated `policies` input; their union is allowed. No workspace
file or arbitrary path is loaded.

The lifecycle is intentional:

1. The internal runtime's **pre** hook loads the selected policy into the job
    environment before StepSecurity's **pre** hook reads its inputs.
2. StepSecurity installs its pinned agent with `egress-policy: block`.
3. The runtime's **main** repairs host DNS for the DNS-proxy agent, or retains
    host DNS when eBPF block enforcement is ready. StepSecurity's own main step
    then runs normally.
4. Both actions retain their post hooks. The runtime restores the original DNS
    file/symlink if the agent has stopped; while the agent remains active it keeps
    the working route for final uploads and job reporting.

DNS repair applies to standard GitHub-hosted Linux runners, including x64 and
ARM64. With the DNS-proxy agent, both architectures must resolve allowed hosts
through its listener on `127.0.0.1:53` so the firewall can recognize destination
IPs; the helper replaces host nameservers with that resolver.

The eBPF agent enforces policy in the kernel without that DNS listener. The helper
accepts either of two readiness signals:

- AgentAlpha initialization records identifying eBPF mode, the current job
  correlation ID, and an applied block policy, together with initialized status.
- The current systemd service invocation's journal confirming ingress/egress probe
  attachment and completed block-policy application. This check also confirms that
  the agent remains active and has not restarted while reading the evidence.

Stale logs, audit mode, incomplete startup, and unreadable evidence cannot satisfy
the corresponding eBPF readiness check. Transient journal or invocation read
failures retain the bounded startup wait so a late DNS proxy can still be used.

Both paths require an active agent configured for block mode and allow up to
60 seconds for startup readiness. The service can report active before its DNS
proxy binds or its eBPF block policy finishes initializing, especially while
upstream API requests retry. The helper does not widen firewall permissions or
change Docker DNS. Other runner types retain StepSecurity's normal behavior.

Set `dns-repair: 'false'` for jobs that need no DNS, such as the echo-only autofix
workflow, or to observe the original behavior in diagnostics. This disables only
the resolver repair; StepSecurity still runs with the selected policy.
The diagnostic workflow invokes the internal helper later to measure before and
after on the same runner. Normal workflows need only the composite above.

All block-mode workflow jobs use this composite. Audit-mode jobs retain their
existing configuration. Recovery was verified on an affected westus2 runner,
including host/container probes, denied traffic, and artifact upload. Reuse
service groups and add narrowly scoped groups for workload-specific endpoints;
do not combine all jobs’ destinations into a global permissive list.

Run tests:

```shell
npm test --prefix .github/actions/harden-runner
```

The pinned actionlint version predates `$/`. `.github/actionlint.yaml` suppresses
only the unsupported-reference diagnostic for this action reference in
workflows. Other lint checks remain enabled.

The [diagnostics action](diagnostics/README.md) retains before/after evidence. In before/after mode, an original
connectivity failure is informational if the repaired probes complete and pass.
A reachable unlisted destination, failed final probe, or incomplete final probe
always fails the job. Repair-first samples require both probe phases to pass.

The YAML parser is bundled and version-pinned, so pre-job policy loading makes
no npm requests. After changing its dependency, rebuild and test:

```shell
npm ci --ignore-scripts --prefix .github/actions/harden-runner
npm run build --prefix .github/actions/harden-runner
npm test --prefix .github/actions/harden-runner
```

Commit `runtime/yaml.cjs` with the lockfile. Its upstream license is included in
`runtime/YAML-LICENSE`.

OS background-service policies use `runner-os` and `endpoints` in the allowlist:

```yaml
windows-services:
  runner-os: Windows
  endpoints:
    - www.msftconnecttest.com:443
```

The runtime validates every selected policy and includes scoped endpoints only
when `RUNNER_OS` matches. Missing or invalid runner OS values fail validation
when a scoped policy is selected. This keeps Linux agent initialization from
depending on Windows or macOS background-service DNS.
