package source

import (
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ExtractSource extracts the source specification from component config.
// Supports both string form (go-getter URI) and map form (VendorComponentSource).
// Returns nil, nil if no source is configured (not an error).
func ExtractSource(componentConfig map[string]any) (*schema.VendorComponentSource, error) {
	defer perf.Track(nil, "source.ExtractSource")()
	if componentConfig == nil {
		return nil, nil
	}

	// Check top-level source.
	if source, ok := componentConfig[cfg.SourceSectionName]; ok && source != nil {
		return parseSource(source, cfg.SourceSectionName)
	}

	return nil, nil // No source configured.
}

// parseSource parses source from either string or map form.
func parseSource(source any, location string) (*schema.VendorComponentSource, error) {
	// Handle string form: "github.com/org/repo//path?ref=v1.0.0".
	if sourceStr, ok := source.(string); ok {
		// Empty string means no source configured.
		if sourceStr == "" {
			return nil, nil
		}
		return &schema.VendorComponentSource{
			Uri: sourceStr,
		}, nil
	}

	// Handle map form: full VendorComponentSource.
	if sourceMap, ok := source.(map[string]any); ok {
		return parseSourceMap(sourceMap)
	}

	return nil, errUtils.Build(errUtils.ErrSourceInvalidSpec).
		WithExplanation(fmt.Sprintf("%s must be a string (go-getter URI) or map (vendor spec)", location)).
		WithContext("type", fmt.Sprintf("%T", source)).
		Err()
}

// parseSourceMap parses a map into VendorComponentSource.
func parseSourceMap(sourceMap map[string]any) (*schema.VendorComponentSource, error) {
	spec := &schema.VendorComponentSource{}

	// Required: uri. If empty or missing, treat as no source configured.
	uri, ok := sourceMap["uri"].(string)
	if !ok || uri == "" {
		// Empty source map or missing uri means no source configured.
		return nil, nil
	}
	spec.Uri = uri

	// Optional: type.
	if t, ok := sourceMap["type"].(string); ok {
		spec.Type = t
	}

	// Optional: version.
	if v, ok := sourceMap["version"].(string); ok {
		spec.Version = v
	}

	// Optional: included_paths.
	if paths, ok := sourceMap["included_paths"].([]any); ok {
		spec.IncludedPaths = toStringSlice(paths)
	}

	// Optional: excluded_paths.
	if paths, ok := sourceMap["excluded_paths"].([]any); ok {
		spec.ExcludedPaths = toStringSlice(paths)
	}

	// Optional: retry.
	if retryMap, ok := sourceMap["retry"].(map[string]any); ok {
		spec.Retry = parseRetryConfig(retryMap)
	}

	return spec, nil
}

// toStringSlice converts []any to []string.
func toStringSlice(items []any) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// parseDurationPtr parses a duration string from a map, returning nil if not found or invalid.
func parseDurationPtr(m map[string]any, key string) *time.Duration {
	if v, ok := m[key].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return &d
		}
		ui.Warningf("invalid duration for %s: %q", key, v)
	}
	return nil
}

// parseIntPtr parses an int from a map (handles both int and float64), returning nil if not found.
func parseIntPtr(m map[string]any, key string) *int {
	switch v := m[key].(type) {
	case int:
		return &v
	case float64:
		i := int(v)
		return &i
	}
	return nil
}

// parseFloat64Ptr parses a float64 from a map, returning nil if not found.
func parseFloat64Ptr(m map[string]any, key string) *float64 {
	if v, ok := m[key].(float64); ok {
		return &v
	}
	return nil
}

// parseRetryConfig parses a map into RetryConfig.
// Fields that are not specified in the map will be nil (meaning "not set" / disabled / unlimited).
func parseRetryConfig(m map[string]any) *schema.RetryConfig {
	cfg := &schema.RetryConfig{
		MaxAttempts:    parseIntPtr(m, "max_attempts"),
		InitialDelay:   parseDurationPtr(m, "initial_delay"),
		MaxDelay:       parseDurationPtr(m, "max_delay"),
		MaxElapsedTime: parseDurationPtr(m, "max_elapsed_time"),
		RandomJitter:   parseFloat64Ptr(m, "random_jitter"),
		Multiplier:     parseFloat64Ptr(m, "multiplier"),
	}

	if v, ok := m["backoff_strategy"].(string); ok {
		cfg.BackoffStrategy = schema.BackoffStrategy(v)
	}

	return cfg
}

// HasSource checks if component config has a valid source defined.
// Returns true only if the source has a valid URI.
func HasSource(componentConfig map[string]any) bool {
	defer perf.Track(nil, "source.HasSource")()
	if componentConfig == nil {
		return false
	}
	// Check top-level source.
	source, ok := componentConfig[cfg.SourceSectionName]
	if !ok || source == nil {
		return false
	}

	// String form: must be non-empty.
	if sourceStr, ok := source.(string); ok {
		return sourceStr != ""
	}

	// Map form: must have uri field.
	if sourceMap, ok := source.(map[string]any); ok {
		uri, hasUri := sourceMap["uri"].(string)
		return hasUri && uri != ""
	}

	return false
}
