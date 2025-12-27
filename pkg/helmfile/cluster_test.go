package helmfile

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockTemplateProcessor is a test helper that returns a predefined result.
func mockTemplateProcessor(result string, err error) TemplateProcessor {
	return func(
		atmosConfig *schema.AtmosConfiguration,
		tmplName string,
		tmplValue string,
		tmplData any,
		ignoreMissingTemplateValues bool,
	) (string, error) {
		if err != nil {
			return "", err
		}
		return result, nil
	}
}

func TestResolveClusterName(t *testing.T) {
	defaultContext := &Context{
		Namespace:   "test-ns",
		Tenant:      "tenant1",
		Environment: "dev",
		Stage:       "ue2",
		Region:      "us-east-2",
	}

	defaultAtmosConfig := &schema.AtmosConfiguration{}
	defaultComponentSection := map[string]any{
		"vars": map[string]any{
			"namespace":   "test-ns",
			"tenant":      "tenant1",
			"environment": "dev",
			"stage":       "ue2",
			"region":      "us-east-2",
		},
	}
	emptyContext := &Context{}

	tests := []struct {
		name               string
		input              ClusterNameInput
		context            *Context
		atmosConfig        *schema.AtmosConfiguration
		componentSection   map[string]any
		templateProcessor  TemplateProcessor
		expectedCluster    string
		expectedSource     string
		expectedDeprecated bool
		expectedError      error
	}{
		{
			name: "flag takes highest precedence",
			input: ClusterNameInput{
				FlagValue:   "flag-cluster",
				ConfigValue: "config-cluster",
				Template:    "{{ .vars.namespace }}-eks",
				Pattern:     "{namespace}-eks",
			},
			context:            defaultContext,
			atmosConfig:        defaultAtmosConfig,
			componentSection:   defaultComponentSection,
			templateProcessor:  mockTemplateProcessor("template-cluster", nil),
			expectedCluster:    "flag-cluster",
			expectedSource:     "flag",
			expectedDeprecated: false,
			expectedError:      nil,
		},
		{
			name: "config takes precedence over template",
			input: ClusterNameInput{
				FlagValue:   "",
				ConfigValue: "config-cluster",
				Template:    "{{ .vars.namespace }}-eks",
				Pattern:     "{namespace}-eks",
			},
			context:            defaultContext,
			atmosConfig:        defaultAtmosConfig,
			componentSection:   defaultComponentSection,
			templateProcessor:  mockTemplateProcessor("template-cluster", nil),
			expectedCluster:    "config-cluster",
			expectedSource:     "config",
			expectedDeprecated: false,
			expectedError:      nil,
		},
		{
			name: "template takes precedence over pattern",
			input: ClusterNameInput{
				FlagValue:   "",
				ConfigValue: "",
				Template:    "{{ .vars.namespace }}-eks",
				Pattern:     "{namespace}-eks",
			},
			context:            defaultContext,
			atmosConfig:        defaultAtmosConfig,
			componentSection:   defaultComponentSection,
			templateProcessor:  mockTemplateProcessor("test-ns-eks", nil),
			expectedCluster:    "test-ns-eks",
			expectedSource:     "template",
			expectedDeprecated: false,
			expectedError:      nil,
		},
		{
			name: "pattern fallback is deprecated",
			input: ClusterNameInput{
				FlagValue:   "",
				ConfigValue: "",
				Template:    "",
				Pattern:     "{namespace}-{stage}-eks",
			},
			context:            defaultContext,
			atmosConfig:        defaultAtmosConfig,
			componentSection:   defaultComponentSection,
			templateProcessor:  mockTemplateProcessor("", nil),
			expectedCluster:    "test-ns-ue2-eks",
			expectedSource:     "pattern",
			expectedDeprecated: true,
			expectedError:      nil,
		},
		{
			name: "error when no source configured",
			input: ClusterNameInput{
				FlagValue:   "",
				ConfigValue: "",
				Template:    "",
				Pattern:     "",
			},
			context:            defaultContext,
			atmosConfig:        defaultAtmosConfig,
			componentSection:   defaultComponentSection,
			templateProcessor:  mockTemplateProcessor("", nil),
			expectedCluster:    "",
			expectedSource:     "",
			expectedDeprecated: false,
			expectedError:      errUtils.ErrMissingHelmfileClusterName,
		},
		{
			name: "template processing error propagates",
			input: ClusterNameInput{
				FlagValue:   "",
				ConfigValue: "",
				Template:    "{{ .invalid }}",
				Pattern:     "",
			},
			context:            defaultContext,
			atmosConfig:        defaultAtmosConfig,
			componentSection:   defaultComponentSection,
			templateProcessor:  mockTemplateProcessor("", errors.New("template error")),
			expectedCluster:    "",
			expectedSource:     "",
			expectedDeprecated: false,
			expectedError:      errors.New("failed to process cluster_name_template"),
		},
		{
			name: "flag only - minimal input",
			input: ClusterNameInput{
				FlagValue: "my-cluster",
			},
			context:            emptyContext,
			atmosConfig:        defaultAtmosConfig,
			componentSection:   nil,
			templateProcessor:  nil, // Not used when flag is provided.
			expectedCluster:    "my-cluster",
			expectedSource:     "flag",
			expectedDeprecated: false,
			expectedError:      nil,
		},
		{
			name: "config only - minimal input",
			input: ClusterNameInput{
				ConfigValue: "configured-cluster",
			},
			context:            emptyContext,
			atmosConfig:        defaultAtmosConfig,
			componentSection:   nil,
			templateProcessor:  nil, // Not used when config is provided.
			expectedCluster:    "configured-cluster",
			expectedSource:     "config",
			expectedDeprecated: false,
			expectedError:      nil,
		},
		{
			name: "complex pattern with all tokens",
			input: ClusterNameInput{
				Pattern: "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
			},
			context: &Context{
				Namespace:   "acme",
				Tenant:      "platform",
				Environment: "prod",
				Stage:       "uw2",
			},
			atmosConfig:        defaultAtmosConfig,
			componentSection:   nil,
			templateProcessor:  nil,
			expectedCluster:    "acme-platform-prod-uw2-eks-cluster",
			expectedSource:     "pattern",
			expectedDeprecated: true,
			expectedError:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveClusterName(
				tt.input,
				tt.context,
				tt.atmosConfig,
				tt.componentSection,
				tt.templateProcessor,
			)

			if tt.expectedError != nil {
				require.Error(t, err)
				if errors.Is(tt.expectedError, errUtils.ErrMissingHelmfileClusterName) {
					assert.ErrorIs(t, err, errUtils.ErrMissingHelmfileClusterName)
				} else {
					assert.Contains(t, err.Error(), tt.expectedError.Error())
				}
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedCluster, result.ClusterName)
			assert.Equal(t, tt.expectedSource, result.Source)
			assert.Equal(t, tt.expectedDeprecated, result.IsDeprecated)
		})
	}
}

func BenchmarkResolveClusterName(b *testing.B) {
	input := ClusterNameInput{
		FlagValue: "benchmark-cluster",
	}
	context := &Context{}
	atmosConfig := &schema.AtmosConfiguration{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ResolveClusterName(input, context, atmosConfig, nil, nil)
	}
}
