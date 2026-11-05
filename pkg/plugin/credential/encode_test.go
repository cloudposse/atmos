package credential

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestCredentialCodecsCanBeRegisteredWithoutReplacingExistingOnes(t *testing.T) {
	t.Parallel()
	var registry Registry
	expiry := time.Now().Add(time.Hour)
	expected := &types.PluginCredentials{Type: "custom/v1", ExpiresAt: expiry, Data: map[string]any{"session": "fixture"}}
	decoder := func(value *pluginv1.Credentials) (types.ICredentials, error) {
		require.Equal(t, "custom/v1", value.Type)
		return expected, nil
	}
	require.ErrorIs(t, registry.Register("", decoder), errUtils.ErrInvalidCredentialCodec)
	require.ErrorIs(t, registry.Register("custom/v1", nil), errUtils.ErrInvalidCredentialCodec)
	require.NoError(t, registry.Register("custom/v1", decoder))
	require.ErrorIs(t, registry.Register("custom/v1", decoder), errUtils.ErrDuplicateCredentialCodec)
	require.ErrorIs(t, registry.RegisterEncoder(nil, encodePlugin), errUtils.ErrInvalidCredentialCodec)
	require.ErrorIs(t, registry.RegisterEncoder(expected, nil), errUtils.ErrInvalidCredentialCodec)
	require.NoError(t, registry.RegisterEncoder(expected, encodePlugin))
	require.ErrorIs(t, registry.RegisterEncoder(expected, encodePlugin), errUtils.ErrDuplicateCredentialCodec)
	encoded, err := registry.Encode(expected, []string{"custom/v1"})
	require.NoError(t, err)
	require.Equal(t, expected.Data, encoded.Data.AsMap())
	decoded, err := registry.Decode(encoded, []string{"custom/v1"})
	require.NoError(t, err)
	require.Same(t, expected, decoded)
}

func TestEncodeRejectsUnavailableOrUnacceptedContracts(t *testing.T) {
	t.Parallel()
	registry := New()
	var missing *types.AWSCredentials
	for _, value := range []types.ICredentials{nil, missing} {
		_, err := registry.Encode(value, []string{"aws/v1"})
		require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	}
	empty := &Registry{}
	_, err := empty.Encode(&types.AWSCredentials{}, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidCredentialCodec)
	expectedError := errors.New("codec failed")
	require.NoError(t, empty.RegisterEncoder(&types.AWSCredentials{}, func(types.ICredentials) (*pluginv1.Credentials, error) { return nil, expectedError }))
	_, err = empty.Encode(&types.AWSCredentials{}, nil)
	require.ErrorIs(t, err, expectedError)
	value := &types.AWSCredentials{AccessKeyID: "key", SecretAccessKey: "secret", SessionToken: "session", Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	_, err = registry.Encode(value, []string{"oidc/v1"})
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	encoded, err := registry.Encode(value, []string{"aws/v1"})
	require.NoError(t, err)
	require.Equal(t, "aws/v1", encoded.Type)
	require.Equal(t, "key", encoded.Data.AsMap()["access_key_id"])
	require.ElementsMatch(t, []string{"access_key_id", "secret_access_key", "session_token"}, encoded.SensitiveKeys)
	value.Expiration = "invalid"
	_, err = registry.Encode(value, []string{"aws/v1"})
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	_, err = encodePlugin(value)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	_, err = encodePlugin(&types.PluginCredentials{Data: map[string]any{"unsupported": make(chan bool)}})
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
}

func TestDecodeRejectsInvalidEnvelopesAndAWSFields(t *testing.T) {
	t.Parallel()
	expiry := time.Now().Add(time.Hour).UTC()
	fields, err := structpb.NewStruct(map[string]any{"access_key_id": "key", "secret_access_key": "secret", "session_token": "session", "expiration": expiry.Format(time.RFC3339)})
	require.NoError(t, err)
	for _, envelope := range []*pluginv1.Credentials{nil, {Type: "aws/v1", ExpiresAt: timestamppb.New(expiry)}, {Type: "aws/v1", Data: fields}, {Type: "aws/v1", Data: fields, ExpiresAt: &timestamppb.Timestamp{Seconds: 1 << 62}}} {
		_, err := New().Decode(envelope, []string{"aws/v1"})
		require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	}
	_, err = New().Decode(&pluginv1.Credentials{Type: "unknown/v1", Data: fields, ExpiresAt: timestamppb.New(expiry)}, []string{"unknown/v1"})
	require.ErrorIs(t, err, errUtils.ErrInvalidCredentialCodec)
	for _, test := range []struct {
		field string
		value any
	}{
		{"access_key_id", ""}, {"secret_access_key", true}, {"expiration", "invalid"}, {"expiration", time.Now().Add(-time.Hour).Format(time.RFC3339)},
	} {
		data := fields.AsMap()
		data[test.field] = test.value
		wire, err := structpb.NewStruct(data)
		require.NoError(t, err)
		_, err = New().Decode(&pluginv1.Credentials{Type: "aws/v1", Data: wire, ExpiresAt: timestamppb.New(expiry)}, []string{"aws/v1"})
		require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	}
}
