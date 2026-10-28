# Terraform component-registry field fixtures

Disposable real-engine comparisons for baseline `512e6e5c32cc46e5d7fffee77b901a8847bb0dcf`
and candidate `c041bea2109bfd35ad8f7589eaf7351000a0da74` (PR #3545).
The field pass used Terraform **1.15.6**, OpenTofu **1.12.5**, and Go **1.27.1**.
These are investigation fixtures, not golden snapshots or a claim of complete coverage.

Always copy fixtures into a new disposable directory with its own Git repository,
state, HOME/XDG caches, and synthetic credentials. Lifecycle, output lookup,
vendoring, cache maintenance, and even dry-run commands can mutate files or backends.
Create linked worktrees only from those disposable repositories. Do not point these
fixtures at a production account, existing state bucket, or shared cache.

| Fixture | Purpose | Dependencies |
|---|---|---|
| `core/` | Existing-state upgrade, saved plans, policies, dependency freshness, selectors, hooks, editions, cancellation, streaming UI | Both pinned engines, Python 3 + PyYAML, Git; Helm/Helmfile for shared-caller check |
| `cache/` | Provider plugin cache, provider/module registry cache, filesystem mirrors, smart init | null provider 3.2.4; registry module cloudposse/label/null 0.25.0; Docker for trusted Linux proxy/offline tests |
| `sources/` | Directory/tagged local Git sources, aliases, workdirs, nested lookups and linked checkouts | Git, both engines; built-in terraform_data |
| `backend/` | S3 provisioning, emulator identity, SSM hooks and source/cache/backend combinations | Isolated Floci endpoint, dummy credentials; null 3.2.4 for combined variant |

## Core reproducers

Build the two immutable revisions with the same Go toolchain/settings in independent
source archives. Then run from the repository root, choosing a **new** output directory
for each command (the harness refuses to overwrite existing case roots):

```sh
python3 tests/fixtures/scenarios/terraform-registry-field-test/core/run.py \
  .context/my-field-run/core /absolute/path/atmos-baseline /absolute/path/atmos-candidate
python3 tests/fixtures/scenarios/terraform-registry-field-test/core/run_extra.py \
  .context/my-field-run/extra /absolute/path/atmos-baseline /absolute/path/atmos-candidate
python3 tests/fixtures/scenarios/terraform-registry-field-test/core/run_contracts.py \
  .context/my-field-run/contracts /absolute/path/atmos-baseline /absolute/path/atmos-candidate
python3 tests/fixtures/scenarios/terraform-registry-field-test/core/run_retry.py \
  .context/my-field-run/retry /absolute/path/atmos-baseline /absolute/path/atmos-candidate
```

The runners execute both engines separately. `results.json` contains assertions;
`commands/` contains argv, controlled environment overrides, stdout, stderr, status,
engine subprocess traces and hook events. Inspect assertions, not just the Python
process exit. Controlled Packer dispatch is explicitly a stub; real Packer validation
is recorded separately in the retained field report. Shared callers never deploy Helm
releases or publish images. The local stored-plan store has CI checks/comments disabled.

The PTY runner uses an actual controlling terminal and the real engine path, because
streaming UI subprocesses do not preserve the instrumentation wrapper's custom variable.
Cancellation preserves the configuration for the recovery command: changing HCL after
interrupting would test backend reconfiguration as well as cancellation.

## Retained evidence

The completed pass lives in
`.context/regression-test/terraform-registry-c041-20261027-01/`.
Its `REPORT.md` links the coverage ledger, reduced reproducers, raw observations,
paired-check JSON, builds, and ten-sample Mage benchmark comparisons. The cache,
source, and backend directories retain their additional runtime harnesses. They use
run-specific paths and must be configured/copied to a fresh run before reuse.

Fixture origins include existing lifecycle/dependency examples, source/workdir
scenarios, `terraform-component-mocks-fallback`, and `aws-store-hooks-floci`.
No product implementation, API, or golden snapshots were changed by this pass.
