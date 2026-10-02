package aws

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentialprocess"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard so a field rename in AWSCredentials fails the build here.
var _ = types.AWSCredentials{AccessKeyID: "", SecretAccessKey: "", SessionToken: "", Expiration: ""}

const (
	testAccessKey    = "AKIAIOSFODNN7EXAMPLE"
	testSecretKey    = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	testSessionToken = "FwoGZXIvYXdzEBYaDHexampleSessionToken"
)

// initTestIO initializes the IO context for tests that write through pkg/data.
func initTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(func() { data.Reset() })
}

// newTestCredentialProcessCmd builds a fresh command and Viper instance so tests share no state.
func newTestCredentialProcessCmd(t *testing.T) (*cobra.Command, *viper.Viper) {
	t.Helper()
	cmd := &cobra.Command{Use: "credential-process", Args: cobra.NoArgs}
	credentialProcessParser.RegisterFlags(cmd)

	v := viper.New()
	require.NoError(t, credentialProcessParser.BindToViper(v))
	t.Cleanup(func() { require.NoError(t, credentialProcessParser.BindToViper(viper.GetViper())) })
	return cmd, v
}

// stubDependencies swaps the config and auth manager seams for the duration of the test.
func stubDependencies(t *testing.T, mgr types.AuthManager, newManagerErr error) {
	t.Helper()
	origInit, origNew := initCliConfigFn, newAuthManagerFn
	t.Cleanup(func() { initCliConfigFn, newAuthManagerFn = origInit, origNew })

	initCliConfigFn = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{CliConfigPath: "/unused"}, nil
	}
	newAuthManagerFn = func(*schema.AuthConfig, string) (types.AuthManager, error) {
		return mgr, newManagerErr
	}
}

// captureStdout runs fn while redirecting os.Stdout and returns what was written.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	runErr := fn()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out), runErr
}

func testCreds(expiration string) *types.WhoamiInfo {
	return &types.WhoamiInfo{Credentials: &types.AWSCredentials{
		AccessKeyID:     testAccessKey,
		SecretAccessKey: testSecretKey,
		SessionToken:    testSessionToken,
		Expiration:      expiration,
	}}
}

func TestCredentialProcessCmd_Structure(t *testing.T) {
	assert.Equal(t, "credential-process", credentialProcessCmd.Use)
	assert.True(t, credentialProcessCmd.SilenceUsage)
	assert.NotNil(t, credentialProcessCmd.Flags().Lookup("identity"))

	minValidity := credentialProcessCmd.Flags().Lookup(minValidityFlagName)
	require.NotNil(t, minValidity)
	assert.Equal(t, "15m", minValidity.DefValue)
	// The displayed default must stay in sync with the producer's default.
	parsed, err := time.ParseDuration(minValidity.DefValue)
	require.NoError(t, err)
	assert.Equal(t, credentialprocess.DefaultMinValidity, parsed)

	found, _, err := awsCmd.Find([]string{"credential-process"})
	require.NoError(t, err)
	assert.Same(t, credentialProcessCmd, found)
}

func TestExecuteCredentialProcess_PrintsUnmaskedJSON(t *testing.T) {
	initTestIO(t)
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "dev-admin").Return(testCreds(exp.Format(time.RFC3339)), nil)
	stubDependencies(t, mgr, nil)

	cmd, v := newTestCredentialProcessCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=dev-admin"}))

	out, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
	require.NoError(t, err)

	// The credentials must appear verbatim: data.Write would have masked them.
	want := `{"Version":1,"AccessKeyId":"` + testAccessKey + `","SecretAccessKey":"` + testSecretKey +
		`","SessionToken":"` + testSessionToken + `","Expiration":"` + exp.Format(time.RFC3339) + `"}` + "\n"
	assert.Equal(t, want, out)
}

func TestExecuteCredentialProcess_AuthenticatesWhenNotCached(t *testing.T) {
	initTestIO(t)
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "dev-admin").Return(nil, errUtils.ErrNoCredentialsFound)
	mgr.EXPECT().Authenticate(gomock.Any(), "dev-admin").Return(testCreds(""), nil)
	stubDependencies(t, mgr, nil)

	cmd, v := newTestCredentialProcessCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=dev-admin"}))

	out, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
	require.NoError(t, err)
	assert.Contains(t, out, testSessionToken)
	assert.NotContains(t, out, "Expiration")
}

func TestExecuteCredentialProcess_DefaultIdentity(t *testing.T) {
	initTestIO(t)
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetDefaultIdentity(false).Return("default-id", nil)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "default-id").Return(testCreds(time.Now().Add(time.Hour).UTC().Format(time.RFC3339)), nil)
	stubDependencies(t, mgr, nil)

	cmd, v := newTestCredentialProcessCmd(t)

	out, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
	require.NoError(t, err)
	assert.Contains(t, out, testAccessKey)
}

