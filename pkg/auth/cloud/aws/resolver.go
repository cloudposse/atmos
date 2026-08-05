package aws

import (
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/go-viper/mapstructure/v2"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AWSConfig defines the legacy AWS-specific configuration for providers and identities.
type AWSConfig struct {
	Resolver *ResolverConfig `yaml:"resolver,omitempty" json:"resolver,omitempty" mapstructure:"resolver"`
}

// ResolverConfig defines the legacy custom endpoint resolver configuration for AWS services.
type ResolverConfig struct {
	URL string `yaml:"url" json:"url" mapstructure:"url"`
}

// GetBaseEndpointConfigOption extracts the AWS base endpoint configuration from identity or provider
// and returns an AWS config option. Returns nil if no endpoint is configured.
// New spec.endpoint_url values take precedence over legacy aws.resolver.url values.
func GetBaseEndpointConfigOption(identity *schema.Identity, provider *schema.Provider) config.LoadOptionsFunc {
	defer perf.Track(nil, "aws.GetBaseEndpointConfigOption")()

	url := baseEndpointURL(identity, provider)
	if url == "" {
		return nil
	}
	return createBaseEndpointOption(url)
}

// GetResolverConfigOption is kept for compatibility with existing callers.
// Use GetBaseEndpointConfigOption for new code.
func GetResolverConfigOption(identity *schema.Identity, provider *schema.Provider) config.LoadOptionsFunc {
	defer perf.Track(nil, "aws.GetResolverConfigOption")()
	return GetBaseEndpointConfigOption(identity, provider)
}

func baseEndpointURL(identity *schema.Identity, provider *schema.Provider) string {
	if identity != nil {
		if url := endpointURLFromSpec(identity.Spec); url != "" {
			return url
		}
		if url := legacyResolverEndpointURL(identity.Credentials); url != "" {
			return url
		}
	}

	if provider != nil {
		if url := endpointURLFromSpec(provider.Spec); url != "" {
			return url
		}
		if url := legacyResolverEndpointURL(provider.Spec); url != "" {
			return url
		}
	}

	return ""
}

func endpointURLFromSpec(spec map[string]interface{}) string {
	if spec == nil {
		return ""
	}
	url, ok := spec["endpoint_url"].(string)
	if !ok {
		return ""
	}
	return url
}

func legacyResolverEndpointURL(m map[string]interface{}) string {
	awsConfig := extractAWSConfig(m)
	if awsConfig == nil || awsConfig.Resolver == nil {
		return ""
	}
	return awsConfig.Resolver.URL
}

// extractAWSConfig extracts legacy AWSConfig from a map[string]interface{} using mapstructure.
// Returns nil if no "aws" key exists or if decoding fails.
func extractAWSConfig(m map[string]interface{}) *AWSConfig {
	if m == nil {
		return nil
	}

	awsRaw, exists := m["aws"]
	if !exists {
		return nil
	}

	var awsConfig AWSConfig
	if err := mapstructure.Decode(awsRaw, &awsConfig); err != nil {
		return nil
	}

	return &awsConfig
}

// createBaseEndpointOption creates an AWS config option with a custom endpoint.
// Uses the newer config.WithBaseEndpoint instead of the deprecated WithEndpointResolverWithOptions.
func createBaseEndpointOption(url string) config.LoadOptionsFunc {
	return config.WithBaseEndpoint(url)
}
