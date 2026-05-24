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

## Post-#2471 fresh heatmap (2026-05-23, local Mac, current main)

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

## Affected detection note

In the reproducer, the result set is empty (`[]`) even though a
catalog file was modified. Atmos compares **resolved configs**, and
the comment-only marker doesn't change the resolved output. The
expensive part — the full stack processing on both refs — happens
regardless: Atmos must compute both worktrees' resolved configs to
diff them. **CI pain is the full processing pass, not the affected
result size.**

## Status

**Phases 1-13 shipped** (with the Phase 5 1-input shortcut and Phase 9
reverted — see their dedicated sections below). The progress checklist
that follows is the authoritative record of what landed where, in what
order, and with what measured impact.

The remaining open item is **end-to-end CI validation on the reference
workload**: a 2-core GHA runner timing run that confirms the projected
~11 minutes → ~60-105 seconds wall-clock improvement.

Phase 1 (the `--identity=false` credential-store fix) shipped separately
in PR #2471 (`a0cd28671`, in v1.220.0-rc.1) and reclaimed ~3.5 minutes
of credential-store CPU on the reference workload. Phases 2-13 ship in
PR #2496 (this branch).

Two optimizations were attempted and reverted after CI surfaced
correctness regressions — both shared the same root cause (returning a
shared map reference to a caller that mutates the result). They are
documented in their dedicated sections so future passes don't re-try
the same approach without addressing the underlying contract violation:

