package rds

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// capturedExec records what the (stubbed) client runner was asked to execute.
type capturedExec struct {
	called bool
	args   []string
	env    []string
}

// stubConnectDeps swaps the connect command's DI seams and restores them via t.Cleanup.
func stubConnectDeps(t *testing.T, cfg schema.AtmosConfiguration, token string, cap *capturedExec) {
	t.Helper()
	origInit, origAuth, origToken, origRun, origCA := initCliConfigFn, authenticateForTokenFn, getRDSTokenFn, runClientFn, findCABundleFn
	t.Cleanup(func() {
		initCliConfigFn, authenticateForTokenFn, getRDSTokenFn, runClientFn, findCABundleFn = origInit, origAuth, origToken, origRun, origCA
	})
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) { return cfg, nil }
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return &types.AWSCredentials{AccessKeyID: "AKIAIOSFODNN7EXAMPLE", SecretAccessKey: "x", Region: "us-east-2"}, nil
	}
	getRDSTokenFn = func(_ context.Context, _ types.ICredentials, _, _, _ string) (string, time.Time, error) {
		return token, time.Now().Add(15 * time.Minute), nil
	}
	runClientFn = func(args, env []string) error {
		cap.called = true
		cap.args = args
		cap.env = env
		return nil
	}
	findCABundleFn = func() string { return "/etc/ssl/cert.pem" }
}

func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			return strings.TrimPrefix(kv, key+"="), true
		}
	}
	return "", false
}

// A realistic RDS presigned token embeds the AKIA access-key-id — it must never reach argv.
const connectToken = "mydb.abc123.us-east-2.rds.amazonaws.com:5432/?Action=connect&DBUser=app&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20260724%2Fus-east-2%2Frds-db%2Faws4_request&X-Amz-Signature=deadbeef"

func TestRunRDSConnect_PostgresTokenInEnvNotArgv(t *testing.T) {
	var cap capturedExec
	stubConnectDeps(t, mockAuthConfig(), connectToken, &cap)

	err := runRDSConnect(connectOptions{
		Host: "mydb.abc123.us-east-2.rds.amazonaws.com", Port: 5432, Username: "app", Region: "us-east-2", Identity: "dev-admin",
	})
	require.NoError(t, err)
	require.True(t, cap.called)

	assert.Equal(t, "psql", cap.args[0])
	joined := strings.Join(cap.args, " ")
	assert.Contains(t, joined, "--host=mydb.abc123.us-east-2.rds.amazonaws.com")
	assert.Contains(t, joined, "--port=5432")
	assert.Contains(t, joined, "--username=app")
	assert.NotContains(t, joined, connectToken, "the token must NEVER appear in argv")
	assert.NotContains(t, joined, "AKIAIOSFODNN7EXAMPLE", "no credential material in argv")

	pw, ok := envValue(cap.env, "PGPASSWORD")
	require.True(t, ok, "token must be passed via PGPASSWORD env")
	assert.Equal(t, connectToken, pw)
	mode, _ := envValue(cap.env, "PGSSLMODE")
	assert.Equal(t, "verify-full", mode)
	ca, _ := envValue(cap.env, "PGSSLROOTCERT")
	assert.Equal(t, "/etc/ssl/cert.pem", ca)
}

func TestRunRDSConnect_MysqlTokenInEnvNotArgv(t *testing.T) {
	var cap capturedExec
	stubConnectDeps(t, mockAuthConfig(), connectToken, &cap)

	err := runRDSConnect(connectOptions{
		Host: "mydb", Port: 3306, Username: "app", Region: "us-east-2", Identity: "dev-admin",
	})
	require.NoError(t, err)
	require.True(t, cap.called)

	assert.Equal(t, "mysql", cap.args[0])
	joined := strings.Join(cap.args, " ")
	assert.Contains(t, joined, "--user=app")
	assert.Contains(t, joined, "--enable-cleartext-plugin")
	assert.Contains(t, joined, "--ssl-mode=VERIFY_IDENTITY")
	assert.Contains(t, joined, "--ssl-ca=/etc/ssl/cert.pem")
	assert.NotContains(t, joined, connectToken, "the token must NEVER appear in argv")

	pw, ok := envValue(cap.env, "MYSQL_PWD")
	require.True(t, ok, "token must be passed via MYSQL_PWD env")
	assert.Equal(t, connectToken, pw)
}

