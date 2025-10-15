package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Atmos YAML functions.
	AtmosYamlFuncExec            = "!exec"
	AtmosYamlFuncStore           = "!store"
	AtmosYamlFuncStoreGet        = "!store.get"
	AtmosYamlFuncTemplate        = "!template"
	AtmosYamlFuncTerraformOutput = "!terraform.output"
	AtmosYamlFuncTerraformState  = "!terraform.state"
	AtmosYamlFuncEnv             = "!env"
	AtmosYamlFuncInclude         = "!include"
	AtmosYamlFuncIncludeRaw      = "!include.raw"
	AtmosYamlFuncGitRoot         = "!repo-root"

	DefaultYAMLIndent = 2

	// Cache statistics constants.
	cacheStatsPercentageMultiplier = 100
	cacheStatsTopFilesCount        = 10
)

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

	// AtmosYamlTagsMap provides O(1) lookup for custom tag checking.
	// This optimization replaces the O(n) SliceContainsString calls that were previously
	// called 75M+ times, causing significant performance overhead.
	atmosYamlTagsMap = map[string]bool{
		AtmosYamlFuncExec:            true,
		AtmosYamlFuncStore:           true,
		AtmosYamlFuncStoreGet:        true,
		AtmosYamlFuncTemplate:        true,
		AtmosYamlFuncTerraformOutput: true,
		AtmosYamlFuncTerraformState:  true,
		AtmosYamlFuncEnv:             true,
	}

	// ParsedYAMLCache stores parsed yaml.Node objects and their position information
	// to avoid re-parsing the same files multiple times.
	// Cache key: file path + content hash.
	parsedYAMLCache   = make(map[string]*parsedYAMLCacheEntry)
	parsedYAMLCacheMu sync.RWMutex

	// Per-key locks to prevent race conditions when multiple goroutines
	// try to parse the same file simultaneously. This prevents 156+ goroutines
	// from all parsing the same file when they could share the result.
	parsedYAMLLocks   = make(map[string]*sync.Mutex)
	parsedYAMLLocksMu sync.Mutex

	// Cache statistics for debugging and optimization.
	parsedYAMLCacheStats = struct {
		sync.RWMutex
		hits         int64
		misses       int64
		totalCalls   int64
		uniqueFiles  map[string]int // file path -> call count
		uniqueHashes map[string]int // content hash -> call count
	}{
		uniqueFiles:  make(map[string]int),
		uniqueHashes: make(map[string]int),
	}

	ErrIncludeYamlFunctionInvalidArguments    = errors.New("invalid number of arguments in the !include function")
	ErrIncludeYamlFunctionInvalidFile         = errors.New("the !include function references a file that does not exist")
	ErrIncludeYamlFunctionInvalidAbsPath      = errors.New("failed to convert the file path to an absolute path in the !include function")
	ErrIncludeYamlFunctionFailedStackManifest = errors.New("failed to process the stack manifest with the !include function")
	ErrNilAtmosConfig                         = errors.New("atmosConfig cannot be nil")

	// Buffer pool that reuses bytes.Buffer objects to reduce allocations in YAML encoding.
	// Buffer pooling significantly reduces memory allocations and GC pressure when
	// converting large data structures to YAML, which happens frequently during
	// stack processing and output generation.
	yamlBufferPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
)

// parsedYAMLCacheEntry stores a parsed YAML node and its position information.
type parsedYAMLCacheEntry struct {
	node      yaml.Node
	positions PositionMap
}

// generateParsedYAMLCacheKey generates a cache key from file path and content.
// The content hash ensures that template-processed files with different contexts
// get different cache entries, while static files benefit from path-only caching.
func generateParsedYAMLCacheKey(file string, content string) string {
	if file == "" || content == "" {
		return ""
	}

	// Compute SHA256 hash of content.
	hash := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(hash[:])

	// Cache key format: "filepath:contenthash"
	// This ensures that:
	// - Static files (same content): same cache key → cache hit
	// - Template files with same context: same cache key → cache hit
	// - Template files with different context: different cache key → cache miss (correct behavior)
	return file + ":" + contentHash
}

