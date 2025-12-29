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
		return &schema.VendorComponentSource{
			Uri: sourceStr,
		}, nil
	}

	// Handle map form: full VendorComponentSource.
	if sourceMap, ok := source.(map[string]any); ok {
		return parseSourceMap(sourceMap, location)
	}

	return nil, errUtils.Build(errUtils.ErrSourceInvalidSpec).
		WithExplanation(fmt.Sprintf("%s must be a string (go-getter URI) or map (vendor spec)", location)).
		WithContext("type", fmt.Sprintf("%T", source)).
		Err()
}

// parseSourceMap parses a map into VendorComponentSource.
func parseSourceMap(sourceMap map[string]any, location string) (*schema.VendorComponentSource, error) {
	spec := &schema.VendorComponentSource{}

	// Required: uri.
	uri, ok := sourceMap["uri"].(string)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrSourceInvalidSpec).
			WithExplanation(fmt.Sprintf("%s.uri is required", location)).
			WithHint("Specify a valid go-getter URI").
			Err()
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

// parseDuration parses a duration string from a map, returning zero duration if not found or invalid.
func parseDuration(m map[string]any, key string) time.Duration {
	if v, ok := m[key].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		} else {
			_ = ui.Warningf("invalid duration for %s: %q", key, v)
		}
	}
	return 0
}

// parseRetryConfig parses a map into RetryConfig.
func parseRetryConfig(m map[string]any) *schema.RetryConfig {
	cfg := &schema.RetryConfig{
		InitialDelay:   parseDuration(m, "initial_delay"),
		MaxDelay:       parseDuration(m, "max_delay"),
		MaxElapsedTime: parseDuration(m, "max_elapsed_time"),
	}

	if v, ok := m["max_attempts"].(int); ok {
		cfg.MaxAttempts = v
	}
	if v, ok := m["backoff_strategy"].(string); ok {
		cfg.BackoffStrategy = schema.BackoffStrategy(v)
	}
	if v, ok := m["random_jitter"].(float64); ok {
		cfg.RandomJitter = v
	}
	if v, ok := m["multiplier"].(float64); ok {
		cfg.Multiplier = v
	}

	return cfg
}

// HasSource checks if component config has source defined.
func HasSource(componentConfig map[string]any) bool {
	defer perf.Track(nil, "source.HasSource")()
	if componentConfig == nil {
		return false
	}
	// Check top-level source.
	if source, ok := componentConfig[cfg.SourceSectionName]; ok && source != nil {
		return true
	}
	return false
}
