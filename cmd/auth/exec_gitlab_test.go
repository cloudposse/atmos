package auth

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/aws/eks"
	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/shell"
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
				assert.Equal(t, wantActions, actions(), "the child uses the credentials prepared for this invocation")
				assertGitLabExecSharedProfilesPreserved(t, files)
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
	assertGitLabExecSharedProfilesPreserved(t, files)
}

// Persisted credentials from a different process must not replace the current job token.
func TestAuthExecGitLabRequiresCurrentToken(t *testing.T) {
	for _, target := range []string{"runner", "production"} {
		t.Run(target, func(t *testing.T) {
			files, actions := setupGitLabExecFixture(t, false)
			require.NoError(t, awsCloud.SetupFiles("gitlab", target, &types.AWSCredentials{
				AccessKeyID: "previous-job-key", SecretAccessKey: "synthetic-secret", SessionToken: "previous-job-session",
				Region: "eu-west-1", Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			}, "", files.GetRealm()))
			t.Setenv("ATMOS_OIDC_TOKEN", "")
			cmd := newIdentityFlowTestCmd(t)
			require.NoError(t, cmd.ParseFlags([]string{"--identity=" + target, "--", "must-not-run"}))

			err := executeAuthExecCommand(cmd, nil)

			require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
			assert.Empty(t, actions(), "missing token must fail before STS or command execution")
		})
	}
}

// A current token must refresh persisted profiles without changing them or sharing their paths.
func TestAuthExecGitLabRefreshesPersistedCredentials(t *testing.T) {
	for _, target := range []string{"runner", "production"} {
		t.Run(target, func(t *testing.T) {
			files, actions := setupGitLabExecFixture(t, false)
			require.NoError(t, awsCloud.SetupFiles("gitlab", target, &types.AWSCredentials{
				AccessKeyID: "previous-job-key", SecretAccessKey: "synthetic-secret", SessionToken: "previous-job-session",
				Region: "eu-west-1", Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			}, "", files.GetRealm()))
			before, err := os.ReadFile(files.GetCredentialsPath("gitlab"))
			require.NoError(t, err)
			binary, err := os.Executable()
			require.NoError(t, err)
			t.Setenv("ATMOS_TEST_GITLAB_EXEC_CHILD", "1")
			t.Setenv("ATMOS_TEST_GITLAB_EXEC_EXIT_CODE", "0")
			t.Setenv("ATMOS_TEST_GITLAB_EXEC_TARGET", target)
			cmd := newIdentityFlowTestCmd(t)
			require.NoError(t, cmd.ParseFlags([]string{"--identity=" + strings.ToUpper(target), "--", binary, "-test.run=^TestGitLabExecChildProcess$"}))

			require.NoError(t, executeAuthExecCommand(cmd, nil))

			want := []string{"AssumeRoleWithWebIdentity"}
			if target == "production" {
				want = append(want, "AssumeRole")
			}
			assert.Equal(t, want, actions())
			after, err := os.ReadFile(files.GetCredentialsPath("gitlab"))
			require.NoError(t, err)
			assert.Equal(t, before, after, "existing login credentials must remain unchanged")
		})
	}
}

// TestAuthExecGitLabPartialFileCleanup fails config setup after credentials are written.
func TestAuthExecGitLabPartialFileCleanup(t *testing.T) {
	var files *awsCloud.AWSFileManager
	files, actions := setupGitLabExecFixture(t, false, func() error {
		dirs, err := filepath.Glob(filepath.Join(os.TempDir(), "atmos-auth-exec-*"))
		if err != nil {
			return err
		}
		if len(dirs) != 1 {
			return fmt.Errorf("expected one invocation directory, got %d", len(dirs))
		}
		privateFiles, err := awsCloud.NewAWSFileManager(dirs[0], files.GetRealm())
		if err != nil {
			return err
		}
		return os.MkdirAll(privateFiles.GetConfigPath("gitlab"), 0o700)
	})
	cmd := newIdentityFlowTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=runner", "--", "must-not-run"}))

	err := executeAuthExecCommand(cmd, nil)

	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Equal(t, []string{"AssumeRoleWithWebIdentity"}, actions())
	assertGitLabExecSharedProfilesPreserved(t, files)
}

