package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	atmosGit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Atmos YAML functions.
	AtmosYamlFuncExec                    = "!exec"
	AtmosYamlFuncStore                   = "!store"
	AtmosYamlFuncStoreGet                = "!store.get"
	AtmosYamlFuncTemplate                = "!template"
	AtmosYamlFuncTerraformOutput         = "!terraform.output"
	AtmosYamlFuncTerraformState          = "!terraform.state"
	AtmosYamlFuncEnv                     = "!env"
	AtmosYamlFuncInclude                 = "!include"
	AtmosYamlFuncIncludeRaw              = "!include.raw"
	AtmosYamlFuncGitRoot                 = atmosGit.YAMLFuncRepoRoot
	AtmosYamlFuncGitRootAlias            = atmosGit.YAMLFuncRoot
	AtmosYamlFuncGitSha                  = atmosGit.YAMLFuncSHA
	AtmosYamlFuncGitBranch               = atmosGit.YAMLFuncBranch
	AtmosYamlFuncGitRef                  = atmosGit.YAMLFuncRef
	AtmosYamlFuncGitRepository           = atmosGit.YAMLFuncRepository
	AtmosYamlFuncGitOwner                = atmosGit.YAMLFuncOwner
	AtmosYamlFuncGitName                 = atmosGit.YAMLFuncName
	AtmosYamlFuncGitHost                 = atmosGit.YAMLFuncHost
	AtmosYamlFuncGitUrl                  = atmosGit.YAMLFuncURL
	AtmosYamlFuncCwd                     = "!cwd"
	AtmosYamlFuncRandom                  = "!random"
	AtmosYamlFuncLiteral                 = "!literal"
	AtmosYamlFuncAwsAccountID            = "!aws.account_id"
	AtmosYamlFuncAwsCallerIdentityArn    = "!aws.caller_identity_arn"
	AtmosYamlFuncAwsCallerIdentityUserID = "!aws.caller_identity_user_id"
	AtmosYamlFuncAwsRegion               = "!aws.region"
	AtmosYamlFuncAwsOrganizationID       = "!aws.organization_id"

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
		AtmosYamlFuncGitRoot,
		AtmosYamlFuncGitRootAlias,
		AtmosYamlFuncGitSha,
		AtmosYamlFuncGitBranch,
		AtmosYamlFuncGitRef,
		AtmosYamlFuncGitRepository,
		AtmosYamlFuncGitOwner,
		AtmosYamlFuncGitName,
		AtmosYamlFuncGitHost,
		AtmosYamlFuncGitUrl,
		AtmosYamlFuncCwd,
		AtmosYamlFuncRandom,
		AtmosYamlFuncLiteral,
		AtmosYamlFuncAwsAccountID,
		AtmosYamlFuncAwsCallerIdentityArn,
		AtmosYamlFuncAwsCallerIdentityUserID,
		AtmosYamlFuncAwsRegion,
		AtmosYamlFuncAwsOrganizationID,
	}

	// AtmosYamlTagsMap provides O(1) lookup for custom tag checking.
	// This optimization replaces the O(n) SliceContainsString calls that were previously
	// called 75M+ times, causing significant performance overhead.
	atmosYamlTagsMap = map[string]bool{
		AtmosYamlFuncExec:                    true,
		AtmosYamlFuncStore:                   true,
		AtmosYamlFuncStoreGet:                true,
		AtmosYamlFuncTemplate:                true,
		AtmosYamlFuncTerraformOutput:         true,
		AtmosYamlFuncTerraformState:          true,
		AtmosYamlFuncEnv:                     true,
		AtmosYamlFuncGitRoot:                 true,
		AtmosYamlFuncGitRootAlias:            true,
		AtmosYamlFuncGitSha:                  true,
		AtmosYamlFuncGitBranch:               true,
		AtmosYamlFuncGitRef:                  true,
		AtmosYamlFuncGitRepository:           true,
		AtmosYamlFuncGitOwner:                true,
		AtmosYamlFuncGitName:                 true,
		AtmosYamlFuncGitHost:                 true,
		AtmosYamlFuncGitUrl:                  true,
		AtmosYamlFuncCwd:                     true,
		AtmosYamlFuncRandom:                  true,
		AtmosYamlFuncLiteral:                 true,
		AtmosYamlFuncAwsAccountID:            true,
		AtmosYamlFuncAwsCallerIdentityArn:    true,
		AtmosYamlFuncAwsCallerIdentityUserID: true,
		AtmosYamlFuncAwsRegion:               true,
		AtmosYamlFuncAwsOrganizationID:       true,
	}

	// ParsedYAML cache stores parsed yaml.Node objects and their position
	// information to avoid re-parsing the same files multiple times. Cache key
	// is file path + content hash. Values are *parsedYAMLCacheEntry and are
	// treated as immutable post-insert; readers receive deep copies (the
	// underlying yaml.Node tree has pointer-sharing aliases that would
	// otherwise corrupt the cache if mutated).
	//
	// Uses sync.Map (not a RWMutex-protected map) because the write path is
	// highly contended at scale: in a large-stack workload (~22k
	// UnmarshalYAMLFromFileWithPositions calls), the prior RWMutex.Lock()
	// inside cacheParsedYAML serialized every write across goroutines while
	// holding the lock during the expensive deepCopyYAMLNode work. The
	// lock-free sync.Map removes the global lock and optimizes for the
	// disjoint-key write pattern this cache exhibits.
	parsedYAMLCache sync.Map // map[string]*parsedYAMLCacheEntry (immutable post-insert).

	// DecodedYAMLCache stores the post-Decode + post-Intern result of
	// UnmarshalYAMLFromFileWithPositions[map[string]any] so that repeat callers
	// (transitively imported files in describe-affected, ~22k calls in the
	// reference workload) skip the per-call yaml.Node.Decode and
	// InternStringsInMap walks. The parsedYAMLCache above only avoids re-parsing;
	// Decode + Intern still ran on every call and accounted for ~500-700µs per
	// invocation, dominating the function's cost once the parsed-node cache hit.
	//
	// Cache key: file path + content hash (same as parsedYAMLCache). Values are
	// *decodedYAMLCacheEntry with both the decoded map and its positions; readers
	// receive deep copies (DeepCopyMap + clonePositions) to preserve the
	// immutable-post-insert contract. Only the map[string]any generic
	// instantiation populates this cache; other generic T fall through to the
	// existing Decode path.
	decodedYAMLCache sync.Map // map[string]*decodedYAMLCacheEntry (immutable post-insert).

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

