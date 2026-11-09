package auth

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// TestAuthExecGitLabCredentialCleanup exercises real command execution and mock STS.
// The selected role's credentials must remain usable during the child and disappear at exit.
func TestAuthExecGitLabCredentialCleanup(t *testing.T) {
	for _, target := range []string{"runner", "production"} {
		for _, exitCode := range []int{0, 7} {
			t.Run(fmt.Sprintf("%s/exit-%d", target, exitCode), func(t *testing.T) {
				files, actions := setupGitLabExecFixture(t, false)
				binary, err := os.Executable()
				require.NoError(t, err)
				t.Setenv("ATMOS_TEST_GITLAB_EXEC_CHILD", "1")
				t.Setenv("ATMOS_TEST_GITLAB_EXEC_EXIT_CODE", strconv.Itoa(exitCode))
				t.Setenv("ATMOS_TEST_GITLAB_EXEC_TARGET", target)
				cmd := newIdentityFlowTestCmd(t)
				require.NoError(t, cmd.ParseFlags([]string{"--identity=" + target, "--", binary, "-test.run=^TestGitLabExecChildProcess$"}))

				err = executeAuthExecCommand(cmd, nil)

				if exitCode == 0 {
					require.NoError(t, err)
				} else {
					var exitErr errUtils.ExitCodeError
					require.ErrorAs(t, err, &exitErr)
					assert.Equal(t, exitCode, exitErr.Code)
				}
				wantActions := []string{"AssumeRoleWithWebIdentity"}
				if target == "production" {
					wantActions = append(wantActions, "AssumeRole")
				}
				assert.Equal(t, wantActions, actions(), "credentials are reused throughout the command")
				assertGitLabExecProfilesRemoved(t, files)
			})
		}
	}
}

// TestAuthExecGitLabPartialAuthenticationCleanup checks that a chained role failure
// retains its authentication error and does not remove unrelated profiles.
func TestAuthExecGitLabPartialAuthenticationCleanup(t *testing.T) {
	files, actions := setupGitLabExecFixture(t, true)
	cmd := newIdentityFlowTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=production", "--", "must-not-run"}))

	err := executeAuthExecCommand(cmd, nil)

	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Equal(t, []string{"AssumeRoleWithWebIdentity", "AssumeRole"}, actions())
	assertGitLabExecProfilesRemoved(t, files)
}

// TestAuthExecGitLabPartialFileCleanup reproduces a config-write failure after
// credentials have been written. Cleanup must still remove the credential profile
// and report its own failure to clean the deliberately broken config path.
func TestAuthExecGitLabPartialFileCleanup(t *testing.T) {
	files, actions := setupGitLabExecFixture(t, false)
	configPath := files.GetConfigPath("gitlab")
	require.NoError(t, os.Remove(configPath))
	require.NoError(t, os.Mkdir(configPath, 0o700))
	cmd := newIdentityFlowTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=runner", "--", "must-not-run"}))

	err := executeAuthExecCommand(cmd, nil)

	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	require.ErrorIs(t, err, errUtils.ErrPartialLogout)
	assert.Equal(t, []string{"AssumeRoleWithWebIdentity"}, actions())
	contents, err := awsCloud.LoadINIFile(files.GetCredentialsPath("gitlab"))
	require.NoError(t, err)
	assert.Equal(t, []string{"DEFAULT", "unrelated"}, contents.SectionStrings())
	assert.Equal(t, "unrelated-key", contents.Section("unrelated").Key("aws_access_key_id").String())
	assert.DirExists(t, configPath)
}

// TestAuthExecGitLabRemovesEmptyFiles verifies that no credential or config file
// remains when the invocation owned the provider's only profile.
func TestAuthExecGitLabRemovesEmptyFiles(t *testing.T) {
	files, _ := setupGitLabExecFixture(t, false)
	require.NoError(t, files.DeleteIdentity(t.Context(), "gitlab", "unrelated"))
	binary, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_CHILD", "1")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_EXIT_CODE", "0")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_TARGET", "runner")
	cmd := newIdentityFlowTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=runner", "--", binary, "-test.run=^TestGitLabExecChildProcess$"}))

	require.NoError(t, executeAuthExecCommand(cmd, nil))

	assert.NoFileExists(t, files.GetCredentialsPath("gitlab"))
	assert.NoFileExists(t, files.GetConfigPath("gitlab"))
	assert.FileExists(t, files.GetCredentialsPath("other-provider"))
}

// TestAuthExecGitLabCleanupFailure preserves a failed child's exit code while
// reporting failed config cleanup and still removing the credential profile.
func TestAuthExecGitLabCleanupFailure(t *testing.T) {
	files, _ := setupGitLabExecFixture(t, false)
	binary, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_CHILD", "1")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_EXIT_CODE", "7")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_TARGET", "runner")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_BREAK_CONFIG", "1")
	cmd := newIdentityFlowTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=runner", "--", binary, "-test.run=^TestGitLabExecChildProcess$"}))

	err = executeAuthExecCommand(cmd, nil)

	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
	require.ErrorIs(t, err, errUtils.ErrPartialLogout)
	contents, err := awsCloud.LoadINIFile(files.GetCredentialsPath("gitlab"))
	require.NoError(t, err)
	assert.Equal(t, []string{"DEFAULT", "unrelated"}, contents.SectionStrings())
}

