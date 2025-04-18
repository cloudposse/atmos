package config

import (
	"reflect"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestGetContextFromVars(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]any
		expected schema.Context
	}{
		{
			name: "all fields set correctly",
			vars: map[string]any{
				"namespace":   "ns",
				"tenant":      "tn",
				"environment": "env",
				"stage":       "st",
				"region":      "us-west-1",
				"attributes":  []any{"attr1", "attr2"},
			},
			expected: schema.Context{
				Namespace:   "ns",
				Tenant:      "tn",
				Environment: "env",
				Stage:       "st",
				Region:      "us-west-1",
				Attributes:  []any{"attr1", "attr2"},
			},
		},
		{
			name: "missing all fields",
			vars: map[string]any{},
			expected: schema.Context{
				Namespace:   "",
				Tenant:      "",
				Environment: "",
				Stage:       "",
				Region:      "",
				Attributes:  nil,
			},
		},
		{
			name: "invalid types for all fields",
			vars: map[string]any{
				"namespace":   123,
				"tenant":      true,
				"environment": []string{"not", "a", "string"},
				"stage":       nil,
				"region":      struct{}{},
				"attributes":  "not-a-slice",
			},
			expected: schema.Context{
				Namespace:   "",
				Tenant:      "",
				Environment: "",
				Stage:       "",
				Region:      "",
				Attributes:  nil,
			},
		},
		{
			name: "some fields missing or wrong",
			vars: map[string]any{
				"namespace":  "prod",
				"tenant":     42, // invalid
				"region":     "eu-central-1",
				"attributes": []any{},
			},
			expected: schema.Context{
				Namespace:   "prod",
				Tenant:      "",
				Environment: "",
				Stage:       "",
				Region:      "eu-central-1",
				Attributes:  []any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetContextFromVars(tt.vars)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("GetContextFromVars() = %+v, want %+v", got, tt.expected)
			}
		})
	}
}

func TestGetConfigFilePatterns(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		forGlobMatch bool
		expected     []string
	}{
		{
			name:         "path with extension",
			path:         "../../tests/fixtures/scenarios/complete/atmos.yaml",
			forGlobMatch: false,
			expected:     []string{"../../tests/fixtures/scenarios/complete/atmos.yaml"},
		},
		{
			name:         "path without extension, forGlobMatch false",
			path:         "atmos",
			forGlobMatch: false,
			expected:     []string{"atmos", "atmos.yaml", "atmos.yml", "atmos.yaml.tmpl", "atmos.yml.tmpl"},
		},
		{
			name:         "path without extension, forGlobMatch true",
			path:         "config",
			forGlobMatch: true,
			expected:     []string{"config.yaml", "config.yml", "config.yaml.tmpl", "config.yml.tmpl"},
		},
		{
			name:         "path starting with dot",
			path:         ".hidden",
			forGlobMatch: false,
			expected:     []string{".hidden"},
		},
		{
			name:         "path ending with dot",
			path:         "config.",
			forGlobMatch: false,
			expected:     []string{"config."},
		},
		{
			name:         "path with directory",
			path:         "/path/to/config",
			forGlobMatch: false,
			expected:     []string{"/path/to/config", "/path/to/config.yaml", "/path/to/config.yml", "/path/to/config.yaml.tmpl", "/path/to/config.yml.tmpl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getConfigFilePatterns(tt.path, tt.forGlobMatch)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetStackNameFromContextAndStackNamePattern(t *testing.T) {
	tests := []struct {
		name             string
		namespace        string
		tenant           string
		environment      string
		stage            string
		stackNamePattern string
		expectedResult   string
		expectedError    string
	}{
		{
			name:             "Empty stackNamePattern",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "",
			expectedError:    "stack name pattern must be provided",
		},
		{
			name:             "Missing namespace",
			namespace:        "",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "{namespace}-{tenant}",
			expectedError:    "stack name pattern '{namespace}-{tenant}' includes '{namespace}', but namespace is not provided",
		},
		{
			name:             "Missing tenant",
			namespace:        "ns",
			tenant:           "",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "{namespace}-{tenant}",
			expectedError:    "stack name pattern '{namespace}-{tenant}' includes '{tenant}', but tenant is not provided",
		},
		{
			name:             "Missing environment",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "",
			stage:            "st",
			stackNamePattern: "{environment}-{stage}",
			expectedError:    "stack name pattern '{environment}-{stage}' includes '{environment}', but environment is not provided",
		},
		{
			name:             "Missing stage",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "",
			stackNamePattern: "{tenant}-{stage}",
			expectedError:    "stack name pattern '{tenant}-{stage}' includes '{stage}', but stage is not provided",
		},
		{
			name:             "Single placeholder - namespace",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "{namespace}",
			expectedResult:   "ns",
			expectedError:    "",
		},
		{
			name:             "Multiple placeholders",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "{namespace}-{tenant}-{environment}-{stage}",
			expectedResult:   "ns-tn-env-st",
			expectedError:    "",
		},
		{
			name:             "Reordered placeholders",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "{stage}-{environment}-{tenant}-{namespace}",
			expectedResult:   "st-env-tn-ns",
			expectedError:    "",
		},
		{
			name:             "Repeated placeholder",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "{namespace}-{namespace}",
			expectedResult:   "ns-ns",
			expectedError:    "",
		},
		{
			name:             "Non-placeholder pattern",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "static",
			expectedResult:   "",
			expectedError:    "",
		},
		{
			name:             "Pattern with extra hyphens",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "{namespace}--{tenant}",
			expectedResult:   "ns-tn",
			expectedError:    "",
		},
		{
			name:             "Case-sensitive placeholder",
			namespace:        "ns",
			tenant:           "tn",
			environment:      "env",
			stage:            "st",
			stackNamePattern: "{NAMESPACE}-{tenant}",
			expectedResult:   "tn",
			expectedError:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetStackNameFromContextAndStackNamePattern(
				tt.namespace,
				tt.tenant,
				tt.environment,
				tt.stage,
				tt.stackNamePattern,
			)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.expectedError)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}