// decodedYAMLCacheEntry stores the post-Decode + post-Intern result for the
// map[string]any specialization of UnmarshalYAMLFromFileWithPositions, plus
// the matching position map. Both are deep-copied on retrieval to keep the
// cached value immutable.
type decodedYAMLCacheEntry struct {
	data      map[string]any
	positions PositionMap
}

// deepCopyDecodedMap is a local deep-copy for the decoded-YAML cache. It
// covers the value types produced by yaml.Node.Decode + InternStringsInMap:
// nested map[string]any, []any, and immutable primitives (string, numbers,
// bool). Atmos custom tags are pre-processed into strings before this point,
// so no exotic types appear. We can't use pkg/merge.DeepCopyMap here because
// pkg/merge imports pkg/utils.
func deepCopyDecodedMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyDecodedValue(v)
	}
	return out
}

func deepCopyDecodedValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[k] = deepCopyDecodedValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = deepCopyDecodedValue(vv)
		}
		return out
	default:
		// Primitives (string, int*, uint*, float*, bool) and nil are
		// immutable in Go's value model — sharing the reference is safe.
		return val
	}
}

// getCachedDecodedYAML retrieves a cached decoded+interned result for the
// given file and content. Returns deep copies so the cache remains immutable
// across concurrent readers. The sync.Map.Load is non-blocking; the deep copy
// runs outside any critical section.
func getCachedDecodedYAML(file, content string) (map[string]any, PositionMap, bool) {
	cacheKey := generateParsedYAMLCacheKey(file, content)
	if cacheKey == "" {
		return nil, nil, false
	}
	raw, found := decodedYAMLCache.Load(cacheKey)
	if !found {
		return nil, nil, false
	}
	entry, ok := raw.(*decodedYAMLCacheEntry)
	if !ok {
		return nil, nil, false
	}
	dataCopy := deepCopyDecodedMap(entry.data)
	return dataCopy, clonePositions(entry.positions), true
}

