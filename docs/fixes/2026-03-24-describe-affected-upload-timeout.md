# describe affected --upload: Timeout on Large Infrastructures

**Date:** 2026-03-24
**Severity:** High — `--upload` flag hangs for 40+ minutes on large infrastructures, never completes
**Affected:** `atmos describe affected --upload` (also affects Atmos Pro GitHub Actions integration)

---

## Symptom

```bash
# These work fine:
atmos describe affected --process-functions=false                    # ~30s
atmos describe affected --process-functions=false --ref <sha>        # ~30s

# This never returns (40-50 min, then timeout or killed):
atmos describe affected --process-functions=false --ref <sha> --upload
```

The `--upload` flag causes the command to hang indefinitely on large infrastructures.
The same infrastructure works fine with smaller change sets (fewer affected components).

---

## Root Cause

### The N×M problem in `addDependentsToAffected`

When `--upload` is set, the code forces `IncludeDependents=true` and `IncludeSettings=true`
(lines 220-222 in `describe_affected.go`).

This triggers `addDependentsToAffected` (line 285-289), which loops over **every** affected
component and calls `ExecuteDescribeDependents` for each one. Inside that function
(line 138 of `describe_dependents.go`), a **full `ExecuteDescribeStacks`** is called —
resolving the entire infrastructure from scratch.

**There is no caching.** Each call independently:

1. Reads all stack YAML files
2. Processes all imports
3. Resolves all inheritance chains
4. Merges all configs (~118k merge calls)

### Concrete numbers (observed on customer infrastructure)

| Metric                                          | Value                |
|-------------------------------------------------|----------------------|
| Total stack files                               | ~1,000               |
| Total affected components (worst case)          | 2,422                |
| Time per `ExecuteDescribeStacks` call           | ~1s                  |
| Time for `addDependentsToAffected` (sequential) | ~2,422s ≈ **40 min** |
| Plus recursive `addDependentsToDependents`      | Additional N×M calls |

The `addDependentsToDependents` function is also recursive — each dependent itself calls
`ExecuteDescribeDependents` again, potentially creating O(N²) or worse total calls.

### Call chain

```text
describe affected --upload
  → IncludeDependents = true (forced by --upload)
  → addDependentsToAffected(affected)     // loops over 2,422 items
    → for each affected item:
      → ExecuteDescribeDependents()        // called 2,422 times
        → ExecuteDescribeStacks()          // FULL stack resolution each time (~1s)
        → ExecuteDescribeComponent()       // component lookup
        → iterate all stacks for depends_on matches
      → addDependentsToDependents(deps)    // RECURSIVE — each dependent does same
        → ExecuteDescribeDependents()      // called again for every dependent
          → ExecuteDescribeStacks()        // ANOTHER full resolution
```

### Additional payload concern

Even after the dependents computation completes, the resulting payload may be very large:

- Without dependents: ~1.1 MB JSON (2,422 affected items)
- With dependents + settings: potentially 10-100 MB
- HTTP client timeout: 30 seconds (`DefaultHTTPTimeoutSecs`)
- Serverless function payload limits: typically 6 MB (AWS Lambda) or 10 MB (Cloudflare Workers)

The `StripAffectedForUpload` function reduces payload by ~70-75%, but may not be enough
for 2,422+ items with recursive dependents.

---

## Affected Code Paths

| File                                                 | Function                    | Issue                                    |
|------------------------------------------------------|-----------------------------|------------------------------------------|
| `internal/exec/describe_affected.go:220-222`         | Upload flag handler         | Forces `IncludeDependents=true`          |
| `internal/exec/describe_affected_utils_2.go:532-594` | `addDependentsToAffected`   | No caching, sequential N calls           |
| `internal/exec/describe_affected_utils_2.go:596-643` | `addDependentsToDependents` | Recursive, no cycle detection            |
| `internal/exec/describe_dependents.go:138`           | `ExecuteDescribeDependents` | Calls `ExecuteDescribeStacks` every time |
| `pkg/pro/api_client.go:165-193`                      | `UploadAffectedStacks`      | 30s HTTP timeout, no compression         |

---

## Fix Options

### Option A: Cache `ExecuteDescribeStacks` result (recommended, quick win)

Call `ExecuteDescribeStacks` once before the loop and pass the result to
`ExecuteDescribeDependents`. The describe-stacks result is the same for every
affected component — there's no reason to recompute it 2,422 times.

**Impact:** Reduces time from O(N × stack_resolution) to O(1 × stack_resolution + N × lookup).
For the customer infra: ~40 min → ~30s + linear dependent scanning.

### Option B: Compute dependents in a single pass