// TestGitLabExecChildProcess is a portable child command that reads its role's
// credentials twice before returning the requested exit status to the parent.
func TestGitLabExecChildProcess(t *testing.T) {
	if os.Getenv("ATMOS_TEST_GITLAB_EXEC_CHILD") != "1" {
		return
	}
	profile := os.Getenv("ATMOS_TEST_GITLAB_EXEC_TARGET")
	for range 2 {
		credentials, err := awsCloud.LoadINIFile(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
		require.NoError(t, err)
		section, err := credentials.GetSection(profile)
		require.NoError(t, err)
		assert.Equal(t, profile+"-key", section.Key("aws_access_key_id").String())
		assert.Equal(t, profile+"-session", section.Key("aws_session_token").String())
	}
	if os.Getenv("ATMOS_TEST_GITLAB_EXEC_BREAK_CONFIG") == "1" {
		path := os.Getenv("AWS_CONFIG_FILE")
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.Mkdir(path, 0o700))
	}
	exitCode, err := strconv.Atoi(os.Getenv("ATMOS_TEST_GITLAB_EXEC_EXIT_CODE"))
	require.NoError(t, err)
	if t.Failed() {
		os.Exit(1)
	}
	os.Exit(exitCode)
}

// setupGitLabExecFixture isolates all credential storage and serves synthetic
// AWS role credentials locally; it never contacts GitLab or AWS.
func setupGitLabExecFixture(t *testing.T, failChainedRole bool) (*awsCloud.AWSFileManager, func() []string) {
	t.Helper()
	setupMockAuthFixture(t)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ATMOS_AUTH_REALM", "")
	t.Setenv("ATMOS_OIDC_TOKEN", "synthetic-gitlab-job-token")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	sdkConfig := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(sdkConfig, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(sdkConfig, "credentials"))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	realm := "gitlab-exec-" + strings.ToLower(rand.Text())
	actions := []string{}
	var actionsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, r.ParseForm()) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		action := r.Form.Get("Action")
		actionsMu.Lock()
		actions = append(actions, action)
		actionsMu.Unlock()
		if failChainedRole && action == "AssumeRole" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `<ErrorResponse><Error><Code>AccessDenied</Code><Message>test rejection</Message></Error></ErrorResponse>`)
			return
		}
		role := "runner"
		if action == "AssumeRole" {
			role = "production"
		} else {
			assert.Equal(t, "AssumeRoleWithWebIdentity", action)
			assert.Equal(t, "synthetic-gitlab-job-token", r.Form.Get("WebIdentityToken"))
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprintf(w, `<%[1]sResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><%[1]sResult><Credentials><AccessKeyId>%[2]s-key</AccessKeyId><SecretAccessKey>synthetic-secret</SecretAccessKey><SessionToken>%[2]s-session</SessionToken><Expiration>%[3]s</Expiration></Credentials></%[1]sResult></%[1]sResponse>`, action, role, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	t.Cleanup(server.Close)
	config := fmt.Sprintf(`base_path: .
auth:
  realm: %q
  keyring:
    type: memory
  providers:
    gitlab:
      kind: gitlab/oidc
  identities:
    runner:
      kind: aws/assume-role
      via:
        provider: gitlab
      principal:
        assume_role: arn:aws:iam::123456789012:role/GitLabRunner
        region: eu-west-1
      spec:
        endpoint_url: %q
    production:
      kind: aws/assume-role
      via:
        identity: runner
      principal:
        assume_role: arn:aws:iam::123456789012:role/GitLabProduction
        region: eu-west-1
      spec:
        endpoint_url: %q
`, realm, server.URL, server.URL)
	require.NoError(t, os.WriteFile("atmos.yaml", []byte(config), 0o600))
	viper.Reset()
	files, err := awsCloud.NewAWSFileManager("", realm)
	require.NoError(t, err)
	// Preserve profiles outside this invocation, including one sharing its provider.
	for _, provider := range []string{"gitlab", "other-provider"} {
		require.NoError(t, awsCloud.SetupFiles(provider, "unrelated", &types.AWSCredentials{
			AccessKeyID: "unrelated-key", SecretAccessKey: "synthetic-secret", SessionToken: "unrelated-session",
			Region: "eu-west-1", Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}, "", realm))
	}
	return files, func() []string {
		actionsMu.Lock()
		defer actionsMu.Unlock()
		return slices.Clone(actions)
	}
}

// assertGitLabExecProfilesRemoved checks both credential and config profiles
// while proving cleanup preserves credentials unrelated to this invocation.
func assertGitLabExecProfilesRemoved(t *testing.T, files *awsCloud.AWSFileManager) {
	t.Helper()
	for _, provider := range []string{"gitlab", "other-provider"} {
		for _, path := range []string{files.GetCredentialsPath(provider), files.GetConfigPath(provider)} {
			contents, err := awsCloud.LoadINIFile(path)
			require.NoError(t, err)
			if strings.HasSuffix(path, "credentials") {
				assert.Equal(t, []string{"DEFAULT", "unrelated"}, contents.SectionStrings())
				assert.Equal(t, "unrelated-key", contents.Section("unrelated").Key("aws_access_key_id").String())
			} else {
				assert.Equal(t, []string{"DEFAULT", "profile unrelated"}, contents.SectionStrings())
			}
		}
	}
}
