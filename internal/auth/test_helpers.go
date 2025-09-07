package auth

import (
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestHelpers provides common test utilities for auth package tests

// CreateTestProvider creates a test provider configuration.
func CreateTestProvider(kind, region string) *schema.Provider {
	return &schema.Provider{
		Kind:   kind,
		Region: region,
	}
}

// CreateTestIdentity creates a test identity configuration.
func CreateTestIdentity(kind string) *schema.Identity {
	return &schema.Identity{
		Kind: kind,
	}
}

// CreateTestCredentials creates test AWS credentials.
func CreateTestCredentials(accessKeyID, secretKey, region string) *schema.Credentials {
	return &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretKey,
			Region:          region,
		},
	}
}

// CreateTestOIDCCredentials creates test OIDC credentials.
func CreateTestOIDCCredentials(token, provider string) *schema.Credentials {
	return &schema.Credentials{
		OIDC: &schema.OIDCCredentials{
			Token:    token,
			Provider: provider,
		},
	}
}

// CreateTestWhoamiInfo creates test whoami information.
func CreateTestWhoamiInfo(provider, identity, principal string) *schema.WhoamiInfo {
	return &schema.WhoamiInfo{
		Provider:    provider,
		Identity:    identity,
		Principal:   principal,
		LastUpdated: time.Now(),
	}
}

// CreateTestAuthConfig creates a test auth configuration.
func CreateTestAuthConfig() *schema.AuthConfig {
	return &schema.AuthConfig{
		Providers:  make(map[string]schema.Provider),
		Identities: make(map[string]schema.Identity),
	}
}