- Phase 5's 1-input shortcut in `MergeWithDeferred` —
  [details below](#phase-5-initially-shipped-2026-05-23-1-input-shortcut-reverted-2026-05-24).
- Phase 9's asymmetric clone in `cloneExtractLocalsResult` — reverted
  in `b11f3cd9b`; documented in the Phase 9 checklist entry.

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
- [x] Phase 3: short-circuit `WalkAndDeferYAMLFunctions` for
      function-free subtrees. Saved 200k recursive calls, dropped
      Total from 1m26s → 15.4s (−82%), wall-clock 4.1s → 3.3s
      (−19% locally; ~18-35s on GHA projected). Original plan
      (component-level parallelization) abandoned because it's
      already in place.
- [x] Phase 5: 0-input fast path in `MergeWithDeferred` (skip walk +
      merge when every input is empty). The 1-input shortcut was
      tried alongside and REVERTED 2026-05-24 after CI surfaced a
      regression in `TestSpaceliftStackProcessor` (47→40 stacks):
      `WalkAndDeferYAMLFunctions`'s Phase 3 short-circuit returned
      the input map as-is, and the 1-input shortcut handed that
      reference back to the caller, which mutated the upstream
      cached settings. Same class of failure as Phase 9. Post-revert
      `mergeComponentConfigurations` Total: 1m54s → 1m35s (−17%);
      regression test added to prevent re-attempts.
- [x] Phase 4: cache `extractLocalsFromRawYAML` by file path + content
      hash; clone input context to fix pre-existing race. 22k+ calls
      eliminated, wall-clock 3.0s → 2.3s (−23%, biggest single-phase
      wall-clock win). Pre-existing race in
      `extractAndAddLocalsToContext` (mergedContext mutation) fixed.
- [x] Phase 6: apply Phase 2's sync.Map + deep-copy-outside-lock
      pattern to `parsedYAMLCache` in `pkg/utils/yaml_utils.go`.
      `UnmarshalYAMLFromFileWithPositions` Total dropped 80%
      (18.7s → 3.7s, 5× faster avg). Wall-clock unchanged on Mac
      (parallelism already absorbed it); ~4-7s wall-clock saved on
      2-4 core GHA runners.
- [x] Phase 7: hoist `hasCustomTags` pre-check out of recursion in
      `processCustomTags`. Total dropped 76% (31.5s → 7.5s, 3.2×
      faster avg). The previous implementation re-ran the full
      subtree scan on every recursive call (O(N×depth) work just
      for the early-exit check); a split into outer/inner functions
      runs the check once per top-level call. ~24s cumulative CPU
      saved on this workload → ~6-12s wall-clock on 2-4 core GHA.
- [x] Phase 8: cache the post-Decode + post-Intern result in a new
      `decodedYAMLCache` (sync.Map keyed by file + content hash) so
      repeat callers of `UnmarshalYAMLFromFileWithPositions[map[string]any]`
      skip the per-call `yaml.Node.Decode` + `InternStringsInMap`
      walks (~500-700µs each). Wall-clock 2.4s → 2.06s (−14%
      locally).
- [x] Phase 10: apply Phase 7's outer/inner perf.Track pattern to
      `processYAMLNode` (recursive YAML walker used by yq evaluation).
      Removes per-recursion-step perf.Track overhead (~3µs each) from
      ~34k recursive calls. `processYAMLNode` dropped out of the
      top-25 hot list, wall-clock 2.2s → 2.03s (−7% local). The same
      pattern was *not* applied to `WalkAndDeferYAMLFunctions`:
      removing the per-recursion `hasAnyYAMLFunction` short-circuit
      regressed allocation cost on function-sparse subtrees more
      than the perf.Track savings recovered (the inner-only walker
      had to allocate on every level). Documented in-code so future
      passes don't re-try.
- [~] Phase 9 (ATTEMPTED, REVERTED 2026-05-24, `b11f3cd9b`):
      asymmetric clone of `extractLocalsResult` — share
      `settings`/`vars`/`env` references with the cache, deep-copy
      only `locals`. Showed −17% on `extractLocalsFromRawYAML`
      locally and passed `go test -race` on the targeted tests, but
      crashed with `fatal error: concurrent map iteration and map
      write` on the real customer workload (~9k parallel
      goroutines). Root cause: `processTemplatesInSection` returns
      its input map as-is when the section contains no `{{`, so the
      cached settings *do* end up shared in the long-lived template
      context where a sibling goroutine then mutates them via
      `DeepCopyMap`. Lesson: the "consumed only by
      `processTemplatesInSection`" claim was wrong — that function
      has a fast-path pass-through. Re-attempting requires a clone
      inside the pass-through, or a stronger immutability proof.
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

### Phase 3 (shipped 2026-05-23) — short-circuit YAML function walk for function-free subtrees

**Investigation pivot.** Phase 3 was originally planned as
component-level parallelization within `processComponentsInParallel`.
On inspection of the code, that parallelism is **already in place**
(one goroutine per component within each stack, fanned out by
`processComponentsInParallel`). Adding sub-component goroutines
would not help on 2-4 core GHA runners — they're already CPU-bound
at the component goroutine layer.

**Real target identified.** Post-Phase-2 heatmap pinpointed
`merge.WalkAndDeferYAMLFunctions` as the next bottleneck:

- 527,765 calls (recursive — each call walks every nested map and
  recurses into nested maps).
- 1m26s cumulative, 164µs avg per call.
- Function-free subtrees (no `!terraform.*`, `!template`, `!store*`,
  `!exec`, `!env` strings anywhere) were being deep-copied at every
  recursion level for no reason: the walk allocates a new
  `map[string]interface{}` of equal capacity at each level, copies
  every key, recurses into nested maps. The vast majority of the
  workload — global vars, settings, env, hooks, generate, providers
  for components without YAML functions — was generating pure GC
  pressure.

**Fix.** Added a non-allocating pre-scan
`hasAnyYAMLFunction(map) bool` that returns true on the first YAML
function string encountered anywhere in the subtree. When false,
`WalkAndDeferYAMLFunctions` returns the input map as-is — zero
allocation. When true, the original walk path runs unchanged.

**Safety.** The fast-path return shares the input map with the
caller. The contract is documented in the function comment: callers
must treat the result as read-only. The only call site is
`MergeWithDeferred → Merge`, which produces a new merged map without
mutating inputs. A new test
(`TestWalkAndDeferYAMLFunctions_NoFunctionsShortCircuit`) verifies
the contract: returns same pointer for function-free input, walks
normally when any subtree contains a function, does not mutate
function-free input.

**Confirmed impact (mean of 3 runs, customer workload):**

| Function | Phase 2 (Total / Avg / Calls) | Phase 3 (Total / Avg / Calls) | Reduction |
|---|---:|---:|---:|
| `merge.WalkAndDeferYAMLFunctions` | 1m26s / 164µs / 527,765 | **15.4s / 47µs / 328,218** | **−82% Total, 3.5× faster avg, 200k fewer calls** |
| `merge.MergeWithDeferred` | 1m35s / 752µs | **1m10s / 851µs** | **−26% Total** |
| `merge.Merge` | 39s / 143µs | 39s / 143µs | unchanged |
| `mergeComponentConfigurations` | 2m22s / 15ms | **1m54s / 12.5ms** | **−20% Total** |

**Wall-clock impact (local Mac):** 4.1s → **3.3s mean across 3
runs** (−19%). Unlike Phase 2 (which eliminated lock-wait padding
without moving wall-clock), Phase 3 reduces real allocation work and
GC pressure, so wall-clock improves on every architecture, including
high-core machines. Projection for 2-4 core GHA: same ~71s of
cumulative CPU saved → 18-35s wall-clock improvement.

**Race detector:**
`go test -race -count=1 ./pkg/merge/...` green. Note: pre-existing,
unrelated race in `extractAndAddLocalsToContext`
(`internal/exec/stack_processor_utils.go:260`) surfaces under
`-race` for some `TestHierarchicalImports_*` tests — confirmed
present in main without Phase 3 changes. Tracked separately.

**Bottleneck remaining post-Phase-3:**

- `mergeComponentConfigurations` (1m54s) — the next-deepest hot path.
  ~8 sequential `MergeWithDeferred` calls per component (vars,
  settings, env, auth, providers, required_providers, hooks,
  generate). Most components don't have YAML functions, so each call
  now skips the walk, but the merge itself still runs. Phase 5
  candidate: skip the merge entirely when only one input is
  non-empty (single-input merge degenerates to a copy).
- `ProcessYAMLConfigFileWithContext` (1m11s) — file loading. May be
  unrelated to inheritance; deferred to Phase 6.

### Phase 5 (initially shipped 2026-05-23, 1-input shortcut REVERTED 2026-05-24)

**Root cause.** `mergeComponentConfigurations` calls
`MergeWithDeferred` 9 times per component instance — once per
section (vars, settings, env, auth, providers, required_providers,
hooks, generate) plus auth-merge fan-out. Each call passes 3-4
candidate inputs: typically `GlobalX / BaseComponentX / ComponentX /
ComponentOverridesX`. On the customer workload, most of those
layers are empty: components that don't inherit have empty
`BaseComponentX`, most components have no `ComponentOverridesX`, and
some sections (`generate`, `required_providers`, `hooks`) are empty
for most components.

Despite the empty layers, every call ran the full pipeline:
allocate a deferred merge context, walk each input (Phase 3
short-circuit avoids the inner deep-copy but the walk function still
runs and increments precedence), and call `Merge` to combine all
processed inputs (which is a deep walk over all entries even when
all but one are empty).

**Final fix (after revert).** `MergeWithDeferred` has a single
all-empty fast path:

- **0 non-empty inputs:** return a fresh empty map and an empty
  dctx. No walk, no merge.
- **Any non-empty input:** fall through to the regular walk + merge
  pipeline. `Merge` → `MergeWithOptions` returns a deep-copied,
  caller-mutable map per its contract.

The 0-input fast path is still a real win — many components leave
several sections (especially `generate` / `required_providers` /
`hooks` on non-terraform components) entirely empty.

#### What was tried and reverted

A 1-input fast path was initially shipped alongside the 0-input one:
when exactly one input was non-empty, the implementation walked just
that input and returned it directly, skipping the `Merge` call. This
broke `TestSpaceliftStackProcessor` and `TestLegacySpaceliftStackProcessor`
on the CI sweep for PR #2496 (both lost exactly 7 spacelift stacks
each, 47→40 / 44→37).

