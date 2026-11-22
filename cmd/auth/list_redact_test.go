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
				"region":            redactedValue,
			}, creds)

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