func TestExecuteCredentialProcess_IdentityFromEnv(t *testing.T) {
	initTestIO(t)
	t.Setenv("ATMOS_IDENTITY", "env-identity")
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "env-identity").Return(testCreds(time.Now().Add(time.Hour).UTC().Format(time.RFC3339)), nil)
	stubDependencies(t, mgr, nil)

	cmd, v := newTestCredentialProcessCmd(t)

	out, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
	require.NoError(t, err)
	assert.Contains(t, out, testAccessKey)
}

func TestExecuteCredentialProcess_FlagOverridesEnvIdentity(t *testing.T) {
	initTestIO(t)
	t.Setenv("ATMOS_IDENTITY", "env-identity")
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "flag-identity").Return(testCreds(time.Now().Add(time.Hour).UTC().Format(time.RFC3339)), nil)
	stubDependencies(t, mgr, nil)

	cmd, v := newTestCredentialProcessCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=flag-identity"}))

	_, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
	require.NoError(t, err)
}

func TestExecuteCredentialProcess_SelectWithoutTTY(t *testing.T) {
	initTestIO(t)
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetDefaultIdentity(true).Return("", errUtils.ErrIdentitySelectionRequiresTTY)
	stubDependencies(t, mgr, nil)

	cmd, v := newTestCredentialProcessCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity"}))

	out, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
	require.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
	assert.Empty(t, out)
}

func TestExecuteCredentialProcess_MinValidityFlagAndEnv(t *testing.T) {
	soon := time.Now().Add(20 * time.Minute).UTC().Format(time.RFC3339)

	tests := []struct {
		name          string
		args          []string
		env           string
		wantAuthCalls int
	}{
		{name: "default 15m accepts 20m of validity", wantAuthCalls: 0},
		{name: "flag 30m forces authentication", args: []string{"--min-validity=30m"}, wantAuthCalls: 1},
		{name: "env 30m forces authentication", env: "30m", wantAuthCalls: 1},
		{name: "flag overrides env", args: []string{"--min-validity=1m"}, env: "30m", wantAuthCalls: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initTestIO(t)
			if tt.env != "" {
				t.Setenv(minValidityEnvVar, tt.env)
			}
			mgr := types.NewMockAuthManager(gomock.NewController(t))
			mgr.EXPECT().GetCachedCredentials(gomock.Any(), "dev").Return(testCreds(soon), nil)
			mgr.EXPECT().Authenticate(gomock.Any(), "dev").Return(testCreds(""), nil).Times(tt.wantAuthCalls)
			stubDependencies(t, mgr, nil)

			cmd, v := newTestCredentialProcessCmd(t)
			require.NoError(t, cmd.ParseFlags(append([]string{"--identity=dev"}, tt.args...)))

			_, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
			require.NoError(t, err)
		})
	}
}

func TestExecuteCredentialProcess_InvalidMinValidity(t *testing.T) {
	for _, value := range []string{"soon", "-5m", ""} {
		t.Run(value, func(t *testing.T) {
			initTestIO(t)
			stubDependencies(t, types.NewMockAuthManager(gomock.NewController(t)), nil)

			cmd, v := newTestCredentialProcessCmd(t)
			require.NoError(t, cmd.ParseFlags([]string{"--identity=dev", "--min-validity=" + value}))

			out, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
			require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
			assert.Empty(t, out)
		})
	}
}

func TestExecuteCredentialProcess_AuthFailure(t *testing.T) {
	initTestIO(t)
	cause := errors.New("sso session expired")
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "dev").Return(nil, errUtils.ErrNoCredentialsFound)
	mgr.EXPECT().Authenticate(gomock.Any(), "dev").Return(nil, cause)
	stubDependencies(t, mgr, nil)

	cmd, v := newTestCredentialProcessCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=dev"}))

	out, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
	require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
	require.ErrorIs(t, err, cause)
	assert.Empty(t, out)
}

func TestExecuteCredentialProcess_NonAWSIdentity(t *testing.T) {
	initTestIO(t)
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "azure-id").
		Return(&types.WhoamiInfo{Credentials: &types.AzureCredentials{}}, nil)
	stubDependencies(t, mgr, nil)

	cmd, v := newTestCredentialProcessCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=azure-id"}))

	out, err := captureStdout(t, func() error { return executeCredentialProcess(cmd, v) })
	require.ErrorIs(t, err, errUtils.ErrIdentityNotAWS)
	assert.Empty(t, out)
}

func TestExecuteCredentialProcess_ConfigInitFailure(t *testing.T) {
	initTestIO(t)
	stubDependencies(t, nil, nil)
	initCliConfigFn = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errors.New("bad atmos.yaml")
	}

	cmd, v := newTestCredentialProcessCmd(t)
	err := executeCredentialProcess(cmd, v)
	require.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

func TestExecuteCredentialProcess_ManagerInitFailure(t *testing.T) {
	initTestIO(t)
	stubDependencies(t, nil, errors.New("keyring unavailable"))

	cmd, v := newTestCredentialProcessCmd(t)
	err := executeCredentialProcess(cmd, v)
	require.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
}