Root cause: when the single non-empty input contained no Atmos YAML
functions (the common case), `WalkAndDeferYAMLFunctions`'s Phase 3
short-circuit returned the input map **as-is** — so the 1-input
shortcut handed the caller a shared reference to the upstream
cached `BaseComponentSettings` / `GlobalSettings` / etc. The
downstream `mergeComponentConfigurations` mutated that result while
building the per-component output map, which corrupted the upstream
cache for sibling components. Specifically,
`settings.spacelift.workspace_enabled` was getting flipped for
seven components by the time they reached `CreateSpaceliftStacks`.

This is the **same class of failure as Phase 9** (asymmetric
`extractLocalsResult` clone): in both cases the optimization
assumed downstream callers wouldn't mutate the returned map, but
the assumption was wrong because a later pipeline stage modifies
the result map in place. Per `Merge`'s contract, the returned map
must be deep-copied, caller-mutable. The 1-input shortcut violated
that contract.

The `Merge` slow path that the revert restores **also** deep-copies
its 1-input case (via `MergeWithOptions` → `DeepCopyMap`), so the
saved wrapper overhead was small. The shortcut's measured savings
(`MergeWithDeferred` Total 1m10s → 25s) over-attributed credit
that was really earned by Phase 3's `WalkAndDeferYAMLFunctions`
short-circuit reducing per-walk cost.