Instead of calling `ExecuteDescribeDependents` per component, build a dependency graph once
from the stacks data and look up dependents from the graph. This would be O(stacks) instead
of O(affected × stacks).

### Option C: Upload without dependents

If Atmos Pro can compute dependents server-side (it has the stack data), the `--upload`
path could skip `addDependentsToAffected` entirely and let the server compute dependents.

### Option D: Payload size mitigations

- Add gzip compression to the upload request.
- Increase the HTTP timeout for upload operations.
- Implement chunked upload for payloads exceeding serverless limits.
- Stream the JSON serialization instead of building the entire string in memory.

---

## Fix Applied

Three optimizations were applied incrementally, achieving a **~250× speedup** (40+ min → ~10s).

### Optimization 1: Cache `ExecuteDescribeStacks` result

`ExecuteDescribeStacks` resolves the entire infrastructure (~1s for large infras). Previously
it was called inside `ExecuteDescribeDependents` for every affected component — 2,422 times.

**Fix:** Call `ExecuteDescribeStacks` once in `addDependentsToAffected` before the loop, then
pass the cached result via a new `Stacks` field on `DescribeDependentsArgs`.

**Result:** 40+ min → ~3.5 min (~12× speedup).

### Optimization 2: Cache `ExecuteDescribeComponent` lookup

`ExecuteDescribeComponent` was called per affected item to get the component's `vars` section.
Each call triggered its own stack resolution internally.

**Fix:** Added `findComponentSectionInCachedStacks` to extract component sections directly
from the cached stacks result.

**Result:** ~3.5 min → ~1 min 54s (~21× cumulative speedup).

### Optimization 3: Pre-built reverse dependency index

`ExecuteDescribeDependents` iterated over ALL stacks × ALL components for each affected item,
looking for `depends_on` matches. With 2,422 items and ~6,000 stack-component pairs, that's
~14.5M iterations.

**Fix:** Added `buildDependencyIndex` which pre-parses all components once and builds a
reverse index: component name → list of (stack, component) pairs that depend on it.
`ExecuteDescribeDependents` then does O(1) lookup instead of O(stacks × components) scan.

**Result:** ~1 min 54s → ~10s (~250× cumulative speedup).

### Incremental timing (2,422 affected components)

| Step                              | Time      | Speedup vs previous | Cumulative speedup |
|-----------------------------------|-----------|--------------------|--------------------|
| Before fix                        | 40+ min   | —                  | —                  |
| + Cache `ExecuteDescribeStacks`   | ~3.5 min  | ~12×               | ~12×               |
| + Cache component lookup          | ~1 min 54s| ~1.8×              | ~21×               |
| + Reverse dependency index        | **~10s**  | ~11×               | **~250×**          |

### Final timing results

| Command                                  | Before fix                | After fix         |
|------------------------------------------|---------------------------|-------------------|
| `describe affected` (no dependents)      | ~7s                       | ~7s (unchanged)   |
| `describe affected --include-dependents` | 40+ min (never completes) | **~10s**          |
| Payload size (with dependents)           | N/A (never completed)     | ~1.2 MB           |

### Files changed

| File                                              | Change                                                                                                         |
|---------------------------------------------------|----------------------------------------------------------------------------------------------------------------|
| `internal/exec/describe_dependents.go`            | Add `Stacks` and `DepIndex` fields to args; use cached stacks and index; extract `findDependentsFromIndex`, `findDependentsByScan`, `buildDependentEntry`, `findComponentSectionInCachedStacks` helpers |
| `internal/exec/describe_dependents_index.go`      | New: `dependencyIndex` type, `dependencyIndexEntry` struct, `buildDependencyIndex` function                    |
| `internal/exec/describe_dependents_index_test.go` | New: 8 tests for index building, component lookup, and dependent resolution                                    |
| `internal/exec/describe_affected_utils_2.go`      | Call `ExecuteDescribeStacks` and `buildDependencyIndex` once; pass to all dependent resolution calls            |

---

## Verification

After the fix:

1. `atmos describe affected --include-dependents` completes in ~10s (down from 40+ min).
2. All existing affected/dependent tests pass (30+ tests).
3. 8 new tests for the dependency index, component lookup, and dependent resolution.
4. No regression for smaller infrastructures — the `Stacks` and `DepIndex` fields are
   optional and only used when pre-computed data is available.

---

## Related

- `pkg/pro/api_client.go`: HTTP client with 30s timeout.
- `internal/exec/describe_affected_upload.go`: `StripAffectedForUpload` reduces payload.
- `internal/exec/describe_dependents.go`: Core dependent resolution logic.
