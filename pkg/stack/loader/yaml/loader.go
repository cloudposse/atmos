package yaml

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/stack/loader"
)

// errWrap is the error wrapping format string.
const errWrap = "%w: %w"

// Loader implements the loader.StackLoader interface for YAML files.
type Loader struct {
	// cache stores parsed YAML content to avoid re-parsing.
	cache   map[string]*cacheEntry
	cacheMu sync.RWMutex

	// Per-key locks for double-checked locking.
	locks   map[string]*sync.Mutex
	locksMu sync.Mutex
}

// cacheEntry stores a parsed YAML document and its metadata.
type cacheEntry struct {
	data      any
	positions map[string]loader.Position
}

// New creates a new YAML loader.
func New() *Loader {
	defer perf.Track(nil, "yaml.New")()

	return &Loader{
		cache: make(map[string]*cacheEntry),
		locks: make(map[string]*sync.Mutex),
	}
}

// Name returns the loader name.
func (l *Loader) Name() string {
	defer perf.Track(nil, "yaml.Loader.Name")()

	return "YAML"
}

// Extensions returns the supported file extensions.
func (l *Loader) Extensions() []string {
	defer perf.Track(nil, "yaml.Loader.Extensions")()

	return []string{".yaml", ".yml"}
}

// Load parses YAML data and returns the result.
func (l *Loader) Load(ctx context.Context, data []byte, opts ...loader.LoadOption) (any, error) {
	defer perf.Track(nil, "yaml.Loader.Load")()

	result, _, err := l.LoadWithMetadata(ctx, data, opts...)
	return result, err
}

// LoadWithMetadata parses YAML data and returns the result with position metadata.
func (l *Loader) LoadWithMetadata(ctx context.Context, data []byte, opts ...loader.LoadOption) (any, *loader.Metadata, error) {
	defer perf.Track(nil, "yaml.Loader.LoadWithMetadata")()

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

// Encode converts data back to YAML format.
func (l *Loader) Encode(ctx context.Context, data any, opts ...loader.EncodeOption) (result []byte, err error) {
	defer perf.Track(nil, "yaml.Loader.Encode")()

	// Recover from yaml.v3 panics on unsupported types (channels, functions, etc.).
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errUtils.ErrEncodeFailed, r)
		}
	}()

	options := loader.ApplyEncodeOptions(opts...)

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)

	// Calculate indent width from string.
	indentWidth := len(options.Indent)
	if indentWidth < 1 {
		indentWidth = 2
	}
	encoder.SetIndent(indentWidth)

	if err := encoder.Encode(data); err != nil {
		return nil, fmt.Errorf(errWrap, errUtils.ErrEncodeFailed, err)
	}

	if err := encoder.Close(); err != nil {
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

// parseWithLocking parses YAML content with double-checked locking.
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

// parse performs the actual YAML parsing.
func (l *Loader) parse(ctx context.Context, data []byte) (any, map[string]loader.Position, error) {
	// Check context cancellation.
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	// Parse into yaml.Node for position tracking.
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, nil, fmt.Errorf(errWrap, errUtils.ErrLoaderParseFailed, err)
	}

	// Extract positions.
	positions := l.extractPositions(&node)

	// Decode into generic structure.
	var result any
	if err := node.Decode(&result); err != nil {
		return nil, nil, fmt.Errorf(errWrap, errUtils.ErrLoaderParseFailed, err)
	}

	return result, positions, nil
}

// extractPositions extracts position information from a YAML node.
func (l *Loader) extractPositions(node *yaml.Node) map[string]loader.Position {
	positions := make(map[string]loader.Position)
	l.extractPositionsRecursive(node, "", positions)
	return positions
}

// extractPositionsRecursive recursively extracts positions from a YAML tree.
func (l *Loader) extractPositionsRecursive(node *yaml.Node, path string, positions map[string]loader.Position) {
	if node == nil {
		return
	}

	// Store position for this path.
	if path != "" {
		positions[path] = loader.Position{
			Line:   node.Line,
			Column: node.Column,
		}
	}

	switch node.Kind {
	case yaml.DocumentNode:
		// Process document content.
		for _, child := range node.Content {
			l.extractPositionsRecursive(child, path, positions)
		}

	case yaml.MappingNode:
		// Process key-value pairs (content alternates key, value, key, value...).
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			key := keyNode.Value
			newPath := key
			if path != "" {
				newPath = path + "." + key
			}

			// Store key position.
			positions[newPath] = loader.Position{
				Line:   keyNode.Line,
				Column: keyNode.Column,
			}

			// Recurse into value.
			l.extractPositionsRecursive(valueNode, newPath, positions)
		}

	case yaml.SequenceNode:
		// Process array elements.
		for i, child := range node.Content {
			indexPath := fmt.Sprintf("%s[%d]", path, i)
			l.extractPositionsRecursive(child, indexPath, positions)
		}
	}
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
	defer perf.Track(nil, "yaml.Loader.ClearCache")()

	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()

	l.cache = make(map[string]*cacheEntry)
}

// CacheStats returns cache statistics.
func (l *Loader) CacheStats() (entries int, keys []string) {
	defer perf.Track(nil, "yaml.Loader.CacheStats")()

	l.cacheMu.RLock()
	defer l.cacheMu.RUnlock()

	entries = len(l.cache)
	keys = make([]string, 0, entries)
	for k := range l.cache {
		keys = append(keys, k)
	}
	return entries, keys
}
