package json

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/stack/loader"
)

// errWrap is the error wrapping format string.
const errWrap = "%w: %w"

// Loader implements the loader.StackLoader interface for JSON files.
type Loader struct {
	// cache stores parsed JSON content to avoid re-parsing.
	cache   map[string]*cacheEntry
	cacheMu sync.RWMutex

	// Per-key locks for double-checked locking.
	locks   map[string]*sync.Mutex
	locksMu sync.Mutex
}

// cacheEntry stores a parsed JSON document and its metadata.
type cacheEntry struct {
	data      any
	positions map[string]loader.Position
}

// New creates a new JSON loader.
func New() *Loader {
	defer perf.Track(nil, "json.New")()

	return &Loader{
		cache: make(map[string]*cacheEntry),
		locks: make(map[string]*sync.Mutex),
	}
}

// Name returns the loader name.
func (l *Loader) Name() string {
	defer perf.Track(nil, "json.Loader.Name")()

	return "JSON"
}

// Extensions returns the supported file extensions.
func (l *Loader) Extensions() []string {
	defer perf.Track(nil, "json.Loader.Extensions")()

	return []string{".json"}
}

// Load parses JSON data and returns the result.
func (l *Loader) Load(ctx context.Context, data []byte, opts ...loader.LoadOption) (any, error) {
	defer perf.Track(nil, "json.Loader.Load")()

	result, _, err := l.LoadWithMetadata(ctx, data, opts...)
	return result, err
}

// LoadWithMetadata parses JSON data and returns the result with position metadata.
func (l *Loader) LoadWithMetadata(ctx context.Context, data []byte, opts ...loader.LoadOption) (any, *loader.Metadata, error) {
	defer perf.Track(nil, "json.Loader.LoadWithMetadata")()

	options := loader.ApplyLoadOptions(opts...)

	// Generate cache key from content hash.
	cacheKey := l.generateCacheKey(options.SourceFile, data)

	// Try cache first.
	if entry := l.getFromCache(cacheKey); entry != nil {
		return entry.data, l.buildMetadata(entry.positions, options.SourceFile), nil
	}

	// Parse with double-checked locking.
	result, positions, err := l.parseWithLocking(ctx, cacheKey, data)
	if err != nil {
		return nil, nil, err
	}

	return result, l.buildMetadata(positions, options.SourceFile), nil
}

// Encode converts data to JSON format.
func (l *Loader) Encode(ctx context.Context, data any, opts ...loader.EncodeOption) ([]byte, error) {
	defer perf.Track(nil, "json.Loader.Encode")()

	options := loader.ApplyEncodeOptions(opts...)

	// Check context cancellation.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)

	// Set indent unless compact output is requested.
	if !options.CompactOutput {
		indent := options.Indent
		if indent == "" {
			indent = "  " // Default to 2-space indent for readability.
		}
		encoder.SetIndent("", indent)
	}
	// When CompactOutput is true, don't call SetIndent for compact JSON.

	// Disable HTML escaping for cleaner output.
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(data); err != nil {
		return nil, fmt.Errorf(errWrap, errUtils.ErrEncodeFailed, err)
	}

	return buf.Bytes(), nil
}

// generateCacheKey creates a cache key from file path and content hash.
func (l *Loader) generateCacheKey(file string, content []byte) string {
	if file == "" || len(content) == 0 {
		return ""
	}

	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:])
	return file + ":" + contentHash
}

// getFromCache retrieves a cached entry if it exists.
func (l *Loader) getFromCache(key string) *cacheEntry {
	if key == "" {
		return nil
	}

	l.cacheMu.RLock()
	defer l.cacheMu.RUnlock()

	return l.cache[key]
}

// storeInCache stores a parsed result in the cache.
func (l *Loader) storeInCache(key string, entry *cacheEntry) {
	if key == "" || entry == nil {
		return
	}

	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()

	l.cache[key] = entry
}

// getLock returns or creates a lock for the given cache key.
// KNOWN LIMITATION: The locks map grows unbounded as new cache keys are added.
// In practice, this is bounded by the number of unique files processed during
// the Atmos session, which is typically small. Memory impact is minimal (one
// sync.Mutex per file). If memory becomes a concern, consider implementing
// periodic cleanup or using sync.Pool for lock recycling.
func (l *Loader) getLock(key string) *sync.Mutex {
	l.locksMu.Lock()
	defer l.locksMu.Unlock()

	mu, exists := l.locks[key]
	if !exists {
		mu = &sync.Mutex{}
		l.locks[key] = mu
	}
	return mu
}

