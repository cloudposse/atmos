package flags

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestParseStringMap(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    interface{}
		expected map[string]string
	}{
		{
			name:     "nil value",
			key:      "set",
			value:    nil,
			expected: map[string]string{},
		},
		{
			name:  "string slice from CLI",
			key:   "set",
			value: []string{"foo=bar", "baz=qux"},
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:  "string slice with whitespace",
			key:   "set",
			value: []string{"  foo = bar  ", "baz=qux"},
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:  "string slice with invalid entries",
			key:   "set",
			value: []string{"foo=bar", "invalid", "baz=qux"},
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:  "string slice with empty key",
			key:   "set",
			value: []string{"foo=bar", "=value", "baz=qux"},
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:  "comma-separated string from env",
			key:   "set",
			value: "foo=bar,baz=qux",
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:  "comma-separated string with whitespace",
			key:   "set",
			value: "foo=bar , baz = qux",
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:  "comma-separated string with invalid entries",
			key:   "set",
			value: "foo=bar,invalid,baz=qux",
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name: "map from config file",
			key:  "set",
			value: map[string]interface{}{
				"foo": "bar",
				"baz": "qux",
			},
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name: "map with non-string values",
			key:  "set",
			value: map[string]interface{}{
				"count":   42,
				"enabled": true,
				"name":    "test",
			},
			expected: map[string]string{
				"count":   "42",
				"enabled": "true",
				"name":    "test",
			},
		},
		{
			name:  "interface slice from config",
			key:   "set",
			value: []interface{}{"foo=bar", "baz=qux"},
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:  "interface slice with mixed types",
			key:   "set",
			value: []interface{}{"foo=bar", 123, "baz=qux"},
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:     "empty string slice",
			key:      "set",
			value:    []string{},
			expected: map[string]string{},
		},
		{
			name:     "empty string",
			key:      "set",
			value:    "",
			expected: map[string]string{},
		},
		{
			name:     "empty map",
			key:      "set",
			value:    map[string]interface{}{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			if tt.value != nil {
				v.Set(tt.key, tt.value)
			}

			result := ParseStringMap(v, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseKeyValuePair(t *testing.T) {
	tests := []struct {
		name        string
		pair        string
		expectedKey string
		expectedVal string
	}{
		{
			name:        "simple pair",
			pair:        "foo=bar",
			expectedKey: "foo",
			expectedVal: "bar",
		},
		{
			name:        "pair with whitespace",
			pair:        "  foo = bar  ",
			expectedKey: "foo",
			expectedVal: "bar",
		},
		{
			name:        "pair with empty value",
			pair:        "foo=",
			expectedKey: "foo",
			expectedVal: "",
		},
		{
			name:        "pair with equals in value",
			pair:        "foo=bar=baz",
			expectedKey: "foo",
			expectedVal: "bar=baz",
		},
		{
			name:        "missing equals",
			pair:        "foobar",
			expectedKey: "",
			expectedVal: "",
		},
		{
			name:        "empty key",
			pair:        "=value",
			expectedKey: "",
			expectedVal: "",
		},
		{
			name:        "whitespace only key",
			pair:        "   =value",
			expectedKey: "",
			expectedVal: "",
		},
		{
			name:        "empty string",
			pair:        "",
			expectedKey: "",
			expectedVal: "",
		},
		{
			name:        "url value",
			pair:        "url=https://example.com/path?query=value",
			expectedKey: "url",
			expectedVal: "https://example.com/path?query=value",
		},
		{
			name:        "json value",
			pair:        "config={\"key\":\"value\"}",
			expectedKey: "config",
			expectedVal: "{\"key\":\"value\"}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, val := parseKeyValuePair(tt.pair)
			assert.Equal(t, tt.expectedKey, key)
			assert.Equal(t, tt.expectedVal, val)
		})
	}
}

func TestParseStringMap_RealWorldScenarios(t *testing.T) {
	t.Run("init command template values", func(t *testing.T) {
		v := viper.New()
		v.Set("set", []string{
			"project_name=my-app",
			"environment=production",
			"region=us-west-2",
		})

		result := ParseStringMap(v, "set")
		assert.Equal(t, map[string]string{
			"project_name": "my-app",
			"environment":  "production",
			"region":       "us-west-2",
		}, result)
	})

	t.Run("scaffold command with env var", func(t *testing.T) {
		v := viper.New()
		// Simulating ATMOS_SCAFFOLD_SET=component=vpc,namespace=core,stage=dev
		v.Set("set", "component=vpc,namespace=core,stage=dev")

		result := ParseStringMap(v, "set")
		assert.Equal(t, map[string]string{
			"component": "vpc",
			"namespace": "core",
			"stage":     "dev",
		}, result)
	})

	t.Run("config file defaults", func(t *testing.T) {
		v := viper.New()
		v.Set("set", map[string]interface{}{
			"default_region":      "us-east-1",
			"default_environment": "dev",
		})

		result := ParseStringMap(v, "set")
		assert.Equal(t, map[string]string{
			"default_region":      "us-east-1",
			"default_environment": "dev",
		}, result)
	})

	t.Run("complex values", func(t *testing.T) {
		v := viper.New()
		v.Set("set", []string{
			"git_url=git@github.com:org/repo.git",
			"docker_image=registry.example.com/app:v1.2.3",
			"description=My awesome application",
		})

		result := ParseStringMap(v, "set")
		assert.Equal(t, map[string]string{
			"git_url":      "git@github.com:org/repo.git",
			"docker_image": "registry.example.com/app:v1.2.3",
			"description":  "My awesome application",
		}, result)
	})
}
