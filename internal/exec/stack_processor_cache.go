package exec

import (
	"os"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// File content sync map.
	getFileContentSyncMap = sync.Map{}

	// Base component inheritance cache to avoid re-processing the same inheritance chains.
	// Cache key: "stack:component:baseComponent" -> *schema.BaseComponentConfig (immutable post-insert).
	// No cache invalidation needed - configuration is immutable per command execution.
	//
	// Uses sync.Map (not a RWMutex-protected map) because the write path is highly
	// contended at scale: in a large-stack workload (~9k component instances across
	// ~700 stacks), the previous RWMutex.Lock() inside cacheBaseComponentConfig
	// serialized every write across goroutines, contributing ~5m50s of cumulative
	// wait time to the heatmap (55ms avg per call) despite the actual deep-copy
	// work being only ~525µs per call. The lock-free sync.Map removes the global
	// lock and optimizes for the disjoint-key write pattern this cache exhibits.
	baseComponentConfigCache sync.Map

	// JSON schema compilation cache to avoid re-compiling the same schema for every stack file.
	// Cache key: absolute file path to schema file -> compiled schema.
	// No cache invalidation needed - schemas are immutable per command execution.
	jsonSchemaCache   = make(map[string]*jsonschema.Schema)
	jsonSchemaCacheMu sync.RWMutex
)

// deepCopyBaseComponentConfigMaps deep copies all map fields from src to dst.
// Returns an error if any deep copy fails.
//
// This must cover EVERY AtmosSectionMapType field of BaseComponentConfig so
// the cache's returned-on-hit shape matches the freshly-computed shape from
// processBaseComponentConfigInternal. Missing a field here causes a silent
// correctness bug: cache MISS yields the full config, cache HIT yields a
// truncated one with the missed field as nil. The companion paired-string
// fields (BaseComponentRequiredVersion etc.) are copied by the struct
// literal in cacheBaseComponentConfig / getCachedBaseComponentConfig.
//
// Empty src fields are skipped (no DeepCopyMap call, no allocation). In the
// customer workload most components leave several of these fields empty
// (notably BaseComponentProviders / Hooks / Generate / Dependencies for
// non-Terraform components), so skipping the no-op deep-copies cuts the
// per-call cost in cacheBaseComponentConfig / getCachedBaseComponentConfig
// proportional to how many fields are unused. Phases 12/13 of the
// describe-affected perf investigation.
//
//nolint:cyclop,funlen // Cohesive field-by-field deep-copy; splitting would obscure that every map field of BaseComponentConfig is covered.
func deepCopyBaseComponentConfigMaps(dst, src *schema.BaseComponentConfig) error {
	var err error
	if len(src.BaseComponentVars) > 0 {
		if dst.BaseComponentVars, err = m.DeepCopyMap(src.BaseComponentVars); err != nil {
			return err
		}
	}
	if len(src.BaseComponentSettings) > 0 {
		if dst.BaseComponentSettings, err = m.DeepCopyMap(src.BaseComponentSettings); err != nil {
			return err
		}
	}
	if len(src.BaseComponentEnv) > 0 {
		if dst.BaseComponentEnv, err = m.DeepCopyMap(src.BaseComponentEnv); err != nil {
			return err
		}
	}
	if len(src.BaseComponentAuth) > 0 {
		if dst.BaseComponentAuth, err = m.DeepCopyMap(src.BaseComponentAuth); err != nil {
			return err
		}
	}
	if len(src.BaseComponentDependencies) > 0 {
		if dst.BaseComponentDependencies, err = m.DeepCopyMap(src.BaseComponentDependencies); err != nil {
			return err
		}
	}
	if len(src.BaseComponentLocals) > 0 {
		if dst.BaseComponentLocals, err = m.DeepCopyMap(src.BaseComponentLocals); err != nil {
			return err
		}
	}
	if len(src.BaseComponentMetadata) > 0 {
		if dst.BaseComponentMetadata, err = m.DeepCopyMap(src.BaseComponentMetadata); err != nil {
			return err
		}
	}
	if len(src.BaseComponentProviders) > 0 {
		if dst.BaseComponentProviders, err = m.DeepCopyMap(src.BaseComponentProviders); err != nil {
			return err
		}
	}
	if len(src.BaseComponentRequiredProviders) > 0 {
		if dst.BaseComponentRequiredProviders, err = m.DeepCopyMap(src.BaseComponentRequiredProviders); err != nil {
			return err
		}
	}
	if len(src.BaseComponentHooks) > 0 {
		if dst.BaseComponentHooks, err = m.DeepCopyMap(src.BaseComponentHooks); err != nil {
			return err
		}
	}
	if len(src.BaseComponentGenerate) > 0 {
		if dst.BaseComponentGenerate, err = m.DeepCopyMap(src.BaseComponentGenerate); err != nil {
			return err
		}
	}
	if len(src.BaseComponentBackendSection) > 0 {
		if dst.BaseComponentBackendSection, err = m.DeepCopyMap(src.BaseComponentBackendSection); err != nil {
			return err
		}
	}
	if len(src.BaseComponentRemoteStateBackendSection) > 0 {
		if dst.BaseComponentRemoteStateBackendSection, err = m.DeepCopyMap(src.BaseComponentRemoteStateBackendSection); err != nil {
			return err
		}
	}
	if len(src.BaseComponentSourceSection) > 0 {
		if dst.BaseComponentSourceSection, err = m.DeepCopyMap(src.BaseComponentSourceSection); err != nil {
			return err
		}
	}
	if len(src.BaseComponentProvisionSection) > 0 {
		if dst.BaseComponentProvisionSection, err = m.DeepCopyMap(src.BaseComponentProvisionSection); err != nil {
			return err
		}
	}
	return nil
}

