# Historical regression hypotheses

Read this when selecting workloads; follow the linked incident only when its failure mechanism
intersects the comparison. Some incidents describe bugs introduced with a feature, rather than
release-to-release regressions. Re-establish baseline behavior before classifying a new finding.

| Failure pattern | Paired experiment | Historical evidence |
| --- | --- | --- |
| Stricter processing rejects existing input | Override inherited lists with `{}`, null, scalars, and maps; compare resolved values and acceptance | [Native merge](../../../../docs/fixes/2026-03-24-deep-merge-type-mismatch-regression.md) |
| Path anchoring changes | Run from root and subdirectory, config in a nested directory, empty/bare/dot-relative base paths, and env overrides | [Path resolution](../../../../docs/fixes/path-resolution-regression.md) |
| Successful output breaks downstream tools | Parse captured JSON/YAML with CI/legacy-action environment enabled; inspect stdout/stderr independently | [CI stdout pollution](../../../../docs/fixes/2026-10-07-ci-legacy-action-stdout-pollution.md) |
| A new default adds side effects | Observe downloads, tool selection, locks, and writes for describe/version/dry-run commands, including offline and read-only fixtures | [Toolchain defaults](../../../../docs/fixes/2026-10-09-component-tool-versions-defaults.md) |
| Config is accepted but dropped before use | Set false/non-default values; trace through stack resolution and observe executor arguments/effects | [Helm namespace toggle](../../../../docs/fixes/2026-09-11-helm-create-namespace-stack-processor-drop.md) |
| Cache protection multiplies work | Repeated Terraform output/state/component references and store reads across stacks; check values, backend reads, and elapsed time | [Lookup caches](../../../../docs/fixes/2026-10-13-describe-affected-function-lookup-cache.md) |
| Caching mixes callers or identities | Alternate identities/config roots, disabled authentication after enabled authentication, cold/warm caches, concurrent consumers | [Deferred authentication isolation](../../../../docs/fixes/2026-09-24-deferred-authentication-isolation.md) |
| Shared mutation makes order/concurrency significant | Multiple components sharing inheritance, sequential reruns, concurrent workers; inspect values as well as crashes | [Concurrent map mutation](../../../../docs/fixes/2026-07-30-dag-concurrent-map-crash.md) |
| Environment looks like a product slowdown | Repeat on the same host without competing work and compare independent controls before attributing timing differences | [Degraded Windows runner](../../../../docs/fixes/2026-08-31-terraform-registry-cache-windows-runner-degradation.md) |

## Classification checks for skill maintenance

Use these scenarios to evaluate changes to the instructions:

- Both versions fail identically: unchanged bug, excluded from new regressions.
- Baseline succeeds and candidate crashes on the identical old workflow: regression.
- Intended unpinned change with a verified old-pin compatibility path: protected compatibility change.
- Intended change with no gate: compatibility gap; assess a new KindValue/KindBehavior transition.
- Journal entry exists but an old pin still executes new behavior: ineffective protection, not resolution.
- Values stay correct but repeated calls and significant elapsed time increase: performance regression.
- Candidate faster but uses another identity's cached value: correctness regression, not an improvement.
- Baseline cannot understand a new-only fixture: invalid upgrade comparison; use old syntax or mark unavailable.
- Timeout caused by unavailable dependencies or host degradation: inconclusive until controlled reproduction.

Historical prose can become stale. In particular, earlier edition notes predate executable
KindBehavior gates. Inspect today's implementation without assuming the framework retroactively
protects every documented historical transition.
