# CI build and required-check overhead

This is the third PR in the CI performance stack, after the workflow split
(#3180) and toolchain race sharding and fixtures (#3182).

## Measured baseline

The successful [Tests run for d77ce03b21ba](https://github.com/cloudposse/atmos/actions/runs/35264928200)
took **25m 11s**, compared with **30m 02s** before the toolchain work.
The longest race job completed around minute 19:39; it no longer determined
this run's completion time.

The critical path was the Mac build, Mac acceptance shard 1, and its required
gate. The Mac build spent **3m 55s** cross-compiling Intel after uploading the
native artifact. Acceptance shard 1 took **13m 31s**, including approximately
**2m 40s** restoring its 3.38 GB Go cache. The required gate then spent
**59s** invoking Mage, plus checkout and Go setup.

## Changes

- Restore a dedicated Intel Go object cache only before cross-compiling the
  Intel binary on ARM. The main-branch warmup builds and saves that same
  namespace. Native acceptance jobs continue to restore their native cache,
  without the Intel objects or a duplicate module archive. Keep two generations
  per exact Intel cache lineage, subject to the existing 24-hour pruning grace
  period. Both Mac artifact names and the Intel runtime tests stay unchanged.
- Run the three legacy acceptance gates with the existing dependency-free
  JavaScript action through `$/`, eliminating checkout, Go setup, and Mage
  compilation. Require every exact shard name once, wait for delayed job
  conclusions, retry transient API failures, and reject superseded attempts.
  Use the latest jobs listing so partial reruns include earlier successful
  shards. Keep the separate registry-cache result check and existing required
  check names.

The legacy gates remain temporary while `CI_SPLIT_WORKFLOWS` is disabled.
Remove them with `test.yml` after the standalone workflow rollout has passed
both PR and merge-group validation, as described in
[the rollout procedure](2026-09-17-ci-workflow-split.md).

## Validation and rollout

The Node tests exercise successful and partial reruns, pagination, missing or
duplicate shards, non-success conclusions, superseded attempts, eventual
consistency, and transient and definitive API failures. The cache pruning
suite includes the Intel lineage. Both the legacy and standalone Go test
workflows already run these suites.

The new Intel cache is cold until this warmup definition reaches `main` and
successfully runs there. PR jobs only restore it; they cannot seed the shared
main-branch cache. After merging the stack, run the main-branch warmup and
compare the next PR's Intel restore plus compile time, native cache size, and
required-gate duration with the baseline above.

These changes target measured overhead, not a demonstrated 20-minute result.
Windows acceptance finished around minute 21:48 in the baseline and could
become the next critical path. Its slowest shard spent approximately 3m 49s
restoring a 3.43 GB Go cache; measure the fresh, pruned warmup generation before
choosing the next cache or test-distribution change.
