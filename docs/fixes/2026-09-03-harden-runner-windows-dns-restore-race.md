# Fix: Windows CI jobs left stuck after "Complete job" by harden-runner's DNS-restore race

**Date:** 2026-09-03

## Summary

Since harden-runner landed on the Windows and macOS legs of `test.yml` (2026-08-31, #2958) and went to
block mode (2026-09-02, #3027), Windows jobs have been finishing every step, including `Post Harden Runner`
and `Complete job`, and then never reporting a conclusion until GitHub cancels them 30–40 minutes later.
`Build (windows)` gates every other job, so one stuck build stalls the whole run, including merge-queue
entries. Root cause is a race in harden-runner's Windows post step: it waits at most 10 s for its agent, then
kills it, and in the losing case the agent is still restoring the runner's DNS settings, leaving the adapter
pointed at a DNS proxy that no longer exists. This change adds a scheduled-task watchdog that repairs DNS
only in that exact state, allowlists the Windows/macOS operating-system endpoints that StepSecurity showed
blocked on every job, removes the `|direct` Go proxy fallback that only fans out to blocked hosts under block
mode, cancels superseded PR runs, and stops the Trivy SARIF upload from evicting PRs from the merge queue.
harden-runner stays in block mode on every leg.

## Context

Measured from `test.yml` run data (Aug 24 – Sept 3), raw job logs (which include the harden-runner agent's
own log), StepSecurity network-event telemetry, and harden-runner's source.

**Mechanism.** harden-runner's Windows agent (v1.0.7-win, hard-coded in `src/install-agent.ts`) enforces
egress with a DNS proxy on `127.0.0.1:53` and repoints every adapter to it with `Set-DnsClientServerAddress`.
Its post step (`src/cleanup.ts`, `handleWindowsCleanup`) writes `C:\agent\post_event.json`, polls for
`done.json` for at most 10 × 1 s, logs `timed out`, sends SIGINT and sees the agent gone within a second.
In every stuck job the agent picked the post event up ~5 s late and had just logged
`[handlePostHardenRunnerEvent] restoring system DNS` (a step that takes ~2.4 s when it completes) when it was
killed. The runner can still upload logs to hosts it already resolved, but the final job-completion call to
the Actions service needs a fresh lookup and never succeeds.

| Job | Kind | 2nd `timed out` in post step | `system DNS settings restored` | `DNS proxy stopped` |
|---|---|---|---|---|
| 100694525046 Build (windows) | stuck | yes | no | no |
| 100532566143 shard 2 | stuck | yes | no (restore began 5.6 s after post step) | no |
| 100127533462 shard (Sept 2, audit mode) | stuck | yes | no (restore began 4.8 s after) | no |
| 100688004653 Build (windows) | healthy | no | yes (2.9 s → 5.3 s after post step) | yes |
| 100532566215 shard 3 | healthy | no | yes (2.2 s → 4.5 s after) | yes |
| 100696221376, 100696221470 shards | healthy | no | yes | yes |

**Frequency.** Counting only cancelled jobs whose job-level completion landed ≥ 5 min after their last step
(true zombies; ordinary cancellations land within seconds):

| Day | true stuck Windows jobs | note |
|---|---|---|
| Aug 25–28 | 0 | 12 "cancelled" jobs all completed ≤ 5 s after their last step (ordinary cancels) |
| Aug 31 (harden-runner `audit` on Windows/macOS legs) | 0 | 11 instant cancels (manual cancel + rerun) |
| Sept 1 (audit) | 5 | |
| Sept 2 (`block` from 21:08 UTC) | 9 | |
| Sept 3 | 23 | ≈ 1.5 % of Windows jobs, ≈ 27 % of runs (18 Windows jobs per run) |

It happens in audit mode too (the agent and proxy run in both), so audit is not a fix, and harden-runner
v2.21.1 does not change the Windows agent.

**Blocked OS endpoints.** StepSecurity Insights shows the same operating-system destinations blocked on
every Windows/macOS job, healthy or stuck, from OS services rather than our steps: NCSI connectivity probes
(`www.msftconnecttest.com`, `www.msftncsi.com`, IPv6 variants, retried every few seconds), WNS,
settings/events telemetry, Windows Update, `time.windows.com`, Store/Live/`go.microsoft.com`, and Sectigo
OCSP/CRL (GitHub's certificate chain, hit from `git-remote-https.exe`); on macOS, Apple software-update, CDN,
iCloud and revocation hosts plus NTP. None are implicitly allowed (StepSecurity's docs: only their own agent
endpoints and dot-less hostnames are), and `allowed-endpoints` supports domain wildcards.

**Go proxy fallback.** With `GOPROXY=https://proxy.golang.org|direct`, one proxy hiccup on a harden-runner PR
run fanned `go mod download` out to ~25 vanity-import hosts (`k8s.io`, `go.uber.org`, `gopkg.in`, `helm.sh`,
…), all blocked, and `Get dependencies` took 9+ minutes of retries. Under block mode the fallback can only
ever cost time.

The same fallback also breaks `govulncheck` in `codeql.yml` on merge-queue runs (run 33785801022:
`unrecognized import path "sigs.k8s.io/kustomize/kyaml": https fetch ... lookup sigs.k8s.io ... operation not
permitted`), evicting the PR; the job passes on this branch with the fallback removed.

**macOS agent matches by IP.** StepSecurity shows `api.github.com:443` dropped on a macOS shard with
`matched_policy: EXACT_IP` even though the host is allowlisted: the macOS agent did not observe the DNS answer
(Runner.Worker and a test binary reused a cached resolution) and blocked the connection by IP. This is an
agent limitation, not an allowlist gap, and is included in the upstream report below.

**Merge queue.** `[lint] Dockerfile` failed on 16 `merge_group` runs: `codeql-action/upload-sarif` fails with
`ref 'refs/heads/gh-readonly-queue/…' not found`, which skips the Trivy scan and fails the second upload on
the missing SARIF file, evicting the PR from the queue.

## Changes

- `.github/actions/windows-dns-guard/` (new): composite action that registers a one-shot scheduled task
  (`atmos-dns-guard`, running as the runner's own Administrator account, outside the runner's process tree so it survives step teardown and the
  runner's orphan-process cleanup) running `dns-guard.ps1`. The script polls once a second and resets DNS
  (`Set-DnsClientServerAddress -ResetServerAddresses` + `Clear-DnsClientCache`) only when
  `C:\agent\post_event.json` exists, the agent from `C:\agent\agent.pid` is gone, and an IPv4 interface still
  lists `127.0.0.1`. The first condition keeps an agent crash mid-job fail-closed exactly as today. It logs to
  `$RUNNER_TEMP\atmos-dns-guard.log` and exits after acting or when DNS is already healthy.
- `.github/workflows/test.yml`:
  - Guard step on every Windows leg (`build`, `terraform-registry-cache`, `test` × 10 shards, `mock`),
    right after checkout, gated the same way as the other Windows steps. Draft PRs skip every Windows step
    including checkout, so Harden Runner is now skipped for them too rather than left to race without the
    guard.
  - Windows and macOS OS endpoints added to the `allowed-endpoints` of `build`, `terraform-registry-cache`,
    `test`, `mock`, `k3s` (macOS leg) and `kubernetes-e2e` (macOS leg), with a comment naming the source and
    the trade-off. `test` also gains `sum.golang.org`, `google.golang.org` and `modernc.org` for parity with
    `build` (its Linux/macOS legs run `atmos build deps`). Firefox telemetry stays blocked. Two gaps surfaced
    by StepSecurity notices during review are closed too: `api.github.com` was never allowlisted on
    `validate-affected`, `floci`, `k3s` and `validate`, and the Windows Software Licensing Service probes
    `tas02.sls.update.microsoft.com`, so the update entry is the wildcard `*.update.microsoft.com`.
  - `GOPROXY` drops `|direct`; `atmos build deps`'s retry policy covers the transient proxy errors it existed
    for.
  - Workflow-level `concurrency` cancels a PR's superseded run on a new push; `merge_group`, `push` and
    `workflow_dispatch` are keyed by SHA and never cancelled.
  - Both `codeql-action/upload-sarif` steps in the `docker` job skip on `merge_group`.
- `.github/workflows/setup-go-cache-warmup.yml`: guard step, the same OS endpoints and Go hosts, `GOPROXY`.
- `.github/workflows/native-ci.yml`, `codeql.yml`, `pre-commit.yml`: `GOPROXY` drops `|direct` (all run under
  block mode).

## Validation

- `python3 -c 'import yaml; yaml.safe_load(...)'` on every edited workflow and the new action: parses.
- `actionlint` on the five edited workflows: no new findings (one pre-existing SC2086 note in
  `pre-commit.yml`, untouched by this change).
- The watchdog's three preconditions were checked against harden-runner's source: `agent.pid` and
  `post_event.json` paths and the post step's "wait 10 s, then kill, then delete the pid file" sequence are
  as read from `src/install-agent.ts` and `src/cleanup.ts` at `main`.
- Not validated locally: the scheduled task itself needs a Windows runner. Validation is the first CI run on
  this branch (guard registers on every Windows job; `$RUNNER_TEMP\atmos-dns-guard.log` is not uploaded, so
  the observable signal is the job-level outcome) and the 48-hour measurement below.

Post-merge measurement (run from any checkout):

```bash
# True stuck Windows jobs: cancelled, every step succeeded, completion landed >= 5 min after the last step.
gh run list -R cloudposse/atmos -w test.yml --created ">=2026-09-04" -L 300 \
  --json databaseId,conclusion --jq '.[]|select(.conclusion!="success")|.databaseId' |
while read id; do
  gh api "repos/cloudposse/atmos/actions/runs/$id/jobs?per_page=100" --paginate --jq '
    def ts: sub("\\.[0-9]+Z$"; "Z") | fromdate;
    .jobs[] | select(.conclusion=="cancelled") |
    select(any(.labels[]?; test("windows"; "i")) or (.name|test("windows"; "i"))) |
    select(all(.steps[]; .conclusion=="success" or .conclusion=="skipped")) |
    select(((.completed_at|ts) - (.steps[-1].completed_at|ts)) >= 300) |
    [.run_id, .name] | @tsv'
done

# Did harden-runner restore DNS on its own in a given Windows job? (0 = the guard had to act)
gh api repos/cloudposse/atmos/actions/jobs/<job-id>/logs | grep -cE 'system DNS settings restored'
```

Success criteria: true stuck Windows jobs 0/day (23/day on Sept 3); no `infra`-labelled blocked calls on
Windows/macOS in StepSecurity Insights; no `Could not resolve host` on Windows checkout; no Trivy failures on
merge-queue runs.

## Follow-ups

- Upstream report to StepSecurity (harden-runner) — draft below; to be filed once approved.
- Phase 2 of the CI-stability plan (Windows Defender exclusions; restore-only toolchain cache and shipping
  toolchains in the build artifact; the Actions cache is a 10 GB LRU being churned by ~5 GB per run, so
  nothing saved from `main` survives) and Phase 3 (auto-rerun of infra-cancelled PR runs) are tracked in
  the follow-up issue linked from the PR.

### Upstream issue draft (step-security/harden-runner)

**Title:** Windows: post step's 10 s `done.json` wait races the agent's DNS restore; job never reports
completion

On GitHub-hosted `windows-latest` with harden-runner v2.21.0 (Windows agent v1.0.7-win), in both `audit` and
`block` mode, about 1.5 % of our jobs finish every step — including `Post Harden Runner` and `Complete job` —
and then never report a conclusion; GitHub cancels them 30–40 minutes later, well past `timeout-minutes`.

From the agent log appended to the post step output: healthy jobs end with
`[handlePostHardenRunnerEvent] restoring system DNS` → `system DNS settings restored` → `DNS proxy stopped`.
Stuck jobs log a second `timed out` from `handleWindowsCleanup` (the 10 × 1 s `done.json` wait), then
`stopping windows agent process` / `agent process stopped gracefully`, and the agent log stops at
`restoring system DNS` (or before it). The agent picked up `post_event.json` ~5 s after the post step began;
the restore takes ~2.4 s, so it loses the 10 s race a few percent of the time. The adapter is left on
`127.0.0.1` with no proxy behind it, and the runner cannot resolve the Actions service to complete the job.

Examples (cloudposse/atmos, public): stuck jobs 100694525046, 100532566143, 100127533462; healthy for
comparison 100688004653, 100532566215.

Requests: (1) wait for the DNS restore to finish (or restore DNS from the action as a fallback after killing
the agent); (2) consider implicitly allowing Windows/macOS OS endpoints in block mode (NCSI probes, WNS,
update/settings/telemetry, time sync, OCSP/CRL, Apple update/CDN) — they are blocked on every job today;
(3) on macOS, allowlisted hosts are sometimes dropped with `matched_policy: EXACT_IP` when the agent did not
see the DNS answer (e.g. `api.github.com:443` from Runner.Worker and from a Go test binary in
cloudposse/atmos run 33766077354, job 100692423806, while `api.github.com:443` was in `allowed-endpoints`).
