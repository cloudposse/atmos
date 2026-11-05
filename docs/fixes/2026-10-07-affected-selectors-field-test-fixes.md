# Fix: `--tags` / `--labels` selectors in CI no longer hide automated dependents, run non-matching dependents, or leak across commands

**Date:** 2026-10-07

## Summary

A second hands-on field test of the `--tags` and `--labels` selectors found seven problems in how they behave in a real CI setup. A privileged parent component hid its automated dependents from `atmos describe affected`. `atmos terraform` ran dependents that did not match the selectors. `ATMOS_TAGS` and `ATMOS_LABELS` leaked into unrelated commands. `describe affected --upload` failed under a job-level selector. Deleted locked components survived `--exclude-locked`. Parse errors did not say where the bad value came from. The job matrix never included dependents. Each is fixed, and the docs, blog post, and agent skills now describe the behavior that ships.

## Context

The first field-test pass is recorded in `2026-10-07-affected-selectors-deleted-and-dependents.md`. This pass used a real stack fixture (`tests/fixtures/scenarios/atmos-describe-affected-with-selectors`, with `stacks/` as HEAD and `stacks-affected/` as the BASE overlay) and a CI-style environment. It found:

- **A privileged parent hid automated dependents.** With `--include-dependents --labels=ci=auto`, a changed `iam` (`ci: manual`) was dropped from the top level, and its automated dependent `app` (`ci: auto`) was dropped with it, because the selectors ran before the dependents were resolved. The dependent needed a run and never got one.
- **Terraform ran non-matching dependents.** `atmos terraform plan --affected --include-dependents --labels=ci=auto` planned every dependent added by `--include-dependents`, including privileged ones, while `describe affected` pruned them. The two commands disagreed about the same selection.
- **`ATMOS_TAGS` and `ATMOS_LABELS` leaked.** Viper's `AutomaticEnv()` resolves a bare `tags` or `labels` key from those variables, so a job-level export narrowed `atmos list *`, `atmos vendor *`, and `atmos auth list`, and turned `atmos terraform plan vpc -s dev` into a conflicting multi-component invocation.
- **`--upload` failed under a job-level variable.** The upload is always unfiltered and rejects selectors, so a workflow that exported `ATMOS_LABELS` for the matrix step broke the upload step in the same job.
- **Deleted locked components survived `--exclude-locked`.** A component that was `locked: true` in BASE and deleted in HEAD was still reported, because the deleted path never read the locked flag from the BASE metadata.
- **Parse errors lacked context.** A malformed `--labels` value reported the pair but not whether it came from the flag or from `ATMOS_LABELS`, and gave no hint about the expected format.
- **The matrix never included dependents.** The nested `dependents` lists are not part of a job matrix, so there was no way to run the dependents of the changed components from a matrix.

Docs also carried claims that the field test showed were wrong: that labels render only when `templates.settings.enabled: true` is set (templates render by default), that locked components are never reported (deleted ones were), and that an empty matrix is an empty string (it is `{"include":[]}`, so a `matrix != ''` job guard never skips).

Items found in the same pass and deliberately left alone, because they predate the selectors or sit outside this change:

- The order of top-level entries from `describe affected` is not deterministic across runs.
- The stack manifest schema rejects `stack:` under `settings.depends_on`.
- The `--schemas-*-dir` flags exist only as constants and are not registered.
- With `--include-spacelift-admin-stacks` and no `--include-dependents`, the Spacelift admin-stack entries bypass the selectors.

## Changes