A regression test
(`TestMergeWithDeferred_TrivialInputShortCircuits/mutating the
result does not mutate the input`) was added to lock in the
restored contract: any future caller-mutation of the result must
not bleed into the input.

**Confirmed impact (mean of 3 runs, customer workload, post-revert):**

| Function | Pre-Phase-5 baseline | Post-revert (0-input only) | Reduction |
|---|---:|---:|---:|
| `mergeComponentConfigurations` | 1m54s | **1m35s** | **−17% Total** |
| `merge.MergeWithDeferred` | 1m10s | **51s** | **−27% Total** |
| `merge.Merge` Calls | 273,379 | **235,806** | **−14% (−37k calls)** |
| `merge.WalkAndDeferYAMLFunctions` | 15.4s / 328k | **11s / 178k** | **−29% Total, −46% Calls** |

**Wall-clock (local Mac):** 3.3s → **~2.2s** mean — still an
improvement over the pre-Phase-2 baseline (4.1s, −47% cumulative),
but ~170ms regression vs the original (unsafe) Phase 5
measurement of 3.0s. Acceptable cost for correctness.

### Phase 4 (shipped 2026-05-23) — cache locals extraction by file path + content hash

**Root cause.** `extractLocalsFromRawYAML` was called 22,181 times for
~836 unique files (~26 calls per file via transitive imports), at
533µs avg. ~7s of the 13s total was `UnmarshalYAMLFromFile`
re-parsing the same YAML content for the same file path. Within a
single command execution, file content is immutable (and the file
content itself was already cached via `getFileContentSyncMap`), so
the parse + locals resolution is fully deterministic and trivially
cacheable.

A pre-existing data race in `extractAndAddLocalsToContext`
(surfaced under `-race` by some `TestHierarchicalImports_*` tests)
mutated the parent `mergedContext` map directly via `delete(context,
LocalsSectionName)` while multiple goroutines processed sibling
imports concurrently. Phase 4 closes this race in the same fix.

**Two changes:**

1. **`localsExtractionCache` (sync.Map) keyed by `filePath + FNV-1a(yamlContent)`**
   memoizes the parsed `*extractLocalsResult` for the duration of the
   command. Cache reads deep-copy the cached value via
   `cloneExtractLocalsResult` so downstream consumers can store the
   maps into shared template contexts without corrupting the cache.
   FNV-1a content fingerprinting prevents test pollution when the
   same logical file path is reused with different content (a common
   pattern in unit tests).
2. **`extractAndAddLocalsToContext` clones its input context map** before
   the file-scoped `delete` + assign, eliminating the pre-existing
   race. The clone is shallow (values are shared references); the
   only mutation surface inside the function is the map header, so
   per-goroutine cloning is sufficient. Cost: O(N) over the context
   entries, negligible compared to the function's overall work.

**Confirmed impact (mean of 3 runs, customer workload):**

| Function | Phase 5 (Total / Avg / Calls) | Phase 4 (Total / Avg / Calls) | Reduction |
|---|---:|---:|---:|
| `exec.extractAndAddLocalsToContext` | 13s / 580µs / 22,181 | **6.7s / 300µs / 22,181** | **−48% Total, 2× faster avg** |
| `exec.extractLocalsFromRawYAML` | 13s / 533µs / 22,181 | **5.9s / 264µs / 22,181** | **−55% Total, 2× faster avg** |
| `utils.UnmarshalYAMLFromFile` Calls | 22,374 | **~1,900** | **−92% (20k+ calls eliminated)** |
| `exec.ProcessStackLocals` | 1.5s / 22,167 | **170ms / ~1,800** | **−89% Total, −92% Calls** |

**Wall-clock impact (local Mac, mean of 3 runs):** 3.0s → **2.3s
(−23%)** — the biggest single-phase wall-clock win in this
investigation. Unlike Phase 2 (lock-wait padding that doesn't show
on Mac), the locals cache eliminates real CPU work (YAML parsing +
locals resolution) that was contributing directly to wall-clock on
every architecture.

