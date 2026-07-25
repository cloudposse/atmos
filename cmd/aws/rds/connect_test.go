package rds

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/cacerts"
	rdsca "github.com/cloudposse/atmos/pkg/cacerts/rds"
	"github.com/cloudposse/atmos/pkg/schema"
)

// capturedExec records what the (stubbed) client runner was asked to execute.
type capturedExec struct {
	called bool
	args   []string
	env    []string
}

// stubConnectDeps swaps the connect command's DI seams and restores them via t.Cleanup. The minted
// token is always connectToken (no test needs a different value), so it is not a parameter.
func stubConnectDeps(t *testing.T, cfg *schema.AtmosConfiguration, cap *capturedExec) {
	t.Helper()
	origInit, origAuth, origToken, origRun, origCA := initCliConfigFn, authenticateForTokenFn, getRDSTokenFn, runClientFn, buildCABundleFn
	t.Cleanup(func() {
		initCliConfigFn, authenticateForTokenFn, getRDSTokenFn, runClientFn, buildCABundleFn = origInit, origAuth, origToken, origRun, origCA
	})
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) { return *cfg, nil }
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return &types.AWSCredentials{AccessKeyID: "AKIAIOSFODNN7EXAMPLE", SecretAccessKey: "x", Region: "us-east-2"}, nil
	}
	getRDSTokenFn = func(_ context.Context, _ types.ICredentials, _, _, _ string) (string, time.Time, error) {
		return connectToken, time.Now().Add(15 * time.Minute), nil
	}
	runClientFn = func(args, env []string) error {
		cap.called = true
		cap.args = args
		cap.env = env
		return nil
	}
	buildCABundleFn = func(_ []byte) (string, error) { return "/etc/ssl/cert.pem", nil }
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
	cfg := mockAuthConfig()
	stubConnectDeps(t, &cfg, &cap)

	err := runRDSConnect(&connectOptions{
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
	cfg := mockAuthConfig()
	stubConnectDeps(t, &cfg, &cap)

	err := runRDSConnect(&connectOptions{
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

// assertEnvKeyExactlyOnce asserts key appears EXACTLY ONCE in env and that its (single) value
// equals want. A first-match helper like envValue is NOT enough here: it would silently pass even
// if stripEnvKeys were deleted and a hostile duplicate sat earlier in the slice while ours sat
// later — Go's os/exec keeps the LAST duplicate of a key, so a naive first-match assertion can
// pass while the actual child process would still see the hostile value. Counting occurrences is
// the part that actually pins the fix down.
func assertEnvKeyExactlyOnce(t *testing.T, env []string, key, want string) {
	t.Helper()
	var matches []string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			matches = append(matches, v)
		}
	}
	require.Len(t, matches, 1, "%s must appear exactly once in the child env, got %v", key, matches)
	assert.Equal(t, want, matches[0])
}

// TestBuildClientEnv_PostgresStripsHostileOverrides guards the Fix-2/3 env-authoritative hardening:
// a hostile/stale atmos.yaml `env:` (globalEnv here) declaring the SAME keys we inject must never
// survive into the child env. Deleting the stripEnvKeys call in buildClientEnv would leave every
// other test in this file green (they all use a nil .Env), so this is the only test that actually
// pins that regression down.
func TestBuildClientEnv_PostgresStripsHostileOverrides(t *testing.T) {
	spec := engineSpecs[enginePostgres]
	hostileEnv := map[string]string{
		"PGSSLMODE":     "disable",
		"PGPASSWORD":    "attacker-supplied-value",
		"PGSSLROOTCERT": "/bad/attacker-path",
	}

	env := buildClientEnv(spec, connectToken, "/etc/ssl/cert.pem", hostileEnv)

	assertEnvKeyExactlyOnce(t, env, "PGSSLMODE", "verify-full")
	assertEnvKeyExactlyOnce(t, env, "PGPASSWORD", connectToken)
	assertEnvKeyExactlyOnce(t, env, "PGSSLROOTCERT", "/etc/ssl/cert.pem")
}

// TestBuildClientEnv_MysqlStripsHostileOverride is the mysql counterpart: a hostile MYSQL_PWD in
// globalEnv must not survive alongside (or instead of) the real token.
func TestBuildClientEnv_MysqlStripsHostileOverride(t *testing.T) {
	spec := engineSpecs[engineMySQL]
	hostileEnv := map[string]string{"MYSQL_PWD": "attacker-supplied-value"}

	env := buildClientEnv(spec, connectToken, "", hostileEnv)

	assertEnvKeyExactlyOnce(t, env, "MYSQL_PWD", connectToken)
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
	stubConnectDeps(t, &cfg, &cap)

	require.NoError(t, runRDSConnect(&connectOptions{Integration: "my-db"}))
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
	stubConnectDeps(t, &cfg, &cap)

	// Unknown integration name.
	err := runRDSConnect(&connectOptions{Integration: "nope"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSIntegrationConfig)

	// Wrong kind.
	err = runRDSConnect(&connectOptions{Integration: "wrong-kind"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSIntegrationConfig)

	assert.False(t, cap.called, "no client should be executed on a config error")
}

// TestRunRDSConnect_IntegrationSpecWithoutDatabase covers the branch where an aws/rds integration
// has a non-nil Spec but a nil Spec.Database — previously untested (the "wrong-kind" case above
// fails on the Kind check first and never reaches this branch).
func TestRunRDSConnect_IntegrationSpecWithoutDatabase(t *testing.T) {
	cfg := mockAuthConfig()
	cfg.Auth.Integrations = map[string]schema.Integration{
		"no-db": {Kind: "aws/rds", Spec: &schema.IntegrationSpec{}},
	}
	var cap capturedExec
	stubConnectDeps(t, &cfg, &cap)

	err := runRDSConnect(&connectOptions{Integration: "no-db"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSIntegrationConfig)
	assert.False(t, cap.called)
}

// TestRunRDSConnect_FlagsOverrideIntegration guards the other direction of applyIntegrationDefaults:
// explicit flags must win over a named integration's declared values, not just fill gaps in them
// (TestRunRDSConnect_NamedIntegration already covers fill-from-integration).
func TestRunRDSConnect_FlagsOverrideIntegration(t *testing.T) {
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
	stubConnectDeps(t, &cfg, &cap)

	var gotRegion string
	getRDSTokenFn = func(_ context.Context, _ types.ICredentials, _, region, _ string) (string, time.Time, error) {
		gotRegion = region
		return connectToken, time.Now().Add(15 * time.Minute), nil
	}

	require.NoError(t, runRDSConnect(&connectOptions{
		Integration: "my-db",
		Host:        "flag-host",
		Username:    "flaguser",
		Region:      "us-west-2",
	}))
	joined := strings.Join(cap.args, " ")
	assert.Contains(t, joined, "--host=flag-host", "explicit --host must win over the integration's declared host")
	assert.Contains(t, joined, "--username=flaguser", "explicit --username must win over the integration's declared username")
	assert.Equal(t, "us-west-2", gotRegion, "explicit --region must win over the integration's declared region")
}

// TestRunRDSConnect_RunnerErrorPropagates guards that runClientFn's error (carrying, e.g.,
// shell.ErrCommandNotFound and its exit code) is returned verbatim, not swallowed or rewrapped.
func TestRunRDSConnect_RunnerErrorPropagates(t *testing.T) {
	var cap capturedExec
	cfg := mockAuthConfig()
	stubConnectDeps(t, &cfg, &cap)

	sentinelErr := errors.New("client not found on PATH")
	runClientFn = func(args, env []string) error {
		cap.called = true
		cap.args = args
		cap.env = env
		return sentinelErr
	}

	err := runRDSConnect(&connectOptions{
		Host: "mydb", Port: 5432, Username: "app", Region: "us-east-2", Identity: "dev-admin",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
	assert.True(t, cap.called)
}

// TestRunRDSConnect_AuthFailureWrapped guards that an authentication failure is wrapped as
// ErrRDSConnectFailed (so callers can errors.Is-match on it) and that the client is never
// executed once auth has failed.
func TestRunRDSConnect_AuthFailureWrapped(t *testing.T) {
	var cap capturedExec
	cfg := mockAuthConfig()
	stubConnectDeps(t, &cfg, &cap)

	authErr := errors.New("no credentials available")
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return nil, authErr
	}

	err := runRDSConnect(&connectOptions{
		Host: "mydb", Port: 5432, Username: "app", Region: "us-east-2", Identity: "dev-admin",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSConnectFailed)
	assert.ErrorIs(t, err, authErr)
	assert.False(t, cap.called, "no client should be executed when auth fails")
}

// TestRunRDSConnect_TokenMintFailureAbortsBeforeRun guards that a token-minting failure aborts
// BEFORE the client is executed — a live client session must never start without a valid token.
func TestRunRDSConnect_TokenMintFailureAbortsBeforeRun(t *testing.T) {
	var cap capturedExec
	cfg := mockAuthConfig()
	stubConnectDeps(t, &cfg, &cap)

	mintErr := errors.New("token minting failed")
	getRDSTokenFn = func(_ context.Context, _ types.ICredentials, _, _, _ string) (string, time.Time, error) {
		return "", time.Time{}, mintErr
	}

	err := runRDSConnect(&connectOptions{
		Host: "mydb", Port: 5432, Username: "app", Region: "us-east-2", Identity: "dev-admin",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, mintErr)
	assert.False(t, cap.called, "no client should be executed when token minting fails")
}

// TestRunRDSConnect_DefaultCABundleEmbedsRDSCert guards Fix 1: with no --ca-bundle given, the
// default CA bundle must embed the Amazon RDS CA bundle (not just the bare host system store),
// since the system store never carries Amazon's private RDS root CAs. This exercises the REAL
// cacerts.BuildBundle (not the fixed-path stub the other tests use), so it restores buildCABundleFn
// to production right after stubConnectDeps.
func TestRunRDSConnect_DefaultCABundleEmbedsRDSCert(t *testing.T) {
	// Isolate the XDG cache dir so this test never touches (or depends on) the real user cache.
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())

	var cap capturedExec
	cfg := mockAuthConfig()
	stubConnectDeps(t, &cfg, &cap)
	buildCABundleFn = cacerts.BuildBundle // exercise the real default-CA resolution path.

	err := runRDSConnect(&connectOptions{
		Host: "mydb.abc123.us-east-2.rds.amazonaws.com", Port: 5432, Username: "app", Region: "us-east-2", Identity: "dev-admin",
	})
	require.NoError(t, err)
	require.True(t, cap.called)

	ca, ok := envValue(cap.env, "PGSSLROOTCERT")
	require.True(t, ok, "PGSSLROOTCERT must be set from the default CA bundle")
	require.NotEmpty(t, ca)

	contents, readErr := os.ReadFile(ca)
	require.NoError(t, readErr)
	assert.Contains(t, string(contents), string(rdsca.Bundle()), "the default bundle must embed the Amazon RDS CA bundle")
}

func TestRunRDSConnect_PrintCommand(t *testing.T) {
	initTestIO(t)
	var cap capturedExec
	cfg := mockAuthConfig()
	stubConnectDeps(t, &cfg, &cap)
	// print-command minting would call getRDSTokenFn; detect it.
	minted := false
	getRDSTokenFn = func(_ context.Context, _ types.ICredentials, _, _, _ string) (string, time.Time, error) {
		minted = true
		return connectToken, time.Now(), nil
	}

	var err error
	out := captureStdout(t, func() {
		err = runRDSConnect(&connectOptions{
			Host: "h", Port: 5432, Username: "app", Region: "us-east-2", Identity: "dev-admin", PrintCommand: true,
		})
	})
	require.NoError(t, err)
	assert.False(t, cap.called, "print-command must not exec the client")
	assert.False(t, minted, "print-command must not mint a live token")

	// The output must go to the DATA channel (stdout), not the UI channel (stderr) — piping the
	// printed command to a file previously yielded an empty file.
	assert.Contains(t, out, "psql")
	// The printed command must be faithful to the real connection: a copied command previously
	// omitted PGSSLMODE/PGSSLROOTCERT, so it would silently connect WITHOUT verify-full.
	assert.Contains(t, out, "PGPASSWORD=<token>")
	assert.Contains(t, out, "PGSSLMODE=verify-full")
	assert.Contains(t, out, "PGSSLROOTCERT=/etc/ssl/cert.pem")
	assert.NotContains(t, out, connectToken, "the live token must never be printed, only the redacted placeholder")
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnectOptions(&tt.opts)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrRDSConnectFailed)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// TestValidateConnectOptions_RegionOptional guards the Fix-4 region fallback: schema.RDSDatabase.Region
// is documented as optional (defaulting to the identity's credential region), and GetRDSToken
// implements that fallback. A hard rejection here would run BEFORE that fallback ever gets a
// chance to resolve it, so an empty region must NOT fail validation.
func TestValidateConnectOptions_RegionOptional(t *testing.T) {
	err := validateConnectOptions(&connectOptions{Host: "h", Port: 5432, Username: "app"})
	assert.NoError(t, err)
}

// TestRunRDSConnect_EmptyRegionFallsThroughToTokenMint is the end-to-end guard for the same fix:
// an empty --region must flow all the way through to getRDSTokenFn (GetRDSToken), which resolves
// it from the identity's credentials, rather than being rejected earlier by validation.
func TestRunRDSConnect_EmptyRegionFallsThroughToTokenMint(t *testing.T) {
	var cap capturedExec
	cfg := mockAuthConfig()
	stubConnectDeps(t, &cfg, &cap)

	var gotRegion string
	var gotRegionCalls int
	getRDSTokenFn = func(_ context.Context, _ types.ICredentials, _, region, _ string) (string, time.Time, error) {
		gotRegion = region
		gotRegionCalls++
		return connectToken, time.Now().Add(15 * time.Minute), nil
	}

	err := runRDSConnect(&connectOptions{
		Host: "mydb", Port: 5432, Username: "app", Identity: "dev-admin", // Region intentionally empty.
	})
	require.NoError(t, err, "an empty --region must not hard-fail validation")
	assert.Equal(t, 1, gotRegionCalls)
	assert.Empty(t, gotRegion, "the empty region must flow through unchanged for GetRDSToken to resolve from credentials")
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