// Two live invocation contexts must keep independent files, including when one finishes first.
func TestAuthExecGitLabOverlappingInvocations(t *testing.T) {
	files, actions := setupGitLabExecFixture(t, false)
	binary, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_CHILD", "1")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_EXIT_CODE", "0")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_TARGET", "runner")
	var contexts []*authExecContext
	for range 2 {
		cmd := newIdentityFlowTestCmd(t)
		require.NoError(t, cmd.ParseFlags([]string{"--identity=runner"}))
		execCtx, err := prepareAuthenticatedEnv(cmd, viper.GetViper())
		require.NoError(t, err)
		contexts = append(contexts, execCtx)
		t.Cleanup(func() { require.NoError(t, cleanupGitLabAuthExec(execCtx)) })
	}
	require.NotEqual(t, contexts[0].credentialDir, contexts[1].credentialDir)
	for _, execCtx := range contexts {
		info, err := os.Stat(execCtx.credentialDir)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			assert.Zero(t, info.Mode().Perm()&0o077, "private directories exclude other users")
		}
	}

	require.NoError(t, cleanupGitLabAuthExec(contexts[0]))
	assert.NoDirExists(t, contexts[0].credentialDir)
	require.NoError(t, shell.RunCommand([]string{binary, "-test.run=^TestGitLabExecChildProcess$"}, contexts[1].envList))
	require.NoError(t, cleanupGitLabAuthExec(contexts[1]))

	assert.Equal(t, []string{"AssumeRoleWithWebIdentity", "AssumeRoleWithWebIdentity"}, actions(),
		"each invocation must authenticate independently with the current job token")
	assertGitLabExecSharedProfilesPreserved(t, files)
}

func TestAuthExecGitLabMissingCommandCleanup(t *testing.T) {
	files, _ := setupGitLabExecFixture(t, false)
	cmd := newIdentityFlowTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=runner", "--", "atmos-test-nonexistent-command"}))

	err := executeAuthExecCommand(cmd, nil)

	require.ErrorIs(t, err, errUtils.ErrCommandNotFound)
	assertGitLabExecSharedProfilesPreserved(t, files)
}

// Kubectl's Atmos exec plugin must inherit the outer invocation's credential storage.
func TestAuthExecGitLabNestedEKSToken(t *testing.T) {
	files, actions := setupGitLabExecFixture(t, false)
	binary, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_CHILD", "1")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_EXIT_CODE", "0")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_TARGET", "runner")
	t.Setenv("ATMOS_TEST_GITLAB_EXEC_EKS", "1")
	cmd := newIdentityFlowTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--identity=runner", "--", binary, "-test.run=^TestGitLabExecChildProcess$"}))

	require.NoError(t, executeAuthExecCommand(cmd, nil))

	assert.Equal(t, []string{"AssumeRoleWithWebIdentity", "AssumeRoleWithWebIdentity"}, actions())
	assertGitLabExecSharedProfilesPreserved(t, files)
}

// TestGitLabExecChildProcess is a portable child command that reads its role's
// credentials twice before returning the requested exit status to the parent.
func TestGitLabExecChildProcess(t *testing.T) {
	if os.Getenv("ATMOS_TEST_GITLAB_EXEC_CHILD") != "1" {
		return
	}
	profile := os.Getenv("ATMOS_TEST_GITLAB_EXEC_TARGET")
	if os.Getenv("ATMOS_TEST_GITLAB_EXEC_EKS") == "1" {
		eks.EksCmd.SetArgs([]string{"token", "--cluster-name=test", "--region=eu-west-1", "--identity=" + profile})
		require.NoError(t, eks.EksCmd.Execute())
	}
	for range 2 {
		credentials, err := awsCloud.LoadINIFile(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
		require.NoError(t, err)
		section, err := credentials.GetSection(profile)
		require.NoError(t, err)
		assert.Equal(t, profile+"-key", section.Key("aws_access_key_id").String())
		assert.Equal(t, profile+"-session", section.Key("aws_session_token").String())
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
func setupGitLabExecFixture(t *testing.T, failChainedRole bool, beforeResponse ...func() error) (*awsCloud.AWSFileManager, func() []string) {
	t.Helper()
	setupMockAuthFixture(t)
	tempRoot := t.TempDir()
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, tempRoot)
	}
	t.Cleanup(func() {
		dirs, err := filepath.Glob(filepath.Join(tempRoot, "atmos-auth-exec-*"))
		require.NoError(t, err)
		assert.Empty(t, dirs, "invocation-owned files must be removed on every return path")
	})
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
		for _, hook := range beforeResponse {
			if !assert.NoError(t, hook()) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
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
		for _, storageRealm := range []string{"", realm} {
			require.NoError(t, awsCloud.SetupFiles(provider, "unrelated", &types.AWSCredentials{
				AccessKeyID: "unrelated-key", SecretAccessKey: "synthetic-secret", SessionToken: "unrelated-session",
				Region: "eu-west-1", Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			}, "", storageRealm))
		}
	}
	return files, func() []string {
		actionsMu.Lock()
		defer actionsMu.Unlock()
		return slices.Clone(actions)
	}
}

// assertGitLabExecSharedProfilesPreserved checks that existing login profiles remain usable.
func assertGitLabExecSharedProfilesPreserved(t *testing.T, files *awsCloud.AWSFileManager) {
	t.Helper()
	for _, storageRealm := range []string{"", files.GetRealm()} {
		shared, err := awsCloud.NewAWSFileManager("", storageRealm)
		require.NoError(t, err)
		for _, provider := range []string{"gitlab", "other-provider"} {
			for _, path := range []string{shared.GetCredentialsPath(provider), shared.GetConfigPath(provider)} {
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
}
