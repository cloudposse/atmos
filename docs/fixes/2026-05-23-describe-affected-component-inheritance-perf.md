# Fix: `atmos describe affected` takes ~11 minutes on GitHub Actions

**Date:** 2026-05-23

**Issue:**

Users running `atmos describe affected` in GitHub Actions on a
large-scale Atmos configuration (≈836 stack YAML files, ~195 final
stacks across three namespaces, ~9.3k component instances) reported
**~11-minute** wall-clock times on the standard GHA runner on
v1.219.0 and earlier. The same workload reproduced locally in
**4m6s** with `--identity=false` and fake AWS credentials in env (no
AWS API calls). Heatmap profiling pointed at two structural problems
that grew since the `pkg/perf` instrumentation arc shipped in
October 2025 (PRs #1576/#1611/#1622/#1639):

1. **`--identity=false` was a lie (FIXED in v1.220.0-rc.1, PR #2471).**
   In v1.219.0 and earlier, Atmos still constructed a credential
   store per component instance even with `--identity=false`:
   - `pkg/auth/credentials/store.go:67` — `NewCredentialStoreWithConfig`
     called **12,124 times** (≈1.3× per component) — **2m36s** of
     cumulative CPU time, 13ms avg, 23ms P95.
   - `pkg/auth/credentials/keyring_system.go:187` —
     `systemKeyringStore.Delete` called **6,062 times** — **1m15s** of
     cumulative CPU time, 12ms avg, 22ms P95.
   - Combined: **~3.5 minutes** of CPU time spent spinning up and
     tearing down credential stores. PR #2471 (`fix(auth): honor
     --identity=false in describe affected and dependents`) gated
     credential-store creation behind the actual identity-resolution
     decision. Confirmed in a local reproduction against
     current `main`: **zero calls** to
     `credentials.NewCredentialStoreWithConfig` and
     `credentials.systemKeyringStore.Delete`. Users on v1.219.0
     should upgrade to v1.220.0-rc.1 (or wait for v1.220.0) to
     reclaim this 3.5 min of CPU time.

2. **Component inheritance dominates the rest (REMAINING).** A
   9,261-component workload hits the inheritance pipeline once per
   instance, and each leg is ~38-46ms. **This is unchanged by
   #2471** and is the remaining optimization target:
   - `internal/exec/stack_processor_process_stacks_helpers.go:113` —
     `processComponent`: **9,261 calls, 5m49s total, 38ms avg, 171ms P95**.
   - `internal/exec/stack_processor_process_stacks_helpers_inheritance.go:15` —
     `processComponentInheritance`: **9,261 calls, 5m47s total, 37ms
     avg, 170ms P95**.
   - `internal/exec/stack_processor_utils.go:1623` —
     `ProcessBaseComponentConfig`: **8,017 calls, 5m45s total, 43ms
     avg, 170ms P95**.
   - `internal/exec/stack_processor_process_stacks_helpers_inheritance.go:157` —
     `processInheritedComponent`: **4,845 calls, 5m43s total, 71ms
     avg, 175ms P95**.
   - `internal/exec/stack_processor_process_stacks_helpers_inheritance.go:91` —
     `processMetadataInheritance`: **9,261 calls, 5m41s total, 37ms
     avg, 170ms P95**.
   - `internal/exec/stack_processor_cache.go:107` —
     `cacheBaseComponentConfig`: **6,431 calls, 5m33s total, 52ms avg,
     160ms P95**.
   - `internal/exec/stack_processor_merge.go:40` —
     `mergeComponentConfigurations`: **9,261 calls, 1m34s total, 10ms
     avg, 38ms P95**.

   That is **~5.7 minutes of CPU work** in the inheritance pipeline,
   partially parallelized across cores. The high P95s (~170ms across
   most legs) indicate the slow tail is not amortizing.

YAML function evaluation is **not** the problem:

- `merge.WalkAndDeferYAMLFunctions`: **685,202 calls, 1m13s total,
  106µs avg, 326µs P95** — cheap per call despite the call count.
- `utils.ProcessIncludeTag`: **1,085 calls, 3s total, 2.8ms avg, 24ms
  P95** — negligible.

Counter-intuitively, `--process-functions=false` made the run
**slower** (5m35s vs 4m52s) — the flag short-circuits cheap work that
was already amortized while leaving the expensive inheritance path
untouched.

### Post-#2471 fresh heatmap (2026-05-23, local Mac, current main)

Reproducer below, `--identity=false --process-templates=false
--process-functions=false`, comparing HEAD vs HEAD~1 via worktree.

**Elapsed: 3.44 s wall-clock locally.** Cumulative CPU time across
9,261 component instances ≈ 6m11s (parallelism ≈ 108× on this
machine; on a 2-4 core GHA runner the wall-clock projects to
**~1.5-3 minutes for the inheritance pipeline alone**).

| Function | Calls | Total | Avg | P95 |
|---|---:|---:|---:|---:|
| `exec.processComponent` | 9,261 | 6m11s | 40ms | 192ms |
| `exec.processComponentInheritance` | 9,261 | 6m02s | 39ms | 191ms |
| `exec.ProcessBaseComponentConfig` | 8,017 | 6m02s | 45ms | 192ms |
| `exec.processInheritedComponent` | 4,845 | 6m00s | 74ms | 198ms |
| `exec.processMetadataInheritance` | 9,261 | 6m00s | 39ms | 189ms |
| `exec.cacheBaseComponentConfig` | 6,431 | 5m50s | 55ms | 180ms |
| `exec.mergeComponentConfigurations` | 9,261 | 1m35s | 10ms | 40ms |
| `exec.ProcessStackConfig` | 744 | 1m11s | 96ms | 239ms |
| `exec.ProcessYAMLConfigFileWithContext` | 1,518 | 1m07s | 44ms | 265ms |
| `exec.processComponentsInParallel` | 719 | 1m06s | 92ms | 223ms |
| `merge.MergeWithDeferred` | 83,349 | 1m02s | 752µs | 4ms |
| `merge.WalkAndDeferYAMLFunctions` | 527,765 | 54s | 102µs | 311µs |
| `utils.processCustomTags` | 55,156 | 38s | 680µs | 390µs |
| `merge.Merge` | 273,379 | 27s | 98µs | 415µs |
| `utils.UnmarshalYAMLFromFileWithPositions` | 22,181 | 16s | 716µs | 5ms |
| `exec.extractAndAddLocalsToContext` | 22,181 | 14s | 617µs | 3ms |
| `exec.extractLocalsFromRawYAML` | 22,181 | 13s | 572µs | 3ms |
| `credentials.NewCredentialStoreWithConfig` | **0** | — | — | — |
| `credentials.systemKeyringStore.Delete` | **0** | — | — | — |

**Key observations:**

- Credential store work is gone (0 calls) — #2471 confirmed.
- Inheritance pipeline numbers are stable vs the v1.219.0 baseline:
  `processComponent` 5m49s → 6m11s, `cacheBaseComponentConfig` 5m33s
  → 5m50s. Within noise; nothing has improved.
- New surface area visible at the bottom of the table:
  `extractAndAddLocalsToContext` + `extractLocalsFromRawYAML` add
  ~26s combined across 22,181 invocations each. The `locals` feature
  added after #1639 is a small but real contributor.
- `cacheBaseComponentConfig` remains a paradox: 6,431 calls × 55ms
  avg = 5m50s, almost as expensive as the un-cached path. The cache
  is doing work on insert that should be amortized.
- `processComponentsInParallel` runs 719 times (one per stack)
  fanning out into 9,261 component invocations — so parallelism
  exists at the **stack** level but not at the **component-within-a-stack**
  level. The 5.7-6 min of CPU work is bound by stack count, not
  component count.

### Affected detection note

In the reproducer, the result set is empty (`[]`) even though a
catalog file was modified. Atmos compares **resolved configs**, and
the comment-only marker doesn't change the resolved output. The
expensive part — the full stack processing on both refs — happens
regardless: Atmos must compute both worktrees' resolved configs to
diff them. **CI pain is the full processing pass, not the affected
result size.**

## Status

**Phase 1 fixed in PR #2471 (v1.220.0-rc.1).** Phases 2 and 3 still
open. Optimization plan:

1. **~~Make `--identity=false` actually skip credential store
   creation.~~ ✅ Shipped in PR #2471 (`fix(auth): honor
   --identity=false in describe affected and dependents`), commit
   `a0cd28671`, in `v1.220.0-rc.1`.** Reclaimed ~3.5 minutes of
   credential-store CPU time. Confirmed zero credential calls in the
   2026-05-23 reproduction.
2. **Cache hit-rate audit of the inheritance pipeline (OPEN).**
   `cacheBaseComponentConfig` is itself hot (6,431 calls, 5m50s),
   which suggests either (a) the cache is being invalidated too
   eagerly, (b) the cache key is too specific and missing equivalent
   configurations, or (c) it is doing real work on first insertion
   that should happen elsewhere. Companion: `getCachedBaseComponentConfig`
   8,017 calls / 6.5s suggests reads are fast, so the cost is concentrated
   on the insert path.
3. **Parallelize the inheritance pipeline more aggressively (OPEN).**
   `processComponentsInParallel` runs 719 times (one per stack)
   fanning out into 9,261 component invocations. Stack-level
   parallelism is in place (PR #1639 work); **component-level
   parallelism within a stack is the next layer**. With ~13
   components per stack on average and modern many-core runners,
   pushing parallelism inside `processComponentsInParallel` should
   yield 2-3× wall-clock improvement on the inheritance pipeline.
4. **`locals` extraction (NEW, OPEN).**
   `extractAndAddLocalsToContext` + `extractLocalsFromRawYAML` add
   ~26s combined across 22,181 invocations each. The `locals`
   feature shipped after #1639 and was never put through the
   optimization taxonomy.
5. **GHA container speed factor.** Even with #2471's win, the
   remaining ~6m of CPU inheritance work projects to ~1.5-3 min on
   a 2-4 core GHA runner. If users still see 5+ min on GHA after
   upgrading to v1.220.0, Phase 2 + Phase 3 are required.

### Progress checklist

- [x] Reproduce locally with fake AWS credentials and `--identity=false`.
- [x] Capture v1.219.0 baseline heatmap (4m6s elapsed, 9,261
      components, 12,124 credential calls).
- [x] Identify hot paths (component inheritance + credential store).
- [x] Document repo scale (836 YAML files, 35k lines, 195 stacks,
      2,114 imports).
- [x] Run `atmos describe affected` against current main and capture
      fresh heatmap — confirms #2471 eliminated credential calls and
      isolates the inheritance pipeline as the remaining hot path.
- [x] Phase 1: gate credential store creation on actual identity
      resolution (shipped in PR #2471).
- [x] Phase 2: eliminate cache-write lock contention (sync.Map +
      deep-copy outside lock). `cacheBaseComponentConfig` Total
      dropped 98.8% (5m50s → 4s), inheritance pipeline Total dropped
      91-94%. Race-clean. Bottleneck shifted to
      `mergeComponentConfigurations`.
- [ ] Phase 3: parallelize component-level inheritance within
      `processComponentsInParallel`.
- [ ] Phase 4: optimize `locals` extraction path.
- [ ] Phase 5 (new): optimize `mergeComponentConfigurations` —
      newly visible 2m22s hot path post-Phase-2.
- [ ] Re-measure on GHA and confirm wall-clock improvement.

---

## Problem

### Repro environment

Customer infrastructure repository, redacted scale:

| Metric | Value |
|---|---|
| Stack YAML files | **836** |
| Stack YAML lines | **35,335** |
| Resolved stacks (final) | **~195** (~65 per namespace × 3) |
| `import:` statements (transitive) | **2,114** |
| Max imports in a single file | **203** |
| Catalog component definitions | **65** (487 component instances) |
| Terraform components in `components/terraform/` | **68** |
| Component instances processed end-to-end | **9,261** |
| Auth profiles | 9 (each with 13-14 identities) |

`atmos.yaml` highlights:
```yaml
stacks:
  base_path: "stacks"
  included_paths: ["orgs/**/*"]
  excluded_paths: ["**/_defaults.yaml"]
  name_pattern: "{namespace}-{tenant}-{environment}-{stage}"

templates:
  settings:
    enabled: true
    sprig: { enabled: true }
    gomplate: { enabled: true }
```

### Reproducer

Modify a heavily-imported catalog file (e.g.,
`stacks/catalog/aws-team-roles/defaults.yaml`) — anything not
matched by the config's `excluded_paths` glob. **Note:**
modifying `**/_defaults.yaml` files does *not* work for this
purpose because the customer's `atmos.yaml` has them in
`excluded_paths`. Commit the change locally, then run with a
git worktree of the previous HEAD:

```bash
# In the customer repo:
git worktree add /tmp/<repo>-base HEAD~1   # uses parent's remote URL
export AWS_ACCESS_KEY_ID=fake AWS_SECRET_ACCESS_KEY=fake
atmos describe affected \
  --repo-path /tmp/<repo>-base \
  --process-templates=false \
  --process-functions=false \
  --identity=false \
  --heatmap --heatmap-mode=table
```

`git clone --local` does *not* work here because the cloned remote's
URL is a local path and Atmos's git URL parser rejects empty hosts
(`repository host '' not supported`). Use `git worktree add`
instead — it shares the parent repo's remote URL.

Even with a comment-only catalog change producing **0 affected
stacks** in the output, the full processing pass still happens on
both refs to compute the diff. The heatmap captures the actual CI
workload.

### Baseline heatmap (collected 2026-05-21, local, no AWS calls)

**Elapsed: 4m6s.** Top functions by cumulative CPU time:

| Function | Calls | Total | Avg | P95 |
|---|---:|---:|---:|---:|
| `exec.processComponent` | 9,261 | 5m49s | 38ms | 171ms |
| `exec.processComponentInheritance` | 9,261 | 5m47s | 37ms | 170ms |
| `exec.ProcessBaseComponentConfig` | 8,017 | 5m45s | 43ms | 170ms |
| `exec.processInheritedComponent` | 4,845 | 5m43s | 71ms | 175ms |
| `exec.processMetadataInheritance` | 9,261 | 5m41s | 37ms | 170ms |
| `exec.cacheBaseComponentConfig` | 6,431 | 5m33s | 52ms | 160ms |
| `credentials.NewCredentialStoreWithConfig` | 12,124 | 2m36s | 13ms | 23ms |
| `exec.mergeComponentConfigurations` | 9,261 | 1m34s | 10ms | 38ms |
| `credentials.systemKeyringStore.Delete` | 6,062 | 1m15s | 12ms | 22ms |
| `merge.WalkAndDeferYAMLFunctions` | 685,202 | 1m13s | 106µs | 326µs |
| `utils.ProcessIncludeTag` | 1,085 | 3s | 2.8ms | 24ms |

(Cumulative times overlap — `processComponent` wraps
`processComponentInheritance` wraps `ProcessBaseComponentConfig`,
etc. The chain accounts for ~5.7 min of CPU work parallelized across
cores ⇒ ~4 min wall-clock locally, ~10 min on GHA.)

### Why this is worse now than when the perf arc shipped

The October 2025 optimization PRs (#1576/#1611/#1622/#1639) targeted
the stack-processing pipeline as it existed at that time. Since then,
Atmos has gained:

- **Auth subsystem** (`pkg/auth/`) — providers, identities, keyring,
  credential store. The credential-store-per-component path that now
  accounts for 3.5 min did not exist in #1639's instrumentation.
- **Profiles subsystem** (`pkg/auth/` + per-profile `atmos.yaml`) —
  loaded once but interacts with identity resolution per component.
- **`locals` block** at stack and component level — additional merge
  passes in the inheritance pipeline.
- **AI / MCP / AWS security** packages — not on the hot path here,
  but consume `perf.Track` slots that may have grown.

The `pkg/perf` infrastructure and the `--heatmap` flag are still
exactly the right tools for this — the discipline survives. What's
missing is a second pass of the #1639 optimization taxonomy
(algorithm / caching / concurrency / I/O / cache correctness) over
the post-#1639 code.

### Code path

1. `atmos describe affected --repo-path <base>` enters
   `internal/exec/describe_affected.go`.
2. For each stack, `ExecuteDescribeStacks` walks components and
   calls `processComponent` in
   `internal/exec/stack_processor_process_stacks_helpers.go:113`.
3. `processComponent` calls `processComponentInheritance`
   (`stack_processor_process_stacks_helpers_inheritance.go:15`).
4. Inheritance recurses through `processInheritedComponent` and
   `processMetadataInheritance`, both of which call
   `ProcessBaseComponentConfig`
   (`stack_processor_utils.go:1623`) — the heaviest leg at 43ms avg.
5. Each invocation hits `cacheBaseComponentConfig`
   (`stack_processor_cache.go:107`) — itself accounting for 5m33s of
   total time across 6,431 calls. Either the cache is missing too
   often, or it is doing expensive work on insert that should be done
   once.
6. Independently, **per-component auth resolution**
   (`internal/exec/describe_stacks_component_processor.go`) calls
   `credentials.NewCredentialStoreWithConfig` even when
   `--identity=false`. Each call constructs a `systemKeyringStore`
   that is then `Delete`d, contributing the ~3.5 min.

### Why `--identity=false` does not skip the credential store

The flag prevents AWS role assumption but the per-component
processor still constructs a credential store as part of the auth
manager bootstrap. The two are conflated: identity *resolution* is
gated by `--identity`, but identity *infrastructure* (the credential
store backing it) is not. With 9,261 components, the store gets
built 12,124 times (some components retry / re-resolve) and the
keyring delete fires 6,062 times.

A short-circuit at the credential-store factory — return a no-op
store when identity resolution is disabled — eliminates 3.5 minutes
of CPU time without touching the inheritance pipeline.

## Fix

### Phase 1 (shipped in PR #2471, `a0cd28671`, v1.220.0-rc.1)

**~~Credential store gating~~.** Already done. Locations touched:

- `pkg/auth/credentials/store.go` — credential store creation gated
  on the actual identity-resolution decision.
- `internal/exec/describe_stacks_component_processor.go` — per-component
  auth resolver short-circuits when `--identity=false` is in effect.

Confirmed impact: **0 credential calls** in the 2026-05-23 local
reproduction against current main. Users on v1.219.0 should upgrade.

### Phase 2 (shipped 2026-05-23) — cache lock contention

**Root cause:** `cacheBaseComponentConfig` was holding the global
`sync.RWMutex.Lock()` for the *entire* function — including a
~525µs `deepCopyBaseComponentConfigMaps` call. With ~9k component
instances processed concurrently across stacks, the exclusive write
lock serialized every cache write. The "55ms avg per call" in the
baseline was almost entirely **lock-wait time**, not real CPU work.

**Two changes:**

1. **`baseComponentConfigCache` is now a `sync.Map`** (was a
   `map[string]*BaseComponentConfig` + `sync.RWMutex`). sync.Map is
   purpose-built for disjoint-key write patterns and has no global
   lock.
2. **Deep copy moved outside the critical section.** In
   `cacheBaseComponentConfig`, the `BaseComponentConfig` is now
   deep-copied *before* the `sync.Map.Store`, not while holding any
   lock. The cached pointer's target is immutable post-insert, so
   `getCachedBaseComponentConfig` deep-copies outside the
   `sync.Map.Load` too. Both reads and writes now have no contention
   coupling.

**Confirmed impact (local re-measure, mean of 3 runs):**

| Function | Before (Total / Avg) | After (Total / Avg) | Reduction |
|---|---:|---:|---:|
| `cacheBaseComponentConfig` | 5m50s / 55ms | **~4.0s / ~640µs** | **−98.8% Total, 86× faster avg** |
| `getCachedBaseComponentConfig` | 6.5s / 855µs | **~880ms / ~110µs** | **−86% Total, 8× faster avg** |
| `processComponent` | 6m11s / 40ms | **35s / 3.8ms** | **−91% Total** |
| `processComponentInheritance` | 6m02s / 39ms | **28s / 3.0ms** | **−92% Total** |
| `ProcessBaseComponentConfig` | 6m02s / 45ms | **22s / 2.8ms** | **−94% Total** |
| `processInheritedComponent` | 6m00s / 74ms | **22s / 4.6ms** | **−94% Total** |
| `processMetadataInheritance` | 6m00s / 39ms | **23s / 2.5ms** | **−94% Total** |

The pre-fix `Total` was mostly lock-wait, not work. Post-fix
numbers reflect real CPU cost.

**Wall-clock note.** On a many-core local Mac (parallelism ≈ 100×),
wall-clock barely moved (3.4s → 4.1s mean) — the lock-waiters were
parking, not consuming CPU. On a **2-4 core GHA runner**, where
contention cannot be hidden behind parallelism, the improvement
projects to **multiple minutes**: with ~30 minutes of synthetic
"Total" time eliminated and real CPU work staying serial-bound on
few cores, the lock-wait was effectively wall-clock on small
runners. End-to-end GHA validation pending.

**Race detector:** `go test -race -count=1 -run "Cache|cache|Base|Inheritance|inherit" ./internal/exec/...`
green.

**Bottleneck shifted:** post-Phase-2, the heaviest function is now
`mergeComponentConfigurations` (2m22s total, 15ms avg, 9,261
calls), followed by `merge.MergeWithDeferred` (1m35s, 752µs avg,
83,349 calls). These are the next optimization targets after Phase
3 lands.

### Phase 3 (open) — component-level parallelization

`internal/exec/stack_processor_process_stacks_helpers.go` runs
`processComponent` serially within each stack's
`processComponentsInParallel`. 9,261 components ÷ 719 stacks ≈ 13
components per stack — substantial parallelism opportunity at the
component level. Worker pool inside the stack-processing goroutine
would push the inheritance pipeline from ~108× parallelism (current,
on an M-series Mac) to a higher ceiling on the same hardware, and
materially help GHA's 2-4 core runners where current parallelism is
capped at the core count.

Expected impact: 2-3× wall-clock improvement on the inheritance
pipeline. On GHA's 2-4 cores, this matters more than on a laptop
because the current parallelism is already saturated by stack count.

### Phase 4 (open) — locals extraction

`extractAndAddLocalsToContext` (22,181 calls, 14s) and
`extractLocalsFromRawYAML` (22,181 calls, 13s) are the post-#1639
additions for the `locals` feature. 22,181 = ~30 imports per
stack × ~744 stacks processed — locals are extracted per imported
file, not per component. Likely caching opportunity: extract once
per file, cache by file path + mtime.

Expected impact: -20s CPU on this workload. Small but easy.

### Verification

Re-run the reproducer with `--heatmap` and confirm:

- `cacheBaseComponentConfig` count drops via better hit rate, or its
  per-call cost drops via lighter insert path.
- `Parallelism` metric in the heatmap header increases (especially
  on small-core runners).
- Wall-clock target on GHA (2-4 cores) after Phases 2+3:
  **< 2 minutes** end-to-end for `describe affected` on this
  workload. Local wall-clock is already < 5s on this Mac post-#2471.

## Tests

### Regression test

A unit test asserting that with `--identity=false` (or its
programmatic equivalent), `NewCredentialStoreWithConfig` is not
invoked. Mock the factory and assert call count == 0 for a fixture
run.

### Benchmark

A benchmark in `internal/exec/` that exercises a synthetic
high-component-count workload (e.g., 1,000 components inheriting
from a common base) and asserts wall-clock under a threshold.
Hooks into CI to catch perf regressions.

### Real-repo simulation

Keep the customer-repo reproducer alive as an offline
regression-detection workflow — once a quarter, run the same
reproducer and compare against the baseline numbers in this doc.

---

## Related

- [`docs/fixes/2026-05-21-ambient-identity-process-cache-panic.md`](2026-05-21-ambient-identity-process-cache-panic.md)
  — adjacent auth-path fix (process credential cache).
- [`docs/fixes/2026-04-17-ambient-identity-nil-credentials.md`](2026-04-17-ambient-identity-nil-credentials.md)
  — the predecessor ambient-credential fix.
- **PR #2471** (commit `a0cd28671`, in v1.220.0-rc.1) — Phase 1
  fix: `fix(auth): honor --identity=false in describe affected and
  dependents`. Eliminates the 3.5 min of credential-store work.
- #1576 — perf heatmap visualization (built the tool).
- #1611 — self-time vs total-time + recursion accounting.
- #1622 — Docker perf fix + CPU Time / Parallelism metrics.
- #1639 — first optimization pass: 5.2× faster execution, 92% memory
  reduction. Predates the `auth`, `profiles`, and `locals`
  subsystems that this doc identifies as the next optimization
  targets.
- `pkg/perf/perf.go` — `perf.Track` instrumentation library.
- `pkg/ui/heatmap/` — interactive TUI for hotspot analysis.
- `internal/exec/stack_processor_process_stacks_helpers.go:113` —
  `processComponent`, the top of the inheritance pipeline.
- `internal/exec/stack_processor_process_stacks_helpers_inheritance.go` —
  the inheritance chain (Phase 3 target).
- `internal/exec/stack_processor_cache.go:107` —
  `cacheBaseComponentConfig`, the hot cache path (Phase 2 target).
- `internal/exec/stack_processor_merge.go:40` —
  `mergeComponentConfigurations`, 1m34s of merge work per run.
- `pkg/auth/credentials/store.go:67` —
  `NewCredentialStoreWithConfig`, gated in PR #2471.
- `pkg/auth/credentials/keyring_system.go:187` —
  `systemKeyringStore.Delete`, gated in PR #2471.
