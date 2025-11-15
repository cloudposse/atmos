package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrincipal_ToMap_Complete(t *testing.T) {
	// Test with all fields populated.
	principal := &Principal{
		Name: "TestRole",
		Account: &Account{
			Name: "prod-account",
			ID:   "123456789012",
		},
	}

	result := principal.ToMap()

	assert.Equal(t, "TestRole", result["name"])
	account, ok := result["account"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "prod-account", account["name"])
	assert.Equal(t, "123456789012", account["id"])
}

func TestPrincipal_ToMap_NameOnly(t *testing.T) {
	// Test with only name field.
	principal := &Principal{
		Name: "TestRole",
	}

	result := principal.ToMap()

	assert.Equal(t, "TestRole", result["name"])
	_, hasAccount := result["account"]
	assert.False(t, hasAccount, "Account should not be present when nil")
}

func TestPrincipal_ToMap_Empty(t *testing.T) {
	// Test with empty principal.
	principal := &Principal{}

	result := principal.ToMap()

	assert.Empty(t, result, "Empty principal should produce empty map")
}

func TestPrincipal_ToMap_PartialAccount(t *testing.T) {
	// Test with account but only name.
	principal := &Principal{
		Name: "TestRole",
		Account: &Account{
			Name: "prod-account",
		},
	}

	result := principal.ToMap()

	assert.Equal(t, "TestRole", result["name"])
	account, ok := result["account"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "prod-account", account["name"])
	_, hasID := account["id"]
	assert.False(t, hasID, "ID should not be present when empty")
}

func TestPrincipal_ToMap_AccountIDOnly(t *testing.T) {
	// Test with account but only ID.
	principal := &Principal{
		Name: "TestRole",
		Account: &Account{
			ID: "123456789012",
		},
	}

	result := principal.ToMap()

	assert.Equal(t, "TestRole", result["name"])
	account, ok := result["account"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "123456789012", account["id"])
	_, hasName := account["name"]
	assert.False(t, hasName, "Name should not be present when empty")
}

func TestPrincipal_ToMap_EmptyAccount(t *testing.T) {
	// Test with empty account struct.
	principal := &Principal{
		Name:    "TestRole",
		Account: &Account{},
	}

	result := principal.ToMap()

	assert.Equal(t, "TestRole", result["name"])
	_, hasAccount := result["account"]
	assert.False(t, hasAccount, "Empty account should not be included in map")
}

func TestAuthConfig_Structure(t *testing.T) {
	// Test that AuthConfig can be created with expected fields.
	config := &AuthConfig{
		Providers: map[string]Provider{
			"test-provider": {
				Kind:   "aws-sso",
				Region: "us-east-1",
			},
		},
		Identities: map[string]Identity{
			"test-identity": {
				Kind:     "aws-assume-role",
				Provider: "test-provider",
			},
		},
		IdentityCaseMap: map[string]string{
			"test-identity": "Test-Identity",
		},
	}

	assert.NotNil(t, config)
	assert.Len(t, config.Providers, 1)
	assert.Len(t, config.Identities, 1)
	assert.Len(t, config.IdentityCaseMap, 1)
}

func TestProvider_Structure(t *testing.T) {
	// Test that Provider struct has expected fields.
	autoProvision := true
	provider := Provider{
		Kind:                    "aws-sso",
		StartURL:                "https://test.awsapps.com/start",
		Region:                  "us-east-1",
		AutoProvisionIdentities: &autoProvision,
		Default:                 true,
		Session: &SessionConfig{
			Duration: "4h",
		},
		Console: &ConsoleConfig{
			SessionDuration: "12h",
		},
	}

	assert.Equal(t, "aws-sso", provider.Kind)
	assert.Equal(t, "https://test.awsapps.com/start", provider.StartURL)
	assert.Equal(t, "us-east-1", provider.Region)
	assert.True(t, *provider.AutoProvisionIdentities)
	assert.True(t, provider.Default)
	assert.Equal(t, "4h", provider.Session.Duration)
	assert.Equal(t, "12h", provider.Console.SessionDuration)
}

func TestIdentity_Structure(t *testing.T) {
	// Test that Identity struct has expected fields.
	identity := Identity{
		Kind:     "aws-assume-role",
		Default:  true,
		Provider: "test-provider",
		Via: &IdentityVia{
			Provider: "base-provider",
			Identity: "base-identity",
		},
		Principal: map[string]interface{}{
			"name": "TestRole",
			"account": map[string]interface{}{
				"name": "prod",
				"id":   "123456789012",
			},
		},
		Alias: "prod-admin",
		Env: []EnvironmentVariable{
			{Key: "AWS_REGION", Value: "us-east-1"},
		},
		Session: &SessionConfig{
			Duration: "1h",
		},
	}

	assert.Equal(t, "aws-assume-role", identity.Kind)
	assert.True(t, identity.Default)
	assert.Equal(t, "test-provider", identity.Provider)
	assert.NotNil(t, identity.Via)
	assert.Equal(t, "base-provider", identity.Via.Provider)
	assert.Equal(t, "base-identity", identity.Via.Identity)
	assert.Equal(t, "prod-admin", identity.Alias)
	assert.Len(t, identity.Env, 1)
	assert.Equal(t, "AWS_REGION", identity.Env[0].Key)
	assert.Equal(t, "us-east-1", identity.Env[0].Value)
	assert.Equal(t, "1h", identity.Session.Duration)
}

func TestKeyringConfig_Structure(t *testing.T) {
	// Test KeyringConfig structure.
	config := KeyringConfig{
		Type: "file",
		Spec: map[string]interface{}{
			"path":         "/custom/path",
			"password_env": "CUSTOM_PASSWORD",
		},
	}

	assert.Equal(t, "file", config.Type)
	assert.Equal(t, "/custom/path", config.Spec["path"])
	assert.Equal(t, "CUSTOM_PASSWORD", config.Spec["password_env"])
}

func TestEnvironmentVariable_Structure(t *testing.T) {
	// Test EnvironmentVariable structure.
	env := EnvironmentVariable{
		Key:   "AWS_REGION",
		Value: "us-west-2",
	}

	assert.Equal(t, "AWS_REGION", env.Key)
	assert.Equal(t, "us-west-2", env.Value)
}

func TestComponentAuthConfig_Structure(t *testing.T) {
	// Test ComponentAuthConfig structure.
	config := ComponentAuthConfig{
		Providers: map[string]Provider{
			"component-provider": {
				Kind:   "aws-sso",
				Region: "eu-west-1",
			},
		},
		Identities: map[string]Identity{
			"component-identity": {
				Kind:     "aws-assume-role",
				Provider: "component-provider",
			},
		},
	}

	assert.Len(t, config.Providers, 1)
	assert.Len(t, config.Identities, 1)
	assert.Equal(t, "aws-sso", config.Providers["component-provider"].Kind)
	assert.Equal(t, "aws-assume-role", config.Identities["component-identity"].Kind)
}

func TestSessionConfig_Structure(t *testing.T) {
	// Test SessionConfig structure.
	session := SessionConfig{
		Duration: "2h30m",
	}

	assert.Equal(t, "2h30m", session.Duration)
}

func TestConsoleConfig_Structure(t *testing.T) {
	// Test ConsoleConfig structure.
	console := ConsoleConfig{
		SessionDuration: "8h",
	}

	assert.Equal(t, "8h", console.SessionDuration)
}
