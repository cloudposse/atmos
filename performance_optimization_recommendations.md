# Performance Optimization Recommendations

**Status**: Deferred to separate PR (after heatmap improvements merge)
**Analysis Date**: 2025-10-10
**Customer Data**: 1m49s elapsed, 83.5M function calls, subjective experience ~12+ minutes

---

## Executive Summary

Customer reported 12-minute execution times for `atmos describe stacks`. Performance analysis revealed catastrophic call volume:
- **75.2M calls** to `SliceContainsString` (90% of all calls)
- **6.6M calls** to `processCustomTags` (recursive YAML tree walking)
- **480K calls** to `MergeWithOptions` (YAML defensive copying)

**Estimated improvement with Priority 1+2 optimizations**: 60-70 seconds (~65% faster)

---

## Root Causes

### 1. 🔥 SliceContainsString Hotspot (75.2M calls)

**Location**: `pkg/utils/yaml_utils.go:246`
```go
if SliceContainsString(AtmosYamlTags, tag) {
```

**Problem**: O(n) linear search through 7-element static slice, called 75M times

**Impact**:
- Call count: 75,251,964
- Avg: 18µs per call
- Total: 22m50s

**Root Cause**:
- `processCustomTags` (6.6M calls) checks every YAML node tag
- Each check iterates through `AtmosYamlTags` slice (7 elements)
- Deep YAML nesting × recursive processing = exponential call volume

---

### 2. 🔥 YAML Defensive Copying (480K calls)

**Location**: `pkg/merge/merge.go:45-53`
```go
yamlCurrent, err := u.ConvertToYAML(current)
dataCurrent, err := u.UnmarshalYAML[any](yamlCurrent)
```

**Problem**: Full YAML marshal→unmarshal round-trip for defensive copying

**Impact**:
- `MergeWithOptions`: 480,944 calls, 11h40m total
- `ConvertToYAML`: 219,701 calls, 1h31m total
- Each unmarshal triggers `processCustomTags` (6.6M calls)

**Root Cause**:
- Working around mergo pointer mutation bug
- YAML round-trip is extremely expensive
- Triggers full custom tag processing on already-processed data

---

### 3. 🔥 processCustomTags Recursive Explosion (6.6M calls)

**Location**: `pkg/utils/yaml_utils.go:235-277`

**Problem**: Recursively walks entire YAML tree for every parsed file

**Impact**:
- Call count: 6,662,622
- Avg: 30µs per call
- Total: 57h14m

**Root Cause**:
- Called for every YAML file parse/unmarshal
- Recursively processes DocumentNode → all children
- No early exit when subtree has no custom tags

---

### 4. 🔥 ProcessBaseComponentConfig Redundancy (27K calls)

**Location**: `internal/exec/stack_processor_utils.go:1152-1460`

**Problem**: No caching of inheritance chain resolution

**Impact**:
- Call count: 27,677
- Avg: 2.49s per call
- Total: 19h7m

**Root Cause**:
- Deep component inheritance chains processed repeatedly
- Same base component configurations calculated multiple times
- No memoization of inheritance results

---

## Optimization Recommendations

### 🔥 Priority 1: Critical (Immediate Impact)

#### 1.1 Replace SliceContainsString with Map Lookup

**Expected Impact**: 10-15 second improvement
**Effort**: 5 minutes
**Risk**: None

**Implementation**:
```go
// pkg/utils/yaml_utils.go

var (
    AtmosYamlTags = []string{
        AtmosYamlFuncExec,
        AtmosYamlFuncStore,
        AtmosYamlFuncStoreGet,
        AtmosYamlFuncTemplate,
        AtmosYamlFuncTerraformOutput,
        AtmosYamlFuncTerraformState,
        AtmosYamlFuncEnv,
    }

    // Pre-computed map for O(1) lookups
    atmosYamlTagsMap = makeAtmosYamlTagsMap()
)

func makeAtmosYamlTagsMap() map[string]bool {
    m := make(map[string]bool, len(AtmosYamlTags))
    for _, tag := range AtmosYamlTags {
        m[tag] = true
    }
    return m
}

// In processCustomTags, line 246:
// BEFORE: if SliceContainsString(AtmosYamlTags, tag) {
// AFTER:  if atmosYamlTagsMap[tag] {
```

---

#### 1.2 Replace YAML Defensive Copying

**Expected Impact**: 30-45 second improvement
**Effort**: 30 minutes
**Risk**: Low (test thoroughly)

**Solution A - Use JSON for defensive copying** (recommended):
```go
// pkg/merge/merge.go

import "encoding/json"

func deepCopyMapViaJSON(m map[string]any) (map[string]any, error) {
    // JSON round-trip is 5-10x faster than YAML
    // Doesn't trigger processCustomTags
    jsonBytes, err := json.Marshal(m)
    if err != nil {
        return nil, err
    }

    var result map[string]any
    err = json.Unmarshal(jsonBytes, &result)
    return result, err
}

// In MergeWithOptions, replace lines 45-53:
dataCurrent, err := deepCopyMapViaJSON(current)
if err != nil {
    return nil, fmt.Errorf("%w: failed to deep copy: %v", errUtils.ErrMerge, err)
}
```

