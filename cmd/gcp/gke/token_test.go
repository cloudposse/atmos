package gke

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func initTokenTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
}

func newTestTokenCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "token", RunE: executeTokenCommand}
	cmd.SetContext(context.Background())
	cmd.Flags().StringP("identity", "i", "", "Atmos GCP identity")
	return cmd
}

func testAtmosConfig() schema.AtmosConfiguration {
	return schema.AtmosConfiguration{Auth: schema.AuthConfig{
		Identities: map[string]schema.Identity{"example-deployer": {Kind: "gcp/service-account"}},
	}}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = original })

	callErr := fn()
	require.NoError(t, writer.Close())
	os.Stdout = original
	output, readErr := io.ReadAll(reader)
	require.NoError(t, readErr)
	require.NoError(t, reader.Close())
	return string(output), callErr
}

func installTokenCommandFakes(t *testing.T) {
	t.Helper()
	originalInit := initCliConfigFn
	originalAuth := authenticateForTokenFn
	originalGetToken := getGKETokenFn
	originalManager := newAuthManagerFn
	t.Cleanup(func() {
		initCliConfigFn = originalInit
		authenticateForTokenFn = originalAuth
		getGKETokenFn = originalGetToken
		newAuthManagerFn = originalManager
	})
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return testAtmosConfig(), nil
	}
}

func TestTokenCommandShape(t *testing.T) {
	assert.Equal(t, "token", tokenCmd.Use)
	assert.Contains(t, tokenCmd.Short, "GKE bearer token")
	assert.True(t, tokenCmd.SilenceUsage)
	assert.Equal(t, "gke", tokenCmd.Parent().Name())
	identityFlag := tokenCmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Nil(t, tokenCmd.Args(tokenCmd, nil))
	assert.Error(t, tokenCmd.Args(tokenCmd, []string{"unexpected"}))
}

func TestExecuteTokenCommandOutputsOnlyExecCredentialJSON(t *testing.T) {
	initTokenTestIO(t)
	installTokenCommandFakes(t)
	expiry := time.Date(2026, 8, 7, 12, 30, 0, 0, time.UTC)
	authenticateForTokenFn = func(ctx context.Context, _ *schema.AuthConfig, _, identity string) (types.ICredentials, error) {
		assert.True(t, auth.IntegrationsSkipped(ctx), "token authentication must suppress integration recursion")
		assert.Equal(t, "example-deployer", identity)
		return &types.GCPCredentials{AccessToken: "example-access-token", TokenExpiry: expiry}, nil
	}
	getGKETokenFn = func(creds types.ICredentials) (string, time.Time, error) {
		return creds.(*types.GCPCredentials).AccessToken, expiry, nil
	}

	cmd := newTestTokenCommand()
	require.NoError(t, cmd.Flags().Set("identity", "example-deployer"))
	stdout, err := captureStdout(t, func() error { return executeTokenCommand(cmd, nil) })
	require.NoError(t, err)
	assert.Equal(t, `{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"expirationTimestamp":"2026-08-07T12:30:00Z","token":"example-access-token"}}`, stdout)

	var credential execCredential
	require.NoError(t, json.Unmarshal([]byte(stdout), &credential))
	assert.Equal(t, "example-access-token", credential.Status.Token)
	assert.Equal(t, "2026-08-07T12:30:00Z", credential.Status.ExpirationTimestamp)
}

func TestExecuteTokenCommandUsesIdentityEnvironment(t *testing.T) {
	initTokenTestIO(t)
	installTokenCommandFakes(t)
	t.Setenv("ATMOS_IDENTITY", "example-deployer")
	var capturedIdentity string
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, identity string) (types.ICredentials, error) {
		capturedIdentity = identity
		return &types.GCPCredentials{AccessToken: "example-access-token", TokenExpiry: time.Now().Add(time.Hour)}, nil
	}
	getGKETokenFn = func(_ types.ICredentials) (string, time.Time, error) {
		return "example-access-token", time.Time{}, nil
	}

	stdout, err := captureStdout(t, func() error { return executeTokenCommand(newTestTokenCommand(), nil) })
	require.NoError(t, err)
	assert.Equal(t, "example-deployer", capturedIdentity)
	assert.NotContains(t, stdout, "expirationTimestamp")
}

