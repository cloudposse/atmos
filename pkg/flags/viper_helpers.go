package flags

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// bindFlagToViper binds a single flag to Viper with environment variable support.
// This is shared helper code used by flag parsers.
func bindFlagToViper(v *viper.Viper, viperKey string, flag Flag) error {
	defer perf.Track(nil, "flags.bindFlagToViper")()

	// Set default value in Viper so it's returned when flag is not explicitly set.
	// This ensures defaults work correctly for CLI flags, ENV vars, and config files.
	v.SetDefault(viperKey, flag.GetDefault())

	// Special handling for flags with NoOptDefVal (identity pattern)
	if flag.GetNoOptDefVal() != "" {
		envVars := flag.GetEnvVars()
		if len(envVars) > 0 {
			args := make([]string, 0, len(envVars)+1)
			args = append(args, viperKey)
			args = append(args, envVars...)
			if err := v.BindEnv(args...); err != nil {
				return fmt.Errorf("failed to bind env vars for flag %s: %w", flag.GetName(), err)
			}
		}
		return nil
	}

	// Bind environment variables
	envVars := flag.GetEnvVars()
	if len(envVars) > 0 {
		args := make([]string, 0, len(envVars)+1)
		args = append(args, viperKey)
		args = append(args, envVars...)
		if err := v.BindEnv(args...); err != nil {
			return fmt.Errorf("failed to bind env vars for flag %s: %w", flag.GetName(), err)
		}
	}

	return nil
}

// ParseStringMap retrieves a string map from Viper, handling both
// CLI flags ([]string of "key=value") and env vars (comma-separated).
//
// This is used with StringMapFlag to convert the flag values into a map.
//
// Example CLI: --set foo=bar --set baz=qux
// Example ENV: ATMOS_SET=foo=bar,baz=qux
//
// Returns: map[string]string{"foo": "bar", "baz": "qux"}
//
// Key validation: Keys must be non-empty after trimming whitespace.
// Malformed pairs (missing "=" or empty key) are silently skipped.
func ParseStringMap(v *viper.Viper, key string) map[string]string {
	defer perf.Track(nil, "flags.ParseStringMap")()

	result := make(map[string]string)

	// Viper returns different types depending on source:
	// - CLI flags: []string (from StringSlice)
	// - Env vars: string (comma-separated)
	// - Config: could be map[string]interface{} or []string

	value := v.Get(key)
	if value == nil {
		return result
	}

	switch val := value.(type) {
	case []string:
		// From CLI flags: ["key1=val1", "key2=val2"].
		addStringSlicePairs(result, val)
	case string:
		// From env var: "key1=val1,key2=val2".
		addCommaSeparatedPairs(result, val)
	case map[string]string:
		// From defaults set via viper.SetDefault: {foo: bar, baz: qux}.
		addStringMapPairs(result, val)
	case map[string]interface{}:
		// From config file: {foo: bar, baz: qux}.
		addAnyMapPairs(result, val)
	case []interface{}:
		// From config file as array: ["foo=bar", "baz=qux"].
		addAnySlicePairs(result, val)
	}

	return result
}

// addStringSlicePairs parses CLI-flag entries ("key=val") into result, skipping malformed pairs.
func addStringSlicePairs(result map[string]string, pairs []string) {
	for _, pair := range pairs {
		k, v := parseKeyValuePair(pair)
		if k != "" {
			result[k] = v
		}
	}
}

// addCommaSeparatedPairs parses a comma-separated env-var string ("key=val,key2=val2") into result.
func addCommaSeparatedPairs(result map[string]string, s string) {
	for _, pair := range strings.Split(s, ",") {
		k, v := parseKeyValuePair(strings.TrimSpace(pair))
		if k != "" {
			result[k] = v
		}
	}
}

// addStringMapPairs copies a string-keyed string map into result, skipping
// keys that are empty or whitespace-only after trimming.
func addStringMapPairs(result map[string]string, pairs map[string]string) {
	for k, v := range pairs {
		if strings.TrimSpace(k) != "" {
			result[strings.TrimSpace(k)] = v
		}
	}
}

// addAnyMapPairs copies a string-keyed interface map into result, formatting
// each value with %v and skipping keys that are empty or whitespace-only after
// trimming.
func addAnyMapPairs(result map[string]string, pairs map[string]interface{}) {
	for k, v := range pairs {
		if strings.TrimSpace(k) != "" {
			result[strings.TrimSpace(k)] = fmt.Sprintf("%v", v)
		}
	}
}

// addAnySlicePairs parses config-file array entries ("key=val") into result, skipping non-strings.
func addAnySlicePairs(result map[string]string, items []interface{}) {
	for _, item := range items {
		str, ok := item.(string)
		if !ok {
			continue
		}
		k, v := parseKeyValuePair(str)
		if k != "" {
			result[k] = v
		}
	}
}

// parseKeyValuePair splits "key=value" into (key, value).
// Returns ("", "") if format is invalid (missing "=" or empty key).
// Keys and values are trimmed of surrounding whitespace.
func parseKeyValuePair(pair string) (string, string) {
	defer perf.Track(nil, "flags.parseKeyValuePair")()

	parts := strings.SplitN(pair, "=", 2)
	if len(parts) != 2 {
		return "", ""
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	// Validate key is non-empty.
	if key == "" {
		return "", ""
	}

	return key, value
}
