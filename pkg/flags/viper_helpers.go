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
		// From CLI flags: ["key1=val1", "key2=val2"]
		for _, pair := range val {
			k, v := parseKeyValuePair(pair)
			if k != "" {
				result[k] = v
			}
		}
	case string:
		// From env var: "key1=val1,key2=val2"
		pairs := strings.Split(val, ",")
		for _, pair := range pairs {
			k, v := parseKeyValuePair(strings.TrimSpace(pair))
			if k != "" {
				result[k] = v
			}
		}
	case map[string]interface{}:
		// From config file: {foo: bar, baz: qux}
		for k, v := range val {
			result[k] = fmt.Sprintf("%v", v)
		}
	case []interface{}:
		// From config file as array: ["foo=bar", "baz=qux"]
		for _, item := range val {
			if str, ok := item.(string); ok {
				k, v := parseKeyValuePair(str)
				if k != "" {
					result[k] = v
				}
			}
		}
	}

	return result
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