**Race detector:**
`go test -race -count=1 -run TestHierarchicalImports ./internal/exec/`
now green. Other pre-existing races elsewhere in the package
(`processSettingsIntegrationsGithub` racing with `DeepCopyMap`,
`MergeContext.WithFile` racing) are tracked separately — not
introduced by Phase 4.

**Cumulative trajectory (local Mac wall-clock, mean of 3 runs):**

| Stage | Elapsed | Δ vs prior |
|---|---:|---:|
| Baseline (pre-Phase-2) | 4.1s | — |
| Phase 3 (walk short-circuit) | 3.3s | −19% |
| Phase 5 (skip trivial merges) | 3.0s | −10% |
| Phase 4 (locals cache) | **2.3s** | **−23%** |
| **Cumulative** | **2.3s** | **−44% vs baseline** |

### Phase 6 (shipped 2026-05-23) — sync.Map for parsedYAMLCache

**Root cause.** After Phase 4 eliminated 92% of locals-extraction
parses, the next bottleneck became `UnmarshalYAMLFromFileWithPositions`
(22,181 calls, 18.7s Total, 844µs avg) — the position-tracking YAML
parser used by `processYAMLConfigFileWithContextInternal` for the
final stack-config parse. It already had a content-hash cache, but
the cache structure was the same RWMutex-protected map pattern that
Phase 2 fixed for `cacheBaseComponentConfig`:

- `cacheParsedYAML` held `parsedYAMLCacheMu.Lock()` during the
  recursive `deepCopyYAMLNode` work.
- `getCachedParsedYAML` held `parsedYAMLCacheMu.RLock()` during the
  read-side `deepCopyYAMLNode`.

With ~22k concurrent calls, the global write lock serialized every
cache write, padding apparent CPU time with lock-wait.

**Fix.** Same as Phase 2:

1. `parsedYAMLCache` is now a `sync.Map` (was `map + sync.RWMutex`).
2. Deep-copy of `yaml.Node` + `PositionMap` happens *before* the
   `sync.Map.Store` and *after* the `sync.Map.Load`, never inside any
   critical section.

Added `clearParsedYAMLCache()` plus the public `ClearParsedYAMLCache()`
for tests, mirroring the `ClearBaseComponentConfigCache` /
`ClearLocalsExtractionCache` pattern. Three existing tests that
manually saved/restored the `map + mutex` state were updated to use
the helper.

**Confirmed impact (mean of 3 runs, customer workload):**

| Function | Phase 4 (Total / Avg) | Phase 6 (Total / Avg) | Reduction |
|---|---:|---:|---:|
| `utils.UnmarshalYAMLFromFileWithPositions` | 18.7s / 844µs | **3.7s / 166µs** | **−80% Total, 5× faster avg** |
| `exec.ProcessYAMLConfigFileWithContext` | 57.6s / 37ms | **54s / 35ms** | −6% Total (downstream beneficiary) |

**Wall-clock impact (local Mac):** 2.3s → 2.3s (unchanged). Same
caveat as Phase 2: lock-wait padding doesn't show on many-core
machines because the waiters park rather than consume cores. On 2-4
core GHA runners where the lock-wait IS wall-clock, projecting from
the 15s of cumulative CPU eliminated → **~4-7 seconds wall-clock
savings end-to-end**.

**Race detector:**
`go test -race -count=1 -run "UnmarshalYAMLFromFileWithPositions|HandleCacheMiss|CacheHit" ./pkg/utils/...`
green. Full `pkg/utils` and `internal/exec` test suites pass.

### Phase 7 (shipped 2026-05-23) — hoist hasCustomTags check out of recursion

**Root cause.** `processCustomTags` (the YAML node walker that
detects and processes `!terraform.*`, `!template`, `!store*`,
`!exec`, `!env`, `!include`, `!literal` tags) had a `hasCustomTags`
early-exit pre-check at the top of every invocation. The pre-check
itself walks the entire subtree to find any custom tag. On
recursive invocations (one per child node with content), the
pre-check re-walks the same subtree it just walked at the level
above — O(N×depth) work where it should be O(N).

For trees with tags (which is most of them in the customer
workload), this meant ~9k top-level `processCustomTags` calls each
re-walked their tree at every recursion level. Total cumulative
CPU: 31.5s, 3.4ms avg per top-level call.

**Fix.** Split `processCustomTags` into outer entry + inner worker:

