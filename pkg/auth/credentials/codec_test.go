package credentials

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/providers/mock"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// customCredentials exercises registration of an independently defined credential shape.
type customCredentials struct {
	types.AWSCredentials
	Subject string `json:"subject"`
}

func TestCodecRegistryLegacyRoundTrip(t *testing.T) {
	registry := NewCodecRegistry()
	for tag, original := range map[string]types.ICredentials{
		"aws":       &types.AWSCredentials{AccessKeyID: "test-access"},
		"gcp":       &types.GCPCredentials{},
		"oidc":      &types.OIDCCredentials{},
		"atmos-pro": &types.ProCredentials{},
		"oci/v1":    &types.OCICredentials{Registry: "ghcr.io", Username: "bot", Password: "test-secret"},
		"mock":      &mock.Credentials{},
	} {
		t.Run(tag, func(t *testing.T) {
			expected, err := json.Marshal(original)
			require.NoError(t, err)
			actualTag, raw, err := registry.Marshal(original)
			require.NoError(t, err)
			assert.Equal(t, tag, actualTag)
			assert.JSONEq(t, string(expected), string(raw))
			decoded, err := registry.Unmarshal(tag, expected)
			require.NoError(t, err)
			assert.Equal(t, original, decoded)
		})
	}
}

func TestCodecRegistryCustomType(t *testing.T) {
	registry := NewCodecRegistry()
	require.NoError(t, registry.Register("custom/v1", func() types.ICredentials { return &customCredentials{} }))
	original := &customCredentials{Subject: "customer"}
	tag, raw, err := registry.Marshal(original)
	require.NoError(t, err)
	assert.Equal(t, "custom/v1", tag)
	decoded, err := registry.Unmarshal(tag, raw)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
	require.ErrorIs(t, registry.Register("custom/v1", func() types.ICredentials { return &mock.Credentials{} }), errUtils.ErrDuplicateCredentialCodec)
	require.ErrorIs(t, registry.Register("different", func() types.ICredentials { return &customCredentials{} }), errUtils.ErrDuplicateCredentialCodec)
}

func TestCodecRegistryRejectsInvalidInput(t *testing.T) {
	registry := NewCodecRegistry()
	for _, tc := range []struct {
		name        string
		constructor CredentialConstructor
	}{
		{"nil function", nil},
		{"nil interface", func() types.ICredentials { return nil }},
		{"typed nil", func() types.ICredentials { return (*customCredentials)(nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, registry.Register("custom", tc.constructor), errUtils.ErrInvalidCredentialCodec)
		})
	}
	require.ErrorIs(t, registry.Register("", func() types.ICredentials { return &customCredentials{} }), errUtils.ErrInvalidCredentialCodec)
	for _, original := range []types.ICredentials{nil, (*customCredentials)(nil), &customCredentials{}} {
		_, _, err := registry.Marshal(original)
		require.ErrorIs(t, err, ErrUnsupportedCredentialType)
	}
	_, err := registry.Unmarshal("missing/v1", []byte(`{}`))
	require.ErrorIs(t, err, ErrUnknownCredentialType)
	for _, raw := range []string{`null`, `[]`, `"string"`, `{`, `{"access_key_id": []}`} {
		_, err := registry.Unmarshal("aws", []byte(raw))
		require.Error(t, err, raw)
	}
}