// getOrCreateCacheLock returns a mutex for the given cache key.
// This implements per-key locking to prevent race conditions when multiple
// goroutines try to parse the same file simultaneously.
func getOrCreateCacheLock(cacheKey string) *sync.Mutex {
	parsedYAMLLocksMu.Lock()
	defer parsedYAMLLocksMu.Unlock()

	mu, exists := parsedYAMLLocks[cacheKey]
	if !exists {
		mu = &sync.Mutex{}
		parsedYAMLLocks[cacheKey] = mu
	}
	return mu
}

// getCachedParsedYAML retrieves a cached parsed YAML node if it exists.
// Returns a copy of the node to prevent external mutations.
// Note: Statistics tracking is done by the caller to avoid double-counting.
// Note: perf.Track() removed from this hot path to reduce overhead.
func getCachedParsedYAML(file string, content string) (*yaml.Node, PositionMap, bool) {
	cacheKey := generateParsedYAMLCacheKey(file, content)
	if cacheKey == "" {
		return nil, nil, false
	}

	parsedYAMLCacheMu.RLock()
	defer parsedYAMLCacheMu.RUnlock()

	entry, found := parsedYAMLCache[cacheKey]
	if !found {
		return nil, nil, false
	}

	// Return a copy of the node to prevent mutations affecting the cache.
	nodeCopy := entry.node
	return &nodeCopy, entry.positions, true
}

// cacheParsedYAML stores a parsed YAML node in the cache.
// Stores a copy to prevent external mutations from affecting the cache.
// Note: perf.Track() removed from this hot path to reduce overhead.
func cacheParsedYAML(file string, content string, node *yaml.Node, positions PositionMap) {
	cacheKey := generateParsedYAMLCacheKey(file, content)
	if cacheKey == "" || node == nil {
		return
	}

	parsedYAMLCacheMu.Lock()
	defer parsedYAMLCacheMu.Unlock()

	// Store a copy to prevent external mutations from affecting the cache.
	nodeCopy := *node
	parsedYAMLCache[cacheKey] = &parsedYAMLCacheEntry{
		node:      nodeCopy,
		positions: positions,
	}
}

// parseAndCacheYAML parses YAML content and caches the result.
// This is extracted to reduce nesting complexity in UnmarshalYAMLFromFileWithPositions.
func parseAndCacheYAML(atmosConfig *schema.AtmosConfiguration, input string, file string) (*yaml.Node, PositionMap, error) {
	// Parse the YAML.
	var parsedNode yaml.Node
	b := []byte(input)

	// Unmarshal into yaml.Node.
	if err := yaml.Unmarshal(b, &parsedNode); err != nil {
		return nil, nil, err
	}

	// Extract positions if provenance tracking is enabled.
	var positions PositionMap
	if atmosConfig.TrackProvenance {
		positions = ExtractYAMLPositions(&parsedNode, true)
	}

	// Process custom tags.
	if err := processCustomTags(atmosConfig, &parsedNode, file); err != nil {
		return nil, nil, err
	}

	// Cache the parsed and processed node with content-aware key.
	cacheParsedYAML(file, input, &parsedNode, positions)

	return &parsedNode, positions, nil
}