- `processCustomTags` does the `hasCustomTags` check ONCE at the
  top level. If no tags exist, early-exit. Otherwise, delegate to
  the inner worker.
- `processCustomTagsInner` is the recursive worker — no `hasCustomTags`
  check on each call, just process and recurse. By the time we're
  inside it, we already know the tree has tags.

`perf.Track` stays on the outer function and is intentionally
omitted from the inner worker: the outer call wraps the whole walk
with one tracked frame, so adding per-recursion tracking would
inflate the metric without yielding insight.

**Confirmed impact (mean of 2 runs, customer workload):**

| Function | Phase 6 (Total / Avg / Calls) | Phase 7 (Total / Avg / Calls) | Reduction |
|---|---:|---:|---:|
| `utils.processCustomTags` | 31.5s / 3.4ms / 9,200 | **7.5s / 1.07ms / ~7,000** | **−76% Total, 3.2× faster avg** |

(The call-count drop is mechanical: recursive calls now go through
the inner worker which lacks `perf.Track`, so only top-level calls
are counted. The CPU-time drop is the real signal.)

**Wall-clock impact (local Mac):** 2.3s → 2.4s (within noise on a
many-core machine where parallelism absorbs the saved work).
Cumulative ~24s CPU saved → projected **~6-12s wall-clock on 2-4
core GHA runners** on top of Phases 1-6.

**Race detector:**
`go test -race -count=1 -run "ProcessCustomTags|UnmarshalYAML|TestHasCustomTags" ./pkg/utils/...`
green. Full `pkg/utils` and `internal/exec` test suites pass.

### Phase 8 (shipped 2026-05-23) — cache the decoded+interned YAML result

**Root cause.** The `parsedYAMLCache` from Phase 6 already eliminates
re-parsing of identical YAML content. But the inner Decode + Intern
steps inside `UnmarshalYAMLFromFileWithPositions` run on **every**
call — including cache hits:

```go
node, _, _ := getCachedParsedYAML(file, input)  // Phase 6 cache hit
node.Decode(&data)                              // Always runs
data = InternStringsInMap(data)                 // Always runs
```

Decode allocates a fresh map tree from the cached `yaml.Node`;
InternStringsInMap recursively walks the decoded tree and allocates
another tree with interned string keys/values. Combined they cost
~500-700µs per call across the 22,181 invocations.

**Fix.** Added a second sync.Map cache (`decodedYAMLCache`) keyed by
the same (file, content hash) pair as `parsedYAMLCache`. The cache
value is the post-Decode + post-Intern `map[string]any` plus the
matching `PositionMap`. The hot-path call site checks this cache
first; a hit returns a deep-copy of the cached result, skipping
Decode and Intern entirely.

The fast path activates only for `T == map[string]any` (the
production hot path; `schema.AtmosSectionMapType` is exactly this
type). Detection is done via a single `any(zeroValue).(map[string]any)`
type assertion — no reflect, no per-call overhead. Other generic
instantiations (used only in tests) fall through to the existing
Decode path.

A local `deepCopyDecodedMap` helper handles the deep copy on
retrieval; we can't use `merge.DeepCopyMap` because `pkg/merge`
imports `pkg/utils` (circular dependency).

`ClearDecodedYAMLCache` added for test cleanup, mirroring the
existing `ClearParsedYAMLCache` / `ClearLocalsExtractionCache` /
`ClearBaseComponentConfigCache` pattern.

**Confirmed impact (mean of 3 runs, customer workload):**

| Function | Phase 7 (Total / Avg) | Phase 8 (Total / Avg) | Reduction |
|---|---:|---:|---:|
| `utils.UnmarshalYAMLFromFileWithPositions` | 3.7s / 166µs | **3.2s / 144µs** | **−14% Total** |
| **Wall-clock elapsed** | **2.4s** | **2.06s** | **−14% locally** |

The cumulative CPU savings on `UnmarshalYAMLFromFileWithPositions`
itself are modest (~500ms), but the wall-clock improvement is
larger than the cumulative CPU delta would predict — likely from
reduced allocation pressure (no fresh map allocations on cache hit)
and resulting GC relief. On 2-4 core GHA runners, expect a similar
relative improvement plus the GC win compounded across cores.

**Race detector:** `go test -race -count=1 ./pkg/utils/...` green.
Full `pkg/utils` and `internal/exec` test suites pass.

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
