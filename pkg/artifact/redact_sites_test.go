package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestDecodeOptions_ErrorDoesNotEchoCredential checks that an option key or value that holds a
// credential never reaches the decode error.
func TestDecodeOptions_ErrorDoesNotEchoCredential(t *testing.T) {
	const credential = "https://usr7:S3cr3tA@example.com/x"
	tests := []struct {
		name string
		raw  map[string]any
	}{
		{name: "unknown key", raw: map[string]any{"registry": "ghcr.io", credential: "x"}},
		{name: "credential-like unknown key", raw: map[string]any{"registry": "ghcr.io", "usr7:S3cr3tA@password": "x"}},
		{name: "mistyped value", raw: map[string]any{"registry": map[string]any{"host": credential}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var options OCIRegistryOptions
			err := DecodeOptions(tt.raw, &options)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.NotContains(t, err.Error(), "S3cr3tA")
			assert.NotContains(t, err.Error(), "usr7")
		})
	}
}

// TestValidationErrors_DoNotEchoCredentials checks the validators that echo a user-supplied
// registry, prefix, key pattern, or reference never repeat a credential in it.
func TestValidationErrors_DoNotEchoCredentials(t *testing.T) {
	const credential = "usr7:S3cr3tA@example.com"
	errs := map[string]error{
		"registry":    (&OCIRegistryOptions{Registry: credential}).Validate(),
		"prefix":      ValidatePrefix(credential + "/../x"),
		"key pattern": ValidateKeyPattern("/"+credential+"/{{.Name}}", PackagingTar),
		"reference":   func() error { _, err := ParseRef(credential); return err }(),
		"name":        ValidateName(credential),
		"version":     ValidateVersion(credential),
	}
	for name, err := range errs {
		t.Run(name, func(t *testing.T) {
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "S3cr3tA")
			assert.NotContains(t, err.Error(), "usr7")
		})
	}
}

// TestValidateKeyPattern_UnknownFieldDoesNotEchoCredential checks that the field name extracted from
// a template action is withheld when the pattern carries a credential: a field name is a fragment
// of the pattern, so scrubbing the pattern alone does not scrub it.
func TestValidateKeyPattern_UnknownFieldDoesNotEchoCredential(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		// secrets are what the error must not hold. A field name in the path of the pattern is
		// echoed with the rest of the redacted pattern, so only the userinfo is a secret there.
		secrets []string
	}{
		{name: "password is the field", pattern: "https://usr7:{{.S3cr3tA}}@example.com/{{.Name}}.tar", secrets: []string{"S3cr3t", "usr7"}},
		{name: "token is the field", pattern: "https://{{.S3cr3tA}}@example.com/{{.Name}}.tar", secrets: []string{"S3cr3t"}},
		{name: "scheme-less userinfo", pattern: "usr7:{{.S3cr3tA}}@example.com/{{.Name}}.tar", secrets: []string{"S3cr3t", "usr7"}},
		{name: "field after a literal credential", pattern: "https://usr7:S3cr3tA@example.com/{{.Bogus}}.tar", secrets: []string{"S3cr3tA", "usr7"}},
		{name: "second action holds the field", pattern: "https://usr7:S3cr3tA@example.com/{{.Name}}/{{.Bogus}}.tar", secrets: []string{"S3cr3tA", "usr7"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKeyPattern(tt.pattern, PackagingTar)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			for _, secret := range tt.secrets {
				assert.NotContains(t, err.Error(), secret)
			}
			assert.NotContains(t, err.Error(), "unknown field .", "the field name is withheld when the pattern carries a credential")
		})
	}
}

// TestValidateKeyPattern_UnknownFieldIsNamedWithoutCredential is the negative path of the test
// above: a pattern that carries no credential still names the unknown field, because the name is
// what lets the user fix the pattern.
func TestValidateKeyPattern_UnknownFieldIsNamedWithoutCredential(t *testing.T) {
	err := ValidateKeyPattern("builds/{{.Bogus}}/{{.Name}}.tar", PackagingTar)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "unknown field .Bogus")
	assert.Contains(t, err.Error(), "builds/{{.Bogus}}/{{.Name}}.tar")
}