// cacheDecodedYAML stores a decoded+interned result. The deep copy runs
// BEFORE the sync.Map.Store so the cache's critical section is only the
// atomic store, not the expensive recursive map copy.
func cacheDecodedYAML(file, content string, data map[string]any, positions PositionMap) {
	cacheKey := generateParsedYAMLCacheKey(file, content)
	if cacheKey == "" || data == nil {
		return
	}
	dataCopy := deepCopyDecodedMap(data)
	decodedYAMLCache.Store(cacheKey, &decodedYAMLCacheEntry{
		data:      dataCopy,
		positions: clonePositions(positions),
	})
}

// clearDecodedYAMLCache empties the decoded YAML cache. Exported indirectly
// via ClearDecodedYAMLCache for tests that need to reset state between subtests.
func clearDecodedYAMLCache() {
	decodedYAMLCache.Range(func(key, _ any) bool {
		decodedYAMLCache.Delete(key)
		return true
	})
}

// ClearDecodedYAMLCache clears the decoded YAML result cache. Call between
// independent operations (like tests) to ensure fresh processing of files
// whose on-disk content may have changed.
func ClearDecodedYAMLCache() {
	clearDecodedYAMLCache()
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

// deepCopyYAMLNode recursively deep-copies a yaml.Node tree.
// This is required because copying a yaml.Node struct only copies the slice header,
// leaving Content and Alias fields aliased to the original.
// Without deep copying, mutations to cached nodes would affect all consumers.
func deepCopyYAMLNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}

	// Copy the struct fields.
	cp := *n

	// Deep copy the Content slice.
	if n.Content != nil {
		cp.Content = make([]*yaml.Node, len(n.Content))
		for i, c := range n.Content {
			cp.Content[i] = deepCopyYAMLNode(c)
		}
	}

	// Deep copy the Alias pointer.
	if n.Alias != nil {
		cp.Alias = deepCopyYAMLNode(n.Alias)
	}

	return &cp
}

// clonePositions creates a copy of a PositionMap to prevent aliasing.
// This is required because maps are reference types in Go - returning or storing
// the same map reference would allow mutations to affect the cache and other consumers.
func clonePositions(positions PositionMap) PositionMap {
	if positions == nil {
		return nil
	}

	// Create new map with same capacity.
	clone := make(PositionMap, len(positions))
	for k, v := range positions {
		// Position is a simple struct with int fields, so value copy is sufficient.
		clone[k] = v
	}
	return clone
}

// getCachedParsedYAML retrieves a cached parsed YAML node if it exists.
// Returns a copy of the node and positions to prevent external mutations.
//
// The sync.Map.Load is non-blocking and the deep-copy of the cached entry runs
// outside any critical section: cached values are treated as immutable
// post-insert (see cacheParsedYAML), so concurrent goroutines may safely
// deep-copy them without coordination.
//
// Note: Statistics tracking is done by the caller to avoid double-counting.
// Note: perf.Track() removed from this hot path to reduce overhead.
func getCachedParsedYAML(file string, content string) (*yaml.Node, PositionMap, bool) {
	cacheKey := generateParsedYAMLCacheKey(file, content)
	if cacheKey == "" {
		return nil, nil, false
	}

	raw, found := parsedYAMLCache.Load(cacheKey)
	if !found {
		return nil, nil, false
	}
	entry, ok := raw.(*parsedYAMLCacheEntry)
	if !ok {
		return nil, nil, false
	}

	// Return copies to prevent mutations affecting the cache.
	nodeCopy := deepCopyYAMLNode(&entry.node)
	positionsCopy := clonePositions(entry.positions)
	return nodeCopy, positionsCopy, true
}

