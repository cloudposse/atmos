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
	// Cache key: "stack:component:baseComponent" -> BaseComponentConfig.
	// No cache invalidation needed - configuration is immutable per command execution.
	baseComponentConfigCache   = make(map[string]*schema.BaseComponentConfig)
	baseComponentConfigCacheMu sync.RWMutex

	// JSON schema compilation cache to avoid re-compiling the same schema for every stack file.
	// Cache key: absolute file path to schema file -> compiled schema.
	// No cache invalidation needed - schemas are immutable per command execution.
	jsonSchemaCache   = make(map[string]*jsonschema.Schema)
	jsonSchemaCacheMu sync.RWMutex
)

// deepCopyBaseComponentConfigMaps deep copies all map fields from src to dst.
// Returns an error if any deep copy fails.
func deepCopyBaseComponentConfigMaps(dst, src *schema.BaseComponentConfig) error {
	var err error
	if dst.BaseComponentVars, err = m.DeepCopyMap(src.BaseComponentVars); err != nil {
		return err
	}
	if dst.BaseComponentSettings, err = m.DeepCopyMap(src.BaseComponentSettings); err != nil {
		return err
	}
	if dst.BaseComponentEnv, err = m.DeepCopyMap(src.BaseComponentEnv); err != nil {
		return err
	}
	if dst.BaseComponentAuth, err = m.DeepCopyMap(src.BaseComponentAuth); err != nil {
		return err
	}
	if dst.BaseComponentMetadata, err = m.DeepCopyMap(src.BaseComponentMetadata); err != nil {
		return err
	}
	if dst.BaseComponentProviders, err = m.DeepCopyMap(src.BaseComponentProviders); err != nil {
		return err
	}
	if dst.BaseComponentHooks, err = m.DeepCopyMap(src.BaseComponentHooks); err != nil {
		return err
	}
	if dst.BaseComponentBackendSection, err = m.DeepCopyMap(src.BaseComponentBackendSection); err != nil {
		return err
	}
	if dst.BaseComponentRemoteStateBackendSection, err = m.DeepCopyMap(src.BaseComponentRemoteStateBackendSection); err != nil {
		return err
	}
	return nil
}

// getCachedBaseComponentConfig retrieves a cached base component config if it exists.
// Returns a deep copy to prevent mutations affecting the cache.
func getCachedBaseComponentConfig(cacheKey string) (*schema.BaseComponentConfig, *[]string, bool) {
	defer perf.Track(nil, "exec.getCachedBaseComponentConfig")()

	baseComponentConfigCacheMu.RLock()
	defer baseComponentConfigCacheMu.RUnlock()

	cached, found := baseComponentConfigCache[cacheKey]
	if !found {
		return nil, nil, false
	}

	// Deep copy to prevent external mutations from affecting the cache.
	// All map fields must be deep copied since they are mutable.
	copyConfig := schema.BaseComponentConfig{
		FinalBaseComponentName:              cached.FinalBaseComponentName,
		BaseComponentCommand:                cached.BaseComponentCommand,
		BaseComponentBackendType:            cached.BaseComponentBackendType,
		BaseComponentRemoteStateBackendType: cached.BaseComponentRemoteStateBackendType,
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
func cacheBaseComponentConfig(cacheKey string, config *schema.BaseComponentConfig) {
	defer perf.Track(nil, "exec.cacheBaseComponentConfig")()

	baseComponentConfigCacheMu.Lock()
	defer baseComponentConfigCacheMu.Unlock()

	// Deep copy to prevent external mutations from affecting the cache.
	// All map fields must be deep copied since they are mutable.
	copyConfig := schema.BaseComponentConfig{
		FinalBaseComponentName:              config.FinalBaseComponentName,
		BaseComponentCommand:                config.BaseComponentCommand,
		BaseComponentBackendType:            config.BaseComponentBackendType,
		BaseComponentRemoteStateBackendType: config.BaseComponentRemoteStateBackendType,
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

	baseComponentConfigCache[cacheKey] = &copyConfig
}

// getCachedCompiledSchema retrieves a cached compiled JSON schema if it exists.
// The compiled schema is thread-safe for concurrent validation operations.
func getCachedCompiledSchema(schemaPath string) (*jsonschema.Schema, bool) {
	defer perf.Track(nil, "exec.getCachedCompiledSchema")()

	jsonSchemaCacheMu.RLock()
	defer jsonSchemaCacheMu.RUnlock()

	schema, found := jsonSchemaCache[schemaPath]
	return schema, found
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

	baseComponentConfigCacheMu.Lock()
	defer baseComponentConfigCacheMu.Unlock()
	baseComponentConfigCache = make(map[string]*schema.BaseComponentConfig)
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