// getCachedBaseComponentConfig retrieves a cached base component config if it exists.
// Returns a deep copy to prevent mutations affecting the cache.
//
// The deep copy runs outside the sync.Map.Load critical section: the cached
// pointer's target is immutable post-insert (see cacheBaseComponentConfig), so
// concurrent goroutines may safely deep-copy it without coordination.
func getCachedBaseComponentConfig(cacheKey string) (*schema.BaseComponentConfig, *[]string, bool) {
	defer perf.Track(nil, "exec.getCachedBaseComponentConfig")()

	raw, found := baseComponentConfigCache.Load(cacheKey)
	if !found {
		return nil, nil, false
	}
	cached, ok := raw.(*schema.BaseComponentConfig)
	if !ok {
		return nil, nil, false
	}

	// Deep copy to prevent external mutations from affecting the cache.
	// All map fields must be deep copied since they are mutable.
	copyConfig := schema.BaseComponentConfig{
		FinalBaseComponentName:              cached.FinalBaseComponentName,
		BaseComponentCommand:                cached.BaseComponentCommand,
		BaseComponentBackendType:            cached.BaseComponentBackendType,
		BaseComponentRemoteStateBackendType: cached.BaseComponentRemoteStateBackendType,
		// BaseComponentRequiredVersion is a string set by
		// processBaseComponentConfigInternal — it must round-trip through the
		// cache so callers that read it after a cache HIT see the same value
		// as after a cache MISS.
		BaseComponentRequiredVersion: cached.BaseComponentRequiredVersion,
	}

	// Deep copy all map fields.
	if err := deepCopyBaseComponentConfigMaps(&copyConfig, cached); err != nil {
		// If deep copy fails, return not found to force reprocessing.
		return nil, nil, false
	}

	// Deep copy the slice.
	copyBaseComponents := make([]string, len(cached.ComponentInheritanceChain))
	copy(copyBaseComponents, cached.ComponentInheritanceChain)
	copyConfig.ComponentInheritanceChain = copyBaseComponents

	return &copyConfig, &copyBaseComponents, true
}

