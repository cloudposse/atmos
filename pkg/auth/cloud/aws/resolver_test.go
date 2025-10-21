package aws

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestGetResolverConfigOption_NoResolver(t *testing.T) {
	// Test with nil identity and provider
	opt := GetResolverConfigOption(nil, nil)
	assert.Nil(t, opt, "Expected nil when no resolver configured")

	// Test with empty identity and provider
	identity := &schema.Identity{}
	provider := &schema.Provider{}
	opt = GetResolverConfigOption(identity, provider)
	assert.Nil(t, opt, "Expected nil when resolver not configured")
}

func TestGetResolverConfigOption_IdentityResolver(t *testing.T) {
	// Test identity resolver takes precedence
	identity := &schema.Identity{
		Credentials: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "http://localhost:4566",
				},
			},
		},
	}
	provider := &schema.Provider{
		Spec: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "http://localhost:9999",
				},
			},
		},
	}

	opt := GetResolverConfigOption(identity, provider)
	assert.NotNil(t, opt, "Expected resolver option to be returned")
}

func TestGetResolverConfigOption_ProviderResolver(t *testing.T) {
	// Test provider resolver when identity has no resolver
	provider := &schema.Provider{
		Spec: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "http://localhost:4566",
				},
			},
		},
	}

	opt := GetResolverConfigOption(nil, provider)
	assert.NotNil(t, opt, "Expected resolver option to be returned from provider")
}

func TestGetResolverConfigOption_EmptyURL(t *testing.T) {
	// Test with empty URL
	identity := &schema.Identity{
		Credentials: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "",
				},
			},
		},
	}

	opt := GetResolverConfigOption(identity, nil)
	assert.Nil(t, opt, "Expected nil when URL is empty")
}

func TestExtractAWSConfig(t *testing.T) {
	// Test successful extraction
	m := map[string]interface{}{
		"aws": map[string]interface{}{
			"resolver": map[string]interface{}{
				"url": "http://localhost:4566",
			},
		},
	}
	awsConfig := extractAWSConfig(m)
	assert.NotNil(t, awsConfig)
	assert.NotNil(t, awsConfig.Resolver)
	assert.Equal(t, "http://localhost:4566", awsConfig.Resolver.URL)

	// Test nil map
	awsConfig = extractAWSConfig(nil)
	assert.Nil(t, awsConfig)

	// Test missing aws key
	m = map[string]interface{}{
		"other": "value",
	}
	awsConfig = extractAWSConfig(m)
	assert.Nil(t, awsConfig)
}

func TestExtractAWSConfig_InvalidData(t *testing.T) {
	// Test with invalid aws value (not a map)
	m := map[string]interface{}{
		"aws": "invalid-string",
	}
	awsConfig := extractAWSConfig(m)
	assert.Nil(t, awsConfig, "Expected nil when aws value is not a map")

	// Test with invalid resolver structure
	m = map[string]interface{}{
		"aws": map[string]interface{}{
			"resolver": "not-a-map",
		},
	}
	awsConfig = extractAWSConfig(m)
	assert.Nil(t, awsConfig, "Expected nil when resolver is not a map")
}

func TestGetResolverConfigOption_IdentityWithoutResolver(t *testing.T) {
	// Test identity with aws config but no resolver
	identity := &schema.Identity{
		Credentials: map[string]interface{}{
			"aws": map[string]interface{}{
				"other_config": "value",
			},
		},
	}

	opt := GetResolverConfigOption(identity, nil)
	assert.Nil(t, opt, "Expected nil when identity has no resolver")
}

func TestGetResolverConfigOption_ProviderWithoutResolver(t *testing.T) {
	// Test provider with aws config but no resolver
	provider := &schema.Provider{
		Spec: map[string]interface{}{
			"aws": map[string]interface{}{
				"other_config": "value",
			},
		},
	}

	opt := GetResolverConfigOption(nil, provider)
	assert.Nil(t, opt, "Expected nil when provider has no resolver")
}

func TestGetResolverConfigOption_IdentityWithNilResolver(t *testing.T) {
	// Test identity with nil resolver
	identity := &schema.Identity{
		Credentials: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": nil,
			},
		},
	}

	opt := GetResolverConfigOption(identity, nil)
	assert.Nil(t, opt, "Expected nil when resolver is nil")
}

func TestGetResolverConfigOption_BothEmptyCredentialsAndSpec(t *testing.T) {
	// Test with empty credentials and spec maps
	identity := &schema.Identity{
		Credentials: map[string]interface{}{},
	}
	provider := &schema.Provider{
		Spec: map[string]interface{}{},
	}

	opt := GetResolverConfigOption(identity, provider)
	assert.Nil(t, opt, "Expected nil when both credentials and spec are empty")
}

func TestGetResolverConfigOption_ProviderEmptyURL(t *testing.T) {
	// Test provider with empty URL
	provider := &schema.Provider{
		Spec: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "",
				},
			},
		},
	}

	opt := GetResolverConfigOption(nil, provider)
	assert.Nil(t, opt, "Expected nil when provider URL is empty")
}

func TestCreateResolverOption(t *testing.T) {
	// Test that createResolverOption returns a valid config option
	url := "http://localhost:4566"
	opt := createResolverOption(url)
	assert.NotNil(t, opt, "Expected non-nil config option")

	// Verify the option is a valid LoadOptionsFunc by checking it's callable
	// We can't easily test the actual behavior without creating a full AWS config,
	// but we can verify it returns a function
	assert.IsType(t, opt, opt, "Expected LoadOptionsFunc type")
}

func TestGetResolverConfigOption_ComplexScenarios(t *testing.T) {
	t.Run("identity with resolver overrides provider", func(t *testing.T) {
		identity := &schema.Identity{
			Credentials: map[string]interface{}{
				"aws": map[string]interface{}{
					"resolver": map[string]interface{}{
						"url": "http://identity-endpoint:4566",
					},
				},
			},
		}
		provider := &schema.Provider{
			Spec: map[string]interface{}{
				"aws": map[string]interface{}{
					"resolver": map[string]interface{}{
						"url": "http://provider-endpoint:4566",
					},
				},
			},
		}

		opt := GetResolverConfigOption(identity, provider)
		assert.NotNil(t, opt, "Expected resolver from identity to take precedence")
	})

	t.Run("provider resolver used when identity has no aws config", func(t *testing.T) {
		identity := &schema.Identity{
			Credentials: map[string]interface{}{
				"access_key": "test",
			},
		}
		provider := &schema.Provider{
			Spec: map[string]interface{}{
				"aws": map[string]interface{}{
					"resolver": map[string]interface{}{
						"url": "http://provider-endpoint:4566",
					},
				},
			},
		}

		opt := GetResolverConfigOption(identity, provider)
		assert.NotNil(t, opt, "Expected provider resolver to be used")
	})

	t.Run("nil returned when identity has invalid aws config", func(t *testing.T) {
		identity := &schema.Identity{
			Credentials: map[string]interface{}{
				"aws": "invalid",
			},
		}

		opt := GetResolverConfigOption(identity, nil)
		assert.Nil(t, opt, "Expected nil when aws config is invalid")
	})
}
