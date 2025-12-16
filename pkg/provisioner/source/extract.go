package source

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExtractMetadataSource extracts the source specification from component config.
// It supports both string form (go-getter URI) and map form (VendorComponentSource).
// Returns nil, nil if no source is configured (not an error).
func ExtractMetadataSource(componentConfig map[string]any) (*schema.VendorComponentSource, error) {
	defer perf.Track(nil, "source.ExtractMetadataSource")()
	if componentConfig == nil {
		return nil, nil
	}

	metadata, ok := componentConfig["metadata"].(map[string]any)
	if !ok {
		return nil, nil // No metadata section.
	}

	source, ok := metadata["source"]
	if !ok || source == nil {
		return nil, nil // No source configured.
	}

	// Handle string form: "github.com/org/repo//path?ref=v1.0.0".
	if sourceStr, ok := source.(string); ok {
		return &schema.VendorComponentSource{
			Uri: sourceStr,
		}, nil
	}

	// Handle map form: full VendorComponentSource.
	if sourceMap, ok := source.(map[string]any); ok {
		return parseSourceMap(sourceMap)
	}

	return nil, errUtils.Build(errUtils.ErrSourceInvalidSpec).
		WithExplanation("metadata.source must be a string (go-getter URI) or map (vendor spec)").
		WithContext("type", fmt.Sprintf("%T", source)).
		Err()
}

// parseSourceMap parses a map into VendorComponentSource.
func parseSourceMap(sourceMap map[string]any) (*schema.VendorComponentSource, error) {
	spec := &schema.VendorComponentSource{}

	// Required: uri.
	uri, ok := sourceMap["uri"].(string)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrSourceInvalidSpec).
			WithExplanation("metadata.source.uri is required").
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

// HasMetadataSource checks if component config has metadata.source defined.
func HasMetadataSource(componentConfig map[string]any) bool {
	defer perf.Track(nil, "source.HasMetadataSource")()
	if componentConfig == nil {
		return false
	}
	metadata, ok := componentConfig["metadata"].(map[string]any)
	if !ok {
		return false
	}
	source, ok := metadata["source"]
	return ok && source != nil
}