**Solution B - Use dedicated deep copy library** (faster):
```go
import "github.com/mohae/deepcopy"

// In MergeWithOptions, replace lines 45-53:
dataCurrent := deepcopy.Copy(current).(map[string]any)
```

**Why this works**:
- Data is already in Go map format (not raw YAML)
- Custom tags already processed
- Only need structural copy, not YAML parsing
- JSON round-trip skips `processCustomTags` entirely

---

### 🔥 Priority 2: High (Significant Impact)

#### 2.1 Add Inheritance Caching

**Expected Impact**: 15-20 second improvement
**Effort**: 1-2 hours
**Risk**: Medium (cache invalidation)

**Implementation**:
```go
// internal/exec/stack_processor_utils.go

var (
    baseComponentConfigCache   = make(map[string]*schema.BaseComponentConfig)
    baseComponentConfigCacheMu sync.RWMutex
)

func getCachedBaseComponentConfig(cacheKey string) (*schema.BaseComponentConfig, bool) {
    baseComponentConfigCacheMu.RLock()
    defer baseComponentConfigCacheMu.RUnlock()

    cached, found := baseComponentConfigCache[cacheKey]
    if found {
        // Return a deep copy to prevent mutations
        copy := *cached
        return &copy, true
    }
    return nil, false
}

func cacheBaseComponentConfig(cacheKey string, config *schema.BaseComponentConfig) {
    baseComponentConfigCacheMu.Lock()
    defer baseComponentConfigCacheMu.Unlock()

    // Store a copy to prevent external mutations
    copy := *config
    baseComponentConfigCache[cacheKey] = &copy
}

func ProcessBaseComponentConfig(...) error {
    defer perf.Track(atmosConfig, "exec.ProcessBaseComponentConfig")()

    // Create cache key from component+stack+baseComponent
    cacheKey := fmt.Sprintf("%s:%s:%s", stack, component, baseComponent)

    // Check cache first
    if cached, found := getCachedBaseComponentConfig(cacheKey); found {
        *baseComponentConfig = *cached
        *baseComponents = cached.ComponentInheritanceChain
        return nil
    }

    // Process as normal
    err := processBaseComponentConfigInternal(...)
    if err != nil {
        return err
    }

    // Cache the result
    cacheBaseComponentConfig(cacheKey, baseComponentConfig)
    return nil
}
```

**Cache Invalidation**: None needed - configuration is immutable per command execution

---

#### 2.2 Optimize processCustomTags

**Expected Impact**: 5-10 second improvement
**Effort**: 1 hour
**Risk**: Low

**Implementation**:
```go
// pkg/utils/yaml_utils.go

func processCustomTags(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, file string) error {
    defer perf.Track(atmosConfig, "utils.processCustomTags")()

    if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
        return processCustomTags(atmosConfig, node.Content[0], file)
    }

    // NEW: Early exit if no custom tags exist in this subtree
    if !hasCustomTags(node) {
        return nil
    }

    for _, n := range node.Content {
        // ... existing logic
    }
    return nil
}

// Helper to check if any custom tags exist (fast scan)
func hasCustomTags(node *yaml.Node) bool {
    if atmosYamlTagsMap[strings.TrimSpace(node.Tag)] {
        return true
    }

    for _, child := range node.Content {
        if hasCustomTags(child) {
            return true
        }
    }

    return false
}
```

**Why this works**: Most YAML subtrees don't have custom tags, so early exit saves recursive processing

---

### ⚡ Priority 3: Optional (Nice to Have)

#### 3.1 Parallelize Import Processing

**Expected Impact**: 5-10 second improvement
**Effort**: 2-3 hours
**Risk**: Medium (complexity, race conditions)

**Current**: Imports processed sequentially in loop (lines 609-695 of stack_processor_utils.go)

**Proposed**: Process imports in parallel with worker pool

**Complexity**: Need to handle:
- Concurrent writes to `importsConfig` map (requires mutex)
- Merge context propagation
- Error aggregation

**Recommendation**: Defer until after Priority 1+2 optimizations are proven

---

## Expected Total Improvement

| Optimization | Expected Improvement | Cumulative |
|--------------|---------------------|------------|
| **P1.1: Map lookup** | 10-15s | 10-15s |
| **P1.2: JSON copying** | 30-45s | 40-60s |
| **P2.1: Inheritance cache** | 15-20s | 55-80s |
| **P2.2: Early exit** | 5-10s | 60-90s |
| **P3.1: Parallel imports** | 5-10s | 65-100s |

**Realistic estimate with P1+P2**: 70-75 seconds improvement (~65% faster)

**Customer impact**: 12 minutes → 4-5 minutes

---

## Testing Strategy

1. **Benchmark existing performance** with customer's stack configuration
2. **Apply P1.1** (map lookup) - validate with unit tests
3. **Apply P1.2** (JSON copying) - validate merge behavior with integration tests
4. **Measure improvement** - should see 40-60 second reduction
5. **Apply P2.1** (caching) - validate inheritance chains are correct
6. **Apply P2.2** (early exit) - validate custom tags still processed
7. **Final measurement** - should see 60-90 second total reduction

