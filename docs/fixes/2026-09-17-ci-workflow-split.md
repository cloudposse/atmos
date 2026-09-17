# Split CI by platform and test suite

The `Tests` workflow combined platform builds, acceptance shards, integration
suites, coverage, and required-check aggregation in one 2,408-line file. The new
workflows separate ownership and reruns without reintroducing the all-platform
build dependency. No reusable workflows are used.

## Workflow layout

| Entry point | Downstream workflows |
| --- | --- |
| `ci-build-linux.yml` | `ci-test-linux.yml`, `ci-test-floci.yml`, `ci-test-k3s-linux.yml`, `ci-test-integrations.yml` |
| `ci-build-macos.yml` | `ci-test-macos.yml`, `ci-test-k3s-macos.yml` |
| `ci-build-windows.yml` | `ci-test-windows.yml` |
| `ci-test-go.yml` | Independent Mage, noticegen, and race tests |
| `ci-test-kubernetes.yml` | Independent Kubernetes E2E tests |
| `ci-lint.yml` | Independent Docker scans and SARIF uploads |
| `ci-coverage.yml` | Event-driven fan-in of Linux acceptance and Mage coverage |
| `ci-report.yml` | Trusted API-only required-check reporting |

Go lint remains in its existing workflow. ARM macOS builds both Mac binaries;
Intel Colima tests still run on Intel. Artifact names, test matrices, runner
choices, and execution timeouts are preserved. Shared build, acceptance setup,
registry setup, mock demo, and k3s setup steps are composite actions referenced
with `$/`. Allowed endpoints remain one per line.

## Source identity and checks

Root workflows encode their exact `github.sha` and comparison base in `run-name`.
Child workflows encode the producer run ID and attempt in their own `run-name`.
These fields come from GitHub's run API, not downloaded artifacts. The resolver
checks the workflow path, event, repository, current attempt, latest root run,
and current PR head. A PR merge revision must contain the API-reported PR head.
Children check out that immutable merge revision and download artifacts from that
producer run. Merge-queue runs retain their synthetic commit identity.

The reporter uses the Checks API to preserve existing acceptance, mock demo, and
k3s check names on the tested revision, with GitHub Actions as the reporting app.
Build checks remain native direct-event checks. Required acceptance checks need
all ten successful shards plus the registry test; missing, duplicate, cancelled,
or skipped jobs cannot pass. The k3s gate requires every fixture on both platforms.
No ruleset changes are needed.

Every reporter invocation reconciles all platforms. A concurrency group per
source revision serializes writes; coalescing pending deliveries does not lose
another platform's result. The resolver rechecks source freshness before writes.
The jobs API gets bounded retries for eventual consistency. Failed-job reruns use
the latest job list, including successful jobs retained from earlier attempts.
A new build attempt invalidates children from the previous attempt.

The timing summary follows parent identities to include downstream runs under the
PR's head instead of main's SHA. Infrastructure reruns retain the existing Go
classifier, but resolve the original PR/push event before checking head freshness.
Mergify dispatches all direct-event entry points; the rollout switch selects the
active pipeline.

## Permissions and coverage

Test children explicitly request only read permissions, use restore-only caches,
and receive no repository secrets beyond the read-only GitHub token. Checkout does
not persist credentials. GitHub also enforces read-only access to default-branch
caches for `workflow_run` unless a write-capable `cache-mode` is declared; these
workflows explicitly declare `cache-mode: read`. SARIF publishing stays in a direct-event workflow.

Cache producers (builds, warmups, and Docker scanners) declare `write`, which
permits both restores and saves. Test consumers declare `read`; reporting,
coverage upload, timing summaries, and cache pruning declare `none`. Pruning uses
the Actions REST API with its existing permission, independently of cache service
access. This policy prevents accidental saves; it does not change retention or
reduce the size of existing archives. See the [cache-mode announcement](https://github.blog/changelog/2026-09-10-control-github-actions-cache-access-with-cache-mode/).
The required-check reporter has `checks: write` and executes only its trusted
bundled action, with no checkout or downloaded test code.

The coverage workflow waits for every Linux shard and the Mage job's artifacts;
it does not require race completion when those artifacts already exist. It checks
out trusted default-branch configuration and invokes the installed Go coverage
tool on data, never a PR executable or Mage package. It requires all ten native
coverage artifacts, filters mock files as before, and uploads with explicit source
commit, PR, and branch overrides. `CODECOV_TOKEN` is available only to the upload
step. Coverage upload retains the existing non-blocking behavior.

`workflow_run` definitions and their `$/` actions come from the default branch.
Changes to those actions require validation through the direct-event fallback or
local tests before rollout; checking out PR source does not select PR versions of
self-referenced actions. Cross-workflow cache scope also differs from PR scope:
children can restore main's caches but cannot depend on a PR-scoped producer cache.

## Rollout and rollback

GitHub does not trigger a new `workflow_run` file until it exists on the default
branch. Removing `test.yml` in the bootstrap PR would strand its required checks.

1. Merge the implementation with repository variable `CI_SPLIT_WORKFLOWS` unset
    or `false`. The existing `test.yml` pipeline remains active, using the extracted
    actions. New direct-event jobs are skipped under distinct `Split disabled /`
    names so they cannot satisfy the legacy required checks.
2. After all child files and actions are on main, enable the repository variable:

    ```shell
    gh variable set CI_SPLIT_WORKFLOWS --repo cloudposse/atmos --body true
    ```

3. Start a fresh PR run, then validate a merge-queue run. Confirm each required
    check appears on the source revision, all artifacts resolve to their producer,
    coverage uploads, and the timing summary includes the children. Exercise a
    failed-job rerun before removing the fallback.
4. After validation, remove `test.yml`, its disabled-name prefixes and Mergify
    dispatch, and the rollout guards in a cleanup PR. Keep the source resolver and
    trusted reporter: they are necessary for cross-workflow correctness.

To roll back, set the variable to `false` and start a fresh source run. Avoid
switching modes during an active merge-queue run. Disabled legacy jobs use
`Legacy disabled /` names and cannot satisfy the split pipeline's required checks.
No repository variables or rulesets are changed by this implementation PR.

The split improves maintenance and rerun granularity. Runtime improvements still
come from platform-specific dependencies, smaller caches, and faster test setup;
`workflow_run` adds scheduling overhead and must be measured after rollout.

## Validation

- Node tests cover source identity, stale PRs and attempts, missing/duplicate/
  skipped shards, platform aggregation, exact-SHA check publication, coverage
  readiness, real Go coverage merging, API pagination and bounded retries.
- Stopwatch tests cover parent resolution and inclusion of current-attempt child
  workflows; the checked-in action bundle is rebuilt.
- Go tests parse the platform YAML and reporter policy, reject broken shard and
  registry routes, and retain the Floci routing check.
- Compare the 26 moved build/test job definitions against the original: every job
  occurs once, with its matrix, runner, timeout, and defaults unchanged.
- Lint workflow syntax and run repository commit hooks. Local actionlint versions
  predating `$/` can validate an in-memory copy normalized to `./` references.
  Atmos's built-in validator now adapts self references in its parsed AST while
  retaining normal action/input validation and original diagnostic locations.
  It also validates the new workflow/job `cache-mode` enum while preserving
  unrelated syntax errors; regression tests cover valid and invalid policies.
  The EditorConfig commit hook now invokes its specific subcommand so the pinned
  release does not lint newer CI syntax; the affected-validation CI job uses the
  source-built artifact and continues to run every applicable validator.

Hosted cross-workflow behavior can only be validated after the default-branch
bootstrap and activation. This change does not claim a measured 20-minute runtime.
