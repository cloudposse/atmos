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
