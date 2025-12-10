package hcl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/stack/loader"
)

// errWrap is the error wrapping format string.
const errWrap = "%w: %w"

// ErrInvalidType is returned when an unsupported type is encountered.
var ErrInvalidType = errors.New("unsupported type")

// ErrInvalidInputType is returned when the input data type is not a map.
var ErrInvalidInputType = errors.New("HCL encoder expects map[string]any")

// Loader implements the loader.StackLoader interface for HCL files.
type Loader struct {
	// cache stores parsed HCL content to avoid re-parsing.
	cache   map[string]*cacheEntry
	cacheMu sync.RWMutex

	// Per-key locks for double-checked locking.
	locks   map[string]*sync.Mutex
	locksMu sync.Mutex
}

// cacheEntry stores a parsed HCL document and its metadata.
type cacheEntry struct {
	data      any
	positions map[string]loader.Position
}

// New creates a new HCL loader.
func New() *Loader {
	defer perf.Track(nil, "hcl.New")()

	return &Loader{
		cache: make(map[string]*cacheEntry),
		locks: make(map[string]*sync.Mutex),
	}
}

// Name returns the loader name.
func (l *Loader) Name() string {
	defer perf.Track(nil, "hcl.Loader.Name")()

	return "HCL"
}

// Extensions returns the supported file extensions.
func (l *Loader) Extensions() []string {
	defer perf.Track(nil, "hcl.Loader.Extensions")()

	return []string{".hcl", ".tf"}
}

// Load parses HCL data and returns the result.
func (l *Loader) Load(ctx context.Context, data []byte, opts ...loader.LoadOption) (any, error) {
	defer perf.Track(nil, "hcl.Loader.Load")()

	result, _, err := l.LoadWithMetadata(ctx, data, opts...)
	return result, err
}

// LoadWithMetadata parses HCL data and returns the result with position metadata.
func (l *Loader) LoadWithMetadata(ctx context.Context, data []byte, opts ...loader.LoadOption) (any, *loader.Metadata, error) {
	defer perf.Track(nil, "hcl.Loader.LoadWithMetadata")()

	options := loader.ApplyLoadOptions(opts...)

	// Generate cache key from content hash.
	cacheKey := l.generateCacheKey(options.SourceFile, data)

	// Try cache first.
	if entry := l.getFromCache(cacheKey); entry != nil {
		return entry.data, l.buildMetadata(entry.positions, options.SourceFile), nil
	}

	// Parse with double-checked locking.
	result, positions, err := l.parseWithLocking(ctx, cacheKey, data, options.SourceFile)
	if err != nil {
		return nil, nil, err
	}

	return result, l.buildMetadata(positions, options.SourceFile), nil
}

// Encode converts data to HCL format.
func (l *Loader) Encode(ctx context.Context, data any, opts ...loader.EncodeOption) ([]byte, error) {
	defer perf.Track(nil, "hcl.Loader.Encode")()

	// Check context cancellation.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	// Convert the data to HCL.
	if err := l.writeValue(rootBody, data); err != nil {
		return nil, fmt.Errorf(errWrap, errUtils.ErrEncodeFailed, err)
	}

	return f.Bytes(), nil
}

// writeValue writes a Go value to an HCL body.
func (l *Loader) writeValue(body *hclwrite.Body, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: got %T", ErrInvalidInputType, data)
	}

	for key, value := range m {
		ctyVal, err := goToCty(value)
		if err != nil {
			return fmt.Errorf("failed to convert %q: %w", key, err)
		}
		body.SetAttributeValue(key, ctyVal)
	}

	return nil
}

// goToCty converts Go types to cty.Value.
func goToCty(value any) (cty.Value, error) {
	switch v := value.(type) {
	case nil:
		return cty.NullVal(cty.DynamicPseudoType), nil
	case bool:
		return cty.BoolVal(v), nil
	case string:
		return cty.StringVal(v), nil
	case int:
		return cty.NumberIntVal(int64(v)), nil
	case int64:
		return cty.NumberIntVal(v), nil
	case float64:
		return cty.NumberFloatVal(v), nil
	case []any:
		return sliceToCty(v)
	case map[string]any:
		return mapToCty(v)
	default:
		return cty.NilVal, fmt.Errorf("%w: %T", ErrInvalidType, value)
	}
}

