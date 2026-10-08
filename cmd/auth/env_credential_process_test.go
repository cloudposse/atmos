package auth

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/aws/credentialprocess"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard so a field rename in AWSCredentials fails the build here.
var _ = authTypes.AWSCredentials{AccessKeyID: "", SecretAccessKey: "", SessionToken: "", Expiration: ""}

const (
	cpAccessKey    = "AKIAIOSFODNN7EXAMPLE"
	cpSecretKey    = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	cpSessionToken = "FwoGZXIvYXdzEBYaDHexampleSessionToken"
)

func cpCaptureStdout(t *testing.T, fn func() error) (string, error) {
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

// cpManager returns a mock manager whose cached credentials are valid for an hour.
func cpManager(t *testing.T, expiration string) *authTypes.MockAuthManager {
	t.Helper()
	mgr := authTypes.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "dev").Return(&authTypes.WhoamiInfo{
		Credentials: &authTypes.AWSCredentials{
			AccessKeyID:     cpAccessKey,
			SecretAccessKey: cpSecretKey,
			SessionToken:    cpSessionToken,
			Expiration:      expiration,
		},
	}, nil).AnyTimes()
	return mgr
}

func TestWriteCredentialProcessDocument_Stdout(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	mgr := cpManager(t, exp)

	out, err := cpCaptureStdout(t, func() error {
		return writeCredentialProcessDocument(context.Background(), viper.New(), mgr, "dev", credentialprocess.DefaultMinValidity)
	})
	require.NoError(t, err)

	// Unmasked, and byte-identical to what `atmos aws credential-process` prints for the same credentials.
	doc, err := credentialprocess.Produce(context.Background(), mgr, "dev")
	require.NoError(t, err)
	assert.Equal(t, credentialprocess.Render(doc), out)
	assert.Equal(t,
		`{"Version":1,"AccessKeyId":"`+cpAccessKey+`","SecretAccessKey":"`+cpSecretKey+
			`","SessionToken":"`+cpSessionToken+`","Expiration":"`+exp+`"}`+"\n",
		out)
}

func TestWriteCredentialProcessDocument_OutputFile(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	mgr := cpManager(t, exp)
	path := filepath.Join(t.TempDir(), "creds.json")
	// Pre-existing content and a permissive mode must be replaced and tightened.
	require.NoError(t, os.WriteFile(path, []byte("stale content that is longer than the new document, "+cpSecretKey), 0o644))

	v := viper.New()
	v.Set(OutputFileFlagName, path)

	out, err := cpCaptureStdout(t, func() error {
		return writeCredentialProcessDocument(context.Background(), v, mgr, "dev", credentialprocess.DefaultMinValidity)
	})
	require.NoError(t, err)
	assert.Empty(t, out, "nothing goes to stdout when --output-file is set")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t,
		`{"Version":1,"AccessKeyId":"`+cpAccessKey+`","SecretAccessKey":"`+cpSecretKey+
			`","SessionToken":"`+cpSessionToken+`","Expiration":"`+exp+`"}`+"\n",
		string(got))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestWriteCredentialProcessDocument_IgnoresGitHubEnv(t *testing.T) {
	mgr := cpManager(t, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	githubEnv := filepath.Join(t.TempDir(), "github_env")
	require.NoError(t, os.WriteFile(githubEnv, []byte("EXISTING=value\n"), 0o600))
	t.Setenv("GITHUB_ENV", githubEnv)

	out, err := cpCaptureStdout(t, func() error {
		return writeCredentialProcessDocument(context.Background(), viper.New(), mgr, "dev", credentialprocess.DefaultMinValidity)
	})
	require.NoError(t, err)
	assert.Contains(t, out, cpAccessKey)

	got, err := os.ReadFile(githubEnv)
	require.NoError(t, err)
	assert.Equal(t, "EXISTING=value\n", string(got), "credential-process must not write to $GITHUB_ENV")
}

func TestWriteCredentialProcessDocument_NonAWSIdentity(t *testing.T) {
	mgr := authTypes.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "dev").
		Return(&authTypes.WhoamiInfo{Credentials: &authTypes.AzureCredentials{}}, nil)

	out, err := cpCaptureStdout(t, func() error {
		return writeCredentialProcessDocument(context.Background(), viper.New(), mgr, "dev", credentialprocess.DefaultMinValidity)
	})
	require.ErrorIs(t, err, errUtils.ErrIdentityNotAWS)
	assert.Empty(t, out)
}

// TestExecuteAuthEnvCommand_CredentialProcessFromEnvVar verifies ATMOS_AUTH_ENV_FORMAT=credential-process
// routes to the credential-process document instead of environment variables. The mock/aws fixture
// produces non-AWS credentials, so reaching ErrIdentityNotAWS proves the branch was taken.
func TestExecuteAuthEnvCommand_CredentialProcessFromEnvVar(t *testing.T) {
	setupMockAuthFixture(t)
	t.Setenv("ATMOS_AUTH_ENV_FORMAT", FormatCredentialProcess)

	cmd := authEnvCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(nil))

	err := executeAuthEnvCommand(cmd, nil)
	require.ErrorIs(t, err, errUtils.ErrIdentityNotAWS)
}

func TestEnvCommand_FormatCredentialProcessAccepted(t *testing.T) {
	assert.Contains(t, SupportedFormats, FormatCredentialProcess)
	assert.Contains(t, authEnvCmd.Flags().Lookup(FormatFlagName).Usage, FormatCredentialProcess)
}