// parseWithLocking parses JSON content with double-checked locking.
func (l *Loader) parseWithLocking(ctx context.Context, cacheKey string, data []byte) (any, map[string]loader.Position, error) {
	if cacheKey == "" {
		return l.parse(ctx, data)
	}

	mu := l.getLock(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	// Double-check cache after acquiring lock.
	if entry := l.getFromCache(cacheKey); entry != nil {
		return entry.data, entry.positions, nil
	}

	// Parse the data.
	result, positions, err := l.parse(ctx, data)
	if err != nil {
		return nil, nil, err
	}

	// Store in cache.
	l.storeInCache(cacheKey, &cacheEntry{
		data:      result,
		positions: positions,
	})

	return result, positions, nil
}

// parse performs the actual JSON parsing.
func (l *Loader) parse(ctx context.Context, data []byte) (any, map[string]loader.Position, error) {
	// Check context cancellation.
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	// Handle empty input.
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil, nil
	}

	// Parse with position tracking.
	var result any
	positions := make(map[string]loader.Position)

	// Use decoder for streaming parse with offset tracking.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber() // Preserve number precision.

	if err := decoder.Decode(&result); err != nil {
		return nil, nil, fmt.Errorf(errWrap, errUtils.ErrLoaderParseFailed, err)
	}

	// Check for trailing content after the JSON value.
	// Valid JSON files should contain exactly one value.
	if decoder.More() {
		return nil, nil, fmt.Errorf("%w: unexpected content after JSON value", errUtils.ErrLoaderParseFailed)
	}

	// Extract positions from the parsed structure.
	l.extractPositions(result, "", positions, data)

	return result, positions, nil
}

// extractPositions extracts position information by scanning the JSON content.
func (l *Loader) extractPositions(data any, path string, positions map[string]loader.Position, content []byte) {
	switch v := data.(type) {
	case map[string]any:
		for key, value := range v {
			keyPath := key
			if path != "" {
				keyPath = path + "." + key
			}
			// Find approximate position by searching for the key in content.
			pos := l.findKeyPosition(content, key)
			positions[keyPath] = pos
			l.extractPositions(value, keyPath, positions, content)
		}
	case []any:
		for i, value := range v {
			indexPath := fmt.Sprintf("%s[%d]", path, i)
			// For array elements, we store a placeholder position.
			positions[indexPath] = loader.Position{Line: 1, Column: 1}
			l.extractPositions(value, indexPath, positions, content)
		}
	}
}

// findKeyPosition finds the approximate line and column of a key in JSON content.
// KNOWN LIMITATION: This finds the first occurrence of the key pattern in the file,
// which may not be the correct position for nested objects with duplicate key names.
// For example, if {"a": {"name": 1}, "b": {"name": 2}}, searching for "name" will
// always return the position of the first occurrence. This is acceptable because:
// 1. Position tracking is best-effort for error reporting, not precise source mapping
// 2. Accurate nested key positioning requires a full JSON parser with position tracking
// 3. The primary use case (error messages) benefits from approximate locations.
func (l *Loader) findKeyPosition(content []byte, key string) loader.Position {
	// Search for the quoted key followed by colon.
	searchPattern := fmt.Sprintf(`"%s"`, key)
	idx := bytes.Index(content, []byte(searchPattern))
	if idx == -1 {
		return loader.Position{Line: 1, Column: 1}
	}

	// Count lines and find column.
	line := 1
	column := 1
	for i := 0; i < idx; i++ {
		if content[i] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}

	return loader.Position{Line: line, Column: column}
}

// buildMetadata constructs loader.Metadata from position information.
func (l *Loader) buildMetadata(positions map[string]loader.Position, sourceFile string) *loader.Metadata {
	return &loader.Metadata{
		Positions:  positions,
		SourceFile: sourceFile,
	}
}

// ClearCache clears the internal cache.
func (l *Loader) ClearCache() {
	defer perf.Track(nil, "json.Loader.ClearCache")()

	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()

	l.cache = make(map[string]*cacheEntry)
}

// CacheStats returns cache statistics.
func (l *Loader) CacheStats() (entries int, keys []string) {
	defer perf.Track(nil, "json.Loader.CacheStats")()

	l.cacheMu.RLock()
	defer l.cacheMu.RUnlock()

	entries = len(l.cache)
	keys = make([]string, 0, entries)
	for k := range l.cache {
		keys = append(keys, k)
	}
	return entries, keys
}