// handleCacheMiss handles the cache miss case with per-key locking and double-checked locking.
// This prevents multiple goroutines from parsing the same file simultaneously.
func handleCacheMiss(atmosConfig *schema.AtmosConfiguration, file string, input string) (*yaml.Node, PositionMap, error) {
	cacheKey := generateParsedYAMLCacheKey(file, input)
	mu := getOrCreateCacheLock(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	// Double-check: another goroutine may have cached it while we waited for the lock.
	node, positions, found := getCachedParsedYAML(file, input)
	if found {
		// Another goroutine cached it while we waited - cache hit!
		parsedYAMLCacheStats.Lock()
		parsedYAMLCacheStats.hits++
		parsedYAMLCacheStats.Unlock()
		return node, positions, nil
	}

	// Still not in cache - we're the first goroutine to parse this file.
	// Track cache miss.
	parsedYAMLCacheStats.Lock()
	parsedYAMLCacheStats.misses++
	parsedYAMLCacheStats.Unlock()

	// Parse and cache the YAML.
	node, positions, err := parseAndCacheYAML(atmosConfig, input, file)
	if err != nil {
		return nil, nil, err
	}

	return node, positions, nil
}

// PrintParsedYAMLCacheStats prints cache statistics for debugging.
// This helps identify cache effectiveness and opportunities for optimization.
func PrintParsedYAMLCacheStats() {
	parsedYAMLCacheStats.RLock()
	defer parsedYAMLCacheStats.RUnlock()

	totalCalls := parsedYAMLCacheStats.totalCalls
	hits := parsedYAMLCacheStats.hits
	misses := parsedYAMLCacheStats.misses
	uniqueFiles := len(parsedYAMLCacheStats.uniqueFiles)
	uniqueHashes := len(parsedYAMLCacheStats.uniqueHashes)

	var hitRate float64
	if totalCalls > 0 {
		hitRate = float64(hits) / float64(totalCalls) * cacheStatsPercentageMultiplier
	}

	log.Info("YAML Cache Statistics",
		"totalCalls", totalCalls,
		"cacheHits", hits,
		"cacheMisses", misses,
		"hitRate", hitRate,
		"uniqueFiles", uniqueFiles,
		"uniqueHashes", uniqueHashes,
		"callsPerFile", float64(totalCalls)/float64(uniqueFiles),
		"callsPerHash", float64(totalCalls)/float64(uniqueHashes),
	)

	// Print top files by call count.
	type fileCount struct {
		file  string
		count int
	}
	var fileCounts []fileCount
	for file, count := range parsedYAMLCacheStats.uniqueFiles {
		fileCounts = append(fileCounts, fileCount{file, count})
	}

	// Sort by count descending.
	for i := 0; i < len(fileCounts); i++ {
		for j := i + 1; j < len(fileCounts); j++ {
			if fileCounts[j].count > fileCounts[i].count {
				fileCounts[i], fileCounts[j] = fileCounts[j], fileCounts[i]
			}
		}
	}

	// Print top most-called files.
	log.Info("Top 10 most-called files:")
	for i := 0; i < cacheStatsTopFilesCount && i < len(fileCounts); i++ {
		log.Info("  ", "file", fileCounts[i].file, "calls", fileCounts[i].count)
	}
}

// PrintAsYAML prints the provided value as YAML document to the console with syntax highlighting.
// Use PrintAsYAMLSimple for non-TTY output (pipes, redirects) to avoid expensive highlighting.
func PrintAsYAML(atmosConfig *schema.AtmosConfiguration, data any) error {
	defer perf.Track(atmosConfig, "utils.PrintAsYAML")()

	y, err := GetHighlightedYAML(atmosConfig, data)
	if err != nil {
		return err
	}
	PrintMessage(y)
	return nil
}

// PrintAsYAMLSimple prints the provided value as YAML document without syntax highlighting.
// This is a fast-path for non-TTY output (files, pipes, redirects) that skips expensive
// syntax highlighting, reducing output time from ~6s to <1s for large configurations.
func PrintAsYAMLSimple(atmosConfig *schema.AtmosConfiguration, data any) error {
	defer perf.Track(atmosConfig, "utils.PrintAsYAMLSimple")()

	if atmosConfig == nil {
		return ErrNilAtmosConfig
	}

	indent := getIndentFromConfig(atmosConfig)
	y, err := ConvertToYAML(data, YAMLOptions{Indent: indent})
	if err != nil {
		return err
	}
	PrintMessage(y)
	return nil
}

func getIndentFromConfig(atmosConfig *schema.AtmosConfiguration) int {
	if atmosConfig == nil || atmosConfig.Settings.Terminal.TabWidth <= 0 {
		return DefaultYAMLIndent
	}
	return atmosConfig.Settings.Terminal.TabWidth
}

func PrintAsYAMLWithConfig(atmosConfig *schema.AtmosConfiguration, data any) error {
	defer perf.Track(atmosConfig, "utils.PrintAsYAMLWithConfig")()

	if atmosConfig == nil {
		return ErrNilAtmosConfig
	}

	indent := getIndentFromConfig(atmosConfig)
	y, err := ConvertToYAML(data, YAMLOptions{Indent: indent})
	if err != nil {
		return err
	}

	highlighted, err := HighlightCodeWithConfig(atmosConfig, y, "yaml")
	if err != nil {
		PrintMessage(y)
		return nil
	}
	PrintMessage(highlighted)
	return nil
}

func GetHighlightedYAML(atmosConfig *schema.AtmosConfiguration, data any) (string, error) {
	defer perf.Track(atmosConfig, "utils.GetHighlightedYAML")()

	y, err := ConvertToYAML(data)
	if err != nil {
		return "", err
	}
	highlighted, err := HighlightCodeWithConfig(atmosConfig, y)
	if err != nil {
		return y, err
	}
	return highlighted, nil
}

// PrintAsYAMLToFileDescriptor prints the provided value as YAML document to a file descriptor.
func PrintAsYAMLToFileDescriptor(atmosConfig *schema.AtmosConfiguration, data any) error {
	defer perf.Track(atmosConfig, "utils.PrintAsYAMLToFileDescriptor")()

	if atmosConfig == nil {
		return ErrNilAtmosConfig
	}

	indent := getIndentFromConfig(atmosConfig)
	y, err := ConvertToYAML(data, YAMLOptions{Indent: indent})
	if err != nil {
		return err
	}

	log.Debug("PrintAsYAMLToFileDescriptor", "data", y)
	return nil
}

// WriteToFileAsYAML converts the provided value to YAML and writes it to the specified file.
func WriteToFileAsYAML(filePath string, data any, fileMode os.FileMode) error {
	defer perf.Track(nil, "utils.WriteToFileAsYAML")()

	y, err := ConvertToYAML(data, YAMLOptions{Indent: DefaultYAMLIndent})
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, []byte(y), fileMode)
	if err != nil {
		return err
	}
	return nil
}