- **Deferred selectors with dependents (`describe affected`).** With `--include-dependents` and a selector, the affected set is computed with the selectors deferred (`AffectedFilter.DeferSelectors`). After the dependents are resolved, a single pass (`applySelectorsToAffectedForest`) prunes the nested dependents, drops non-matching top-level entries, and promotes the matching dependents of a dropped entry to the top level with `affected: dependent` and `affected_all: [dependent]`. Matching dependents of a pruned dependent move up to the nearest remaining ancestor. Deleted components are still matched while the affected set is computed, because their metadata exists only in BASE and they have no dependents. `FinalizeAffectedDependents` is the one path for dependents resolution and shaping.
- **`--flatten`.** New flag (`ATMOS_DESCRIBE_AFFECTED_FLATTEN`) that lifts every remaining dependent into the top-level list as `affected: dependent`, so the `json`, `yaml`, and `matrix` outputs can carry dependents. It requires `--include-dependents` and is rejected with `--upload`. A value that came from the environment is dropped with a warning instead of failing.
- **`--exclude-locked` and deleted components.** A deleted component is excluded when `metadata.locked` is true in its BASE metadata.
- **Selector errors and `--upload`.** `--labels` parse errors name the flag or the environment variable (`pkg/tags` `ParseLabelsFlagFrom`) and carry a hint. With `--upload`, selectors that came from `ATMOS_TAGS` or `ATMOS_LABELS` are ignored with a warning, while explicit command-line selectors still fail, with hints (including how to override the environment values with `--tags= --labels=`).
- **`list affected --include-dependents`.** Now resolves the dependents through `FinalizeAffectedDependents` and emits `affected: dependent` rows, with the same selector rules.
- **Terraform parity.** `--affected` and `--all` with `--include-dependents` and `--tags` or `--labels` now prune closure-added dependents that do not match (`pkg/dependency` `FilterSelection`, `pkg/scheduler/adapters/terraform.go`, `pkg/list/dependencies`). Dropped intermediates are contracted so the surviving components keep their dependency order. Prerequisites from `--include-dependencies` are unchanged.
- **Environment leak.** `list`, `vendor`, and `auth list` register their `--tags` and `--labels` flags under namespaced Viper keys (`list.tags`, `vendor.tags`, `auth.tags`, and so on) through the new `flags.WithViperKey` option, so `AutomaticEnv()` no longer resolves `ATMOS_TAGS` or `ATMOS_LABELS` for them. Their own variables are bound explicitly: `ATMOS_COMPONENT_TAGS` and `ATMOS_COMPONENT_LABELS` for `list`, `ATMOS_VENDOR_TAGS` and `ATMOS_VENDOR_LABELS` for `vendor`, and `ATMOS_AUTH_TAGS` for `auth list`. `atmos terraform` ignores selectors that came only from the environment when a component argument is given. A flag annotation (`flags.MarkFlagValueFromEnv`) records which variable supplied a value so validation can tell an ambient value from a typed one.
- **Tests.** A real-stack regression test (`tests/describe_affected_selectors_test.go`) covers `ci=auto` with dependents, the same with `--exclude-locked`, the same with `--flatten`, `ci=manual`, `--tags=security` with and without dependents, the unfiltered path (16 top-level entries), and a parity check against the terraform graph filter on the described HEAD stacks. Unit tests cover the forest pruning and promotion, flatten, the deleted-locked rule, selector parsing and sources, the dependency `FilterSelection`, the scoped closure, and per-command environment isolation for `list`, `vendor`, `auth`, and `terraform`.
- **Docs, blog post, and agent skills.**
  - `describe affected` documents case-sensitive matching, last-wins duplicate label keys, and that a component without `metadata` never matches. It also documents `--flatten`, `--output-file` (`matrix=` and `count=`), the full pruning and promotion rule, the `--exclude-locked` change, the `--upload` behavior for environment selectors, and a caution about job-level `ATMOS_TAGS` and `ATMOS_LABELS`.
  - `list affected` documents its own `ATMOS_COMPONENT_TAGS` and `ATMOS_COMPONENT_LABELS` variables and the `dependent` rows.
  - The native CI guide, the GitHub Actions deploy examples, the `atmos-ci` and `atmos-migration` agent skills, and the earlier matrix blog post guard the downstream job on a `count` output (`count != '0'`) instead of `matrix != ''`.
  - Templates are described as rendered by default. `metadata.mdx` lists `describe affected` and `list affected` as `tags` and `labels` consumers.
  - The selectors blog post describes the promotion rule, `--flatten`, the terraform `--include-dependents` behavior change, and the `count` guard.

