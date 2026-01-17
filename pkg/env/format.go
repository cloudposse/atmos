// Package env provides unified environment variable formatting across multiple output formats.
// It supports bash, dotenv, env, and github formats with consistent escaping and heredoc handling.
package env

import (
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Format represents an environment variable output format.
type Format string

const (
	// FormatEnv outputs key=value pairs without quoting.
	FormatEnv Format = "env"
	// FormatDotenv outputs key='value' pairs with single-quote escaping.
	FormatDotenv Format = "dotenv"
	// FormatBash outputs export key='value' statements with single-quote escaping.
	FormatBash Format = "bash"
	// FormatGitHub outputs key=value or heredoc syntax for multiline values.
	// Used for $GITHUB_OUTPUT and $GITHUB_ENV in GitHub Actions.
	FormatGitHub Format = "github"
)

// SupportedFormats lists all supported environment variable output formats.
var SupportedFormats = []Format{FormatEnv, FormatDotenv, FormatBash, FormatGitHub}

// ParseFormat converts a format string to a Format type.
// Returns an error for unsupported format strings.
func ParseFormat(s string) (Format, error) {
	defer perf.Track(nil, "env.ParseFormat")()

	switch s {
	case "env":
		return FormatEnv, nil
	case "dotenv":
		return FormatDotenv, nil
	case "bash":
		return FormatBash, nil
	case "github":
		return FormatGitHub, nil
	default:
		return "", errUtils.Build(errUtils.ErrInvalidFormat).
			WithExplanationf("unsupported format: %s", s).
			Err()
	}
}

// FormatData formats key-value data in the specified format.
// Complex values (maps, slices) are JSON-encoded.
// Keys are sorted alphabetically for consistent output.
func FormatData(data map[string]any, format Format, opts ...Option) (string, error) {
	defer perf.Track(nil, "env.FormatData")()

	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Apply transformations to data.
	transformed := transformData(data, cfg)

	// Get sorted keys for consistent output.
	keys := sortedKeys(transformed)

	var sb strings.Builder
	for _, key := range keys {
		value := transformed[key]
		if value == nil {
			continue
		}

		line, err := formatSingleValue(key, value, format, cfg)
		if err != nil {
			return "", err
		}
		sb.WriteString(line)
	}

	return sb.String(), nil
}

// FormatValue formats a single key-value pair in the specified format.
func FormatValue(key string, value any, format Format, opts ...Option) (string, error) {
	defer perf.Track(nil, "env.FormatValue")()

	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Apply key transformation.
	if cfg.uppercase {
		key = strings.ToUpper(key)
	}

	return formatSingleValue(key, value, format, cfg)
}

// formatSingleValue formats a single key-value pair without applying options.
func formatSingleValue(key string, value any, format Format, cfg *config) (string, error) {
	strValue := ValueToString(value)

	switch format {
	case FormatEnv:
		return formatEnvValue(key, strValue), nil
	case FormatDotenv:
		return formatDotenvValue(key, strValue), nil
	case FormatBash:
		return formatBashValue(key, strValue, cfg), nil
	case FormatGitHub:
		return formatGitHubValue(key, strValue), nil
	default:
		return "", errUtils.Build(errUtils.ErrInvalidFormat).
			WithExplanationf("unsupported format: %s", format).
			Err()
	}
}

// transformData applies configuration options to transform the data.
func transformData(data map[string]any, cfg *config) map[string]any {
	result := make(map[string]any, len(data))

	for k, v := range data {
		key := k
		if cfg.uppercase {
			key = strings.ToUpper(k)
		}

		// Handle flattening of nested maps.
		if cfg.flatten && cfg.flattenSeparator != "" {
			if nested, ok := v.(map[string]any); ok {
				flattenMap(result, key, nested, cfg.flattenSeparator, cfg.uppercase)
				continue
			}
		}

		result[key] = v
	}

	return result
}

// flattenMap recursively flattens nested maps into the result map.
func flattenMap(result map[string]any, prefix string, data map[string]any, separator string, uppercase bool) {
	for k, v := range data {
		key := k
		if uppercase {
			key = strings.ToUpper(k)
		}
		fullKey := prefix + separator + key

		if nested, ok := v.(map[string]any); ok {
			flattenMap(result, fullKey, nested, separator, uppercase)
		} else {
			result[fullKey] = v
		}
	}
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