// newCredentialProcessEnvCmd builds a fresh env command and Viper instance bound to the real parser,
// so tests exercise the registered flags and environment bindings without sharing state.
func newCredentialProcessEnvCmd(t *testing.T, args ...string) (*cobra.Command, *viper.Viper) {
	t.Helper()
	cmd := &cobra.Command{Use: "env", Args: cobra.NoArgs}
	envParser.RegisterFlags(cmd)
	// The real --identity flag is persistent on the auth command; mirror its optional-value behavior.
	cmd.Flags().String(IdentityFlagName, "", "")
	cmd.Flags().Lookup(IdentityFlagName).NoOptDefVal = IdentityFlagSelectValue

	v := viper.New()
	require.NoError(t, envParser.BindToViper(v))
	t.Cleanup(func() { require.NoError(t, envParser.BindToViper(viper.GetViper())) })

	require.NoError(t, cmd.ParseFlags(args))
	require.NoError(t, envParser.BindFlagsToViper(cmd, v))
	return cmd, v
}

func TestExecuteCredentialProcessFormat_MinValidity(t *testing.T) {
	// 20 minutes of validity satisfies the 15 minute default but not a 30 minute request.
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
			if tt.env != "" {
				t.Setenv(credentialprocess.MinValidityEnvVar, tt.env)
			}
			mgr := authTypes.NewMockAuthManager(gomock.NewController(t))
			creds := &authTypes.WhoamiInfo{Credentials: &authTypes.AWSCredentials{
				AccessKeyID: cpAccessKey, SecretAccessKey: cpSecretKey, SessionToken: cpSessionToken, Expiration: soon,
			}}
			mgr.EXPECT().GetCachedCredentials(gomock.Any(), "dev").Return(creds, nil)
			mgr.EXPECT().Authenticate(gomock.Any(), "dev").Return(creds, nil).Times(tt.wantAuthCalls)

			cmd, v := newCredentialProcessEnvCmd(t, append([]string{"--identity=dev", "--format=" + FormatCredentialProcess}, tt.args...)...)

			out, err := cpCaptureStdout(t, func() error { return executeCredentialProcessFormat(cmd, v, mgr) })
			require.NoError(t, err)
			assert.Contains(t, out, cpAccessKey)
		})
	}
}

func TestExecuteCredentialProcessFormat_InvalidMinValidity(t *testing.T) {
	mgr := authTypes.NewMockAuthManager(gomock.NewController(t))
	cmd, v := newCredentialProcessEnvCmd(t, "--identity=dev", "--format="+FormatCredentialProcess, "--min-validity=soon")

	out, err := cpCaptureStdout(t, func() error { return executeCredentialProcessFormat(cmd, v, mgr) })
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Empty(t, out)
}

func TestExecuteCredentialProcessFormat_LoginFalseIsRejected(t *testing.T) {
	// The format always authenticates when credentials are missing or about to expire, so an explicit
	// --login=false must fail instead of being silently ignored. The mock has no expectations: nothing runs.
	mgr := authTypes.NewMockAuthManager(gomock.NewController(t))
	cmd, v := newCredentialProcessEnvCmd(t, "--identity=dev", "--format="+FormatCredentialProcess, "--login=false")

	out, err := cpCaptureStdout(t, func() error { return executeCredentialProcessFormat(cmd, v, mgr) })
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "--login=false")
	assert.Empty(t, out)
}

func TestExecuteCredentialProcessFormat_LoginTrueIsAccepted(t *testing.T) {
	mgr := cpManager(t, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	cmd, v := newCredentialProcessEnvCmd(t, "--identity=dev", "--format="+FormatCredentialProcess, "--login")

	out, err := cpCaptureStdout(t, func() error { return executeCredentialProcessFormat(cmd, v, mgr) })
	require.NoError(t, err)
	assert.Contains(t, out, cpAccessKey)
}

func TestExecuteCredentialProcessFormat_NeverPrompts(t *testing.T) {
	// Like `atmos aws credential-process`, the alias must not open the identity selector: the AWS CLI
	// captures stderr and the prompt would hang it. GetDefaultIdentity is not expected on the mock.
	tests := []struct {
		name      string
		args      []string
		identity  map[string]schema.Identity
		wantErrIs error
	}{
		{name: "bare --identity", args: []string{"--identity"}, wantErrIs: errUtils.ErrCredentialProcessIdentityRequired},
		{name: "--identity=false", args: []string{"--identity=false"}, wantErrIs: errUtils.ErrCredentialProcessIdentityRequired},
		{
			name:      "no identity and no default",
			identity:  map[string]schema.Identity{"dev": {Kind: "aws/user"}},
			wantErrIs: errUtils.ErrNoDefaultIdentity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := authTypes.NewMockAuthManager(gomock.NewController(t))
			if tt.identity != nil {
				mgr.EXPECT().GetIdentities().Return(tt.identity)
			}
			cmd, v := newCredentialProcessEnvCmd(t, append([]string{"--format=" + FormatCredentialProcess}, tt.args...)...)

			out, err := cpCaptureStdout(t, func() error { return executeCredentialProcessFormat(cmd, v, mgr) })
			require.ErrorIs(t, err, tt.wantErrIs)
			assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "atmos auth list")
			assert.Empty(t, out)
		})
	}
}

func TestExecuteAuthEnvCommand_MinValidityRequiresCredentialProcessFormat(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authEnvCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags([]string{"--format=bash", "--min-validity=30m"}))

	err := executeAuthEnvCommand(cmd, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), FormatCredentialProcess)
}

func TestEnvCommand_MinValidityFlagMatchesProducerDefault(t *testing.T) {
	f := authEnvCmd.Flags().Lookup(credentialprocess.MinValidityFlagName)
	require.NotNil(t, f)
	assert.Equal(t, credentialprocess.FormatMinValidity(credentialprocess.DefaultMinValidity), f.DefValue)
}
