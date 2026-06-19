package aws

import (
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/go-viper/mapstructure/v2"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AWSConfig defines AWS-specific configuration for providers and identities.
type AWSConfig struct {
	Resolver *ResolverConfig `yaml:"resolver,omitempty" json:"resolver,omitempty" mapstructure:"resolver"`
}

// ResolverConfig defines custom endpoint resolver configuration for AWS services.
type ResolverConfig struct {
	URL string `yaml:"url" json:"url" mapstructure:"url"`
}

// GetResolverConfigOption extracts the AWS resolver configuration from identity or provider
// and returns an AWS config option. Returns nil if no resolver is configured.
// Identity resolver takes precedence over provider resolver.
// AWS config is extracted from the Credentials map for identities and Spec map for providers.
func GetResolverConfigOption(identity *schema.Identity, provider *schema.Provider) config.LoadOptionsFunc {
	defer perf.Track(nil, "aws.GetResolverConfigOption")()

	url := resolverEndpointURL(identity, provider)
	if url == "" {
		return nil
	}
	return createResolverOption(url)
}

func resolverEndpointURL(identity *schema.Identity, provider *schema.Provider) string {
	if identity != nil {
		if awsConfig := extractAWSConfig(identity.Credentials); awsConfig != nil {
			if awsConfig.Resolver != nil && awsConfig.Resolver.URL != "" {
				return awsConfig.Resolver.URL
			}
		}
	}

	if provider != nil {
		if awsConfig := extractAWSConfig(provider.Spec); awsConfig != nil {
			if awsConfig.Resolver != nil && awsConfig.Resolver.URL != "" {
				return awsConfig.Resolver.URL
			}
		}
	}

	return ""
}

// extractAWSConfig extracts AWSConfig from a map[string]interface{} using mapstructure.
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

// createResolverOption creates an AWS config option with a custom endpoint.
// Uses the newer config.WithBaseEndpoint instead of the deprecated WithEndpointResolverWithOptions.
func createResolverOption(url string) config.LoadOptionsFunc {
	return config.WithBaseEndpoint(url)
}