// cacheParsedYAML stores a parsed YAML node in the cache.
// Stores copies to prevent external mutations from affecting the cache.
//
// The deep copy of node + positions runs BEFORE the sync.Map.Store so the
// cache's critical section is only the atomic store, not the expensive
// recursive yaml.Node copy. Combined with sync.Map's lock-free read path,
// this removes the global write-lock that previously serialized every cache
// write across the ~22k UnmarshalYAMLFromFileWithPositions calls produced by
// a large-stack describe-affected run.
//
// Note: perf.Track() removed from this hot path to reduce overhead.
func cacheParsedYAML(file string, content string, node *yaml.Node, positions PositionMap) {
	cacheKey := generateParsedYAMLCacheKey(file, content)
	if cacheKey == "" || node == nil {
		return
	}

	// Store copies to prevent external mutations from affecting the cache.
	nodeCopy := deepCopyYAMLNode(node)
	positionsCopy := clonePositions(positions)
	parsedYAMLCache.Store(cacheKey, &parsedYAMLCacheEntry{
		node:      *nodeCopy,
		positions: positionsCopy,
	})
}

// clearParsedYAMLCache empties the parsed YAML cache. Exported indirectly via
// ClearParsedYAMLCache for tests that need to reset state between subtests.
func clearParsedYAMLCache() {
	parsedYAMLCache.Range(func(key, _ any) bool {
		parsedYAMLCache.Delete(key)
		return true
	})
}

