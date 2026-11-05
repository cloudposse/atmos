package credential

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestOIDCRejectsRefreshTokensAndInvalidLifetime(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(time.Hour).UTC()
	for _, name := range []string{"refresh token", "extended lifetime", "missing access token", "unaccepted type", "expired"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fields := map[string]any{"issuer": "https://issuer.example", "subject": "subject", "client_id": "client", "access_token": "access", "token_type": "Bearer", "access_token_expires_at": future.Format(time.RFC3339Nano)}
			envelope := &pluginv1.Credentials{Type: "oidc/v1", ExpiresAt: timestamppb.New(future)}
			allowed := []string{"oidc/v1"}
			switch name {
			case "refresh token":
				fields["refresh_token"] = "private"
			case "extended lifetime":
				envelope.ExpiresAt = timestamppb.New(future.Add(time.Minute))
			case "missing access token":
				delete(fields, "access_token")
			case "unaccepted type":
				allowed = []string{"aws/v1"}
			case "expired":
				envelope.ExpiresAt = timestamppb.New(time.Now().Add(-time.Minute))
			}
			var err error
			envelope.Data, err = structpb.NewStruct(fields)
			require.NoError(t, err)
			_, err = New().Decode(envelope, allowed)
			require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
		})
	}
}

func TestOIDCCredentialsAlwaysMarkTokensSensitive(t *testing.T) {
	t.Parallel()
	expiry := time.Now().Add(time.Hour).UTC()
	fields, err := structpb.NewStruct(map[string]any{"issuer": "https://issuer.example", "subject": "subject", "client_id": "client", "access_token": "access", "token_type": "Bearer", "access_token_expires_at": expiry.Format(time.RFC3339Nano)})
	require.NoError(t, err)
	value, err := New().Decode(&pluginv1.Credentials{Type: "oidc/v1", Data: fields, ExpiresAt: timestamppb.New(expiry)}, []string{"oidc/v1"})
	require.NoError(t, err)
	decoded := value.(*types.PluginCredentials)
	require.ElementsMatch(t, []string{"access_token", "id_token"}, decoded.SensitiveKeys)
	require.Equal(t, "subject", decoded.Data["subject"])
}

func TestAWSCredentialsCannotExtendEnvelopeLifetime(t *testing.T) {
	t.Parallel()
	expiry := time.Now().Add(time.Hour).Truncate(time.Second).UTC()
	fields, err := structpb.NewStruct(map[string]any{"access_key_id": "fixture", "secret_access_key": "secret", "session_token": "session", "expiration": expiry.Add(time.Hour).Format(time.RFC3339)})
	require.NoError(t, err)
	value, err := New().Decode(&pluginv1.Credentials{Type: "aws/v1", Data: fields, ExpiresAt: timestamppb.New(expiry)}, []string{"aws/v1"})
	require.NoError(t, err)
	require.Equal(t, expiry.Format(time.RFC3339), value.(*types.AWSCredentials).Expiration)
}