// cacheBaseComponentConfig stores a base component config in the cache.
// Stores a deep copy to prevent external mutations from affecting the cache.
//
// The deep copy is performed BEFORE the store so the cache's critical section
// is only the sync.Map.Store call, not the ~525µs deep-copy work. Combined with
// sync.Map's lock-free read path, this keeps write contention out of the
// inheritance pipeline's hot path.
func cacheBaseComponentConfig(cacheKey string, config *schema.BaseComponentConfig) {
	defer perf.Track(nil, "exec.cacheBaseComponentConfig")()

	// Deep copy to prevent external mutations from affecting the cache.
	// All map fields must be deep copied since they are mutable.
	copyConfig := schema.BaseComponentConfig{
		FinalBaseComponentName:              config.FinalBaseComponentName,
		BaseComponentCommand:                config.BaseComponentCommand,
		BaseComponentBackendType:            config.BaseComponentBackendType,
		BaseComponentRemoteStateBackendType: config.BaseComponentRemoteStateBackendType,
		// BaseComponentRequiredVersion is a string set by
		// processBaseComponentConfigInternal — it must round-trip through the
		// cache so callers that read it after a cache HIT see the same value
		// as after a cache MISS.
		BaseComponentRequiredVersion: config.BaseComponentRequiredVersion,
	}

	// Deep copy all map fields.
	if err := deepCopyBaseComponentConfigMaps(&copyConfig, config); err != nil {
		// If deep copy fails, don't cache - return silently.
		return
	}

	// Deep copy the slice.
	copyBaseComponents := make([]string, len(config.ComponentInheritanceChain))
	copy(copyBaseComponents, config.ComponentInheritanceChain)
	copyConfig.ComponentInheritanceChain = copyBaseComponents

	baseComponentConfigCache.Store(cacheKey, &copyConfig)
}

// getCachedCompiledSchema retrieves a cached compiled JSON schema if it exists.
// The compiled schema is thread-safe for concurrent validation operations.
func getCachedCompiledSchema(schemaPath string) (*jsonschema.Schema, bool) {
	defer perf.Track(nil, "exec.getCachedCompiledSchema")()

	jsonSchemaCacheMu.RLock()
	defer jsonSchemaCacheMu.RUnlock()

	compiledSchema, found := jsonSchemaCache[schemaPath]
	return compiledSchema, found
}

// cacheCompiledSchema stores a compiled JSON schema in the cache.
// The compiled schema is thread-safe and can be safely shared across goroutines.
func cacheCompiledSchema(schemaPath string, schema *jsonschema.Schema) {
	defer perf.Track(nil, "exec.cacheCompiledSchema")()

	jsonSchemaCacheMu.Lock()
	defer jsonSchemaCacheMu.Unlock()

	jsonSchemaCache[schemaPath] = schema
}

// ClearBaseComponentConfigCache clears the base component config cache.
// This should be called between independent operations (like tests) to ensure fresh processing.
func ClearBaseComponentConfigCache() {
	defer perf.Track(nil, "exec.ClearBaseComponentConfigCache")()

	baseComponentConfigCache.Range(func(key, _ any) bool {
		baseComponentConfigCache.Delete(key)
		return true
	})
}

// ClearJsonSchemaCache clears the JSON schema cache.
// This should be called between independent operations (like tests) to ensure fresh processing.
func ClearJsonSchemaCache() {
	defer perf.Track(nil, "exec.ClearJsonSchemaCache")()

	jsonSchemaCacheMu.Lock()
	defer jsonSchemaCacheMu.Unlock()
	jsonSchemaCache = make(map[string]*jsonschema.Schema)
}

// ClearFileContentCache clears the file content cache.
// This should be called between independent operations (like tests) to ensure fresh processing.
func ClearFileContentCache() {
	defer perf.Track(nil, "exec.ClearFileContentCache")()

	getFileContentSyncMap.Range(func(key, value interface{}) bool {
		getFileContentSyncMap.Delete(key)
		return true
	})
}

// GetFileContent tries to read and return the file content from the sync map if it exists in the map.
// Otherwise, it reads the file, stores its content in the map, and returns the content.
func GetFileContent(filePath string) (string, error) {
	defer perf.Track(nil, "exec.GetFileContent")()

	if existingContent, found := getFileContentSyncMap.Load(filePath); found {
		switch v := existingContent.(type) {
		case []byte:
			return string(v), nil
		case string:
			return v, nil
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrReadFile).
			WithCause(err).
			WithContext("path", filePath).
			Err()
	}
	getFileContentSyncMap.Store(filePath, content)

	return string(content), nil
}

// GetFileContentWithoutCache reads file content without using the cache.
// Used when provenance tracking is enabled to ensure fresh reads with position tracking.
func GetFileContentWithoutCache(filePath string) (string, error) {
	defer perf.Track(nil, "exec.GetFileContentWithoutCache")()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrReadFile).
			WithCause(err).
			WithContext("path", filePath).
			Err()
	}

	return string(content), nil
}