## Validation

- `gofumpt -l -w tests/describe_affected_selectors_test.go`: clean. `go vet ./tests/`: pass.
- `go test ./tests/ -run TestDescribeAffectedSelectors -v` skips in this worktree, because `RequireGitRemoteWithValidURL` rejects the `worktreeconfig` repository extension (`Not in a Git repository: core.repositoryformatversion does not support extension: worktreeconfig`).
- `ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true go test ./tests/ -run TestDescribeAffectedSelectors -v`: pass, all 8 sub-tests (`labels ci=auto with dependents`, `... and exclude-locked`, `... and flatten`, `labels ci=manual with dependents`, `tags security`, `tags security with dependents`, `no selectors with dependents`, `terraform parity with flattened describe affected`). With `ci=manual`, the changed `chain/base` (`ci: auto`) is dropped and its manual dependent `chain/middle` is promoted to a top-level `dependent` entry, which is the documented promotion rule.
- `cd website && npm run build`: pass (about 3 minutes), with no broken-link or broken-anchor warnings.
- Agent-skill limits: `agent-skills/skills/atmos-ci/SKILL.md` is 415 lines and about 19 KB (under 500 lines and 20 KB), and `references/native-ci.md` is about 15 KB (under 25 KB).
- `atmos --chdir=demo/casts casts generate screengrabs cli --filter "describe affected"` and `--filter "list affected"` regenerated `atmos-describe-affected--help.cast` (now lists `--flatten`) and `atmos-list-affected--help.cast`. `atmos --chdir=demo/casts casts validate screengrabs cli`: pass.
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh docs/fixes/2026-10-07-affected-selectors-field-test-fixes.md`: pass.
- `git diff --check` on the changed website, agent-skills, docs, and test files: clean.
- Live-binary verification with `./build/atmos` (built from this branch) run from `tests/fixtures/scenarios/atmos-describe-affected-with-selectors` against a temporary BASE repository created from `stacks-affected/`:
  - `describe affected --repo-path <base> --include-dependents --labels=ci=auto` lists `app` as a top-level `dependent` entry and no longer lists `iam`; `chain/leaf` is nested under `chain/base` and `chain/middle` is absent. With `--flatten`, `chain/leaf`, `app`, and every `tgw/attachment` appear top-level as `dependent` with empty `dependents`.
  - `--exclude-locked --labels=ci=auto` no longer lists the deleted, locked `legacy-locked`.
  - `--labels=ci` fails with `invalid label "ci" for --labels (or ATMOS_LABELS), expected key=value or key:value` and a hint; `--flatten` without `--include-dependents` fails with a hint.
  - `ATMOS_LABELS=ci=auto atmos describe affected --upload` prints `Ignoring ATMOS_LABELS: --upload always uploads the unfiltered affected set` and proceeds to the upload (which then fails only on the missing OIDC token); `--labels=ci=auto --upload` fails with the two hints.
  - `ATMOS_LABELS=ci=manual atmos list components` and `ATMOS_TAGS=security atmos list stacks` are unfiltered; `ATMOS_COMPONENT_LABELS=ci=manual atmos list components` still filters to the four manual components; `ATMOS_LABELS=ci=manual atmos list affected` is unfiltered.
  - `ATMOS_TAGS=security atmos terraform plan iam -s ue1-network --dry-run` runs (debug log: `Ignoring ATMOS_TAGS because a component argument was given`); `--tags=security` with a component argument still errors.
  - `terraform plan --affected --include-dependents --labels=ci=auto --repo-path <base> --dry-run` plans `app`, `chain/base` then `chain/leaf`, and never `chain/middle`; `terraform destroy` with the same flags orders `chain/leaf` before `chain/base`.
  - `list affected --repo-path <base> --include-dependents --labels=ci=auto` emits `dependent` rows for `app`, `chain/leaf`, and the four `tgw/attachment` instances.
- `atmos test` (short suite): every package passes; the `tests` package built cleanly on a re-check after the new test file landed.

## Follow-ups

None.