func TestExecuteTokenCommandPreservesCommandContext(t *testing.T) {
	initTokenTestIO(t)
	installTokenCommandFakes(t)

	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	cancel()

	authenticateForTokenFn = func(got context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		gotDeadline, ok := got.Deadline()
		require.True(t, ok)
		assert.Equal(t, deadline, gotDeadline)
		assert.ErrorIs(t, got.Err(), context.Canceled)
		assert.True(t, auth.IntegrationsSkipped(got))
		return nil, got.Err()
	}

	cmd := newTestTokenCommand()
	cmd.SetContext(ctx)
	_, err := captureStdout(t, func() error { return executeTokenCommand(cmd, nil) })
	require.ErrorIs(t, err, context.Canceled)
}

func TestExecuteTokenCommandErrorsNeverExposeToken(t *testing.T) {
	initTokenTestIO(t)
	installTokenCommandFakes(t)
	const sensitiveToken = "sensitive-token-must-not-leak"
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return &types.GCPCredentials{AccessToken: sensitiveToken, TokenExpiry: time.Now().Add(-time.Hour)}, nil
	}
	getGKETokenFn = func(creds types.ICredentials) (string, time.Time, error) {
		gcpCreds := creds.(*types.GCPCredentials)
		if gcpCreds.IsExpired() {
			return "", time.Time{}, errors.New("resolved GCP credentials are expired")
		}
		return gcpCreds.AccessToken, gcpCreds.TokenExpiry, nil
	}

	stdout, err := captureStdout(t, func() error { return executeTokenCommand(newTestTokenCommand(), nil) })
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.NotContains(t, err.Error(), sensitiveToken)
	assert.ErrorIs(t, err, errUtils.ErrGKETokenGeneration)
}

func TestExecuteTokenCommandConfigAndAuthenticationErrors(t *testing.T) {
	initTokenTestIO(t)
	installTokenCommandFakes(t)

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errors.New("config unavailable")
	}
	stdout, err := captureStdout(t, func() error { return executeTokenCommand(newTestTokenCommand(), nil) })
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return testAtmosConfig(), nil
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return nil, errors.New("identity credentials unavailable")
	}
	stdout, err = captureStdout(t, func() error { return executeTokenCommand(newTestTokenCommand(), nil) })
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.ErrorIs(t, err, errUtils.ErrGKETokenGeneration)
}

func TestAuthenticateForTokenMissingIdentity(t *testing.T) {
	_, err := authenticateForToken(t.Context(), &schema.AuthConfig{Identities: map[string]schema.Identity{}}, "", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGKETokenGeneration)
	assert.Contains(t, err.Error(), "no identity specified")
}

func TestAuthenticateForTokenRefreshesThroughAuthManager(t *testing.T) {
	installTokenCommandFakes(t)
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	fresh := &types.GCPCredentials{AccessToken: "fresh-access-token", TokenExpiry: time.Now().Add(time.Hour)}
	mgr.EXPECT().Authenticate(gomock.Any(), "example-deployer").DoAndReturn(
		func(ctx context.Context, _ string) (*types.WhoamiInfo, error) {
			assert.True(t, auth.IntegrationsSkipped(ctx))
			return &types.WhoamiInfo{Credentials: fresh}, nil
		},
	)
	newAuthManagerFn = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return mgr, nil
	}

	creds, err := authenticateForToken(
		auth.ContextWithSkipIntegrations(t.Context()),
		&schema.AuthConfig{Identities: map[string]schema.Identity{"example-deployer": {Kind: "gcp/service-account"}}},
		"",
		"example-deployer",
	)
	require.NoError(t, err)
	assert.Same(t, fresh, creds)
}

func TestAuthenticateForTokenRejectsMissingAndWrongCredentials(t *testing.T) {
	for _, tt := range []struct {
		name  string
		creds types.ICredentials
	}{
		{name: "missing credentials"},
		{name: "wrong credentials", creds: &types.AWSCredentials{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			installTokenCommandFakes(t)
			ctrl := gomock.NewController(t)
			mgr := types.NewMockAuthManager(ctrl)
			mgr.EXPECT().Authenticate(gomock.Any(), "example-deployer").Return(&types.WhoamiInfo{Credentials: tt.creds}, nil)
			newAuthManagerFn = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return mgr, nil
			}
			_, err := authenticateForToken(t.Context(), &schema.AuthConfig{}, "", "example-deployer")
			require.Error(t, err)
			if tt.creds == nil {
				assert.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
			} else {
				assert.ErrorIs(t, err, errUtils.ErrGKETokenGeneration)
			}
		})
	}
}
