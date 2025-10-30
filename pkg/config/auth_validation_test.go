package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/syntax"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateAuthConfigSyntax_EmptyConfig(t *testing.T) {
	// Empty auth config should not trigger validation.
	authConfig := &schema.AuthConfig{}
	err := syntax.ValidateSyntax(authConfig)
	assert.NoError(t, err)
}

func TestValidateAuthConfigSyntax_OnlyProvidersConfigured(t *testing.T) {
	// Valid provider configuration should pass.
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"aws-sso": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://example.awsapps.com/start",
			},
		},
	}
	err := syntax.ValidateSyntax(authConfig)
	assert.NoError(t, err)
}

func TestValidateAuthConfigSyntax_InvalidProviderKind(t *testing.T) {
	// Invalid provider kind should fail validation.
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"bad-provider": {
				Kind: "invalid/kind",
			},
		},
	}
	err := syntax.ValidateSyntax(authConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid provider kind")
}