func WriteToFileAsYAMLWithConfig(atmosConfig *schema.AtmosConfiguration, filePath string, data any, fileMode os.FileMode) error {
	defer perf.Track(atmosConfig, "utils.WriteToFileAsYAMLWithConfig")()

	if atmosConfig == nil {
		return ErrNilAtmosConfig
	}

	indent := getIndentFromConfig(atmosConfig)
	log.Debug("WriteToFileAsYAMLWithConfig", "tabWidth", indent, "filePath", filePath)

	y, err := ConvertToYAML(data, YAMLOptions{Indent: indent})
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, []byte(y), fileMode)
	if err != nil {
		return err
	}
	return nil
}

type YAMLOptions struct {
	Indent int
}

// LongString is a string type that encodes as a YAML folded scalar (>).
// This is used to wrap long strings across multiple lines for better readability.
type LongString string

// MarshalYAML implements yaml.Marshaler to encode as a folded scalar.
func (s LongString) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Style: yaml.FoldedStyle, // Use > style for folded scalar
		Value: string(s),
	}
	return node, nil
}

// WrapLongStrings walks a data structure and converts strings longer than maxLength
// to LongString type, which will be encoded as YAML folded scalars (>) for better readability.
func WrapLongStrings(data any, maxLength int) any {
	defer perf.Track(nil, "utils.WrapLongStrings")()

	if maxLength <= 0 {
		return data
	}

	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, value := range v {
			result[key] = WrapLongStrings(value, maxLength)
		}
		return result

	case []any:
		result := make([]any, len(v))
		for i, value := range v {
			result[i] = WrapLongStrings(value, maxLength)
		}
		return result

	case string:
		// Convert long single-line strings to LongString
		if len(v) > maxLength && !strings.Contains(v, "\n") {
			return LongString(v)
		}
		return v

	default:
		// For all other types (int, bool, etc.), return as-is
		return data
	}
}

