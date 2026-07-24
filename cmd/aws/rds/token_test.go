package rds

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// initTestIO initializes the IO context for tests that call data.WriteUnmasked().
func initTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to create IO context: %v", err)
	}
	data.InitWriter(ioCtx)
	t.Cleanup(func() { data.Reset() })
}

// mockAuthConfig returns a minimal AtmosConfiguration for testing.
func mockAuthConfig() schema.AtmosConfiguration {
	return schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"dev-admin": {Kind: "aws/user"},
			},
		},
	}
}

// mockAWSCreds returns test AWS credentials.
func mockAWSCreds() *types.AWSCredentials {
	return &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:          "us-east-2",
	}
}

// stubTokenDeps swaps the package-level DI seams and restores them via t.Cleanup.
func stubTokenDeps(t *testing.T, cfgFn func() (schema.AtmosConfiguration, error), authErr error, tokenFn func(endpoint, region, dbUser string) (string, time.Time, error)) {
	t.Helper()
	origInit, origAuth, origToken := initCliConfigFn, authenticateForTokenFn, getRDSTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInit
		authenticateForTokenFn = origAuth
		getRDSTokenFn = origToken
	})
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return cfgFn()
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		if authErr != nil {
			return nil, authErr
		}
		return mockAWSCreds(), nil
	}
	getRDSTokenFn = func(_ context.Context, _ types.ICredentials, endpoint, region, dbUser string) (string, time.Time, error) {
		return tokenFn(endpoint, region, dbUser)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func TestRdsCmd_Structure(t *testing.T) {
	assert.Equal(t, "rds", RdsCmd.Use)
	assert.NotEmpty(t, RdsCmd.Short)
	assert.Nil(t, RdsCmd.Args(RdsCmd, []string{}))
	assert.NotNil(t, RdsCmd.Args(RdsCmd, []string{"extra"}))
}

func TestTokenCmd_Structure(t *testing.T) {
	assert.Equal(t, "token", tokenCmd.Use)
	assert.NotEmpty(t, tokenCmd.Short)
	assert.True(t, tokenCmd.SilenceUsage)
	assert.Nil(t, tokenCmd.Args(tokenCmd, []string{}))
	assert.NotNil(t, tokenCmd.Args(tokenCmd, []string{"extra"}))
}

func TestTokenCmd_ParentIsRdsCmd(t *testing.T) {
	require.NotNil(t, tokenCmd.Parent())
	assert.Equal(t, "rds", tokenCmd.Parent().Name())
}

func TestTokenCmd_HasFlags(t *testing.T) {
	for _, name := range []string{"host", "port", "username", "region", "identity"} {
		assert.NotNil(t, tokenCmd.Flags().Lookup(name), "flag --%s should be registered", name)
	}
	assert.Equal(t, "u", tokenCmd.Flags().Lookup("username").Shorthand)
	assert.Equal(t, "i", tokenCmd.Flags().Lookup("identity").Shorthand)
}

func TestResolveDefaultIdentity(t *testing.T) {
	assert.Equal(t, "", resolveDefaultIdentity(nil))
	assert.Equal(t, "", resolveDefaultIdentity(&schema.AuthConfig{Identities: map[string]schema.Identity{}}))
	assert.Equal(t, "dev-admin", resolveDefaultIdentity(&schema.AuthConfig{
		Identities: map[string]schema.Identity{"dev-admin": {Kind: "aws/user"}},
	}))
	// Multiple identities cannot be auto-selected.
	assert.Equal(t, "", resolveDefaultIdentity(&schema.AuthConfig{
		Identities: map[string]schema.Identity{"a": {}, "b": {}},
	}))
}

func TestRunRDSTokenGeneration_MissingFlags(t *testing.T) {
	tests := []struct {
		name string
		opts tokenOptions
		want string
	}{
		{"missing host", tokenOptions{Port: 5432, Username: "app", Region: "us-east-2"}, "--host"},
		{"missing port", tokenOptions{Host: "db", Username: "app", Region: "us-east-2"}, "--port"},
		{"missing username", tokenOptions{Host: "db", Port: 5432, Region: "us-east-2"}, "--username"},
		{"missing region", tokenOptions{Host: "db", Port: 5432, Username: "app"}, "--region"},
		{"port too high", tokenOptions{Host: "db", Port: 70000, Username: "app", Region: "us-east-2"}, "between 1 and 65535"},
		{"port negative", tokenOptions{Host: "db", Port: -1, Username: "app", Region: "us-east-2"}, "between 1 and 65535"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runRDSTokenGeneration(tt.opts)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrRDSTokenGeneration)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestRunRDSTokenGeneration_Success(t *testing.T) {
	initTestIO(t)
	const wantToken = "mydb.abc123.us-east-2.rds.amazonaws.com:5432/?Action=connect&DBUser=app&X-Amz-Signature=deadbeef"
	stubTokenDeps(t,
		func() (schema.AtmosConfiguration, error) { return mockAuthConfig(), nil },
		nil,
		func(endpoint, region, dbUser string) (string, time.Time, error) {
			assert.Equal(t, "mydb.abc123.us-east-2.rds.amazonaws.com:5432", endpoint)
			assert.Equal(t, "us-east-2", region)
			assert.Equal(t, "app", dbUser)
			return wantToken, time.Now().Add(15 * time.Minute), nil
		},
	)

	var err error
	out := captureStdout(t, func() {
		err = runRDSTokenGeneration(tokenOptions{
			Host: "mydb.abc123.us-east-2.rds.amazonaws.com", Port: 5432, Username: "app", Region: "us-east-2", Identity: "dev-admin",
		})
	})
	require.NoError(t, err)
	// The raw token must be emitted verbatim (unmasked) so it works as a DB password.
	assert.Equal(t, wantToken, out)
}

func TestRunRDSTokenGeneration_AuthError(t *testing.T) {
	stubTokenDeps(t,
		func() (schema.AtmosConfiguration, error) { return mockAuthConfig(), nil },
		errors.New("no identity"),
		func(_, _, _ string) (string, time.Time, error) { return "", time.Time{}, nil },
	)
	err := runRDSTokenGeneration(tokenOptions{Host: "db", Port: 5432, Username: "app", Region: "us-east-2"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSTokenGeneration)
}

func TestRunRDSTokenGeneration_TokenError(t *testing.T) {
	stubTokenDeps(t,
		func() (schema.AtmosConfiguration, error) { return mockAuthConfig(), nil },
		nil,
		func(_, _, _ string) (string, time.Time, error) {
			return "", time.Time{}, errUtils.ErrRDSTokenGeneration
		},
	)
	err := runRDSTokenGeneration(tokenOptions{Host: "db", Port: 5432, Username: "app", Region: "us-east-2"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSTokenGeneration)
}

func TestExecuteTokenCommand_ReadsFlags(t *testing.T) {
	initTestIO(t)
	const wantToken = "db:5432/?Action=connect&DBUser=app&X-Amz-Signature=abc"
	stubTokenDeps(t,
		func() (schema.AtmosConfiguration, error) { return mockAuthConfig(), nil },
		nil,
		func(endpoint, _, _ string) (string, time.Time, error) {
			assert.Equal(t, "db:5432", endpoint)
			return wantToken, time.Now().Add(15 * time.Minute), nil
		},
	)

	cmd := &cobra.Command{Use: "token", RunE: executeTokenCommand}
	rdsTokenParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("host", "db"))
	require.NoError(t, cmd.Flags().Set("port", "5432"))
	require.NoError(t, cmd.Flags().Set("username", "app"))
	require.NoError(t, cmd.Flags().Set("region", "us-east-2"))

	var err error
	out := captureStdout(t, func() { err = executeTokenCommand(cmd, []string{}) })
	require.NoError(t, err)
	assert.Equal(t, wantToken, out)
}

func TestExecuteTokenCommand_ReadsEnvVars(t *testing.T) {
	initTestIO(t)
	// The documented ATMOS_AWS_RDS_* env interface must populate the flags via the "rds" Viper
	// prefix, and a colliding sibling env var (ATMOS_AWS_SECURITY_REGION, bound by `aws security`
	// on the shared global Viper) must NOT leak into --region. This guards the WithViperPrefix fix.
	t.Setenv("ATMOS_AWS_RDS_HOST", "envhost")
	t.Setenv("ATMOS_AWS_RDS_PORT", "5432")
	t.Setenv("ATMOS_AWS_RDS_USERNAME", "envuser")
	t.Setenv("ATMOS_AWS_RDS_REGION", "eu-west-1")
	t.Setenv("ATMOS_AWS_SECURITY_REGION", "us-west-2")

	const wantToken = "envhost:5432/?Action=connect&DBUser=envuser&X-Amz-Signature=abc"
	stubTokenDeps(t,
		func() (schema.AtmosConfiguration, error) { return mockAuthConfig(), nil },
		nil,
		func(endpoint, region, dbUser string) (string, time.Time, error) {
			assert.Equal(t, "envhost:5432", endpoint)
			assert.Equal(t, "eu-west-1", region, "region must come from ATMOS_AWS_RDS_REGION, not the colliding ATMOS_AWS_SECURITY_REGION")
			assert.Equal(t, "envuser", dbUser)
			return wantToken, time.Now().Add(15 * time.Minute), nil
		},
	)

	cmd := &cobra.Command{Use: "token", RunE: executeTokenCommand}
	rdsTokenParser.RegisterFlags(cmd)

	var err error
	out := captureStdout(t, func() { err = executeTokenCommand(cmd, []string{}) })
	require.NoError(t, err)
	assert.Equal(t, wantToken, out)
}

func TestAuthenticateForToken_NoDefaultIdentity(t *testing.T) {
	// No identity given and no single default identity: fails before any real authentication.
	_, err := authenticateForToken(context.Background(), &schema.AuthConfig{}, "", "")
	require.Error(t, err)
}

func TestAuthenticateForToken_UnknownIdentity(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{"dev-admin": {Kind: "aws/user"}},
	}
	// Requesting an identity that is not configured fails fast (no network).
	_, err := authenticateForToken(context.Background(), authConfig, "", "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
}

func TestRDSTokenErrors(t *testing.T) {
	assert.NotNil(t, errUtils.ErrRDSTokenGeneration)
}