---

## Implementation Order

1. **Week 1**: Priority 1 optimizations (P1.1 + P1.2)
   - Low risk, high impact
   - Can be done independently
   - Immediate customer benefit

2. **Week 2**: Priority 2 optimizations (P2.1 + P2.2)
   - Medium risk, significant impact
   - Requires more testing
   - Further improves customer experience

3. **Future**: Priority 3 (P3.1)
   - Only if P1+P2 insufficient
   - Higher complexity, lower return

---

---

## Implementation Status

### ✅ Completed Optimizations (2025-10-13)

#### P1.1: Map Lookup (Completed)
- **Commit**: 71ceb637
- **Impact**: Replaced O(n) SliceContainsString with O(1) map lookup
- **Result**: 75M+ calls now use constant-time lookup

#### P1.2: copystructure Deep Copy (Completed)
- **Commit**: f81c44ed
- **Impact**: Replaced YAML round-trip with mitchellh/copystructure
- **Result**: Avoids unnecessary processCustomTags calls, preserves numeric types

#### P2.2: Early Exit in processCustomTags (Completed)
- **Commit**: 8a304a39
- **Impact**: Added hasCustomTags() pre-scan to skip subtrees without custom tags
- **Result**: Reduced calls from 519K → 62K (88% reduction), saved 69s CPU time

#### P2.1: Inheritance Caching (Completed)
- **Commit**: 8185c2d5
- **Impact**: Added cache for ProcessBaseComponentConfig inheritance chains
- **Result**: 66% hit rate (5,102 calls, 1,756 misses), ~1.5s savings

#### P3.1: Fast-Path Merge Checks (Completed)
- **Commit**: 854ce157
- **Impact**: Skip merge operations for empty/single-input trivial cases
- **Result**: MergeWithOptions calls reduced from 327K → 157K (52% reduction)

#### P3.2: Custom deepCopyMap (Completed)
- **Commit**: 4c66e395
- **Impact**: Replaced reflection-based copystructure with optimized implementation for map[string]any
- **Result**: deepCopyMap improved from 38µs → 17µs per call (2.2x faster), total runtime 14.5s → 7.4s (49% faster)

#### P3.3: YAML Node Caching (Completed)
- **Commit**: 185575c7
- **Impact**: Cache parsed yaml.Node + positions after custom tag processing
- **Result**: 96% cache hit rate (876 misses, 22,287 hits), processCustomTags calls reduced 75% (62,602 → 15,863), runtime 7.4s → 6.8s (57% total improvement from baseline)

#### P3.4: FindStacksMap Caching (Completed)
- **Commit**: c8f90e71
- **Impact**: Cache FindStacksMap results to eliminate duplicate ProcessYAMLConfigFiles calls
- **Result**: ProcessYAMLConfigFiles calls reduced from 2 → 1 (50% reduction), ~317ms savings per describe stacks command
- **Context**: ValidateStacks (called before ExecuteDescribeStacks) was calling FindStacksMap, causing duplicate processing of all YAML files

**Measured Performance Improvement**:
- Setup 1: 12 min → 8.4 min (30% improvement with P1.1 + P1.2)
- Setup 2: 23 sec → 15 sec (35% improvement with P1.1 + P1.2 + P2.2)
- Setup 3: 15.8s (with P1.1 + P1.2 + P2.1 + P2.2)
- Setup 4: 6.8s (with P1.1 + P1.2 + P2.1 + P2.2 + P3.1 + P3.2 + P3.3)

### 🔄 Future Optimization Opportunities

**Status**: All P1, P2, and P3.1-P3.4 optimizations completed. Current performance: 6.8s (57% improvement from 15.8s baseline)

**Potential further optimizations** (if sub-5s performance is required):

#### P4.1: Iterative processCustomTags
**Expected Impact**: 1-2 second improvement
**Effort**: 2-3 hours
**Risk**: Medium (complexity)

Replace recursive traversal with iterative stack-based approach for shallow structures to reduce function call overhead.

#### P4.2: Parallel Component Processing
**Expected Impact**: 1-2 second improvement
**Effort**: 3-4 hours
**Risk**: High (race conditions, complexity)

Process independent components in parallel during describe stacks operations.

**Note**: With 57% performance improvement already achieved (15.8s → 6.8s), further optimizations should be evaluated based on actual user requirements and diminishing returns.

---

## References

- Customer heatmap data: `docs/img/heatmap/image007.png`, `image011.png`
- Analysis conversation: Current session
- Performance profile data: 2025-10-13 (15.8s elapsed, 5m7s CPU, ~19.5x parallelism)
- Related files:
  - `pkg/utils/yaml_utils.go` (processCustomTags)
  - `pkg/merge/merge.go` (MergeWithOptions, deepCopyMap)
  - `internal/exec/stack_processor_utils.go` (ProcessBaseComponentConfig)
