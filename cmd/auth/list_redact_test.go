package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the secret-bearing schema fields these tests redact.
var (
	_ = schema.Provider{Password: ""}
	_ = schema.Identity{Credentials: nil}
)

const (
	testSAMLPassword  = "synthetic-saml-password"
	testAccessKeyID   = "AKIASYNTHETIC0000000"
	testSecretKeyText = "synthetic-secret-access-key"
)

func secretBearingAuthConfig() (map[string]schema.Provider, map[string]schema.Identity) {
	providers := map[string]schema.Provider{
		"okta-saml": {Kind: "aws/saml", URL: "https://idp.example/saml", Username: "jane", Password: testSAMLPassword},
		"sso":       {Kind: "aws/iam-identity-center", StartURL: "https://example.awsapps.com/start"},
	}
	identities := map[string]schema.Identity{
		"ci-user": {Kind: "aws/user", Credentials: map[string]interface{}{
			"access_key_id":     testAccessKeyID,
			"secret_access_key": testSecretKeyText,
			"region":            "us-east-1",
		}},
		"admin": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "sso"}},
	}
	return providers, identities
}

func TestRenderStructuredAuthListRedactsSecrets(t *testing.T) {
	renderers := map[string]func(map[string]schema.Provider, map[string]schema.Identity) (string, error){
		"json": renderJSON,
		"yaml": renderYAML,
	}
	for name, render := range renderers {
		t.Run(name, func(t *testing.T) {
			providers, identities := secretBearingAuthConfig()

			out, err := render(providers, identities)
			require.NoError(t, err)

			for _, secret := range []string{testSAMLPassword, testAccessKeyID, testSecretKeyText} {
				assert.NotContains(t, out, secret, "%s output must not print configured secrets", name)
			}
			assert.Contains(t, out, redactedValue, "redacted fields must stay visible as placeholders")
			// Non-secret configuration is still listed.
			assert.Contains(t, out, "https://idp.example/saml")
			assert.Contains(t, out, "jane")

			// The rendered document keeps the credential keys with redacted values.
			var doc map[string]map[string]map[string]interface{}
			if name == "json" {
				require.NoError(t, json.Unmarshal([]byte(out), &doc))
			} else {
				require.NoError(t, yaml.Unmarshal([]byte(out), &doc))
			}
			assert.Equal(t, redactedValue, doc[providersKey]["okta-saml"]["password"])
			_, hasPassword := doc[providersKey]["sso"]["password"]
			assert.False(t, hasPassword, "an unset password must stay omitted rather than shown as redacted")
			creds, ok := doc[identitiesKey]["ci-user"]["credentials"].(map[string]interface{})
			require.True(t, ok, "credentials must still be listed as a map of redacted values")
			assert.Equal(t, map[string]interface{}{
				"access_key_id":     redactedValue,
				"secret_access_key": redactedValue,
				"region":            "us-east-1",
			}, creds, "only secret credential values are redacted; region is configuration")

			// The caller's configuration must not be mutated by redaction.
			assert.Equal(t, testSAMLPassword, providers["okta-saml"].Password)
			assert.Equal(t, testSecretKeyText, identities["ci-user"].Credentials["secret_access_key"])
		})
	}
}

// TestExecuteAuthListCommand_StructuredOutputUsesDataChannel verifies the JSON
// and YAML output is written through the masked data channel (stdout) rather
// than straight to os.Stdout, where registered secrets would not be masked.
func TestExecuteAuthListCommand_StructuredOutputUsesDataChannel(t *testing.T) {
	setupMockAuthFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(consoleLinkTestStreams{output: stdout, error: stderr}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(func() {
		restore, restoreErr := iolib.NewContext()
		require.NoError(t, restoreErr)
		data.InitWriter(restore)
	})

	for _, format := range []string{"json", "yaml"} {
		stdout.Reset()
		cmd := authListCmd
		resetAuthCmdFlags(t, cmd)
		cmd.SetContext(context.Background())
		require.NoError(t, cmd.ParseFlags([]string{"--format=" + format}))

		require.NoError(t, executeAuthListCommand(cmd, nil))
		assert.Contains(t, stdout.String(), "mock-identity", "%s output must reach the data channel", format)
		assert.Contains(t, stdout.String(), "mock-provider", "%s output must reach the data channel", format)
	}
}

// TestRedactIdentitiesForList_KeepsNonSecretValuesAndTypes is the regression for
// `atmos auth list --format json|yaml` over-redacting: non-secret credential
// fields such as region must be shown, and booleans and numbers must keep their
// type instead of turning into the string "[redacted]".
func TestRedactIdentitiesForList_KeepsNonSecretValuesAndTypes(t *testing.T) {
	identities := map[string]schema.Identity{
		"dev": {
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"access_key_id":     testAccessKeyID,
				"secret_access_key": testSecretKeyText,
				"session_token":     "synthetic-session-token",
				"region":            "us-east-1",
				"webflow_enabled":   false,
				"mfa_arn":           "arn:aws:iam::123456789012:mfa/dev",
				"session_duration":  "12h",
			},
			Spec: map[string]interface{}{
				"endpoint_url":  "http://127.0.0.1:4566",
				"client_secret": "synthetic-client-secret",
				"retries":       3,
				"oauth": map[string]interface{}{
					"refresh_token": "synthetic-refresh-token",
					"scopes":        []interface{}{"openid", "profile"},
				},
			},
			Env: []schema.EnvironmentVariable{
				{Key: "AWS_PROFILE", Value: "dev"},
				{Key: "VAULT_TOKEN", Value: "synthetic-vault-token"},
			},
		},
	}

	got := redactIdentitiesForList(identities)["dev"]

	assert.Equal(t, map[string]interface{}{
		"access_key_id":     redactedValue,
		"secret_access_key": redactedValue,
		"session_token":     redactedValue,
		"region":            "us-east-1",
		"webflow_enabled":   false,
		"mfa_arn":           "arn:aws:iam::123456789012:mfa/dev",
		"session_duration":  "12h",
	}, got.Credentials)
	assert.Equal(t, "http://127.0.0.1:4566", got.Spec["endpoint_url"])
	assert.Equal(t, redactedValue, got.Spec["client_secret"])
	assert.Equal(t, 3, got.Spec["retries"], "numbers keep their type")
	oauth, ok := got.Spec["oauth"].(map[string]interface{})
	require.True(t, ok, "nested maps stay maps")
	assert.Equal(t, redactedValue, oauth["refresh_token"])
	assert.Equal(t, []interface{}{"openid", "profile"}, oauth["scopes"])
	assert.Equal(t, []schema.EnvironmentVariable{
		{Key: "AWS_PROFILE", Value: "dev"},
		{Key: "VAULT_TOKEN", Value: redactedValue},
	}, got.Env)

	// The caller's configuration is never mutated.
	original := identities["dev"]
	assert.Equal(t, testSecretKeyText, original.Credentials["secret_access_key"])
	assert.Equal(t, "synthetic-client-secret", original.Spec["client_secret"])
	assert.Equal(t, "synthetic-refresh-token", original.Spec["oauth"].(map[string]interface{})["refresh_token"])
	assert.Equal(t, "synthetic-vault-token", original.Env[1].Value)
}