// ClearParsedYAMLCache clears the parsed YAML cache.
// This should be called between independent operations (like tests) to ensure
// fresh processing of files whose on-disk content may have changed.
func ClearParsedYAMLCache() {
	clearParsedYAMLCache()
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
	if atmosConfig != nil && atmosConfig.TrackProvenance {
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
		// Check if we need positions but the cached entry lacks them.
		// This can happen if the file was first parsed without provenance tracking.
		needsPositions := atmosConfig != nil && atmosConfig.TrackProvenance && len(positions) == 0
		if !needsPositions {
			// Another goroutine cached it while we waited - valid cache hit!
			parsedYAMLCacheStats.Lock()
			parsedYAMLCacheStats.hits++
			parsedYAMLCacheStats.Unlock()
			return node, positions, nil
		}
		// Fall through to re-parse with position tracking.
	}

	// Still not in cache (or needs re-parsing for positions).
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

	var callsPerFile, callsPerHash float64
	if uniqueFiles > 0 {
		callsPerFile = float64(totalCalls) / float64(uniqueFiles)
	}
	if uniqueHashes > 0 {
		callsPerHash = float64(totalCalls) / float64(uniqueHashes)
	}

	log.Info(
		"YAML Cache Statistics",
		"totalCalls", totalCalls,
		"cacheHits", hits,
		"cacheMisses", misses,
		"hitRate", hitRate,
		"uniqueFiles", uniqueFiles,
		"uniqueHashes", uniqueHashes,
		"callsPerFile", callsPerFile,
		"callsPerHash", callsPerHash,
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
	sort.Slice(fileCounts, func(i, j int) bool {
		return fileCounts[i].count > fileCounts[j].count
	})

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

	// Log the YAML data.
	// Note: This logs multiline YAML which will be formatted by the logger.
	// For large data structures, this may produce verbose output.
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

// GetUserHomeDir returns the current user's home directory or empty string if unavailable.
func GetUserHomeDir() string {
	defer perf.Track(nil, "utils.GetUserHomeDir")()

	hd, err := homedir.Dir()
	if err != nil {
		return ""
	}
	return hd
}

// ObfuscateSensitivePaths walks any data structure (maps, slices, etc), and in any string which starts with the specified homeDir, replaces it with "~".
func ObfuscateSensitivePaths(data any, homeDir string) any {
	defer perf.Track(nil, "utils.ObfuscateSensitivePaths")()

	switch v := data.(type) {
	case map[string]any:
		res := make(map[string]any, len(v))
		for k, val := range v {
			res[k] = ObfuscateSensitivePaths(val, homeDir)
		}
		return res
	case []any:
		res := make([]any, len(v))
		for i, val := range v {
			res[i] = ObfuscateSensitivePaths(val, homeDir)
		}
		return res
	case string:
		if homeDir != "" && strings.HasPrefix(v, homeDir) {
			return "~" + v[len(homeDir):]
		}
		return v
	default:
		return v
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

// processCustomTags walks a YAML node tree and processes any custom Atmos
// tags it contains. The entry point performs the fast hasCustomTags scan
// ONCE; if the tree contains no custom tags anywhere, it returns immediately.
// If any custom tag is found, processCustomTagsInner does the actual walk
// without re-checking subtrees — saving O(depth) redundant tree scans that
// the prior implementation incurred by re-running hasCustomTags on every
// recursive call.
//
// Background: in a large describe-affected run (~9k processCustomTags
// invocations) the redundant per-recursion hasCustomTags walks accounted
// for the bulk of the 31s cumulative CPU time. Hoisting the check to the
// top eliminates that overhead without changing behavior for tag-free
// subtrees (which still benefit from the early exit).
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

	// We've established there IS a custom tag somewhere in this subtree;
	// walk it once via the inner helper which skips the (now redundant)
	// hasCustomTags check on every recursion.
	return processCustomTagsInner(atmosConfig, node, file)
}

// processCustomTagsInner is the recursive worker for processCustomTags.
// Callers must have already established that the input tree contains at
// least one custom tag (via hasCustomTags); this function does not perform
// that check on each call, which is the key optimization vs the prior
// implementation. The perf.Track on the outer processCustomTags wraps the
// entire walk with one tracked frame, so per-recursion tracking is
// intentionally omitted here to avoid inflating the metric (recursive
// calls would be counted in addition to the top-level invocation).
//
//nolint:gocognit
func processCustomTagsInner(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, file string) error {
	for _, n := range node.Content {
		tag := strings.TrimSpace(n.Tag)
		val := strings.TrimSpace(n.Value)

		// Handle !literal tag - preserve value exactly as-is, bypass all template processing.
		// This is processed early (like !include) so the value is never sent through
		// Go template or Gomplate evaluation.
		if tag == AtmosYamlFuncLiteral {
			// Just clear the tag and keep the value unchanged.
			// The value will pass through without any template processing.
			n.Tag = ""
			continue
		}

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
			if err := processCustomTagsInner(atmosConfig, n, file); err != nil {
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

	// Fast path: when callers want map[string]any (the production hot path
	// — schema.AtmosSectionMapType is `map[string]any`), try the post-Decode
	// + post-Intern cache first. A hit lets us skip yaml.Node.Decode and
	// InternStringsInMap, which together cost ~500-700µs per call and dominate
	// the function once the parsedYAMLCache (yaml.Node) is hot. Provenance
	// requires positions, which the decoded cache stores alongside the data.
	if _, ok := any(zeroValue).(map[string]any); ok {
		if cachedMap, cachedPositions, found := getCachedDecodedYAML(file, input); found {
			// If the caller needs positions but the cached entry has none
			// (entry inserted by a non-provenance caller), fall through to
			// re-decode via the slow path which will repopulate positions.
			if !atmosConfig.TrackProvenance || len(cachedPositions) > 0 {
				// Count decoded-cache hits in the same counter as parsed-node
				// cache hits; both represent successful avoidance of work.
				parsedYAMLCacheStats.Lock()
				parsedYAMLCacheStats.hits++
				parsedYAMLCacheStats.Unlock()
				// Safe by construction: zeroValue's type proves T == map[string]any.
				typed, _ := any(cachedMap).(T)
				return typed, cachedPositions, nil
			}
		}
	}

	// Try to get cached parsed YAML first (fast path with read lock).
	node, positions, found := getCachedParsedYAML(file, input)
	if found {
		// Cache hit - but check if we need positions and don't have them.
		// This can happen if the file was first parsed without provenance tracking,
		// then later requested with provenance enabled.
		if atmosConfig.TrackProvenance && len(positions) == 0 {
			// Need to re-parse with position tracking.
			// Force a cache miss to re-parse and update the cache with positions.
			found = false
		} else {
			// Valid cache hit.
			parsedYAMLCacheStats.Lock()
			parsedYAMLCacheStats.hits++
			parsedYAMLCacheStats.Unlock()
		}
	}

	if !found {
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

		// Populate the decoded-result cache so subsequent calls for the same
		// (file, content) skip the Decode + Intern walks. Storing the
		// post-Intern value lets future hits return ready-to-use maps.
		if internedMap, ok := interned.(map[string]any); ok {
			cacheDecodedYAML(file, input, internedMap, positions)
		}
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
		// Intern string values.
		return Intern(atmosConfig, v)

	default:
		// For all other types (int, bool, float, etc.), return as-is.
		return data
	}
}

//nolint:revive // File length justified by cohesive YAML processing functionality and recent critical bug fixes.
