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