func TestResolveEngine(t *testing.T) {
	pg, err := resolveEngine("", 5432)
	require.NoError(t, err)
	assert.Equal(t, "psql", pg.bin)

	my, err := resolveEngine("", 3306)
	require.NoError(t, err)
	assert.Equal(t, "mysql", my.bin)

	// Explicit engine wins over a non-standard port.
	ex, err := resolveEngine("postgres", 9999)
	require.NoError(t, err)
	assert.Equal(t, "psql", ex.bin)

	_, err = resolveEngine("", 9999)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSEngineUnknown)

	_, err = resolveEngine("oracle", 1521)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSEngineUnknown)
}

func TestRunRDSConnect_NamedIntegration(t *testing.T) {
	cfg := mockAuthConfig()
	cfg.Auth.Integrations = map[string]schema.Integration{
		"my-db": {
			Kind: "aws/rds",
			Via:  &schema.IntegrationVia{Identity: "dev-admin"},
			Spec: &schema.IntegrationSpec{Database: &schema.RDSDatabase{
				Host: "int-host", Port: 5432, Username: "intuser", Region: "eu-west-1", Engine: "postgres",
			}},
		},
	}
	var cap capturedExec
	stubConnectDeps(t, cfg, connectToken, &cap)

	require.NoError(t, runRDSConnect(connectOptions{Integration: "my-db"}))
	joined := strings.Join(cap.args, " ")
	assert.Contains(t, joined, "--host=int-host")
	assert.Contains(t, joined, "--username=intuser")
}

func TestRunRDSConnect_IntegrationErrors(t *testing.T) {
	cfg := mockAuthConfig()
	cfg.Auth.Integrations = map[string]schema.Integration{
		"wrong-kind": {Kind: "aws/ecr"},
	}
	var cap capturedExec
	stubConnectDeps(t, cfg, connectToken, &cap)

	// Unknown integration name.
	err := runRDSConnect(connectOptions{Integration: "nope"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSIntegrationConfig)

	// Wrong kind.
	err = runRDSConnect(connectOptions{Integration: "wrong-kind"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSIntegrationConfig)

	assert.False(t, cap.called, "no client should be executed on a config error")
}

func TestRunRDSConnect_PrintCommand(t *testing.T) {
	var cap capturedExec
	stubConnectDeps(t, mockAuthConfig(), connectToken, &cap)
	// print-command minting would call getRDSTokenFn; detect it.
	minted := false
	getRDSTokenFn = func(_ context.Context, _ types.ICredentials, _, _, _ string) (string, time.Time, error) {
		minted = true
		return connectToken, time.Now(), nil
	}

	err := runRDSConnect(connectOptions{
		Host: "h", Port: 5432, Username: "app", Region: "us-east-2", Identity: "dev-admin", PrintCommand: true,
	})
	require.NoError(t, err)
	assert.False(t, cap.called, "print-command must not exec the client")
	assert.False(t, minted, "print-command must not mint a live token")
}

func TestValidateConnectOptions(t *testing.T) {
	tests := []struct {
		name string
		opts connectOptions
		want string
	}{
		{"missing host", connectOptions{Port: 5432, Username: "app", Region: "us-east-2"}, "--host"},
		{"missing port", connectOptions{Host: "h", Username: "app", Region: "us-east-2"}, "--port"},
		{"port too high", connectOptions{Host: "h", Port: 70000, Username: "app", Region: "us-east-2"}, "between 1 and 65535"},
		{"missing username", connectOptions{Host: "h", Port: 5432, Region: "us-east-2"}, "--username"},
		{"missing region", connectOptions{Host: "h", Port: 5432, Username: "app"}, "--region"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnectOptions(tt.opts)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrRDSConnectFailed)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestConnectCmd_Structure(t *testing.T) {
	assert.Equal(t, "connect [integration]", connectCmd.Use)
	assert.True(t, connectCmd.SilenceUsage)
	require.NotNil(t, connectCmd.Parent())
	assert.Equal(t, "rds", connectCmd.Parent().Name())
	for _, name := range []string{"host", "port", "username", "region", "engine", "database", "ca-bundle", "print-command", "identity"} {
		assert.NotNil(t, connectCmd.Flags().Lookup(name), "flag --%s should be registered", name)
	}
}