// TestRedactIdentitiesForList_AccessKeyIDUnderAnyKey verifies that a value shaped
// like an AWS access key ID is redacted even under a key the denylist does not name.
func TestRedactIdentitiesForList_AccessKeyIDUnderAnyKey(t *testing.T) {
	got := redactIdentitiesForList(map[string]schema.Identity{
		"dev": {Kind: "aws/user", Credentials: map[string]interface{}{"key": testAccessKeyID, "id": "ASIASYNTHETIC0000001"}},
	})["dev"]
	assert.Equal(t, map[string]interface{}{"key": redactedValue, "id": redactedValue}, got.Credentials)
}

// TestRedactProvidersForList_RedactsSpecSecrets verifies provider spec secrets are
// redacted while the rest of the provider configuration is shown unchanged.
func TestRedactProvidersForList_RedactsSpecSecrets(t *testing.T) {
	providers := map[string]schema.Provider{
		"azure": {Kind: "azure/oidc", Region: "eastus", Spec: map[string]interface{}{
			"tenant_id":     "00000000-0000-0000-0000-000000000000",
			"client_secret": "synthetic-client-secret",
			"use_oidc":      true,
		}},
	}
	got := redactProvidersForList(providers)["azure"]
	assert.Equal(t, "eastus", got.Region)
	assert.Equal(t, map[string]interface{}{
		"tenant_id":     "00000000-0000-0000-0000-000000000000",
		"client_secret": redactedValue,
		"use_oidc":      true,
	}, got.Spec)
	assert.Equal(t, "synthetic-client-secret", providers["azure"].Spec["client_secret"])
}

// TestRenderStructuredAuthList_KeepsBooleanTypes verifies the rendered JSON and
// YAML keep a boolean credential field as a boolean.
func TestRenderStructuredAuthList_KeepsBooleanTypes(t *testing.T) {
	identities := map[string]schema.Identity{
		"dev": {Kind: "aws/user", Credentials: map[string]interface{}{"region": "us-east-1", "webflow_enabled": false}},
	}
	out, err := renderJSON(nil, identities)
	require.NoError(t, err)
	var doc map[string]map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	creds := doc[identitiesKey]["dev"]["credentials"].(map[string]interface{})
	assert.Equal(t, false, creds["webflow_enabled"])
	assert.Equal(t, "us-east-1", creds["region"])

	out, err = renderYAML(nil, identities)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal([]byte(out), &doc))
	creds = doc[identitiesKey]["dev"]["credentials"].(map[string]interface{})
	assert.Equal(t, false, creds["webflow_enabled"])
}

// TestIsSecretListKey pins the denylist of key names whose values are redacted.
func TestIsSecretListKey(t *testing.T) {
	for _, key := range []string{
		"password", "Password", "secret_access_key", "client_secret", "session_token", "refresh_token",
		"access_key_id", "aws_access_key_id", "private_key", "api_key", "apikey", "passphrase", "client-secret",
	} {
		assert.True(t, isSecretListKey(key), key)
	}
	for _, key := range []string{"region", "webflow_enabled", "mfa_arn", "session_duration", "endpoint_url", "tenant_id", "username", "AWS_PROFILE"} {
		assert.False(t, isSecretListKey(key), key)
	}
}

// TestRedactSecretValue_NestedUnderSecretKey verifies that every string below a
// secret key is redacted, while non-string values below it keep their type.
func TestRedactSecretValue_NestedUnderSecretKey(t *testing.T) {
	got := redactSecretMap(map[string]interface{}{
		"token":  map[string]interface{}{"value": "synthetic-token", "ttl": 3},
		"tokens": []interface{}{"synthetic-a", "synthetic-b", true},
		"empty":  "",
	})
	assert.Equal(t, map[string]interface{}{
		"token":  map[string]interface{}{"value": redactedValue, "ttl": 3},
		"tokens": []interface{}{redactedValue, redactedValue, true},
		"empty":  "",
	}, got)
	assert.Nil(t, redactSecretMap(nil), "an omitted section stays omitted")
	assert.Nil(t, redactSecretEnv(nil))
}
