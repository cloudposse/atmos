package aws

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/mitchellh/mapstructure"

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
	// Check identity first (takes precedence)
	// Look for aws.resolver in identity.Credentials map
	if identity != nil {
		if awsConfig := extractAWSConfig(identity.Credentials); awsConfig != nil {
			if awsConfig.Resolver != nil && awsConfig.Resolver.URL != "" {
				return createResolverOption(awsConfig.Resolver.URL)
			}
		}
	}

	// Fallback to provider
	// Look for aws.resolver in provider.Spec map
	if provider != nil {
		if awsConfig := extractAWSConfig(provider.Spec); awsConfig != nil {
			if awsConfig.Resolver != nil && awsConfig.Resolver.URL != "" {
				return createResolverOption(awsConfig.Resolver.URL)
			}
		}
	}

	return nil
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

// createResolverOption creates an AWS config option with a custom endpoint resolver.
func createResolverOption(url string) config.LoadOptionsFunc {
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               url,
			HostnameImmutable: true, // prevent SDK from rewriting the host
		}, nil
	})

	return config.WithEndpointResolverWithOptions(resolver)
}