// sliceToCty converts a Go slice to cty.Value.
func sliceToCty(v []any) (cty.Value, error) {
	if len(v) == 0 {
		return cty.ListValEmpty(cty.DynamicPseudoType), nil
	}
	vals := make([]cty.Value, len(v))
	for i, item := range v {
		val, err := goToCty(item)
		if err != nil {
			return cty.NilVal, err
		}
		vals[i] = val
	}
	return cty.TupleVal(vals), nil
}

// mapToCty converts a Go map to cty.Value.
func mapToCty(v map[string]any) (cty.Value, error) {
	if len(v) == 0 {
		return cty.EmptyObjectVal, nil
	}
	vals := make(map[string]cty.Value)
	for k, item := range v {
		val, err := goToCty(item)
		if err != nil {
			return cty.NilVal, err
		}
		vals[k] = val
	}
	return cty.ObjectVal(vals), nil
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

// parseWithLocking parses HCL content with double-checked locking.
func (l *Loader) parseWithLocking(ctx context.Context, cacheKey string, data []byte, filename string) (any, map[string]loader.Position, error) {
	if cacheKey == "" {
		return l.parse(ctx, data, filename)
	}

	mu := l.getLock(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	// Double-check cache after acquiring lock.
	if entry := l.getFromCache(cacheKey); entry != nil {
		return entry.data, entry.positions, nil
	}

	// Parse the data.
	result, positions, err := l.parse(ctx, data, filename)
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

// parse performs the actual HCL parsing.
func (l *Loader) parse(ctx context.Context, data []byte, filename string) (any, map[string]loader.Position, error) {
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

	// Parse HCL file.
	file, err := l.parseHCLFile(data, filename)
	if err != nil {
		return nil, nil, err
	}

	// Extract and convert attributes.
	return l.extractAttributes(file)
}

// parseHCLFile parses HCL data into an hcl.File.
func (l *Loader) parseHCLFile(data []byte, filename string) (*hcl.File, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(data, filename)
	if err := checkDiags(diags); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("%w: file parsing returned nil", errUtils.ErrLoaderParseFailed)
	}
	return file, nil
}

// extractAttributes extracts attributes from an HCL file body.
func (l *Loader) extractAttributes(file *hcl.File) (map[string]any, map[string]loader.Position, error) {
	attributes, diags := file.Body.JustAttributes()
	if err := checkDiags(diags); err != nil {
		return nil, nil, err
	}

	result := make(map[string]any)
	positions := make(map[string]loader.Position)

	for name, attr := range attributes {
		positions[name] = loader.Position{
			Line:   attr.Range.Start.Line,
			Column: attr.Range.Start.Column,
		}

		goValue, err := l.evaluateAttribute(attr, name, positions)
		if err != nil {
			return nil, nil, err
		}
		result[name] = goValue
	}

	return result, positions, nil
}

// evaluateAttribute evaluates an HCL attribute and converts it to Go types.
func (l *Loader) evaluateAttribute(attr *hcl.Attribute, name string, positions map[string]loader.Position) (any, error) {
	ctyValue, diags := attr.Expr.Value(nil)
	if diags != nil && diags.HasErrors() {
		return nil, fmt.Errorf("%w: attribute %q: %s", errUtils.ErrLoaderParseFailed, name, diags.Error())
	}
	return ctyToGo(ctyValue, name, positions)
}

// checkDiags checks HCL diagnostics and returns an error if there are any errors.
func checkDiags(diags hcl.Diagnostics) error {
	if diags != nil && diags.HasErrors() {
		return fmt.Errorf("%w: %s", errUtils.ErrLoaderParseFailed, diags.Error())
	}
	return nil
}

// ctyToGo converts cty.Value to Go types and extracts positions.
func ctyToGo(value cty.Value, path string, positions map[string]loader.Position) (any, error) {
	if value.IsNull() {
		return nil, nil
	}

	// Handle primitive types.
	if result, handled := ctyPrimitiveToGo(value); handled {
		return result, nil
	}

	// Handle collection types.
	return ctyCollectionToGo(value, path, positions)
}

// ctyPrimitiveToGo converts primitive cty types to Go types.
// Returns the converted value and whether the type was handled.
func ctyPrimitiveToGo(value cty.Value) (any, bool) {
	switch {
	case value.Type() == cty.String:
		return value.AsString(), true
	case value.Type() == cty.Number:
		return ctyNumberToGo(value), true
	case value.Type() == cty.Bool:
		return value.True(), true
	default:
		return nil, false
	}
}

// ctyCollectionToGo converts collection cty types (maps, lists) to Go types.
func ctyCollectionToGo(value cty.Value, path string, positions map[string]loader.Position) (any, error) {
	switch {
	case value.Type().IsObjectType() || value.Type().IsMapType():
		return ctyMapToGo(value, path, positions)
	case value.Type().IsListType() || value.Type().IsTupleType() || value.Type().IsSetType():
		return ctyListToGo(value, path, positions)
	default:
		return value.GoString(), nil
	}
}

// ctyNumberToGo converts a cty.Number to Go int64 or float64.
func ctyNumberToGo(value cty.Value) any {
	bf := value.AsBigFloat()
	if bf.IsInt() {
		i, acc := bf.Int64()
		if acc == big.Exact {
			return i
		}
	}
	f, _ := bf.Float64()
	return f
}

// ctyMapToGo converts a cty object/map to Go map[string]any.
func ctyMapToGo(value cty.Value, path string, positions map[string]loader.Position) (map[string]any, error) {
	m := make(map[string]any)
	for k, v := range value.AsValueMap() {
		childPath := path + "." + k
		positions[childPath] = loader.Position{Line: 1, Column: 1}
		goVal, err := ctyToGo(v, childPath, positions)
		if err != nil {
			return nil, err
		}
		m[k] = goVal
	}
	return m, nil
}

// ctyListToGo converts a cty list/tuple/set to Go []any.
func ctyListToGo(value cty.Value, path string, positions map[string]loader.Position) ([]any, error) {
	var list []any
	for i, v := range value.AsValueSlice() {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		positions[childPath] = loader.Position{Line: 1, Column: 1}
		goVal, err := ctyToGo(v, childPath, positions)
		if err != nil {
			return nil, err
		}
		list = append(list, goVal)
	}
	return list, nil
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
	defer perf.Track(nil, "hcl.Loader.ClearCache")()

	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()

	l.cache = make(map[string]*cacheEntry)
}

// CacheStats returns cache statistics.
func (l *Loader) CacheStats() (entries int, keys []string) {
	defer perf.Track(nil, "hcl.Loader.CacheStats")()

	l.cacheMu.RLock()
	defer l.cacheMu.RUnlock()

	entries = len(l.cache)
	keys = make([]string, 0, entries)
	for k := range l.cache {
		keys = append(keys, k)
	}
	return entries, keys
}

// ParseBlocks parses HCL blocks (like Terraform resource blocks) in addition to attributes.
// This is useful for parsing Terraform-style configurations with blocks.
func (l *Loader) ParseBlocks(ctx context.Context, data []byte, filename string) (map[string]any, error) {
	defer perf.Track(nil, "hcl.Loader.ParseBlocks")()

	// Check context cancellation.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Handle empty input.
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	// Parse HCL file.
	file, err := l.parseHCLFile(data, filename)
	if err != nil {
		return nil, err
	}

	return l.extractPartialContent(file)
}

// extractPartialContent extracts attributes from HCL using partial content parsing.
// NOTE: Attribute evaluation errors are intentionally silently skipped to allow
// partial extraction of valid attributes. This is useful when some attributes
// reference undefined variables or functions - we extract what we can and continue.
// Debug logging could be added here if needed for troubleshooting, but would be
// noisy in normal operation since some attributes may legitimately fail evaluation
// until the full context is available.
func (l *Loader) extractPartialContent(file *hcl.File) (map[string]any, error) {
	content, _, diags := file.Body.PartialContent(&hcl.BodySchema{})
	if err := checkDiags(diags); err != nil {
		return nil, err
	}

	result := make(map[string]any)
	for name, attr := range content.Attributes {
		ctyValue, attrDiags := attr.Expr.Value(nil)
		if attrDiags != nil && attrDiags.HasErrors() {
			// Skip attributes that fail evaluation - they may reference undefined
			// variables or functions that will be resolved later in processing.
			continue
		}
		positions := make(map[string]loader.Position)
		goValue, err := ctyToGo(ctyValue, name, positions)
		if err != nil {
			// Skip attributes that fail type conversion.
			continue
		}
		result[name] = goValue
	}

	return result, nil
}