func ConvertToYAML(data any, opts ...YAMLOptions) (string, error) {
	defer perf.Track(nil, "utils.ConvertToYAML")()

	// Get a buffer from the pool to reduce allocations.
	buf := yamlBufferPool.Get().(*bytes.Buffer)
	buf.Reset() // Ensure buffer is clean.

	// Return buffer to pool when done.
	defer func() {
		buf.Reset()
		yamlBufferPool.Put(buf)
	}()

	encoder := yaml.NewEncoder(buf)

	indent := DefaultYAMLIndent
	if len(opts) > 0 && opts[0].Indent > 0 {
		indent = opts[0].Indent
	}
	encoder.SetIndent(indent)

	if err := encoder.Encode(data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

//nolint:gocognit,revive
func processCustomTags(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, file string) error {
	defer perf.Track(atmosConfig, "utils.processCustomTags")()

	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return processCustomTags(atmosConfig, node.Content[0], file)
	}

	// Early exit: skip processing if this subtree has no custom tags.
	// This avoids expensive recursive processing for YAML subtrees that don't use custom tags.
	// Most YAML content doesn't use custom tags, so this optimization significantly reduces
	// unnecessary recursion and tag checking.
	if !hasCustomTags(node) {
		return nil
	}

	for _, n := range node.Content {
		tag := strings.TrimSpace(n.Tag)
		val := strings.TrimSpace(n.Value)

		// Use O(1) map lookup instead of O(n) slice search for performance.
		// This optimization reduces 75M+ linear searches to constant-time lookups.
		if atmosYamlTagsMap[tag] {
			n.Value = getValueWithTag(n)
			// Clear the custom tag to prevent the YAML decoder from processing it again.
			// We keep the value as is since it will be processed later by processCustomTags.
			// We don't set a specific type tag (like !!str) because the function might return
			// any type (string, map, list, etc.) when it's actually executed.
			n.Tag = ""
		}

		// Handle the !include tag with extension-based parsing
		if tag == AtmosYamlFuncInclude {
			if err := ProcessIncludeTag(atmosConfig, n, val, file); err != nil {
				return err
			}
		}

		// Handle the !include.raw tag (always returns raw string)
		if tag == AtmosYamlFuncIncludeRaw {
			if err := ProcessIncludeRawTag(atmosConfig, n, val, file); err != nil {
				return err
			}
		}

		// Recursively process the child nodes
		if len(n.Content) > 0 {
			if err := processCustomTags(atmosConfig, n, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func getValueWithTag(n *yaml.Node) string {
	tag := strings.TrimSpace(n.Tag)
	val := strings.TrimSpace(n.Value)
	return strings.TrimSpace(tag + " " + val)
}

// hasCustomTags performs a fast scan to check if a node or any of its children contain custom Atmos tags.
// This enables early exit optimization in processCustomTags, avoiding expensive recursive processing
// for YAML subtrees that don't use custom tags (which is the majority of YAML content).
func hasCustomTags(node *yaml.Node) bool {
	if node == nil {
		return false
	}

	// Check if this node has a custom tag.
	tag := strings.TrimSpace(node.Tag)
	if atmosYamlTagsMap[tag] || tag == AtmosYamlFuncInclude || tag == AtmosYamlFuncIncludeRaw {
		return true
	}

	// Recursively check children.
	for _, child := range node.Content {
		if hasCustomTags(child) {
			return true
		}
	}

	return false
}

// UnmarshalYAML unmarshals YAML into a Go type.
func UnmarshalYAML[T any](input string) (T, error) {
	return UnmarshalYAMLFromFile[T](&schema.AtmosConfiguration{}, input, "")
}

// UnmarshalYAMLFromFile unmarshals YAML downloaded from a file into a Go type.
func UnmarshalYAMLFromFile[T any](atmosConfig *schema.AtmosConfiguration, input string, file string) (T, error) {
	defer perf.Track(atmosConfig, "utils.UnmarshalYAMLFromFile")()

	if atmosConfig == nil {
		return *new(T), ErrNilAtmosConfig
	}

	var zeroValue T
	var node yaml.Node
	b := []byte(input)

	// Unmarshal into yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return zeroValue, err
	}

	if err := processCustomTags(atmosConfig, &node, file); err != nil {
		return zeroValue, err
	}

	// Decode the yaml.Node into the desired type T
	var data T
	if err := node.Decode(&data); err != nil {
		return zeroValue, err
	}

	return data, nil
}

// UnmarshalYAMLFromFileWithPositions unmarshals YAML and returns position information.
// The positions map contains line/column information for each value in the YAML.
// If atmosConfig.TrackProvenance is false, returns an empty position map.
// Uses caching with content-aware keys to correctly handle template-processed files.
// The cache key includes both file path and content hash, ensuring that:
// - Static files are cached by path (same content = cache hit)
// - Template files with same context get cache hits
// - Template files with different contexts get separate cache entries (correct behavior).
func UnmarshalYAMLFromFileWithPositions[T any](atmosConfig *schema.AtmosConfiguration, input string, file string) (T, PositionMap, error) {
	defer perf.Track(atmosConfig, "utils.UnmarshalYAMLFromFileWithPositions")()

	if atmosConfig == nil {
		return *new(T), nil, ErrNilAtmosConfig
	}

	var zeroValue T

	// Track total calls and unique files/hashes.
	parsedYAMLCacheStats.Lock()
	parsedYAMLCacheStats.totalCalls++
	parsedYAMLCacheStats.uniqueFiles[file]++
	// Extract content hash for tracking.
	hash := sha256.Sum256([]byte(input))
	contentHash := hex.EncodeToString(hash[:])
	parsedYAMLCacheStats.uniqueHashes[contentHash]++
	parsedYAMLCacheStats.Unlock()

	// Try to get cached parsed YAML first (fast path with read lock).
	node, positions, found := getCachedParsedYAML(file, input)
	if found {
		// Cache hit on first check.
		parsedYAMLCacheStats.Lock()
		parsedYAMLCacheStats.hits++
		parsedYAMLCacheStats.Unlock()
	} else {
		// Cache miss - use per-key locking to prevent multiple goroutines
		// from parsing the same file simultaneously.
		var err error
		node, positions, err = handleCacheMiss(atmosConfig, file, input)
		if err != nil {
			return zeroValue, nil, err
		}
	}

	// Decode the yaml.Node into the desired type T.
	var data T
	if err := node.Decode(&data); err != nil {
		return zeroValue, nil, err
	}

	// Apply string interning for map[string]any types to reduce memory usage.
	// String interning deduplicates common strings across YAML files:
	// - Common keys: "vars", "settings", "metadata", "env", "backend", etc.
	// - Common values: region names, "true", "false", component/stack names, etc.
	// This can save significant memory when loading many similar configs.
	// Only intern non-nil, non-empty maps to preserve original nil/empty semantics.
	if m, ok := any(data).(map[string]any); ok && m != nil && len(m) > 0 {
		interned := InternStringsInMap(atmosConfig, m)
		data = interned.(T)
	}

	return data, positions, nil
}

// InternStringsInMap recursively interns all string keys and string values in a map[string]any.
// This reduces memory usage by deduplicating common strings across YAML files.
// Common interned values: component names, stack names, "true"/"false", region names, etc.
// Note: perf.Track removed from this critical path function as it's called recursively many times.
func InternStringsInMap(atmosConfig *schema.AtmosConfiguration, data any) any {
	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, val := range v {
			// Intern the key (common keys: vars, settings, metadata, env, backend, etc.)
			internedKey := Intern(atmosConfig, k)
			// Recursively process the value
			result[internedKey] = InternStringsInMap(atmosConfig, val)
		}
		return result

	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = InternStringsInMap(atmosConfig, val)
		}
		return result

	case string:
		// Intern string values
		return Intern(atmosConfig, v)

	default:
		// For all other types (int, bool, float, etc.), return as-is
		return data
	}
}
